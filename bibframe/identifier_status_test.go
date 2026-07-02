package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// findIdentifier returns the first identifier with the given value, or nil.
func findIdentifier(g *BIBFRAME, value string) *Identifier {
	for i := range g.Instance.Identifiers {
		if g.Instance.Identifiers[i].Value == value {
			return &g.Instance.Identifiers[i]
		}
	}
	return nil
}

// TestIdentifierStatusFromRecord covers canceled/invalid identifier numbers being
// kept with a bf:status rather than dropped (task 063): 020/024 $z -> cancinv,
// 022 $y -> incorrect, 022 $z -> cancinv.
func TestIdentifierStatusFromRecord(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "9780000000001"),
			codex.NewSubfield('z', "9780000000002"))).
		AddField(codex.NewDataField("022", ' ', ' ', codex.NewSubfield('a', "1111-1111"),
			codex.NewSubfield('y', "2222-2222"), codex.NewSubfield('z', "3333-3333")))
	g := FromRecord(rec)

	for value, want := range map[string]string{
		"9780000000001": "",
		"9780000000002": statusCancInv,
		"1111-1111":     "",
		"2222-2222":     statusIncorrect,
		"3333-3333":     statusCancInv,
	} {
		id := findIdentifier(g, value)
		if id == nil {
			t.Fatalf("identifier %q not found in %+v", value, g.Instance.Identifiers)
		}
		if id.Status != want {
			t.Errorf("identifier %q status = %q, want %q", value, id.Status, want)
		}
	}
}

// TestIdentifierStatusRoundTrip confirms a canceled ISBN and an incorrect ISSN
// survive Encode -> Decode, coming back in $z / $y with the status preserved.
func TestIdentifierStatusRoundTrip(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('z', "9780000000002"))).
		AddField(codex.NewDataField("022", ' ', ' ', codex.NewSubfield('y', "2222-2222")))

	encoded, err := Encode(rec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	if f := firstField(recs[0], "020"); f == nil || f.SubfieldValue('z') != "9780000000002" {
		t.Errorf("020 $z after round-trip = %v, want canceled ISBN in $z", f)
	}
	if f := firstField(recs[0], "022"); f == nil || f.SubfieldValue('y') != "2222-2222" {
		t.Errorf("022 $y after round-trip = %v, want incorrect ISSN in $y", f)
	}
}
