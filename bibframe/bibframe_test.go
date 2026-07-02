package bibframe

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/iso2709"
)

// sample is a rich record exercising most of the crosswalk branches.
func sample() *codex.Record {
	return codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewControlField("001", "92005291")).
		AddField(codex.NewControlField("008", "920219s1993    nyua   j      000 1 eng  ")).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "0786803525"))).
		AddField(codex.NewDataField("022", ' ', ' ', codex.NewSubfield('a', "1234-5678"))).
		AddField(codex.NewDataField("024", ' ', ' ', codex.NewSubfield('a', "urn:isbn:0786803525"))).
		AddField(codex.NewDataField("041", ' ', ' ', codex.NewSubfield('a', "engfre"))).
		AddField(codex.NewDataField("050", ' ', '0', codex.NewSubfield('a', "PS3556"), codex.NewSubfield('b', ".E446"))).
		AddField(codex.NewDataField("082", ' ', ' ', codex.NewSubfield('a', "813.54"))).
		AddField(codex.NewDataField("100", '1', ' ', codex.NewSubfield('a', "Feinberg, Leslie,"), codex.NewSubfield('e', "author"))).
		AddField(codex.NewDataField("240", '1', '0', codex.NewSubfield('a', "Stone butch blues (Uniform)"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Stone butch blues :"), codex.NewSubfield('b', "a novel /"), codex.NewSubfield('c', "Leslie Feinberg."))).
		AddField(codex.NewDataField("250", ' ', ' ', codex.NewSubfield('a', "First edition."))).
		AddField(codex.NewDataField("264", ' ', '1', codex.NewSubfield('a', "Ithaca, New York :"), codex.NewSubfield('b', "Firebrand Books,"), codex.NewSubfield('c', "[1993]"))).
		AddField(codex.NewDataField("300", ' ', ' ', codex.NewSubfield('a', "301 pages ;"), codex.NewSubfield('c', "22 cm"))).
		AddField(codex.NewDataField("520", ' ', ' ', codex.NewSubfield('a', "A novel about <gender> & identity."))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Lesbians"), codex.NewSubfield('v', "Fiction"))).
		AddField(codex.NewDataField("651", ' ', '0', codex.NewSubfield('a', "New York (State)"))).
		AddField(codex.NewDataField("655", ' ', '7', codex.NewSubfield('a', "Bildungsromans."))).
		AddField(codex.NewDataField("600", '1', '0', codex.NewSubfield('a', "Feinberg, Leslie"))).
		AddField(codex.NewDataField("610", '2', '0', codex.NewSubfield('a', "Firebrand Books"))).
		AddField(codex.NewDataField("611", '2', '0', codex.NewSubfield('a', "Some Conference"))).
		AddField(codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Editor, An,"), codex.NewSubfield('4', "edt"))).
		AddField(codex.NewDataField("710", '2', ' ', codex.NewSubfield('a', "A Corporate Body"))).
		AddField(codex.NewDataField("856", '4', '0', codex.NewSubfield('u', "https://example.org/item")))
}

// TestEncodeWellFormed checks the RDF/XML output parses cleanly and carries the
// expected resources.
func TestEncodeWellFormed(t *testing.T) {
	b, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	if err := xmlWellFormed(b); err != nil {
		t.Fatalf("RDF/XML not well-formed: %v\n%s", err, b)
	}
	out := string(b)
	for _, want := range []string{
		`<bf:Work rdf:about="#92005291Work">`,
		`<rdf:type rdf:resource="http://id.loc.gov/ontologies/bibframe/Text"/>`,
		`<bf:mainTitle>Stone butch blues (Uniform)</bf:mainTitle>`, // Work uses uniform title
		`<bflc:PrimaryContribution>`,
		`<bf:Person>`,
		`<rdfs:label>Feinberg, Leslie</rdfs:label>`,
		`<rdfs:label>author</rdfs:label>`,
		`<bf:Topic>`,
		`<rdfs:label>Lesbians--Fiction</rdfs:label>`,
		`<bf:Place>`,
		`<bf:GenreForm>`,
		`<bf:Language rdf:about="http://id.loc.gov/vocabulary/languages/eng">`,
		`<bf:ClassificationLcc>`,
		`<bf:classificationPortion>PS3556 .E446</bf:classificationPortion>`,
		`<bf:ClassificationDdc>`,
		`<bf:hasInstance rdf:resource="#92005291Instance"/>`,
		`<bf:Instance rdf:about="#92005291Instance">`,
		`<bf:instanceOf rdf:resource="#92005291Work"/>`,
		`<bf:mainTitle>Stone butch blues</bf:mainTitle>`, // Instance uses transcribed title
		`<bf:responsibilityStatement>Leslie Feinberg.</bf:responsibilityStatement>`,
		`<bf:editionStatement>First edition.</bf:editionStatement>`,
		`<bf:Publication>`,
		`<bf:date>1993</bf:date>`,
		`<bf:Isbn>`,
		`<rdf:value>0786803525</rdf:value>`,
		`<bf:Issn>`,
		`<bf:electronicLocator rdf:resource="https://example.org/item"/>`,
		`A novel about &lt;gender&gt; &amp; identity.`, // XML escaping
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RDF/XML missing %q", want)
		}
	}
}

// TestEncodeJSONLDWellFormed checks the JSON-LD output parses and carries the
// expected graph.
func TestEncodeJSONLDWellFormed(t *testing.T) {
	b, err := EncodeJSONLD(sample())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Context map[string]string `json:"@context"`
		Graph   []map[string]any  `json:"@graph"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("JSON-LD invalid: %v\n%s", err, b)
	}
	if doc.Context["bf"] != bfNS {
		t.Errorf("@context bf = %q", doc.Context["bf"])
	}
	if len(doc.Graph) != 2 {
		t.Fatalf("graph has %d nodes, want 2", len(doc.Graph))
	}
	work, inst := doc.Graph[0], doc.Graph[1]
	if work["@id"] != "#92005291Work" {
		t.Errorf("work @id = %v", work["@id"])
	}
	if types, ok := work["@type"].([]any); !ok || len(types) != 2 || types[1] != "bf:Text" {
		t.Errorf("work @type = %v", work["@type"])
	}
	if inst["@id"] != "#92005291Instance" {
		t.Errorf("instance @id = %v", inst["@id"])
	}
	if io, ok := inst["bf:instanceOf"].(map[string]any); !ok || io["@id"] != "#92005291Work" {
		t.Errorf("instanceOf = %v", inst["bf:instanceOf"])
	}
}

// TestFromRecord asserts the crosswalk mapping directly on the model.
func TestFromRecord(t *testing.T) {
	g := FromRecord(sample())
	if g.Work.Class != "Text" {
		t.Errorf("work class = %q", g.Work.Class)
	}
	if len(g.Work.Titles) != 1 || g.Work.Titles[0].Type != "uniform" {
		t.Errorf("work titles = %+v", g.Work.Titles)
	}
	if len(g.Instance.Titles) != 1 || g.Instance.Titles[0].MainTitle != "Stone butch blues" {
		t.Errorf("instance titles = %+v", g.Instance.Titles)
	}
	// 100 (primary person) + 700 (person) + 710 (organization).
	if n := len(g.Work.Contributions); n != 3 {
		t.Fatalf("contributions = %d, want 3", n)
	}
	if c := g.Work.Contributions[0]; !c.Primary || len(c.Roles) != 1 || c.Roles[0].Term != "author" || c.Roles[0].IRI != "" {
		t.Errorf("primary contribution = %+v", c)
	}
	if c := g.Work.Contributions[1]; c.Primary || len(c.Roles) != 1 || c.Roles[0].IRI != relatorVocab+"edt" {
		t.Errorf("added contribution = %+v", c)
	}
	if g.Work.Contributions[2].Class != "Organization" {
		t.Errorf("corporate contribution = %+v", g.Work.Contributions[2])
	}
	// 650 Topic, 651 Place, 600 Person, 610 Organization, 611 Meeting.
	if n := len(g.Work.Subjects); n != 5 {
		t.Errorf("subjects = %d (%+v)", n, g.Work.Subjects)
	}
	if len(g.Work.GenreForms) != 1 {
		t.Errorf("genreForms = %+v", g.Work.GenreForms)
	}
	if len(g.Work.Languages) != 2 || g.Work.Languages[0] != "eng" || g.Work.Languages[1] != "fre" {
		t.Errorf("languages = %v", g.Work.Languages)
	}
	if len(g.Work.Classifications) != 2 {
		t.Errorf("classifications = %+v", g.Work.Classifications)
	}
	if len(g.Instance.Identifiers) != 3 {
		t.Errorf("identifiers = %+v", g.Instance.Identifiers)
	}
	if p := g.Instance.Provision; p == nil || p.Date != "1993" || p.Publisher != "Firebrand Books" {
		t.Errorf("provision = %+v", p)
	}
	if len(g.Instance.ElectronicLocator) != 1 {
		t.Errorf("locator = %v", g.Instance.ElectronicLocator)
	}
}

// TestMinimalRecord checks an almost-empty record still yields a valid Work and
// Instance with relative node ids.
func TestMinimalRecord(t *testing.T) {
	rec := codex.NewRecord().SetLeader(codex.Leader("00000nxx a2200000 a 4500"))
	b, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := xmlWellFormed(b); err != nil {
		t.Fatalf("not well-formed: %v\n%s", err, b)
	}
	out := string(b)
	if !strings.Contains(out, `<bf:Work rdf:about="#r0Work">`) {
		t.Errorf("expected relative work id:\n%s", out)
	}
	if strings.Contains(out, "<rdf:type") {
		t.Errorf("type 'x' should yield no specific class:\n%s", out)
	}
	jb, _ := EncodeJSONLD(rec)
	var v any
	if err := json.Unmarshal(jb, &v); err != nil {
		t.Fatalf("minimal JSON-LD invalid: %v\n%s", err, jb)
	}
}

func TestWorkClass(t *testing.T) {
	cases := map[byte]string{
		'a': "Text", 't': "Text", 'c': "NotatedMusic", 'd': "NotatedMusic",
		'e': "Cartography", 'f': "Cartography", 'g': "MovingImage",
		'i': "Audio", 'j': "Audio", 'k': "StillImage", 'm': "Multimedia",
		'o': "MixedMaterial", 'p': "MixedMaterial", 'r': "Object", 'z': "",
	}
	for in, want := range cases {
		if got := workClass(in); got != want {
			t.Errorf("workClass(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeID(t *testing.T) {
	if got := sanitizeID("ocm 12345/x?"); got != "ocm12345x" {
		t.Errorf("sanitizeID = %q", got)
	}
	if got := sanitizeID("***"); got != "" {
		t.Errorf("sanitizeID(***) = %q, want empty", got)
	}
}

// TestCollectionWriters runs both collection writers through codex.Convert and
// checks the container is well-formed and ids are disambiguated per record.
func TestCollectionWriters(t *testing.T) {
	mrc, err := iso2709.Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	stream := append(append([]byte{}, mrc...), mrc...) // two records

	t.Run("rdfxml", func(t *testing.T) {
		var out bytes.Buffer
		w := NewWriter(&out)
		if err := codex.Convert(iso2709.NewReader(bytes.NewReader(stream)), w); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil { // idempotent
			t.Fatal(err)
		}
		if err := xmlWellFormed(out.Bytes()); err != nil {
			t.Fatalf("collection not well-formed: %v", err)
		}
		if n := strings.Count(out.String(), "<bf:Work "); n != 2 {
			t.Errorf("got %d works, want 2", n)
		}
	})

	t.Run("jsonld", func(t *testing.T) {
		var out bytes.Buffer
		w := NewJSONLDWriter(&out)
		if err := codex.Convert(iso2709.NewReader(bytes.NewReader(stream)), w); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Graph []map[string]any `json:"@graph"`
		}
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("collection JSON-LD invalid: %v\n%s", err, out.String())
		}
		if len(doc.Graph) != 4 { // 2 records x (Work+Instance)
			t.Errorf("graph has %d nodes, want 4", len(doc.Graph))
		}
	})
}

// TestWriteAfterClose verifies Write returns an error once closed.
func TestWriteAfterClose(t *testing.T) {
	var out bytes.Buffer
	w := NewWriter(&out)
	w.Close()
	if err := w.Write(sample()); !errors.Is(err, errWriteAfterClose) {
		t.Errorf("Write after Close = %v", err)
	}
	jw := NewJSONLDWriter(&out)
	jw.Close()
	if err := jw.Write(sample()); !errors.Is(err, errWriteAfterClose) {
		t.Errorf("JSON-LD Write after Close = %v", err)
	}
}

// failWriter fails the nth Write call (1-based) to exercise error propagation.
type failWriter struct {
	n, count int
}

func (f *failWriter) Write(p []byte) (int, error) {
	f.count++
	if f.count >= f.n {
		return 0, errors.New("boom")
	}
	return len(p), nil
}

func TestWriterErrorPaths(t *testing.T) {
	// Error on the header write (first call): both Write and Close surface it.
	for _, fail := range []int{1, 2, 3} {
		w := NewWriter(&failWriter{n: fail})
		_ = w.Write(sample())
		_ = w.Write(sample())
		if err := w.Close(); err == nil {
			t.Errorf("RDF/XML: expected error with fail=%d", fail)
		}
		// A subsequent Write keeps returning the stored error.
		if err := w.Write(sample()); err == nil {
			t.Errorf("RDF/XML: expected sticky error with fail=%d", fail)
		}

		jw := NewJSONLDWriter(&failWriter{n: fail})
		_ = jw.Write(sample())
		_ = jw.Write(sample())
		if err := jw.Close(); err == nil {
			t.Errorf("JSON-LD: expected error with fail=%d", fail)
		}
	}
	// Close-only path (no prior Write) with a failing writer.
	w := NewWriter(&failWriter{n: 1})
	if err := w.Close(); err == nil {
		t.Error("RDF/XML: expected error closing empty writer")
	}
	jw := NewJSONLDWriter(&failWriter{n: 1})
	if err := jw.Close(); err == nil {
		t.Error("JSON-LD: expected error closing empty writer")
	}
}

func TestWriteFiles(t *testing.T) {
	recs := []*codex.Record{sample(), sample()}
	for _, c := range []struct {
		ext   string
		write func(string, []*codex.Record) error
		check func([]byte) error
	}{
		{"rdf", WriteFile, xmlWellFormed},
		{"jsonld", WriteJSONLDFile, func(b []byte) error { var v any; return json.Unmarshal(b, &v) }},
	} {
		path := filepath.Join(t.TempDir(), "out."+c.ext)
		if err := c.write(path, recs); err != nil {
			t.Fatalf("%s: %v", c.ext, err)
		}
		b, _ := os.ReadFile(path)
		if err := c.check(b); err != nil {
			t.Errorf("%s: %v", c.ext, err)
		}
		// A path in a missing directory must error.
		if err := c.write(filepath.Join(t.TempDir(), "no-dir", "x"), recs); err == nil {
			t.Errorf("%s: expected error for bad path", c.ext)
		}
	}
}

// TestEscapingAndEdgeCases exercises the escaping branches, title part numbers,
// the 008-date fallback and contributions lacking a name.
func TestEscapingAndEdgeCases(t *testing.T) {
	// Values containing every class of character the escapers must handle.
	nasty := "say \"hi\"\n\tback\\slash & <tag> \r\x01\x7f bad\xffbyte"
	url := `https://example.org/a?x=1&y="2"<b>` + "\r\n\t\x02\xff"
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000cam a2200000 a 4500")).
		AddField(codex.NewControlField("008", "920219s1995    nyua   j      000 1 eng  ")).
		AddField(codex.NewDataField("100", '1', ' ', codex.NewSubfield('e', "no name here"))). // no $a -> dropped
		AddField(codex.NewDataField("245", '1', '0',
			codex.NewSubfield('a', nasty),
			codex.NewSubfield('n', "Part 1"),
			codex.NewSubfield('p', "The "+nasty))).
		AddField(codex.NewDataField("856", '4', '0', codex.NewSubfield('u', url)))

	g := FromRecord(rec)
	if len(g.Work.Contributions) != 0 {
		t.Errorf("a 100 without $a must yield no contribution: %+v", g.Work.Contributions)
	}
	// No 264, so the date comes from the 008 fallback.
	if g.Instance.Provision == nil || g.Instance.Provision.Date != "1995" {
		t.Errorf("008 date fallback = %+v", g.Instance.Provision)
	}
	if len(g.Instance.Titles) != 1 || g.Instance.Titles[0].PartNumber != "Part 1" {
		t.Errorf("part number not carried: %+v", g.Instance.Titles)
	}

	xb, _ := Encode(rec)
	if !utf8.Valid(xb) {
		t.Errorf("RDF/XML not valid UTF-8: %q", xb)
	}
	if err := xmlWellFormed(xb); err != nil {
		t.Fatalf("RDF/XML not well-formed: %v\n%s", err, xb)
	}
	jb, _ := EncodeJSONLD(rec)
	if !utf8.Valid(jb) {
		t.Errorf("JSON-LD not valid UTF-8: %q", jb)
	}
	var v any
	if err := json.Unmarshal(jb, &v); err != nil {
		t.Fatalf("JSON-LD invalid: %v\n%s", err, jb)
	}

	// date008 edge cases: a "0000" date and a too-short 008 both yield no date.
	for _, c008 := range []string{"920219s0000    nyua", "tooShort"} {
		r := codex.NewRecord().AddField(codex.NewControlField("008", c008))
		if d := FromRecord(r).Instance.Provision; d != nil {
			t.Errorf("008=%q should give no provision date, got %+v", c008, d)
		}
	}
}

// xmlWellFormed reports whether b is well-formed XML by tokenizing it fully.
func xmlWellFormed(b []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// TestGolden pins the exact serialized output for the rich sample record. Set
// UPDATE_GOLDEN=1 to regenerate the files after an intentional change.
func TestGolden(t *testing.T) {
	for _, c := range []struct {
		name string
		fn   func(*codex.Record) ([]byte, error)
	}{
		{"sample.rdf", Encode},
		{"sample.jsonld", EncodeJSONLD},
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

// FuzzFromMARC ensures the MARC->BIBFRAME paths never panic and always produce
// well-formed, valid-UTF-8 RDF/XML and JSON for any decodable record.
func FuzzFromMARC(f *testing.F) {
	mrc, _ := iso2709.Encode(sample())
	f.Add(mrc)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		rec, _, err := iso2709.Decode(data)
		if err != nil || rec == nil {
			return
		}
		xb, _ := Encode(rec)
		if !utf8.Valid(xb) {
			t.Errorf("RDF/XML not valid UTF-8: %q", xb)
		}
		if err := xmlWellFormed(xb); err != nil {
			t.Errorf("RDF/XML not well-formed: %v\n%q", err, xb)
		}
		jb, _ := EncodeJSONLD(rec)
		var v any
		if err := json.Unmarshal(jb, &v); err != nil {
			t.Errorf("JSON-LD invalid: %v\n%q", err, jb)
		}
	})
}

// TestMinimalOmitsEmpty checks that a record with no optional fields emits none of
// the empty array wrappers — pinning the "omit empty" branches in both serializers.
func TestMinimalOmitsEmpty(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "min1"))
	jb, _ := EncodeJSONLD(rec)
	xb, _ := Encode(rec)
	for _, empty := range []string{
		`"bf:title":[]`, `"bf:contribution":[]`, `"bf:subject":[]`, `"bf:language":[]`,
		`"bf:classification":[]`, `"bf:identifiedBy":[]`, `"bf:genreForm":[]`,
		`"bf:summary":[]`, `"bf:extent":[]`, `"bf:electronicLocator":[]`,
	} {
		if strings.Contains(string(jb), empty) {
			t.Errorf("JSON-LD emits empty wrapper %s:\n%s", empty, jb)
		}
	}
	if err := xmlWellFormed(xb); err != nil {
		t.Errorf("RDF/XML not well-formed: %v", err)
	}
	var v any
	if err := json.Unmarshal(jb, &v); err != nil {
		t.Errorf("JSON-LD invalid: %v", err)
	}
}
