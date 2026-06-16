// Package marcjson reads and writes MARC 21 records in the de-facto
// "MARC-in-JSON" structure (the pymarc/ruby-marc/marc4j layout), implementing
// codex.RecordReader and codex.RecordWriter using only encoding/json.
//
// A record is a JSON object with a "leader" string and an ordered "fields"
// array. Each field is a single-key object whose key is the 3-character tag: a
// control field maps the tag to a string value, while a data field maps it to an
// object with "ind1"/"ind2" strings and an ordered "subfields" array of
// single-key {code: value} objects:
//
//	{"leader":"…","fields":[
//	  {"001":"ocm12345"},
//	  {"245":{"ind1":"1","ind2":"0","subfields":[{"a":"Title"},{"c":"Author"}]}}
//	]}
//
// Decoding walks JSON tokens (no reflection), preserving field and subfield
// order, and accepts a single object, a whitespace-separated stream of objects,
// or a top-level array. A Writer emits a JSON array and must be closed.
package marcjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
)

const hexDigits = "0123456789abcdef"

// ---- encoding ----

// appendRecord appends the compact MARC-in-JSON object for r to dst.
func appendRecord(dst []byte, r *codex.Record) []byte {
	dst = append(dst, `{"leader":`...)
	dst = appendString(dst, r.Leader().String())
	dst = append(dst, `,"fields":[`...)
	for i, f := range r.Fields() {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, '{')
		dst = appendString(dst, f.Tag)
		dst = append(dst, ':')
		if f.IsControl() {
			dst = appendString(dst, f.Value)
		} else {
			dst = append(dst, `{"ind1":`...)
			dst = appendString(dst, indStr(f.Ind1))
			dst = append(dst, `,"ind2":`...)
			dst = appendString(dst, indStr(f.Ind2))
			dst = append(dst, `,"subfields":[`...)
			for j, s := range f.Subfields {
				if j > 0 {
					dst = append(dst, ',')
				}
				dst = append(dst, '{')
				dst = appendString(dst, string(s.Code))
				dst = append(dst, ':')
				dst = appendString(dst, s.Value)
				dst = append(dst, '}')
			}
			dst = append(dst, "]}"...)
		}
		dst = append(dst, '}')
	}
	return append(dst, "]}"...)
}

// appendString appends s as a quoted, escaped JSON string.
func appendString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if c < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			} else {
				dst = append(dst, c)
			}
		}
	}
	return append(dst, '"')
}

// indStr renders an indicator byte, mapping an unset (zero) indicator to a blank
// space.
func indStr(b byte) string {
	if b == 0 {
		return " "
	}
	return string(b)
}

// indByte parses an indicator string, defaulting an empty one to a blank space.
func indByte(s string) byte {
	if s == "" {
		return ' '
	}
	return s[0]
}

// codeByte parses a subfield code string.
func codeByte(s string) byte {
	if s == "" {
		return 0
	}
	return s[0]
}

// Encode serializes a record as a single compact MARC-in-JSON object. It returns
// an error if a value is not valid UTF-8 (which JSON cannot represent).
func Encode(r *codex.Record) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	return appendRecord(nil, r), nil
}

// validate reports an error if any value is not valid UTF-8; JSON strings must be
// valid UTF-8, so such a record cannot be represented.
func validate(r *codex.Record) error {
	if !utf8.ValidString(r.Leader().String()) {
		return fmt.Errorf("marcjson: leader is not valid UTF-8")
	}
	for _, f := range r.Fields() {
		if f.IsControl() {
			if !utf8.ValidString(f.Value) {
				return fmt.Errorf("marcjson: control field %s value is not valid UTF-8", f.Tag)
			}
			continue
		}
		for _, s := range f.Subfields {
			if !utf8.ValidString(s.Value) {
				return fmt.Errorf("marcjson: field %s subfield value is not valid UTF-8", f.Tag)
			}
		}
	}
	return nil
}

// ---- decoding ----

// Reader reads MARC records from a MARC-in-JSON stream one record at a time. It
// accepts a single object, a whitespace-separated stream of objects, or a
// top-level array of objects.
type Reader struct {
	dec     *json.Decoder
	started bool
	inArray bool
}

// compile-time assertion that Reader satisfies the core interface.
var _ codex.RecordReader = (*Reader)(nil)

// NewReader returns a Reader that reads records from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{dec: json.NewDecoder(r)}
}

// Read returns the next record, or io.EOF when the stream is exhausted.
func (rd *Reader) Read() (*codex.Record, error) {
	if !rd.started {
		rd.started = true
		tok, err := rd.dec.Token()
		if err != nil {
			return nil, err
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '[':
				rd.inArray = true
			case '{':
				return rd.readRecordBody() // the first object's brace is already consumed
			default:
				return nil, fmt.Errorf("marcjson: unexpected %q at start of stream", d)
			}
		} else {
			return nil, fmt.Errorf("marcjson: expected object or array, got %v", tok)
		}
	}
	if rd.inArray && !rd.dec.More() {
		return nil, io.EOF
	}
	tok, err := rd.dec.Token() // expect '{'
	if err != nil {
		return nil, err // io.EOF ends a non-array stream
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("marcjson: expected record object, got %v", tok)
	}
	return rd.readRecordBody()
}

// readRecordBody reads a record's "leader" and "fields" up to the object's
// closing brace, which it consumes.
func (rd *Reader) readRecordBody() (*codex.Record, error) {
	rec := codex.NewRecord()
	for rd.dec.More() {
		key, err := rd.readKey()
		if err != nil {
			return nil, err
		}
		switch key {
		case "leader":
			s, err := rd.readString()
			if err != nil {
				return nil, err
			}
			if s != "" {
				rec.SetLeader(codex.Leader(s))
			}
		case "fields":
			if err := rd.readFields(rec); err != nil {
				return nil, err
			}
		default:
			if err := rd.skipValue(); err != nil {
				return nil, err
			}
		}
	}
	_, err := rd.dec.Token() // consume '}'
	return rec, err
}

// readFields reads the "fields" array, appending each field to rec.
func (rd *Reader) readFields(rec *codex.Record) error {
	if err := rd.expect('['); err != nil {
		return err
	}
	for rd.dec.More() {
		f, err := rd.readField()
		if err != nil {
			return err
		}
		rec.AddField(f)
	}
	_, err := rd.dec.Token() // consume ']'
	return err
}

// readField reads one single-key field object, consuming its closing brace.
func (rd *Reader) readField() (codex.Field, error) {
	if err := rd.expect('{'); err != nil {
		return codex.Field{}, err
	}
	tag, err := rd.readKey()
	if err != nil {
		return codex.Field{}, err
	}
	tok, err := rd.dec.Token()
	if err != nil {
		return codex.Field{}, err
	}
	var f codex.Field
	switch v := tok.(type) {
	case string:
		f = codex.NewControlField(tag, v)
	case json.Delim:
		if v != '{' {
			return codex.Field{}, fmt.Errorf("marcjson: bad value for field %s", tag)
		}
		if f, err = rd.readDataField(tag); err != nil {
			return codex.Field{}, err
		}
	default:
		return codex.Field{}, fmt.Errorf("marcjson: unexpected value for field %s", tag)
	}
	// A well-formed field object has a single key (the tag); tolerate and skip any
	// extra keys before the closing brace.
	for rd.dec.More() {
		if _, err := rd.readKey(); err != nil {
			return f, err
		}
		if err := rd.skipValue(); err != nil {
			return f, err
		}
	}
	_, err = rd.dec.Token() // consume the field object's '}'
	return f, err
}

// readDataField reads a data field's body (ind1/ind2/subfields) after its
// opening brace, consuming its closing brace.
func (rd *Reader) readDataField(tag string) (codex.Field, error) {
	f := codex.Field{Tag: tag, Ind1: ' ', Ind2: ' '}
	for rd.dec.More() {
		key, err := rd.readKey()
		if err != nil {
			return f, err
		}
		switch key {
		case "ind1":
			s, err := rd.readString()
			if err != nil {
				return f, err
			}
			f.Ind1 = indByte(s)
		case "ind2":
			s, err := rd.readString()
			if err != nil {
				return f, err
			}
			f.Ind2 = indByte(s)
		case "subfields":
			if err := rd.readSubfields(&f); err != nil {
				return f, err
			}
		default:
			if err := rd.skipValue(); err != nil {
				return f, err
			}
		}
	}
	_, err := rd.dec.Token() // consume the data field object's '}'
	return f, err
}

// readSubfields reads the "subfields" array of single-key {code: value} objects.
func (rd *Reader) readSubfields(f *codex.Field) error {
	if err := rd.expect('['); err != nil {
		return err
	}
	for rd.dec.More() {
		if err := rd.expect('{'); err != nil {
			return err
		}
		code, err := rd.readKey()
		if err != nil {
			return err
		}
		val, err := rd.readString()
		if err != nil {
			return err
		}
		f.Subfields = append(f.Subfields, codex.Subfield{Code: codeByte(code), Value: val})
		if _, err := rd.dec.Token(); err != nil { // consume the subfield's '}'
			return err
		}
	}
	_, err := rd.dec.Token() // consume ']'
	return err
}

// expect consumes one token and checks it is the given delimiter.
func (rd *Reader) expect(d json.Delim) error {
	tok, err := rd.dec.Token()
	if err != nil {
		return err
	}
	if got, ok := tok.(json.Delim); !ok || got != d {
		return fmt.Errorf("marcjson: expected %q, got %v", d, tok)
	}
	return nil
}

// readKey reads an object key (a string token).
func (rd *Reader) readKey() (string, error) {
	tok, err := rd.dec.Token()
	if err != nil {
		return "", err
	}
	s, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("marcjson: expected object key, got %v", tok)
	}
	return s, nil
}

// readString reads a string value token.
func (rd *Reader) readString() (string, error) {
	tok, err := rd.dec.Token()
	if err != nil {
		return "", err
	}
	s, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("marcjson: expected string, got %v", tok)
	}
	return s, nil
}

// skipValue reads and discards the next JSON value, including nested containers.
func (rd *Reader) skipValue() error {
	tok, err := rd.dec.Token()
	if err != nil {
		return err
	}
	d, ok := tok.(json.Delim)
	if !ok || (d != '{' && d != '[') {
		return nil
	}
	for depth := 1; depth > 0; {
		tok, err := rd.dec.Token()
		if err != nil {
			return err
		}
		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				depth++
			} else {
				depth--
			}
		}
	}
	return nil
}

// All returns an iterator over the remaining records, for use as
// "for rec, err := range r.All()". It stops at the first error.
func (rd *Reader) All() iter.Seq2[*codex.Record, error] {
	return codex.All(rd)
}

// Decode parses a single record from a MARC-in-JSON byte slice.
func Decode(b []byte) (*codex.Record, error) {
	rec, err := NewReader(bytes.NewReader(b)).Read()
	if err == io.EOF {
		return nil, fmt.Errorf("marcjson: no record found")
	}
	return rec, err
}

// ReadFile reads every record from the named MARC-in-JSON file. On the first
// malformed record it returns the records parsed so far together with the error.
func ReadFile(path string) ([]*codex.Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := NewReader(f)
	var out []*codex.Record
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, rec)
	}
}

// ---- writing ----

// Writer streams records into a JSON array. Close must be called to emit the
// closing "]" and complete the document.
type Writer struct {
	w      io.Writer
	buf    []byte
	opened bool
	closed bool
	wrote  bool // at least one record written, for comma placement
	err    error
}

// compile-time assertion that Writer satisfies the core interface.
var _ codex.RecordWriter = (*Writer)(nil)

// NewWriter returns a Writer that writes a JSON array of records to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Write serializes one record as an element of the JSON array, emitting the
// opening "[" before the first record.
func (wr *Writer) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return fmt.Errorf("marcjson: Write after Close")
	}
	if err := validate(r); err != nil {
		return err
	}
	if err := wr.open(); err != nil {
		return err
	}
	wr.buf = wr.buf[:0]
	if wr.wrote {
		wr.buf = append(wr.buf, ',', '\n')
	}
	wr.wrote = true
	wr.buf = appendRecord(wr.buf, r)
	return wr.writeAll(wr.buf)
}

// Close writes the closing "]". It must be called to produce a complete
// document; an empty array is emitted if no records were written.
func (wr *Writer) Close() error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return nil
	}
	wr.closed = true
	if err := wr.open(); err != nil {
		return err
	}
	return wr.writeAll([]byte("\n]\n"))
}

func (wr *Writer) open() error {
	if wr.opened {
		return wr.err
	}
	wr.opened = true
	return wr.writeAll([]byte("[\n"))
}

func (wr *Writer) writeAll(b []byte) error {
	if wr.err != nil {
		return wr.err
	}
	if _, err := wr.w.Write(b); err != nil {
		wr.err = err
	}
	return wr.err
}

// WriteFile writes every record to the named file as a JSON array, creating it
// or truncating an existing file.
func WriteFile(path string, records []*codex.Record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := NewWriter(f)
	for _, rec := range records {
		if err := w.Write(rec); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
