package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// TestSeriesEntryForward covers the 8xx series added entries and 760/762 series
// linking entries -> SeriesEntry: the heading name and its agent class from the
// tag, the title from $a (830) or $t (name-title), the subseries flag for 762, and
// the source field carried verbatim in MARCKey (task 113 piece 1).
func TestSeriesEntryForward(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("800", '1', ' ', codex.NewSubfield('a', "Tolkien, J.R.R."),
			codex.NewSubfield('t', "Middle-earth"), codex.NewSubfield('v', "3"),
			codex.NewSubfield('x', "1111-2222")),
		codex.NewDataField("810", '2', ' ', codex.NewSubfield('a', "Acme Corp."),
			codex.NewSubfield('t', "Reports")),
		codex.NewDataField("811", '2', ' ', codex.NewSubfield('a', "Symposium"),
			codex.NewSubfield('t', "Proceedings")),
		codex.NewDataField("830", ' ', '0', codex.NewSubfield('a', "Lecture notes in math"),
			codex.NewSubfield('v', "42")),
		codex.NewDataField("762", ' ', ' ', codex.NewSubfield('a', "Parent body"),
			codex.NewSubfield('t', "Subseries title")),
	))
	if len(g.Work.SeriesEntries) != 5 {
		t.Fatalf("series entries = %+v, want 5", g.Work.SeriesEntries)
	}
	byTitle := map[string]SeriesEntry{}
	for _, e := range g.Work.SeriesEntries {
		byTitle[e.Title] = e
	}
	if e := byTitle["Middle-earth"]; e.Name != "Tolkien, J.R.R." || e.NameClass != "Person" ||
		e.Enumeration != "3" || e.ISSN != "1111-2222" || e.Subseries {
		t.Errorf("800 entry = %+v", e)
	}
	if e := byTitle["Reports"]; e.NameClass != "Organization" || e.Name != "Acme Corp." {
		t.Errorf("810 entry = %+v", e)
	}
	if e := byTitle["Proceedings"]; e.NameClass != "Meeting" {
		t.Errorf("811 entry = %+v", e)
	}
	if e := byTitle["Lecture notes in math"]; e.Name != "" || e.NameClass != "" || e.Enumeration != "42" {
		t.Errorf("830 entry = %+v (uniform title carries no agent)", e)
	}
	if e := byTitle["Subseries title"]; !e.Subseries {
		t.Errorf("762 entry = %+v, want Subseries", e)
	}
}

// TestSeriesEntryDualTypeHubSeries pins the chosen node model: the associated
// resource of an 8xx relation is one node typed BOTH bf:Hub and bf:Series, off a
// bf:relation whose relationship is relationship/series, with the volume
// designation on the relation. This is the decision Eve took for task 113 piece 1.
func TestSeriesEntryDualTypeHubSeries(t *testing.T) {
	rec := recordWith(codex.NewDataField("830", ' ', '0',
		codex.NewSubfield('a', "Springer series"), codex.NewSubfield('v', "7"),
		codex.NewSubfield('x', "0000-1111")))
	graph, err := rdf.ParseNTriples(mustEncodeNT(t, rec))
	if err != nil {
		t.Fatal(err)
	}
	hubs := graph.SubjectsOfType(classHub)
	if len(hubs) != 1 {
		t.Fatalf("bf:Hub nodes = %d, want 1", len(hubs))
	}
	hub := hubs[0]
	if !graph.HasType(hub, classSeries) {
		t.Error("the associated resource must be typed both bf:Hub and bf:Series")
	}
	rels := graph.SubjectsOfType(classRelation)
	if len(rels) != 1 {
		t.Fatalf("bf:Relation nodes = %d, want 1", len(rels))
	}
	if code := relationshipCode(graph, rels[0]); code != seriesRelationship {
		t.Errorf("relationship = %q, want series", code)
	}
	if v := literal(graph, rels[0], pSeriesEnumeration); v != "7" {
		t.Errorf("seriesEnumeration = %q, want 7 (on the relation, not the Hub)", v)
	}
	if issn, _ := associatedIdentifiers(graph, hub); issn != "0000-1111" {
		t.Errorf("Hub ISSN = %q, want 0000-1111", issn)
	}
}

// TestSeriesEntryRoundTrip encodes each series-entry tag and decodes it, asserting
// the source field returns exactly -- including subfields the flat view never
// surfaces (personal-name dates $d, the record control number $w), which prove the
// marcKey carrier, not the BIBFRAME view, is what round-trips.
func TestSeriesEntryRoundTrip(t *testing.T) {
	fields := []codex.Field{
		codex.NewDataField("800", '1', ' ', codex.NewSubfield('a', "Asimov, Isaac"),
			codex.NewSubfield('d', "1920-1992"), codex.NewSubfield('t', "Foundation"),
			codex.NewSubfield('v', "2"), codex.NewSubfield('w', "(DLC)   58012345")),
		codex.NewDataField("810", '2', ' ', codex.NewSubfield('a', "United Nations"),
			codex.NewSubfield('t', "Treaty series"), codex.NewSubfield('x', "0379-8267")),
		codex.NewDataField("811", '2', ' ', codex.NewSubfield('a', "Olympiad"),
			codex.NewSubfield('t', "Records")),
		codex.NewDataField("830", ' ', '0', codex.NewSubfield('a', "Studies in logic"),
			codex.NewSubfield('v', "15"), codex.NewSubfield('x', "1234-5678")),
		codex.NewDataField("762", ' ', ' ', codex.NewSubfield('a', "Main society"),
			codex.NewSubfield('t', "Bulletin subseries")),
	}
	rec := recordWith(fields...)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	got := recs[0]
	for _, want := range fields {
		f := firstField(got, want.Tag)
		if f == nil {
			t.Errorf("%s missing after round trip", want.Tag)
			continue
		}
		if marcKeyOf(*f) != marcKeyOf(want) {
			t.Errorf("%s not reconstructed exactly:\n got %q\nwant %q", want.Tag, marcKeyOf(*f), marcKeyOf(want))
		}
	}
}

// TestSeriesEntryDiscriminatesFrom490 is the crux of task 113: a traced 490 and its
// 830 must stay two distinct relations that decode back to their own tags, not
// collapse or cross-contaminate. Emitting the 830 is also what closes the dangling
// mstatus/tr reference the traced 490 asserts.
func TestSeriesEntryDiscriminatesFrom490(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("490", '1', ' ', codex.NewSubfield('a', "Penguin classics"),
			codex.NewSubfield('v', "12")),
		codex.NewDataField("830", ' ', '0', codex.NewSubfield('a', "Penguin classics"),
			codex.NewSubfield('v', "12"), codex.NewSubfield('x', "9999-8888")),
	)
	// Forward: one transcribed-traced 490 and one 8xx entry; the graph carries both
	// a plain bf:Series (the 490) and a bf:Hub node (the 830).
	g := FromRecord(rec)
	if len(g.Work.Series) != 1 || !g.Work.Series[0].Traced {
		t.Fatalf("Series = %+v, want one traced", g.Work.Series)
	}
	if len(g.Work.SeriesEntries) != 1 {
		t.Fatalf("SeriesEntries = %+v, want one", g.Work.SeriesEntries)
	}
	graph, err := rdf.ParseNTriples(mustEncodeNT(t, rec))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(graph.SubjectsOfType(classHub)); n != 1 {
		t.Errorf("bf:Hub nodes = %d, want 1 (the 830 closes the traced-490 dangling reference)", n)
	}
	// Round trip: the 490 keeps ind1=1 and stays a 490; the 830 stays an 830.
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	f490 := firstField(recs[0], "490")
	if f490 == nil || f490.Ind1 != '1' || f490.SubfieldValue('a') != "Penguin classics" {
		t.Errorf("490 not reconstructed as traced; got %+v", f490)
	}
	f830 := firstField(recs[0], "830")
	if f830 == nil || f830.SubfieldValue('a') != "Penguin classics" || f830.SubfieldValue('x') != "9999-8888" {
		t.Errorf("830 not reconstructed; got %+v", f830)
	}
}

// TestSeriesEntryPropertiesFallback decodes a third-party graph that carries a
// bf:Hub/bf:Series relation with no marcKey note, asserting the approximate
// reconstruction: a bare-title series Hub becomes an 830, a subseries Hub a 762.
func TestSeriesEntryPropertiesFallback(t *testing.T) {
	nt := `<http://example.org/w> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <` + classWork + `> .
<http://example.org/w> <` + pRelation + `> _:r .
_:r <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <` + classRelation + `> .
_:r <` + pRelationship + `> <` + relationshipVocab + `series> .
_:r <` + pSeriesEnumeration + `> "9" .
_:r <` + pAssociatedResource + `> _:h .
_:h <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <` + classHub + `> .
_:h <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <` + classSeries + `> .
_:h <` + pTitle + `> _:t .
_:t <` + pMainTitle + `> "Third-party series" .
`
	recs, err := Decode([]byte(nt))
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	f := firstField(recs[0], "830")
	if f == nil || f.SubfieldValue('a') != "Third-party series" || f.SubfieldValue('v') != "9" {
		t.Errorf("830 not approximated from Hub properties; got %+v", f)
	}
}
