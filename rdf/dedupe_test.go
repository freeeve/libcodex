package rdf

import (
	"strconv"
	"strings"
	"testing"
)

// dupDoc states one triple twice and a second triple once, the shape LC's own
// BIBFRAME serializations produce whenever a shared node is referenced from two
// properties.
const dupDoc = `<http://a> <http://p> <http://o> .
<http://a> <http://p> <http://o> .
<http://a> <http://p> <http://o2> .
`

// TestParsePreservesDuplicates pins the deliberate choice: parsing keeps the
// document's list of triples, duplicates and all, rather than collapsing to the
// set RDF 1.1 defines. Callers who want the set call Dedupe or Canonical.
func TestParsePreservesDuplicates(t *testing.T) {
	g, err := ParseNTriples([]byte(dupDoc))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Triples) != 3 {
		t.Errorf("len(Triples) = %d, want 3 (the document's own count)", len(g.Triples))
	}
}

// TestDedupe checks Dedupe collapses to the set, keeps first-occurrence order,
// reports the count removed, and invalidates the subject index it invalidated
// positions for.
func TestDedupe(t *testing.T) {
	g, err := ParseNTriples([]byte(dupDoc))
	if err != nil {
		t.Fatal(err)
	}
	// Force the index to exist so Dedupe has something stale to invalidate.
	g.index()

	if removed := g.Dedupe(); removed != 1 {
		t.Errorf("Dedupe removed %d, want 1", removed)
	}
	if len(g.Triples) != 2 {
		t.Fatalf("len(Triples) = %d, want 2", len(g.Triples))
	}
	if g.Triples[0].O.Value != "http://o" || g.Triples[1].O.Value != "http://o2" {
		t.Errorf("Dedupe did not keep first-occurrence order: %v", g.Triples)
	}
	// The index must have been rebuilt against the compacted slice, not left
	// pointing at positions that no longer hold what they held.
	objs := g.Objects(NewIRI("http://a"), "http://p")
	if len(objs) != 2 {
		t.Errorf("Objects after Dedupe = %d terms, want 2", len(objs))
	}

	if removed := g.Dedupe(); removed != 0 {
		t.Errorf("second Dedupe removed %d, want 0", removed)
	}
}

// TestDedupeEmptyAndSingle guards the early return.
func TestDedupeEmptyAndSingle(t *testing.T) {
	var g Graph
	if removed := g.Dedupe(); removed != 0 || len(g.Triples) != 0 {
		t.Errorf("empty Dedupe: removed=%d len=%d", removed, len(g.Triples))
	}
	g.Add(NewIRI("http://a"), NewIRI("http://p"), NewIRI("http://o"))
	if removed := g.Dedupe(); removed != 0 || len(g.Triples) != 1 {
		t.Errorf("single Dedupe: removed=%d len=%d", removed, len(g.Triples))
	}
}

// TestObjectsAreDistinct is the property that protects callers: a repeated
// statement must not make a property look like it has repeated values. libcat
// counts result lengths (`s.Items += len(g.Objects(inst, hasItem))`), so a
// duplicate leaking through here silently inflates a user-visible number.
func TestObjectsAreDistinct(t *testing.T) {
	g, err := ParseNTriples([]byte(dupDoc))
	if err != nil {
		t.Fatal(err)
	}
	got := g.Objects(NewIRI("http://a"), "http://p")
	if len(got) != 2 {
		t.Fatalf("Objects = %d terms, want 2 distinct", len(got))
	}
	if got[0].Value != "http://o" || got[1].Value != "http://o2" {
		t.Errorf("Objects lost document order: %v", got)
	}
}

// TestObjectsDistinctAcrossMapPromotion drives Objects past objectSetMapAbove so
// both the linear-scan and hash-set halves of objectSet are exercised, including
// the promotion boundary itself.
func TestObjectsDistinctAcrossMapPromotion(t *testing.T) {
	for _, distinct := range []int{1, objectSetMapAbove - 1, objectSetMapAbove, objectSetMapAbove + 1, objectSetMapAbove * 3} {
		t.Run(strconv.Itoa(distinct), func(t *testing.T) {
			var b strings.Builder
			// Every object stated twice, so a correct result is `distinct` terms
			// in first-occurrence order.
			triple := func(i int) {
				b.WriteString("<http://a> <http://p> <http://o")
				b.WriteString(strconv.Itoa(i))
				b.WriteString("> .\n")
			}
			for i := range distinct {
				triple(i)
			}
			for i := range distinct {
				triple(i)
			}
			g, err := ParseNTriples([]byte(b.String()))
			if err != nil {
				t.Fatal(err)
			}
			got := g.Objects(NewIRI("http://a"), "http://p")
			if len(got) != distinct {
				t.Fatalf("Objects = %d terms, want %d distinct", len(got), distinct)
			}
			for i, o := range got {
				if want := "http://o" + strconv.Itoa(i); o.Value != want {
					t.Fatalf("Objects[%d] = %q, want %q", i, o.Value, want)
				}
			}
		})
	}
}

// TestObjectsWithRepeats is the escape hatch for a caller reading a
// serialization as written -- bibframe's positional bf:seriesEnumeration relies
// on it, since two 490s with the same $v encode to two identical triples.
func TestObjectsWithRepeats(t *testing.T) {
	g, err := ParseNTriples([]byte(dupDoc))
	if err != nil {
		t.Fatal(err)
	}
	got := g.ObjectsWithRepeats(NewIRI("http://a"), "http://p")
	if len(got) != 3 {
		t.Fatalf("ObjectsWithRepeats = %d terms, want 3 (statement for statement)", len(got))
	}
	if got[0] != got[1] || got[2].Value != "http://o2" {
		t.Errorf("ObjectsWithRepeats lost the repeat or the order: %v", got)
	}
	// And it must not disturb the deduplicating sibling.
	if n := len(g.Objects(NewIRI("http://a"), "http://p")); n != 2 {
		t.Errorf("Objects = %d terms, want 2", n)
	}
}

// TestGraphViewObjectsAreDistinct holds the Dataset-backed view to the same
// contract as Graph, since a caller behind either reads the same property.
func TestGraphViewObjectsAreDistinct(t *testing.T) {
	nq := `<http://a> <http://p> <http://o> <http://g> .
<http://a> <http://p> <http://o> <http://g> .
<http://a> <http://p> <http://o2> <http://g> .
`
	d, err := ParseNQuads([]byte(nq))
	if err != nil {
		t.Fatal(err)
	}
	v := d.GraphView(NewIRI("http://g"))
	got := v.Objects(NewIRI("http://a"), "http://p")
	if len(got) != 2 {
		t.Fatalf("GraphView.Objects = %d terms, want 2 distinct", len(got))
	}
	if len(d.Quads) != 3 {
		t.Errorf("the Dataset itself must keep the document's %d quads, got %d", 3, len(d.Quads))
	}
}

// TestCanonicalCollapsesDuplicates records that canonical form already had set
// semantics, which is why Dedupe is not needed before comparing two graphs.
func TestCanonicalCollapsesDuplicates(t *testing.T) {
	g, err := ParseNTriples([]byte(dupDoc))
	if err != nil {
		t.Fatal(err)
	}
	c, err := g.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.TrimSpace(string(c)), "\n") + 1; n != 2 {
		t.Errorf("canonical form has %d lines, want 2:\n%s", n, c)
	}
}
