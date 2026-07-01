package codex

import "testing"

// linkedRecord pairs a romanized 245 with its original-script 880, the standard
// MARC vernacular-linkage arrangement.
func linkedRecord() *Record {
	return NewRecord().
		SetLeader(Leader("00000cam a2200000 a 4500")).
		AddField(NewDataField("100", '1', ' ',
			NewSubfield('6', "880-01"),
			NewSubfield('a', "Tolstoy, Leo,"))).
		AddField(NewDataField("245", '1', '0',
			NewSubfield('6', "880-02"),
			NewSubfield('a', "Voina i mir"))).
		AddField(NewDataField("880", '1', ' ',
			NewSubfield('6', "100-01/(N"),
			NewSubfield('a', "Толстой, Лев,"))).
		AddField(NewDataField("880", '1', '0',
			NewSubfield('6', "245-02/(N"),
			NewSubfield('a', "Война и мир")))
}

func TestLinkParse(t *testing.T) {
	f := NewDataField("245", '1', '0', NewSubfield('6', "880-02/(N/r"), NewSubfield('a', "x"))
	l, ok := f.Link()
	if !ok {
		t.Fatal("Link not parsed")
	}
	if l.Tag != "880" || l.Occurrence != "02" {
		t.Errorf("tag/occ = %q/%q", l.Tag, l.Occurrence)
	}
	if l.Script != "(N" || l.ScriptName() != "Cyrillic" {
		t.Errorf("script = %q (%s)", l.Script, l.ScriptName())
	}
	if !l.RightToLeft {
		t.Error("expected right-to-left orientation")
	}
	if !l.Linked() {
		t.Error("expected Linked")
	}

	// No $6, malformed $6, and an explicit unlinked occurrence.
	if _, ok := NewDataField("245", ' ', ' ', NewSubfield('a', "x")).Link(); ok {
		t.Error("expected no link without $6")
	}
	if _, ok := NewDataField("245", ' ', ' ', NewSubfield('6', "bad")).Link(); ok {
		t.Error("expected no link for malformed $6")
	}
	l, _ = NewDataField("245", ' ', ' ', NewSubfield('6', "880-00")).Link()
	if l.Linked() {
		t.Error("occurrence 00 must not count as linked")
	}
}

// TestLinkStrictOccurrence rejects references whose occurrence is the wrong
// length or non-numeric, which the old len < 6 check silently accepted (and
// truncated).
func TestLinkStrictOccurrence(t *testing.T) {
	for _, bad := range []string{"880-012", "880-xy", "880-0", "88-01", "8800-01", "880/01"} {
		if _, ok := NewDataField("245", '1', '0', NewSubfield('6', bad)).Link(); ok {
			t.Errorf("Link(%q) = ok, want rejected", bad)
		}
	}
	// A valid reference with trailing script/orientation segments still parses.
	l, ok := NewDataField("245", '1', '0', NewSubfield('6', "880-01/(N/r")).Link()
	if !ok || l.Occurrence != "01" || l.Script != "(N" || !l.RightToLeft {
		t.Errorf("Link of valid reference = %+v (ok=%v)", l, ok)
	}
}

// FuzzLink checks $6 parsing never panics and only accepts well-formed
// references.
func FuzzLink(f *testing.F) {
	for _, s := range []string{"880-01", "880-01/(N/r", "bad", "880-012", "245-99/$1"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		l, ok := NewDataField("245", '1', '0', NewSubfield('6', s)).Link()
		if !ok {
			return
		}
		if len(l.Tag) != 3 || len(l.Occurrence) != 2 {
			t.Errorf("Link(%q) accepted with tag=%q occ=%q", s, l.Tag, l.Occurrence)
		}
		if !isDigit(l.Occurrence[0]) || !isDigit(l.Occurrence[1]) {
			t.Errorf("Link(%q) accepted non-digit occurrence %q", s, l.Occurrence)
		}
	})
}

func TestScriptNames(t *testing.T) {
	for code, want := range map[string]string{
		"(3": "Arabic", "(B": "Latin", "$1": "CJK",
		"(N": "Cyrillic", "(S": "Greek", "(2": "Hebrew", "(9": "",
	} {
		if got := (Linkage{Script: code}).ScriptName(); got != want {
			t.Errorf("ScriptName(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestAlternateGraphicNoPartner(t *testing.T) {
	// A regular field linked to an occurrence with no matching 880 partner.
	rec := NewRecord().
		AddField(NewDataField("245", '1', '0', NewSubfield('6', "880-07"), NewSubfield('a', "x"))).
		AddField(NewDataField("880", '1', '0', NewSubfield('6', "245-09"), NewSubfield('a', "y")))
	if _, ok := rec.AlternateGraphic(rec.Fields()[0]); ok {
		t.Error("expected no partner for a non-matching occurrence")
	}
	// An 880 whose $6 occurrence matches but tags don't line up.
	if _, ok := rec.AlternateGraphic(rec.Fields()[1]); ok {
		t.Error("expected no partner when the back-reference tag is absent")
	}
}

func TestAlternateGraphic(t *testing.T) {
	rec := linkedRecord()
	fields := rec.Fields()

	// From the regular 245 to its 880 original-script partner.
	f245 := fields[1]
	alt, ok := rec.AlternateGraphic(f245)
	if !ok || alt.Tag != "880" || alt.SubfieldValue('a') != "Война и мир" {
		t.Errorf("245 -> 880 = %+v (ok=%v)", alt, ok)
	}

	// And back from the 880 to the regular field.
	f880 := fields[3]
	back, ok := rec.AlternateGraphic(f880)
	if !ok || back.Tag != "245" || back.SubfieldValue('a') != "Voina i mir" {
		t.Errorf("880 -> 245 = %+v (ok=%v)", back, ok)
	}

	// A field without linkage has no alternate.
	if _, ok := rec.AlternateGraphic(NewDataField("250", ' ', ' ', NewSubfield('a', "x"))); ok {
		t.Error("unlinked field should have no alternate")
	}
}

func TestVernacular(t *testing.T) {
	rec := linkedRecord()
	if got := rec.Vernacular("245", 'a'); got != "Война и мир" {
		t.Errorf("Vernacular(245,a) = %q", got)
	}
	if got := rec.Vernacular("100", 'a'); got != "Толстой, Лев," {
		t.Errorf("Vernacular(100,a) = %q", got)
	}
	if got := rec.Vernacular("999", 'a'); got != "" {
		t.Errorf("Vernacular of absent tag = %q", got)
	}
}
