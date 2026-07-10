package bibframe

import (
	"reflect"
	"testing"

	"github.com/freeeve/libcodex"
)

// TestLinkingEntryForward covers 76x-78x linking entries -> bf:relation: the
// marc2bibframe2 relationship term from the tag and (for 780/785) the second
// indicator, plus the linked resource's title, creator and ISSN. The terms are
// LC's own (ConvSpec-760-788-Links.xsl), which resolve at id.loc.gov, and the
// source field rides along in MARCKey (tasks 073, 116).
func TestLinkingEntryForward(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("780", '0', '0', codex.NewSubfield('t', "Old title"),
			codex.NewSubfield('x', "1111-2222")), // continuationof
		codex.NewDataField("785", '0', '2', codex.NewSubfield('t', "New title"),
			codex.NewSubfield('a', "Publisher")), // succeededby
		codex.NewDataField("773", '0', ' ', codex.NewSubfield('t', "Host journal")),  // partof
		codex.NewDataField("776", '0', ' ', codex.NewSubfield('t', "Print version")), // otherphysicalformat
	))
	if len(g.Work.Relations) != 4 {
		t.Fatalf("relations = %+v, want 4", g.Work.Relations)
	}
	byCode := map[string]Relation{}
	for _, r := range g.Work.Relations {
		byCode[r.Relationship] = r
	}
	if r, ok := byCode["continuationof"]; !ok || r.Title != "Old title" || r.ISSN != "1111-2222" {
		t.Errorf("continuationof relation = %+v", r)
	}
	if r, ok := byCode["continuationof"]; !ok || r.MARCKey != "78000$tOld title$x1111-2222" {
		t.Errorf("continuationof MARCKey = %q", r.MARCKey)
	}
	if r, ok := byCode["succeededby"]; !ok || r.Title != "New title" || r.Name != "Publisher" {
		t.Errorf("succeededby relation = %+v", r)
	}
	if _, ok := byCode["partof"]; !ok {
		t.Errorf("missing partof (773); got %+v", g.Work.Relations)
	}
	if _, ok := byCode["otherphysicalformat"]; !ok {
		t.Errorf("missing otherphysicalformat (776); got %+v", g.Work.Relations)
	}
}

// TestLinkingEntryRelationshipTerms pins each (tag, ind2) to the marc2bibframe2
// term it must emit, including the collapses -- 780 ind2 5 and 6 both onto
// absorptionof, 785 ind2 0 and 8 both onto continuedby -- which is exactly why the
// marcKey note is needed to round-trip the indicator.
func TestLinkingEntryRelationshipTerms(t *testing.T) {
	cases := []struct {
		tag  string
		ind2 byte
		want string
	}{
		{"773", ' ', "partof"},
		{"776", ' ', "otherphysicalformat"},
		{"780", '0', "continuationof"},
		{"780", '1', "continuedinpart"},
		{"780", '4', "mergerof"},
		{"780", '5', "absorptionof"},
		{"780", '6', "absorptionof"}, // collapses with 5
		{"780", '7', "separatedfrom"},
		{"780", '2', "precededby"},
		{"780", '8', "precededby"}, // the otherwise branch
		{"785", '0', "continuedby"},
		{"785", '8', "continuedby"}, // collapses with 0
		{"785", '1', "continuedinpartby"},
		{"785", '4', "absorbedby"},
		{"785", '5', "absorbedby"}, // collapses with 4
		{"785", '6', "splitinto"},
		{"785", '7', "mergedtoform"},
		{"785", '3', "succeededby"}, // the otherwise branch
	}
	for _, tc := range cases {
		got, ok := relationCodeFor(tc.tag, tc.ind2)
		if !ok || got != tc.want {
			t.Errorf("relationCodeFor(%s, %c) = %q, %v; want %q", tc.tag, tc.ind2, got, ok, tc.want)
		}
	}
}

// TestLinkingEntryLosslessThroughNote is the payoff of task 116: a field whose
// second indicator collapses onto a shared term, and whose non-access-point
// subfield the flat model dropped, both survive a full round trip because the
// marcKey note carries the field verbatim. Two 780s that differ only in ind2 (5
// vs 6) share the absorptionof term yet must come back distinct.
func TestLinkingEntryLosslessThroughNote(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("780", '0', '5', codex.NewSubfield('t', "Absorbed A"),
			codex.NewSubfield('g', "v.1")), // absorbed
		codex.NewDataField("780", '0', '6', codex.NewSubfield('t', "Absorbed B"),
			codex.NewSubfield('g', "v.2")), // absorbed in part -- same term
	)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	var got [][3]string
	for _, f := range recs[0].Fields() {
		if f.Tag == "780" {
			got = append(got, [3]string{string(f.Ind2), f.SubfieldValue('t'), f.SubfieldValue('g')})
		}
	}
	want := [][3]string{{"5", "Absorbed A", "v.1"}, {"6", "Absorbed B", "v.2"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("780s round-tripped as %v, want %v (ind2 and $g must survive the shared term)", got, want)
	}
}

// TestLinkingEntryDecodesThirdPartyGraph checks the note-absent fallback: a graph
// carrying the relationship term and an associated resource but no marcKey note
// still yields a linking field, at the term's canonical indicator.
func TestLinkingEntryDecodesThirdPartyGraph(t *testing.T) {
	const nt = `<http://example.org/r#Work> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://id.loc.gov/ontologies/bibframe/Work> .
<http://example.org/r#Work> <http://id.loc.gov/ontologies/bibframe/hasInstance> <http://example.org/r#Instance> .
<http://example.org/r#Instance> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://id.loc.gov/ontologies/bibframe/Instance> .
<http://example.org/r#Work> <http://id.loc.gov/ontologies/bibframe/relation> _:rel .
_:rel <http://id.loc.gov/ontologies/bibframe/relationship> <http://id.loc.gov/vocabulary/relationship/otherphysicalformat> .
_:rel <http://id.loc.gov/ontologies/bibframe/associatedResource> _:res .
_:res <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://id.loc.gov/ontologies/bibframe/Work> .
_:res <http://id.loc.gov/ontologies/bibframe/title> _:tit .
_:tit <http://id.loc.gov/ontologies/bibframe/mainTitle> "Online edition" .
`
	recs, err := Decode([]byte(nt))
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	f := firstField(recs[0], "776")
	if f == nil || f.SubfieldValue('t') != "Online edition" {
		t.Errorf("776 from a note-less third-party graph = %+v", f)
	}
}

// TestLinkingEntryRoundTrip encodes a record carrying preceding/succeeding/host/
// other-format links and decodes it, asserting each returns to its 76x-78x tag with
// the relationship-bearing second indicator and access-point subfields intact, and
// that no linked resource surfaces as its own record (task 073).
func TestLinkingEntryRoundTrip(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("780", '0', '2', codex.NewSubfield('a', "Prior Co."),
			codex.NewSubfield('t', "Predecessor"), codex.NewSubfield('x', "0000-1111")), // supersedes
		codex.NewDataField("785", '0', '0', codex.NewSubfield('t', "Successor")), // continuedBy
		codex.NewDataField("776", '0', ' ', codex.NewSubfield('t', "Online edition"),
			codex.NewSubfield('x', "2222-3333")), // otherPhysicalFormat
	)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("decoded %d records, want 1 (linked resources must not be their own records)", len(recs))
	}
	got := recs[0]

	if f := firstField(got, "780"); f == nil || f.Ind2 != '2' ||
		f.SubfieldValue('a') != "Prior Co." || f.SubfieldValue('t') != "Predecessor" ||
		f.SubfieldValue('x') != "0000-1111" {
		t.Errorf("780 not reconstructed; got %+v", f)
	}
	if f := firstField(got, "785"); f == nil || f.Ind2 != '0' || f.SubfieldValue('t') != "Successor" {
		t.Errorf("785 not reconstructed; got %+v", f)
	}
	if f := firstField(got, "776"); f == nil || f.SubfieldValue('t') != "Online edition" ||
		f.SubfieldValue('x') != "2222-3333" {
		t.Errorf("776 not reconstructed; got %+v", f)
	}
}
