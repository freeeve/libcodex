package bibframe

import (
	"sort"
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

func recordFromWorkInstance(g *rdf.Graph, work, inst rdf.Term, hasInst bool) *codex.Record {
	rec := codex.NewRecord()
	rec.SetLeader(leaderFor(typeExcept(g, work, "Work"), issuanceCode(g, inst)))

	var fields []codex.Field
	add := func(f codex.Field) { fields = append(fields, f) }

	if id := controlNumber(work.Value); id != "" {
		add(codex.NewControlField("001", id))
	}
	if f, ok := catalogingSourceField(g, work, inst); ok {
		add(f)
	}

	instTitle := titleOf(g, work, inst, hasInst)
	contribs, hasPrimary := contributions(g, work)

	// Uniform title (130/240): the Work's own title when it differs from the
	// transcribed title the Instance carries.
	if wt := firstTitle(g, work); wt.MainTitle != "" && wt.MainTitle != instTitle.MainTitle {
		tag := "130"
		if hasPrimary {
			tag = "240"
		}
		add(codex.NewDataField(tag, '0', ' ', titleSubfields(wt, "")...))
	}
	if instTitle.MainTitle != "" {
		resp := literal(g, inst, pRespStmt)
		ind1 := byte('0')
		if hasPrimary {
			ind1 = '1'
		}
		add(codex.NewDataField("245", ind1, titleInd2(instTitle.NonSortNum), titleSubfields(instTitle, resp)...))
	}
	fields = append(fields, variantTitleFields(g, work)...)
	fields = append(fields, variantTitleFields(g, inst)...)

	fields = append(fields, contribs...)
	fields = append(fields, relatedWorkFields(g, work)...)
	fields = append(fields, relationFields(g, work)...)
	fields = append(fields, subjectFields(g, work)...)
	fields = append(fields, identifierFields(g, inst)...)
	fields = append(fields, classificationFields(g, work)...)
	fields = append(fields, languageField(g, work)...)

	for _, gf := range labelsOf(g, work, pGenreForm) {
		add(codex.NewDataField("655", ' ', '0', codex.NewSubfield('a', gf)))
	}
	if ed := literal(g, inst, pEdition); ed != "" {
		add(codex.NewDataField("250", ' ', ' ', codex.NewSubfield('a', ed)))
	}
	// A Work's bf:relation series nodes are the current shape. Graphs written
	// before v0.25.0 -- and by producers that copied that shape -- carry flat
	// bf:seriesStatement literals on the Instance instead; read those only when
	// the Work has no series relations, so a graph carrying both does not
	// duplicate its 490s.
	seriesFs := seriesFields(g, work)
	if len(seriesFs) == 0 {
		seriesFs = legacySeriesFields(g, inst)
	}
	for _, f := range seriesFs {
		add(f)
	}
	if f := durationField(g, inst); f != nil {
		add(*f)
	}
	if f := digitalCharacteristicsField(g, inst); f != nil {
		add(*f)
	}
	for _, f := range provisionFields(g, inst) {
		add(f)
	}
	if f, ok := control008(g, work, inst); ok {
		add(f)
	}
	for _, f := range physicalFields(g, inst) {
		add(f)
	}
	if c := contentField(g, work); c != nil {
		add(*c)
	}
	for _, m := range rdaFields(g, inst, pMedia, "337", mediaVocab, "rdamedia") {
		add(m)
	}
	for _, c := range rdaFields(g, inst, pCarrier, "338", carrierVocab, "rdacarrier") {
		add(c)
	}
	fields = append(fields, codedFields(g, inst, rec.Leader().RecordType())...)
	for _, s := range labelsOf(g, work, pSummary) {
		add(codex.NewDataField("520", ' ', ' ', codex.NewSubfield('a', s)))
	}
	for _, toc := range literalsOf(g, work, pTableOfContents) {
		add(codex.NewDataField("505", ' ', ' ', codex.NewSubfield('a', toc)))
	}
	fields = append(fields, noteFields(g, work)...)
	fields = append(fields, noteFields(g, inst)...)
	for _, loc := range g.Objects(inst, pLocator) {
		if f, ok := locatorField(g, loc); ok {
			add(f)
		}
	}

	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Tag < fields[j].Tag })
	for _, f := range fields {
		rec.AddField(f)
	}
	return rec
}

// titleOf returns the transcribed title to render as 245: the Instance's title,
// or the Work's when there is no Instance title.
func titleOf(g *rdf.Graph, work, inst rdf.Term, hasInst bool) Title {
	if hasInst {
		if t := firstTitle(g, inst); t.MainTitle != "" {
			return t
		}
	}
	return firstTitle(g, work)
}

// contributions reverses bf:contribution into 1xx/7xx fields and reports whether
// a primary (1xx) contribution is present.
func contributions(g *rdf.Graph, work rdf.Term) ([]codex.Field, bool) {
	var fields []codex.Field
	primary := false
	for _, c := range g.Objects(work, pContribution) {
		agent, ok := g.Object(c, pAgent)
		if !ok {
			continue
		}
		label := literal(g, agent, pLabel)
		if label == "" {
			continue
		}
		isPrimary := g.HasType(c, primaryContribution) || g.HasType(c, bfPrimaryContribution)
		class := agentClass(g, agent)
		tag := contribTag(class, isPrimary)
		subs := append([]codex.Subfield{codex.NewSubfield('a', label)}, roleSubfields(g, c, class)...)
		fields = append(fields, codex.NewDataField(tag, ind1ForClass(class), ' ', subs...))
		primary = primary || isPrimary
	}
	return fields, primary
}

// relatedWorkFields reverses bf:relatedTo name-title nodes into 1xx/7xx fields
// carrying the creator name in $a and the related work's title in $t. The 1xx vs
// 7xx tag and the first indicator follow the related work's creator (primary
// contribution -> 1xx, agent class -> tag/ind1), inverting emitRelatedWork.
func relatedWorkFields(g *rdf.Graph, work rdf.Term) []codex.Field {
	var fields []codex.Field
	for _, rw := range g.Objects(work, pRelatedTo) {
		var name, class string
		primary := false
		if c, ok := g.Object(rw, pContribution); ok {
			if agent, ok := g.Object(c, pAgent); ok {
				name = literal(g, agent, pLabel)
				class = agentClass(g, agent)
			}
			primary = g.HasType(c, primaryContribution) || g.HasType(c, bfPrimaryContribution)
		}
		title := firstTitle(g, rw).MainTitle
		if name == "" && title == "" {
			continue
		}
		var subs []codex.Subfield
		if name != "" {
			subs = append(subs, codex.NewSubfield('a', name))
		}
		if title != "" {
			subs = append(subs, codex.NewSubfield('t', title))
		}
		fields = append(fields, codex.NewDataField(contribTag(class, primary), ind1ForClass(class), ' ', subs...))
	}
	return fields
}

// relationFields reverses a Work's bf:relation nodes into 76x-78x linking fields:
// the tag and second indicator come from the bf:relationship code, and the linked
// resource's creator ($a), title ($t) and ISSN ($x) come from its
// bf:associatedResource Work. It inverts emitRelation.
func relationFields(g *rdf.Graph, work rdf.Term) []codex.Field {
	var fields []codex.Field
	for _, rel := range g.Objects(work, pRelation) {
		tag, ind2, ok := relationField(relationshipCode(g, rel))
		if !ok {
			continue
		}
		res, ok := g.Object(rel, pAssociatedResource)
		if !ok {
			continue
		}
		var subs []codex.Subfield
		if name := associatedName(g, res); name != "" {
			subs = append(subs, codex.NewSubfield('a', name))
		}
		if title := firstTitle(g, res).MainTitle; title != "" {
			subs = append(subs, codex.NewSubfield('t', title))
		}
		issn, isbn := associatedIdentifiers(g, res)
		if issn != "" {
			subs = append(subs, codex.NewSubfield('x', issn))
		}
		if isbn != "" {
			subs = append(subs, codex.NewSubfield('z', isbn))
		}
		if len(subs) == 0 {
			continue
		}
		fields = append(fields, codex.NewDataField(tag, ' ', ind2, subs...))
	}
	return fields
}

// codedFields rebuilds minimal 006/007 control fields from the Instance's RDA
// media and carrier terms -- the derive-don't-fabricate shape of the partial 008
// reconstruction. Each carrier with a 007 mapping yields a 2-byte 007
// (category + specific material designation); a computer media type on a record
// whose leader is not itself a computer file yields the 006 'm' electronic
// aspect, with the remaining positions left as fill.
func codedFields(g *rdf.Graph, inst rdf.Term, leaderType byte) []codex.Field {
	var fields []codex.Field
	seen := map[string]bool{}
	for _, node := range g.Objects(inst, pCarrier) {
		code := rdaValue(node, carrierVocab)
		if coded, ok := f007ForCarrier(code); ok && !seen[coded] {
			seen[coded] = true
			fields = append(fields, codex.NewControlField("007", coded))
		}
	}
	if leaderType != 'm' {
		for _, node := range g.Objects(inst, pMedia) {
			if rdaValue(node, mediaVocab) == "c" {
				fields = append(fields, codex.NewControlField("006", "m"+strings.Repeat(" ", 17)))
				break
			}
		}
	}
	return fields
}

// seriesFields reverses a Work's series bf:relation nodes into 490s: the title
// ($a) and ISSN ($x) come from the bf:associatedResource bf:Series, the volume
// designation ($v) from the relation's own bf:seriesEnumeration, and ind1 from
// whether the series carries the traced status. It inverts emitSeries.
//
// The 76x-78x decoder walks the same bf:relation list and skips these, because
// "series" is not one of its linking-entry relationship codes.
func seriesFields(g *rdf.Graph, work rdf.Term) []codex.Field {
	var fields []codex.Field
	for _, rel := range g.Objects(work, pRelation) {
		if relationshipCode(g, rel) != seriesRelationship {
			continue
		}
		res, ok := g.Object(rel, pAssociatedResource)
		if !ok {
			continue
		}
		title := firstTitle(g, res).MainTitle
		if title == "" {
			continue // a series with no title says nothing; a bare 490 is not a field
		}
		subs := []codex.Subfield{codex.NewSubfield('a', title)}
		if issn, _ := associatedIdentifiers(g, res); issn != "" {
			subs = append(subs, codex.NewSubfield('x', issn))
		}
		if v := literal(g, rel, pSeriesEnumeration); v != "" {
			subs = append(subs, codex.NewSubfield('v', v))
		}
		ind1 := byte('0')
		if seriesTraced(g, res) {
			ind1 = '1'
		}
		fields = append(fields, codex.NewDataField("490", ind1, ' ', subs...))
	}
	return fields
}

// seriesTraced reports whether a bf:Series carries the traced status, by IRI or
// by label -- marc2bibframe2 writes both, and a hand-written graph may carry only
// one.
func seriesTraced(g *rdf.Graph, res rdf.Term) bool {
	for _, st := range g.Objects(res, pStatus) {
		if st.IsIRI() && st.Value == statusVocab+statusTraced {
			return true
		}
		if strings.EqualFold(literal(g, st, pLabel), "traced") {
			return true
		}
	}
	return false
}

// legacySeriesFields reads the pre-v0.25.0 shape: flat bf:seriesStatement and
// bf:seriesEnumeration literals on the Instance, paired by position. It is a
// compatibility path for graphs libcodex itself wrote before the series relation
// existed, and it inherits that shape's defect -- two 490s sharing a $v are one
// triple, so the pairing cannot be recovered. See task 110.
func legacySeriesFields(g *rdf.Graph, inst rdf.Term) []codex.Field {
	stmts := literalsOf(g, inst, pSeriesStatement)
	if len(stmts) == 0 {
		return nil
	}
	enums := seriesEnumerationsFor(stmts, allLiteralsOf(g, inst, pSeriesEnumeration))
	fields := make([]codex.Field, 0, len(stmts))
	for i, stmt := range stmts {
		fields = append(fields, seriesField(stmt, enums[i]))
	}
	return fields
}

// seriesField reverses one bf:seriesStatement literal into a 490, with its paired
// bf:seriesEnumeration (empty for none) as $v.
func seriesField(stmt, volume string) codex.Field {
	subs := []codex.Subfield{codex.NewSubfield('a', stmt)}
	if volume != "" {
		subs = append(subs, codex.NewSubfield('v', volume))
	}
	return codex.NewDataField("490", '0', ' ', subs...)
}

// seriesEnumerationsFor pairs each bf:seriesStatement with its
// bf:seriesEnumeration, returning one volume designation (possibly empty) per
// statement.
//
// The two are flat sibling literals on the Instance -- LoC's shape -- which
// cannot say which statement a given enumeration belongs to, so they are paired
// only where the pairing is unambiguous. A graph this package wrote carries one
// enumeration per statement (empty where a 490 had no $v), which pairs by
// position. A graph written by hand or by another producer commonly carries a
// single series and a single enumeration, which pairs too. Anything else -- more
// enumerations than statements, or several statements with fewer enumerations --
// cannot be attributed, and the enumerations are dropped rather than guessed onto
// the wrong series.
func seriesEnumerationsFor(stmts, enums []string) []string {
	out := make([]string, len(stmts))
	switch {
	case len(enums) == len(stmts):
		copy(out, enums)
	case len(stmts) == 1 && len(enums) == 1:
		out[0] = enums[0]
	}
	return out
}

// durationField reverses the Instance's bf:duration literals into one 306 with a
// repeated $a, or nil when there are none.
func durationField(g *rdf.Graph, inst rdf.Term) *codex.Field {
	durations := literalsOf(g, inst, pDuration)
	if len(durations) == 0 {
		return nil
	}
	var subs []codex.Subfield
	for _, d := range durations {
		subs = append(subs, codex.NewSubfield('a', d))
	}
	f := codex.NewDataField("306", ' ', ' ', subs...)
	return &f
}

// digitalCharacteristicsField reverses the Instance's bf:digitalCharacteristic
// nodes into one 347: FileType labels as $a, EncodingFormat labels as $b, or nil
// when there are none.
func digitalCharacteristicsField(g *rdf.Graph, inst rdf.Term) *codex.Field {
	var subs []codex.Subfield
	for _, dc := range g.Objects(inst, pDigitalCharacteristic) {
		label := literal(g, dc, pLabel)
		if label == "" {
			continue
		}
		switch typeExcept(g, dc, "") {
		case "FileType":
			subs = append(subs, codex.NewSubfield('a', label))
		case "EncodingFormat":
			subs = append(subs, codex.NewSubfield('b', label))
		}
	}
	if len(subs) == 0 {
		return nil
	}
	f := codex.NewDataField("347", ' ', ' ', subs...)
	return &f
}

// relationshipCode returns a bf:Relation node's relationship code: the local name of
// its bf:relationship IRI when the IRI sits under the relationship vocabulary, else "".
func relationshipCode(g *rdf.Graph, rel rdf.Term) string {
	r, ok := g.Object(rel, pRelationship)
	if !ok || !r.IsIRI() {
		return ""
	}
	if code := strings.TrimPrefix(r.Value, relationshipVocab); code != r.Value {
		return code
	}
	return ""
}

// associatedName returns the creator label of a bf:associatedResource Work: the
// rdfs:label of its bf:contribution's bf:agent, or "".
func associatedName(g *rdf.Graph, res rdf.Term) string {
	if c, ok := g.Object(res, pContribution); ok {
		if agent, ok := g.Object(c, pAgent); ok {
			return literal(g, agent, pLabel)
		}
	}
	return ""
}

// associatedIdentifiers returns the ISSN and ISBN of a bf:associatedResource
// Work's identifiers, routed by identifier class ("" when absent).
func associatedIdentifiers(g *rdf.Graph, res rdf.Term) (issn, isbn string) {
	for _, id := range g.Objects(res, pIdentifiedBy) {
		v := literal(g, id, pValue)
		if v == "" {
			continue
		}
		switch typeExcept(g, id, "") {
		case "Isbn":
			if isbn == "" {
				isbn = v
			}
		default: // Issn (and the pre-081 untyped shape)
			if issn == "" {
				issn = v
			}
		}
	}
	return issn, isbn
}

// roleSubfields reverses a contribution's bf:role nodes: an IRI role becomes $4 (the
// relators code when the IRI sits under the relators vocabulary, else the whole IRI);
// a literal role becomes $j for a meeting agent and $e otherwise.
func roleSubfields(g *rdf.Graph, c rdf.Term, class string) []codex.Subfield {
	var subs []codex.Subfield
	for _, r := range g.Objects(c, pRole) {
		if r.IsIRI() {
			subs = append(subs, codex.NewSubfield('4', relatorCode(r.Value)))
			continue
		}
		if term := literal(g, r, pLabel); term != "" {
			subs = append(subs, codex.NewSubfield(roleLiteralSub(class), term))
		}
	}
	return subs
}

// relatorCode returns the $4 value for a role IRI: the local relator code when the
// IRI is under the LoC relators vocabulary, else the IRI itself (a verbatim URI role).
func relatorCode(iri string) string {
	if code := strings.TrimPrefix(iri, relatorVocab); code != iri {
		return code
	}
	return iri
}

// roleLiteralSub picks the literal-role subfield for an agent class: $j for a
// meeting (x11), $e otherwise.
func roleLiteralSub(class string) byte {
	if class == "Meeting" {
		return 'j'
	}
	return 'e'
}

// ind1ForClass returns the MARC first indicator the forward crosswalk reads back to
// this agent class: 3 for a Family, 2 for a corporate/meeting name in direct order,
// 1 for a personal surname or a jurisdiction.
func ind1ForClass(class string) byte {
	switch class {
	case "Family":
		return '3'
	case "Organization", "Meeting":
		return '2'
	default: // Person, Jurisdiction and unknown agents
		return '1'
	}
}

// agentClass returns the agent's specific bf class (Person, Organization, …),
// preferring it over the generic bf:Agent type that LoC attaches alongside it.
func agentClass(g *rdf.Graph, agent rdf.Term) string {
	types := g.Objects(agent, pType)
	for _, want := range agentClasses {
		for _, o := range types {
			if o.IsIRI() && rdf.LocalName(o.Value) == want {
				return want
			}
		}
	}
	return ""
}

// contribTag picks the MARC tag for an agent class and primary/added entry.
func contribTag(class string, primary bool) string {
	var base string
	switch class {
	case "Organization", "Jurisdiction":
		base = "10"
	case "Meeting":
		base = "11"
	default:
		base = "00" // Person, Family and unknown agents use the X00 personal-name tag
	}
	if primary {
		return "1" + base
	}
	return "7" + base
}

// subjectFields reverses bf:subject into 6xx fields, re-splitting the "--"-joined
// heading of topical and geographic subjects into $a and $x subdivisions. Each
// subject term yields exactly one heading, even when it carries labels in several
// languages or the graph repeats the bf:subject edge.
func subjectFields(g *rdf.Graph, work rdf.Term) []codex.Field {
	var fields []codex.Field
	seen := make(map[rdf.Term]bool)
	for _, s := range g.Objects(work, pSubject) {
		if seen[s] { // RDF is a set: a repeated bf:subject edge is one subject
			continue
		}
		seen[s] = true
		label := preferredLabel(g, s, pLabel)
		if label == "" {
			label = preferredLabel(g, s, pPrefLabel) // read the SKOS shape natively
		}
		if label == "" {
			continue
		}
		// An IRI subject node carries its authority link in $0; derive the
		// thesaurus from the IRI prefix when no bf:source node names it.
		var authority string
		if s.IsIRI() {
			authority = s.Value
		}
		source := sourceLabel(g, s)
		if source == "" && authority != "" {
			source = sourceFromIRI(authority)
		}
		ind2, sub2 := subjectInd2(source)
		class := typeExcept(g, s, "")
		if class == "" {
			class = "Topic" // an untyped SKOS concept defaults to a topical 650
		}
		switch class {
		case "Topic":
			fields = append(fields, headingField("650", label, ind2, sub2, authority))
		case "Place":
			fields = append(fields, headingField("651", label, ind2, sub2, authority))
		case "Person":
			fields = append(fields, nameHeadingField("600", '1', ind2, label, sub2, authority))
		case "Organization":
			fields = append(fields, nameHeadingField("610", '2', ind2, label, sub2, authority))
		case "Meeting":
			fields = append(fields, nameHeadingField("611", '2', ind2, label, sub2, authority))
		}
	}
	return fields
}

// preferredLabel picks the single label to render as a heading from the literals a
// node carries under predicate, which a SKOS concept may hold in several languages.
// English wins ("en", then any "en-*" subtag), then an untagged literal, then the
// lowest language tag lexicographically. The last two steps matter because document
// order is not meaningful in RDF: without them the heading would depend on which
// language the serializer happened to write first.
func preferredLabel(g *rdf.Graph, node rdf.Term, predicate string) string {
	var enSubtag, untagged, lowest, lowestLang string
	for _, o := range g.Objects(node, predicate) {
		if !o.IsLiteral() || o.Value == "" {
			continue
		}
		lang := strings.ToLower(o.Lang)
		switch {
		case lang == "en":
			return o.Value
		case strings.HasPrefix(lang, "en-"):
			if enSubtag == "" {
				enSubtag = o.Value
			}
		case lang == "":
			if untagged == "" {
				untagged = o.Value
			}
		case lowestLang == "" || lang < lowestLang:
			lowestLang, lowest = lang, o.Value
		}
	}
	return firstNonEmpty(enSubtag, untagged, lowest)
}

// sourceFromIRI derives a subject thesaurus code from a well-known authority IRI
// prefix, so a bare SKOS concept IRI still resolves to the right 6xx second
// indicator / $2. It returns "" for an unrecognized host.
func sourceFromIRI(iri string) string {
	switch {
	case strings.Contains(iri, "id.loc.gov/authorities/subjects"):
		return "lcsh"
	case strings.Contains(iri, "id.loc.gov/authorities/childrensSubjects"):
		return "lcshac"
	case strings.Contains(iri, "id.worldcat.org/fast"):
		return "fast"
	case strings.Contains(iri, "homosaurus.org"):
		return "homosaurus"
	case strings.Contains(iri, "id.nlm.nih.gov/mesh"):
		return "mesh"
	}
	return ""
}

// subjectInd2 reverses a subject bf:source code into a 6xx second indicator, plus
// a $2 value when the code is not one of the numeric-indicator thesauri (or the
// source is unknown, which maps to ind2='7' with the code in $2).
func subjectInd2(source string) (ind2 byte, sub2 string) {
	switch source {
	case "":
		return ' ', "" // no thesaurus recorded
	case "lcsh":
		return '0', ""
	case "lcshac":
		return '1', ""
	case "mesh":
		return '2', ""
	case "nal":
		return '3', ""
	case "cash":
		return '5', ""
	case "rvm":
		return '6', ""
	default:
		return '7', source
	}
}

// headingField builds a 650/651 from a "--"-subdivided label: the first portion
// is $a, each remaining portion a general subdivision $x, with the thesaurus in
// $2 when the second indicator defers to it and the authority link in $0 when the
// subject node was an IRI.
func headingField(tag, label string, ind2 byte, sub2, authority string) codex.Field {
	parts := strings.Split(label, "--")
	subs := []codex.Subfield{codex.NewSubfield('a', parts[0])}
	for _, p := range parts[1:] {
		subs = append(subs, codex.NewSubfield('x', p))
	}
	subs = appendThesaurusAndAuthority(subs, sub2, authority)
	return codex.NewDataField(tag, ' ', ind2, subs...)
}

// nameHeadingField builds a 600/610/611 name subject from its label, carrying the
// thesaurus in $2 and the authority link in $0 as headingField does.
func nameHeadingField(tag string, ind1, ind2 byte, label, sub2, authority string) codex.Field {
	subs := []codex.Subfield{codex.NewSubfield('a', label)}
	subs = appendThesaurusAndAuthority(subs, sub2, authority)
	return codex.NewDataField(tag, ind1, ind2, subs...)
}

// appendThesaurusAndAuthority appends the $2 thesaurus (when the indicator defers
// to it) and the $0 authority link (when present) to a 6xx subfield list, in MARC
// order.
func appendThesaurusAndAuthority(subs []codex.Subfield, sub2, authority string) []codex.Subfield {
	if sub2 != "" {
		subs = append(subs, codex.NewSubfield('2', sub2))
	}
	if authority != "" {
		subs = append(subs, codex.NewSubfield('0', authority))
	}
	return subs
}

// catalogingSourceField reconstructs field 040 from the record's bf:AdminMetadata.
// The internal bf:Note carries the whole field in marcKey form, so it is preferred
// and yields the field exactly; a graph without one (hand-built, or a third-party
// BIBFRAME) falls back to the modelled properties, which cover every subfield but
// $c. A graph carrying no cataloging source at all yields no 040 rather than a
// fabricated one.
func catalogingSourceField(g *rdf.Graph, work, inst rdf.Term) (codex.Field, bool) {
	// LoC marc2bibframe2 hangs several bf:AdminMetadata nodes off a record (one per
	// 005/008 status event); only one carries the 040, so scan them all.
	var admins []rdf.Term
	for _, subj := range []rdf.Term{inst, work} {
		admins = append(admins, g.Objects(subj, pAdminMetadata)...)
	}
	for _, admin := range admins {
		if f, ok := field040FromNote(g, admin); ok {
			return f, true
		}
	}
	for _, admin := range admins {
		if f, ok := field040FromProperties(g, admin); ok {
			return f, true
		}
	}
	return codex.Field{}, false
}

// field040FromNote recovers field 040 from the AdminMetadata's internal bf:Note,
// whose rdfs:label holds the field in marcKey form.
func field040FromNote(g *rdf.Graph, admin rdf.Term) (codex.Field, bool) {
	for _, note := range g.Objects(admin, pNote) {
		if !g.HasType(note, internalNoteType) {
			continue
		}
		if f, ok := parseMARCKey(literal(g, note, pLabel)); ok && f.Tag == "040" {
			return f, true
		}
	}
	return codex.Field{}, false
}

// field040FromProperties rebuilds field 040 from the AdminMetadata's modelled
// properties, in canonical $a $b $d... $e subfield order. $c has no BIBFRAME
// property, so it cannot be recovered here.
func field040FromProperties(g *rdf.Graph, admin rdf.Term) (codex.Field, bool) {
	var subs []codex.Subfield
	if a := agencyCode(g, mustNode(g, admin, pAssigner)); a != "" {
		subs = append(subs, codex.NewSubfield('a', a))
	}
	if l := vocabCode(g, mustNode(g, admin, pDescriptionLanguage)); l != "" {
		subs = append(subs, codex.NewSubfield('b', l))
	}
	for _, m := range g.Objects(admin, pDescriptionModifier) {
		if c := agencyCode(g, m); c != "" {
			subs = append(subs, codex.NewSubfield('d', c))
		}
	}
	for _, dc := range g.Objects(admin, pDescriptionConventions) {
		if c := vocabCode(g, dc); c != "" {
			subs = append(subs, codex.NewSubfield('e', c))
		}
	}
	if len(subs) == 0 {
		return codex.Field{}, false
	}
	return codex.NewDataField("040", ' ', ' ', subs...), true
}

// agencyCode reads a cataloging agency's MARC organization code: its bf:code, else
// the last segment of its organizations-vocabulary IRI, which is the code lowercased.
func agencyCode(g *rdf.Graph, node rdf.Term) string {
	if c := literal(g, node, pCode); c != "" {
		return c
	}
	if node.IsIRI() {
		return strings.ToUpper(rdf.LocalName(node.Value))
	}
	return ""
}

// vocabCode reads a controlled term's code: its bf:code, else the last segment of
// its vocabulary IRI, which for languages and description conventions is the code.
func vocabCode(g *rdf.Graph, node rdf.Term) string {
	if c := literal(g, node, pCode); c != "" {
		return c
	}
	if node.IsIRI() {
		return rdf.LocalName(node.Value)
	}
	return ""
}

// parseMARCKey parses a marcKey literal -- three tag characters, two indicator
// characters (a blank indicator is a space), then "$<code><value>" per subfield,
// as in "040  $aDLC$beng" -- into a data field. A literal too short to hold a
// subfield, or whose subfields do not start where the indicators end, is not a
// marcKey.
func parseMARCKey(key string) (codex.Field, bool) {
	if len(key) < 6 || key[5] != '$' {
		return codex.Field{}, false
	}
	var subs []codex.Subfield
	for _, s := range strings.Split(key[6:], "$") {
		if s == "" {
			continue
		}
		subs = append(subs, codex.NewSubfield(s[0], s[1:]))
	}
	if len(subs) == 0 {
		return codex.Field{}, false
	}
	return codex.NewDataField(key[0:3], key[3], key[4], subs...), true
}

// identifierFields reverses bf:identifiedBy into 020/022/024, restoring the
// scheme ($2) from any bf:source node the forward crosswalk attached.
func identifierFields(g *rdf.Graph, inst rdf.Term) []codex.Field {
	var fields []codex.Field
	for _, id := range g.Objects(inst, pIdentifiedBy) {
		value := literal(g, id, pValue)
		if value == "" {
			continue
		}
		source := sourceLabel(g, id)
		qualifier := literal(g, id, pQualifier)
		status := statusLabel(g, id)
		switch class := typeExcept(g, id, ""); class {
		case "Isbn":
			fields = append(fields, identifierField("020", ' ', ' ', value, source, qualifier, status))
		case "Issn":
			fields = append(fields, identifierField("022", ' ', ' ', value, source, qualifier, status))
		case "Lccn":
			code := byte('a')
			if status == statusCancInv {
				code = 'z'
			}
			fields = append(fields, codex.NewDataField("010", ' ', ' ', codex.NewSubfield(code, strings.TrimSpace(value))))
		default:
			fields = append(fields, identifier024Field(class, value, source, qualifier, status))
		}
	}
	return fields
}

// ind1ByScheme024 and codeByScheme024 invert the forward 024 scheme maps so the
// reverse crosswalk can restore a typed identifier to its 024 indicator / $2.
var ind1ByScheme024 = func() map[string]byte {
	m := make(map[string]byte, len(scheme024ByInd1))
	for k, v := range scheme024ByInd1 {
		m[v] = k
	}
	return m
}()

var codeByScheme024 = func() map[string]string {
	m := make(map[string]string, len(scheme024ByCode))
	for k, v := range scheme024ByCode {
		m[v] = k
	}
	return m
}()

// identifier024Field reverses a typed identifier back into an 024: ind1 0-4 for the
// standard schemes, ind1='7' with the $2 scheme code for the others, and ind1='8'
// (or '7' when a source was recorded) for a generic identifier.
func identifier024Field(class, value, source, qualifier, status string) codex.Field {
	if ind1, ok := ind1ByScheme024[class]; ok {
		return identifierField("024", ind1, ' ', value, "", qualifier, status)
	}
	if code, ok := codeByScheme024[class]; ok {
		return identifierField("024", '7', ' ', value, code, qualifier, status)
	}
	ind1 := byte('8')
	if source != "" {
		ind1 = '7'
	}
	return identifierField("024", ind1, ' ', value, source, qualifier, status)
}

// statusLabel returns the rdfs:label of an identifier's bf:status node, or "".
func statusLabel(g *rdf.Graph, node rdf.Term) string {
	st, ok := g.Object(node, pStatus)
	if !ok {
		return ""
	}
	return literal(g, st, pLabel)
}

// identifierField builds an 020/022/024 from a value plus an optional scheme,
// qualifier and status, which round-trip through subfields $2, $q and the value
// subfield. A canceled/invalid number goes in $z, an incorrect ISSN in $y; a valid
// number in $a. A bf:qualifier lifted out of an ISBN parenthetical is normalized
// back into $q, the modern form.
func identifierField(tag string, ind1, ind2 byte, value, source, qualifier, status string) codex.Field {
	valueCode := byte('a')
	switch status {
	case statusCancInv:
		valueCode = 'z'
	case statusIncorrect:
		valueCode = 'y'
	}
	subs := []codex.Subfield{codex.NewSubfield(valueCode, value)}
	if qualifier != "" {
		subs = append(subs, codex.NewSubfield('q', qualifier))
	}
	if source != "" {
		subs = append(subs, codex.NewSubfield('2', source))
	}
	return codex.NewDataField(tag, ind1, ind2, subs...)
}

// classificationFields reverses bf:classification into 050/082, and a generic
// bf:Classification (source-qualified, as 072 produces) back into 072 with its
// scheme in $2. A Classification's rdfs:label (Label) is display-only and has no
// standard MARC channel, so it is not rendered here: a Classification round-tripped
// through MARC keeps its code ($a) but loses its Label.
func classificationFields(g *rdf.Graph, work rdf.Term) []codex.Field {
	var fields []codex.Field
	for _, c := range g.Objects(work, pClassif) {
		value := literal(g, c, pClassPortion)
		if value == "" {
			continue
		}
		item := literal(g, c, pItemPortion)
		switch typeExcept(g, c, "") {
		case "ClassificationLcc":
			fields = append(fields, codex.NewDataField("050", ' ', '4', callNumberSubs(value, item)...))
		case "ClassificationDdc":
			subs := callNumberSubs(value, item)
			if src := sourceLabel(g, c); src != "" {
				subs = append(subs, codex.NewSubfield('2', src))
			}
			fields = append(fields, codex.NewDataField("082", deweyInd1(literal(g, c, pClassEdition)), '4', subs...))
		case "Classification":
			subs := callNumberSubs(value, item)
			if src := sourceLabel(g, c); src != "" {
				subs = append(subs, codex.NewSubfield('2', src))
			}
			fields = append(fields, codex.NewDataField("072", ' ', '7', subs...))
		}
	}
	return fields
}

// callNumberSubs renders a classification portion and optional item portion as $a/$b.
func callNumberSubs(value, item string) []codex.Subfield {
	subs := []codex.Subfield{codex.NewSubfield('a', value)}
	if item != "" {
		subs = append(subs, codex.NewSubfield('b', item))
	}
	return subs
}

// deweyInd1 inverts deweyEdition: a bf:edition value back to the 082 first indicator.
func deweyInd1(edition string) byte {
	switch edition {
	case "full":
		return '0'
	case "abridged":
		return '1'
	}
	return ' '
}

// physicalFields reverses the Instance's bf:extent labels and bf:dimensions into
// 300 fields: one 300 per extent (the first also carrying every $c dimension), or a
// dimensions-only 300 when there is no extent.
func physicalFields(g *rdf.Graph, inst rdf.Term) []codex.Field {
	extents := labelsOf(g, inst, pExtent)
	dims := literalsOf(g, inst, pDimensions)
	dimSubs := func() []codex.Subfield {
		var subs []codex.Subfield
		for _, d := range dims {
			subs = append(subs, codex.NewSubfield('c', d))
		}
		return subs
	}
	var fields []codex.Field
	for i, e := range extents {
		subs := []codex.Subfield{codex.NewSubfield('a', e)}
		if i == 0 {
			subs = append(subs, dimSubs()...)
		}
		fields = append(fields, codex.NewDataField("300", ' ', ' ', subs...))
	}
	if len(extents) == 0 && len(dims) > 0 {
		fields = append(fields, codex.NewDataField("300", ' ', ' ', dimSubs()...))
	}
	return fields
}

// contentField reverses the Work's bf:content into a 336 with the RDA code in $b
// and the rdacontent source in $2, or nil when there is no content term.
func contentField(g *rdf.Graph, work rdf.Term) *codex.Field {
	c, ok := g.Object(work, pContent)
	if !ok {
		return nil
	}
	code := rdaValue(c, contentVocab)
	if code == "" {
		return nil
	}
	f := codex.NewDataField("336", ' ', ' ', codex.NewSubfield('b', code), codex.NewSubfield('2', "rdacontent"))
	return &f
}

// rdaFields reverses the Instance's bf:media/bf:carrier nodes into 337/338 fields:
// the vocabulary code in $b, a differing label in $a, and the RDA source in $2.
func rdaFields(g *rdf.Graph, inst rdf.Term, pred, tag, vocab, source string) []codex.Field {
	var fields []codex.Field
	for _, node := range g.Objects(inst, pred) {
		code := rdaValue(node, vocab)
		label := literal(g, node, pLabel)
		var subs []codex.Subfield
		if label != "" && label != code {
			subs = append(subs, codex.NewSubfield('a', label))
		}
		if code != "" {
			subs = append(subs, codex.NewSubfield('b', code), codex.NewSubfield('2', source))
		}
		if len(subs) == 0 {
			continue
		}
		fields = append(fields, codex.NewDataField(tag, ' ', ' ', subs...))
	}
	return fields
}

// rdaValue returns an RDA node's code: the local name of its vocabulary IRI when it
// sits under vocab, else "".
func rdaValue(node rdf.Term, vocab string) string {
	if node.IsIRI() {
		if code := strings.TrimPrefix(node.Value, vocab); code != node.Value {
			return code
		}
	}
	return ""
}

// literalsOf returns every literal object of subject reached through predicate.
func literalsOf(g *rdf.Graph, subject rdf.Term, predicate string) []string {
	var out []string
	for _, o := range g.Objects(subject, predicate) {
		if o.IsLiteral() && o.Value != "" {
			out = append(out, o.Value)
		}
	}
	return out
}

// allLiteralsOf is literalsOf without the empty-value filter, for a predicate
// whose empty literal is meaningful -- bf:seriesEnumeration emits one as a
// positional placeholder for a 490 that carried no $v.
//
// It reads the repeats deliberately: bf:seriesEnumeration is positional, aligned
// index-for-index with bf:seriesStatement, so two 490s carrying the same $v (or
// no $v) must yield two literals rather than one. That is the same multiplicity
// RDF's abstract syntax does not preserve, which is why the alignment survives
// our own list-backed graph but not a round trip through a set-backed store.
// See task 110.
func allLiteralsOf(g *rdf.Graph, subject rdf.Term, predicate string) []string {
	var out []string
	for _, o := range g.ObjectsWithRepeats(subject, predicate) {
		if o.IsLiteral() {
			out = append(out, o.Value)
		}
	}
	return out
}

// sourceLabel returns the rdfs:label of a node's bf:source scheme node, or "".
func sourceLabel(g *rdf.Graph, node rdf.Term) string {
	src, ok := g.Object(node, pSource)
	if !ok {
		return ""
	}
	return literal(g, src, pLabel)
}

// languageField reverses bf:language into a single 041 with one $a per code.
func languageField(g *rdf.Graph, work rdf.Term) []codex.Field {
	var subs, orig []codex.Subfield
	for _, l := range g.Objects(work, pLanguage) {
		code := langCode(g, l)
		if code == "" {
			continue
		}
		if literal(g, l, pPart) == "original" { // 041 $h -- language of the original
			orig = append(orig, codex.NewSubfield('h', code))
		} else {
			subs = append(subs, codex.NewSubfield('a', code))
		}
	}
	subs = append(subs, orig...)
	if len(subs) == 0 {
		return nil
	}
	return []codex.Field{codex.NewDataField("041", ' ', ' ', subs...)}
}

// langCode resolves a bf:Language node to a three-letter code, trying bf:code,
// then the vocabulary IRI's local name, then rdfs:label. LoC puts the code in
// bf:code and a human name ("English") in rdfs:label, while this library's own
// output carries the code in rdfs:label — so bf:code and the IRI take precedence.
func langCode(g *rdf.Graph, l rdf.Term) string {
	candidates := []string{literal(g, l, pCode)}
	if l.IsIRI() {
		candidates = append(candidates, rdf.LocalName(l.Value))
	}
	candidates = append(candidates, literal(g, l, pLabel))
	for _, c := range candidates {
		if c = strings.TrimSpace(c); isLangCode(c) {
			return c
		}
	}
	return ""
}

// provisionFields reverses the Instance's provision activities into 260 statements
// (one per node) and a 264 _4 for a bf:copyrightDate. Provision classes collapse to
// the generic 260 rather than typed 264 indicators, keeping the transcribed
// statement front and centre in this library's flatter model. The 008 positions
// these nodes also feed are rendered by control008.
func provisionFields(g *rdf.Graph, inst rdf.Term) []codex.Field {
	var fields []codex.Field
	for _, prov := range g.Objects(inst, pProvision) {
		if subs := provision26XSubfields(g, prov); len(subs) > 0 {
			fields = append(fields, codex.NewDataField("260", ' ', ' ', subs...))
		}
	}
	if cd := literal(g, inst, pCopyright); cd != "" {
		fields = append(fields, codex.NewDataField("264", ' ', '4', codex.NewSubfield('c', cd)))
	}
	return fields
}

// provision26XSubfields reads one provision node into 260 $a/$b/$c, preferring the
// transcribed bflc:simple* forms; the controlled bf:place label is used only when
// it is a blank labeled node, never a country IRI (whose label is an authority
// name, not the transcribed place).
func provision26XSubfields(g *rdf.Graph, prov rdf.Term) []codex.Subfield {
	var subs []codex.Subfield
	if place := firstNonEmpty(literal(g, prov, pSimplePlace), transcribedPlace(g, prov)); place != "" {
		subs = append(subs, codex.NewSubfield('a', place))
	}
	if pub := firstNonEmpty(literal(g, prov, pSimpleAgent), literal(g, mustNode(g, prov, pAgent), pLabel)); pub != "" {
		subs = append(subs, codex.NewSubfield('b', pub))
	}
	if date := firstNonEmpty(literal(g, prov, pDate), literal(g, prov, pSimpleDate)); date != "" {
		subs = append(subs, codex.NewSubfield('c', date))
	}
	return subs
}

// transcribedPlace returns the label of a blank bf:place node, or "" when the place
// is a controlled IRI (a country authority, not the transcribed place of publication).
func transcribedPlace(g *rdf.Graph, prov rdf.Term) string {
	p, ok := g.Object(prov, pPlace)
	if !ok || p.IsIRI() {
		return ""
	}
	return literal(g, p, pLabel)
}

// countryCode returns the MARC country code of a provision node's controlled
// bf:place IRI when it sits under the LoC countries vocabulary, else "".
func countryCode(g *rdf.Graph, prov rdf.Term) string {
	p, ok := g.Object(prov, pPlace)
	if !ok || !p.IsIRI() {
		return ""
	}
	if code := strings.TrimPrefix(p.Value, countriesVocab); code != p.Value && isCountryCode(code) {
		return code
	}
	return ""
}

// control008 renders the partial 40-byte 008 the graph supports, mirroring exactly
// the positions the forward crosswalk reads out of an 008: the publication date at
// 06-10, the country at 15-17, and the language at 35-37. Each is a derivation from
// a property FromRecord built out of that position, so decode returns the record to
// the same fixed field rather than fabricating one; positions the graph cannot
// speak to stay blank. It reports false when the graph populates none of them.
func control008(g *rdf.Graph, work, inst rdf.Term) (codex.Field, bool) {
	b := []byte(strings.Repeat(" ", 40))
	populated := false
	if year := soleProvisionYear(g, inst); year != "" {
		b[6] = 's' // single known date, the only status this reconstruction can assert
		copy(b[7:11], year)
		populated = true
	}
	if code := provisionCountry(g, inst); code != "" {
		copy(b[15:18], code)
		populated = true
	}
	if code := primaryLanguage(g, work); code != "" {
		copy(b[35:38], code)
		populated = true
	}
	if !populated {
		return codex.Field{}, false
	}
	return codex.NewControlField("008", string(b)), true
}

// provisionCountry returns the country code of the first provision node that names
// one through a controlled bf:place IRI.
func provisionCountry(g *rdf.Graph, inst rdf.Term) string {
	for _, prov := range g.Objects(inst, pProvision) {
		if code := countryCode(g, prov); code != "" {
			return code
		}
	}
	return ""
}

// soleProvisionYear returns the publication year for 008/07-10: the one plain
// four-digit year the Instance's provision activities agree on. A date that is not
// a bare year (an open range, a bracketed guess, a month) is left to the 260 $c
// rather than parsed, and provisions disagreeing on the year yield nothing, since
// the reconstruction cannot say which one 008 meant.
func soleProvisionYear(g *rdf.Graph, inst rdf.Term) string {
	year := ""
	for _, prov := range g.Objects(inst, pProvision) {
		d := firstNonEmpty(literal(g, prov, pDate), literal(g, prov, pSimpleDate))
		if !isYear(d) {
			continue
		}
		if year != "" && year != d {
			return "" // two provisions, two different years: ambiguous
		}
		year = d
	}
	return year
}

// isYear reports whether s is a bare four-digit year.
func isYear(s string) bool {
	if len(s) != 4 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// primaryLanguage returns the code for 008/35-37: the Work's first content
// language, skipping a language of the original (041 $h), which the 008 slot never
// holds.
func primaryLanguage(g *rdf.Graph, work rdf.Term) string {
	for _, l := range g.Objects(work, pLanguage) {
		if literal(g, l, pPart) == "original" {
			continue
		}
		if code := langCode(g, l); code != "" {
			return code
		}
	}
	return ""
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// locatorField reconstructs one 856 from a bf:electronicLocator node: the node
// IRI is $u, its rdfs:label $3 materials, a literal bf:note $z, and a bf:note node
// typed bf:noteType "link text" the $y link text.
func locatorField(g *rdf.Graph, loc rdf.Term) (codex.Field, bool) {
	if loc.Value == "" {
		return codex.Field{}, false
	}
	subs := []codex.Subfield{codex.NewSubfield('u', loc.Value)}
	if m := literal(g, loc, pLabel); m != "" {
		subs = append(subs, codex.NewSubfield('3', m))
	}
	var note, linkText string
	for _, n := range g.Objects(loc, pNote) {
		if n.IsLiteral() {
			note = n.Value // literal bf:note -> $z
			continue
		}
		if literal(g, n, pNoteType) == locatorNoteType {
			linkText = literal(g, n, pLabel) // typed bf:note node -> $y
		}
	}
	if note != "" {
		subs = append(subs, codex.NewSubfield('z', note))
	}
	if linkText != "" {
		subs = append(subs, codex.NewSubfield('y', linkText))
	}
	return codex.NewDataField("856", '4', '0', subs...), true
}

// ---- title helpers ----

// firstTitle returns the components of a subject's first main bf:Title, skipping any
// bf:VariantTitle/bf:ParallelTitle nodes (which the 246 reverse handles separately).
func firstTitle(g *rdf.Graph, subject rdf.Term) Title {
	for _, node := range g.Objects(subject, pTitle) {
		if g.HasType(node, classVariantTitle) || g.HasType(node, classParallelTitle) {
			continue
		}
		return Title{
			MainTitle:  literal(g, node, pMainTitle),
			Subtitle:   literal(g, node, pSubtitle),
			PartNumber: literal(g, node, pPartNumber),
			PartName:   literal(g, node, pPartName),
			NonSortNum: literal(g, node, pNonSortNum),
		}
	}
	return Title{}
}

// variantTitleFields reverses a subject's bf:VariantTitle/bf:ParallelTitle nodes into
// 246 fields, restoring the second indicator from the variant type / parallel flag.
func variantTitleFields(g *rdf.Graph, subject rdf.Term) []codex.Field {
	var fields []codex.Field
	for _, node := range g.Objects(subject, pTitle) {
		parallel := g.HasType(node, classParallelTitle)
		if !parallel && !g.HasType(node, classVariantTitle) {
			continue
		}
		vt := VariantTitle{
			Parallel:    parallel,
			VariantType: literal(g, node, pVariantType),
			MainTitle:   literal(g, node, pMainTitle),
			Subtitle:    literal(g, node, pSubtitle),
			PartNumber:  literal(g, node, pPartNumber),
			PartName:    literal(g, node, pPartName),
		}
		subs := []codex.Subfield{codex.NewSubfield('a', vt.MainTitle)}
		if vt.Subtitle != "" {
			subs = append(subs, codex.NewSubfield('b', vt.Subtitle))
		}
		if vt.PartNumber != "" {
			subs = append(subs, codex.NewSubfield('n', vt.PartNumber))
		}
		if vt.PartName != "" {
			subs = append(subs, codex.NewSubfield('p', vt.PartName))
		}
		fields = append(fields, codex.NewDataField("246", ' ', ind2ForVariant(vt), subs...))
	}
	return fields
}

// noteFields reverses a subject's bf:note nodes into 5xx fields, choosing the tag
// from each note's bf:noteType (500 general, 504 bibliography, 546 language).
func noteFields(g *rdf.Graph, subject rdf.Term) []codex.Field {
	var fields []codex.Field
	for _, n := range g.Objects(subject, pNote) {
		label := literal(g, n, pLabel)
		if label == "" {
			continue
		}
		tag := tagForNoteType(literal(g, n, pNoteType))
		fields = append(fields, codex.NewDataField(tag, ' ', ' ', codex.NewSubfield('a', label)))
	}
	return fields
}

// titleInd2 returns the 245 second indicator from a nonfiling character count: the
// single digit when valid, else '0'.
func titleInd2(nonSortNum string) byte {
	if len(nonSortNum) == 1 && nonSortNum[0] >= '1' && nonSortNum[0] <= '9' {
		return nonSortNum[0]
	}
	return '0'
}

// titleSubfields renders a Title (and optional statement of responsibility) as
// 245/130/240 subfields.
func titleSubfields(t Title, resp string) []codex.Subfield {
	subs := []codex.Subfield{codex.NewSubfield('a', t.MainTitle)}
	if t.Subtitle != "" {
		subs = append(subs, codex.NewSubfield('b', t.Subtitle))
	}
	if t.PartNumber != "" {
		subs = append(subs, codex.NewSubfield('n', t.PartNumber))
	}
	if t.PartName != "" {
		subs = append(subs, codex.NewSubfield('p', t.PartName))
	}
	if resp != "" {
		subs = append(subs, codex.NewSubfield('c', resp))
	}
	return subs
}

// ---- graph helpers ----

// literal returns the lexical value of subject's first literal object for the
// predicate, or "".
func literal(g *rdf.Graph, subject rdf.Term, predicate string) string {
	v, _ := g.Literal(subject, predicate)
	return v
}

// labelsOf returns the rdfs:label of every node object reached through the
// predicate (the common bf:Class -> rdfs:label shape).
func labelsOf(g *rdf.Graph, subject rdf.Term, predicate string) []string {
	var out []string
	for _, node := range g.Objects(subject, predicate) {
		if v := literal(g, node, pLabel); v != "" {
			out = append(out, v)
		} else if node.IsLiteral() && node.Value != "" {
			out = append(out, node.Value)
		}
	}
	return out
}

// mustNode returns subject's first object for the predicate (a zero Term when
// absent, which carries no triples and so reads as empty).
func mustNode(g *rdf.Graph, subject rdf.Term, predicate string) rdf.Term {
	n, _ := g.Object(subject, predicate)
	return n
}

// typeExcept returns the local name of subject's first rdf:type whose local name
// is not exclude (used to read the refining class beside bf:Work, the agent class
// beside the wrapper, etc.).
func typeExcept(g *rdf.Graph, subject rdf.Term, exclude string) string {
	for _, o := range g.Objects(subject, pType) {
		if o.IsIRI() {
			if ln := rdf.LocalName(o.Value); ln != exclude {
				return ln
			}
		}
	}
	return ""
}

// controlNumber recovers the 001 from a "#<id>Work" fragment IRI the forward
// crosswalk mints; other IRI shapes yield no control number. A synthetic
// stream-index base ("r" followed by digits), which resolveBase mints for a
// record that had no 001, is not a real control number and yields "" -- so a
// 001-less record does not decode to a fabricated 001 that would collide with
// every other 001-less record.
func controlNumber(iri string) string {
	ln := rdf.LocalName(iri)
	if strings.HasPrefix(iri, "#") && strings.HasSuffix(ln, "Work") {
		id := strings.TrimSuffix(ln, "Work")
		if isFallbackBase(id) {
			return ""
		}
		return id
	}
	return ""
}

// isFallbackBase reports whether id has the shape resolveBase mints for a record
// with no 001: the letter "r" followed by one or more digits. A genuine control
// number of exactly that shape is indistinguishable and is treated as a fallback
// base -- an accepted trade-off, since the alternative fabricates a shared 001.
func isFallbackBase(id string) bool {
	if len(id) < 2 || id[0] != 'r' {
		return false
	}
	for i := 1; i < len(id); i++ {
		if id[i] < '0' || id[i] > '9' {
			return false
		}
	}
	return true
}

// leaderFor returns a default leader with byte 6 (type of record) set from a
// BIBFRAME content class and byte 7 (bibliographic level) from a mode-of-issuance
// code -- the inverse of workClass and issuanceForLevel.
func leaderFor(class, issuance string) codex.Leader {
	b := []byte(codex.NewRecord().Leader().String())
	if t := recordType(class); t != 0 {
		b[6] = t
	}
	if lvl := levelForIssuance(issuance); lvl != 0 {
		b[7] = lvl
	}
	return codex.Leader(b)
}

// issuanceCode returns the Instance's mode-of-issuance code: the local name of its
// bf:issuance IRI when it sits under the LoC issuance vocabulary, else "".
func issuanceCode(g *rdf.Graph, inst rdf.Term) string {
	o, ok := g.Object(inst, pIssuance)
	if !ok || !o.IsIRI() {
		return ""
	}
	if code := strings.TrimPrefix(o.Value, issuanceVocab); code != o.Value {
		return code
	}
	return ""
}

// recordType maps a BIBFRAME content class back to a representative leader byte 6.
func recordType(class string) byte {
	switch class {
	case "Text":
		return 'a'
	case "NotatedMusic":
		return 'c'
	case "Cartography":
		return 'e'
	case "MovingImage":
		return 'g'
	case "NonMusicAudio", "Audio":
		return 'i'
	case "MusicAudio":
		return 'j'
	case "StillImage":
		return 'k'
	case "Multimedia":
		return 'm'
	case "MixedMaterial":
		return 'o'
	case "Object":
		return 'r'
	default:
		return 0
	}
}
