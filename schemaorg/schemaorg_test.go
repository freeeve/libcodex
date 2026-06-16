package schemaorg

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/iso2709"
)

func sample() *codex.Record {
	raw := []byte("920219s1993    nyua   j      000 1 eng  ")
	raw[23] = 'd' // large print
	return codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewControlField("008", string(raw))).
		AddField(codex.NewControlField("007", "fb")). // tactile / braille
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "0786803525"))).
		AddField(codex.NewDataField("041", ' ', ' ', codex.NewSubfield('a', "engfre"))).
		AddField(codex.NewDataField("100", '1', ' ', codex.NewSubfield('a', "Feinberg, Leslie,"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Stone butch blues :"), codex.NewSubfield('b', "a novel /"))).
		AddField(codex.NewDataField("250", ' ', ' ', codex.NewSubfield('a', "First edition."))).
		AddField(codex.NewDataField("264", ' ', '1', codex.NewSubfield('b', "Firebrand Books,"), codex.NewSubfield('c', "[1993]"))).
		AddField(codex.NewDataField("520", ' ', ' ', codex.NewSubfield('a', "A novel <of> & identity."))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Lesbians"), codex.NewSubfield('v', "Fiction"))).
		AddField(codex.NewDataField("655", ' ', '7', codex.NewSubfield('a', "Bildungsromans."))).
		AddField(codex.NewDataField("700", '1', ' ', codex.NewSubfield('a', "Editor, An,"))).
		AddField(codex.NewDataField("710", '2', ' ', codex.NewSubfield('a', "A Corporate Body"))).
		AddField(codex.NewDataField("856", '4', '0', codex.NewSubfield('u', "https://example.org/item"))).
		AddField(codex.NewDataField("341", '0', ' ', codex.NewSubfield('a', "textual"), codex.NewSubfield('a', "visual"))).
		AddField(codex.NewDataField("532", '1', ' ', codex.NewSubfield('a', "Text resized to 18 point.")))
}

func TestEncodeWellFormed(t *testing.T) {
	b, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, b)
	}
	if m["@context"] != "https://schema.org" || m["@type"] != "Book" {
		t.Errorf("context/type = %v / %v", m["@context"], m["@type"])
	}
	if m["name"] != "Stone butch blues a novel" {
		t.Errorf("name = %v", m["name"])
	}
	if author, ok := m["author"].(map[string]any); !ok || author["name"] != "Feinberg, Leslie" {
		t.Errorf("author = %v", m["author"])
	}
	if pub, ok := m["publisher"].(map[string]any); !ok || pub["@type"] != "Organization" {
		t.Errorf("publisher = %v", m["publisher"])
	}
	if m["datePublished"] != "1993" || m["isbn"] != "0786803525" {
		t.Errorf("date/isbn = %v / %v", m["datePublished"], m["isbn"])
	}
	// & < > in the description must be valid JSON (no XML-style escaping needed).
	if m["description"] != "A novel <of> & identity." {
		t.Errorf("description = %v", m["description"])
	}
}

func TestFromRecord(t *testing.T) {
	b := FromRecord(sample())
	if b.Type != "Book" {
		t.Errorf("type = %q", b.Type)
	}
	if len(b.Authors) != 1 || b.Authors[0].Type != "Person" {
		t.Errorf("authors = %+v", b.Authors)
	}
	// 700 (Person) + 710 (Organization).
	if len(b.Contributors) != 2 || b.Contributors[1].Type != "Organization" {
		t.Errorf("contributors = %+v", b.Contributors)
	}
	if len(b.InLanguage) != 2 || b.InLanguage[0] != "en" || b.InLanguage[1] != "fr" {
		t.Errorf("inLanguage = %v (want en, fr)", b.InLanguage)
	}
	if b.Edition != "First edition." {
		t.Errorf("edition = %q", b.Edition)
	}
	if len(b.Genre) != 1 || b.Genre[0] != "Bildungsromans." {
		t.Errorf("genre = %v", b.Genre)
	}
}

func TestSchemaType(t *testing.T) {
	cases := map[byte]string{
		'a': "Book", 't': "Book", 'c': "MusicComposition", 'e': "Map",
		'g': "Movie", 'i': "AudioObject", 'j': "MusicRecording",
		'k': "ImageObject", 'm': "SoftwareApplication", 'z': "CreativeWork",
	}
	for rt, want := range cases {
		if got := schemaType(rt); got != want {
			t.Errorf("schemaType(%q) = %q, want %q", rt, got, want)
		}
	}
}

func TestAccessibilityMapping(t *testing.T) {
	b := FromRecord(sample())
	if !slices.Contains(b.AccessibilityFeature, "largePrint") || !slices.Contains(b.AccessibilityFeature, "brailleViaTouch") {
		t.Errorf("accessibilityFeature = %v", b.AccessibilityFeature)
	}
	if !slices.Contains(b.AccessMode, "textual") || !slices.Contains(b.AccessMode, "visual") {
		t.Errorf("accessMode = %v", b.AccessMode)
	}
	if b.AccessibilitySummary != "Text resized to 18 point." {
		t.Errorf("summary = %q", b.AccessibilitySummary)
	}
}

func TestWriterCollection(t *testing.T) {
	mrc, err := iso2709.Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	stream := append(append([]byte{}, mrc...), mrc...)
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
	var arr []map[string]any
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("invalid JSON array: %v\n%s", err, out.String())
	}
	if len(arr) != 2 {
		t.Errorf("got %d objects, want 2", len(arr))
	}
	if err := w.Write(sample()); !errors.Is(err, errWriteAfterClose) {
		t.Errorf("Write after Close = %v", err)
	}
}

// failWriter fails the nth Write call (1-based).
type failWriter struct{ n, count int }

func (f *failWriter) Write(p []byte) (int, error) {
	f.count++
	if f.count >= f.n {
		return 0, errors.New("boom")
	}
	return len(p), nil
}

func TestWriterErrorPaths(t *testing.T) {
	for _, fail := range []int{1, 2, 3} {
		w := NewWriter(&failWriter{n: fail})
		_ = w.Write(sample())
		_ = w.Write(sample())
		if err := w.Close(); err == nil {
			t.Errorf("expected error with fail=%d", fail)
		}
		if err := w.Write(sample()); err == nil {
			t.Errorf("expected sticky error with fail=%d", fail)
		}
	}
	// Close-only with a failing writer.
	if err := NewWriter(&failWriter{n: 1}).Close(); err == nil {
		t.Error("expected error closing empty writer")
	}
}

func TestWriteFile(t *testing.T) {
	recs := []*codex.Record{sample(), sample()}
	path := filepath.Join(t.TempDir(), "out.jsonld")
	if err := WriteFile(path, recs); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	b, _ := os.ReadFile(path)
	var arr []any
	if err := json.Unmarshal(b, &arr); err != nil {
		t.Errorf("file not valid JSON: %v", err)
	}
	if err := WriteFile(filepath.Join(t.TempDir(), "no-dir", "x"), recs); err == nil {
		t.Error("expected error for bad path")
	}
}

func TestEscapingAndInvalidUTF8(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "say \"hi\"\n\tx\\y bad\xffbyte")))
	b, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(b) {
		t.Errorf("output not valid UTF-8: %q", b)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, b)
	}
}

func TestEdgeCases(t *testing.T) {
	// A name field with no $a yields no agent; an unknown language falls back to
	// its 3-letter code; a carriage return is escaped; the minimal record is just
	// a typed object.
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewDataField("100", '1', ' ', codex.NewSubfield('e', "author"))). // no $a
		AddField(codex.NewDataField("041", ' ', ' ', codex.NewSubfield('a', "xyz"))).    // unknown code
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Line one\rtwo")))
	b := FromRecord(rec)
	if len(b.Authors) != 0 {
		t.Errorf("a 100 without $a must yield no author: %+v", b.Authors)
	}
	if len(b.InLanguage) != 1 || b.InLanguage[0] != "xyz" {
		t.Errorf("unknown language should pass through: %v", b.InLanguage)
	}
	out, _ := Encode(rec)
	if !utf8.Valid(out) {
		t.Error("not valid UTF-8")
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	// A record whose leader type maps to nothing specific becomes a CreativeWork.
	cw, _ := Encode(codex.NewRecord().SetLeader(codex.Leader("00000nzm a2200000 a 4500")))
	var m2 map[string]any
	if err := json.Unmarshal(cw, &m2); err != nil {
		t.Fatalf("CreativeWork JSON invalid: %v\n%s", err, cw)
	}
	if m2["@type"] != "CreativeWork" {
		t.Errorf("unmapped leader @type = %v", m2["@type"])
	}
}

func TestGolden(t *testing.T) {
	b, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "sample.jsonld")
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
}

// FuzzFromMARC ensures the MARC->schema.org path never panics and produces valid,
// valid-UTF-8 JSON for any decodable record.
func FuzzFromMARC(f *testing.F) {
	mrc, _ := iso2709.Encode(sample())
	f.Add(mrc)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		rec, _, err := iso2709.Decode(data)
		if err != nil || rec == nil {
			return
		}
		b, _ := Encode(rec)
		if !utf8.Valid(b) {
			t.Errorf("output not valid UTF-8: %q", b)
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Errorf("invalid JSON: %v\n%q", err, b)
		}
	})
}

var _ io.Writer = (*failWriter)(nil)
