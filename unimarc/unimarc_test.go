package unimarc

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/citation"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/mods"
	"github.com/freeeve/libcodex/schemaorg"
)

func corpus(t *testing.T) []*codex.Record {
	t.Helper()
	recs, err := ReadFile(filepath.Join("testdata", "iccu-unimarc.dat"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("read %d records, want 1", len(recs))
	}
	return recs
}

func TestAccessors(t *testing.T) {
	r := corpus(t)[0]
	if got := Title(r); got != "L'altra faccia della spirale" {
		t.Errorf("Title = %q", got) // non-sort control characters stripped
	}
	authors := Authors(r)
	if len(authors) == 0 || authors[0] != "Asimov, Isaac" {
		t.Errorf("Authors = %v", authors)
	}
	if got := ISBN(r); len(got) != 1 || got[0] != "88-04-40682-8" {
		t.Errorf("ISBN = %v", got)
	}
	if got := Language(r); got != "ita" {
		t.Errorf("Language = %q", got)
	}
	if got := Publisher(r); got != "A. Mondadori" {
		t.Errorf("Publisher = %q", got)
	}
	if got := PublicationDate(r); got != "1996" {
		t.Errorf("PublicationDate = %q", got)
	}
}

func TestToMARC21(t *testing.T) {
	m := ToMARC21(corpus(t)[0])
	checks := map[string]string{
		"245a": "L'altra faccia della spirale",
		"245c": "Isaac Asimov",
		"100a": "Asimov, Isaac",
		"020a": "88-04-40682-8",
		"260b": "A. Mondadori",
		"260c": "1996",
	}
	for k, want := range checks {
		tag, code := k[:3], k[3]
		if got := m.SubfieldValue(tag, code); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if c := m.ControlField("008"); len(c) < 38 || c[35:38] != "ita" {
		t.Errorf("008 language = %q", c)
	}
	if m.Leader().Encoding() != 'a' {
		t.Errorf("MARC 21 leader should mark UTF-8, got %q", m.Leader().Encoding())
	}
}

// TestToMARC21FeedsExporters confirms a converted record is valid input for the
// existing exporters (the whole point of the crosswalk).
func TestToMARC21FeedsExporters(t *testing.T) {
	m := ToMARC21(corpus(t)[0])
	b, err := schemaorg.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"@type":"Book"`, `"L'altra faccia della spirale"`, `"Asimov, Isaac"`, `"inLanguage":"it"`} {
		if !contains(s, want) {
			t.Errorf("schema.org output missing %q:\n%s", want, s)
		}
	}
	// And it round-trips through ISO 2709 as a valid MARC 21 record.
	if _, err := iso2709.Encode(m); err != nil {
		t.Errorf("ToMARC21 result is not encodable: %v", err)
	}
}

// TestISO5426Path builds a synthetic legacy UNIMARC record (charset code 01) and
// confirms its values are transcoded from ISO 5426 to UTF-8 on read.
func TestISO5426Path(t *testing.T) {
	coded := make([]byte, 34)
	for i := range coded {
		coded[i] = ' '
	}
	coded[26], coded[27] = '0', '1' // base Latin (ISO 646) in the first slot
	coded[28], coded[29] = '0', '3' // ISO 5426 extended Latin in the second slot
	rec := codex.NewRecord().
		AddField(codex.NewDataField("100", ' ', ' ', codex.NewSubfield('a', string(coded)))).
		AddField(codex.NewDataField("200", '1', ' ',
			codex.NewSubfield('a', string([]byte{0xC2, 0x65, 'c', 'o', 'l', 'e'})))) // "école" via ISO 5426 acute+e

	raw, err := iso2709.Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if v := got.SubfieldValue("200", 'a'); v != "école" {
		t.Errorf("ISO 5426 200$a = %q, want %q", v, "école")
	}
	if Title(got) != "école" {
		t.Errorf("Title = %q", Title(got))
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// richRecord is a synthetic UNIMARC record exercising the full crosswalk.
func richRecord() *codex.Record {
	coded := make([]byte, 34)
	for i := range coded {
		coded[i] = ' '
	}
	copy(coded[0:8], "20200101")
	coded[8] = 'd'
	copy(coded[9:13], "2020")
	coded[26], coded[27] = '5', '0' // Unicode
	return codex.NewRecord().
		SetLeader(codex.Leader("00000nam0 22000000 4500")).
		AddField(codex.NewDataField("010", ' ', ' ', codex.NewSubfield('a', "979-12-345"))).
		AddField(codex.NewDataField("011", ' ', ' ', codex.NewSubfield('a', "1234-5678"))).
		AddField(codex.NewDataField("100", ' ', ' ', codex.NewSubfield('a', string(coded)))).
		AddField(codex.NewDataField("101", '0', ' ', codex.NewSubfield('a', "fre"))).
		AddField(codex.NewDataField("200", '1', ' ', codex.NewSubfield('a', "Le titre"), codex.NewSubfield('e', "sous-titre"), codex.NewSubfield('f', "par Untel"))).
		AddField(codex.NewDataField("205", ' ', ' ', codex.NewSubfield('a', "2e édition"))).
		AddField(codex.NewDataField("210", ' ', ' ', codex.NewSubfield('a', "Paris"), codex.NewSubfield('c', "Gallimard"), codex.NewSubfield('d', "2020"))).
		AddField(codex.NewDataField("215", ' ', ' ', codex.NewSubfield('a', "300 p."), codex.NewSubfield('d', "24 cm"))).
		AddField(codex.NewDataField("225", ' ', ' ', codex.NewSubfield('a', "Collection X"), codex.NewSubfield('v', "5"))).
		AddField(codex.NewDataField("330", ' ', ' ', codex.NewSubfield('a', "Résumé du livre."))).
		AddField(codex.NewDataField("606", ' ', ' ', codex.NewSubfield('a', "Sujet"), codex.NewSubfield('x', "Sous-sujet"))).
		AddField(codex.NewDataField("607", ' ', ' ', codex.NewSubfield('a', "France"))).
		AddField(codex.NewDataField("700", ' ', '1', codex.NewSubfield('a', "Dupont"), codex.NewSubfield('b', "Jean"), codex.NewSubfield('f', "1950-"))).
		AddField(codex.NewDataField("710", ' ', '1', codex.NewSubfield('a', "Institut National")))
}

func TestToMARC21Comprehensive(t *testing.T) {
	m := ToMARC21(richRecord())
	checks := map[string]string{
		"020a": "979-12-345", "022a": "1234-5678",
		"245a": "Le titre", "245b": "sous-titre", "245c": "par Untel",
		"250a": "2e édition", "260a": "Paris", "260b": "Gallimard", "260c": "2020",
		"300a": "300 p.", "490a": "Collection X", "520a": "Résumé du livre.",
		"650a": "Sujet", "650x": "Sous-sujet", "651a": "France",
		"100a": "Dupont, Jean", "100d": "1950-", "110a": "Institut National",
	}
	for k, want := range checks {
		if got := m.SubfieldValue(k[:3], k[3]); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	// UNIMARC 100/8 'd' (monograph complete when issued) must become MARC 008/06
	// 's' (single known date), not the verbatim 'd' (a ceased serial to MARC).
	if c := m.ControlField("008"); len(c) < 7 || c[6] != 's' {
		t.Errorf("008/06 type-of-date = %q, want 's'", c)
	}
}

// TestSubjectSubdivisionSwap confirms UNIMARC's geographical $y and chronological
// $z are swapped into MARC 21's $z (geographic) and $y (chronological), and that
// the result renders as the correct MODS temporal/geographic elements.
func TestSubjectSubdivisionSwap(t *testing.T) {
	r := codex.NewRecord().
		AddField(codex.NewDataField("606", ' ', ' ',
			codex.NewSubfield('a', "Art"),
			codex.NewSubfield('y', "France"),      // UNIMARC geographical
			codex.NewSubfield('z', "19e siècle"))) // UNIMARC chronological
	m := ToMARC21(r)
	if got := m.SubfieldValue("650", 'z'); got != "France" {
		t.Errorf("650$z (geographic) = %q, want %q", got, "France")
	}
	if got := m.SubfieldValue("650", 'y'); got != "19e siècle" {
		t.Errorf("650$y (chronological) = %q, want %q", got, "19e siècle")
	}
	b, err := mods.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !contains(s, "<geographic>France</geographic>") {
		t.Errorf("MODS missing <geographic>France</geographic>:\n%s", s)
	}
	if !contains(s, "<temporal>19e siècle</temporal>") {
		t.Errorf("MODS missing <temporal>19e siècle</temporal>:\n%s", s)
	}
}

// TestDateType covers the UNIMARC 100/8 -> MARC 008/06 translation table.
func TestDateType(t *testing.T) {
	cases := []struct {
		u        byte
		hasDate1 bool
		want     byte
	}{
		{'a', true, 'c'}, {'b', true, 'd'}, {'d', true, 's'}, {'f', true, 'q'},
		{'g', true, 'm'}, {'h', true, 't'}, {'i', true, 't'}, {'j', true, 'e'},
		{'u', true, 'n'}, {'x', true, 's'}, {'x', false, 'b'}, {' ', false, 'b'},
	}
	for _, c := range cases {
		if got := dateType(c.u, c.hasDate1); got != c.want {
			t.Errorf("dateType(%q, %v) = %q, want %q", c.u, c.hasDate1, got, c.want)
		}
	}
}

// TestNameInversionCitation confirms the surname-first name (700 ind2=1) keeps
// its comma so citation derives the surname correctly instead of treating the
// forename as part of it.
func TestNameInversionCitation(t *testing.T) {
	m := ToMARC21(richRecord())
	if got := m.SubfieldValue("100", 'a'); got != "Dupont, Jean" {
		t.Fatalf("100$a = %q, want %q", got, "Dupont, Jean")
	}
	b := citation.FromRecord(m).BibTeX()
	s := string(b)
	if !contains(s, "Dupont, Jean") {
		t.Errorf("BibTeX author not inverted (want 'Dupont, Jean'):\n%s", s)
	}
	// The cite key uses the surname (text before the comma), not "dupontjean".
	if !contains(s, "@book{dupont2020") {
		t.Errorf("BibTeX cite key should derive surname 'dupont':\n%s", s)
	}
}

// TestCyrillicFlaggedLossy confirms a record declaring an unsupported Cyrillic
// set is flagged lossy by the streaming reader rather than silently mis-decoded,
// and that a plain UTF-8 record is not flagged.
func TestCyrillicFlaggedLossy(t *testing.T) {
	build := func(cs string) []byte {
		coded := make([]byte, 34)
		for i := range coded {
			coded[i] = ' '
		}
		coded[26], coded[27] = cs[0], cs[1]
		rec := codex.NewRecord().
			AddField(codex.NewDataField("100", ' ', ' ', codex.NewSubfield('a', string(coded)))).
			AddField(codex.NewDataField("200", '1', ' ', codex.NewSubfield('a', "Title")))
		raw, err := iso2709.Encode(rec)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	for _, tc := range []struct {
		cs        string
		wantLossy bool
	}{
		{"50", false}, // UTF-8
		{"01", false}, // base Latin
		{"02", true},  // basic Cyrillic -- no decoder
		{"05", true},  // Greek -- no decoder
	} {
		rd := NewReader(bytes.NewReader(build(tc.cs)))
		if _, err := rd.Read(); err != nil {
			t.Fatalf("charset %q: Read: %v", tc.cs, err)
		}
		if got := rd.Lossy(); got != tc.wantLossy {
			t.Errorf("charset %q: Lossy() = %v, want %v", tc.cs, got, tc.wantLossy)
		}
	}
}

func TestAccessorsRich(t *testing.T) {
	r := richRecord()
	if got := ISSN(r); len(got) != 1 || got[0] != "1234-5678" {
		t.Errorf("ISSN = %v", got)
	}
	if got := Edition(r); got != "2e édition" {
		t.Errorf("Edition = %q", got)
	}
	subs := Subjects(r)
	if len(subs) != 2 || subs[0] != "Sujet--Sous-sujet" || subs[1] != "France" {
		t.Errorf("Subjects = %v", subs)
	}
}

func TestReadMalformed(t *testing.T) {
	// A non-numeric record length must error, not loop or panic.
	if _, err := NewReader(strings.NewReader("XXXXXrest")).Read(); err == nil {
		t.Error("expected error for non-numeric length")
	}
	// A truncated record (length larger than the data) must error.
	if _, err := NewReader(strings.NewReader("00099ab")).Read(); err == nil {
		t.Error("expected error for truncated record")
	}
}

// iccuRaw returns the single ISO 2709 record from the ICCU corpus, trimmed to
// its leader-declared length.
func iccuRaw(tb testing.TB) []byte {
	tb.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "iccu-unimarc.dat"))
	if err != nil {
		tb.Fatalf("ReadFile: %v", err)
	}
	if n, ok := atoi5(raw[:5]); ok && n <= len(raw) {
		raw = raw[:n]
	}
	return raw
}

func BenchmarkDecode(b *testing.B) {
	raw := iccuRaw(b)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for i := 0; i < b.N; i++ {
		if _, err := Decode(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToMARC21(b *testing.B) {
	r := richRecord()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ToMARC21(r)
	}
}
