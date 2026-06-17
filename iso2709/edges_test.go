package iso2709

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// TestControlDataBoundary pins the tag "010" boundary between control and data
// fields: "010" is the lowest data-field tag and "009" the highest control tag.
func TestControlDataBoundary(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("009", "ctrl")).
		AddField(codex.NewDataField("010", ' ', ' ', codex.NewSubfield('a', "123")))
	b, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	// 010 must be a data field (indicators + subfields), not a control field.
	f, ok := got.DataField("010")
	if !ok {
		t.Fatal("tag 010 must parse as a data field")
	}
	if f.SubfieldValue('a') != "123" {
		t.Errorf("010 $a = %q, want 123", f.SubfieldValue('a'))
	}
	if got.ControlField("010") != "" {
		t.Error("tag 010 must not parse as a control field")
	}
	// 009 must remain a control field.
	if got.ControlField("009") != "ctrl" {
		t.Errorf("009 control value = %q, want ctrl", got.ControlField("009"))
	}
	if _, ok := got.DataField("009"); ok {
		t.Error("tag 009 must not parse as a data field")
	}
}

// TestDirectoryEntryAtBodyEnd exercises the start+length == len(body) boundary: a
// field whose data ends exactly at the field area must decode fully.
func TestDirectoryEntryAtBodyEnd(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Exactly at the end")))
	b, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if v := got.SubfieldValue("245", 'a'); v != "Exactly at the end" {
		t.Errorf("245$a = %q", v)
	}
}
