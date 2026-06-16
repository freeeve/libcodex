package codex

import "testing"

func TestControl008Common(t *testing.T) {
	// A typical book 008: date entered 920219, type 's', date1 1993, place nyu,
	// language eng, form of item (pos 23) blank.
	rec := NewRecord().
		SetLeader(Leader("00000cam a2200000 a 4500")).
		AddField(NewControlField("008", "920219s1993    nyua   j      000 1 eng  "))
	c, ok := rec.Control008()
	if !ok {
		t.Fatal("Control008 not found")
	}
	if got := c.DateEntered(); got != "920219" {
		t.Errorf("DateEntered = %q", got)
	}
	if got := c.DateType(); got != 's' {
		t.Errorf("DateType = %q", got)
	}
	if got := c.Date1(); got != "1993" {
		t.Errorf("Date1 = %q", got)
	}
	if got := c.Place(); got != "nyu" {
		t.Errorf("Place = %q", got)
	}
	if got := c.Language(); got != "eng" {
		t.Errorf("Language = %q", got)
	}
}

func TestControl008FormOfItem(t *testing.T) {
	// Books carry form of item at position 23.
	mk := func(leader string, pos int, code byte) *Record {
		raw := []byte("920219s1993    nyu                   eng  ")
		raw[pos] = code
		return NewRecord().SetLeader(Leader(leader)).AddField(NewControlField("008", string(raw)))
	}
	book := mk("00000cam a2200000 a 4500", 23, 'd') // large print book
	c, _ := book.Control008()
	if !c.IsLargePrint() || c.IsBraille() {
		t.Errorf("expected large print book, form=%q", c.FormOfItem())
	}

	braille := mk("00000cam a2200000 a 4500", 23, 'f')
	c, _ = braille.Control008()
	if !c.IsBraille() {
		t.Errorf("expected braille, form=%q", c.FormOfItem())
	}

	// Visual materials (leader 06 = 'g') carry form of item at position 29.
	visual := mk("00000ngm a2200000 a 4500", 29, 'd')
	c, _ = visual.Control008()
	if !c.IsLargePrint() {
		t.Errorf("expected visual material form at pos 29, form=%q", c.FormOfItem())
	}
}

func TestControl008StringAndDate2(t *testing.T) {
	raw := "920219m19931995nyu                   eng  " // date type 'm', date2 1995
	c, ok := NewRecord().AddField(NewControlField("008", raw)).Control008()
	if !ok {
		t.Fatal("Control008 not found")
	}
	if c.String() != raw {
		t.Errorf("String() = %q", c.String())
	}
	if c.Date2() != "1995" {
		t.Errorf("Date2() = %q", c.Date2())
	}
	if c.DateType() != 'm' {
		t.Errorf("DateType() = %q", c.DateType())
	}
}

func TestControl008ShortOrMissing(t *testing.T) {
	// No 008 at all.
	if _, ok := NewRecord().Control008(); ok {
		t.Error("expected no Control008 without an 008 field")
	}
	// An 008 too short for the common header.
	if _, ok := NewRecord().AddField(NewControlField("008", "920219")).Control008(); ok {
		t.Error("expected no Control008 for a short 008")
	}
	// Present header but truncated before language/form: accessors degrade safely.
	c, ok := NewRecord().AddField(NewControlField("008", "920219s1993    nyu")).Control008()
	if !ok {
		t.Fatal("expected Control008 for an 18-char 008")
	}
	if c.Language() != "" || c.FormOfItem() != 0 || c.CatalogingSource() != 0 {
		t.Error("out-of-range positions should return empty/zero")
	}
}
