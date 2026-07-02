package iso5426

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

// TestDecode checks known ISO 5426 byte sequences against the reference mappings
// (cross-checked with the marc4j ISO 5426 converter).
func TestDecode(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"ascii", []byte("Hello"), "Hello"},
		{"acute e (compose)", []byte{0xC2, 0x65}, "é"},          // mark + base
		{"grave a (compose)", []byte{0xC1, 0x61}, "à"},          // à
		{"caron S", []byte{0xCF, 0x53}, "Š"},                    // U+0160
		{"diaeresis u", []byte{0xC8, 0x75}, "ü"},                // U+00FC
		{"L with stroke", []byte{0xE8}, "Ł"},                    // U+0141 (the letter block)
		{"o with stroke", []byte{0xE9}, "Ø"},                    // U+00D8
		{"OE ligature", []byte{0xEA}, "Œ"},                      // U+0152
		{"oe ligature", []byte{0xFA}, "œ"},                      // U+0153
		{"o slash (0xF9 as graphic)", []byte{0xF9}, "ø"},        // U+00F8, the ambiguous byte
		{"breve below (0xF9 as mark)", []byte{0xF9, 0x48}, "Ḫ"}, // U+1E2A, 0xF9 as a combining mark
		{"word", []byte{'M', 0xC8, 0x75, 'l', 'l', 'e', 'r'}, "Müller"},
		{"trailing mark", []byte{0xC2}, "́"}, // combining acute, no base
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Decode(c.in); got != c.want {
				t.Errorf("Decode(% x) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	for _, s := range []string{
		"", "Plain ASCII 123",
		"Müller", "café", "à la carte", "Łódź naïve",
		"Tłumaczenie", "Škoda Šárka", "Œuvre œuvre",
		"Edizione italiana: però perché città",
	} {
		b, err := Encode(s)
		if err != nil {
			t.Fatalf("Encode(%q): %v", s, err)
		}
		if got := Decode(b); got != s {
			t.Errorf("round trip Encode->Decode(%q) = %q (% x)", s, got, b)
		}
	}
}

func TestEncodeNFCAndNFDAgree(t *testing.T) {
	// Precomposed (NFC) and decomposed (NFD) forms of the same text must encode to
	// identical bytes, so the encoder is deterministic regardless of input form.
	nfc, err1 := Encode("é")  // U+00E9
	nfd, err2 := Encode("é") // e + combining acute
	if err1 != nil || err2 != nil {
		t.Fatalf("encode errors: %v %v", err1, err2)
	}
	if !bytes.Equal(nfc, nfd) {
		t.Errorf("NFC % x != NFD % x", nfc, nfd)
	}
}

// TestEncodeStackedMarks pins the order of two combining marks through an
// Encode->Decode round trip. ISO 5426 stores marks before the base innermost
// first; the mark nearest the base in Unicode order must stay innermost. The old
// decoder composed the base with the outermost mark, silently swapping the two
// diacritics (e.g. NFD ế decoding as é followed by a circumflex).
func TestEncodeStackedMarks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // Decode(Encode(in)); canonically equivalent to in
	}{
		// e + combining circumflex + combining acute -> ê + combining acute.
		{"e circumflex acute", "ế", "ế"},
		// A + combining diaeresis + combining grave -> Ä + combining grave.
		{"A diaeresis grave", "Ä̀", "Ä̀"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := Encode(c.in)
			if err != nil {
				t.Fatalf("Encode(%q): %v", c.in, err)
			}
			if got := Decode(b); got != c.want {
				t.Errorf("Decode(Encode(%q)) = %q (% x), want %q", c.in, got, b, c.want)
			}
		})
	}
}

// TestDecodeLossy checks the lossiness signal: a defined byte decodes cleanly
// while an undefined high byte passes through best-effort and is flagged.
func TestDecodeLossy(t *testing.T) {
	if _, lossy := DecodeLossy([]byte{0xC2, 0x65}); lossy {
		t.Error("a defined acute+e pair should not be flagged lossy")
	}
	if _, lossy := DecodeLossy([]byte{0x80}); !lossy {
		t.Error("an undefined high byte should be flagged lossy")
	}
}

func TestEncodeRejectsUnrepresentable(t *testing.T) {
	for _, s := range []string{"日本語", "😀", "Привет"} {
		if _, err := Encode(s); err == nil {
			t.Errorf("Encode(%q): expected error", s)
		}
	}
}

// FuzzEncode ensures encoding never panics and that the decode/encode/decode
// cycle preserves text. ISO 5426 has several valid byte encodings for the same
// characters (a precomposed letter as a graphic byte vs a mark+base pair, spacing
// characters as a composition vs a direct byte), so the bytes need not be
// identical across a round trip — but the decoded text must be.
func FuzzEncode(f *testing.F) {
	f.Add("Müller café naïve")
	f.Add("Škoda Œuvre città")
	f.Add("plain ascii")
	// Stacked combining marks exercise the mark-order path where task 040's
	// swap bug lived: NFC ế, its NFD form (e + circumflex + acute), and a base
	// carrying two different-class marks (u + dot-below + circumflex).
	f.Add("\u1ebf")
	f.Add("e\u0302\u0301")
	f.Add("u\u0323\u0302")
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return
		}
		b, err := Encode(s)
		if err != nil {
			return // outside the ISO 5426 repertoire
		}
		canonical := Decode(b)
		if !utf8.ValidString(canonical) {
			t.Fatalf("decode produced invalid UTF-8: %q", canonical)
		}
		b2, err := Encode(canonical)
		if err != nil {
			t.Fatalf("re-encode of decoded form failed: %v", err)
		}
		if got := Decode(b2); got != canonical {
			t.Errorf("round trip not stable:\n have %q\n want %q\n  b=% x b2=% x", got, canonical, b, b2)
		}
	})
}

func BenchmarkDecode(b *testing.B) {
	in, _ := Encode("Müller café naïve Škoda Œuvre città però perché")
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		_ = Decode(in)
	}
}
