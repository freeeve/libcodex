package marc8

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"ascii", []byte("Plain ASCII"), "Plain ASCII"},
		// Combining diacritics precede their base byte in MARC-8; the decoder
		// reorders the mark after the base and composes to NFC.
		{"compose acute", []byte("Beyonc\xe2e"), "Beyoncé"},
		{"compose diaeresis", []byte("na\xe8ive"), "naïve"},
		{"ansel graphic", []byte{0xA5}, "Æ"},
		{"alif apostrophe", []byte{0xAE}, "ʼ"}, // LoC 2005 remapping from 02BE
		{"o with horn", []byte{0xBC}, "ơ"},     // assigned graphic, was missing
		{"u with horn", []byte{0xBD}, "ư"},
		{"compose macron", []byte{0xE5, 'o'}, "ō"},
		{"zero width joiner", []byte{0x8D}, "\u200d"},     // ANSEL control function
		{"zero width non-joiner", []byte{0x8E}, "\u200c"}, // ANSEL control function
		{"non-sort begin", []byte{0x88}, "\u0098"},        // ANSEL non-sort marker
		{"unknown high byte", []byte{0xFF}, "ÿ"},          // best-effort pass-through
		{"trailing mark", []byte{0xE2}, "́"},              // dangling combining acute
		{"skip escape", []byte{escape, '(', 'B', 'A'}, "A"},
		{"escape to ansel", []byte{escape, ')', 'E', 0xA5}, "Æ"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Decode(c.in); got != c.want {
				t.Errorf("Decode(% x) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestDecodeUnsupportedSet(t *testing.T) {
	// Designate an unsupported set to G1 (ESC ) 1), then a high byte must pass
	// through best-effort (Latin-1) rather than be ANSEL-decoded, without
	// crashing. 0xB5 is ANSEL 'æ' but Latin-1 'µ'.
	in := []byte{escape, ')', '1', 0xB5}
	if got := Decode(in); got != "µ" {
		t.Errorf("Decode = %q (% x), want best-effort pass-through 'µ'", got, got)
	}
}

func TestDecoderLossy(t *testing.T) {
	cases := []struct {
		name      string
		in        []byte
		wantLossy bool
	}{
		{"ascii", []byte("plain text"), false},
		{"ansel compose", []byte("Beyonc\xe2e"), false},
		{"ansel graphic", []byte{0xA5}, false},
		{"unsupported set", []byte{escape, ')', '1', 0xB5}, true},
		{"undefined high byte", []byte{0xFF}, true},
		{"eacc designation only", []byte{escape, '$', '1'}, false},
		{"eacc valid char", []byte{escape, '$', '1', 0x21, 0x30, 0x21}, false},     // U+4E00
		{"eacc unmapped triple", []byte{escape, '$', '1', 0x21, 0x21, 0x21}, true}, // not in table
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewDecoder()
			d.Decode(c.in)
			if d.Lossy() != c.wantLossy {
				t.Errorf("Lossy() = %v, want %v", d.Lossy(), c.wantLossy)
			}
		})
	}
}

func TestDecoderStatePersists(t *testing.T) {
	// G1 designated to ASCII in the first call must remain in effect for the
	// second, so a high byte passes through instead of being ANSEL-decoded.
	d := NewDecoder()
	if got := d.Decode([]byte{escape, ')', 'B'}); got != "" {
		t.Errorf("designation call emitted %q, want empty", got)
	}
	if got := d.Decode([]byte{0xB5}); got != "µ" {
		t.Errorf("after G1=ASCII, 0xB5 = %q, want µ (state did not persist)", got)
	}
}

func TestEncode(t *testing.T) {
	// Precomposed (NFC) strings in the supported subset round-trip exactly.
	for _, s := range []string{
		"",
		"Plain ASCII text 123",
		"Beyoncé", "naïve", "café", "Müller", "Łódź",
		"àáâãäåèéêëìíîïòóôõöùúûüç ñ",
		"æØŒ©®°±£€ß¿¡ðþ", // ANSEL graphics
		"Stone butch blues : a novel / Leslie Feinberg.",
	} {
		b, err := Encode(s)
		if err != nil {
			t.Fatalf("Encode(%q): %v", s, err)
		}
		if got := Decode(b); got != s {
			t.Errorf("round trip: Encode then Decode(%q) = %q (% x)", s, got, b)
		}
	}
}

func TestEncodeReordersCombining(t *testing.T) {
	// The combining mark must be emitted BEFORE its base (the reverse of Unicode).
	want := []byte{0xE2, 'e'} // ANSEL acute, then 'e'
	if b, err := Encode("é"); err != nil || !bytes.Equal(b, want) {
		t.Errorf("Encode(precomposed é) = % x, %v; want % x", b, err, want)
	}
	if b, err := Encode("é"); err != nil || !bytes.Equal(b, want) {
		t.Errorf("Encode(NFD é) = % x, %v; want % x", b, err, want)
	}
}

func TestEncodeRejectsUnrepresentable(t *testing.T) {
	// No MARC-8 set covers emoji or mathematical alphanumerics.
	for _, s := range []string{"😀", "🎉party", "𝔘niverse", "a\U0001F600b"} {
		if _, err := Encode(s); err == nil {
			t.Errorf("Encode(%q): expected error for an unrepresentable character", s)
		}
	}
}

// TestNonLatinRoundTrip exercises the newly supported scripts: every string must
// survive Encode then Decode unchanged, including switches back to Latin.
func TestNonLatinRoundTrip(t *testing.T) {
	for _, s := range []string{
		"Привет мир",               // Basic Cyrillic
		"ґ ђ ѓ",                    // Extended Cyrillic
		"ΑΒΓΔ αβγδ",                // Basic Greek letters
		"\u03b1\u0301\u03b2\u03b3", // accented Greek (alpha + combining tonos), NFD form
		"שלום",                     // Basic Hebrew
		"العربية",                  // Basic Arabic
		"日本語と中文",                   // EACC (CJK)
		"一二三四五",                    // EACC ideographs
		"H₂O",                      // subscript two
		"Title: Война и мир / Толстой.", // ASCII -> Cyrillic -> ASCII switching
		"café 日本 Привет",                // Latin + CJK + Cyrillic in one value
	} {
		b, err := Encode(s)
		if err != nil {
			t.Fatalf("Encode(%q): %v", s, err)
		}
		if got := Decode(b); got != s {
			t.Errorf("round trip Encode->Decode(%q) = %q (% x)", s, got, b)
		}
		// Output must be valid MARC-8: it must end back in the default sets so the
		// next value decodes correctly (no trailing non-default designation).
		if !utf8.ValidString(Decode(b)) {
			t.Errorf("decoded %q is not valid UTF-8", s)
		}
	}
}

// TestEncodeReturnsToDefault verifies a non-Latin value resets G0 to ASCII at the
// end, so a following ASCII value decodes correctly with the same field decoder.
func TestEncodeReturnsToDefault(t *testing.T) {
	cyr, err := Encode("Мир")
	if err != nil {
		t.Fatal(err)
	}
	ascii, err := Encode("ok")
	if err != nil {
		t.Fatal(err)
	}
	// Decode both through one decoder, as iso2709 does for subfields of a field.
	d := NewDecoder()
	if got := d.Decode(cyr); got != "Мир" {
		t.Errorf("first value = %q", got)
	}
	if got := d.Decode(ascii); got != "ok" {
		t.Errorf("second value = %q (designation leaked across values)", got)
	}
}

func TestEncodeRejectsStructuralBytes(t *testing.T) {
	// The escape introducer and the ISO 2709 separators cannot appear as data.
	for _, b := range []byte{0x1b, 0x1d, 0x1e, 0x1f} {
		if _, err := Encode("a" + string(b) + "b"); err == nil {
			t.Errorf("Encode with 0x%02X: expected error", b)
		}
	}
}

// TestEverySetDecodes designates each single-byte set and decodes one of its
// codes, covering every set's decode path (including those encode never selects).
func TestEverySetDecodes(t *testing.T) {
	for _, set := range []*charSet{
		csBasicHebrew, csBasicCyrillic, csExtCyrillic, csBasicArabic,
		csExtArabic, csBasicGreek, csGreekSymbols, csSubscripts, csSuperscripts,
	} {
		var b byte = 0xFF // lowest code in the set, for a deterministic pick
		for k := range set.dec {
			if k < b {
				b = k
			}
		}
		want := set.dec[b]
		inter := byte('(') // G0 sets use ESC ( final; G1 sets use ESC ) final
		if set.homeG1 {
			inter = ')'
		}
		got := []rune(Decode([]byte{escape, inter, set.final, b}))
		if len(got) == 0 || got[0] != want {
			t.Errorf("%s: decode 0x%02X = %q, want %q", set.name, b, string(got), string(want))
		}
	}
}

// TestReinstateASCII checks the ESC s technique-1 escape returns G0 to ASCII so
// following bytes decode cleanly rather than being flagged lossy.
func TestReinstateASCII(t *testing.T) {
	d := NewDecoder()
	if got := d.Decode([]byte{escape, 's', 'a', 'b', 'c'}); got != "abc" {
		t.Errorf("Decode(ESC s abc) = %q, want %q", got, "abc")
	}
	if d.Lossy() {
		t.Error("ESC s should reinstate ASCII, not flag the decode lossy")
	}
}

// TestMalformedEscapeKeepsData checks a malformed escape consumes only the escape
// byte, so the following data byte still decodes (and the decode is flagged lossy)
// rather than being silently dropped.
func TestMalformedEscapeKeepsData(t *testing.T) {
	d := NewDecoder()
	if got := d.Decode([]byte{escape, 0xA5}); got != "Æ" {
		t.Errorf("Decode(ESC 0xA5) = %q, want %q (data byte must not be dropped)", got, "Æ")
	}
	if !d.Lossy() {
		t.Error("a malformed escape should flag the decode lossy")
	}
}

func TestDecodeEscapeEdges(t *testing.T) {
	// None of these may panic; the lossy flag reports unusable input.
	cases := []struct {
		name      string
		in        []byte
		wantLossy bool
	}{
		{"escape at end", []byte{escape}, true},
		{"intermediate no final", []byte{escape, '('}, true},
		{"unknown final to G0", []byte{escape, '(', 'Z', 'a'}, true},
		{"unknown final to G1", []byte{escape, ')', 'Z', 0xB5}, true},
		{"truncated eacc triple", []byte{escape, '$', '1', 0x21}, true},
		{"eacc two bytes", []byte{escape, '$', '1', 0x21, 0x30}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewDecoder()
			if !utf8.ValidString(d.Decode(c.in)) {
				t.Error("decode produced invalid UTF-8")
			}
			if d.Lossy() != c.wantLossy {
				t.Errorf("Lossy() = %v, want %v", d.Lossy(), c.wantLossy)
			}
		})
	}
}

// FuzzEncode ensures encoding never panics and that re-encoding the decoded form
// is stable (Encode is the inverse of Decode on the canonical NFC form).
func FuzzEncode(f *testing.F) {
	f.Add("Beyoncé naïve café Łódź æØ")
	f.Add("plain ascii")
	f.Add("\u041f\u0440\u0438\u0432\u0435\u0442 \u65e5\u672c\u8a9e \u05e9\u05dc\u05d5\u05dd \u0627\u0644\u0639\u0631\u0628\u064a\u0629 \u0391\u0392\u0393")
	f.Add("\u03b1\u0301\u03b2 mixed \u4e00\u4e8c H\u2082O")
	f.Add("\u0314leading mark")
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return
		}
		b, err := Encode(s)
		if err != nil {
			return // out of the supported subset
		}
		canonical := Decode(b) // NFC form
		b2, err := Encode(canonical)
		if err != nil {
			t.Fatalf("re-encode of decoded form failed: %v", err)
		}
		if !bytes.Equal(b, b2) {
			t.Errorf("Encode not stable: % x vs % x", b, b2)
		}
		if got := Decode(b2); got != canonical {
			t.Errorf("Decode not stable: %q vs %q", got, canonical)
		}
	})
}

func BenchmarkDecode(b *testing.B) {
	in := []byte("Beyonc\xe2e na\xe8ive caf\xe2e \xe5o \xa5 \xc7 cocktail recipes for the modern bar")
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		_ = Decode(in)
	}
}
