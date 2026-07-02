package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// findISBN returns the first Isbn identifier on the Instance, or nil.
func findISBN(g *BIBFRAME) *Identifier {
	for i := range g.Instance.Identifiers {
		if g.Instance.Identifiers[i].Class == "Isbn" {
			return &g.Instance.Identifiers[i]
		}
	}
	return nil
}

// TestISBNQualifierFromRecord covers the forward crosswalk lifting an ISBN's
// qualifying information into bf:qualifier, both from a trailing parenthetical in
// 020 $a and from an explicit 020 $q, mirroring marc2bibframe2.
func TestISBNQualifierFromRecord(t *testing.T) {
	cases := []struct {
		name          string
		subfields     []codex.Subfield
		wantValue     string
		wantQualifier string
	}{
		{
			name:          "parenthetical in $a",
			subfields:     []codex.Subfield{codex.NewSubfield('a', "9781234567842 (electronic bk)")},
			wantValue:     "9781234567842",
			wantQualifier: "electronic bk",
		},
		{
			name:          "explicit $q",
			subfields:     []codex.Subfield{codex.NewSubfield('a', "0781234567"), codex.NewSubfield('q', "v.1")},
			wantValue:     "0781234567",
			wantQualifier: "v.1",
		},
		{
			name:          "$q wins over an empty parenthetical",
			subfields:     []codex.Subfield{codex.NewSubfield('a', "0781234567"), codex.NewSubfield('q', "paperback")},
			wantValue:     "0781234567",
			wantQualifier: "paperback",
		},
		{
			name:          "plain ISBN keeps no qualifier",
			subfields:     []codex.Subfield{codex.NewSubfield('a', "0786803525")},
			wantValue:     "0786803525",
			wantQualifier: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := codex.NewRecord().
				SetLeader(codex.Leader("00000nam a2200000 a 4500")).
				AddField(codex.NewControlField("001", "x")).
				AddField(codex.NewDataField("020", ' ', ' ', tc.subfields...)).
				AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))
			isbn := findISBN(FromRecord(rec))
			if isbn == nil {
				t.Fatalf("no Isbn identifier produced")
			}
			if isbn.Value != tc.wantValue {
				t.Errorf("value = %q, want %q", isbn.Value, tc.wantValue)
			}
			if isbn.Qualifier != tc.wantQualifier {
				t.Errorf("qualifier = %q, want %q", isbn.Qualifier, tc.wantQualifier)
			}
		})
	}
}

// TestSplitParenthetical pins the qualifier-stripping helper, including inputs
// that must be left untouched.
func TestSplitParenthetical(t *testing.T) {
	cases := []struct{ in, value, qualifier string }{
		{"9781234567842 (electronic bk)", "9781234567842", "electronic bk"},
		{"0781234567 (v. 1 ; paperback)", "0781234567", "v. 1 ; paperback"},
		{"0786803525", "0786803525", ""},
		{"broken (no close", "broken (no close", ""},
		{"no open)", "no open)", ""},
		{"(leading) 12345", "12345", "leading"},
	}
	for _, tc := range cases {
		value, qualifier := splitParenthetical(tc.in)
		if value != tc.value || qualifier != tc.qualifier {
			t.Errorf("splitParenthetical(%q) = (%q, %q), want (%q, %q)", tc.in, value, qualifier, tc.value, tc.qualifier)
		}
	}
}

// TestISBNQualifierRoundTrip confirms the qualifier survives Encode -> Decode:
// a parenthetical on 020 $a is emitted as bf:qualifier and comes back normalized
// into $q (the modern canonical subfield), with the bare number in $a.
func TestISBNQualifierRoundTrip(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "od99")).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "9781234567842 (electronic bk)"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "A Title")))

	encoded, err := Encode(rec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	f := firstField(recs[0], "020")
	if f == nil {
		t.Fatalf("020 missing after round-trip")
	}
	if got := f.SubfieldValue('a'); got != "9781234567842" {
		t.Errorf("020 $a = %q, want bare ISBN %q", got, "9781234567842")
	}
	if got := f.SubfieldValue('q'); got != "electronic bk" {
		t.Errorf("020 $q = %q, want %q", got, "electronic bk")
	}
}
