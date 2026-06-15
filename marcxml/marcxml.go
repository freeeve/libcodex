// Package marcxml reads and writes MARC 21 records in the Library of Congress
// MARCXML "slim" serialization (namespace http://www.loc.gov/MARC21/slim),
// implementing codex.RecordReader and codex.RecordWriter using only
// encoding/xml.
//
// A MARCXML document is a <collection> of <record> elements, each holding a
// <leader>, <controlfield> elements (tags below "010") and <datafield> elements
// carrying two indicator attributes and <subfield> children. Values are UTF-8
// and are XML-escaped on write. Decoding is namespace-agnostic, so documents
// with or without the slim namespace (and a bare <record> with no <collection>
// wrapper) are accepted.
package marcxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"iter"
	"os"

	"github.com/freeeve/libcodex"
)

// Namespace is the MARCXML slim schema namespace.
const Namespace = "http://www.loc.gov/MARC21/slim"

const (
	xmlHeader       = `<?xml version="1.0" encoding="UTF-8"?>`
	collectionOpen  = `<collection xmlns="` + Namespace + `">`
	collectionClose = `</collection>`
)

// recordXML mirrors a MARCXML <record>. The same type marshals (Encode/Writer)
// and unmarshals (Reader); Xmlns is set only when encoding a standalone record.
// appendRecord appends the MARCXML <record> element for r to dst. pre is the
// per-line indentation prefix ("" for a standalone record, "  " for records
// inside a <collection>); when xmlns is non-empty it is declared on the record
// element. The result ends at </record> with no trailing newline. Hand-rolling
// the encoder (rather than reflection-based encoding/xml marshaling) keeps the
// write path allocation-lean.
func appendRecord(dst []byte, r *codex.Record, pre, xmlns string) []byte {
	dst = append(dst, pre...)
	dst = append(dst, "<record"...)
	if xmlns != "" {
		dst = append(dst, ` xmlns="`...)
		dst = append(dst, xmlns...)
		dst = append(dst, '"')
	}
	dst = append(dst, ">\n"...)

	dst = append(dst, pre...)
	dst = append(dst, "  <leader>"...)
	dst = appendChardata(dst, r.Leader().String())
	dst = append(dst, "</leader>\n"...)

	for _, f := range r.Fields() {
		if f.IsControl() {
			dst = append(dst, pre...)
			dst = append(dst, `  <controlfield tag="`...)
			dst = append(dst, f.Tag...)
			dst = append(dst, `">`...)
			dst = appendChardata(dst, f.Value)
			dst = append(dst, "</controlfield>\n"...)
			continue
		}
		dst = append(dst, pre...)
		dst = append(dst, `  <datafield tag="`...)
		dst = append(dst, f.Tag...)
		dst = append(dst, `" ind1="`...)
		dst = appendInd(dst, f.Ind1)
		dst = append(dst, `" ind2="`...)
		dst = appendInd(dst, f.Ind2)
		dst = append(dst, "\">\n"...)
		for _, s := range f.Subfields {
			dst = append(dst, pre...)
			dst = append(dst, `    <subfield code="`...)
			dst = appendAttrByte(dst, s.Code)
			dst = append(dst, `">`...)
			dst = appendChardata(dst, s.Value)
			dst = append(dst, "</subfield>\n"...)
		}
		dst = append(dst, pre...)
		dst = append(dst, "  </datafield>\n"...)
	}
	dst = append(dst, pre...)
	return append(dst, "</record>"...)
}

// appendChardata appends s to dst, escaping the characters that are significant
// in XML element content. Carriage return is escaped so it survives the XML
// line-ending normalization a decoder applies.
func appendChardata(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			dst = append(dst, "&amp;"...)
		case '<':
			dst = append(dst, "&lt;"...)
		case '>':
			dst = append(dst, "&gt;"...)
		case '\r':
			dst = append(dst, "&#xD;"...)
		default:
			dst = append(dst, s[i])
		}
	}
	return dst
}

// appendInd appends an indicator byte as attribute content, mapping an unset
// (zero) indicator to a blank space.
func appendInd(dst []byte, b byte) []byte {
	if b == 0 {
		b = ' '
	}
	return appendAttrByte(dst, b)
}

// appendAttrByte appends a single byte as attribute content, escaping the
// characters significant inside a double-quoted XML attribute.
func appendAttrByte(dst []byte, b byte) []byte {
	switch b {
	case '&':
		return append(dst, "&amp;"...)
	case '<':
		return append(dst, "&lt;"...)
	case '"':
		return append(dst, "&quot;"...)
	default:
		return append(dst, b)
	}
}

// validate reports an error if any datum in r contains a character XML 1.0
// cannot represent (a control character other than tab, newline or return, e.g.
// a NUL), which would make the output invalid. An unset (zero) indicator is
// allowed because it is emitted as a blank space.
func validate(r *codex.Record) error {
	if !xmlText(r.Leader().String()) {
		return fmt.Errorf("marcxml: leader contains a character not allowed in XML")
	}
	for _, f := range r.Fields() {
		if !xmlText(f.Tag) {
			return fmt.Errorf("marcxml: tag %q contains a character not allowed in XML", f.Tag)
		}
		if f.IsControl() {
			if !xmlText(f.Value) {
				return fmt.Errorf("marcxml: control field %s value contains a character not allowed in XML", f.Tag)
			}
			continue
		}
		if (f.Ind1 != 0 && !xmlByte(f.Ind1)) || (f.Ind2 != 0 && !xmlByte(f.Ind2)) {
			return fmt.Errorf("marcxml: field %s indicator is not allowed in XML", f.Tag)
		}
		for _, s := range f.Subfields {
			if !xmlByte(s.Code) {
				return fmt.Errorf("marcxml: field %s subfield code is not allowed in XML", f.Tag)
			}
			if !xmlText(s.Value) {
				return fmt.Errorf("marcxml: field %s subfield value contains a character not allowed in XML", f.Tag)
			}
		}
	}
	return nil
}

// xmlText reports whether every byte of s is allowed in XML 1.0.
func xmlText(s string) bool {
	for i := 0; i < len(s); i++ {
		if !xmlByte(s[i]) {
			return false
		}
	}
	return true
}

// xmlByte reports whether b is allowed in XML 1.0 character data: any byte at or
// above 0x20 (including UTF-8 multibyte sequences), or tab, newline or return.
func xmlByte(b byte) bool {
	return b >= 0x20 || b == '\t' || b == '\n' || b == '\r'
}

// indByte parses a one-character indicator attribute, defaulting a missing
// attribute to a blank space.
func indByte(s string) byte {
	if s == "" {
		return ' '
	}
	return s[0]
}

// codeByte parses a one-character subfield code attribute.
func codeByte(s string) byte {
	if s == "" {
		return 0
	}
	return s[0]
}

// Reader reads MARC records from a MARCXML stream one <record> at a time. It does
// not buffer the whole document.
type Reader struct {
	dec *xml.Decoder
}

// compile-time assertion that Reader satisfies the core interface.
var _ codex.RecordReader = (*Reader)(nil)

// NewReader returns a Reader that reads records from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{dec: xml.NewDecoder(r)}
}

// Read returns the next record, or io.EOF when the stream is exhausted. It
// advances to the next <record> element (inside a <collection> or at the top
// level) and decodes it by walking tokens, so field order is preserved exactly
// and the reflection cost of struct unmarshaling is avoided.
func (rd *Reader) Read() (*codex.Record, error) {
	for {
		tok, err := rd.dec.Token()
		if err != nil {
			return nil, err // io.EOF at a clean end of stream
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "record" {
			return rd.readRecord()
		}
	}
}

// readRecord builds a record from the tokens up to and including </record>.
func (rd *Reader) readRecord() (*codex.Record, error) {
	rec := codex.NewRecord()
	for {
		tok, err := rd.dec.Token()
		if err != nil {
			return nil, wrap(err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "leader":
				s, err := rd.text()
				if err != nil {
					return nil, err
				}
				if s != "" {
					rec.SetLeader(codex.Leader(s))
				}
			case "controlfield":
				tag := attrValue(t, "tag") // read before the next Token() reuses t.Attr
				s, err := rd.text()
				if err != nil {
					return nil, err
				}
				rec.AddField(codex.NewControlField(tag, s))
			case "datafield":
				f, err := rd.readDataField(t)
				if err != nil {
					return nil, err
				}
				rec.AddField(f)
			default:
				if err := rd.dec.Skip(); err != nil {
					return nil, wrap(err)
				}
			}
		case xml.EndElement:
			if t.Name.Local == "record" {
				return rec, nil
			}
		}
	}
}

// readDataField builds a data field from a <datafield> start element and its
// <subfield> children.
func (rd *Reader) readDataField(start xml.StartElement) (codex.Field, error) {
	f := codex.Field{
		Tag:  attrValue(start, "tag"),
		Ind1: indByte(attrValue(start, "ind1")),
		Ind2: indByte(attrValue(start, "ind2")),
	}
	for {
		tok, err := rd.dec.Token()
		if err != nil {
			return f, wrap(err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "subfield" {
				code := attrValue(t, "code") // read before text() advances the decoder
				val, err := rd.text()
				if err != nil {
					return f, err
				}
				f.Subfields = append(f.Subfields, codex.Subfield{Code: codeByte(code), Value: val})
			} else if err := rd.dec.Skip(); err != nil {
				return f, wrap(err)
			}
		case xml.EndElement:
			if t.Name.Local == "datafield" {
				return f, nil
			}
		}
	}
}

// text reads the character data of the current leaf element up to its end tag.
// CharData tokens are valid only until the next Token call, so the bytes are
// copied as they are read.
func (rd *Reader) text() (string, error) {
	var b []byte
	for {
		tok, err := rd.dec.Token()
		if err != nil {
			return "", wrap(err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			b = append(b, t...)
		case xml.EndElement:
			return string(b), nil
		case xml.StartElement:
			if err := rd.dec.Skip(); err != nil { // unexpected child element
				return "", wrap(err)
			}
		}
	}
}

// attrValue returns the value of the attribute with the given local name, or the
// empty string if it is absent.
func attrValue(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// wrap annotates a decoder error, passing io.EOF through unchanged so callers can
// detect end of stream.
func wrap(err error) error {
	if err == io.EOF {
		return err
	}
	return fmt.Errorf("marcxml: %w", err)
}

// All returns an iterator over the remaining records in the stream, for use as
// "for rec, err := range r.All()". It stops at the first error.
func (rd *Reader) All() iter.Seq2[*codex.Record, error] {
	return codex.All(rd)
}

// Decode parses a single record from a MARCXML byte slice (a <record>, or the
// first record of a <collection>).
func Decode(b []byte) (*codex.Record, error) {
	rec, err := NewReader(bytes.NewReader(b)).Read()
	if err == io.EOF {
		return nil, fmt.Errorf("marcxml: no record found")
	}
	return rec, err
}

// Encode serializes a record as a standalone MARCXML <record> element, indented
// and carrying the slim namespace declaration. Use a Writer to emit a full
// <collection> document.
func Encode(r *codex.Record) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	return appendRecord(nil, r, "", Namespace), nil
}

// ReadFile reads every record from the named MARCXML file. On the first
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

// Writer streams records into a MARCXML <collection>. Close must be called to
// emit the closing </collection> and complete the document.
type Writer struct {
	w      io.Writer
	buf    []byte // reused across writes
	opened bool
	closed bool
	err    error
}

// compile-time assertion that Writer satisfies the core interface.
var _ codex.RecordWriter = (*Writer)(nil)

// NewWriter returns a Writer that writes a MARCXML collection to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Write serializes one record as a <record> element within the collection,
// emitting the document header and <collection> open tag before the first
// record.
func (wr *Writer) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return fmt.Errorf("marcxml: Write after Close")
	}
	if err := validate(r); err != nil {
		return err
	}
	if err := wr.open(); err != nil {
		return err
	}
	wr.buf = appendRecord(wr.buf[:0], r, "  ", "")
	wr.buf = append(wr.buf, '\n')
	return wr.writeAll(wr.buf)
}

// Close writes the closing </collection> tag. It must be called to produce a
// complete document; an empty collection is emitted if no records were written.
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
	return wr.writeAll([]byte(collectionClose + "\n"))
}

func (wr *Writer) open() error {
	if wr.opened {
		return wr.err
	}
	wr.opened = true
	return wr.writeAll([]byte(xmlHeader + "\n" + collectionOpen + "\n"))
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

// WriteFile writes every record to the named file as a MARCXML collection,
// creating it or truncating an existing file.
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
