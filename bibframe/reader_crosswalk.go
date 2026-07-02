package bibframe

import (
	"sort"
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

func recordFromWorkInstance(g *rdf.Graph, work, inst rdf.Term, hasInst bool) *codex.Record {
	rec := codex.NewRecord()
	rec.SetLeader(leaderForClass(typeExcept(g, work, "Work")))

	var fields []codex.Field
	add := func(f codex.Field) { fields = append(fields, f) }

	if id := controlNumber(work.Value); id != "" {
		add(codex.NewControlField("001", id))
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
		add(codex.NewDataField("245", ind1, '0', titleSubfields(instTitle, resp)...))
	}

	fields = append(fields, contribs...)
	fields = append(fields, relatedWorkFields(g, work)...)
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
	for _, f := range provisionFields(g, inst) {
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
	for _, s := range labelsOf(g, work, pSummary) {
		add(codex.NewDataField("520", ' ', ' ', codex.NewSubfield('a', s)))
	}
	for _, u := range locators(g, inst) {
		add(codex.NewDataField("856", '4', '0', codex.NewSubfield('u', u)))
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
// heading of topical and geographic subjects into $a and $x subdivisions.
func subjectFields(g *rdf.Graph, work rdf.Term) []codex.Field {
	var fields []codex.Field
	for _, s := range g.Objects(work, pSubject) {
		label := literal(g, s, pLabel)
		if label == "" {
			continue
		}
		ind2, sub2 := subjectInd2(sourceLabel(g, s))
		switch typeExcept(g, s, "") {
		case "Topic":
			fields = append(fields, headingField("650", label, ind2, sub2))
		case "Place":
			fields = append(fields, headingField("651", label, ind2, sub2))
		case "Person":
			fields = append(fields, nameHeadingField("600", '1', ind2, label, sub2))
		case "Organization":
			fields = append(fields, nameHeadingField("610", '2', ind2, label, sub2))
		case "Meeting":
			fields = append(fields, nameHeadingField("611", '2', ind2, label, sub2))
		}
	}
	return fields
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
// $2 when the second indicator defers to it.
func headingField(tag, label string, ind2 byte, sub2 string) codex.Field {
	parts := strings.Split(label, "--")
	subs := []codex.Subfield{codex.NewSubfield('a', parts[0])}
	for _, p := range parts[1:] {
		subs = append(subs, codex.NewSubfield('x', p))
	}
	if sub2 != "" {
		subs = append(subs, codex.NewSubfield('2', sub2))
	}
	return codex.NewDataField(tag, ' ', ind2, subs...)
}

// nameHeadingField builds a 600/610/611 name subject from its label, carrying the
// thesaurus in $2 when the second indicator defers to it.
func nameHeadingField(tag string, ind1, ind2 byte, label, sub2 string) codex.Field {
	subs := []codex.Subfield{codex.NewSubfield('a', label)}
	if sub2 != "" {
		subs = append(subs, codex.NewSubfield('2', sub2))
	}
	return codex.NewDataField(tag, ind1, ind2, subs...)
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
// scheme in $2.
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
// (one per node), plus a reconstructed 008 carrying the country of the first node
// that has one, and a 264 _4 for a bf:copyrightDate. Provision classes collapse to
// the generic 260 rather than typed 264 indicators, keeping the transcribed
// statement front and centre in this library's flatter model.
func provisionFields(g *rdf.Graph, inst rdf.Term) []codex.Field {
	var fields []codex.Field
	country := ""
	for _, prov := range g.Objects(inst, pProvision) {
		if subs := provision26XSubfields(g, prov); len(subs) > 0 {
			fields = append(fields, codex.NewDataField("260", ' ', ' ', subs...))
		}
		if country == "" {
			country = countryCode(g, prov)
		}
	}
	if country != "" {
		fields = append(fields, codex.NewControlField("008", control008Country(country)))
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

// control008Country renders a minimal 40-byte 008 whose only populated field is the
// country at 15-17, so the reconstructed record carries the place back into the
// forward crosswalk without fabricating date or language positions.
func control008Country(code string) string {
	b := []byte(strings.Repeat(" ", 40))
	copy(b[15:18], code)
	return string(b)
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

// locators returns the bf:electronicLocator URIs (IRI references or literals).
func locators(g *rdf.Graph, inst rdf.Term) []string {
	var out []string
	for _, o := range g.Objects(inst, pLocator) {
		if o.Value != "" {
			out = append(out, o.Value)
		}
	}
	return out
}

// ---- title helpers ----

// firstTitle returns the components of a subject's first bf:Title.
func firstTitle(g *rdf.Graph, subject rdf.Term) Title {
	node, ok := g.Object(subject, pTitle)
	if !ok {
		return Title{}
	}
	return Title{
		MainTitle:  literal(g, node, pMainTitle),
		Subtitle:   literal(g, node, pSubtitle),
		PartNumber: literal(g, node, pPartNumber),
		PartName:   literal(g, node, pPartName),
	}
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

// leaderForClass returns a default leader with byte 6 (type of record) set to
// match a BIBFRAME content class, the inverse of workClass.
func leaderForClass(class string) codex.Leader {
	b := []byte(codex.NewRecord().Leader().String())
	if t := recordType(class); t != 0 {
		b[6] = t
	}
	return codex.Leader(b)
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
	case "Audio":
		return 'i'
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
