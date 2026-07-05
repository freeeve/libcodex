// Package marcjson reads and writes MARC 21 records in the de-facto
// "MARC-in-JSON" structure (the pymarc/ruby-marc/marc4j layout), implementing
// codex.RecordReader and codex.RecordWriter with a hand-rolled tokenizer (see
// scan.go) rather than encoding/json, so both directions avoid reflection and
// most per-token allocation.
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

// indByte parses an indicator string, folding an empty or non-printable one to a
// blank space — the encode side already renders an unset (zero) indicator as a
// blank, so accepting the raw byte here would make decode→encode→decode unstable.
func indByte(s string) byte {
	if s == "" || !asciiChar(s[0]) {
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

// validate reports an error if the record cannot be represented: a value or tag
// that is not valid UTF-8 (JSON strings must be UTF-8), or an indicator or
// subfield code that is not printable ASCII. A high indicator/code byte would be
// emitted by string(b) as multibyte UTF-8 and read back as its first byte,
// silently corrupting the record, so it is rejected as marcxml does.
func validate(r *codex.Record) error {
	if !utf8.ValidString(r.Leader().String()) {
		return fmt.Errorf("marcjson: leader is not valid UTF-8")
	}
	for _, f := range r.Fields() {
		if !utf8.ValidString(f.Tag) {
			return fmt.Errorf("marcjson: tag %q is not valid UTF-8", f.Tag)
		}
		if f.IsControl() {
			if !utf8.ValidString(f.Value) {
				return fmt.Errorf("marcjson: control field %s value is not valid UTF-8", f.Tag)
			}
			continue
		}
		if (f.Ind1 != 0 && !asciiChar(f.Ind1)) || (f.Ind2 != 0 && !asciiChar(f.Ind2)) {
			return fmt.Errorf("marcjson: field %s indicator is not printable ASCII", f.Tag)
		}
		for _, s := range f.Subfields {
			if !asciiChar(s.Code) {
				return fmt.Errorf("marcjson: field %s subfield code is not printable ASCII", f.Tag)
			}
			if !utf8.ValidString(s.Value) {
				return fmt.Errorf("marcjson: field %s subfield value is not valid UTF-8", f.Tag)
			}
		}
	}
	return nil
}

// asciiChar reports whether a single indicator or subfield-code byte is printable
// ASCII (never multibyte, so it round-trips through string(b) and s[0]).
func asciiChar(b byte) bool {
	return b >= 0x20 && b < 0x7f
}

// controlTag reports whether tag is in the control-field range (below "010"),
// matching codex.Field.IsControl so the decoder can reject a wire type that
// contradicts the tag.
func controlTag(tag string) bool {
	return tag < "010"
}

// ---- decoding ----

// Reader reads MARC records from a MARC-in-JSON stream one record at a time. It
// accepts a single object, a whitespace-separated stream of objects, or a
// top-level array of objects.
type Reader struct {
	sc      *scanner
	started bool
	inArray bool
}

// compile-time assertion that Reader satisfies the core interface.
var _ codex.RecordReader = (*Reader)(nil)

// NewReader returns a Reader that reads records from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{sc: newScanner(r)}
}

// Read returns the next record, or io.EOF when the stream is exhausted.
func (rd *Reader) Read() (*codex.Record, error) {
	if !rd.started {
		rd.started = true
		c, err := rd.sc.consume()
		if err != nil {
			return nil, err
		}
		switch c {
		case '[':
			rd.inArray = true
		case '{':
			return rd.readRecordBody() // the first object's brace is already consumed
		default:
			return nil, fmt.Errorf("marcjson: unexpected %q at start of stream", c)
		}
	}
	if rd.inArray {
		more, err := rd.sc.more(']')
		if err != nil {
			return nil, err
		}
		if !more {
			return nil, io.EOF
		}
	}
	c, err := rd.sc.consume() // expect '{'
	if err != nil {
		return nil, err // io.EOF ends a non-array stream
	}
	if c != '{' {
		return nil, fmt.Errorf("marcjson: expected record object, got %q", c)
	}
	return rd.readRecordBody()
}

// readRecordBody reads a record's "leader" and "fields" up to the object's
// closing brace, which it consumes.
func (rd *Reader) readRecordBody() (*codex.Record, error) {
	rec := codex.NewRecord()
	for {
		more, err := rd.sc.more('}')
		if err != nil {
			return nil, err
		}
		if !more {
			break
		}
		key, err := rd.sc.readString()
		if err != nil {
			return nil, err
		}
		switch key {
		case "leader":
			s, err := rd.sc.readString()
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
			if err := rd.sc.skipValue(); err != nil {
				return nil, err
			}
		}
	}
	return rec, rd.sc.expect('}')
}

// readFields reads the "fields" array, appending each field to rec.
func (rd *Reader) readFields(rec *codex.Record) error {
	if err := rd.sc.expect('['); err != nil {
		return err
	}
	for {
		more, err := rd.sc.more(']')
		if err != nil {
			return err
		}
		if !more {
			break
		}
		f, err := rd.readField()
		if err != nil {
			return err
		}
		rec.AddField(f)
	}
	return rd.sc.expect(']')
}

// readField reads one single-key field object, consuming its closing brace.
func (rd *Reader) readField() (codex.Field, error) {
	if err := rd.sc.expect('{'); err != nil {
		return codex.Field{}, err
	}
	tag, err := rd.sc.readString()
	if err != nil {
		return codex.Field{}, err
	}
	c, err := rd.sc.peek() // a string value denotes a control field, '{' a data field
	if err != nil {
		return codex.Field{}, err
	}
	var f codex.Field
	switch c {
	case '"':
		v, err := rd.sc.readString()
		if err != nil {
			return codex.Field{}, err
		}
		// A data-range tag would make the value vanish on re-encode, so reject the
		// contradiction.
		if !controlTag(tag) {
			return codex.Field{}, fmt.Errorf("marcjson: field %s has a control-field (string) value but a data-field tag", tag)
		}
		f = codex.NewControlField(tag, v)
	case '{':
		if controlTag(tag) {
			return codex.Field{}, fmt.Errorf("marcjson: field %s has a data-field (object) value but a control-field tag", tag)
		}
		if f, err = rd.readDataField(tag); err != nil {
			return codex.Field{}, err
		}
	default:
		return codex.Field{}, fmt.Errorf("marcjson: unexpected value for field %s", tag)
	}
	// A well-formed field object has a single key (the tag); tolerate and skip any
	// extra keys before the closing brace.
	for {
		more, err := rd.sc.more('}')
		if err != nil {
			return f, err
		}
		if !more {
			break
		}
		if _, err := rd.sc.readString(); err != nil {
			return f, err
		}
		if err := rd.sc.skipValue(); err != nil {
			return f, err
		}
	}
	return f, rd.sc.expect('}')
}

// readDataField reads a data field's body (ind1/ind2/subfields), consuming its
// opening and closing braces.
func (rd *Reader) readDataField(tag string) (codex.Field, error) {
	if err := rd.sc.expect('{'); err != nil {
		return codex.Field{}, err
	}
	f := codex.Field{Tag: tag, Ind1: ' ', Ind2: ' '}
	for {
		more, err := rd.sc.more('}')
		if err != nil {
			return f, err
		}
		if !more {
			break
		}
		key, err := rd.sc.readString()
		if err != nil {
			return f, err
		}
		switch key {
		case "ind1":
			s, err := rd.sc.readString()
			if err != nil {
				return f, err
			}
			f.Ind1 = indByte(s)
		case "ind2":
			s, err := rd.sc.readString()
			if err != nil {
				return f, err
			}
			f.Ind2 = indByte(s)
		case "subfields":
			if err := rd.readSubfields(&f); err != nil {
				return f, err
			}
		default:
			if err := rd.sc.skipValue(); err != nil {
				return f, err
			}
		}
	}
	return f, rd.sc.expect('}')
}

// readSubfields reads the "subfields" array of single-key {code: value} objects.
func (rd *Reader) readSubfields(f *codex.Field) error {
	if err := rd.sc.expect('['); err != nil {
		return err
	}
	for {
		more, err := rd.sc.more(']')
		if err != nil {
			return err
		}
		if !more {
			break
		}
		if err := rd.sc.expect('{'); err != nil {
			return err
		}
		code, err := rd.sc.readString()
		if err != nil {
			return err
		}
		val, err := rd.sc.readString()
		if err != nil {
			return err
		}
		f.Subfields = append(f.Subfields, codex.Subfield{Code: codeByte(code), Value: val})
		if err := rd.sc.expect('}'); err != nil { // consume the subfield's '}'
			return err
		}
	}
	return rd.sc.expect(']')
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
	return codex.ReadAll(NewReader(f))
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
	if err := codex.WriteAll(NewWriter(f), records); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
