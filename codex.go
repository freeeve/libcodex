// Package codex is a format-agnostic in-memory model for MARC 21 bibliographic
// records, shared by the serialization subpackages.
//
// A Record is a 24-byte Leader plus an ordered list of Fields. A control field
// (tag below "010") holds raw data; a data field carries two indicators and zero
// or more Subfields. Every MARC serialization maps onto this one model, so the
// same record round-trips through any format.
//
// The model is domain-agnostic: it exposes leaders, fields, subfields and
// indicators, and leaves interpretation of specific tags to the caller.
//
// Serialization lives in subpackages, each implementing RecordReader and
// RecordWriter:
//
//   - iso2709 — the binary ISO 2709 interchange format (.mrc)
//   - marcxml — the Library of Congress MARCXML slim schema (planned)
//   - marcjson — the MARC-in-JSON structure (planned)
//   - mrk — the MARCMaker mnemonic text format (planned)
//
// All values exposed by this package are UTF-8 Go strings; each codec is
// responsible for transcoding its wire encoding to and from UTF-8.
package codex

import (
	"fmt"
	"io"
	"iter"
	"strconv"
)

// RecordReader reads MARC records one at a time from an underlying stream. Each
// format subpackage provides an implementation; Read returns io.EOF when the
// stream is exhausted.
type RecordReader interface {
	Read() (*Record, error)
}

// RecordWriter serializes MARC records to an underlying stream. Each format
// subpackage provides an implementation.
type RecordWriter interface {
	Write(*Record) error
}

// Convert reads every record from r and writes it to w, returning the first
// error encountered (io.EOF is the normal end and is not returned). Because it
// is written against the interfaces, it converts between any two formats. A
// writer that buffers a wrapper (e.g. a marcxml <collection> or a marcjson
// array) still needs the caller to call its Close method afterward to finalize
// the output.
func Convert(r RecordReader, w RecordWriter) error {
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
}

// All returns an iterator over the records produced by r, for use as
//
//	for rec, err := range codex.All(reader) { ... }
//
// It stops at the first error, yielding (nil, err) once; io.EOF ends iteration
// cleanly without yielding. It works with any RecordReader, so every format
// shares one iterator.
func All(r RecordReader) iter.Seq2[*Record, error] {
	return func(yield func(*Record, error) bool) {
		for {
			rec, err := r.Read()
			if err == io.EOF {
				return
			}
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(rec, nil) {
				return
			}
		}
	}
}

// defaultLeaderTemplate is a syntactically valid 24-byte leader used for records
// built from scratch. Byte 9 ('a') marks UTF-8 encoding; codecs recompute the
// length [0:5] and base address [12:17] on write.
const defaultLeaderTemplate = "00000nam a2200000   4500"

// leaderLen is the fixed MARC leader length in bytes.
const leaderLen = 24

// Subfield is a single subfield of a MARC data field: a one-byte code and its
// UTF-8 value.
type Subfield struct {
	Code  byte
	Value string
}

// NewSubfield constructs a subfield with the given code and value.
func NewSubfield(code byte, value string) Subfield {
	return Subfield{Code: code, Value: value}
}

// Field is a single MARC field. A control field (Tag < "010") carries raw data
// in Value and has no indicators or subfields. A data field (Tag >= "010")
// carries two indicators and zero or more subfields. The conventional blank
// indicator is the ASCII space (' '); an unset (zero) data-field indicator is
// treated as a blank space when the field is serialized.
type Field struct {
	Tag       string
	Ind1      byte
	Ind2      byte
	Value     string
	Subfields []Subfield
}

// NewControlField constructs a control field (e.g. 001, 003, 008) holding raw
// data. The tag is not validated here; values below "010" are treated as
// control fields by the reader and writer.
func NewControlField(tag, value string) Field {
	return Field{Tag: tag, Value: value}
}

// NewDataField constructs a data field with the given tag, two indicators and
// subfields. A blank indicator is conventionally the ASCII space (' ').
func NewDataField(tag string, ind1, ind2 byte, subfields ...Subfield) Field {
	return Field{Tag: tag, Ind1: ind1, Ind2: ind2, Subfields: subfields}
}

// IsControl reports whether the field is a control field (tag below "010").
func (f Field) IsControl() bool {
	return f.Tag < "010"
}

// Indicators returns the field's two indicator bytes. For control fields both
// are zero.
func (f Field) Indicators() (byte, byte) {
	return f.Ind1, f.Ind2
}

// Subfield returns the value of the first subfield with the given code and
// reports whether one was found.
func (f Field) Subfield(code byte) (string, bool) {
	for _, s := range f.Subfields {
		if s.Code == code {
			return s.Value, true
		}
	}
	return "", false
}

// SubfieldValue returns the value of the first subfield with the given code, or
// the empty string if none is present.
func (f Field) SubfieldValue(code byte) string {
	v, _ := f.Subfield(code)
	return v
}

// SubfieldValues returns the values of every subfield with the given code, in
// order.
func (f Field) SubfieldValues(code byte) []string {
	var out []string
	for _, s := range f.Subfields {
		if s.Code == code {
			out = append(out, s.Value)
		}
	}
	return out
}

// Leader is the 24-byte MARC record leader. Helper methods decode the fields
// this package needs; the raw bytes are available via String.
type Leader string

// String returns the leader as a 24-byte string.
func (l Leader) String() string {
	return string(l)
}

// RecordStatus returns leader byte 5 (the record status), or 0 if the leader is
// malformed.
func (l Leader) RecordStatus() byte {
	return l.byteAt(5)
}

// RecordType returns leader byte 6 (the type of record), or 0 if the leader is
// malformed.
func (l Leader) RecordType() byte {
	return l.byteAt(6)
}

// BibLevel returns leader byte 7 (the bibliographic level: 'm' monograph, 's'
// serial, 'a' monographic component part, etc.), or 0 if the leader is malformed.
func (l Leader) BibLevel() byte {
	return l.byteAt(7)
}

// Encoding returns leader byte 9, the character coding scheme: 'a' for UTF-8
// (Unicode) or blank for MARC-8.
func (l Leader) Encoding() byte {
	return l.byteAt(9)
}

// IsUnicode reports whether the leader declares UTF-8 (Unicode) encoding.
func (l Leader) IsUnicode() bool {
	return l.Encoding() == 'a'
}

// RecordLength returns the total record length declared in leader bytes [0:5].
// It returns 0 if those bytes are not a valid number.
func (l Leader) RecordLength() int {
	return l.numAt(0, 5)
}

// BaseAddress returns the base address of data declared in leader bytes
// [12:17]. It returns 0 if those bytes are not a valid number.
func (l Leader) BaseAddress() int {
	return l.numAt(12, 17)
}

func (l Leader) byteAt(i int) byte {
	if i < 0 || i >= len(l) {
		return 0
	}
	return l[i]
}

func (l Leader) numAt(start, end int) int {
	if end > len(l) {
		return 0
	}
	n, err := strconv.Atoi(string(l[start:end]))
	if err != nil {
		return 0
	}
	return n
}

// Record is a parsed MARC record: its leader and ordered fields.
type Record struct {
	leader Leader
	fields []Field
}

// NewRecord creates an empty record with a syntactically valid default leader
// (UTF-8 encoding). Fields are added with AddField.
func NewRecord() *Record {
	return &Record{leader: Leader(defaultLeaderTemplate)}
}

// NewRecordCap creates an empty record like NewRecord but with space
// preallocated for n fields. Codecs that know the field count up front use it to
// avoid reallocating the field slice while appending.
func NewRecordCap(n int) *Record {
	return &Record{leader: Leader(defaultLeaderTemplate), fields: make([]Field, 0, n)}
}

// Leader returns the record's leader.
func (r *Record) Leader() Leader {
	return r.leader
}

// SetLeader replaces the record's leader.
func (r *Record) SetLeader(l Leader) *Record {
	r.leader = l
	return r
}

// Encoding returns the record's declared character encoding (leader byte 9).
func (r *Record) Encoding() byte {
	return r.leader.Encoding()
}

// Fields returns all fields in record order.
func (r *Record) Fields() []Field {
	return r.fields
}

// AddField appends a field to the record and returns the record for chaining.
func (r *Record) AddField(f Field) *Record {
	r.fields = append(r.fields, f)
	return r
}

// RemoveFields removes every field with the given tag and returns the record.
func (r *Record) RemoveFields(tag string) *Record {
	kept := r.fields[:0]
	for _, f := range r.fields {
		if f.Tag != tag {
			kept = append(kept, f)
		}
	}
	r.fields = kept
	return r
}

// ReplaceField replaces the first field that shares f's tag with f, or appends f
// when no field has that tag. It returns the record.
func (r *Record) ReplaceField(f Field) *Record {
	for i := range r.fields {
		if r.fields[i].Tag == f.Tag {
			r.fields[i] = f
			return r
		}
	}
	return r.AddField(f)
}

// InsertField inserts f keeping the record ordered by ascending tag: f is placed
// after any existing fields with a tag less than or equal to f's. It returns the
// record. Insert into an already tag-ordered record to keep it ordered.
func (r *Record) InsertField(f Field) *Record {
	i := 0
	for i < len(r.fields) && r.fields[i].Tag <= f.Tag {
		i++
	}
	r.fields = append(r.fields, Field{})
	copy(r.fields[i+1:], r.fields[i:])
	r.fields[i] = f
	return r
}

// ControlField returns the raw value of the first control field with the given
// tag, or the empty string if none is present.
func (r *Record) ControlField(tag string) string {
	for _, f := range r.fields {
		if f.Tag == tag && f.IsControl() {
			return f.Value
		}
	}
	return ""
}

// DataFields returns every data field with the given tag, in order.
func (r *Record) DataFields(tag string) []Field {
	var out []Field
	for _, f := range r.fields {
		if f.Tag == tag && !f.IsControl() {
			out = append(out, f)
		}
	}
	return out
}

// DataField returns the first data field with the given tag and reports whether
// one was found.
func (r *Record) DataField(tag string) (Field, bool) {
	for _, f := range r.fields {
		if f.Tag == tag && !f.IsControl() {
			return f, true
		}
	}
	return Field{}, false
}

// SubfieldValue returns the value of the first subfield with the given code in
// the first data field with the given tag, or the empty string.
func (r *Record) SubfieldValue(tag string, code byte) string {
	for _, f := range r.fields {
		if f.Tag != tag || f.IsControl() {
			continue
		}
		if v, ok := f.Subfield(code); ok {
			return v
		}
	}
	return ""
}

// SubfieldValues returns the values of every subfield with the given code
// across all data fields with the given tag, in order.
func (r *Record) SubfieldValues(tag string, code byte) []string {
	var out []string
	for _, f := range r.fields {
		if f.Tag != tag || f.IsControl() {
			continue
		}
		out = append(out, f.SubfieldValues(code)...)
	}
	return out
}

// Validate reports the first structural problem with the record: a leader that
// is not 24 bytes, a field tag that is not 3 bytes, or a data field with no
// subfields. It returns nil when the record is structurally well-formed. It does
// not check tag semantics, indicator values, or character encoding.
func (r *Record) Validate() error {
	if len(r.leader) != leaderLen {
		return fmt.Errorf("codex: leader is %d bytes, want %d", len(r.leader), leaderLen)
	}
	for _, f := range r.fields {
		if len(f.Tag) != 3 {
			return fmt.Errorf("codex: field tag %q is not 3 bytes", f.Tag)
		}
		if !f.IsControl() && len(f.Subfields) == 0 {
			return fmt.Errorf("codex: data field %s has no subfields", f.Tag)
		}
	}
	return nil
}
