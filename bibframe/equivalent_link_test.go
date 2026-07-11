package bibframe

import (
	"sort"
	"testing"

	"github.com/freeeve/libcodex"
)

// values758 returns the sorted $1 values of every 758 field in the record,
// asserting each carries the blank indicators task 121 specifies.
func values758(t *testing.T, rec *codex.Record) []string {
	t.Helper()
	var got []string
	for _, f := range rec.Fields() {
		if f.Tag == "758" {
			if f.Ind1 != ' ' || f.Ind2 != ' ' {
				t.Errorf("758 indicators = %q/%q, want blank/blank", f.Ind1, f.Ind2)
			}
			got = append(got, f.SubfieldValue('1'))
		}
	}
	sort.Strings(got)
	return got
}

// decodeNT decodes an N-Triples document to exactly one record.
func decodeNT(t *testing.T, nt string) *codex.Record {
	t.Helper()
	recs, err := Decode([]byte(nt))
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	return recs[0]
}

const equivWorkHead = `<http://example.org/r#Work> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://id.loc.gov/ontologies/bibframe/Work> .
<http://example.org/r#Work> <http://id.loc.gov/ontologies/bibframe/hasInstance> <http://example.org/r#Instance> .
<http://example.org/r#Instance> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://id.loc.gov/ontologies/bibframe/Instance> .
<http://example.org/r#Work> <http://id.loc.gov/ontologies/bibframe/title> _:t .
_:t <http://id.loc.gov/ontologies/bibframe/mainTitle> "A work" .
`

// TestEquivalentLinkDecodesTo758 covers task 121: a bf:Work's owl:sameAs and
// bf:hasEquivalent identity links to external real-world-object URIs decode to MARC
// 758 Resource Identifier fields, one per distinct URI in $1 with blank indicators.
// The owl:sameAs shape is exactly what libcat emits from its enrichment (tasks/066).
func TestEquivalentLinkDecodesTo758(t *testing.T) {
	rec := decodeNT(t, equivWorkHead+
		`<http://example.org/r#Work> <http://www.w3.org/2002/07/owl#sameAs> <https://openlibrary.org/works/OL45804W> .
<http://example.org/r#Work> <http://id.loc.gov/ontologies/bibframe/hasEquivalent> <http://id.loc.gov/resources/hubs/abc> .
`)
	got := values758(t, rec)
	want := []string{"http://id.loc.gov/resources/hubs/abc", "https://openlibrary.org/works/OL45804W"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("758 $1 values = %v, want %v", got, want)
	}
}

// TestEquivalentLinkDedupesSharedURI checks that the same URI reached by both
// owl:sameAs and bf:hasEquivalent yields a single 758, not a duplicate pair.
func TestEquivalentLinkDedupesSharedURI(t *testing.T) {
	rec := decodeNT(t, equivWorkHead+
		`<http://example.org/r#Work> <http://www.w3.org/2002/07/owl#sameAs> <https://openlibrary.org/works/OL1W> .
<http://example.org/r#Work> <http://id.loc.gov/ontologies/bibframe/hasEquivalent> <https://openlibrary.org/works/OL1W> .
`)
	if got := values758(t, rec); len(got) != 1 || got[0] != "https://openlibrary.org/works/OL1W" {
		t.Errorf("758 $1 values = %v, want one OL1W", got)
	}
}

// TestEquivalentLinkSkipsNonURIAndAbsent checks that a literal identity object
// produces no 758 (the object is always a resolvable RWO URI), and that a Work with
// no identity links produces none.
func TestEquivalentLinkSkipsNonURIAndAbsent(t *testing.T) {
	rec := decodeNT(t, equivWorkHead+
		`<http://example.org/r#Work> <http://id.loc.gov/ontologies/bibframe/hasEquivalent> "not a uri" .
`)
	if got := values758(t, rec); len(got) != 0 {
		t.Errorf("literal object produced 758 %v, want none", got)
	}
	if got := values758(t, decodeNT(t, equivWorkHead)); len(got) != 0 {
		t.Errorf("work with no identity links produced 758 %v, want none", got)
	}
}
