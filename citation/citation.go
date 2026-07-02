// Package citation converts MARC 21 records to the RIS and BibTeX citation
// formats used by reference managers (Zotero, EndNote, Mendeley) and LaTeX.
//
// Citations are a flat, lossy subset of a bibliographic record, so this is a
// one-way MARC->citation mapping, not a codec. The Writers implement
// codex.RecordWriter, so they plug into codex.Convert; both formats are
// self-delimiting (an RIS record ends with ER, a BibTeX entry is a complete
// @type{...} block), so no Close is needed.
//
// Unlike the single-format exporters (mods, dublincore, schemaorg) that expose
// one Encode, this package renders two distinct citation formats, so the
// package-level entry points are named for the format they produce: [RIS] and
// [BibTeX] (with [FromRecord] as the shared extraction step). Both return
// ([]byte, error) for signature parity with the other exporters so callers can
// treat any exporter uniformly; the error is always nil because rendering a
// citation string cannot fail.
package citation

import (
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/crosswalk"
)

// Author is a name with intellectual responsibility. Corporate marks a corporate
// body or conference name (MARC 110/710/111/711) so BibTeX can brace-protect it
// -- otherwise BibTeX would split "Food and Agriculture Organization" into two
// authors on the internal "and".
type Author struct {
	Name      string
	Corporate bool
}

// Entry is the bibliographic information extracted from a record, shared by the
// RIS and BibTeX renderers.
type Entry struct {
	risType   string // RIS TY value
	bibType   string // BibTeX entry type (without the @)
	Title     string
	Authors   []Author
	Year      string
	Date      string
	Publisher string
	Place     string
	Edition   string
	ISBN      []string
	ISSN      []string
	Keywords  []string
	Language  string
	Abstract  string
	URL       []string
}

// FromRecord extracts citation fields from a MARC record in a single pass.
func FromRecord(r *codex.Record) *Entry {
	e := &Entry{}
	e.risType, e.bibType = kind(r.Leader())
	for _, f := range r.Fields() {
		switch f.Tag {
		case "245":
			e.Title = crosswalk.JoinSub(f, "ab", " ")
		case "100", "110", "111", "700", "710", "711":
			if v := crosswalk.TrimISBD(f.SubfieldValue('a')); v != "" {
				e.Authors = append(e.Authors, Author{Name: v, Corporate: f.Tag != "100" && f.Tag != "700"})
			}
		case "250":
			e.Edition = crosswalk.TrimISBD(f.SubfieldValue('a'))
		case "260", "264":
			if e.Place == "" {
				e.Place = crosswalk.TrimISBD(f.SubfieldValue('a'))
			}
			if e.Publisher == "" {
				e.Publisher = crosswalk.TrimISBD(f.SubfieldValue('b'))
			}
			if e.Date == "" {
				e.Date = crosswalk.TrimISBD(f.SubfieldValue('c'))
				e.Year = crosswalk.Year(e.Date)
			}
		case "020":
			e.ISBN = appendValues(e.ISBN, f, 'a')
		case "022":
			e.ISSN = appendValues(e.ISSN, f, 'a')
		case "600", "610", "611", "630", "650", "651", "653", "655":
			if v := crosswalk.Subject(f); v != "" {
				e.Keywords = append(e.Keywords, v)
			}
		case "520":
			if e.Abstract == "" {
				e.Abstract = f.SubfieldValue('a')
			}
		case "856":
			e.URL = appendValues(e.URL, f, 'u')
		}
	}
	if e.Year == "" {
		if c := r.ControlField("008"); len(c) >= 11 {
			e.Year = crosswalk.Year(c[7:11])
		}
	}
	if c := r.ControlField("008"); len(c) >= 38 {
		if lang := strings.TrimSpace(c[35:38]); len(lang) == 3 {
			e.Language = lang
		}
	}
	return e
}

// kind maps the leader's type of record (byte 6) and bibliographic level (byte 7)
// to an RIS TY value and a BibTeX entry type.
func kind(l codex.Leader) (string, string) {
	t := l.RecordType()
	level := l.BibLevel()
	switch t {
	case 'a', 't':
		switch level {
		case 's', 'b':
			return "JOUR", "article"
		case 'a':
			return "CHAP", "inbook"
		default:
			return "BOOK", "book"
		}
	case 'c', 'd':
		return "MUSIC", "misc"
	case 'e', 'f':
		return "MAP", "misc"
	case 'g':
		return "VIDEO", "misc"
	case 'i', 'j':
		return "SOUND", "misc"
	case 'm':
		return "COMP", "misc"
	default:
		return "GEN", "misc"
	}
}

func appendValues(dst []string, f codex.Field, code byte) []string {
	for _, sf := range f.Subfields {
		if sf.Code == code {
			if v := crosswalk.TrimISBD(sf.Value); v != "" {
				dst = append(dst, v)
			}
		}
	}
	return dst
}

// ---- RIS ----

// RIS renders the entry as an RIS record.
func (e *Entry) RIS() []byte {
	var b []byte
	b = risLine(b, "TY", e.risType)
	b = risLine(b, "TI", e.Title)
	for _, a := range e.Authors {
		b = risLine(b, "AU", a.Name)
	}
	b = risLine(b, "PY", e.Year)
	b = risLine(b, "DA", e.Date)
	b = risLine(b, "ET", e.Edition)
	b = risLine(b, "PB", e.Publisher)
	b = risLine(b, "CY", e.Place)
	for _, s := range e.ISBN {
		b = risLine(b, "SN", s)
	}
	for _, s := range e.ISSN {
		b = risLine(b, "SN", s)
	}
	for _, k := range e.Keywords {
		b = risLine(b, "KW", k)
	}
	b = risLine(b, "LA", e.Language)
	b = risLine(b, "AB", e.Abstract)
	for _, u := range e.URL {
		b = risLine(b, "UR", u)
	}
	b = append(b, "ER  - \n"...)
	return b
}

func risLine(b []byte, tag, value string) []byte {
	if value == "" {
		return b
	}
	b = append(b, tag...)
	b = append(b, "  - "...)
	b = appendPlain(b, value)
	return append(b, '\n')
}

// appendPlain appends s with line breaks replaced by spaces and invalid UTF-8
// dropped, so a value stays on one line and the output is valid UTF-8.
func appendPlain(b []byte, s string) []byte {
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			i++
			if c == '\n' || c == '\r' {
				b = append(b, ' ')
			} else {
				b = append(b, c)
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		b = append(b, s[i:i+size]...)
		i += size
	}
	return b
}

// RIS converts a record to an RIS record.
func RIS(r *codex.Record) ([]byte, error) { return FromRecord(r).RIS(), nil }

// ---- BibTeX ----

// BibTeX renders the entry as a BibTeX entry.
func (e *Entry) BibTeX() []byte {
	b := append([]byte{'@'}, e.bibType...)
	b = append(b, '{')
	b = append(b, e.citeKey()...)
	b = append(b, ",\n"...)
	if len(e.Authors) > 0 {
		b = bibAuthors(b, e.Authors)
	}
	b = bibField(b, "title", e.Title)
	b = bibField(b, "year", e.Year)
	b = bibField(b, "edition", e.Edition)
	b = bibField(b, "publisher", e.Publisher)
	b = bibField(b, "address", e.Place)
	if len(e.ISBN) > 0 {
		b = bibField(b, "isbn", strings.Join(e.ISBN, ", "))
	}
	if len(e.ISSN) > 0 {
		b = bibField(b, "issn", strings.Join(e.ISSN, ", "))
	}
	if len(e.Keywords) > 0 {
		b = bibField(b, "keywords", strings.Join(e.Keywords, ", "))
	}
	b = bibField(b, "language", e.Language)
	b = bibField(b, "abstract", e.Abstract)
	if len(e.URL) > 0 {
		b = bibField(b, "url", strings.Join(e.URL, " "))
	}
	return append(b, "}\n"...)
}

func bibField(b []byte, name, value string) []byte {
	if value == "" {
		return b
	}
	b = append(b, "  "...)
	b = append(b, name...)
	b = append(b, " = {"...)
	b = appendBibTeX(b, value)
	return append(b, "},\n"...)
}

// bibAuthors writes the author field, joining names with BibTeX's " and "
// separator. A corporate name is wrapped in an extra brace group so BibTeX
// treats it as a single unit -- without it "Food and Agriculture Organization"
// would split into two authors on the internal "and". The literal braces are
// written directly (not through appendBibTeX, which would escape them).
func bibAuthors(b []byte, authors []Author) []byte {
	b = append(b, "  author = {"...)
	for i, a := range authors {
		if i > 0 {
			b = append(b, " and "...)
		}
		if a.Corporate {
			b = append(b, '{')
			b = appendBibTeX(b, a.Name)
			b = append(b, '}')
		} else {
			b = appendBibTeX(b, a.Name)
		}
	}
	return append(b, "},\n"...)
}

// appendBibTeX appends s escaping the characters significant in a brace-delimited
// BibTeX value, replacing line breaks with spaces and dropping invalid UTF-8.
func appendBibTeX(b []byte, s string) []byte {
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			i++
			switch c {
			case '{', '}', '&', '%', '$', '#', '_':
				b = append(b, '\\', c)
			case '\\':
				b = append(b, `\textbackslash{}`...)
			case '~':
				b = append(b, `\textasciitilde{}`...)
			case '^':
				b = append(b, `\textasciicircum{}`...)
			case '\n', '\r':
				b = append(b, ' ')
			default:
				b = append(b, c)
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		b = append(b, s[i:i+size]...)
		i += size
	}
	return b
}

// citeKey builds a stable BibTeX key from the first author surname, year and the
// first significant title word.
func (e *Entry) citeKey() string {
	var b strings.Builder
	if len(e.Authors) > 0 {
		surname := e.Authors[0].Name
		if i := strings.IndexByte(surname, ','); i >= 0 {
			surname = surname[:i]
		}
		b.WriteString(asciiKey(surname))
	}
	b.WriteString(e.Year)
	for w := range strings.FieldsSeq(e.Title) {
		if k := asciiKey(w); k != "" {
			b.WriteString(k)
			break
		}
	}
	if b.Len() == 0 {
		return "ref"
	}
	return b.String()
}

// asciiKey lowercases s and keeps only ASCII letters and digits.
func asciiKey(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c + 32)
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			b.WriteByte(c)
		}
	}
	return b.String()
}

// BibTeX converts a record to a BibTeX entry.
func BibTeX(r *codex.Record) ([]byte, error) { return FromRecord(r).BibTeX(), nil }

// ---- writers ----

// RISWriter writes records as a sequence of RIS records.
type RISWriter struct {
	w   io.Writer
	err error
}

var _ codex.RecordWriter = (*RISWriter)(nil)

// NewRISWriter returns a Writer that writes RIS records to w.
func NewRISWriter(w io.Writer) *RISWriter { return &RISWriter{w: w} }

func (wr *RISWriter) Write(r *codex.Record) error {
	if wr.err == nil {
		_, wr.err = wr.w.Write(FromRecord(r).RIS())
	}
	return wr.err
}

// BibTeXWriter writes records as a sequence of BibTeX entries.
type BibTeXWriter struct {
	w     io.Writer
	wrote bool
	err   error
}

var _ codex.RecordWriter = (*BibTeXWriter)(nil)

// NewBibTeXWriter returns a Writer that writes BibTeX entries to w.
func NewBibTeXWriter(w io.Writer) *BibTeXWriter { return &BibTeXWriter{w: w} }

func (wr *BibTeXWriter) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.wrote {
		if _, wr.err = wr.w.Write(entrySep); wr.err != nil { // blank line between entries
			return wr.err
		}
	}
	wr.wrote = true
	_, wr.err = wr.w.Write(FromRecord(r).BibTeX())
	return wr.err
}

// entrySep is the blank line written between BibTeX entries.
var entrySep = []byte{'\n'}

// WriteRISFile writes every record to the named file in RIS format.
func WriteRISFile(path string, records []*codex.Record) error {
	return writeFile(path, records, func(w io.Writer) codex.RecordWriter { return NewRISWriter(w) })
}

// WriteBibTeXFile writes every record to the named file in BibTeX format.
func WriteBibTeXFile(path string, records []*codex.Record) error {
	return writeFile(path, records, func(w io.Writer) codex.RecordWriter { return NewBibTeXWriter(w) })
}

func writeFile(path string, records []*codex.Record, newW func(io.Writer) codex.RecordWriter) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := newW(f)
	for _, rec := range records {
		if err := w.Write(rec); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}
