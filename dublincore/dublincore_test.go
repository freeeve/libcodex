package dublincore

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/crosswalk"
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
	if got := crosswalk.TrimISBD("Title /"); got != "Title" {
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
// failAfterWriter succeeds for n Write calls then permanently errors.
type failAfterWriter struct {
	n    int
	done int
}

func (w *failAfterWriter) Write(b []byte) (int, error) {
	if w.done >= w.n {
		return 0, errors.New("injected write error")
	}
	w.done++
	return len(b), nil
}

// TestWriteFile covers WriteFile: success path and os.Create error branch.
func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.xml")
	recs := []*codex.Record{sample()}

	if err := WriteFile(path, recs); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("WriteFile wrote an empty file")
	}
	wellFormedXML(t, data)

	// Cover the os.Create error branch (directory does not exist).
	badPath := filepath.Join(dir, "no-such-dir", "out.xml")
	if err := WriteFile(badPath, recs); err == nil {
		t.Error("WriteFile: expected error for non-existent directory")
	}
}

// TestWriterErrorPaths exercises sticky errors, write failures, and close
// idempotency for Writer.
func TestWriterErrorPaths(t *testing.T) {
	t.Run("header_fail_then_sticky", func(t *testing.T) {
		fw := &failAfterWriter{n: 0}
		w := NewWriter(fw)
		// First Write triggers header flush which fails immediately.
		if err := w.Write(sample()); err == nil {
			t.Fatal("expected error from header write")
		}
		// Subsequent Write returns the stored (sticky) error.
		if err := w.Write(sample()); err == nil {
			t.Fatal("expected sticky error on second Write")
		}
		// Close also returns the sticky error.
		if err := w.Close(); err == nil {
			t.Fatal("expected sticky error from Close after failed Write")
		}
	})

	t.Run("record_write_fail", func(t *testing.T) {
		// n=1: header write succeeds; record-body write fails.
		fw := &failAfterWriter{n: 1}
		w := NewWriter(fw)
		if err := w.Write(sample()); err == nil {
			t.Fatal("expected error writing record body")
		}
		// Sticky on next call.
		if err := w.Write(sample()); err == nil {
			t.Fatal("expected sticky error on second Write")
		}
	})

	t.Run("close_header_fail", func(t *testing.T) {
		// Close on an unopened writer: the header write fails.
		fw := &failAfterWriter{n: 0}
		w := NewWriter(fw)
		if err := w.Close(); err == nil {
			t.Fatal("expected error from Close header write")
		}
	})

	t.Run("close_closing_tag_fail", func(t *testing.T) {
		// n=1: header write succeeds; closing-tag write fails.
		fw := &failAfterWriter{n: 1}
		w := NewWriter(fw)
		if err := w.Close(); err == nil {
			t.Fatal("expected error writing closing tag in Close")
		}
	})

	t.Run("close_after_close_ok", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewWriter(&buf)
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		// Second Close must be idempotent (return nil).
		if err := w.Close(); err != nil {
			t.Fatalf("second Close returned error: %v", err)
		}
	})
}

// TestJSONWriterErrorPaths mirrors TestWriterErrorPaths for JSONWriter.
func TestJSONWriterErrorPaths(t *testing.T) {
	t.Run("header_fail_then_sticky", func(t *testing.T) {
		fw := &failAfterWriter{n: 0}
		w := NewJSONWriter(fw)
		if err := w.Write(sample()); err == nil {
			t.Fatal("expected error from header write")
		}
		if err := w.Write(sample()); err == nil {
			t.Fatal("expected sticky error on second Write")
		}
		if err := w.Close(); err == nil {
			t.Fatal("expected sticky error from Close after failed Write")
		}
	})

	t.Run("record_write_fail", func(t *testing.T) {
		fw := &failAfterWriter{n: 1}
		w := NewJSONWriter(fw)
		if err := w.Write(sample()); err == nil {
			t.Fatal("expected error writing record body")
		}
	})

	t.Run("close_header_fail", func(t *testing.T) {
		fw := &failAfterWriter{n: 0}
		w := NewJSONWriter(fw)
		if err := w.Close(); err == nil {
			t.Fatal("expected error from Close header write")
		}
	})

	t.Run("close_closing_tag_fail", func(t *testing.T) {
		fw := &failAfterWriter{n: 1}
		w := NewJSONWriter(fw)
		if err := w.Close(); err == nil {
			t.Fatal("expected error writing closing tag in Close")
		}
	})

	t.Run("close_after_close_ok", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewJSONWriter(&buf)
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("second Close returned error: %v", err)
		}
	})
}

// TestXMLEscaping covers the remaining appendXMLText branches: CR (→ &#xD;),
// control-char drop, valid multi-byte UTF-8 passthrough, and invalid-UTF-8 drop.
func TestXMLEscaping(t *testing.T) {
	// \r → &#xD;   \x1e (RS, < 0x20, not tab/newline) → dropped
	// \xc3\xa9 = é (valid 2-byte UTF-8) → kept as-is
	// \xff (invalid UTF-8 byte) → dropped
	special := "tab:\there\nnewline CR:\r ctrl:\x1e caf\xc3\xa9 inv:\xffy"
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewDataField("520", ' ', ' ', codex.NewSubfield('a', special)))

	b, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	wellFormedXML(t, b)
	if !utf8.Valid(b) {
		t.Error("encoded XML is not valid UTF-8")
	}
	out := string(b)
	if !strings.Contains(out, "&#xD;") {
		t.Errorf("expected &#xD; for CR; got:\n%s", out)
	}
	if strings.Contains(out, "\x1e") {
		t.Error("control char \\x1e should be dropped from XML")
	}
	if strings.Contains(out, "\xff") {
		t.Error("invalid UTF-8 byte \\xff should be dropped from XML")
	}
	if !strings.Contains(out, "caf\xc3\xa9") {
		t.Errorf("valid UTF-8 é should be preserved in XML; got:\n%s", out)
	}
	if !strings.Contains(out, "\t") {
		t.Error("tab character should be preserved in XML")
	}
}

// TestJSONEscaping covers the remaining appendJSONString branches: quote,
// backslash, newline, tab, CR, control-char \u-escape, valid multi-byte UTF-8
// passthrough, and invalid-UTF-8 drop.
func TestJSONEscaping(t *testing.T) {
	// All special JSON characters plus valid and invalid multi-byte UTF-8.
	special := "q:\"bs:\\nl:\ntab:\tcr:\rctrl:\x01caf\xc3\xa9inv:\xffy"
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewDataField("520", ' ', ' ', codex.NewSubfield('a', special)))

	b, err := EncodeJSON(rec)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Fatalf("JSON invalid:\n%s", b)
	}
	if !utf8.Valid(b) {
		t.Error("encoded JSON is not valid UTF-8")
	}
	out := string(b)
	if !strings.Contains(out, `\"`) {
		t.Errorf("expected \\\" in JSON; got:\n%s", out)
	}
	if !strings.Contains(out, `\\`) {
		t.Errorf("expected \\\\ in JSON; got:\n%s", out)
	}
	if !strings.Contains(out, `\n`) {
		t.Errorf("expected \\n in JSON; got:\n%s", out)
	}
	if !strings.Contains(out, `\t`) {
		t.Errorf("expected \\t in JSON; got:\n%s", out)
	}
	if !strings.Contains(out, `\r`) {
		t.Errorf("expected \\r in JSON; got:\n%s", out)
	}
	if !strings.Contains(out, "\\u0001") {
		t.Errorf("expected \\u0001 for control char; got:\n%s", out)
	}
	if strings.Contains(out, "\xff") {
		t.Error("invalid UTF-8 byte \\xff should be dropped from JSON")
	}
	if !strings.Contains(out, "caf\xc3\xa9") {
		t.Errorf("valid UTF-8 é should be preserved in JSON; got:\n%s", out)
	}
}

// TestJSONMultiValueArray verifies the comma separator between array elements
// in appendJSON (covers the i > 0 branch).
func TestJSONMultiValueArray(t *testing.T) {
	// Two ISBN fields produce two identifiers, forcing appendJSON to emit
	// a comma between the first and second array elements.
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "9780786803521"))).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "9780786803538")))

	b, err := EncodeJSON(rec)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Fatalf("JSON invalid:\n%s", b)
	}
	var m map[string][]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if len(m["identifier"]) != 2 {
		t.Errorf("identifier = %v, want 2 values", m["identifier"])
	}
}

// TestWriteAllStickyError exercises the defensive sticky-error guard at the top
// of writeAll (unreachable from the public Write/Close API because those methods
// check wr.err first; accessible here because the test is in package dublincore).
func TestWriteAllStickyError(t *testing.T) {
	sentinel := errors.New("pre-set error")

	// Writer.writeAll with err already set.
	{
		w := NewWriter(&bytes.Buffer{})
		w.err = sentinel
		if got := w.writeAll([]byte("data")); got != sentinel {
			t.Errorf("Writer.writeAll sticky: got %v, want sentinel", got)
		}
	}

	// JSONWriter.writeAll with err already set.
	{
		w := NewJSONWriter(&bytes.Buffer{})
		w.err = sentinel
		if got := w.writeAll([]byte("data")); got != sentinel {
			t.Errorf("JSONWriter.writeAll sticky: got %v, want sentinel", got)
		}
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
			if err := codex.Close(w); err != nil {
				t.Fatal(err)
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
