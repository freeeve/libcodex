// Package crosswalk holds the small MARC-field helpers shared by the derivative
// export codecs (mods, dublincore, citation, schemaorg, unimarc): ISBD
// punctuation trimming, subfield joining, subject assembly, year extraction, and
// JSON string escaping. Centralizing them keeps a fix (or a change to the
// punctuation rules) in one place rather than drifting across five packages.
package crosswalk

import (
	"strings"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
)

// TrimISBD trims trailing whitespace and a single trailing ISBD punctuation mark
// (one of "/:;,") together with the whitespace before it, the cleanup MARC
// display data needs when it is repurposed as a plain value.
func TrimISBD(s string) string {
	s = strings.TrimRight(s, " ")
	if n := len(s); n > 0 && strings.IndexByte("/:;,", s[n-1]) >= 0 {
		s = strings.TrimRight(s[:n-1], " ")
	}
	return s
}

// JoinSub joins the ISBD-trimmed values of f's subfields whose codes appear in
// codes, in field order, separated by sep. Empty values are skipped.
func JoinSub(f codex.Field, codes, sep string) string {
	var parts []string
	for _, sf := range f.Subfields {
		if strings.IndexByte(codes, sf.Code) >= 0 {
			if v := TrimISBD(sf.Value); v != "" {
				parts = append(parts, v)
			}
		}
	}
	return strings.Join(parts, sep)
}

// Subject assembles a subject string from a 6XX field: the topical and
// subdivision subfields (a, x, y, z, v) in field order, joined with "--", each
// value trimmed of trailing spaces.
func Subject(f codex.Field) string {
	var parts []string
	for _, sf := range f.Subfields {
		switch sf.Code {
		case 'a', 'x', 'y', 'z', 'v':
			if v := strings.TrimRight(sf.Value, " "); v != "" {
				parts = append(parts, v)
			}
		}
	}
	return strings.Join(parts, "--")
}

// Year returns the first four-consecutive-digit run in s (a publication year
// embedded in a date string), or "" if there is none.
func Year(s string) string {
	for i := 0; i+4 <= len(s); i++ {
		if isDigit(s[i]) && isDigit(s[i+1]) && isDigit(s[i+2]) && isDigit(s[i+3]) {
			return s[i : i+4]
		}
	}
	return ""
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

const hexDigits = "0123456789abcdef"

// AppendJSONString appends s to b as a quoted, escaped JSON string, escaping the
// characters JSON requires and dropping invalid UTF-8 bytes so the output is
// always well-formed.
func AppendJSONString(b []byte, s string) []byte {
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

// AppendXMLText appends s to b as escaped XML character data: it escapes the
// markup-significant runes (& < >) and a carriage return, drops control
// characters XML 1.0 cannot represent (a lossy export), and drops invalid UTF-8
// bytes so the output is always well-formed.
func AppendXMLText(b []byte, s string) []byte {
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
