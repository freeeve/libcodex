package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// TestSubjectAuthorityFromRecord covers reading a 6xx $0 authority link into
// Subject.Authority, taking only a URI-shaped $0 and ignoring a record-control
// $0 (task 089).
func TestSubjectAuthorityFromRecord(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Cats"),
			codex.NewSubfield('0', "http://id.loc.gov/authorities/subjects/sh85021262")),
		codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Dogs"),
			codex.NewSubfield('0', "(DLC)sh85038796")),
	)
	g := FromRecord(rec)
	if s := findSubject(g, "Cats"); s == nil || s.Authority != "http://id.loc.gov/authorities/subjects/sh85021262" {
		t.Errorf("Cats authority not read from URI $0; got %+v", s)
	}
	if s := findSubject(g, "Dogs"); s == nil || s.Authority != "" {
		t.Errorf("Dogs authority should be empty for a non-URI $0; got %+v", s)
	}
}

// TestSubjectAuthorityRoundTrip confirms a 6xx $0 IRI survives Encode -> Decode:
// the subject node becomes an IRI and the reverse crosswalk re-emits $0.
func TestSubjectAuthorityRoundTrip(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("650", ' ', '7', codex.NewSubfield('a', "Lesbians"),
			codex.NewSubfield('2', "homosaurus"),
			codex.NewSubfield('0', "http://homosaurus.org/v3/homoit0000670")),
	)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	f := firstField(recs[0], "650")
	if f == nil {
		t.Fatal("650 missing after round-trip")
	}
	if got := f.SubfieldValue('a'); got != "Lesbians" {
		t.Errorf("650 $a = %q, want Lesbians", got)
	}
	if got := f.SubfieldValue('2'); got != "homosaurus" {
		t.Errorf("650 $2 = %q, want homosaurus", got)
	}
	if got := f.SubfieldValue('0'); got != "http://homosaurus.org/v3/homoit0000670" {
		t.Errorf("650 $0 = %q, want the homosaurus IRI", got)
	}
	if f.Ind2 != '7' {
		t.Errorf("650 ind2 = %c, want 7", f.Ind2)
	}
}

// TestSubjectSKOSNative drives the reverse crosswalk directly with a SKOS-shaped
// subject: an IRI object carrying only skos:prefLabel, no rdfs:label, no rdf:type
// and no bf:source. It must yield a 650 with the label in $a, the authority in
// $0, and the thesaurus derived from the IRI prefix (task 089).
func TestSubjectSKOSNative(t *testing.T) {
	g := &rdf.Graph{}
	work := rdf.NewIRI("http://example.org/w")
	iri := "http://id.loc.gov/authorities/subjects/sh85021262"
	subj := rdf.NewIRI(iri)
	g.Add(work, rdf.NewIRI(pSubject), subj)
	g.Add(subj, rdf.NewIRI(pPrefLabel), rdf.NewLiteral("Cats", "", ""))

	fields := subjectFields(g, work)
	if len(fields) != 1 {
		t.Fatalf("subjectFields = %d fields, want 1: %+v", len(fields), fields)
	}
	f := fields[0]
	if f.Tag != "650" {
		t.Errorf("tag = %s, want 650 (untyped SKOS concept defaults to Topic)", f.Tag)
	}
	if f.Ind2 != '0' {
		t.Errorf("ind2 = %c, want 0 (lcsh derived from the IRI prefix)", f.Ind2)
	}
	if got := f.SubfieldValue('a'); got != "Cats" {
		t.Errorf("$a = %q, want Cats (from skos:prefLabel)", got)
	}
	if got := f.SubfieldValue('0'); got != iri {
		t.Errorf("$0 = %q, want %q", got, iri)
	}
}

// TestSourceFromIRI spot-checks the well-known authority-prefix table.
func TestSourceFromIRI(t *testing.T) {
	cases := map[string]string{
		"http://id.loc.gov/authorities/subjects/sh1": "lcsh",
		"http://id.worldcat.org/fast/1234":           "fast",
		"http://homosaurus.org/v3/homoit0000670":     "homosaurus",
		"http://example.org/unknown/1":               "",
	}
	for iri, want := range cases {
		if got := sourceFromIRI(iri); got != want {
			t.Errorf("sourceFromIRI(%q) = %q, want %q", iri, got, want)
		}
	}
}
