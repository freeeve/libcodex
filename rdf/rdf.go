// Package rdf is a small, fast, dependency-free RDF toolkit. It provides the RDF
// triple model (Term, Triple, Graph), parsers and serializers for the four common
// serializations — RDF/XML, JSON-LD, Turtle and N-Triples — and a streaming
// decoder for the line-based formats.
//
// Two reading modes:
//
//   - Whole document: ParseRDFXML, ParseJSONLD, ParseTurtle and ParseNTriples take
//     a []byte and return a *Graph. Fast and convenient for inputs that fit in
//     memory. The line-based parsers have zero-copy variants (ParseNTriplesShared,
//     ParseNQuadsShared) that back terms with the caller's buffer instead of a
//     private copy — one input-sized allocation less, for callers that keep the
//     buffer immutable.
//   - Streaming: NewDecoder reads N-Triples, N-Quads, RDF/XML or Turtle from an
//     io.Reader one triple at a time in constant memory, for inputs too large to
//     materialize (e.g. the multi-gigabyte Library of Congress authority dumps).
//     JSON-LD is whole-document only.
//
// A parsed N-Quads document is a Dataset. Reading one of its named graphs has two
// modes: Dataset.Graph materializes the graph's triples as a Graph, while
// Dataset.GraphView answers the same queries (the GraphQuery surface) over
// positions into the dataset's own quads, copying no triple. Prefer the view for
// read-only access at corpus scale, where the copy dominates the allocation
// profile.
//
// The parsers target the constructs real-world RDF uses rather than the whole of
// each specification; see each parser's documentation for what is and isn't
// handled (notably, relative-IRI resolution against a document base is not
// performed). There are no third-party dependencies.
package rdf

import (
	"slices"
	"strings"
)

// Well-known IRIs.
const (
	NS         = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	TypeIRI    = NS + "type"
	FirstIRI   = NS + "first"
	RestIRI    = NS + "rest"
	NilIRI     = NS + "nil"
	langString = NS + "langString"
	XSDString  = "http://www.w3.org/2001/XMLSchema#string"
)

// Kind distinguishes the three RDF term types.
type Kind uint8

const (
	IRI Kind = iota
	Blank
	Literal
)

// Term is an RDF term: an IRI, a blank node, or a literal.
type Term struct {
	Kind     Kind
	Value    string // IRI, blank-node identifier, or a literal's lexical form
	Lang     string // literal language tag (Literal only)
	Datatype string // literal datatype IRI (Literal only)
}

// NewIRI, NewBlank and NewLiteral construct the three term kinds.
func NewIRI(s string) Term    { return Term{Kind: IRI, Value: s} }
func NewBlank(id string) Term { return Term{Kind: Blank, Value: id} }

// NewLiteral makes a literal; an empty datatype defaults per RDF (xsd:string, or
// rdf:langString when a language tag is present).
func NewLiteral(value, lang, datatype string) Term {
	if datatype == "" {
		if lang != "" {
			datatype = langString
		} else {
			datatype = XSDString
		}
	}
	return Term{Kind: Literal, Value: value, Lang: lang, Datatype: datatype}
}

// IsIRI, IsBlank and IsLiteral report the term kind.
func (t Term) IsIRI() bool     { return t.Kind == IRI }
func (t Term) IsBlank() bool   { return t.Kind == Blank }
func (t Term) IsLiteral() bool { return t.Kind == Literal }

// Triple is an RDF statement.
type Triple struct {
	S, P, O Term
}

// Graph holds the triples of one RDF graph in document order, with simple
// lookup helpers built on first use. Term is comparable, so it indexes by
// subject term directly — no key strings.
//
// Triples is a list, not a set. RDF 1.1 defines a graph as "a set of RDF
// triples", but a serialization is free to state the same triple more than
// once, and real ones do: LC's own marc2bibframe2 output repeats a shared
// agency or vocabulary node under every property that references it, so one of
// their N-Triples files carries 449 lines for 389 distinct triples. Parsing
// preserves what the document said, which keeps the parse allocation-light and
// keeps redundant statements visible to a caller who cares. It also means
// len(g.Triples) can exceed the triple count a set-backed parser such as rdflib
// reports for the same document.
//
// The query helpers below hide that: [Graph.Objects] and [Graph.SubjectsOfType]
// return each distinct answer once. Callers who want set semantics over the
// triples themselves can call [Graph.Dedupe], and [Graph.Canonical] already
// collapses duplicates on its way to canonical form.
type Graph struct {
	// Triples are in document order and may repeat; see the type doc.
	Triples []Triple

	spo map[Term][]int32 // subject -> positions in Triples, carved from one shared arena
}

// Dedupe removes duplicate triples in place, keeping the first occurrence of
// each and preserving document order among the survivors, and reports how many
// it removed. It gives the graph the set semantics RDF 1.1 ascribes to one, at
// the cost of a hash of every triple — which is why parsing does not do it.
func (g *Graph) Dedupe() int {
	if len(g.Triples) < 2 {
		return 0
	}
	seen := make(map[Triple]struct{}, len(g.Triples))
	out := g.Triples[:0]
	for _, t := range g.Triples {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	removed := len(g.Triples) - len(out)
	if removed > 0 {
		g.Triples = out
		g.spo = nil // positions shifted; invalidate the index
	}
	return removed
}

// Add appends a triple.
func (g *Graph) Add(s, p, o Term) {
	g.Triples = append(g.Triples, Triple{s, p, o})
	g.spo = nil // invalidate the index
}

// index builds the lazy subject index in two passes: count the triples per
// subject, then carve each subject's bucket from one shared arena at exactly
// its final size. Buckets hold int32 positions into Triples rather than Triple
// copies, so a corpus-scale build costs a few bytes per triple instead of the
// hundreds that per-subject append growth over copied triples did. (int32
// bounds a graph at 2^31 triples — hundreds of gigabytes of Triple values,
// far past in-memory reach.)
func (g *Graph) index() {
	if g.spo != nil {
		return
	}
	counts := make(map[Term]int32, len(g.Triples)/4+1)
	for i := range g.Triples {
		counts[g.Triples[i].S]++
	}
	arena := make([]int32, 0, len(g.Triples))
	spo := make(map[Term][]int32, len(counts))
	for i := range g.Triples {
		s := g.Triples[i].S
		bucket, ok := spo[s]
		if !ok {
			n := len(arena) + int(counts[s])
			bucket = arena[len(arena):len(arena):n]
			arena = arena[:n]
		}
		spo[s] = append(bucket, int32(i))
	}
	g.spo = spo
}

// Objects returns the distinct objects of every triple with the given subject
// and predicate IRI, in document order. A document that states the same triple
// twice yields that object once: the answer is a property's value set, not a
// count of how often the document repeated itself.
func (g *Graph) Objects(subject Term, predicate string) []Term {
	g.index()
	bucket := g.spo[subject]
	var out []Term
	seen := objectSet{hint: len(bucket)}
	for _, i := range bucket {
		if t := &g.Triples[i]; t.P.Kind == IRI && t.P.Value == predicate {
			out = seen.append(out, t.O)
		}
	}
	return out
}

// objectSetMapAbove is the result length past which objectSet stops scanning
// linearly and builds a hash set. A property's value set is almost always a
// handful of terms, so the scan wins for every realistic subject; the map only
// exists so a pathological subject with thousands of objects cannot go
// quadratic.
const objectSetMapAbove = 16

// objectSet accumulates the distinct objects of one (subject, predicate) as
// they are found, cheaply for the small results that dominate and safely for
// the large ones that do not. hint bounds the distinct objects from above --
// the subject's whole index bucket -- and sizes the hash set in one allocation
// if the result ever grows into one. The zero value degrades to an unsized map,
// so callers should set hint.
type objectSet struct {
	seen map[Term]struct{}
	hint int
}

// append adds o to out unless an equal term is already there, returning the
// possibly-grown slice.
func (s *objectSet) append(out []Term, o Term) []Term {
	if s.seen != nil {
		if _, dup := s.seen[o]; dup {
			return out
		}
		s.seen[o] = struct{}{}
		return append(out, o)
	}
	if slices.Contains(out, o) {
		return out
	}
	if len(out) >= objectSetMapAbove {
		s.seen = make(map[Term]struct{}, max(s.hint, len(out)+1))
		for _, t := range out {
			s.seen[t] = struct{}{}
		}
		s.seen[o] = struct{}{}
	}
	return append(out, o)
}

// ObjectsWithRepeats returns the objects of every triple with the given subject
// and predicate IRI, statement for statement, in document order -- including a
// term the document stated more than once.
//
// This is the raw list view, and it exists because [Graph.Triples] is a list.
// Prefer [Graph.Objects]: a repeated statement carries no information in RDF, so
// a caller that treats its multiplicity as meaningful is reading something the
// abstract syntax does not say, and will disagree with any set-backed store that
// touches the same document. The one honest use is inspecting a serialization as
// written -- counting redundant statements, or round-tripping a positional
// encoding that a conformant parser would already have flattened.
func (g *Graph) ObjectsWithRepeats(subject Term, predicate string) []Term {
	g.index()
	var out []Term
	for _, i := range g.spo[subject] {
		if t := &g.Triples[i]; t.P.Kind == IRI && t.P.Value == predicate {
			out = append(out, t.O)
		}
	}
	return out
}

// Object returns the first object for (subject, predicate), or false. It scans the
// index without allocating an intermediate slice.
func (g *Graph) Object(subject Term, predicate string) (Term, bool) {
	g.index()
	for _, i := range g.spo[subject] {
		if t := &g.Triples[i]; t.P.Kind == IRI && t.P.Value == predicate {
			return t.O, true
		}
	}
	return Term{}, false
}

// HasType reports whether the subject has rdf:type typeIRI. It scans the index
// without allocating.
func (g *Graph) HasType(subject Term, typeIRI string) bool {
	g.index()
	for _, i := range g.spo[subject] {
		if t := &g.Triples[i]; t.P.Kind == IRI && t.P.Value == TypeIRI && t.O.Kind == IRI && t.O.Value == typeIRI {
			return true
		}
	}
	return false
}

// Literal returns the value of the subject's first literal object for the
// predicate, or false. It scans the index without allocating an intermediate
// slice, serving the frequent single-value field reads.
func (g *Graph) Literal(subject Term, predicate string) (string, bool) {
	g.index()
	for _, i := range g.spo[subject] {
		if t := &g.Triples[i]; t.P.Kind == IRI && t.P.Value == predicate && t.O.Kind == Literal {
			return t.O.Value, true
		}
	}
	return "", false
}

// SubjectsOfType returns every subject with rdf:type typeIRI, in document order
// (deduplicated).
func (g *Graph) SubjectsOfType(typeIRI string) []Term {
	var out []Term
	seen := map[Term]bool{}
	for _, t := range g.Triples {
		if t.P.Kind == IRI && t.P.Value == TypeIRI && t.O.Kind == IRI && t.O.Value == typeIRI {
			if !seen[t.S] {
				seen[t.S] = true
				out = append(out, t.S)
			}
		}
	}
	return out
}

// LocalName returns the fragment or last path segment of an IRI (the part after
// the final '#' or '/').
func LocalName(iri string) string {
	if i := strings.LastIndexByte(iri, '#'); i >= 0 {
		return iri[i+1:]
	}
	if i := strings.LastIndexByte(iri, '/'); i >= 0 {
		return iri[i+1:]
	}
	return iri
}
