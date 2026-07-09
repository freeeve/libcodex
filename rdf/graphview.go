package rdf

import "iter"

// This file gives Dataset a zero-copy read path. Dataset.Graph materializes one
// Triple per matching quad, which at corpus scale dominates the allocation
// profile of any consumer that splits a dataset into its named graphs and then
// only reads them. GraphView answers the same queries over positions into the
// dataset's own Quads slice, so a view costs a few bytes per statement instead
// of a whole Triple (three Terms) per statement.

// GraphQuery is the read-only query surface shared by *Graph and *GraphView, so
// a consumer can write one function over a materialized graph and a view alike.
type GraphQuery interface {
	SubjectsOfType(typeIRI string) []Term
	Objects(subject Term, predicate string) []Term
	Object(subject Term, predicate string) (Term, bool)
	Literal(subject Term, predicate string) (string, bool)
	HasType(subject Term, typeIRI string) bool
}

var (
	_ GraphQuery = (*Graph)(nil)
	_ GraphQuery = (*GraphView)(nil)
)

// GraphView is a read-only view of the statements in one named graph of a
// Dataset. It answers the Graph query surface without copying any triple: its
// lazy index holds int32 positions into the dataset's Quads, the same shape as
// Graph's subject index. Pass a zero-value graph term to view the default graph.
//
// A view borrows the dataset, so the dataset must outlive it. Appending to the
// dataset invalidates the view's index; the next query rebuilds it, so a view
// stays correct across Add at the cost of one reindex. Views are not safe for
// concurrent use with a writer, and the first query on a view builds the index,
// so concurrent readers must either share a view whose index is already built
// or hold one view each.
type GraphView struct {
	d     *Dataset
	graph Term

	spo   map[Term][]int32 // subject -> positions in d.Quads, carved from one shared arena
	gen   int              // len(d.Quads) when spo was built, to detect appends
	spoOK bool
}

// GraphView returns a read-only, zero-copy view of the given graph term; pass a
// zero-value term for the default graph. It is the allocation-free counterpart
// to Graph, which materializes the same statements as a []Triple copy.
func (d *Dataset) GraphView(graph Term) *GraphView {
	return &GraphView{d: d, graph: graph}
}

// subjectIndex builds the view's lazy subject index on first subject-keyed query,
// in two passes mirroring Graph.index: count this graph's quads per subject, then
// carve each subject's bucket from one shared arena at exactly its final size.
// Buckets hold int32 positions into the dataset's Quads, so the index costs a few
// bytes per statement and copies no triple. Both passes skip quads belonging to
// other graphs, so the index is sized to the view rather than to the dataset.
//
// The whole-graph scans (Len, Triples, SubjectsOfType) deliberately do not go
// through here: they need no subject keying, and building the map for them would
// cost more than the scan itself.
func (v *GraphView) subjectIndex() map[Term][]int32 {
	if v.spoOK && v.gen == len(v.d.Quads) {
		return v.spo
	}
	quads := v.d.Quads
	n := 0
	counts := make(map[Term]int32, len(quads)/8+1)
	for i := range quads {
		if quads[i].G == v.graph {
			n++
			counts[quads[i].S]++
		}
	}
	arena := make([]int32, 0, n)
	spo := make(map[Term][]int32, len(counts))
	for i := range quads {
		if quads[i].G != v.graph {
			continue
		}
		s := quads[i].S
		bucket, ok := spo[s]
		if !ok {
			end := len(arena) + int(counts[s])
			bucket = arena[len(arena):len(arena):end]
			arena = arena[:end]
		}
		spo[s] = append(bucket, int32(i))
	}
	v.spo, v.gen, v.spoOK = spo, len(quads), true
	return spo
}

// Len returns the number of statements in the view's graph. It reads the
// dataset's cached per-graph counts, so it costs no scan after the first call on
// that dataset, however many graphs are asked about.
func (v *GraphView) Len() int { return v.d.GraphLen(v.graph) }

// Empty reports whether the view's graph holds no statements. Like Len it answers
// from the dataset's cached counts, so a consumer that reads a graph only when it
// is populated — an editorial overlay, say — pays nothing to skip an absent one.
func (v *GraphView) Empty() bool { return !v.d.HasGraph(v.graph) }

// GraphTerm returns the graph term this view is scoped to.
func (v *GraphView) GraphTerm() Term { return v.graph }

// Triples iterates the view's statements in document order, projecting each quad
// onto its triple. It builds no index and materializes no slice, yielding one
// Triple value at a time from the dataset's quads; the only allocation is the
// iterator closure itself, a fixed ~56 bytes per call regardless of graph size.
//
// The cost that matters is the pass, not the yield. Triples filters the whole
// dataset, so walking it is one full-dataset pass per view — reading N graphs out
// of one dataset costs N passes, even for graphs that turn out to be empty. Code
// that consumes several graphs at once can beat that by fusing the dispatch into
// a single hand-written pass over Dataset.Quads switching on each quad's graph
// term. Use Empty to skip a graph without walking it at all: it reads the
// dataset's cached counts and never scans.
//
// Per-triple overhead, by contrast, is not worth avoiding: in this package's
// Corpus/Grain/SingleGraph Triples benchmarks the iterator beats an equivalent
// hand-written single-graph loop at both corpus and per-grain scale, even against
// a single-graph dataset whose loop needs no filter at all.
func (v *GraphView) Triples() iter.Seq[Triple] {
	return func(yield func(Triple) bool) {
		for i := range v.d.Quads {
			if q := &v.d.Quads[i]; q.G == v.graph {
				if !yield(q.Triple()) {
					return
				}
			}
		}
	}
}

// Objects returns the objects of every statement in this graph with the given
// subject and predicate IRI, in document order.
func (v *GraphView) Objects(subject Term, predicate string) []Term {
	var out []Term
	for _, i := range v.subjectIndex()[subject] {
		if q := &v.d.Quads[i]; q.P.Kind == IRI && q.P.Value == predicate {
			out = append(out, q.O)
		}
	}
	return out
}

// Object returns the first object for (subject, predicate) in this graph, or
// false. It scans the index without allocating an intermediate slice.
func (v *GraphView) Object(subject Term, predicate string) (Term, bool) {
	for _, i := range v.subjectIndex()[subject] {
		if q := &v.d.Quads[i]; q.P.Kind == IRI && q.P.Value == predicate {
			return q.O, true
		}
	}
	return Term{}, false
}

// HasType reports whether the subject has rdf:type typeIRI in this graph. It
// scans the index without allocating.
func (v *GraphView) HasType(subject Term, typeIRI string) bool {
	for _, i := range v.subjectIndex()[subject] {
		if q := &v.d.Quads[i]; q.P.Kind == IRI && q.P.Value == TypeIRI && q.O.Kind == IRI && q.O.Value == typeIRI {
			return true
		}
	}
	return false
}

// Literal returns the value of the subject's first literal object for the
// predicate in this graph, or false. It scans the index without allocating an
// intermediate slice, serving the frequent single-value field reads.
func (v *GraphView) Literal(subject Term, predicate string) (string, bool) {
	for _, i := range v.subjectIndex()[subject] {
		if q := &v.d.Quads[i]; q.P.Kind == IRI && q.P.Value == predicate && q.O.Kind == Literal {
			return q.O.Value, true
		}
	}
	return "", false
}

// SubjectsOfType returns every subject in this graph with rdf:type typeIRI, in
// document order (deduplicated). Like Graph.SubjectsOfType it scans rather than
// keying off the subject index, so it never triggers an index build.
func (v *GraphView) SubjectsOfType(typeIRI string) []Term {
	var out []Term
	seen := map[Term]bool{}
	for i := range v.d.Quads {
		q := &v.d.Quads[i]
		if q.G == v.graph && q.P.Kind == IRI && q.P.Value == TypeIRI && q.O.Kind == IRI && q.O.Value == typeIRI {
			if !seen[q.S] {
				seen[q.S] = true
				out = append(out, q.S)
			}
		}
	}
	return out
}
