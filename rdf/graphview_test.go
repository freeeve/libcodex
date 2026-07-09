package rdf

import (
	"reflect"
	"slices"
	"testing"
)

// viewDataset builds a dataset with two named graphs and a default graph, whose
// subjects overlap across graphs so a view that leaked another graph's
// statements would be caught.
func viewDataset() (*Dataset, Term, Term) {
	feed := NewIRI("http://ex/graphs/feed")
	edit := NewIRI("http://ex/graphs/editorial")
	work := NewIRI("http://ex/w1")
	other := NewIRI("http://ex/w2")

	d := &Dataset{}
	d.Add(work, NewIRI(TypeIRI), NewIRI("http://ex/Work"), feed)
	d.Add(work, NewIRI("http://ex/title"), NewLiteral("Feed title", "", ""), feed)
	d.Add(work, NewIRI("http://ex/tag"), NewLiteral("a", "", ""), feed)
	d.Add(work, NewIRI("http://ex/tag"), NewLiteral("b", "", ""), feed)
	d.Add(other, NewIRI(TypeIRI), NewIRI("http://ex/Work"), feed)

	// Same subject, same predicates, different graph: the editorial overlay.
	d.Add(work, NewIRI(TypeIRI), NewIRI("http://ex/Edited"), edit)
	d.Add(work, NewIRI("http://ex/title"), NewLiteral("Editorial title", "", ""), edit)

	d.Add(work, NewIRI("http://ex/note"), NewLiteral("default graph", "", ""), Term{})
	return d, feed, edit
}

// TestGraphViewMatchesGraph is the contract: every query on a view answers
// exactly what the same query on the materialized Dataset.Graph answers, for
// every graph in the dataset including the default graph.
func TestGraphViewMatchesGraph(t *testing.T) {
	d, _, _ := viewDataset()
	work := NewIRI("http://ex/w1")

	for _, graph := range append(d.Graphs(), NewIRI("http://ex/graphs/absent")) {
		t.Run(graph.Value, func(t *testing.T) {
			g := d.Graph(graph)
			v := d.GraphView(graph)

			if got, want := v.Len(), len(g.Triples); got != want {
				t.Errorf("Len = %d, want %d", got, want)
			}
			if got, want := slices.Collect(v.Triples()), g.Triples; !reflect.DeepEqual(got, want) {
				t.Errorf("Triples = %v, want %v", got, want)
			}
			if got, want := v.SubjectsOfType("http://ex/Work"), g.SubjectsOfType("http://ex/Work"); !reflect.DeepEqual(got, want) {
				t.Errorf("SubjectsOfType = %v, want %v", got, want)
			}
			for _, pred := range []string{"http://ex/title", "http://ex/tag", "http://ex/note", "http://ex/absent"} {
				if got, want := v.Objects(work, pred), g.Objects(work, pred); !reflect.DeepEqual(got, want) {
					t.Errorf("Objects(%s) = %v, want %v", pred, got, want)
				}
				gotT, gotOK := v.Object(work, pred)
				wantT, wantOK := g.Object(work, pred)
				if gotT != wantT || gotOK != wantOK {
					t.Errorf("Object(%s) = %v,%v want %v,%v", pred, gotT, gotOK, wantT, wantOK)
				}
				gotL, gotOK := v.Literal(work, pred)
				wantL, wantOK := g.Literal(work, pred)
				if gotL != wantL || gotOK != wantOK {
					t.Errorf("Literal(%s) = %q,%v want %q,%v", pred, gotL, gotOK, wantL, wantOK)
				}
			}
			for _, typ := range []string{"http://ex/Work", "http://ex/Edited", "http://ex/Absent"} {
				if got, want := v.HasType(work, typ), g.HasType(work, typ); got != want {
					t.Errorf("HasType(%s) = %v, want %v", typ, got, want)
				}
			}
		})
	}
}

// TestGraphViewIsolatesGraphs pins the property the overlap in viewDataset
// exists to test: a view never reports another graph's statements, even for a
// subject and predicate the two graphs share.
func TestGraphViewIsolatesGraphs(t *testing.T) {
	d, feed, edit := viewDataset()
	work := NewIRI("http://ex/w1")

	if got, _ := d.GraphView(feed).Literal(work, "http://ex/title"); got != "Feed title" {
		t.Errorf("feed title = %q, want the feed graph's title", got)
	}
	if got, _ := d.GraphView(edit).Literal(work, "http://ex/title"); got != "Editorial title" {
		t.Errorf("editorial title = %q, want the editorial graph's title", got)
	}
	if d.GraphView(edit).HasType(work, "http://ex/Work") {
		t.Error("editorial view sees the feed graph's rdf:type")
	}
	if d.GraphView(feed).HasType(work, "http://ex/Edited") {
		t.Error("feed view sees the editorial graph's rdf:type")
	}
	// The default graph is addressed by the zero-value term and holds only its own.
	def := d.GraphView(Term{})
	if got, _ := def.Literal(work, "http://ex/note"); got != "default graph" {
		t.Errorf("default-graph note = %q", got)
	}
	if def.Len() != 1 {
		t.Errorf("default graph Len = %d, want 1", def.Len())
	}
}

// TestGraphViewReindexesAfterAdd covers the one mutation hazard: a view caches an
// index over the dataset's quads, so appending must invalidate it rather than
// silently answer from a stale index.
func TestGraphViewReindexesAfterAdd(t *testing.T) {
	d, feed, _ := viewDataset()
	v := d.GraphView(feed)
	before := v.Len()

	w3 := NewIRI("http://ex/w3")
	d.Add(w3, NewIRI(TypeIRI), NewIRI("http://ex/Work"), feed)

	if got := v.Len(); got != before+1 {
		t.Errorf("Len after Add = %d, want %d", got, before+1)
	}
	if !v.HasType(w3, "http://ex/Work") {
		t.Error("view did not reindex after Add: new subject invisible")
	}
}

// TestGraphViewTriplesEarlyExit confirms the iterator honors an early break
// rather than walking the whole graph.
func TestGraphViewTriplesEarlyExit(t *testing.T) {
	d, feed, _ := viewDataset()
	n := 0
	for range d.GraphView(feed).Triples() {
		n++
		break
	}
	if n != 1 {
		t.Errorf("iterated %d triples after break, want 1", n)
	}
}

// TestGraphViewZeroCopy is the point of the type: reading a graph through a view
// must not allocate per statement the way materializing it does. The view's whole
// index is int32 positions, so it stays far below the Triple copies Graph makes.
func TestGraphViewZeroCopy(t *testing.T) {
	d, err := ParseNQuads(corpusNQ(200))
	if err != nil {
		t.Fatal(err)
	}
	feed := NewIRI("http://catalog.example.org/graphs/feed")

	graphAllocs := testing.AllocsPerRun(3, func() {
		g := d.Graph(feed)
		g.SubjectsOfType(bfWorkClass)
	})
	viewAllocs := testing.AllocsPerRun(3, func() {
		v := d.GraphView(feed)
		v.SubjectsOfType(bfWorkClass)
	})
	// Not a ratio assertion (allocation counts are coarse); the view must simply
	// make strictly fewer allocations than materializing the triples.
	if viewAllocs >= graphAllocs {
		t.Errorf("view allocs %.0f not fewer than Graph allocs %.0f", viewAllocs, graphAllocs)
	}
}
