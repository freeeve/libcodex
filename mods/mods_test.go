package mods

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/crosswalk"
	"github.com/freeeve/libcodex/iso2709"
)

func sample() *codex.Record {
	return codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewControlField("001", "92005291")).
		AddField(codex.NewControlField("008", "920219s1993    nyua   j      000 1 eng  ")).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "0786803525"))).
		AddField(codex.NewDataField("100", '1', ' ',
			codex.NewSubfield('a', "Feinberg, Leslie,"),
			codex.NewSubfield('d', "1949-2014"),
			codex.NewSubfield('e', "author"))).
		AddField(codex.NewDataField("245", '1', '0',
			codex.NewSubfield('a', "Stone butch blues :"),
			codex.NewSubfield('b', "a novel /"),
			codex.NewSubfield('c', "Leslie Feinberg."))).
		AddField(codex.NewDataField("264", ' ', '1',
			codex.NewSubfield('a', "Ithaca, New York :"),
			codex.NewSubfield('b', "Firebrand Books,"),
			codex.NewSubfield('c', "1993"))).
		AddField(codex.NewDataField("300", ' ', ' ', codex.NewSubfield('a', "301 pages ;"), codex.NewSubfield('c', "22 cm"))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Lesbians"), codex.NewSubfield('v', "Fiction."))).
		AddField(codex.NewDataField("655", ' ', '7', codex.NewSubfield('a', "Bildungsromans")))
}

// wellFormed reports an error if b is not well-formed XML.
func wellFormed(t *testing.T, b []byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		if _, err := dec.Token(); err != nil {
			if err == io.EOF {
				return
			}
			t.Fatalf("not well-formed XML: %v\n%s", err, b)
		}
	}
}

func TestEncodeMapping(t *testing.T) {
	b, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, b)

	var m MODS
	if err := xml.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(m.TitleInfo) != 1 || m.TitleInfo[0].Title != "Stone butch blues" || m.TitleInfo[0].SubTitle != "a novel" {
		t.Errorf("titleInfo = %+v", m.TitleInfo)
	}
	if len(m.Name) != 1 || m.Name[0].Type != "personal" || m.Name[0].NamePart[0].Value != "Feinberg, Leslie" {
		t.Errorf("name = %+v", m.Name)
	}
	if m.Name[0].Role == nil || m.Name[0].Role.RoleTerm.Value != "author" {
		t.Errorf("role = %+v", m.Name[0].Role)
	}
	if m.TypeOfResource != "text" {
		t.Errorf("typeOfResource = %q", m.TypeOfResource)
	}
	if m.OriginInfo == nil || m.OriginInfo.Publisher != "Firebrand Books" || m.OriginInfo.DateIssued != "1993" {
		t.Errorf("originInfo = %+v", m.OriginInfo)
	}
	if len(m.Language) != 1 || m.Language[0].LanguageTerm.Value != "eng" {
		t.Errorf("language = %+v", m.Language)
	}
	if m.PhysicalDesc == nil || m.PhysicalDesc.Extent != "301 pages 22 cm" {
		t.Errorf("physicalDescription = %+v", m.PhysicalDesc)
	}
	if len(m.Subject) != 2 {
		t.Fatalf("subjects = %d, want 2", len(m.Subject))
	}
	if m.Subject[0].Authority != "lcsh" || len(m.Subject[0].Topic) != 1 || m.Subject[0].Topic[0] != "Lesbians" {
		t.Errorf("subject[0] = %+v", m.Subject[0])
	}
	if len(m.Subject[0].Genre) != 1 || m.Subject[0].Genre[0] != "Fiction." {
		t.Errorf("subject[0] genre = %+v", m.Subject[0].Genre)
	}
	if len(m.Identifier) != 1 || m.Identifier[0].Type != "isbn" || m.Identifier[0].Value != "0786803525" {
		t.Errorf("identifier = %+v", m.Identifier)
	}
	if m.RecordInfo == nil || m.RecordInfo.RecordIdentifier != "92005291" {
		t.Errorf("recordInfo = %+v", m.RecordInfo)
	}
}

func TestEncodeMappingExtras(t *testing.T) {
	// A record exercising the corporate/conference/uniform/subject/identifier
	// branches not covered by sample().
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000cem a2200000 a 4500")). // 'e' -> cartographic
		AddField(codex.NewControlField("008", "200101s2020    xxu                 fre  ")).
		AddField(codex.NewDataField("022", ' ', ' ', codex.NewSubfield('a', "1234-5678"))).
		AddField(codex.NewDataField("024", ' ', ' ', codex.NewSubfield('a', "urn:x"))).
		AddField(codex.NewDataField("041", ' ', ' ', codex.NewSubfield('a', "engfre"))).
		AddField(codex.NewDataField("110", '2', ' ', codex.NewSubfield('a', "Acme Corp."))).
		AddField(codex.NewDataField("111", '2', ' ', codex.NewSubfield('a', "Conf 2020."))).
		AddField(codex.NewDataField("130", '0', ' ', codex.NewSubfield('a', "Uniform title"))).
		AddField(codex.NewDataField("250", ' ', ' ', codex.NewSubfield('a', "2nd ed."))).
		AddField(codex.NewDataField("500", ' ', ' ', codex.NewSubfield('a', "A general note"))).
		AddField(codex.NewDataField("520", ' ', ' ', codex.NewSubfield('a', "A summary"))).
		AddField(codex.NewDataField("600", '1', '0', codex.NewSubfield('a', "Person, A."))).
		AddField(codex.NewDataField("651", ' ', '0', codex.NewSubfield('a', "France"))).
		AddField(codex.NewDataField("856", '4', '0', codex.NewSubfield('u', "https://example.org")))

	b, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, b)

	var m MODS
	if err := xml.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.TypeOfResource != "cartographic" {
		t.Errorf("typeOfResource = %q, want cartographic", m.TypeOfResource)
	}
	// 110 corporate, 111 conference names.
	types := map[string]bool{}
	for _, n := range m.Name {
		types[n.Type] = true
	}
	if !types["corporate"] || !types["conference"] {
		t.Errorf("name types = %v", types)
	}
	// 130 uniform title.
	var uniform bool
	for _, ti := range m.TitleInfo {
		if ti.Type == "uniform" {
			uniform = true
		}
	}
	if !uniform {
		t.Error("missing uniform titleInfo from 130")
	}
	if m.OriginInfo == nil || m.OriginInfo.Edition != "2nd ed." {
		t.Errorf("edition = %+v", m.OriginInfo)
	}
	// Two languages (008 fre + 041 eng): order is 008 first.
	if len(m.Language) != 2 {
		t.Errorf("languages = %d, want 2 (%+v)", len(m.Language), m.Language)
	}
	if len(m.Note) != 2 {
		t.Errorf("notes = %d, want 2", len(m.Note))
	}
	if len(m.Identifier) < 3 { // issn, other, uri
		t.Errorf("identifiers = %d, want >= 3", len(m.Identifier))
	}
	// 600 name subject + 651 geographic subject.
	var nameSubj, geoSubj bool
	for _, s := range m.Subject {
		if s.Name != nil {
			nameSubj = true
		}
		if len(s.Geographic) > 0 {
			geoSubj = true
		}
	}
	if !nameSubj || !geoSubj {
		t.Errorf("subjects missing name(%v)/geographic(%v)", nameSubj, geoSubj)
	}
}

// TestOrigin264Indicators checks the 264 second-indicator handling: publication
// (1) supplies publisher/date; a copyright statement (4) maps $c to
// copyrightDate and never to publisher/dateIssued; and when both are present the
// publication statement wins.
func TestOrigin264Indicators(t *testing.T) {
	// A copyright-only record: no publisher, copyrightDate set, dateIssued not "©".
	cr := codex.NewRecord().
		AddField(codex.NewDataField("264", ' ', '4', codex.NewSubfield('c', "©2016")))
	o := FromRecord(cr).OriginInfo
	if o == nil || o.CopyrightDate != "©2016" {
		t.Errorf("copyright-only originInfo = %+v, want CopyrightDate ©2016", o)
	}
	if o != nil && o.DateIssued == "©2016" {
		t.Errorf("copyright date leaked into dateIssued: %+v", o)
	}
	if o != nil && o.Publisher != "" {
		t.Errorf("copyright statement must not set publisher: %+v", o)
	}

	// Publication (264_1) preferred over a distribution statement (264_2).
	both := codex.NewRecord().
		AddField(codex.NewDataField("264", ' ', '1',
			codex.NewSubfield('a', "London :"), codex.NewSubfield('b', "Penguin,"), codex.NewSubfield('c', "2019"))).
		AddField(codex.NewDataField("264", ' ', '2',
			codex.NewSubfield('a', "Boston :"), codex.NewSubfield('b', "Distributor,"), codex.NewSubfield('c', "2020")))
	o = FromRecord(both).OriginInfo
	if o == nil || o.Publisher != "Penguin" || o.DateIssued != "2019" {
		t.Errorf("publication statement should win: %+v", o)
	}
	if len(o.Place) != 1 || o.Place[0].PlaceTerm.Value != "London" {
		t.Errorf("place should come only from the publication statement: %+v", o.Place)
	}
}

// TestRDAHybridNoDuplicatePlace confirms a legacy 260 plus an RDA 264 does not
// emit the place twice.
func TestRDAHybridNoDuplicatePlace(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewDataField("260", ' ', ' ',
			codex.NewSubfield('a', "New York :"), codex.NewSubfield('b', "Old Pub,"), codex.NewSubfield('c', "1990"))).
		AddField(codex.NewDataField("264", ' ', '1',
			codex.NewSubfield('a', "New York :"), codex.NewSubfield('b', "New Pub,"), codex.NewSubfield('c', "1990")))
	o := FromRecord(rec).OriginInfo
	if o == nil || len(o.Place) != 1 {
		t.Errorf("RDA hybrid should yield exactly one place, got %+v", o)
	}
	if o != nil && o.Publisher != "New Pub" {
		t.Errorf("264 publication publisher should win: %+v", o)
	}
}

// TestTitleFromPartOnly confirms a 245 titled only via $n/$p (no $a) is not
// dropped.
func TestTitleFromPartOnly(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewDataField("245", '0', '0',
			codex.NewSubfield('n', "Part 2,"), codex.NewSubfield('p', "The Return")))
	m := FromRecord(rec)
	if len(m.TitleInfo) != 1 || m.TitleInfo[0].PartName != "The Return" {
		t.Errorf("part-only titleInfo dropped: %+v", m.TitleInfo)
	}
}

func TestEncodeNamespace(t *testing.T) {
	b, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `xmlns="`+Namespace+`"`) {
		t.Errorf("missing namespace:\n%s", b)
	}
}

func TestWriterCollection(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(sample()); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(codex.NewRecord().AddField(codex.NewDataField("245", '0', '0', codex.NewSubfield('a', "Second")))); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	wellFormed(t, out)
	if !strings.Contains(string(out), collectionOpen) || !strings.Contains(string(out), collectionClose) {
		t.Error("missing modsCollection wrapper")
	}
	if n := strings.Count(string(out), "<mods>"); n != 2 {
		t.Errorf("found %d <mods> elements, want 2", n)
	}
}

func TestWriteAfterCloseFails(t *testing.T) {
	w := NewWriter(&bytes.Buffer{})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(sample()); err == nil {
		t.Error("expected error writing after Close")
	}
}

func TestConvertFromISO2709(t *testing.T) {
	// MARC binary -> MODS through the core interfaces.
	mrc, err := iso2709.Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := NewWriter(&out)
	if err := codex.Convert(iso2709.NewReader(bytes.NewReader(mrc)), w); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	wellFormed(t, out.Bytes())
	if !strings.Contains(out.String(), "<title>Stone butch blues</title>") {
		t.Errorf("converted MODS missing title:\n%s", out.String())
	}
}

func TestEmptyRecord(t *testing.T) {
	// A record with no mapped fields still produces a valid <mods> document.
	b, err := Encode(codex.NewRecord())
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, b)
}

func TestReadWriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.mods.xml")
	if err := WriteFile(path, []*codex.Record{sample(), sample()}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wellFormed(t, b)
	if err := WriteFile(filepath.Join(t.TempDir(), "no-dir", "x.xml"), []*codex.Record{sample()}); err == nil {
		t.Error("expected error writing into a nonexistent directory")
	}
}

func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{
		"Stone butch blues :": "Stone butch blues",
		"a novel /":           "a novel",
		"text;":               "text",
		"Feinberg, Leslie,":   "Feinberg, Leslie",
		"keep.":               "keep.",
		"plain":               "plain",
	} {
		if got := crosswalk.TrimISBD(in); got != want {
			t.Errorf("crosswalk.TrimISBD(%q) = %q, want %q", in, got, want)
		}
	}
	if got := typeOfResource('e'); got != "cartographic" {
		t.Errorf("typeOfResource(e) = %q", got)
	}
	if got := typeOfResource('z'); got != "text" {
		t.Errorf("typeOfResource(unknown) = %q, want text", got)
	}
	if got := authority('2'); got != "mesh" {
		t.Errorf("authority(2) = %q", got)
	}
}

// FuzzFromMARC ensures that any decodable MARC record converts to well-formed
// MODS without panicking.
func FuzzFromMARC(f *testing.F) {
	mrc, _ := iso2709.Encode(sample())
	f.Add(mrc)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		rec, _, err := iso2709.Decode(data)
		if err != nil || rec == nil {
			return
		}
		b, err := Encode(rec)
		if err != nil {
			return
		}
		wellFormed(t, b)
	})
}

func TestGolden(t *testing.T) {
	path := filepath.Join("testdata", "sample.mods.xml")
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(sample()); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (UPDATE_GOLDEN=1 to create): %v", err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("output differs from %s:\n%s", path, buf.Bytes())
	}
}
