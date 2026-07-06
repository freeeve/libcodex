package bibframe

import "github.com/freeeve/libcodex/rdf"

// This file holds the three sinks that render the shape traversal in shape.go.
// Each keeps its formatting state in inline fixed-size stacks, so a sink adds no
// per-record heap allocation beyond its own value: the Work/Instance shape never
// nests more than a handful of levels deep.

const maxDepth = 16 // node/context nesting never approaches this

// ---- graph sink ----

// graphSink turns the traversal into triples. It keeps a stack of open subjects
// and a parallel stack of the predicates that link them to their parents, so a
// child node is joined to the subject that opened it.
type graphSink struct {
	gb    graphBuilder // held by value so the sink and builder are one allocation
	subj  []rdf.Term
	preds []qname
	subjA [maxDepth]rdf.Term
	predA [maxDepth]qname
	// Two-slot memo for the fragment IRIs: the Work and Instance nodes are each
	// referenced once (bf:hasInstance, bf:instanceOf) besides being declared, so
	// caching their joined term joins each IRI once, as the old builder did.
	fragKey  [2]iriVal
	fragTerm [2]rdf.Term
	fragN    int
}

// newGraphSink returns a sink that builds into a fresh graph, minting blank labels
// under prefix (the node base) so separately built graphs merge without colliding.
func newGraphSink(prefix string) *graphSink {
	s := &graphSink{gb: graphBuilder{g: &rdf.Graph{}, prefix: prefix}}
	s.subj = s.subjA[:0]
	s.preds = s.predA[:0]
	return s
}

// iriTerm returns the term for a fragment IRI, joining it at most once by memoizing
// the first two distinct IRIs (the Work and Instance nodes).
func (s *graphSink) iriTerm(iri iriVal) rdf.Term {
	for i := 0; i < s.fragN; i++ {
		if s.fragKey[i] == iri {
			return s.fragTerm[i]
		}
	}
	t := rdf.NewIRI(iri.join())
	if s.fragN < len(s.fragKey) {
		s.fragKey[s.fragN] = iri
		s.fragTerm[s.fragN] = t
		s.fragN++
	}
	return t
}

func (s *graphSink) beginNode(class qname, iri iriVal, extra qname) {
	var term rdf.Term
	if iri.blank() {
		term = s.gb.fresh()
	} else {
		term = s.iriTerm(iri)
	}
	if n := len(s.preds); n > 0 && len(s.subj) > 0 {
		s.gb.g.Add(s.subj[len(s.subj)-1], rdf.NewIRI(s.preds[n-1].fullIRI()), term)
	}
	if !class.empty() { // an rdf:Description node carries no rdf:type
		s.gb.typ(term, class.fullIRI())
	}
	if extra.local != "" {
		s.gb.typ(term, extra.fullIRI())
	}
	s.subj = append(s.subj, term)
}

func (s *graphSink) endNode() { s.subj = s.subj[:len(s.subj)-1] }

func (s *graphSink) lit(pred qname, text string) {
	s.gb.lit(s.subj[len(s.subj)-1], pred.fullIRI(), text)
}

func (s *graphSink) litTyped(pred qname, text, datatype string) {
	if text != "" {
		s.gb.g.Add(s.subj[len(s.subj)-1], rdf.NewIRI(pred.fullIRI()), rdf.NewLiteral(text, "", datatype))
	}
}

func (s *graphSink) litList(pred qname, texts []string) {
	if len(texts) == 0 {
		return
	}
	subj := s.subj[len(s.subj)-1]
	p := rdf.NewIRI(pred.fullIRI())
	for _, t := range texts {
		s.gb.g.Add(subj, p, rdf.NewLiteral(t, "", ""))
	}
}

func (s *graphSink) ref(pred qname, iri iriVal) {
	s.gb.g.Add(s.subj[len(s.subj)-1], rdf.NewIRI(pred.fullIRI()), s.iriTerm(iri))
}

func (s *graphSink) refList(pred qname, iris []string) {
	subj := s.subj[len(s.subj)-1]
	p := rdf.NewIRI(pred.fullIRI())
	for _, iri := range iris {
		s.gb.g.Add(subj, p, rdf.NewIRI(iri))
	}
}

func (s *graphSink) beginChild(pred qname) { s.preds = append(s.preds, pred) }
func (s *graphSink) endChild()             { s.preds = s.preds[:len(s.preds)-1] }
func (s *graphSink) beginList(pred qname)  { s.preds = append(s.preds, pred) }
func (s *graphSink) endList()              { s.preds = s.preds[:len(s.preds)-1] }

// ---- RDF/XML sink ----

const spacePad = "                        " // 24 spaces; indent never exceeds this in practice

func appendIndent(b []byte, n int) []byte {
	for n > len(spacePad) {
		b = append(b, spacePad...)
		n -= len(spacePad)
	}
	return append(b, spacePad[:n]...)
}

// appendQName appends the prefixed name. Constants append their folded pfx in one
// copy; data-derived names fall back to ns:local (still without concatenating).
func appendQName(b []byte, q qname) []byte {
	if q.pfx != "" {
		return append(b, q.pfx...)
	}
	b = append(b, q.ns...)
	b = append(b, ':')
	return append(b, q.local...)
}

// xnode records an open element for closing: its class (element name) and the
// wrapper predicate element around it (zero qname for a root).
type xnode struct{ pred, class qname }

// xmlSink emits nested elements, tracking indent depth, a stack of open nodes (for
// closing tags) and a stack of wrapper predicates (set by beginChild/beginList,
// read by the next beginNode).
type xmlSink struct {
	b     []byte
	depth int
	nodes []xnode
	preds []qname
	nodeA [maxDepth]xnode
	predA [maxDepth]qname
}

func newXMLSink(b []byte) *xmlSink {
	s := &xmlSink{}
	s.reset(b)
	return s
}

// reset points the sink at a fresh buffer and empties its stacks, so a streaming
// writer can reuse one sink (and its inline stacks) across records.
func (s *xmlSink) reset(b []byte) {
	s.b = b
	s.depth = 2
	s.nodes = s.nodeA[:0]
	s.preds = s.predA[:0]
}

func (s *xmlSink) beginNode(class qname, iri iriVal, extra qname) {
	var pred qname
	if n := len(s.preds); n > 0 {
		pred = s.preds[n-1]
	}
	if pred.local != "" {
		s.b = appendIndent(s.b, s.depth)
		s.b = append(s.b, '<')
		s.b = appendQName(s.b, pred)
		s.b = append(s.b, ">\n"...)
		s.depth += 2
	}
	if class.empty() { // an untyped node is the rdf:Description element
		class = qcDescription
	}
	s.b = appendIndent(s.b, s.depth)
	s.b = append(s.b, '<')
	s.b = appendQName(s.b, class)
	if !iri.blank() {
		// Escape the IRI: internal fragment/vocab IRIs are clean (a no-op here), but
		// a node IRI can also be untrusted record data, e.g. an 856 locator URL.
		s.b = append(s.b, ` rdf:about="`...)
		s.b = appendXMLAttr(s.b, iri.a)
		s.b = appendXMLAttr(s.b, iri.b)
		s.b = appendXMLAttr(s.b, iri.c)
		s.b = append(s.b, '"')
	}
	s.b = append(s.b, ">\n"...)
	s.depth += 2
	if extra.local != "" {
		s.b = appendIndent(s.b, s.depth)
		s.b = append(s.b, `<rdf:type rdf:resource="`...)
		s.b = append(s.b, nsIRI(extra.ns)...)
		s.b = append(s.b, extra.local...)
		s.b = append(s.b, "\"/>\n"...)
	}
	s.nodes = append(s.nodes, xnode{pred, class})
}

func (s *xmlSink) endNode() {
	xn := s.nodes[len(s.nodes)-1]
	s.nodes = s.nodes[:len(s.nodes)-1]
	s.depth -= 2
	s.b = appendIndent(s.b, s.depth)
	s.b = append(s.b, "</"...)
	s.b = appendQName(s.b, xn.class)
	s.b = append(s.b, ">\n"...)
	if xn.pred.local != "" {
		s.depth -= 2
		s.b = appendIndent(s.b, s.depth)
		s.b = append(s.b, "</"...)
		s.b = appendQName(s.b, xn.pred)
		s.b = append(s.b, ">\n"...)
	}
}

func (s *xmlSink) lit(pred qname, text string) {
	s.b = appendIndent(s.b, s.depth)
	s.b = append(s.b, '<')
	s.b = appendQName(s.b, pred)
	s.b = append(s.b, '>')
	s.b = appendXMLText(s.b, text)
	s.b = append(s.b, "</"...)
	s.b = appendQName(s.b, pred)
	s.b = append(s.b, ">\n"...)
}

func (s *xmlSink) litList(pred qname, texts []string) {
	for _, t := range texts {
		s.lit(pred, t)
	}
}

func (s *xmlSink) litTyped(pred qname, text, datatype string) {
	s.b = appendIndent(s.b, s.depth)
	s.b = append(s.b, '<')
	s.b = appendQName(s.b, pred)
	s.b = append(s.b, ` rdf:datatype="`...)
	s.b = append(s.b, datatype...) // a known-safe constant datatype IRI
	s.b = append(s.b, '"', '>')
	s.b = appendXMLText(s.b, text)
	s.b = append(s.b, "</"...)
	s.b = appendQName(s.b, pred)
	s.b = append(s.b, ">\n"...)
}

func (s *xmlSink) ref(pred qname, iri iriVal) {
	b := appendIndent(s.b, s.depth)
	b = append(b, '<')
	b = appendQName(b, pred)
	b = append(b, ` rdf:resource="`...)
	b = appendXMLAttr(b, iri.a)
	b = appendXMLAttr(b, iri.b)
	b = appendXMLAttr(b, iri.c)
	s.b = append(b, "\"/>\n"...)
}

func (s *xmlSink) refList(pred qname, iris []string) {
	for _, iri := range iris {
		b := appendIndent(s.b, s.depth)
		b = append(b, '<')
		b = appendQName(b, pred)
		b = append(b, ` rdf:resource="`...)
		b = appendXMLAttr(b, iri)
		s.b = append(b, "\"/>\n"...)
	}
}

func (s *xmlSink) beginChild(pred qname) { s.preds = append(s.preds, pred) }
func (s *xmlSink) endChild()             { s.preds = s.preds[:len(s.preds)-1] }
func (s *xmlSink) beginList(pred qname)  { s.preds = append(s.preds, pred) }
func (s *xmlSink) endList()              { s.preds = s.preds[:len(s.preds)-1] }

// ---- JSON-LD sink ----

// jframe tracks a property context: whether it is a list (JSON array) and, for a
// list, whether the next item is the first.
type jframe struct {
	list  bool
	first bool
}

// jsonSink emits JSON-LD objects. beginChild/beginList emit the key (and, for a
// list, the opening bracket); beginNode emits the object, inserting a comma between
// list items.
type jsonSink struct {
	b      []byte
	frames []jframe
	frameA [maxDepth]jframe
}

func newJSONSink(b []byte) *jsonSink {
	s := &jsonSink{}
	s.reset(b)
	return s
}

// reset points the sink at a fresh buffer and empties its stack, so a streaming
// writer can reuse one sink across records.
func (s *jsonSink) reset(b []byte) {
	s.b = b
	s.frames = s.frameA[:0]
}

func (s *jsonSink) beginNode(class qname, iri iriVal, extra qname) {
	if n := len(s.frames); n > 0 && s.frames[n-1].list {
		if !s.frames[n-1].first {
			s.b = append(s.b, ',')
		}
		s.frames[n-1].first = false
	}
	s.b = append(s.b, '{')
	if !iri.blank() {
		s.b = append(s.b, `"@id":"`...)
		s.b = appendJSONBody(s.b, iri.a)
		s.b = appendJSONBody(s.b, iri.b)
		s.b = appendJSONBody(s.b, iri.c)
		s.b = append(s.b, '"', ',')
	}
	s.b = append(s.b, `"@type":`...)
	switch {
	case class.empty(): // an untyped node: empty @type array, so key()'s comma still holds
		s.b = append(s.b, '[', ']')
	case extra.local != "":
		s.b = append(s.b, '[')
		s.b = appendJSONQName(s.b, class)
		s.b = append(s.b, ',')
		s.b = appendJSONQName(s.b, extra)
		s.b = append(s.b, ']')
	default:
		s.b = appendJSONQName(s.b, class)
	}
}

func (s *jsonSink) endNode() { s.b = append(s.b, '}') }

func (s *jsonSink) lit(pred qname, text string) {
	s.b = s.key(pred)
	s.b = appendJSONString(s.b, text)
}

func (s *jsonSink) litList(pred qname, texts []string) {
	if len(texts) == 0 {
		return
	}
	s.b = s.key(pred)
	s.b = append(s.b, '[')
	for i, t := range texts {
		if i > 0 {
			s.b = append(s.b, ',')
		}
		s.b = appendJSONString(s.b, t)
	}
	s.b = append(s.b, ']')
}

func (s *jsonSink) litTyped(pred qname, text, datatype string) {
	s.b = s.key(pred)
	s.b = append(s.b, `{"@value":`...)
	s.b = appendJSONString(s.b, text)
	s.b = append(s.b, `,"@type":`...)
	s.b = appendJSONString(s.b, datatype)
	s.b = append(s.b, '}')
}

func (s *jsonSink) ref(pred qname, iri iriVal) {
	s.b = s.key(pred)
	s.b = append(s.b, `{"@id":"`...)
	s.b = appendJSONBody(s.b, iri.a)
	s.b = appendJSONBody(s.b, iri.b)
	s.b = appendJSONBody(s.b, iri.c)
	s.b = append(s.b, '"', '}')
}

func (s *jsonSink) refList(pred qname, iris []string) {
	s.b = s.key(pred)
	s.b = append(s.b, '[')
	for i, iri := range iris {
		if i > 0 {
			s.b = append(s.b, ',')
		}
		s.b = appendJSONRef(s.b, iri)
	}
	s.b = append(s.b, ']')
}

func (s *jsonSink) beginChild(pred qname) {
	s.b = s.key(pred)
	s.frames = append(s.frames, jframe{})
}

func (s *jsonSink) endChild() { s.frames = s.frames[:len(s.frames)-1] }

func (s *jsonSink) beginList(pred qname) {
	s.b = s.key(pred)
	s.b = append(s.b, '[')
	s.frames = append(s.frames, jframe{list: true, first: true})
}

func (s *jsonSink) endList() {
	s.b = append(s.b, ']')
	s.frames = s.frames[:len(s.frames)-1]
}

// key appends `,"ns:local":`; every property follows the always-present @type, so
// the leading comma is unconditional.
func (s *jsonSink) key(pred qname) []byte {
	b := append(s.b, ',', '"')
	b = appendQName(b, pred)
	return append(b, '"', ':')
}

// appendJSONQName appends a quoted prefixed name ("ns:local").
func appendJSONQName(b []byte, q qname) []byte {
	b = append(b, '"')
	b = appendQName(b, q)
	return append(b, '"')
}

// appendJSONRef appends an IRI reference object {"@id":"uri"}.
func appendJSONRef(b []byte, uri string) []byte {
	b = append(b, `{"@id":`...)
	b = appendJSONString(b, uri)
	return append(b, '}')
}
