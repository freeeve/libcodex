// Package mrk reads and writes MARC 21 records in the MARCMaker / MARCBreaker
// mnemonic line format (".mrk"), implementing codex.RecordReader and
// codex.RecordWriter using only the standard library.
//
// Each field is one line beginning with "=": "=LDR  " carries the leader, and
// "=TAG  " carries a field, where TAG is the 3-character tag followed by two
// spaces. A control field (tag below "010") is the raw value; a data field is
// two indicator characters — a blank indicator written as a backslash ("\") —
// followed by subfields, each introduced by "$" and a one-character code.
// Records are separated by a blank line:
//
//	=LDR  00000nam a2200000   4500
//	=001  ocm12345
//	=245  10$aStone butch blues :$ba novel /$cLeslie Feinberg.
//	=650  \0$aLesbians
//
// The literal characters "$", "{", "}" and "&" are written as the mnemonics
// {dollar}, {lcub}, {rcub} and {amp}; decoding also accepts numeric character
// references (&#xHHHH; and &#DDDD;). Values are otherwise UTF-8; the wider
// MARC-8 mnemonic repertoire is out of scope.
package mrk

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"iter"
	"os"
	"strconv"
	"strings"

	"github.com/freeeve/libcodex"
)

// ---- encoding ----

// appendRecord appends the .mrk lines for r to dst (one line per field, ending
// with a newline after the last field; no record-separating blank line).
func appendRecord(dst []byte, r *codex.Record) []byte {
	dst = append(dst, "=LDR  "...)
	dst = appendEscaped(dst, r.Leader().String())
	dst = append(dst, '\n')
	for _, f := range r.Fields() {
		dst = append(dst, '=')
		dst = append(dst, f.Tag...)
		dst = append(dst, ' ', ' ')
		if f.IsControl() {
			dst = appendEscaped(dst, f.Value)
		} else {
			dst = append(dst, indChar(f.Ind1), indChar(f.Ind2))
			for _, s := range f.Subfields {
				dst = append(dst, '$', s.Code)
				dst = appendEscaped(dst, s.Value)
			}
		}
		dst = append(dst, '\n')
	}
	return dst
}

// appendEscaped appends s, replacing the mnemonic-significant characters.
func appendEscaped(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '$':
			dst = append(dst, "{dollar}"...)
		case '{':
			dst = append(dst, "{lcub}"...)
		case '}':
			dst = append(dst, "{rcub}"...)
		case '&':
			// Escape "&" so a literal value shaped like a numeric character
			// reference (e.g. "&#38;") is not resolved away on decode.
			dst = append(dst, "{amp}"...)
		default:
			dst = append(dst, s[i])
		}
	}
	return dst
}

// indChar renders an indicator byte, writing a blank (or unset) indicator as the
// backslash MARCMaker uses.
func indChar(b byte) byte {
	if b == ' ' || b == 0 {
		return '\\'
	}
	return b
}

// indByte parses an indicator character, mapping the backslash back to a blank.
func indByte(b byte) byte {
	if b == '\\' {
		return ' '
	}
	return b
}

// validate reports an error if any datum in r cannot be represented in the
// line-based format: a line break (which the format cannot carry, and which a
// reader strips as a line ending) anywhere, or a subfield delimiter "$" used as
// a subfield code (it would be misread as a new subfield).
func validate(r *codex.Record) error {
	if hasNewline(r.Leader().String()) {
		return fmt.Errorf("mrk: leader contains a line break")
	}
	for _, f := range r.Fields() {
		if !validTag(f.Tag) {
			return fmt.Errorf("mrk: field tag %q is not representable", f.Tag)
		}
		if f.IsControl() {
			if hasNewline(f.Value) {
				return fmt.Errorf("mrk: control field %s value contains a line break", f.Tag)
			}
			continue
		}
		if isBreak(f.Ind1) || isBreak(f.Ind2) {
			return fmt.Errorf("mrk: field %s indicator contains a line break", f.Tag)
		}
		// A backslash indicator would decode back to a blank (the blank mnemonic),
		// silently changing the data, so reject it as unrepresentable.
		if f.Ind1 == '\\' || f.Ind2 == '\\' {
			return fmt.Errorf("mrk: field %s backslash indicator is not representable", f.Tag)
		}
		for _, s := range f.Subfields {
			if isBreak(s.Code) || s.Code == '$' {
				return fmt.Errorf("mrk: field %s subfield code %q is not representable", f.Tag, s.Code)
			}
			if hasNewline(s.Value) {
				return fmt.Errorf("mrk: field %s subfield value contains a line break", f.Tag)
			}
		}
	}
	return nil
}

// validTag reports whether tag can be written as a .mrk line label: exactly three
// printable ASCII bytes, none of them a space, control byte, or the "=" that
// begins a line. A tag failing this would inject or shift lines on decode.
func validTag(tag string) bool {
	if len(tag) != 3 {
		return false
	}
	for i := 0; i < len(tag); i++ {
		if c := tag[i]; c <= 0x20 || c >= 0x7f || c == '=' {
			return false
		}
	}
	return true
}

func hasNewline(s string) bool {
	return strings.ContainsAny(s, "\n\r")
}

func isBreak(b byte) bool {
	return b == '\n' || b == '\r'
}

// Encode serializes a record to its .mrk lines (no trailing record separator).
func Encode(r *codex.Record) ([]byte, error) {
	if err := validate(r); err != nil {
		return nil, err
	}
	return appendRecord(nil, r), nil
}

// ---- decoding ----

// unescape reverses appendEscaped and resolves numeric character references.
func unescape(s string) string {
	if !strings.ContainsAny(s, "{&") {
		return s // fast path: nothing to unescape
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "{dollar}"):
			b.WriteByte('$')
			i += len("{dollar}")
		case strings.HasPrefix(s[i:], "{lcub}"):
			b.WriteByte('{')
			i += len("{lcub}")
		case strings.HasPrefix(s[i:], "{rcub}"):
			b.WriteByte('}')
			i += len("{rcub}")
		case strings.HasPrefix(s[i:], "{amp}"):
			b.WriteByte('&')
			i += len("{amp}")
		case strings.HasPrefix(s[i:], "&#"):
			if r, n := charRef(s[i:]); n > 0 {
				b.WriteRune(r)
				i += n
				continue
			}
			b.WriteByte(s[i])
			i++
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// charRef parses a numeric character reference (&#xHHHH; or &#DDDD;) at the start
// of s, returning the rune and the number of bytes consumed, or n == 0 if s does
// not begin with a valid reference.
func charRef(s string) (rune, int) {
	if len(s) < 4 || s[0] != '&' || s[1] != '#' {
		return 0, 0
	}
	j, base := 2, 10
	if s[j] == 'x' || s[j] == 'X' {
		base, j = 16, j+1
	}
	start := j
	for j < len(s) && s[j] != ';' {
		if !isRefDigit(s[j], base) {
			return 0, 0 // reject signs and stray characters (e.g. "&#+65;")
		}
		j++
	}
	if j >= len(s) || j == start {
		return 0, 0
	}
	n, err := strconv.ParseInt(s[start:j], base, 32)
	if err != nil || n < 0 || n > 0x10FFFF {
		return 0, 0
	}
	r := rune(n)
	// Reject a reference to a code point the line format cannot carry (a line
	// break) or that is not a valid Unicode scalar (a surrogate).
	if r == '\n' || r == '\r' || (r >= 0xD800 && r <= 0xDFFF) {
		return 0, 0
	}
	return r, j + 1
}

// isRefDigit reports whether b is a valid digit for a base-10 or base-16 numeric
// character reference, excluding signs.
func isRefDigit(b byte, base int) bool {
	if b >= '0' && b <= '9' {
		return true
	}
	if base == 16 {
		return (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
	}
	return false
}

// Reader reads MARC records from a .mrk stream one record at a time.
type Reader struct {
	br *bufio.Reader
}

// compile-time assertion that Reader satisfies the core interface.
var _ codex.RecordReader = (*Reader)(nil)

// NewReader returns a Reader that reads records from r.
func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReader(r)}
}

// Read returns the next record, or io.EOF when the stream is exhausted. A record
// is the run of "=" lines up to the next blank line or end of input.
func (rd *Reader) Read() (*codex.Record, error) {
	var rec *codex.Record
	for {
		line, err := rd.br.ReadString('\n')
		text := strings.TrimRight(line, "\r\n")
		switch {
		case text == "":
			if rec != nil {
				return rec, nil // blank line ends the record
			}
		case text[0] == '=':
			if rec == nil {
				rec = codex.NewRecord()
			}
			if perr := parseLine(rec, text); perr != nil {
				return nil, perr
			}
		}
		if err != nil {
			if rec != nil {
				return rec, nil // last record at end of input
			}
			return nil, err // io.EOF or a read error
		}
	}
}

// parseLine adds the field described by one "=" line to rec. A field line is
// "=TAG  DATA": "=", a 3-character tag, exactly two spaces, then the data. It
// returns an error if the line does not have that structure, so a misframed line
// is reported rather than silently shifting indicators into the data.
func parseLine(rec *codex.Record, line string) error {
	if len(line) < 6 || line[0] != '=' || line[4] != ' ' || line[5] != ' ' {
		return fmt.Errorf("mrk: malformed line %q", line)
	}
	tag := line[1:4]
	data := line[6:]

	switch {
	case tag == "LDR":
		rec.SetLeader(codex.Leader(unescape(data)))
	case tag < "010":
		rec.AddField(codex.NewControlField(tag, unescape(data)))
	default:
		f := codex.Field{Tag: tag, Ind1: ' ', Ind2: ' '}
		if len(data) >= 1 {
			f.Ind1 = indByte(data[0])
		}
		if len(data) >= 2 {
			f.Ind2 = indByte(data[1])
		}
		subs := ""
		if len(data) > 2 {
			subs = data[2:]
		}
		for part := range strings.SplitSeq(subs, "$") {
			if part != "" {
				f.Subfields = append(f.Subfields, codex.Subfield{Code: part[0], Value: unescape(part[1:])})
			}
		}
		rec.AddField(f)
	}
	return nil
}

// All returns an iterator over the remaining records, for use as
// "for rec, err := range r.All()". It stops at the first error.
func (rd *Reader) All() iter.Seq2[*codex.Record, error] {
	return codex.All(rd)
}

// Decode parses a single record from a .mrk byte slice.
func Decode(b []byte) (*codex.Record, error) {
	rec, err := NewReader(bytes.NewReader(b)).Read()
	if err == io.EOF {
		return nil, fmt.Errorf("mrk: no record found")
	}
	return rec, err
}

// ReadFile reads every record from the named .mrk file. On the first malformed
// record it returns the records parsed so far together with the error.
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

// Writer writes records to a .mrk stream, separating them with a blank line.
type Writer struct {
	w      io.Writer
	buf    []byte // reused across writes
	closed bool
	err    error
}

// compile-time assertion that Writer satisfies the core interface.
var _ codex.RecordWriter = (*Writer)(nil)

// NewWriter returns a Writer that writes records to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Write serializes one record and writes it, followed by a blank line. After a
// write error the Writer is sticky: every later call returns the same error.
func (wr *Writer) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return fmt.Errorf("mrk: Write after Close")
	}
	if err := validate(r); err != nil {
		return err
	}
	wr.buf = appendRecord(wr.buf[:0], r)
	wr.buf = append(wr.buf, '\n') // blank line separating records
	if _, err := wr.w.Write(wr.buf); err != nil {
		wr.err = err
	}
	return wr.err
}

// Close reports the first write error, if any, and rejects further writes. The
// mrk stream needs no trailer, so Close flushes nothing; it exists for parity
// with the marcxml and marcjson writers.
func (wr *Writer) Close() error {
	if wr.err != nil {
		return wr.err
	}
	wr.closed = true
	return nil
}

// WriteFile writes every record to the named file, creating it or truncating an
// existing file.
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
	return f.Close()
}
