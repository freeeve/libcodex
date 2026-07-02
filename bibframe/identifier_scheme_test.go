package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// TestIdentifier024Scheme covers 024 first-indicator / $2 scheme typing (task 064):
// ind1 0-4 name standard schemes, ind1='7' resolves the class from $2, and an
// unrecognized $2 scheme is kept as a generic identifier carrying the $2 source.
func TestIdentifier024Scheme(t *testing.T) {
	cases := []struct {
		ind1      byte
		sub2      string
		wantClass string
		wantSrc   string
	}{
		{'0', "", "Isrc", ""},
		{'1', "", "Upc", ""},
		{'2', "", "Ismn", ""},
		{'3', "", "Ean", ""},
		{'7', "doi", "Doi", ""},
		{'7', "isni", "Isni", ""},
		{'7', "gtin-14", "Gtin14Number", ""},
		{'7', "weird", "Identifier", "weird"}, // unknown scheme -> generic + source
		{'8', "", "Identifier", ""},
	}
	for _, tc := range cases {
		subs := []codex.Subfield{codex.NewSubfield('a', "VAL")}
		if tc.sub2 != "" {
			subs = append(subs, codex.NewSubfield('2', tc.sub2))
		}
		rec := codex.NewRecord().
			SetLeader(codex.Leader("00000nam a2200000 a 4500")).
			AddField(codex.NewControlField("001", "x")).
			AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
			AddField(codex.NewDataField("024", tc.ind1, ' ', subs...))
		id := findIdentifier(FromRecord(rec), "VAL")
		if id == nil {
			t.Fatalf("024 ind1=%c $2=%q produced no identifier", tc.ind1, tc.sub2)
		}
		if id.Class != tc.wantClass || id.Source != tc.wantSrc {
			t.Errorf("024 ind1=%c $2=%q -> class %q src %q, want class %q src %q",
				tc.ind1, tc.sub2, id.Class, id.Source, tc.wantClass, tc.wantSrc)
		}
	}
}

// TestIdentifier024RoundTrip confirms the scheme survives Encode -> Decode back
// into the correct 024 indicator / $2.
func TestIdentifier024RoundTrip(t *testing.T) {
	cases := []struct {
		ind1 byte
		sub2 string
	}{
		{'1', ""},     // Upc
		{'3', ""},     // Ean
		{'7', "doi"},  // Doi via $2
		{'7', "istc"}, // Istc via $2
	}
	for _, tc := range cases {
		subs := []codex.Subfield{codex.NewSubfield('a', "VAL")}
		if tc.sub2 != "" {
			subs = append(subs, codex.NewSubfield('2', tc.sub2))
		}
		rec := codex.NewRecord().
			SetLeader(codex.Leader("00000nam a2200000 a 4500")).
			AddField(codex.NewControlField("001", "x")).
			AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
			AddField(codex.NewDataField("024", tc.ind1, ' ', subs...))

		encoded, _ := Encode(rec)
		recs, err := Decode(encoded)
		if err != nil || len(recs) != 1 {
			t.Fatalf("Decode: %v (%d records)", err, len(recs))
		}
		f := firstField(recs[0], "024")
		if f == nil {
			t.Fatalf("024 missing after round-trip (ind1=%c $2=%q)", tc.ind1, tc.sub2)
		}
		if f.Ind1 != tc.ind1 || f.SubfieldValue('2') != tc.sub2 {
			t.Errorf("024 round-trip ind1=%c $2=%q, want ind1=%c $2=%q",
				f.Ind1, f.SubfieldValue('2'), tc.ind1, tc.sub2)
		}
	}
}

// TestLccnForward covers the forward 010 -> bf:Lccn producer (task 064), including
// a canceled LCCN ($z) round-tripping through bf:status.
func TestLccnForward(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
		AddField(codex.NewDataField("010", ' ', ' ', codex.NewSubfield('a', "  92005291 "),
			codex.NewSubfield('z', "99001234")))
	g := FromRecord(rec)
	if id := findIdentifier(g, "92005291"); id == nil || id.Class != "Lccn" || id.Status != "" {
		t.Errorf("010 $a not a valid bf:Lccn; got %+v", g.Instance.Identifiers)
	}
	if id := findIdentifier(g, "99001234"); id == nil || id.Class != "Lccn" || id.Status != statusCancInv {
		t.Errorf("010 $z not a canceled bf:Lccn; got %+v", g.Instance.Identifiers)
	}

	encoded, _ := Encode(rec)
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	if f := firstField(recs[0], "010"); f == nil || f.SubfieldValue('a') != "92005291" {
		t.Errorf("010 $a not reconstructed; got %v", f)
	}
}
