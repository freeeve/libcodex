package marc8

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// TestRealMARC8Corpus decodes authentic MARC-8 multiscript data (Latin, CJK,
// Arabic and Hebrew) line by line, asserting the decoder never produces invalid
// UTF-8 and that decoded text re-encodes to a stable byte sequence. See
// testdata/README.md for provenance and license.
func TestRealMARC8Corpus(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "pymarc_marc8.txt"))
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var decoded, lossy, roundTripped, n int
	for line := range bytes.SplitSeq(raw, []byte{'\n'}) {
		n++
		if len(line) == 0 {
			continue
		}
		decoded++

		d := NewDecoder()
		s := d.Decode(line)
		if !utf8.ValidString(s) {
			t.Errorf("line %d: decode produced invalid UTF-8: %q", n, s)
			continue
		}
		if d.Lossy() {
			lossy++
			continue // an unmapped EACC triple decodes to U+FFFD, which cannot re-encode
		}

		// Lossless lines must survive a decode/encode/decode round trip unchanged.
		b, err := Encode(s)
		if err != nil {
			t.Errorf("line %d: re-encode of %q failed: %v", n, s, err)
			continue
		}
		if again := Decode(b); again != s {
			t.Errorf("line %d: round trip changed text\n have %q\n want %q", n, again, s)
			continue
		}
		roundTripped++
	}

	t.Logf("decoded %d lines: %d round-tripped losslessly, %d contained unmapped characters",
		decoded, roundTripped, lossy)
	if roundTripped == 0 {
		t.Fatal("no lines round-tripped; the corpus may not have loaded")
	}
}

// TestRealMARC8Scripts spot-checks that representative lines decode into the
// expected scripts, confirming the set designations are honored on real data.
func TestRealMARC8Scripts(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "pymarc_marc8.txt"))
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var sawCJK, sawArabic, sawHebrew bool
	for line := range bytes.SplitSeq(raw, []byte{'\n'}) {
		s := Decode(line)
		for _, r := range s {
			switch {
			case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
				sawCJK = true
			case r >= 0x0600 && r <= 0x06FF: // Arabic
				sawArabic = true
			case r >= 0x0590 && r <= 0x05FF: // Hebrew
				sawHebrew = true
			}
		}
	}
	if !sawCJK || !sawArabic || !sawHebrew {
		t.Errorf("expected CJK, Arabic and Hebrew in the corpus; got cjk=%v arabic=%v hebrew=%v",
			sawCJK, sawArabic, sawHebrew)
	}
}
