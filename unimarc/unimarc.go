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
//   - The character set is declared in field 100 positions 26-33 (up to four
//     two-character graphic-set codes), not the leader. The reader triggers ISO
//     5426 transcoding when the extended-Latin code "03" appears in any slot,
//     leaves UTF-8 ("50") and base Latin ("01") untouched, and best-effort
//     decodes the unsupported Cyrillic/Greek sets while flagging them via
//     [Reader.Lossy].
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

// Graphic character-set codes from UNIMARC field 100, positions 26-33.
const (
	csBaseLatin = "01" // ISO 646 (ASCII); passes through as UTF-8
	csISO5426   = "03" // ISO 5426 extended Latin
	csUnicode   = "50" // ISO 10646 / UTF-8
)

// charsetKind classifies the transcoding a record needs based on its declared
// graphic character sets.
type charsetKind int

const (
	charUTF8        charsetKind = iota // UTF-8 or base Latin: values kept verbatim
	charLatin5426                      // ISO 5426 extended Latin: transcode
	charUnsupported                    // Cyrillic/Greek: best-effort, flagged lossy
)

// Decode parses one UNIMARC record from its raw ISO 2709 bytes, transcoding values
// to UTF-8 according to the character set declared in field 100. Decode does not
// modify raw.
func Decode(raw []byte) (*codex.Record, error) {
	if len(raw) >= 10 && raw[9] != 'a' {
		raw = append([]byte(nil), raw...) // preserve the caller's buffer
	}
	rec, _, err := decode(raw)
	return rec, err
}

// decode parses raw, whose leader byte 9 it may overwrite in place, and reports
// whether transcoding to UTF-8 was lossy. Callers pass a buffer they own.
func decode(raw []byte) (*codex.Record, bool, error) {
	// Parse the structure with the verbatim (UTF-8) byte path so values are kept as
	// raw bytes; field 100 is ASCII and reads correctly regardless of charset.
	if len(raw) >= 10 {
		raw[9] = 'a' // UNIMARC does not signal its encoding through the leader
	}
	rec, _, err := iso2709.Decode(raw)
	if err != nil {
		return nil, false, err
	}
	switch charset(rec) {
	case charLatin5426:
		out, lossy := retranscode(rec, iso5426.DecodeLossy)
		return out, lossy, nil
	case charUnsupported:
		// No decoder for the declared Cyrillic/Greek set; best-effort Latin decode,
		// always flagged so callers that must not corrupt data can react.
		out, _ := retranscode(rec, iso5426.DecodeLossy)
		return out, true, nil
	default:
		// Unicode (50) or base Latin (01): values are already UTF-8.
		return rec, false, nil
	}
}

// charset inspects the up-to-four graphic-set codes in field 100 $a positions
// 26-33 and classifies the transcoding needed. Extended Latin (code "03") in any
// slot wins; the Cyrillic (02/04) and Greek (05) codes have no decoder here and
// are reported as unsupported. (UNIMARC 100 is a data field whose coded data
// sits in $a.)
func charset(r *codex.Record) charsetKind {
	c := r.SubfieldValue("100", 'a')
	end := len(c)
	if end > 34 {
		end = 34
	}
	unsupported := false
	for i := 26; i+2 <= end; i += 2 {
		switch c[i : i+2] {
		case csISO5426:
			return charLatin5426
		case "02", "04", "05":
			unsupported = true // basic/extended Cyrillic, Greek: no decoder here
		case csBaseLatin, csUnicode:
			// ASCII or UTF-8: values are already valid UTF-8, no transcoding needed
		}
	}
	if unsupported {
		return charUnsupported
	}
	return charUTF8
}

// retranscode returns a copy of r with every value re-decoded from its raw bytes
// through dec (an ISO 5426 decoder), and whether any value decoded lossily.
func retranscode(r *codex.Record, dec func([]byte) (string, bool)) (*codex.Record, bool) {
	lossy := false
	d := func(b []byte) string {
		s, l := dec(b)
		lossy = lossy || l
		return s
	}
	out := codex.NewRecordCap(len(r.Fields())).SetLeader(r.Leader())
	for _, f := range r.Fields() {
		if f.IsControl() {
			out.AddField(codex.NewControlField(f.Tag, d([]byte(f.Value))))
			continue
		}
		nf := codex.Field{Tag: f.Tag, Ind1: f.Ind1, Ind2: f.Ind2}
		for _, s := range f.Subfields {
			nf.Subfields = append(nf.Subfields, codex.Subfield{Code: s.Code, Value: d([]byte(s.Value))})
		}
		out.AddField(nf)
	}
	return out, lossy
}

// Reader streams UNIMARC records from an ISO 2709 byte stream.
type Reader struct {
	br    *bufio.Reader
	lossy bool
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
	rec, lossy, err := decode(raw) // raw is freshly allocated and owned by this Read
	rd.lossy = lossy
	return rec, err
}

// Lossy reports whether the record returned by the most recent successful Read
// required best-effort transcoding that may have dropped characters (an
// undefined ISO 5426 byte, or a declared Cyrillic/Greek set with no decoder).
// Mirrors iso2709.Reader.Lossy.
func (rd *Reader) Lossy() bool { return rd.lossy }

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
