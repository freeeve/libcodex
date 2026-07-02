package bibframe

import (
	"sort"
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

func recordFromWork(g *rdf.Graph, work rdf.Term, backref map[rdf.Term]rdf.Term) *codex.Record {
	rec := codex.NewRecord()
	rec.SetLeader(leaderForClass(typeExcept(g, work, "Work")))

	inst, hasInst := g.Object(work, pHasInstance)
	if !hasInst {
		inst, hasInst = backref[work]
	}

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
	if p := provisionSubfields(g, inst); len(p) > 0 {
		add(codex.NewDataField("260", ' ', ' ', p...))
	}
	for _, e := range labelsOf(g, inst, pExtent) {
		add(codex.NewDataField("300", ' ', ' ', codex.NewSubfield('a', e)))
	}
	for _, m := range labelsOf(g, inst, pMedia) {
		add(codex.NewDataField("337", ' ', ' ', codex.NewSubfield('a', m)))
	}
	for _, c := range labelsOf(g, inst, pCarrier) {
		add(codex.NewDataField("338", ' ', ' ', codex.NewSubfield('a', c)))
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
		tag := contribTag(agentClass(g, agent), isPrimary)
		subs := []codex.Subfield{codex.NewSubfield('a', label)}
		if r := literal(g, roleNode(g, c), pLabel); r != "" {
			subs = append(subs, codex.NewSubfield('e', r))
		}
		fields = append(fields, codex.NewDataField(tag, '1', ' ', subs...))
		primary = primary || isPrimary
	}
	return fields, primary
}

// roleNode returns the bf:role node of a contribution (zero Term when absent).
func roleNode(g *rdf.Graph, contribution rdf.Term) rdf.Term {
	r, _ := g.Object(contribution, pRole)
	return r
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
		switch typeExcept(g, s, "") {
		case "Topic":
			fields = append(fields, headingField("650", label))
		case "Place":
			fields = append(fields, headingField("651", label))
		case "Person":
			fields = append(fields, codex.NewDataField("600", '1', '0', codex.NewSubfield('a', label)))
		case "Organization":
			fields = append(fields, codex.NewDataField("610", '2', '0', codex.NewSubfield('a', label)))
		case "Meeting":
			fields = append(fields, codex.NewDataField("611", '2', '0', codex.NewSubfield('a', label)))
		}
	}
	return fields
}

// headingField builds a 650/651 from a "--"-subdivided label: the first portion
// is $a, each remaining portion a general subdivision $x.
func headingField(tag, label string) codex.Field {
	parts := strings.Split(label, "--")
	subs := []codex.Subfield{codex.NewSubfield('a', parts[0])}
	for _, p := range parts[1:] {
		subs = append(subs, codex.NewSubfield('x', p))
	}
	return codex.NewDataField(tag, ' ', '0', subs...)
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
		switch typeExcept(g, id, "") {
		case "Isbn":
			fields = append(fields, identifierField("020", ' ', ' ', value, source))
		case "Issn":
			fields = append(fields, identifierField("022", ' ', ' ', value, source))
		case "Lccn":
			fields = append(fields, codex.NewDataField("010", ' ', ' ', codex.NewSubfield('a', strings.TrimSpace(value))))
		default:
			fields = append(fields, identifierField("024", '8', ' ', value, source))
		}
	}
	return fields
}

// identifierField builds an 020/022/024 from a value and an optional scheme,
// which round-trips through subfield $2.
func identifierField(tag string, ind1, ind2 byte, value, source string) codex.Field {
	subs := []codex.Subfield{codex.NewSubfield('a', value)}
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
		switch typeExcept(g, c, "") {
		case "ClassificationLcc":
			fields = append(fields, codex.NewDataField("050", ' ', '4', codex.NewSubfield('a', value)))
		case "ClassificationDdc":
			fields = append(fields, codex.NewDataField("082", ' ', '4', codex.NewSubfield('a', value)))
		case "Classification":
			subs := []codex.Subfield{codex.NewSubfield('a', value)}
			if src := sourceLabel(g, c); src != "" {
				subs = append(subs, codex.NewSubfield('2', src))
			}
			fields = append(fields, codex.NewDataField("072", ' ', '7', subs...))
		}
	}
	return fields
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
	var subs []codex.Subfield
	for _, l := range g.Objects(work, pLanguage) {
		if code := langCode(g, l); code != "" {
			subs = append(subs, codex.NewSubfield('a', code))
		}
	}
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

// provisionSubfields reverses bf:provisionActivity into 260 $a/$b/$c. It prefers
// LoC's transcribed bflc:simplePlace/simpleAgent/simpleDate (which map directly to
// the 260 statement) over the controlled bf:place/bf:agent nodes (whose labels are
// authority forms — e.g. the country, not the city), and falls back to the
// controlled labels this library's own output uses.
func provisionSubfields(g *rdf.Graph, inst rdf.Term) []codex.Subfield {
	prov, ok := g.Object(inst, pProvision)
	if !ok {
		return nil
	}
	var subs []codex.Subfield
	if place := firstNonEmpty(literal(g, prov, pSimplePlace), literal(g, mustNode(g, prov, pPlace), pLabel)); place != "" {
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
