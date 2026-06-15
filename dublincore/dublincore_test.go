package dublincore

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/iso2709"
)

func sample() *codex.Record {
	return codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewControlField("008", "920219s1993    nyua   j      000 1 eng  ")).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "0786803525"))).
		AddField(codex.NewDataField("100", '1', ' ', codex.NewSubfield('a', "Feinberg, Leslie,"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Stone butch blues :"), codex.NewSubfield('b', "a novel /"))).
		AddField(codex.NewDataField("260", ' ', ' ', codex.NewSubfield('b', "Firebrand Books,"), codex.NewSubfield('c', "1993"))).
		AddField(codex.NewDataField("300", ' ', ' ', codex.NewSubfield('a', "301 pages ;"), codex.NewSubfield('c', "22 cm"))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Lesbians"), codex.NewSubfield('v', "Fiction"))).
		AddField(codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Editor, An,"))).
		AddField(codex.NewDataField("520", ' ', ' ', codex.NewSubfield('a', "A novel about <gender> & identity")))
}

func wellFormedXML(t *testing.T, b []byte) {
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

func TestEncodeXML(t *testing.T) {
	b, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	wellFormedXML(t, b)
	out := string(b)
	for _, want := range []string{
		`xmlns:dc="` + dcNamespace + `"`,
		"<dc:title>Stone butch blues a novel</dc:title>",
		"<dc:creator>Feinberg, Leslie</dc:creator>",
		"<dc:contributor>Editor, An</dc:contributor>",
		"<dc:subject>Lesbians--Fiction</dc:subject>",
		"<dc:publisher>Firebrand Books</dc:publisher>",
		"<dc:type>Text</dc:type>",
		"<dc:language>eng</dc:language>",
		"&lt;gender&gt; &amp; identity",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("XML missing %q:\n%s", want, out)
		}
	}
}

func TestEncodeJSON(t *testing.T) {
	b, err := EncodeJSON(sample())
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Fatalf("invalid JSON:\n%s", b)
	}
	var m map[string][]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if got := m["title"]; len(got) != 1 || got[0] != "Stone butch blues a novel" {
		t.Errorf("title = %v", got)
	}
	if got := m["subject"]; len(got) != 1 || got[0] != "Lesbians--Fiction" {
		t.Errorf("subject = %v", got)
	}
	if got := m["description"]; len(got) != 1 || got[0] != "A novel about <gender> & identity" {
		t.Errorf("description = %v", got)
	}
	if _, ok := m["source"]; ok {
		t.Error("empty element 'source' should be omitted")
	}
}

func TestConvertFromISO2709(t *testing.T) {
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
	wellFormedXML(t, out.Bytes())
	if !strings.Contains(out.String(), collectionOpen) {
		t.Error("missing collection wrapper")
	}
}

func TestJSONWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewJSONWriter(&buf)
	if err := w.Write(sample()); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(codex.NewRecord().AddField(codex.NewDataField("245", '0', '0', codex.NewSubfield('a', "Second")))); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("JSON array invalid:\n%s", buf.Bytes())
	}
	var arr []map[string][]string
	if err := json.Unmarshal(buf.Bytes(), &arr); err != nil {
		t.Fatal(err)
	}
	if len(arr) != 2 {
		t.Errorf("array length = %d, want 2", len(arr))
	}
}

func TestWriteAfterClose(t *testing.T) {
	w := NewWriter(&bytes.Buffer{})
	w.Close()
	if err := w.Write(sample()); err == nil {
		t.Error("Writer: expected error after Close")
	}
	jw := NewJSONWriter(&bytes.Buffer{})
	jw.Close()
	if err := jw.Write(sample()); err == nil {
		t.Error("JSONWriter: expected error after Close")
	}
}

func TestEmptyRecord(t *testing.T) {
	b, err := Encode(codex.NewRecord())
	if err != nil {
		t.Fatal(err)
	}
	wellFormedXML(t, b)
	j, _ := EncodeJSON(codex.NewRecord())
	if !json.Valid(j) {
		t.Errorf("empty record JSON invalid: %s", j)
	}
}

func TestHelpers(t *testing.T) {
	for in, want := range map[byte]string{
		'g': "MovingImage", 'r': "PhysicalObject", 'a': "Text", 'j': "Sound",
		'e': "Image", 'k': "Image", 'i': "Sound", 'm': "Software", 'o': "Collection",
	} {
		if got := dcType(in); got != want {
			t.Errorf("dcType(%c) = %q, want %q", in, got, want)
		}
	}
	if got := trimISBD("Title /"); got != "Title" {
		t.Errorf("trimISBD = %q", got)
	}
}

func TestMappingExtras(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("008", "                                   fre  ")).
		AddField(codex.NewDataField("110", '2', ' ', codex.NewSubfield('a', "Acme Corp."))).
		AddField(codex.NewDataField("710", '2', ' ', codex.NewSubfield('a', "Helper Org."))).
		AddField(codex.NewDataField("041", ' ', ' ', codex.NewSubfield('a', "engger"))).
		AddField(codex.NewDataField("506", ' ', ' ', codex.NewSubfield('a', "Open access"))).
		AddField(codex.NewDataField("856", '4', '0', codex.NewSubfield('u', "https://example.org/x")))

	dc := FromRecord(rec)
	if len(dc.Creator) != 1 || dc.Creator[0] != "Acme Corp." {
		t.Errorf("creator = %v", dc.Creator)
	}
	if len(dc.Contributor) != 1 {
		t.Errorf("contributor = %v", dc.Contributor)
	}
	// languages: 008 'fre' first, then 041 'eng','ger'.
	if want := []string{"fre", "eng", "ger"}; !reflect.DeepEqual(dc.Language, want) {
		t.Errorf("language = %v, want %v", dc.Language, want)
	}
	if len(dc.Rights) != 1 || dc.Rights[0] != "Open access" {
		t.Errorf("rights = %v", dc.Rights)
	}
	if !slices.Contains(dc.Identifier, "https://example.org/x") {
		t.Errorf("identifier = %v", dc.Identifier)
	}
}

// FuzzFromMARC ensures any decodable MARC record converts to well-formed XML and
// valid JSON without panicking.
func FuzzFromMARC(f *testing.F) {
	mrc, _ := iso2709.Encode(sample())
	f.Add(mrc)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		rec, _, err := iso2709.Decode(data)
		if err != nil || rec == nil {
			return
		}
		x, _ := Encode(rec)
		wellFormedXML(t, x)
		j, _ := EncodeJSON(rec)
		if !json.Valid(j) {
			t.Errorf("invalid JSON for record:\n%s", j)
		}
	})
}

func TestGolden(t *testing.T) {
	recs := []*codex.Record{sample()}
	for _, g := range []struct {
		name  string
		write func(io.Writer) codex.RecordWriter
	}{
		{"sample.oai_dc.xml", func(w io.Writer) codex.RecordWriter { return NewWriter(w) }},
		{"sample.dc.json", func(w io.Writer) codex.RecordWriter { return NewJSONWriter(w) }},
	} {
		t.Run(g.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := g.write(&buf)
			for _, rec := range recs {
				if err := w.Write(rec); err != nil {
					t.Fatal(err)
				}
			}
			if c, ok := w.(interface{ Close() error }); ok {
				if err := c.Close(); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join("testdata", g.name)
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
				t.Errorf("differs from %s:\n%s", path, buf.Bytes())
			}
		})
	}
}
