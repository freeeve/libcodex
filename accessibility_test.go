package codex

import (
	"slices"
	"testing"
)

func TestAccessibilityEmpty(t *testing.T) {
	a := NewRecord().AddField(NewControlField("008", "920219s1993    nyu                   eng  ")).Accessibility()
	if !a.Empty() {
		t.Errorf("expected empty accessibility, got %+v", a)
	}
}

func TestAccessibilityLargePrintAndNotes(t *testing.T) {
	raw := []byte("920219s1993    nyu                   eng  ")
	raw[23] = 'd' // large print
	rec := NewRecord().
		SetLeader(Leader("00000cam a2200000 a 4500")).
		AddField(NewControlField("008", string(raw))).
		AddField(NewDataField("341", '0', ' ', NewSubfield('a', "textual"), NewSubfield('b', "large print"))).
		AddField(NewDataField("532", '1', ' ', NewSubfield('a', "Text resized to 18 point."))).
		AddField(NewDataField("532", '1', ' ', NewSubfield('a', "High-contrast layout.")))
	a := rec.Accessibility()
	if a.Empty() {
		t.Fatal("expected non-empty accessibility")
	}
	if !a.LargePrint || a.Braille {
		t.Errorf("LargePrint/Braille = %v/%v", a.LargePrint, a.Braille)
	}
	if !slices.Contains(a.AccessModes, "textual") {
		t.Errorf("AccessModes = %v", a.AccessModes)
	}
	if !slices.Contains(a.Features, "large print") {
		t.Errorf("Features = %v", a.Features)
	}
	if len(a.Notes) != 2 {
		t.Errorf("Notes = %v", a.Notes)
	}
}

func TestAccessibilityBrailleAndTactile(t *testing.T) {
	raw := []byte("920219s1993    nyu                   eng  ")
	raw[23] = 'f' // braille
	rec := NewRecord().
		SetLeader(Leader("00000cam a2200000 a 4500")).
		AddField(NewControlField("008", string(raw))).
		AddField(NewControlField("007", "fb")) // tactile material, braille
	a := rec.Accessibility()
	if !a.Braille {
		t.Error("expected Braille from 008 form of item")
	}
	if !a.Tactile {
		t.Error("expected Tactile from 007 category 'f'")
	}
}
