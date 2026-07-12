package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// TestContributionAuthorityForward covers task 127: a 1xx/7xx agent's $1 (RWO URI)
// or $0 (authority-record URI) becomes Contribution.Authority, with $1 preferred,
// and a non-URI $0 record control number left out.
func TestContributionAuthorityForward(t *testing.T) {
	viaf := "http://viaf.org/viaf/12345"
	lcnaf := "http://id.loc.gov/authorities/names/n79021164"
	cases := []struct {
		name string
		subs []codex.Subfield
		want string
	}{
		{"rwo $1", []codex.Subfield{codex.NewSubfield('a', "A"), codex.NewSubfield('1', viaf)}, viaf},
		{"authority $0", []codex.Subfield{codex.NewSubfield('a', "A"), codex.NewSubfield('0', lcnaf)}, lcnaf},
		{"$1 preferred over $0", []codex.Subfield{codex.NewSubfield('a', "A"), codex.NewSubfield('0', lcnaf), codex.NewSubfield('1', viaf)}, viaf},
		{"record number ignored", []codex.Subfield{codex.NewSubfield('a', "A"), codex.NewSubfield('0', "(DLC)n79021164")}, ""},
	}
	for _, tc := range cases {
		g := FromRecord(recordWith(codex.NewDataField("100", '1', ' ', tc.subs...)))
		if len(g.Work.Contributions) != 1 {
			t.Fatalf("%s: contributions = %+v, want 1", tc.name, g.Work.Contributions)
		}
		if got := g.Work.Contributions[0].Authority; got != tc.want {
			t.Errorf("%s: Authority = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestContributionAuthorityRoundTrip confirms the agent identity IRI survives
// Encode -> Decode: it rides as the bf:agent node's own IRI and decodes back to $0
// (id.loc.gov/authorities) or $1 (any other RWO URI).
func TestContributionAuthorityRoundTrip(t *testing.T) {
	viaf := "http://viaf.org/viaf/99"
	lcnaf := "http://id.loc.gov/authorities/names/n123"
	rec := recordWith(
		codex.NewDataField("100", '1', ' ', codex.NewSubfield('a', "Primary, P"), codex.NewSubfield('1', viaf)),
		codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Added, A"), codex.NewSubfield('0', lcnaf)),
	)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	got := recs[0]
	if f := firstField(got, "100"); f == nil || f.SubfieldValue('1') != viaf {
		t.Errorf("100 $1 = %+v, want %q (RWO URI decodes to $1)", f, viaf)
	}
	if f := firstField(got, "700"); f == nil || f.SubfieldValue('0') != lcnaf {
		t.Errorf("700 $0 = %+v, want %q (id.loc.gov authority decodes to $0)", f, lcnaf)
	}
}

// TestContributionNoAuthorityStaysBlank checks that an agent without an IRI
// identifier still emits a blank node (no spurious $0/$1 on decode).
func TestContributionNoAuthorityStaysBlank(t *testing.T) {
	rec := recordWith(codex.NewDataField("100", '1', ' ', codex.NewSubfield('a', "Plain, P")))
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	f := firstField(recs[0], "100")
	if f == nil || f.SubfieldValue('0') != "" || f.SubfieldValue('1') != "" {
		t.Errorf("100 = %+v, want no $0/$1 for an agent with no authority", f)
	}
}
