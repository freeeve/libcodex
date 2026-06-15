package iso2709

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/marc8"
)

// Writer serializes MARC records to an underlying stream as ISO 2709. Output is
// always UTF-8 with leader byte 9 set to 'a'.
type Writer struct {
	w   io.Writer
	buf []byte // reused across writes so streaming does not allocate per record
}

// compile-time assertion that Writer satisfies the core interface.
var _ codex.RecordWriter = (*Writer)(nil)

// NewWriter returns a Writer that writes records to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Write serializes one record and writes it to the stream. It reuses an internal
// buffer, so writing many records allocates only when that buffer must grow.
func (wr *Writer) Write(r *codex.Record) error {
	b, err := EncodeInto(wr.buf[:0], r)
	if err != nil {
		return err
	}
	wr.buf = b
	_, err = wr.w.Write(b)
	return err
}

// WriteFile writes every record to the named file in order, creating it or
// truncating an existing file. A single Writer is used so the encode buffer is
// reused across records.
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

// Encode serializes a record to a new ISO 2709 byte slice. It is EncodeInto with
// a nil destination; use EncodeInto to reuse a buffer across records.
func Encode(r *codex.Record) ([]byte, error) {
	return EncodeInto(nil, r)
}

// EncodeMARC8 serializes a record to ISO 2709 with MARC-8 encoded values and
// leader byte 9 set to blank, for systems that require legacy MARC-8 rather than
// UTF-8. It returns an error if any value contains a character outside the
// supported MARC-8 subset (ASCII plus ANSEL Extended Latin; see internal/marc8).
func EncodeMARC8(r *codex.Record) ([]byte, error) {
	m8, err := toMARC8(r)
	if err != nil {
		return nil, err
	}
	b, err := EncodeInto(nil, m8)
	if err != nil {
		return nil, err
	}
	b[9] = ' ' // mark the record MARC-8; EncodeInto forces 'a' (UTF-8)
	return b, nil
}

// toMARC8 returns a copy of r with every value transcoded from UTF-8 to MARC-8.
func toMARC8(r *codex.Record) (*codex.Record, error) {
	out := codex.NewRecordCap(len(r.Fields())).SetLeader(r.Leader())
	for _, f := range r.Fields() {
		if f.IsControl() {
			v, err := marc8.Encode(f.Value)
			if err != nil {
				return nil, fmt.Errorf("iso2709: control field %s: %w", f.Tag, err)
			}
			out.AddField(codex.NewControlField(f.Tag, string(v)))
			continue
		}
		nf := codex.Field{Tag: f.Tag, Ind1: f.Ind1, Ind2: f.Ind2}
		for _, s := range f.Subfields {
			v, err := marc8.Encode(s.Value)
			if err != nil {
				return nil, fmt.Errorf("iso2709: field %s subfield %q: %w", f.Tag, s.Code, err)
			}
			nf.Subfields = append(nf.Subfields, codex.Subfield{Code: s.Code, Value: string(v)})
		}
		out.AddField(nf)
	}
	return out, nil
}

// EncodeInto appends the ISO 2709 encoding of r to dst and returns the extended
// slice, growing dst once when needed. It computes the leader's record length
// [0:5] and base address [12:17], builds the directory (3-byte tag, 4-digit
// length, 5-digit offset per field) and emits the subfield, field and record
// delimiters; the leader geometry is forced to the MARC 21 fixed values (UTF-8,
// 2 indicators, "4500").
//
// It returns dst unchanged with an error if a tag is not three bytes, a field is
// too long to encode in four digits, the record is too long for five digits, or
// any field datum contains a reserved structural delimiter byte (0x1d/0x1e/0x1f)
// that the binary format cannot carry.
func EncodeInto(dst []byte, r *codex.Record) ([]byte, error) {
	fields := r.Fields()

	// Pre-pass: validate tags and delimiters, and sum the exact field-data size so
	// dst grows at most once below.
	dataLen := 0
	for _, f := range fields {
		if len(f.Tag) != 3 {
			return dst, fmt.Errorf("iso2709: field tag %q must be 3 bytes", f.Tag)
		}
		if err := checkDelimiters(f); err != nil {
			return dst, err
		}
		flen := fieldLen(f)
		if flen > 9999 {
			return dst, fmt.Errorf("iso2709: field %s length %d exceeds 9999 bytes", f.Tag, flen)
		}
		dataLen += flen
	}

	base := leaderLen + len(fields)*dirEntryLen + 1 // +1 for the directory terminator
	total := base + dataLen + 1
	if base > 99999 || total > 99999 {
		return dst, fmt.Errorf("iso2709: record too long to encode (length %d, base %d)", total, base)
	}

	dst = slices.Grow(dst, total)

	var leader [leaderLen]byte
	writeLeader(leader[:], r.Leader(), total, base)
	dst = append(dst, leader[:]...)

	start := 0
	for _, f := range fields {
		flen := fieldLen(f)
		dst = append(dst, f.Tag...)
		dst = appendFixed(dst, flen, 4)
		dst = appendFixed(dst, start, 5)
		start += flen
	}
	dst = append(dst, FieldTerminator) // directory terminator

	for _, f := range fields {
		dst = appendField(dst, f)
	}
	dst = append(dst, RecordTerminator)
	return dst, nil
}

// checkDelimiters reports an error if any datum in f contains a reserved
// structural delimiter byte. ISO 2709 uses 0x1d/0x1e/0x1f as record, field and
// subfield separators, so a value, indicator or subfield code containing one
// would corrupt the record on read.
func checkDelimiters(f codex.Field) error {
	if f.IsControl() {
		if hasDelimiter(f.Value) {
			return fmt.Errorf("iso2709: control field %s value contains a reserved delimiter byte", f.Tag)
		}
		return nil
	}
	if isDelimiter(f.Ind1) || isDelimiter(f.Ind2) {
		return fmt.Errorf("iso2709: field %s indicator is a reserved delimiter byte", f.Tag)
	}
	for _, s := range f.Subfields {
		if isDelimiter(s.Code) {
			return fmt.Errorf("iso2709: field %s subfield code is a reserved delimiter byte", f.Tag)
		}
		if hasDelimiter(s.Value) {
			return fmt.Errorf("iso2709: field %s subfield %q value contains a reserved delimiter byte", f.Tag, s.Code)
		}
	}
	return nil
}

// isDelimiter reports whether b is an ISO 2709 structural delimiter.
func isDelimiter(b byte) bool {
	return b == SubfieldDelimiter || b == FieldTerminator || b == RecordTerminator
}

// hasDelimiter reports whether s contains any structural delimiter byte.
func hasDelimiter(s string) bool {
	for i := 0; i < len(s); i++ {
		if isDelimiter(s[i]) {
			return true
		}
	}
	return false
}

// fieldLen returns the number of bytes appendField writes for f, including the
// trailing field terminator.
func fieldLen(f codex.Field) int {
	if f.IsControl() {
		return len(f.Value) + 1
	}
	n := 3 // two indicators + field terminator
	for _, s := range f.Subfields {
		n += 2 + len(s.Value) // subfield delimiter + code + value
	}
	return n
}

// appendFixed appends n as a width-digit, zero-padded decimal to dst.
func appendFixed(dst []byte, n, width int) []byte {
	var tmp [20]byte
	for i := width - 1; i >= 0; i-- {
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return append(dst, tmp[:width]...)
}

// appendField appends the serialized form of one field (including its trailing
// field terminator) to data.
func appendField(data []byte, f codex.Field) []byte {
	if f.IsControl() {
		data = append(data, f.Value...)
		return append(data, FieldTerminator)
	}
	data = append(data, indicator(f.Ind1), indicator(f.Ind2))
	for _, s := range f.Subfields {
		data = append(data, SubfieldDelimiter, s.Code)
		data = append(data, s.Value...)
	}
	return append(data, FieldTerminator)
}

// indicator returns a usable indicator byte, defaulting an unset (zero) value to
// a blank space.
func indicator(b byte) byte {
	if b == 0 {
		return ' '
	}
	return b
}

// writeLeader fills dst (the 24-byte leader region of the output) from the
// record's leader, then writes the computed record length and base address and
// forces the MARC 21 fixed geometry the encoder emits: UTF-8 (byte 9 = 'a'), 2
// indicators, 1-character subfield codes and the "4500" entry map.
func writeLeader(dst []byte, l codex.Leader, total, base int) {
	if len(l) == leaderLen {
		copy(dst, l)
	} else {
		copy(dst, defaultLeaderTemplate)
	}
	writeFixed(dst[0:5], total)
	writeFixed(dst[12:17], base)
	dst[9] = 'a'
	dst[10] = '2'
	dst[11] = '2'
	copy(dst[20:24], "4500")
}

// writeFixed writes n into dst as a zero-padded decimal, least-significant digit
// last, filling exactly len(dst) bytes and truncating any higher-order digits.
func writeFixed(dst []byte, n int) {
	for i := len(dst) - 1; i >= 0; i-- {
		dst[i] = byte('0' + n%10)
		n /= 10
	}
}
