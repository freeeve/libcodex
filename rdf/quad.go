package rdf

import (
	"slices"
	"strings"
)

// Quad is an RDF statement in a named graph: a triple plus the graph it belongs
// to. A zero-value graph term (G) denotes the default graph, so a Quad is a
// strict superset of a Triple.
type Quad struct {
	S, P, O, G Term
}

// Triple projects the quad onto its statement, dropping the graph term.
func (q Quad) Triple() Triple { return Triple{q.S, q.P, q.O} }

// Dataset is a set of quads — RDF statements each tagged with the graph they
// belong to. It is the quad-level analogue of Graph and what ParseNQuads
// returns; it is how provenance (which source a statement came from) is carried.
type Dataset struct {
	Quads []Quad
}

// Add appends a quad.
func (d *Dataset) Add(s, p, o, g Term) { d.Quads = append(d.Quads, Quad{s, p, o, g}) }

// Graph returns the triples belonging to the given graph term as a Graph; pass a
// zero-value term for the default graph.
func (d *Dataset) Graph(graph Term) *Graph {
	g := &Graph{}
	for _, q := range d.Quads {
		if q.G == graph {
			g.Triples = append(g.Triples, q.Triple())
		}
	}
	return g
}

// Graphs returns the distinct graph terms present in the dataset, in first-seen
// order — the set of provenance sources.
func (d *Dataset) Graphs() []Term {
	var out []Term
	seen := map[Term]bool{}
	for _, q := range d.Quads {
		if !seen[q.G] {
			seen[q.G] = true
			out = append(out, q.G)
		}
	}
	return out
}

// ---- serialization ----

// Encoder serializes a sequence of graphs into one document — N-Triples,
// N-Quads or Turtle — keeping blank-node labels unique across the graphs it
// writes. Blank-node labels are scoped to the whole document, so a single
// Encoder must be reused for every graph written to one stream; a fresh encoder
// per graph would let graphs that each number their blanks from scratch (such as
// one BIBFRAME graph per record) collide and merge. It is the serialization
// counterpart to Decoder; the whole-graph Graph.NTriples, Graph.NQuads and
// Graph.Turtle methods each use a fresh Encoder internally.
type Encoder struct {
	bn blankNamer
}

// AppendQuad appends one quad to b as an N-Quads line. A default-graph quad
// (zero-value G) is written as a three-term N-Triples line.
func (e *Encoder) AppendQuad(b []byte, q Quad) []byte {
	b = appendNTTerm(b, q.S, &e.bn)
	b = append(b, ' ')
	b = appendNTTerm(b, q.P, &e.bn)
	b = append(b, ' ')
	b = appendNTTerm(b, q.O, &e.bn)
	if q.G != (Term{}) { // omit the graph term for the default graph
		b = append(b, ' ')
		b = appendNTTerm(b, q.G, &e.bn)
	}
	return append(b, ' ', '.', '\n')
}

// AppendNQuads appends every triple of g to b as N-Quads tagged with the graph
// term — g's named graph / provenance. A zero-value graph term writes the
// default graph (plain N-Triples). Each call is a fresh blank-node scope, so
// graphs that label their blanks from scratch never merge, while their output
// labels stay unique across the whole document.
func (e *Encoder) AppendNQuads(b []byte, g *Graph, graph Term) []byte {
	e.bn.newScope()
	n := 0
	for _, t := range g.Triples {
		n += quadBytes(t.S, t.P, t.O, graph)
	}
	b = slices.Grow(b, n) // one growth instead of the log(n) of an empty start
	for _, t := range g.Triples {
		b = e.AppendQuad(b, Quad{t.S, t.P, t.O, graph})
	}
	return b
}

// AppendNTriples appends g's triples to b as N-Triples in a fresh blank-node
// scope — N-Triples being N-Quads restricted to the default graph.
func (e *Encoder) AppendNTriples(b []byte, g *Graph) []byte {
	return e.AppendNQuads(b, g, Term{})
}

// quadBytes estimates the serialized N-Quads byte length of one statement, used
// to pre-size output buffers. It ignores escaping, so the count is a slight
// undercount that removes the reallocations of growing a buffer from empty.
func quadBytes(s, p, o, g Term) int {
	n := 14 + len(s.Value) + len(p.Value) + len(o.Value) // brackets, spaces, " .\n"
	if o.Kind == Literal {
		n += len(o.Lang) + len(o.Datatype) + 6
	}
	if g != (Term{}) {
		n += len(g.Value) + 3
	}
	return n
}

// NQuads serializes the dataset as an N-Quads document.
func (d *Dataset) NQuads() []byte {
	var e Encoder
	n := 0
	for _, q := range d.Quads {
		n += quadBytes(q.S, q.P, q.O, q.G)
	}
	b := make([]byte, 0, n)
	for _, q := range d.Quads {
		b = e.AppendQuad(b, q)
	}
	return b
}

// NQuads serializes the graph's triples as N-Quads, each tagged with the given
// graph term (its named graph / provenance). A zero-value graph term produces
// plain N-Triples.
func (g *Graph) NQuads(graph Term) []byte {
	var e Encoder
	return e.AppendNQuads(nil, g, graph)
}

// ---- N-Quads parsing ----

// ParseNQuads parses an N-Quads document into a Dataset, preserving each
// statement's graph term; lines carrying only three terms fall in the default
// graph. It shares the N-Triples term reader, so the same escaping and leniency
// apply — blank, comment and malformed lines are skipped. One private copy of
// the input backs every term, so data is free for reuse once it returns.
func ParseNQuads(data []byte) (*Dataset, error) {
	return parseNQuads(string(data))
}

// ParseNQuadsShared is ParseNQuads without the private input copy: terms are
// zero-copy views into data itself, saving one input-sized allocation — worth
// it when read-heavy consumers parse corpus-scale documents. In exchange the
// caller must not modify data while the Dataset, or any Graph or Term derived
// from it, remains in use.
func ParseNQuadsShared(data []byte) (*Dataset, error) {
	return parseNQuads(bytesView(data))
}

func parseNQuads(data string) (*Dataset, error) {
	d := &Dataset{Quads: make([]Quad, 0, strings.Count(data, "\n")+1)}
	var a arena
	for line := range strings.SplitSeq(data, "\n") {
		if q, ok := parseNQuadLine(line, &a); ok {
			d.Quads = append(d.Quads, q)
		}
	}
	return d, nil
}
