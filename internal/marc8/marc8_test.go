package marc8

import "testing"

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
		{"unknown high byte", []byte{0xFF}, "ÿ"}, // best-effort pass-through
		{"trailing mark", []byte{0xE2}, "́"},     // dangling combining acute
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
		{"multibyte designation", []byte{escape, '$', '1'}, true},
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

func BenchmarkDecode(b *testing.B) {
	in := []byte("Beyonc\xe2e na\xe8ive caf\xe2e \xe5o \xa5 \xc7 cocktail recipes for the modern bar")
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		_ = Decode(in)
	}
}
