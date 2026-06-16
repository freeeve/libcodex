package schemaorg

import (
	"errors"
	"io"
	"os"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
)

// errWriteAfterClose is returned by Write once Close has run.
var errWriteAfterClose = errors.New("schemaorg: Write after Close")

const contextOpen = `{"@context":"https://schema.org"`

// appendBook renders one Book as a JSON-LD object. @context and @type are always
// present, so every later property can simply prepend a comma.
func appendBook(b []byte, bk *Book) []byte {
	b = append(b, contextOpen...)
	b = append(b, `,"@type":`...)
	b = appendString(b, bk.Type)
	b = strProp(b, "name", bk.Name)
	b = agentProp(b, "author", bk.Authors)
	b = agentProp(b, "contributor", bk.Contributors)
	b = orgProp(b, "publisher", bk.Publisher)
	b = strProp(b, "datePublished", bk.DatePublished)
	b = strProp(b, "bookEdition", bk.Edition)
	b = arrayProp(b, "isbn", bk.ISBN)
	b = arrayProp(b, "issn", bk.ISSN)
	b = arrayProp(b, "inLanguage", bk.InLanguage)
	b = arrayProp(b, "about", bk.About)
	b = arrayProp(b, "genre", bk.Genre)
	b = arrayProp(b, "url", bk.URL)
	b = strProp(b, "description", bk.Description)
	b = arrayProp(b, "accessMode", bk.AccessMode)
	b = arrayProp(b, "accessibilityFeature", bk.AccessibilityFeature)
	b = strProp(b, "accessibilitySummary", bk.AccessibilitySummary)
	return append(b, '}')
}

// strProp appends `,"name":"value"`, or nothing when value is empty.
func strProp(b []byte, name, value string) []byte {
	if value == "" {
		return b
	}
	b = key(b, name)
	return appendString(b, value)
}

// arrayProp appends a scalar for one value, a JSON array for several, or nothing
// when empty.
func arrayProp(b []byte, name string, values []string) []byte {
	if len(values) == 0 {
		return b
	}
	b = key(b, name)
	if len(values) == 1 {
		return appendString(b, values[0])
	}
	b = append(b, '[')
	for i, v := range values {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendString(b, v)
	}
	return append(b, ']')
}

// agentProp appends a Person/Organization object for one agent, an array for
// several, or nothing when empty.
func agentProp(b []byte, name string, agents []Agent) []byte {
	if len(agents) == 0 {
		return b
	}
	b = key(b, name)
	if len(agents) == 1 {
		return appendAgentJSON(b, agents[0])
	}
	b = append(b, '[')
	for i, a := range agents {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendAgentJSON(b, a)
	}
	return append(b, ']')
}

func appendAgentJSON(b []byte, a Agent) []byte {
	b = append(b, `{"@type":`...)
	b = appendString(b, a.Type)
	b = append(b, `,"name":`...)
	b = appendString(b, a.Name)
	return append(b, '}')
}

// orgProp appends an Organization object for a publisher name, or nothing.
func orgProp(b []byte, name, value string) []byte {
	if value == "" {
		return b
	}
	b = key(b, name)
	return appendAgentJSON(b, Agent{Type: "Organization", Name: value})
}

func key(b []byte, name string) []byte {
	b = append(b, ',')
	b = appendString(b, name)
	return append(b, ':')
}

const hexDigits = "0123456789abcdef"

// appendString appends s as a quoted JSON string, escaping control and
// markup-significant characters and dropping invalid UTF-8.
func appendString(b []byte, s string) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			i++
			switch c {
			case '"':
				b = append(b, '\\', '"')
			case '\\':
				b = append(b, '\\', '\\')
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			default:
				if c < 0x20 {
					b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
				} else {
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
	return append(b, '"')
}

// Encode converts a record to a standalone schema.org JSON-LD object.
func Encode(r *codex.Record) ([]byte, error) {
	return appendBook(make([]byte, 0, 512), FromRecord(r)), nil
}

// Writer converts records and writes them as a JSON array of schema.org objects.
// Close must be called to terminate the array.
type Writer struct {
	w      io.Writer
	buf    []byte
	wrote  bool
	opened bool
	closed bool
	err    error
}

var _ codex.RecordWriter = (*Writer)(nil)

// NewWriter returns a Writer that writes a JSON array of schema.org Book objects.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

func (wr *Writer) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return errWriteAfterClose
	}
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte("[\n")); err != nil {
			return err
		}
	}
	wr.buf = wr.buf[:0]
	if wr.wrote {
		wr.buf = append(wr.buf, ',', '\n')
	}
	wr.wrote = true
	wr.buf = appendBook(wr.buf, FromRecord(r))
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
		if err := wr.writeAll([]byte("[\n")); err != nil {
			return err
		}
	}
	return wr.writeAll([]byte("\n]\n"))
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

// WriteFile writes every record to the named file as a schema.org JSON-LD array.
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
