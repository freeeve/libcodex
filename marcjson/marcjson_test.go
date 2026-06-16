package marcjson

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/marcxml"
)

// sample exercises control fields, repeated data fields, a blank indicator, and
// values needing JSON escaping (tab, quote, backslash, newline) plus UTF-8.
func sample() *codex.Record {
	return codex.NewRecord().
		AddField(codex.NewControlField("001", "ocm12345")).
		AddField(codex.NewControlField("008", "210101s2021    nyu")).
		AddField(codex.NewDataField("245", '1', '0',
			codex.NewSubfield('a', "Stone butch blues :"),
			codex.NewSubfield('b', "a novel /"),
			codex.NewSubfield('c', "Leslie Feinberg."))).
		AddField(codex.NewDataField("520", ' ', ' ',
			codex.NewSubfield('a', "tab\there & \"quotes\" \\ <ok>\nnext"))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Café—Lesbians"))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Gender identity")))
}

func readAll(t *testing.T, r *Reader) []*codex.Record {
	t.Helper()
	var out []*codex.Record
	for rec, err := range r.All() {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		out = append(out, rec)
	}
	return out
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	rec := sample()
	b, err := Encode(rec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(rec, got) {
		t.Errorf("round trip mismatch:\n in  = %#v\n out = %#v", rec, got)
	}
}

func TestEscaping(t *testing.T) {
	b, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, want := range []string{`\t`, `\"`, `\\`, `\n`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing escape %q:\n%s", want, out)
		}
	}
	// & and < are valid raw in JSON strings and must NOT be escaped.
	if !strings.Contains(out, "here & \\\"quotes\\\" \\\\ <ok>") {
		t.Errorf("unexpected escaping of & or <:\n%s", out)
	}
}

func TestCrossFormatISO2709(t *testing.T) {
	mrc, err := iso2709.Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	fromMRC, _, err := iso2709.Decode(mrc)
	if err != nil {
		t.Fatal(err)
	}
	jsonb, err := Encode(fromMRC)
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := Decode(jsonb)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromMRC, fromJSON) {
		t.Errorf("iso2709 record not preserved through marcjson")
	}
	back, err := iso2709.Encode(fromJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mrc, back) {
		t.Error("iso2709 -> marcjson -> iso2709 is not byte-stable")
	}
}

func TestCrossFormatMARCXML(t *testing.T) {
	// marcxml and marcjson must agree on the model.
	xmlb, err := marcxml.Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	fromXML, err := marcxml.Decode(xmlb)
	if err != nil {
		t.Fatal(err)
	}
	jsonb, err := Encode(fromXML)
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := Decode(jsonb)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromXML, fromJSON) {
		t.Errorf("marcxml and marcjson disagree on the model")
	}
}

func TestReaderInputShapes(t *testing.T) {
	one, _ := Encode(sample())
	two, _ := Encode(codex.NewRecord().AddField(codex.NewControlField("001", "two")))

	cases := map[string]struct {
		in   string
		want int
	}{
		"single object": {string(one), 1},
		"object stream": {string(one) + "\n" + string(two), 2},
		"json array":    {"[\n" + string(one) + ",\n" + string(two) + "\n]", 2},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			recs := readAll(t, NewReader(strings.NewReader(c.in)))
			if len(recs) != c.want {
				t.Fatalf("read %d records, want %d", len(recs), c.want)
			}
			if recs[0].SubfieldValue("245", 'a') == "" && recs[0].ControlField("001") == "" {
				t.Error("first record empty")
			}
		})
	}
}

func TestWriterArrayRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(sample()); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(codex.NewRecord().AddField(codex.NewControlField("001", "two"))); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	out := buf.Bytes()
	if !bytes.HasPrefix(out, []byte("[\n")) || !bytes.HasSuffix(out, []byte("\n]\n")) {
		t.Errorf("missing array wrapper:\n%s", out)
	}
	recs := readAll(t, NewReader(bytes.NewReader(out)))
	if len(recs) != 2 {
		t.Fatalf("read %d records, want 2", len(recs))
	}
	if recs[1].ControlField("001") != "two" {
		t.Errorf("record 1 001 = %q", recs[1].ControlField("001"))
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

func TestDecodeSkipsUnknownKeys(t *testing.T) {
	in := `{"leader":"00000nam a2200000   4500","x":123,"fields":[
		{"001":"a","extra":"ignored"},
		{"245":{"ind1":"1","ind2":"0","note":[1,2],"subfields":[{"a":"T"}]}}
	],"trailing":{"nested":true}}`
	rec, err := Decode([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if rec.ControlField("001") != "a" {
		t.Errorf("001 = %q", rec.ControlField("001"))
	}
	if rec.SubfieldValue("245", 'a') != "T" {
		t.Errorf("245a = %q", rec.SubfieldValue("245", 'a'))
	}
}

func TestDecodeErrors(t *testing.T) {
	for name, in := range map[string]string{
		"empty":               ``,
		"not json":            `not json`,
		"empty array":         `[]`,
		"bad field value":     `{"fields":[{"001":123}]}`,
		"truncated":           `{"leader":"x","fields":[`,
		"wrong toplvl":        `"just a string"`,
		"leader not string":   `{"leader":123}`,
		"fields not array":    `{"fields":"x"}`,
		"field not object":    `{"fields":["x"]}`,
		"ind not string":      `{"fields":[{"245":{"ind1":1,"subfields":[]}}]}`,
		"subfields not array": `{"fields":[{"245":{"subfields":"x"}}]}`,
		"subfield not object": `{"fields":[{"245":{"subfields":["x"]}}]}`,
		"subfield bad value":  `{"fields":[{"245":{"subfields":[{"a":1}]}}]}`,
		"trunc after key":     `{"leader":`,
		"trunc after field":   `{"fields":[{`,
		"trunc in datafield":  `{"fields":[{"245":{"ind1":`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(in)); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// FuzzDecode ensures decoding never panics on arbitrary input and that a record
// that decodes can be re-encoded and decoded again.
// FuzzFromMARC ensures the MARC->marcjson path produces valid, re-decodable JSON
// (or a clean error) and never invalid output or a panic.
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
			return // not valid UTF-8
		}
		if _, err := Decode(b); err != nil {
			t.Errorf("re-decode of MARC->marcjson output failed: %v\n%s", err, b)
		}
	})
}

func FuzzDecode(f *testing.F) {
	b, _ := Encode(sample())
	f.Add(b)
	f.Add([]byte(`{"leader":"x","fields":[]}`))
	f.Add([]byte(`[{"001":"a"},{"245":{"ind1":"1","ind2":"0","subfields":[{"a":"t"}]}}]`))
	f.Add([]byte(``))
	f.Fuzz(func(t *testing.T, data []byte) {
		rec, err := Decode(data)
		if err != nil || rec == nil {
			return
		}
		out, err := Encode(rec)
		if err != nil {
			t.Fatalf("re-encode failed: %v", err)
		}
		rec2, err := Decode(out)
		if err != nil {
			t.Fatalf("re-decode of encoded record failed: %v", err)
		}
		// JSON represents every character, so round-trips are stable for records
		// whose tag-based classification matches their attributes (see comment).
		if selfConsistent(rec) && !reflect.DeepEqual(rec, rec2) {
			t.Errorf("round-trip not stable:\n a = %#v\n b = %#v", rec, rec2)
		}
	})
}

// selfConsistent reports whether every field's tag-based control/data
// classification matches the attributes it carries, so the record round-trips. A
// control field must have zero indicators and no subfields; a data field must
// have non-zero (set) indicators (an unset one serializes as a blank space).
func selfConsistent(rec *codex.Record) bool {
	for _, f := range rec.Fields() {
		if f.IsControl() {
			if f.Ind1 != 0 || f.Ind2 != 0 || len(f.Subfields) > 0 {
				return false
			}
		} else if f.Ind1 == 0 || f.Ind2 == 0 {
			return false
		}
	}
	return true
}

func TestReadWriteFile(t *testing.T) {
	recs := []*codex.Record{sample(), codex.NewRecord().AddField(codex.NewControlField("001", "x"))}
	path := filepath.Join(t.TempDir(), "c.json")
	if err := WriteFile(path, recs); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d records, want 2", len(got))
	}
	if v := got[0].SubfieldValue("650", 'a'); v != "Café—Lesbians" {
		t.Errorf("UTF-8 not preserved: %q", v)
	}
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestEncodeDecodeHelpers(t *testing.T) {
	if got := indStr(0); got != " " {
		t.Errorf("indStr(0) = %q, want space", got)
	}
	if got := indStr('1'); got != "1" {
		t.Errorf("indStr('1') = %q", got)
	}
	if got := indByte(""); got != ' ' {
		t.Errorf("indByte(empty) = %q", got)
	}
	if got := codeByte(""); got != 0 {
		t.Errorf("codeByte(empty) = %d", got)
	}
	// Control character below 0x20 escapes to \u00XX.
	if got := string(appendString(nil, "a\x01b")); got != `"a\u0001b"` {
		t.Errorf("appendString control = %q", got)
	}
}

// errWriter fails on the nth write, to exercise the Writer's error paths.
type errWriter struct {
	failAt int
	n      int
}

func (e *errWriter) Write(p []byte) (int, error) {
	e.n++
	if e.n > e.failAt {
		return 0, fmt.Errorf("boom")
	}
	return len(p), nil
}

func TestWriterErrorSticky(t *testing.T) {
	w := NewWriter(&errWriter{failAt: 0})
	if err := w.Write(sample()); err == nil {
		t.Error("expected error on first write")
	}
	if err := w.Write(sample()); err == nil {
		t.Error("expected sticky error")
	}
	if err := w.Close(); err == nil {
		t.Error("expected sticky error on Close")
	}
}

func TestWriteFileError(t *testing.T) {
	if err := WriteFile(filepath.Join(t.TempDir(), "missing-dir", "x.json"), []*codex.Record{sample()}); err == nil {
		t.Error("expected error writing into a nonexistent directory")
	}
}

// TestAppendStringCarriageReturn covers the \r escape path in appendString.
func TestAppendStringCarriageReturn(t *testing.T) {
	if got := string(appendString(nil, "a\rb")); got != `"a\rb"` {
		t.Errorf("appendString \\r = %q, want %q", got, `"a\rb"`)
	}
}

// TestValidateErrors covers all three invalid-UTF-8 paths in validate and the
// resulting Encode error.
func TestValidateErrors(t *testing.T) {
	// invalid leader
	badLeader := codex.NewRecord()
	badLeader.SetLeader(codex.Leader("00000nam \xff2200000   4500"))
	if _, err := Encode(badLeader); err == nil {
		t.Error("Encode: expected error for invalid UTF-8 in leader")
	}

	// invalid control field value
	badCtrl := codex.NewRecord().AddField(codex.NewControlField("001", "\xff"))
	if _, err := Encode(badCtrl); err == nil {
		t.Error("Encode: expected error for invalid UTF-8 in control field")
	}

	// invalid subfield value
	badSub := codex.NewRecord().AddField(codex.NewDataField("245", '1', '0',
		codex.NewSubfield('a', "\xff")))
	if _, err := Encode(badSub); err == nil {
		t.Error("Encode: expected error for invalid UTF-8 in subfield")
	}
}

// TestReadStartsWithUnexpectedDelimiter covers the default branch in Read's
// opening-token switch (a delimiter that is neither '[' nor '{').
func TestReadStartsWithUnexpectedDelimiter(t *testing.T) {
	for _, input := range []string{`]`, `}`} {
		if _, err := NewReader(strings.NewReader(input)).Read(); err == nil {
			t.Errorf("expected error for %q at stream start", input)
		}
	}
}

// TestReadNonObjectTokenInStream covers the wrong-token branch inside Read
// when reading subsequent records from a non-array stream.
func TestReadNonObjectTokenInStream(t *testing.T) {
	one, _ := Encode(sample())
	r := NewReader(strings.NewReader(string(one) + "\n123"))
	if _, err := r.Read(); err != nil {
		t.Fatalf("first Read: %v", err)
	}
	if _, err := r.Read(); err == nil {
		t.Error("expected error for number token where record object expected")
	}
}

// TestSkipValueDeeplyNested covers the depth-increment path in skipValue when a
// skipped value contains a nested container.
func TestSkipValueDeeplyNested(t *testing.T) {
	// "extra" value is {"a":{"b":2}} — depth reaches 2 inside skipValue.
	in := `{"leader":"00000nam a2200000   4500","extra":{"a":{"b":2}},"fields":[]}`
	if _, err := Decode([]byte(in)); err != nil {
		t.Fatalf("unexpected error decoding deeply-nested skip: %v", err)
	}
}

// TestReadFieldDelimNotBrace covers the branch in readField where the field
// value is a JSON array (a Delim that is not '{').
func TestReadFieldDelimNotBrace(t *testing.T) {
	if _, err := Decode([]byte(`{"fields":[{"245":[]}]}`)); err == nil {
		t.Error("expected error for array as data field value")
	}
}

// TestReadFieldTruncatedAfterTag covers the Token() error in readField after
// the tag key has been read but before the value token arrives.
func TestReadFieldTruncatedAfterTag(t *testing.T) {
	if _, err := Decode([]byte(`{"fields":[{"245"`)); err == nil {
		t.Error("expected error for truncated stream after field tag")
	}
}

// TestReadFieldExtraKeyErrors covers the error paths in readField's extra-key
// loop: skipValue error when the extra value is truncated.
func TestReadFieldExtraKeyErrors(t *testing.T) {
	// After the data-field body, there is an extra key with a truncated value.
	in := `{"fields":[{"245":{"ind1":"1","ind2":"0","subfields":[{"a":"T"}]},"extra":`
	if _, err := Decode([]byte(in)); err == nil {
		t.Error("expected error for truncated extra-key value in field object")
	}
}

// TestReadRecordBodyTruncatedKey covers the readKey error path in readRecordBody
// when the stream is truncated inside a key string.
func TestReadRecordBodyTruncatedKey(t *testing.T) {
	if _, err := Decode([]byte(`{"leader":"x","`)); err == nil {
		t.Error("expected error for truncated record body key")
	}
}

// TestExpectTokenError covers the Token() error path inside expect when the
// stream ends before the expected delimiter.
func TestExpectTokenError(t *testing.T) {
	// readFields calls expect('[') but the stream ends after "fields":
	if _, err := Decode([]byte(`{"fields":`)); err == nil {
		t.Error("expected error for stream truncated before fields array delimiter")
	}
}

// TestReadDataFieldInd2NotString covers the readString error path for "ind2"
// inside readDataField.
func TestReadDataFieldInd2NotString(t *testing.T) {
	in := `{"fields":[{"245":{"ind1":"1","ind2":9,"subfields":[]}}]}`
	if _, err := Decode([]byte(in)); err == nil {
		t.Error("expected error for non-string ind2")
	}
}

// TestReadDataFieldKeyError covers the readKey error path inside readDataField
// when the stream is truncated mid-key.
func TestReadDataFieldKeyError(t *testing.T) {
	in := `{"fields":[{"245":{"ind1":"1","`
	if _, err := Decode([]byte(in)); err == nil {
		t.Error("expected error for truncated key inside data field body")
	}
}

// TestReadDataFieldSkipError covers the skipValue error path for an unknown key
// inside readDataField when the skipped value is truncated.
func TestReadDataFieldSkipError(t *testing.T) {
	in := `{"fields":[{"245":{"unknown":`
	if _, err := Decode([]byte(in)); err == nil {
		t.Error("expected error for truncated unknown value in data field")
	}
}

// TestReadSubfieldsTruncatedValue covers the Token() error when consuming the
// closing '}' of a subfield object whose value has been read but the brace is missing.
func TestReadSubfieldsTruncatedValue(t *testing.T) {
	in := `{"fields":[{"245":{"subfields":[{"a":"val"`
	if _, err := Decode([]byte(in)); err == nil {
		t.Error("expected error for truncated subfield (missing closing brace)")
	}
}

// TestReadFilePartialError covers the error-with-partial-results path in ReadFile.
func TestReadFilePartialError(t *testing.T) {
	one, _ := Encode(sample())
	content := string(one) + "\nnot-valid-json"
	path := filepath.Join(t.TempDir(), "partial.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	recs, err := ReadFile(path)
	if err == nil {
		t.Error("expected error for malformed second record")
	}
	if len(recs) != 1 {
		t.Errorf("got %d partial records, want 1", len(recs))
	}
}

// TestWriteInvalidRecord covers the validate error path inside Writer.Write.
func TestWriteInvalidRecord(t *testing.T) {
	bad := codex.NewRecord().AddField(codex.NewDataField("245", '1', '0',
		codex.NewSubfield('a', "\xff")))
	if err := NewWriter(&bytes.Buffer{}).Write(bad); err == nil {
		t.Error("expected error writing record with invalid UTF-8")
	}
}

// TestWriterDoubleClose covers the idempotent double-Close path that returns nil.
func TestWriterDoubleClose(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close: expected nil, got %v", err)
	}
}

// TestWriterCloseOpenFail covers the path inside Close where open() fails
// because the underlying writer refuses the first write.
func TestWriterCloseOpenFail(t *testing.T) {
	w := NewWriter(&errWriter{failAt: 0})
	if err := w.Close(); err == nil {
		t.Error("expected error from Close when underlying writer fails on first write")
	}
}

// TestWriterWriteAfterStickyErr covers the writeAll early-return when the sticky
// error is set before writeAll is invoked from Write.
func TestWriterWriteAfterStickyErr(t *testing.T) {
	// failAt:1 lets the "[\n" header through but fails on the first record write.
	w := NewWriter(&errWriter{failAt: 1})
	_ = w.Write(sample()) // sets wr.err via writeAll
	if err := w.Write(sample()); err == nil {
		t.Error("expected sticky error on second Write")
	}
}

// TestWriteFileInvalidRecord covers the Write-error branch inside WriteFile,
// which closes the file and returns the error when a record fails validation.
func TestWriteFileInvalidRecord(t *testing.T) {
	bad := codex.NewRecord().AddField(codex.NewDataField("245", '1', '0',
		codex.NewSubfield('a', "\xff")))
	path := filepath.Join(t.TempDir(), "out.json")
	if err := WriteFile(path, []*codex.Record{bad}); err == nil {
		t.Error("expected error from WriteFile for invalid UTF-8 record")
	}
}

// TestSkipValueTruncatedNested covers the Token() error path inside skipValue's
// depth loop (when a skipped object/array is truncated mid-stream) and the
// corresponding skipValue-error return in readRecordBody.
func TestSkipValueTruncatedNested(t *testing.T) {
	// "unknown" value is "{" with nothing after it; skipValue enters the depth
	// loop, then Token() returns EOF.
	if _, err := Decode([]byte(`{"unknown":{`)); err == nil {
		t.Error("expected error for truncated nested value in record body skip")
	}
}

// TestReadFieldExtraKeyReadKeyError covers the readKey error path inside
// readField's extra-key loop when the key string is truncated.
func TestReadFieldExtraKeyReadKeyError(t *testing.T) {
	// After the data-field body, the extra key is truncated mid-string.
	in := `{"fields":[{"245":{"ind1":"1","ind2":"0","subfields":[{"a":"T"}]},"extr`
	if _, err := Decode([]byte(in)); err == nil {
		t.Error("expected error for truncated extra-key name in field object")
	}
}

// TestReadSubfieldsKeyError covers the readKey error path in readSubfields
// when the subfield object is opened but its key string is truncated.
func TestReadSubfieldsKeyError(t *testing.T) {
	// After the subfield "{" is consumed, the key string is truncated.
	in := `{"fields":[{"245":{"subfields":[{"a`
	if _, err := Decode([]byte(in)); err == nil {
		t.Error("expected error for truncated subfield key string")
	}
}

// TestWriteFileRoundTrip covers the WriteFile success path and verifies the
// written file round-trips through ReadFile.
func TestWriteFileRoundTrip(t *testing.T) {
	recs := []*codex.Record{
		sample(),
		codex.NewRecord().AddField(codex.NewControlField("001", "rt-001")),
	}
	path := filepath.Join(t.TempDir(), "rt.json")
	if err := WriteFile(path, recs); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	if got[1].ControlField("001") != "rt-001" {
		t.Errorf("001 = %q", got[1].ControlField("001"))
	}
}

func TestGolden(t *testing.T) {
	recs := []*codex.Record{sample(), codex.NewRecord().AddField(codex.NewControlField("001", "x"))}
	path := filepath.Join("testdata", "sample.json")

	var buf bytes.Buffer
	w := NewWriter(&buf)
	for _, rec := range recs {
		if err := w.Write(rec); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (UPDATE_GOLDEN=1 to create): %v", err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("output differs from %s:\n got:\n%s\n want:\n%s", path, buf.Bytes(), want)
	}
}
