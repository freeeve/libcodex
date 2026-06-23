package bibframe

import (
	"bytes"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/rdf"
)

// BIBFRAME vocabulary IRIs used by the reverse crosswalk.
const (
	classWork     = bfNS + "Work"
	classInstance = bfNS + "Instance"

	pType         = rdfNS + "type"
	pLabel        = rdfsNS + "label"
	pValue        = rdfNS + "value"
	pTitle        = bfNS + "title"
	pMainTitle    = bfNS + "mainTitle"
	pSubtitle     = bfNS + "subtitle"
	pPartNumber   = bfNS + "partNumber"
	pPartName     = bfNS + "partName"
	pContribution = bfNS + "contribution"
	pAgent        = bfNS + "agent"
	pRole         = bfNS + "role"
	pSubject      = bfNS + "subject"
	pGenreForm    = bfNS + "genreForm"
	pLanguage     = bfNS + "language"
	pClassif      = bfNS + "classification"
	pClassPortion = bfNS + "classificationPortion"
	pSummary      = bfNS + "summary"
	pHasInstance  = bfNS + "hasInstance"
	pInstanceOf   = bfNS + "instanceOf"
	pRespStmt     = bfNS + "responsibilityStatement"
	pEdition      = bfNS + "editionStatement"
	pProvision    = bfNS + "provisionActivity"
	pPlace        = bfNS + "place"
	pDate         = bfNS + "date"
	pExtent       = bfNS + "extent"
	pIdentifiedBy = bfNS + "identifiedBy"
	pLocator      = bfNS + "electronicLocator"

	primaryContribution = bflcNS + "PrimaryContribution"
)

// Decode parses a BIBFRAME document — RDF/XML or JSON-LD, autodetected — and
// reverse-crosswalks every bf:Work (with its linked bf:Instance) to a MARC 21
// record. It reads the vocabulary the forward crosswalk emits and the common
// shape of LoC marc2bibframe2 output. BIBFRAME is a lossier model than MARC, so
// the result carries the crosswalked fields rather than reproducing the original
// record byte for byte; re-encoding it yields an equivalent BIBFRAME graph.
func Decode(data []byte) ([]*codex.Record, error) {
	g, err := parseGraph(data)
	if err != nil {
		return nil, err
	}
	var out []*codex.Record
	for _, work := range g.SubjectsOfType(classWork) {
		out = append(out, recordFromWork(g, work))
	}
	return out, nil
}

// parseGraph picks the RDF parser by sniffing the first non-space byte: '{' or
// '[' is JSON-LD, anything else is RDF/XML.
func parseGraph(data []byte) (*rdf.Graph, error) {
	t := bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // optional UTF-8 BOM
	t = bytes.TrimLeft(t, " \t\r\n")
	if len(t) > 0 && (t[0] == '{' || t[0] == '[') {
		return rdf.ParseJSONLD(data)
	}
	return rdf.ParseRDFXML(data)
}

// recordFromWork builds a MARC record from one Work node and the Instance it
// links to, assembling fields in ascending tag order.
func recordFromWork(g *rdf.Graph, work rdf.Term) *codex.Record {
	rec := codex.NewRecord()
	rec.SetLeader(leaderForClass(typeExcept(g, work, "Work")))

	inst, hasInst := g.Object(work, pHasInstance)
	if !hasInst {
		inst, hasInst = instanceBackref(g, work)
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
		isPrimary := g.HasType(c, primaryContribution)
		tag := contribTag(typeExcept(g, agent, ""), isPrimary)
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

// contribTag picks the MARC tag for an agent class and primary/added entry.
func contribTag(class string, primary bool) string {
	base := map[string]string{"Organization": "10", "Meeting": "11"}[class]
	if base == "" {
		base = "00" // Person and unknown agents
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

// identifierFields reverses bf:identifiedBy into 020/022/024.
func identifierFields(g *rdf.Graph, inst rdf.Term) []codex.Field {
	var fields []codex.Field
	for _, id := range g.Objects(inst, pIdentifiedBy) {
		value := literal(g, id, pValue)
		if value == "" {
			continue
		}
		switch typeExcept(g, id, "") {
		case "Isbn":
			fields = append(fields, codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', value)))
		case "Issn":
			fields = append(fields, codex.NewDataField("022", ' ', ' ', codex.NewSubfield('a', value)))
		default:
			fields = append(fields, codex.NewDataField("024", '8', ' ', codex.NewSubfield('a', value)))
		}
	}
	return fields
}

// classificationFields reverses bf:classification into 050/082.
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
		}
	}
	return fields
}

// languageField reverses bf:language into a single 041 with one $a per code.
func languageField(g *rdf.Graph, work rdf.Term) []codex.Field {
	var subs []codex.Subfield
	for _, l := range g.Objects(work, pLanguage) {
		code := literal(g, l, pLabel)
		if code == "" && l.IsIRI() {
			code = rdf.LocalName(l.Value)
		}
		if isLangCode(strings.TrimSpace(code)) {
			subs = append(subs, codex.NewSubfield('a', strings.TrimSpace(code)))
		}
	}
	if len(subs) == 0 {
		return nil
	}
	return []codex.Field{codex.NewDataField("041", ' ', ' ', subs...)}
}

// provisionSubfields reverses bf:provisionActivity into 260 $a/$b/$c.
func provisionSubfields(g *rdf.Graph, inst rdf.Term) []codex.Subfield {
	prov, ok := g.Object(inst, pProvision)
	if !ok {
		return nil
	}
	var subs []codex.Subfield
	if place := literal(g, mustNode(g, prov, pPlace), pLabel); place != "" {
		subs = append(subs, codex.NewSubfield('a', place))
	}
	if pub := literal(g, mustNode(g, prov, pAgent), pLabel); pub != "" {
		subs = append(subs, codex.NewSubfield('b', pub))
	}
	if date := literal(g, prov, pDate); date != "" {
		subs = append(subs, codex.NewSubfield('c', date))
	}
	return subs
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

// instanceBackref finds an Instance whose bf:instanceOf points at the Work, for
// graphs that link only in that direction.
func instanceBackref(g *rdf.Graph, work rdf.Term) (rdf.Term, bool) {
	for _, inst := range g.SubjectsOfType(classInstance) {
		if o, ok := g.Object(inst, pInstanceOf); ok && o.Value == work.Value && o.Kind == work.Kind {
			return inst, true
		}
	}
	return rdf.Term{}, false
}

// literal returns the lexical value of subject's first literal object for the
// predicate, or "".
func literal(g *rdf.Graph, subject rdf.Term, predicate string) string {
	for _, o := range g.Objects(subject, predicate) {
		if o.IsLiteral() {
			return o.Value
		}
	}
	return ""
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
// crosswalk mints; other IRI shapes yield no control number.
func controlNumber(iri string) string {
	ln := rdf.LocalName(iri)
	if strings.HasPrefix(iri, "#") && strings.HasSuffix(ln, "Work") {
		return strings.TrimSuffix(ln, "Work")
	}
	return ""
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

// ---- entry points ----

// Reader reads BIBFRAME records from a stream. A BIBFRAME document is a single
// RDF graph, so the first Read parses the whole input; successive calls return
// the reconstructed records in document order, then io.EOF.
type Reader struct {
	src  io.Reader
	recs []*codex.Record
	i    int
	err  error
	done bool
}

// NewReader returns a Reader over r. It implements codex.RecordReader, so a
// BIBFRAME document can be a source for codex.Convert.
func NewReader(r io.Reader) *Reader { return &Reader{src: r} }

// Read returns the next record, or io.EOF when the document is exhausted.
func (rd *Reader) Read() (*codex.Record, error) {
	if !rd.done {
		rd.done = true
		var data []byte
		if data, rd.err = io.ReadAll(rd.src); rd.err == nil {
			rd.recs, rd.err = Decode(data)
		}
	}
	if rd.err != nil {
		return nil, rd.err
	}
	if rd.i >= len(rd.recs) {
		return nil, io.EOF
	}
	rec := rd.recs[rd.i]
	rd.i++
	return rec, nil
}

// ReadFile reads and decodes every BIBFRAME record in the file at path.
func ReadFile(path string) ([]*codex.Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(data)
}
