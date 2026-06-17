// Package unimarc reads UNIMARC bibliographic records and maps them to the MARC
// 21 model (the codex.Record), so a UNIMARC record flows through every libcodex
// exporter (mods, dublincore, citation, bibframe, schemaorg).
//
// UNIMARC shares the ISO 2709 physical structure with MARC 21 but differs in two
// ways this package handles:
//
//   - The data dictionary: different tag semantics (title is 200, ISBN is 010,
//     authors are 700/701/710, …). [Title], [Authors] and friends interpret the
//     common UNIMARC fields, and [ToMARC21] re-tags a record to MARC 21.
//   - The character set is declared in field 100 positions 26-27, not the leader.
//     The reader selects UTF-8 (code "50"), ISO 5426 (code "01"/"02") or a
//     best-effort fallback accordingly, transcoding values to UTF-8.
package unimarc

import (
	"bufio"
	"fmt"
	"io"
	"iter"
	"os"
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/iso5426"
	"github.com/freeeve/libcodex/iso2709"
)

// Character-set codes from UNIMARC field 100, positions 26-27.
const (
	csUnicode = "50" // ISO 10646 / UTF-8
	csISO5426 = "01" // ISO 5426 extended Latin
)

// Decode parses one UNIMARC record from its raw ISO 2709 bytes, transcoding values
// to UTF-8 according to the character set declared in field 100.
func Decode(raw []byte) (*codex.Record, error) {
	// Parse the structure with the verbatim (UTF-8) byte path so values are kept as
	// raw bytes; field 100 is ASCII and reads correctly regardless of charset.
	rec, _, err := iso2709.Decode(forceUTF8Leader(raw))
	if err != nil {
		return nil, err
	}
	switch charsetCode(rec) {
	case csISO5426, "02":
		return retranscode(rec, iso5426.Decode), nil
	default:
		// Unicode (50) or unspecified: values are already UTF-8.
		return rec, nil
	}
}

// charsetCode returns the UNIMARC character-set code from field 100 $a positions
// 26-27, or "". (UNIMARC 100 is a data field whose coded data sits in $a.)
func charsetCode(r *codex.Record) string {
	if c := r.SubfieldValue("100", 'a'); len(c) >= 28 {
		return c[26:28]
	}
	return ""
}

// forceUTF8Leader returns raw with leader byte 9 set to 'a', so iso2709 keeps
// values as verbatim bytes instead of applying MARC-8 transcoding (UNIMARC does
// not signal its encoding through the leader).
func forceUTF8Leader(raw []byte) []byte {
	if len(raw) < 10 || raw[9] == 'a' {
		return raw
	}
	b := append([]byte(nil), raw...)
	b[9] = 'a'
	return b
}

// retranscode returns a copy of r with every value re-decoded from its raw bytes
// through dec (an ISO 5426 / Latin-1 decoder).
func retranscode(r *codex.Record, dec func([]byte) string) *codex.Record {
	out := codex.NewRecordCap(len(r.Fields())).SetLeader(r.Leader())
	for _, f := range r.Fields() {
		if f.IsControl() {
			out.AddField(codex.NewControlField(f.Tag, dec([]byte(f.Value))))
			continue
		}
		nf := codex.Field{Tag: f.Tag, Ind1: f.Ind1, Ind2: f.Ind2}
		for _, s := range f.Subfields {
			nf.Subfields = append(nf.Subfields, codex.Subfield{Code: s.Code, Value: dec([]byte(s.Value))})
		}
		out.AddField(nf)
	}
	return out
}

// Reader streams UNIMARC records from an ISO 2709 byte stream.
type Reader struct {
	br *bufio.Reader
}

var _ codex.RecordReader = (*Reader)(nil)

// NewReader returns a Reader over an ISO 2709 UNIMARC stream.
func NewReader(r io.Reader) *Reader { return &Reader{br: bufio.NewReader(r)} }

// Read returns the next record, or io.EOF at the end of the stream.
func (rd *Reader) Read() (*codex.Record, error) {
	var head [5]byte
	if _, err := io.ReadFull(rd.br, head[:]); err != nil {
		if err == io.ErrUnexpectedEOF {
			return nil, io.EOF // trailing bytes (e.g. a final newline) after the last record
		}
		return nil, err // io.EOF at a clean boundary
	}
	n, ok := atoi5(head[:])
	if !ok || n < 5 {
		return nil, fmt.Errorf("unimarc: invalid record length %q", head)
	}
	raw := make([]byte, n)
	copy(raw, head[:])
	if _, err := io.ReadFull(rd.br, raw[5:]); err != nil {
		return nil, fmt.Errorf("unimarc: short record: %w", err)
	}
	return Decode(raw)
}

// All returns an iterator over the records in the stream, stopping at the first
// error.
func (rd *Reader) All() iter.Seq2[*codex.Record, error] {
	return func(yield func(*codex.Record, error) bool) {
		for {
			rec, err := rd.Read()
			if err == io.EOF {
				return
			}
			if !yield(rec, err) || err != nil {
				return
			}
		}
	}
}

// ReadFile reads every UNIMARC record from the named file.
func ReadFile(path string) ([]*codex.Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []*codex.Record
	for rec, err := range NewReader(f).All() {
		if err != nil {
			return out, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func atoi5(b []byte) (int, bool) {
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// clean strips the C1 non-sort / formatting control characters (U+0080-U+009F)
// that UNIMARC embeds to mark non-filing portions, leaving display text.
func clean(s string) string {
	if !strings.ContainsFunc(s, isC1) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isC1(r) {
			return -1
		}
		return r
	}, s)
}

func isC1(r rune) bool { return r >= 0x80 && r <= 0x9F }
