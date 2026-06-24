package marc8

import "testing"

// TestDecodeASCIIIdentity verifies that MARC-8 decoding is the identity on plain
// ASCII (no escape, no byte >= 0x80). The iso2709 reader relies on this to take
// such fields as zero-copy substrings instead of transcoding them.
func TestDecodeASCIIIdentity(t *testing.T) {
	cases := []string{
		"", "a", "Nursing services", "Feinberg, Leslie,", "PS3556 .E446",
		"0786803525", "abc 123 !@#$%^&*()_+-=[]{};':,./<>?", "Melipona",
	}
	// every printable ASCII character, and the whole 0x20-0x7e range as one string
	var all []byte
	for c := byte(0x20); c <= 0x7e; c++ {
		all = append(all, c)
	}
	cases = append(cases, string(all))
	for _, s := range cases {
		if got := NewDecoder().Decode([]byte(s)); got != s {
			t.Errorf("Decode(%q) = %q, want identity", s, got)
		}
	}
}
