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

func TestEncodeRejectsUnrepresentable(t *testing.T) {
	for _, s := range []string{"日本語", "😀", "Привет"} {
		if _, err := Encode(s); err == nil {
			t.Errorf("Encode(%q): expected error", s)
		}
	}
}

// FuzzEncode ensures encoding never panics and that re-encoding the decoded form
// is stable (Encode is the inverse of Decode on the canonical form).
func FuzzEncode(f *testing.F) {
	f.Add("Müller café naïve")
	f.Add("Škoda Œuvre città")
	f.Add("plain ascii")
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return
		}
		b, err := Encode(s)
		if err != nil {
			return // outside the ISO 5426 repertoire
		}
		canonical := Decode(b)
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
	in, _ := Encode("Müller café naïve Škoda Œuvre città però perché")
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		_ = Decode(in)
	}
}
