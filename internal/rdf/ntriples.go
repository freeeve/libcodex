package rdf

import (
	"strings"
	"unicode/utf8"
)

// ParseNTriples parses an N-Triples document (one "subject predicate object ."
// statement per line) into a Graph. It also accepts N-Quads, ignoring any fourth
// (graph) term. Blank, comment and malformed lines are skipped, so it is robust to
// the trailing noise real-world dumps carry.
func ParseNTriples(data []byte) (*Graph, error) {
	g := &Graph{}
	for line := range strings.SplitSeq(string(data), "\n") {
		if tr, ok := parseNTLine(line); ok {
			g.Add(tr.S, tr.P, tr.O)
		}
	}
	return g, nil
}

// parseNTLine parses one N-Triples/N-Quads line, returning false for blank,
// comment or malformed lines.
func parseNTLine(line string) (Triple, bool) {
	s := strings.TrimSpace(line)
	if s == "" || s[0] == '#' {
		return Triple{}, false
	}
	subj, s, ok := readNTTerm(s)
	if !ok {
		return Triple{}, false
	}
	pred, s, ok := readNTTerm(strings.TrimLeft(s, " \t"))
	if !ok || !pred.IsIRI() {
		return Triple{}, false
	}
	obj, _, ok := readNTTerm(strings.TrimLeft(s, " \t"))
	if !ok {
		return Triple{}, false
	}
	return Triple{subj, pred, obj}, true
}

// readNTTerm reads one term from the front of s, returning the term and the
// remaining string.
func readNTTerm(s string) (Term, string, bool) {
	switch {
	case strings.HasPrefix(s, "<"):
		i := strings.IndexByte(s, '>')
		if i < 0 {
			return Term{}, s, false
		}
		return NewIRI(unescapeRDF(s[1:i])), s[i+1:], true
	case strings.HasPrefix(s, "_:"):
		i := strings.IndexAny(s, " \t")
		if i < 0 {
			i = len(s)
		}
		return NewBlank(s[2:i]), s[i:], true
	case strings.HasPrefix(s, `"`):
		return readNTLiteral(s)
	}
	return Term{}, s, false
}

// readNTLiteral reads a quoted literal and any ^^<datatype> or @lang suffix.
func readNTLiteral(s string) (Term, string, bool) {
	var b strings.Builder
	i := 1
	for i < len(s) {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			r, n := unescapeRune(s[i:])
			b.WriteRune(r)
			i += n
			continue
		}
		if c == '"' {
			i++
			break
		}
		b.WriteByte(c)
		i++
	}
	rest := s[i:]
	switch {
	case strings.HasPrefix(rest, "^^<"):
		j := strings.IndexByte(rest, '>')
		if j < 0 {
			return Term{}, s, false
		}
		return NewLiteral(b.String(), "", unescapeRDF(rest[3:j])), rest[j+1:], true
	case strings.HasPrefix(rest, "@"):
		j := strings.IndexAny(rest, " \t")
		if j < 0 {
			j = len(rest)
		}
		return NewLiteral(b.String(), rest[1:j], ""), rest[j:], true
	}
	return NewLiteral(b.String(), "", ""), rest, true
}

// NTriples serializes the graph as N-Triples.
func (g *Graph) NTriples() []byte {
	var b []byte
	for _, t := range g.Triples {
		b = appendNTTerm(b, t.S)
		b = append(b, ' ')
		b = appendNTTerm(b, t.P)
		b = append(b, ' ')
		b = appendNTTerm(b, t.O)
		b = append(b, ' ', '.', '\n')
	}
	return b
}

// appendNTTerm writes a term in N-Triples syntax.
func appendNTTerm(b []byte, t Term) []byte {
	switch t.Kind {
	case IRI:
		b = append(b, '<')
		b = appendEscapedIRI(b, t.Value)
		return append(b, '>')
	case Blank:
		b = append(b, '_', ':')
		return appendBlankLabel(b, t.Value)
	default:
		b = append(b, '"')
		b = appendEscapedLiteral(b, t.Value)
		b = append(b, '"')
		if t.Lang != "" {
			b = append(b, '@')
			return append(b, t.Lang...)
		}
		if t.Datatype != "" && t.Datatype != XSDString {
			b = append(b, "^^<"...)
			b = appendEscapedIRI(b, t.Datatype)
			return append(b, '>')
		}
		return b
	}
}

// ---- shared escaping ----

// appendEscapedIRI escapes the characters not allowed bare in an IRI reference.
func appendEscapedIRI(b []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= 0x20 || c == '<' || c == '>' || c == '"' || c == '{' || c == '}' ||
			c == '|' || c == '^' || c == '`' || c == '\\' {
			b = appendUnicodeEscape(b, rune(c))
			continue
		}
		b = append(b, c)
	}
	return b
}

// appendEscapedLiteral escapes a literal's lexical form for Turtle/N-Triples.
func appendEscapedLiteral(b []byte, s string) []byte {
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			switch c {
			case '\\':
				b = append(b, '\\', '\\')
			case '"':
				b = append(b, '\\', '"')
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			default:
				if c < 0x20 {
					b = appendUnicodeEscape(b, rune(c))
				} else {
					b = append(b, c)
				}
			}
			i++
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

// appendBlankLabel writes a blank-node label, keeping only the characters valid
// in an N-Triples/Turtle blank-node identifier (others become '_'); an empty label
// becomes "b".
func appendBlankLabel(b []byte, label string) []byte {
	start := len(b)
	for i := 0; i < len(label); i++ {
		c := label[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	if len(b) == start {
		b = append(b, 'b')
	}
	return b
}

func appendUnicodeEscape(b []byte, r rune) []byte {
	const hex = "0123456789ABCDEF"
	return append(b, '\\', 'u', hex[(r>>12)&0xf], hex[(r>>8)&0xf], hex[(r>>4)&0xf], hex[r&0xf])
}

// unescapeRDF unescapes \uXXXX, \UXXXXXXXX and the string escapes in an IRI or
// literal lexical form.
func unescapeRDF(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) {
			r, n := unescapeRune(s[i:])
			b.WriteRune(r)
			i += n
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// unescapeRune decodes one backslash escape at the front of s, returning the rune
// and the number of bytes consumed.
func unescapeRune(s string) (rune, int) {
	if len(s) < 2 {
		return '\\', 1
	}
	switch s[1] {
	case 'n':
		return '\n', 2
	case 'r':
		return '\r', 2
	case 't':
		return '\t', 2
	case 'b':
		return '\b', 2
	case 'f':
		return '\f', 2
	case '"':
		return '"', 2
	case '\'':
		return '\'', 2
	case '\\':
		return '\\', 2
	case 'u':
		if len(s) >= 6 {
			if r, ok := parseHex(s[2:6]); ok {
				return r, 6
			}
		}
	case 'U':
		if len(s) >= 10 {
			if r, ok := parseHex(s[2:10]); ok {
				return r, 10
			}
		}
	}
	return rune(s[1]), 2
}

func parseHex(s string) (rune, bool) {
	var r rune
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			r = r<<4 | rune(c-'0')
		case c >= 'a' && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		case c >= 'A' && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return r, true
}
