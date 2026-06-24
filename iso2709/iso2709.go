// Package iso2709 reads and writes MARC 21 records in the binary ISO 2709
// interchange format (.mrc), implementing codex.RecordReader and
// codex.RecordWriter.
//
// An ISO 2709 record is a 24-byte leader, a directory of fixed 12-byte entries,
// and the variable fields, terminated by a record terminator. Fields are either
// control fields (tags below "010"), which hold raw data, or data fields, which
// carry two indicators followed by delimited subfields.
//
// Character encoding: MARC 21 with UTF-8 (leader byte 9 == 'a') is the primary,
// preferred form. Records flagged as MARC-8 (leader byte 9 == blank) are
// transcoded to UTF-8 on read for the Western subset (Basic Latin plus ANSEL
// Extended Latin, including combining diacritics); see the internal/marc8
// package and the README for scope and limitations. Writing always emits UTF-8
// and sets leader byte 9 to 'a'.
//
// Conformance notes: the encoder always emits the MARC 21 fixed leader geometry
// (2 indicators, 1-character subfield codes, and a 3+4+5-byte directory entry,
// entry map "4500"); the decoder honors a non-default indicator count and entry
// map declared in leader bytes 10-11 and 20-22, so well-formed non-standard
// ISO 2709 directories are not misparsed. The MARC 21 fill character (0x7C, "|")
// is treated as ordinary data and passed through unchanged; note that a fill in
// leader byte 9 reads as MARC-8. MARC-8 transcoding composes common Latin
// base+mark pairs to NFC but does not apply full Unicode normalization, so mixed
// NFC/NFD output is possible, and UTF-8 input is passed through unchanged.
package iso2709

import (
	"fmt"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/marc8"
)

// MARC 21 control characters used as field, subfield and record delimiters.
const (
	// SubfieldDelimiter (0x1f) precedes each subfield code within a data field.
	SubfieldDelimiter = 0x1f
	// FieldTerminator (0x1e) ends each variable field and the directory.
	FieldTerminator = 0x1e
	// RecordTerminator (0x1d) ends a complete record.
	RecordTerminator = 0x1d
)

const (
	leaderLen = 24
	// dirEntryLen is the MARC 21 fixed directory entry size the encoder emits:
	// 3-byte tag + 4-digit length + 5-digit start position. (The decoder reads
	// the entry geometry from the leader instead of assuming this.)
	dirEntryLen = 12
)

// defaultLeaderTemplate is a syntactically valid 24-byte leader used as the
// fallback when a record carries a malformed leader on write.
const defaultLeaderTemplate = "00000nam a2200000   4500"

// Decode parses a single ISO 2709 record from its raw bytes (the leader,
// directory, fields, and optionally the trailing record terminator). Directory
// entries that point outside the field data are skipped. If the leader declares
// MARC-8 encoding, values are transcoded to UTF-8.
//
// The returned bool reports whether decoding fell back to best-effort passthrough
// of an unrecognized MARC-8 designation or an unmapped character, meaning some
// values may contain mojibake; it is always false for UTF-8 records. Re-serializing
// a lossy record as UTF-8 persists the mojibake, so callers that must not corrupt
// data should check it.
func Decode(b []byte) (*codex.Record, bool, error) {
	if len(b) < leaderLen {
		return nil, false, fmt.Errorf("iso2709: record too short: %d bytes", len(b))
	}
	// bs is a private copy, so the returned record's substrings are safe even if the
	// caller reuses b. ReadFile instead shares one buffer across all records.
	return decode(b, string(b))
}

// decode parses a record from b using bs as its string form. bs must equal
// string(b); callers pass either a private copy (Decode) or a zero-copy view of a
// retained buffer (ReadFile). The leader, UTF-8 tags and plain-ASCII values are
// taken as zero-copy substrings of bs.
func decode(b []byte, bs string) (*codex.Record, bool, error) {
	if len(b) < leaderLen {
		return nil, false, fmt.Errorf("iso2709: record too short: %d bytes", len(b))
	}

	base, ok := atoiBytes(b[12:17])
	if !ok {
		return nil, false, fmt.Errorf("iso2709: invalid base address %q", b[12:17])
	}
	if base <= leaderLen || base > len(b) {
		return nil, false, fmt.Errorf("iso2709: base address %d out of range (record length %d)", base, len(b))
	}

	// MARC 21 fixes the directory geometry to a 3-byte tag, 4-digit length and
	// 5-digit start position (leader entry map "4500") and 2 indicators, but
	// ISO 2709 lets these vary; honor the leader so a well-formed non-standard
	// record is not misparsed. Non-digit positions fall back to the MARC 21
	// defaults, preserving tolerance of slightly malformed leaders.
	lenWidth := leaderDigit(b, 20, 4)
	posWidth := leaderDigit(b, 21, 5)
	implWidth := leaderDigit(b, 22, 0)
	if lenWidth < 1 {
		lenWidth = 4
	}
	if posWidth < 1 {
		posWidth = 5
	}
	entryLen := 3 + lenWidth + posWidth + implWidth
	indCount := leaderDigit(b, 10, 2)
	codeLen := max(leaderDigit(b, 11, 2)-1, 1)

	dir := b[leaderLen : base-1]
	body := b[base:]
	unicode := b[9] == 'a'

	rec := codex.NewRecordCap(prealloc(len(dir) / entryLen)).SetLeader(codex.Leader(bs[:leaderLen]))
	subs := make([]codex.Subfield, 0, prealloc(countByte(body, SubfieldDelimiter)))

	// One MARC-8 decoder is reused across the record's transcoded fields, reset to
	// the default working sets at the start of each (designations persist across a
	// field's subfields, not across fields); its Lossy state accumulates.
	var dec *marc8.Decoder
	for i := 0; i+entryLen <= len(dir); i += entryLen {
		tagOff := leaderLen + i
		tag := bs[tagOff : tagOff+3]
		length, ok1 := atoiBytes(b[tagOff+3 : tagOff+3+lenWidth])
		start, ok2 := atoiBytes(b[tagOff+3+lenWidth : tagOff+3+lenWidth+posWidth])
		if !ok1 || !ok2 || start+length > len(body) {
			continue
		}
		lo, hi := base+start, base+start+length
		for hi > lo && b[hi-1] == FieldTerminator { // strip trailing field terminator(s)
			hi--
		}

		// A field whose bytes are all plain ASCII (no byte >= 0x80, no MARC-8 escape)
		// decodes identically in MARC-8 and UTF-8, so it is taken zero-copy from bs
		// rather than transcoded — the common case for control numbers and English
		// headings.
		plain := unicode || !needsTranscode(b[lo:hi])
		if !plain {
			if dec == nil {
				dec = marc8.NewDecoder()
			} else {
				dec.Reset()
			}
		}

		if tag < "010" {
			var v string
			if plain {
				v = bs[lo:hi]
			} else {
				v = dec.Decode(b[lo:hi])
			}
			rec.AddField(codex.Field{Tag: tag, Value: v})
			continue
		}

		f := codex.Field{Tag: tag, Ind1: ' ', Ind2: ' '}
		p := lo
		if hi-lo >= indCount {
			if indCount >= 1 {
				f.Ind1 = b[lo]
			}
			if indCount >= 2 {
				f.Ind2 = b[lo+1]
			}
			p = lo + indCount
		}
		first := len(subs)
		for p < hi {
			if b[p] != SubfieldDelimiter {
				p++
				continue
			}
			q := p + 1
			for q < hi && b[q] != SubfieldDelimiter {
				q++
			}
			if q-(p+1) >= codeLen {
				vlo := p + 1 + codeLen
				var v string
				if plain {
					v = bs[vlo:q]
				} else {
					v = dec.Decode(b[vlo:q])
				}
				subs = append(subs, codex.Subfield{Code: b[p+1], Value: v})
			}
			p = q
		}
		if len(subs) > first { // leave Subfields nil when a data field has none
			f.Subfields = subs[first:len(subs):len(subs)]
		}
		rec.AddField(f)
	}
	// The decoder accumulated lossiness across every transcoded field.
	return rec, dec != nil && dec.Lossy(), nil
}

// leaderDigit returns the decimal digit at leader byte position pos, or def when
// that position is absent or not a digit. It reads the ISO 2709 entry map and
// indicator/subfield-code counts, which MARC 21 fixes but the standard permits
// to vary.
func leaderDigit(b []byte, pos, def int) int {
	if pos >= len(b) || b[pos] < '0' || b[pos] > '9' {
		return def
	}
	return int(b[pos] - '0')
}

// atoiBytes parses b as an unsigned decimal integer without allocating. It
// reports false if b is empty or holds a non-digit, matching the previous
// strconv-based behavior of skipping malformed fixed-width fields.
func atoiBytes(b []byte) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// needsTranscode reports whether a field's bytes contain anything outside plain
// ASCII — a byte >= 0x80, or the MARC-8 escape (0x1b) that designates an alternate
// character set. A field with neither decodes identically under MARC-8 and UTF-8,
// so it can be taken as a zero-copy substring instead of transcoded.
func needsTranscode(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 || c == 0x1b {
			return true
		}
	}
	return false
}

// countByte counts occurrences of c in b without allocating.
func countByte(b []byte, c byte) int {
	n := 0
	for _, x := range b {
		if x == c {
			n++
		}
	}
	return n
}

// prealloc bounds a preallocation hint so a malformed record declaring a huge
// directory cannot trigger an oversized allocation; beyond the cap the slices
// grow on demand instead.
func prealloc(n int) int {
	const max = 1 << 16
	switch {
	case n < 0:
		return 0
	case n > max:
		return max
	default:
		return n
	}
}
