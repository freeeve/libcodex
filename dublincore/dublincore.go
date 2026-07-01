// Package dublincore converts MARC 21 records to Dublin Core (DCMI), the
// lowest-common-denominator metadata used by OAI-PMH and most repository
// software. It emits the OAI oai_dc XML form and a DC-in-JSON form.
//
// Dublin Core is a different, flat and lossy model (15 repeatable elements), so
// this is a one-way MARC->DC crosswalk, not a codec. The Writers implement
// codex.RecordWriter, so they plug into codex.Convert as conversion targets:
//
//	w := dublincore.NewWriter(out) // oai_dc XML; or NewJSONWriter for DC-JSON
//	codex.Convert(iso2709.NewReader(src), w)
//	w.Close()
package dublincore

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/crosswalk"
)

// Namespaces used by the oai_dc serialization.
const (
	oaiNamespace = "http://www.openarchives.org/OAI/2.0/oai_dc/"
	dcNamespace  = "http://purl.org/dc/elements/1.1/"
)

const (
	xmlHeader       = `<?xml version="1.0" encoding="UTF-8"?>`
	collectionOpen  = `<dcCollection xmlns:oai_dc="` + oaiNamespace + `" xmlns:dc="` + dcNamespace + `">`
	collectionClose = `</dcCollection>`
	dcOpen          = `<oai_dc:dc xmlns:oai_dc="` + oaiNamespace + `" xmlns:dc="` + dcNamespace + `">`
	dcOpenNested    = `<oai_dc:dc>`
)

// DC holds the fifteen Dublin Core elements, each repeatable.
type DC struct {
	Title       []string
	Creator     []string
	Subject     []string
	Description []string
	Publisher   []string
	Contributor []string
	Date        []string
	Type        []string
	Format      []string
	Identifier  []string
	Source      []string
	Language    []string
	Relation    []string
	Coverage    []string
	Rights      []string
}

// FromRecord maps a MARC record to Dublin Core in a single pass over the fields.
func FromRecord(r *codex.Record) *DC {
	dc := &DC{Type: []string{dcType(r.Leader().RecordType())}}
	for _, f := range r.Fields() {
		switch f.Tag {
		case "245":
			dc.Title = appendValue(dc.Title, crosswalk.JoinSub(f, "ab", " "))
		case "100", "110", "111":
			dc.Creator = appendValue(dc.Creator, crosswalk.TrimISBD(f.SubfieldValue('a')))
		case "700", "710", "711":
			dc.Contributor = appendValue(dc.Contributor, crosswalk.TrimISBD(f.SubfieldValue('a')))
		case "600", "610", "611", "630", "650", "651", "653", "655":
			dc.Subject = appendValue(dc.Subject, crosswalk.Subject(f))
		case "500", "505", "520", "521", "545", "550":
			dc.Description = appendValue(dc.Description, f.SubfieldValue('a'))
		case "260", "264":
			dc.Publisher = appendValue(dc.Publisher, crosswalk.TrimISBD(f.SubfieldValue('b')))
			dc.Date = appendValue(dc.Date, crosswalk.TrimISBD(f.SubfieldValue('c')))
		case "300":
			dc.Format = appendValue(dc.Format, crosswalk.JoinSub(f, "ac", " "))
		case "020", "022", "024":
			for _, v := range subfieldValues(f, 'a') {
				dc.Identifier = appendValue(dc.Identifier, v)
			}
		case "856":
			for _, v := range subfieldValues(f, 'u') {
				dc.Identifier = appendValue(dc.Identifier, v)
			}
		case "041":
			for _, code := range subfieldValues(f, 'a') {
				for i := 0; i+3 <= len(code); i += 3 {
					dc.Language = appendValue(dc.Language, code[i:i+3])
				}
			}
		case "506", "540":
			dc.Rights = appendValue(dc.Rights, f.SubfieldValue('a'))
		}
	}
	if c := r.ControlField("008"); len(c) >= 38 {
		if lang := strings.TrimSpace(c[35:38]); len(lang) == 3 && !slices.Contains(dc.Language, lang) {
			dc.Language = append([]string{lang}, dc.Language...)
		}
	}
	return dc
}

// ---- crosswalk helpers ----

func subfieldValues(f codex.Field, code byte) []string {
	var out []string
	for _, sf := range f.Subfields {
		if sf.Code == code {
			if v := crosswalk.TrimISBD(sf.Value); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// dcType maps leader byte 6 (type of record) to a DCMI Type Vocabulary term.
func dcType(recordType byte) string {
	switch recordType {
	case 'e', 'f', 'k':
		return "Image"
	case 'g':
		return "MovingImage"
	case 'i', 'j':
		return "Sound"
	case 'm':
		return "Software"
	case 'o', 'p':
		return "Collection"
	case 'r':
		return "PhysicalObject"
	default:
		return "Text"
	}
}

func appendValue(dst []string, v string) []string {
	if v != "" {
		return append(dst, v)
	}
	return dst
}

// ---- rendering ----

// each calls fn for every element in canonical Dublin Core order.
func appendXMLElems(b []byte, dc *DC, indent string) []byte {
	add := func(name string, values []string) {
		for _, v := range values {
			b = append(b, indent...)
			b = append(b, "<dc:"...)
			b = append(b, name...)
			b = append(b, '>')
			b = appendXMLText(b, v)
			b = append(b, "</dc:"...)
			b = append(b, name...)
			b = append(b, ">\n"...)
		}
	}
	add("title", dc.Title)
	add("creator", dc.Creator)
	add("subject", dc.Subject)
	add("description", dc.Description)
	add("publisher", dc.Publisher)
	add("contributor", dc.Contributor)
	add("date", dc.Date)
	add("type", dc.Type)
	add("format", dc.Format)
	add("identifier", dc.Identifier)
	add("source", dc.Source)
	add("language", dc.Language)
	add("relation", dc.Relation)
	add("coverage", dc.Coverage)
	add("rights", dc.Rights)
	return b
}

// appendXML appends the oai_dc element for dc. open is the opening tag (with or
// without namespace declarations); indent prefixes each element line.
func appendXML(b []byte, dc *DC, open, indent string) []byte {
	b = append(b, open...)
	b = append(b, '\n')
	b = appendXMLElems(b, dc, indent)
	return append(b, "</oai_dc:dc>"...)
}

func appendXMLText(b []byte, s string) []byte {
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			i++
			switch c {
			case '&':
				b = append(b, "&amp;"...)
			case '<':
				b = append(b, "&lt;"...)
			case '>':
				b = append(b, "&gt;"...)
			case '\r':
				b = append(b, "&#xD;"...)
			default:
				// Drop control characters XML 1.0 cannot represent (lossy export).
				if c >= 0x20 || c == '\t' || c == '\n' {
					b = append(b, c)
				}
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			i++ // drop an invalid UTF-8 byte
			continue
		}
		b = append(b, s[i:i+size]...)
		i += size
	}
	return b
}

func appendJSON(b []byte, dc *DC) []byte {
	b = append(b, '{')
	c := false
	for _, e := range []struct {
		name   string
		values []string
	}{
		{"title", dc.Title}, {"creator", dc.Creator}, {"subject", dc.Subject},
		{"description", dc.Description}, {"publisher", dc.Publisher},
		{"contributor", dc.Contributor}, {"date", dc.Date}, {"type", dc.Type},
		{"format", dc.Format}, {"identifier", dc.Identifier}, {"source", dc.Source},
		{"language", dc.Language}, {"relation", dc.Relation},
		{"coverage", dc.Coverage}, {"rights", dc.Rights},
	} {
		if len(e.values) == 0 {
			continue
		}
		if c {
			b = append(b, ',')
		}
		c = true
		b = crosswalk.AppendJSONString(b, e.name)
		b = append(b, ':', '[')
		for i, v := range e.values {
			if i > 0 {
				b = append(b, ',')
			}
			b = crosswalk.AppendJSONString(b, v)
		}
		b = append(b, ']')
	}
	return append(b, '}')
}

// Encode converts a record to a standalone oai_dc XML document.
func Encode(r *codex.Record) ([]byte, error) {
	return appendXML(nil, FromRecord(r), dcOpen, "  "), nil
}

// EncodeJSON converts a record to a compact Dublin Core JSON object.
func EncodeJSON(r *codex.Record) ([]byte, error) {
	return appendJSON(nil, FromRecord(r)), nil
}

// Writer converts records and writes them as oai_dc elements within a
// <dcCollection>. Close must be called to emit the closing tag.
type Writer struct {
	w      io.Writer
	buf    []byte
	opened bool
	closed bool
	err    error
}

var _ codex.RecordWriter = (*Writer)(nil)

// NewWriter returns a Writer that writes an oai_dc XML collection to w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

func (wr *Writer) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return fmt.Errorf("dublincore: Write after Close")
	}
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte(xmlHeader + "\n" + collectionOpen + "\n")); err != nil {
			return err
		}
	}
	wr.buf = appendXML(wr.buf[:0], FromRecord(r), dcOpenNested, "  ")
	wr.buf = append(wr.buf, '\n')
	return wr.writeAll(wr.buf)
}

func (wr *Writer) Close() error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return nil
	}
	wr.closed = true
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte(xmlHeader + "\n" + collectionOpen + "\n")); err != nil {
			return err
		}
	}
	return wr.writeAll([]byte(collectionClose + "\n"))
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

// JSONWriter converts records and writes them as a JSON array of DC objects.
type JSONWriter struct {
	w     io.Writer
	buf   []byte
	wrote bool
	open  bool
	close bool
	err   error
}

var _ codex.RecordWriter = (*JSONWriter)(nil)

// NewJSONWriter returns a Writer that writes a JSON array of Dublin Core objects.
func NewJSONWriter(w io.Writer) *JSONWriter { return &JSONWriter{w: w} }

func (wr *JSONWriter) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.close {
		return fmt.Errorf("dublincore: Write after Close")
	}
	if !wr.open {
		wr.open = true
		if err := wr.writeAll([]byte("[\n")); err != nil {
			return err
		}
	}
	wr.buf = wr.buf[:0]
	if wr.wrote {
		wr.buf = append(wr.buf, ',', '\n')
	}
	wr.wrote = true
	wr.buf = appendJSON(wr.buf, FromRecord(r))
	return wr.writeAll(wr.buf)
}

func (wr *JSONWriter) Close() error {
	if wr.err != nil {
		return wr.err
	}
	if wr.close {
		return nil
	}
	wr.close = true
	if !wr.open {
		wr.open = true
		if err := wr.writeAll([]byte("[\n")); err != nil {
			return err
		}
	}
	return wr.writeAll([]byte("\n]\n"))
}

func (wr *JSONWriter) writeAll(b []byte) error {
	if wr.err != nil {
		return wr.err
	}
	if _, err := wr.w.Write(b); err != nil {
		wr.err = err
	}
	return wr.err
}

// WriteFile writes every record to the named file as an oai_dc XML collection.
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
