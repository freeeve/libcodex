package citation

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/iso2709"
)

// failWriter is an io.Writer that always returns the stored error.
type failWriter struct{ err error }

func (fw failWriter) Write(_ []byte) (int, error) { return 0, fw.err }

func sample() *codex.Record {
	return codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewControlField("008", "920219s1993    nyua   j      000 1 eng  ")).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "0786803525"))).
		AddField(codex.NewDataField("100", '1', ' ', codex.NewSubfield('a', "Feinberg, Leslie,"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Stone butch blues :"), codex.NewSubfield('b', "a novel /"))).
		AddField(codex.NewDataField("250", ' ', ' ', codex.NewSubfield('a', "First edition."))).
		AddField(codex.NewDataField("264", ' ', '1', codex.NewSubfield('a', "Ithaca, New York :"), codex.NewSubfield('b', "Firebrand Books,"), codex.NewSubfield('c', "[1993]"))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Lesbians"), codex.NewSubfield('v', "Fiction"))).
		AddField(codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Editor, An,"))).
		AddField(codex.NewDataField("520", ' ', ' ', codex.NewSubfield('a', "100% true & {special}")))
}

func TestRIS(t *testing.T) {
	b, err := RIS(sample())
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, want := range []string{
		"TY  - BOOK\n",
		"TI  - Stone butch blues a novel\n",
		"AU  - Feinberg, Leslie\n",
		"AU  - Editor, An\n",
		"PY  - 1993\n",
		"PB  - Firebrand Books\n",
		"CY  - Ithaca, New York\n",
		"SN  - 0786803525\n",
		"KW  - Lesbians--Fiction\n",
		"LA  - eng\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RIS missing %q:\n%s", want, out)
		}
	}
	if !strings.HasSuffix(out, "ER  - \n") {
		t.Errorf("RIS must end with ER terminator:\n%s", out)
	}
	// RIS is plain text; specials are not escaped.
	if !strings.Contains(out, "AB  - 100% true & {special}\n") {
		t.Errorf("RIS abstract not plain:\n%s", out)
	}
}

func TestBibTeX(t *testing.T) {
	b, err := BibTeX(sample())
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.HasPrefix(out, "@book{feinberg1993stone,\n") {
		t.Errorf("BibTeX entry/key wrong:\n%s", out)
	}
	for _, want := range []string{
		"author = {Feinberg, Leslie and Editor, An},\n",
		"title = {Stone butch blues a novel},\n",
		"year = {1993},\n",
		"edition = {First edition.},\n",
		"publisher = {Firebrand Books},\n",
		"isbn = {0786803525},\n",
		"keywords = {Lesbians--Fiction},\n",
		`abstract = {100\% true \& \{special\}},`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("BibTeX missing %q:\n%s", want, out)
		}
	}
	if !strings.HasSuffix(out, "}\n") {
		t.Error("BibTeX entry must end with }")
	}
}

func TestKind(t *testing.T) {
	cases := map[string][2]string{
		"00000nam a2200000 a 4500": {"BOOK", "book"},    // monograph
		"00000nas a2200000 a 4500": {"JOUR", "article"}, // serial
		"00000naa a2200000 a 4500": {"CHAP", "inbook"},  // component part
		"00000njm a2200000 a 4500": {"SOUND", "misc"},   // musical sound
		"00000nem a2200000 a 4500": {"MAP", "misc"},     // cartographic
	}
	for leader, want := range cases {
		ris, bib := kind(codex.Leader(leader))
		if ris != want[0] || bib != want[1] {
			t.Errorf("kind(%q) = %q/%q, want %q/%q", leader[6:8], ris, bib, want[0], want[1])
		}
	}
}

func TestCiteKeyFallback(t *testing.T) {
	// No author, no title: a fallback key.
	e := FromRecord(codex.NewRecord())
	if k := e.citeKey(); k != "ref" {
		t.Errorf("fallback citeKey = %q, want ref", k)
	}
	// Title only.
	e2 := FromRecord(codex.NewRecord().AddField(codex.NewDataField("245", '0', '0', codex.NewSubfield('a', "The Go Programming Language"))))
	if k := e2.citeKey(); k != "the" {
		t.Errorf("title-only citeKey = %q", k)
	}
}

func TestConvertWriters(t *testing.T) {
	mrc, err := iso2709.Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	for name, newW := range map[string]func(*bytes.Buffer) codex.RecordWriter{
		"ris":    func(b *bytes.Buffer) codex.RecordWriter { return NewRISWriter(b) },
		"bibtex": func(b *bytes.Buffer) codex.RecordWriter { return NewBibTeXWriter(b) },
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := codex.Convert(iso2709.NewReader(bytes.NewReader(mrc)), newW(&out)); err != nil {
				t.Fatal(err)
			}
			if out.Len() == 0 {
				t.Error("no output")
			}
		})
	}
}

func TestWriteFiles(t *testing.T) {
	recs := []*codex.Record{sample(), sample()}
	for _, c := range []struct {
		ext   string
		write func(string, []*codex.Record) error
	}{
		{"ris", WriteRISFile},
		{"bib", WriteBibTeXFile},
	} {
		path := filepath.Join(t.TempDir(), "out."+c.ext)
		if err := c.write(path, recs); err != nil {
			t.Fatalf("%s: %v", c.ext, err)
		}
		b, _ := os.ReadFile(path)
		if len(b) == 0 {
			t.Errorf("%s empty", c.ext)
		}
		if err := c.write(filepath.Join(t.TempDir(), "no-dir", "x"), recs); err == nil {
			t.Errorf("%s: expected error for bad path", c.ext)
		}
	}
}

func TestHelpers(t *testing.T) {
	if got := year("c1993, printed 1995"); got != "1993" {
		t.Errorf("year = %q", got)
	}
	if got := year("n.d."); got != "" {
		t.Errorf("year(n.d.) = %q", got)
	}
	if got := asciiKey("Müller-O'Brien"); got != "mllerobrien" {
		t.Errorf("asciiKey = %q", got)
	}
}

// FuzzFromMARC ensures the MARC->citation paths never panic and produce valid
// UTF-8 output for any decodable record.
func FuzzFromMARC(f *testing.F) {
	mrc, _ := iso2709.Encode(sample())
	f.Add(mrc)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		rec, _, err := iso2709.Decode(data)
		if err != nil || rec == nil {
			return
		}
		for _, b := range [][]byte{mustBytes(RIS(rec)), mustBytes(BibTeX(rec))} {
			if !utf8.Valid(b) {
				t.Errorf("output is not valid UTF-8: %q", b)
			}
		}
	})
}

func mustBytes(b []byte, _ error) []byte { return b }

// TestKindMissing covers the kind branches for type-of-record codes that are
// not exercised by TestKind: 'c'/'d' (notated music → MUSIC), 'g' (projected
// medium → VIDEO), 'm' (computer file → COMP), and an unrecognised code (→ GEN).
func TestKindMissing(t *testing.T) {
	cases := []struct {
		leader  string
		risType string
		bibType string
	}{
		{"00000ncm a2200000 a 4500", "MUSIC", "misc"},
		{"00000ndm a2200000 a 4500", "MUSIC", "misc"},
		{"00000ngm a2200000 a 4500", "VIDEO", "misc"},
		{"00000nmm a2200000 a 4500", "COMP", "misc"},
		{"00000nxm a2200000 a 4500", "GEN", "misc"},
	}
	for _, c := range cases {
		ris, bib := kind(codex.Leader(c.leader))
		if ris != c.risType || bib != c.bibType {
			t.Errorf("kind(%q) = %q/%q, want %q/%q",
				c.leader[6:8], ris, bib, c.risType, c.bibType)
		}
	}
}

// TestFromRecordISSNURLAndYear builds a record that contains an ISSN (022),
// a URL (856), and an 008 control field with a year but no 260/264 date
// subfield — covering three FromRecord branches not reached by sample().
func TestFromRecordISSNURLAndYear(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nas a2200000 a 4500")).
		AddField(codex.NewControlField("008", "921201s2001    nyu           000 0 eng  ")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Test Serial :"))).
		AddField(codex.NewDataField("022", ' ', ' ', codex.NewSubfield('a', "1234-5678"))).
		AddField(codex.NewDataField("856", '4', '0', codex.NewSubfield('u', "https://example.com/resource")))

	e := FromRecord(rec)

	if e.Year != "2001" {
		t.Errorf("008 year fallback: got %q, want %q", e.Year, "2001")
	}
	if len(e.ISSN) == 0 || e.ISSN[0] != "1234-5678" {
		t.Errorf("ISSN: got %v", e.ISSN)
	}
	if len(e.URL) == 0 || e.URL[0] != "https://example.com/resource" {
		t.Errorf("URL: got %v", e.URL)
	}

	// Exercise RIS/BibTeX renderers so ISSN and URL loop bodies are covered.
	ris := string(e.RIS())
	if !strings.Contains(ris, "SN  - 1234-5678\n") {
		t.Errorf("RIS ISSN missing: %s", ris)
	}
	if !strings.Contains(ris, "UR  - https://example.com/resource\n") {
		t.Errorf("RIS URL missing: %s", ris)
	}

	bib := string(e.BibTeX())
	if !strings.Contains(bib, "issn = {1234-5678}") {
		t.Errorf("BibTeX issn missing: %s", bib)
	}
	if !strings.Contains(bib, "url = {https://example.com/resource}") {
		t.Errorf("BibTeX url missing: %s", bib)
	}
}

// TestEmptyEntryOutput calls RIS and BibTeX on an Entry with no populated
// fields, exercising the early-return paths in risLine and bibField when the
// value argument is empty.
func TestEmptyEntryOutput(t *testing.T) {
	e := &Entry{risType: "GEN", bibType: "misc"}
	ris := string(e.RIS())
	if !strings.Contains(ris, "TY  - GEN") {
		t.Errorf("empty-entry RIS missing TY: %s", ris)
	}
	bib := string(e.BibTeX())
	if !strings.HasPrefix(bib, "@misc{ref,\n") {
		t.Errorf("empty-entry BibTeX wrong header: %s", bib)
	}
}

// TestAppendPlainSpecials covers the newline/CR → space replacement,
// valid multi-byte UTF-8 pass-through, and invalid UTF-8 byte dropping in
// appendPlain.
func TestAppendPlainSpecials(t *testing.T) {
	// café is valid multi-byte UTF-8 (U+00E9 = 0xC3 0xA9).
	// \xff is an invalid UTF-8 byte that must be dropped.
	got := string(appendPlain(nil, "line1\nline2\rend caf\xc3\xa9 x\xffy"))
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("appendPlain: raw newline/CR in output: %q", got)
	}
	if !strings.Contains(got, "café") {
		t.Errorf("appendPlain: multi-byte char lost: %q", got)
	}
	if strings.ContainsRune(got, '\xff') {
		t.Errorf("appendPlain: invalid UTF-8 byte not dropped: %q", got)
	}
	if !utf8.Valid([]byte(got)) {
		t.Errorf("appendPlain: output not valid UTF-8: %q", got)
	}
	if !strings.Contains(got, "line1 line2 end") {
		t.Errorf("appendPlain: newlines not replaced with spaces: %q", got)
	}
}

// TestAppendBibTeXSpecials covers backslash, tilde, caret, newline/CR, valid
// multi-byte UTF-8, and invalid UTF-8 in appendBibTeX.
func TestAppendBibTeXSpecials(t *testing.T) {
	// Go raw literal: \\ is a single backslash in the actual string.
	in := "back\\slash tilde~ caret^ nl\n cr\r caf\xc3\xa9 x\xffy"
	got := string(appendBibTeX(nil, in))

	for _, want := range []string{
		`\textbackslash{}`,
		`\textasciitilde{}`,
		`\textasciicircum{}`,
		"café",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("appendBibTeX: missing %q in %q", want, got)
		}
	}
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("appendBibTeX: raw newline/CR in output: %q", got)
	}
	if !utf8.Valid([]byte(got)) {
		t.Errorf("appendBibTeX: output not valid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, '\xff') {
		t.Errorf("appendBibTeX: invalid UTF-8 byte not dropped: %q", got)
	}
}

// TestBibTeXWriterStickyError verifies that BibTeXWriter.Write propagates a
// write error and holds it on subsequent calls (covering the early-return path
// when wr.err is already non-nil).
func TestBibTeXWriterStickyError(t *testing.T) {
	fw := failWriter{errors.New("injected write error")}
	w := NewBibTeXWriter(fw)
	err1 := w.Write(sample())
	if err1 == nil {
		t.Fatal("expected error on first Write")
	}
	err2 := w.Write(sample())
	if err2 != err1 {
		t.Errorf("sticky error mismatch: got %v, want %v", err2, err1)
	}
}

func TestGolden(t *testing.T) {
	for _, c := range []struct {
		name string
		fn   func(*codex.Record) ([]byte, error)
	}{
		{"sample.ris", RIS},
		{"sample.bib", BibTeX},
	} {
		t.Run(c.name, func(t *testing.T) {
			b, err := c.fn(sample())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", c.name)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				os.MkdirAll("testdata", 0o755)
				os.WriteFile(path, b, 0o644)
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (UPDATE_GOLDEN=1 to create): %v", err)
			}
			if !bytes.Equal(b, want) {
				t.Errorf("differs from %s:\n%s", path, b)
			}
		})
	}
}
