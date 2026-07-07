package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/marcjson"
	"github.com/freeeve/libcodex/marcxml"
)

// sampleRecords returns two small but well-formed records covering a control
// field, a title, and the 084/650 classification/subject split seen in the
// OverDrive feed.
func sampleRecords() []*codex.Record {
	r1 := codex.NewRecord().
		AddField(codex.NewControlField("001", "rec-1")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "A First Title"))).
		AddField(codex.NewDataField("084", ' ', ' ', codex.NewSubfield('a', "YAF010010"), codex.NewSubfield('2', "bisacsh"))).
		AddField(codex.NewDataField("650", ' ', '7', codex.NewSubfield('a', "LGBTQIA+ (Fiction)"), codex.NewSubfield('2', "OverDrive")))
	r2 := codex.NewRecord().
		AddField(codex.NewControlField("001", "rec-2")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "A Second Title")))
	return []*codex.Record{r1, r2}
}

// encode serializes records in the named format to bytes for a test fixture.
func encode(t *testing.T, format string, recs []*codex.Record) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := newWriter(format, &buf)
	if err != nil {
		t.Fatalf("newWriter(%s): %v", format, err)
	}
	if err := codex.WriteAll(w, recs); err != nil {
		t.Fatalf("WriteAll(%s): %v", format, err)
	}
	return buf.Bytes()
}

// writeTemp writes data to a temp file and returns its path.
func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCatMrk(t *testing.T) {
	path := writeTemp(t, "in.mrc", encode(t, "marc", sampleRecords()))
	var out bytes.Buffer
	if err := run("cat", []string{path}, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"=LDR", "=245  10$aA First Title", "=084", "rec-1", "=245  10$aA Second Title"} {
		if !strings.Contains(s, want) {
			t.Errorf("cat output missing %q:\n%s", want, s)
		}
	}
}

func TestCatTagsAndLimit(t *testing.T) {
	path := writeTemp(t, "in.mrc", encode(t, "marc", sampleRecords()))
	var out bytes.Buffer
	if err := run("cat", []string{"-t", "084", "-n", "1", path}, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "=084") {
		t.Errorf("expected 084 in output:\n%s", s)
	}
	if strings.Contains(s, "=245") || strings.Contains(s, "=650") {
		t.Errorf("tag filter should drop 245/650:\n%s", s)
	}
	if strings.Contains(s, "A Second Title") {
		t.Errorf("limit=1 should stop after the first record:\n%s", s)
	}
}

func TestCatJSON(t *testing.T) {
	path := writeTemp(t, "in.mrc", encode(t, "marc", sampleRecords()))
	var out bytes.Buffer
	if err := run("cat", []string{"--json", "-n", "1", path}, &out); err != nil {
		t.Fatal(err)
	}
	// The emitted JSON must parse back to a record with the title.
	recs, err := marcjson.NewReader(bytes.NewReader(out.Bytes())).Read()
	if err != nil {
		t.Fatalf("re-reading emitted marcjson: %v", err)
	}
	if got := recs.SubfieldValue("245", 'a'); got != "A First Title" {
		t.Errorf("245$a = %q, want %q", got, "A First Title")
	}
}

func TestConvertRoundTrip(t *testing.T) {
	path := writeTemp(t, "in.mrc", encode(t, "marc", sampleRecords()))
	var xml bytes.Buffer
	if err := run("convert", []string{"-o", "marcxml", path}, &xml); err != nil {
		t.Fatal(err)
	}
	recs, err := codex.ReadAll(marcxml.NewReader(&xml))
	if err != nil {
		t.Fatalf("reading converted marcxml: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("round-trip produced %d records, want 2", len(recs))
	}
	if got := recs[0].SubfieldValue("084", 'a'); got != "YAF010010" {
		t.Errorf("084$a survived as %q, want YAF010010", got)
	}
}

func TestConvertAutodetect(t *testing.T) {
	// A marcjson input with no -i must be sniffed and transcoded.
	path := writeTemp(t, "in.json", encode(t, "marcjson", sampleRecords()))
	var out bytes.Buffer
	if err := run("convert", []string{"-o", "mrk", path}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "A First Title") {
		t.Errorf("autodetected marcjson did not transcode:\n%s", out.String())
	}
}

func TestConvertMissingOutput(t *testing.T) {
	path := writeTemp(t, "in.mrc", encode(t, "marc", sampleRecords()))
	if err := run("convert", []string{path}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when -o is omitted")
	}
}

func TestConvertUnknownOutput(t *testing.T) {
	path := writeTemp(t, "in.mrc", encode(t, "marc", sampleRecords()))
	if err := run("convert", []string{"-o", "bogus", path}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for unknown output format")
	}
}

func TestConvertUnknownInput(t *testing.T) {
	path := writeTemp(t, "in.mrc", encode(t, "marc", sampleRecords()))
	if err := run("convert", []string{"-i", "bogus", "-o", "mrk", path}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for unknown input format")
	}
}

// TestConvertUndetectable feeds a stream whose format cannot be sniffed, so
// convert must fail rather than emit nothing.
func TestConvertUndetectable(t *testing.T) {
	path := writeTemp(t, "in.bin", []byte("not a bibliographic record at all"))
	if err := run("convert", []string{"-o", "mrk", path}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for undetectable input")
	}
}

func TestValidateClean(t *testing.T) {
	path := writeTemp(t, "in.mrc", encode(t, "marc", sampleRecords()))
	var out bytes.Buffer
	if err := run("validate", []string{path}, &out); err != nil {
		t.Fatalf("clean records should validate: %v", err)
	}
	if !strings.Contains(out.String(), "checked 2 record(s), 0 invalid") {
		t.Errorf("unexpected validate summary:\n%s", out.String())
	}
}

func TestStats(t *testing.T) {
	path := writeTemp(t, "in.mrc", encode(t, "marc", sampleRecords()))
	var out bytes.Buffer
	if err := run("stats", []string{path}, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"records: 2", "245  2", "084  1", "record type (leader/06):"} {
		if !strings.Contains(s, want) {
			t.Errorf("stats missing %q:\n%s", want, s)
		}
	}
}

func TestUnknownSubcommand(t *testing.T) {
	if err := run("frobnicate", nil, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestSniff(t *testing.T) {
	cases := map[string][]byte{
		"iso2709":  encode(t, "marc", sampleRecords()),
		"marcxml":  encode(t, "marcxml", sampleRecords()),
		"marcjson": encode(t, "marcjson", sampleRecords()),
		"mrk":      encode(t, "mrk", sampleRecords()),
	}
	for want, data := range cases {
		if got := sniff(bufio.NewReader(bytes.NewReader(data))); got != want {
			t.Errorf("sniff = %q, want %q", got, want)
		}
	}
	if got := sniff(bufio.NewReader(strings.NewReader("not a record"))); got != "" {
		t.Errorf("sniff of garbage = %q, want empty", got)
	}
}

// TestBibframeSniff checks an RDF/XML document is distinguished from plain
// marcxml so a bibframe file round-detects.
func TestBibframeSniff(t *testing.T) {
	doc := `<?xml version="1.0"?>` + "\n" + `<rdf:RDF xmlns:bf="http://id.loc.gov/ontologies/bibframe/">`
	if got := sniff(bufio.NewReader(strings.NewReader(doc))); got != "bibframe" {
		t.Errorf("sniff of rdf:RDF = %q, want bibframe", got)
	}
}

func TestHelp(t *testing.T) {
	var out bytes.Buffer
	if err := run("help", nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "inspect and convert") {
		t.Errorf("help missing banner:\n%s", out.String())
	}
}

func TestVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run("version", nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "libcodex ") {
		t.Errorf("version output = %q, want a \"libcodex <version>\" line", out.String())
	}
}

func TestFileNotFound(t *testing.T) {
	if err := run("cat", []string{filepath.Join(t.TempDir(), "nope.mrc")}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestUnknownInputFormat(t *testing.T) {
	path := writeTemp(t, "in.mrc", encode(t, "marc", sampleRecords()))
	if err := run("cat", []string{"-i", "bogus", path}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for unknown input format")
	}
}

func TestMultipleFiles(t *testing.T) {
	a := writeTemp(t, "a.mrc", encode(t, "marc", sampleRecords()))
	b := writeTemp(t, "b.mrc", encode(t, "marc", sampleRecords()))
	var out bytes.Buffer
	if err := run("stats", []string{a, b}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "records: 4") {
		t.Errorf("two 2-record files should total 4:\n%s", out.String())
	}
}

// TestValidateInvalid feeds a MARC-in-JSON record whose data field carries no
// subfields, which the structural check must flag, and confirms the failing
// record's control number is reported.
func TestValidateInvalid(t *testing.T) {
	doc := `[{"leader":"00000nam a2200000 a 4500","fields":[` +
		`{"001":"bad-1"},` +
		`{"650":{"ind1":" ","ind2":" ","subfields":[]}}]}]`
	path := writeTemp(t, "bad.json", []byte(doc))
	var out bytes.Buffer
	err := run("validate", []string{path}, &out)
	if err == nil {
		t.Fatal("expected non-nil error when a record is invalid")
	}
	s := out.String()
	if !strings.Contains(s, "1 invalid") || !strings.Contains(s, "bad-1") {
		t.Errorf("invalid record not reported with its 001:\n%s", s)
	}
}

// TestPureHelpers exercises the small parsing helpers directly.
func TestPureHelpers(t *testing.T) {
	set := tagSet(" 084 , 650 ,")
	if !set["084"] || !set["650"] || len(set) != 2 {
		t.Errorf("tagSet = %v", set)
	}
	if tagSet("") != nil {
		t.Error("empty tag list should yield nil (keep all)")
	}
	if !isDigits("01234") || isDigits("12x45") {
		t.Error("isDigits misclassified")
	}
	rec := codex.NewRecord().AddField(codex.NewControlField("001", "cn-9"))
	if got := recControlNumber(rec); got != "cn-9" {
		t.Errorf("recControlNumber = %q, want cn-9", got)
	}
	if got := recControlNumber(codex.NewRecord()); got != "?" {
		t.Errorf("recControlNumber of 001-less record = %q, want ?", got)
	}
}
