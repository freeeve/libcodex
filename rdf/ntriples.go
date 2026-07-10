package rdf

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// SyntaxError reports a line that is neither blank, nor a comment, nor a valid
// N-Triples/N-Quads statement. Line is 1-based; Text is the offending line,
// trimmed and truncated for the message.
//
// It exists so that a truncated document is an error rather than a smaller graph.
// A parser that skips what it cannot read turns a half-written dump into a
// well-formed, wrong answer, and no caller downstream can tell.
type SyntaxError struct {
	Line int    // 1-based line number
	Text string // the offending line, trimmed
}

func (e *SyntaxError) Error() string {
	text := e.Text
	if len(text) > 64 {
		text = text[:61] + "..."
	}
	return "rdf: line " + strconv.Itoa(e.Line) + ": malformed N-Triples/N-Quads statement: " + strconv.Quote(text)
}

// lineKind classifies one line of an N-Triples/N-Quads document. Blank and
// comment lines carry no statement but are not errors; a malformed line is.
// Collapsing the two into one "not a statement" bool is what let a truncated
// document parse clean.
type lineKind uint8

const (
	lineStatement lineKind = iota // a well-formed statement
	lineIgnorable                 // blank or comment: no statement, no error
	lineMalformed                 // not parseable as a statement
)

// ParseNTriples parses an N-Triples document (one "subject predicate object ."
// statement per line) into a Graph. It also accepts N-Quads, ignoring any fourth
// (graph) term. Blank and comment lines are skipped; a malformed line is a
// *SyntaxError naming the line number, so a truncated document does not parse as
// a smaller graph. One private copy of the input backs every term, so data is
// free for reuse once it returns.
//
// Use [NewDecoder] with [Decoder.SkipMalformed] to tolerate the trailing noise
// some real-world dumps carry.
func ParseNTriples(data []byte) (*Graph, error) {
	return parseNTriples(string(data))
}

// ParseNTriplesShared is ParseNTriples without the private input copy: terms
// are zero-copy views into data itself, saving one input-sized allocation. In
// exchange the caller must not modify data while the Graph, or any Term drawn
// from it, remains in use.
func ParseNTriplesShared(data []byte) (*Graph, error) {
	return parseNTriples(bytesView(data))
}

func parseNTriples(data string) (*Graph, error) {
	// Zero-copy substrings of data back every term; preallocate the triple
	// slice from the line count so it never grows.
	g := &Graph{Triples: make([]Triple, 0, strings.Count(data, "\n")+1)}
	var a arena
	n := 0
	for line := range strings.SplitSeq(data, "\n") {
		n++
		switch tr, kind := parseNTLine(line, &a); kind {
		case lineStatement:
			g.Triples = append(g.Triples, tr)
		case lineMalformed:
			return g, &SyntaxError{Line: n, Text: strings.TrimSpace(line)}
		}
	}
	return g, nil
}

// parseNTLine parses one N-Triples/N-Quads line into a triple, dropping any
// fourth (graph) term.
func parseNTLine(line string, a *arena) (Triple, lineKind) {
	q, kind := parseNQuadLine(line, a)
	return q.Triple(), kind
}

// parseNQuadLine parses one N-Triples/N-Quads line into a quad, keeping the
// optional fourth (graph) term; a three-term line falls in the default graph
// (zero-value G).
func parseNQuadLine(line string, a *arena) (Quad, lineKind) {
	s := strings.TrimSpace(line)
	if s == "" || s[0] == '#' {
		return Quad{}, lineIgnorable
	}
	subj, s, ok := readNTTerm(s, a)
	if !ok || subj.IsLiteral() { // a literal subject is not valid RDF
		return Quad{}, lineMalformed
	}
	pred, s, ok := readNTTerm(strings.TrimLeft(s, " \t"), a)
	if !ok || !pred.IsIRI() {
		return Quad{}, lineMalformed
	}
	obj, s, ok := readNTTerm(strings.TrimLeft(s, " \t"), a)
	if !ok {
		return Quad{}, lineMalformed
	}
	// The optional graph label is an IRI or blank node before the terminating
	// '.'; a literal there is not a valid graph name and is ignored.
	var graph Term
	if rest := strings.TrimLeft(s, " \t"); len(rest) > 0 && rest[0] != '.' {
		if g, _, ok := readNTTerm(rest, a); ok && !g.IsLiteral() {
			graph = g
		}
	}
	return Quad{subj, pred, obj, graph}, lineStatement
}

// readNTTerm reads one term from the front of s, returning the term and the
// remaining string.
func readNTTerm(s string, a *arena) (Term, string, bool) {
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
		// A trailing "." is the statement terminator, not part of the label
		// (a label may contain "." but not end with one), so strip it.
		end := i
		for end > 2 && s[end-1] == '.' {
			end--
		}
		return NewBlank(s[2:end]), s[end:], true
	case strings.HasPrefix(s, `"`):
		return readNTLiteral(s, a)
	}
	return Term{}, s, false
}

// readNTLiteral reads a quoted literal and any ^^<datatype> or @lang suffix. The
// lexical form is a zero-copy slice of the input when it has no escapes, and is
// unescaped only when needed.
func readNTLiteral(s string, a *arena) (Term, string, bool) {
	hasEsc := false
	i := 1
	for i < len(s) {
		switch s[i] {
		case '\\':
			hasEsc = true
			i += 2 // skip the escaped character so an escaped quote is not the close
		case '"':
			value := s[1:i]
			if hasEsc {
				if a != nil {
					value = a.unescape(value) // arena-backed (whole-document parse)
				} else {
					value = unescapeRDF(value) // streaming: each triple owns its strings
				}
			}
			return ntLiteralSuffix(value, s[i+1:])
		default:
			i++
		}
	}
	return Term{}, s, false // unterminated
}

// ntLiteralSuffix attaches a ^^<datatype> or @lang suffix to a literal value.
func ntLiteralSuffix(value, rest string) (Term, string, bool) {
	switch {
	case strings.HasPrefix(rest, "^^<"):
		j := strings.IndexByte(rest, '>')
		if j < 0 {
			return Term{}, rest, false
		}
		return NewLiteral(value, "", unescapeRDF(rest[3:j])), rest[j+1:], true
	case strings.HasPrefix(rest, "@"):
		n := langTagLen(rest[1:])
		return NewLiteral(value, rest[1:1+n], ""), rest[1+n:], true
	}
	return NewLiteral(value, "", ""), rest, true
}

// langTagLen returns the length of a well-formed language tag at the start of s
// (the text after the '@'): [a-zA-Z]+ ('-' [a-zA-Z0-9]+)*. It returns 0 when s does
// not begin with a letter, so an invalid tag like "0" yields no language rather
// than a value the Turtle grammar would reject on a round trip.
func langTagLen(s string) int {
	i := 0
	for i < len(s) && isAlpha(s[i]) {
		i++
	}
	if i == 0 {
		return 0
	}
	for i < len(s) && s[i] == '-' {
		j := i + 1
		for j < len(s) && (isAlpha(s[j]) || (s[j] >= '0' && s[j] <= '9')) {
			j++
		}
		if j == i+1 { // a '-' must be followed by at least one alphanumeric
			break
		}
		i = j
	}
	return i
}

// NTriples serializes the graph as N-Triples.
func (g *Graph) NTriples() []byte {
	var e Encoder
	return e.AppendNTriples(nil, g)
}

// appendNTTerm writes a term in N-Triples syntax.
func appendNTTerm(b []byte, t Term, bn *blankNamer) []byte {
	switch t.Kind {
	case IRI:
		b = append(b, '<')
		b = appendEscapedIRI(b, t.Value)
		return append(b, '>')
	case Blank:
		b = append(b, '_', ':')
		return append(b, bn.name(t.Value)...)
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

// iriNeedsEscape[c] reports whether byte c must be \u-escaped in an IRI
// reference — the ASCII controls and space plus the delimiters IRIs forbid bare.
// A table lookup replaces a per-byte chain of comparisons on the write path.
var iriNeedsEscape = func() (t [256]bool) {
	for c := 0; c <= 0x20; c++ {
		t[c] = true
	}
	for _, c := range []byte{'<', '>', '"', '{', '}', '|', '^', '`', '\\'} {
		t[c] = true
	}
	return
}()

// appendEscapedIRI escapes the characters not allowed bare in an IRI reference.
// Clean runs — the overwhelming common case, since real IRIs need no escaping —
// are bulk-copied in one append rather than byte by byte.
func appendEscapedIRI(b []byte, s string) []byte {
	start := 0
	for i := 0; i < len(s); i++ {
		if iriNeedsEscape[s[i]] {
			b = append(b, s[start:i]...)
			b = appendUnicodeEscape(b, rune(s[i]))
			start = i + 1
		}
	}
	return append(b, s[start:]...)
}

// literalEscape parameterizes appendLiteralEscaped for the two output profiles
// that share its body. The N-Triples/Turtle serializer uses the zero value; the
// canonical N-Quads form additionally emits \b and \f as named escapes and escapes
// U+007F, as RDFC-1.0 requires.
type literalEscape struct {
	namedBF   bool // emit \b and \f as named escapes (else the \u00XX control rule)
	escapeDEL bool // escape U+007F (DEL) as \u007F rather than passing it through
}

// appendLiteralEscaped escapes a literal's lexical form per cfg: always the named
// escapes \\ \" \n \r \t and \uXXXX for C0 controls, with \b \f and U+007F handled
// per cfg; valid UTF-8 passes through and a lone invalid byte is dropped. It is the
// single escaper both the N-Triples serializer and the canonicalizer call, so the
// two cannot drift.
func appendLiteralEscaped(b []byte, s string, cfg literalEscape) []byte {
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			switch {
			case c == '\\':
				b = append(b, '\\', '\\')
			case c == '"':
				b = append(b, '\\', '"')
			case c == '\n':
				b = append(b, '\\', 'n')
			case c == '\r':
				b = append(b, '\\', 'r')
			case c == '\t':
				b = append(b, '\\', 't')
			case c == '\b' && cfg.namedBF:
				b = append(b, '\\', 'b')
			case c == '\f' && cfg.namedBF:
				b = append(b, '\\', 'f')
			case c < 0x20 || (c == 0x7f && cfg.escapeDEL):
				b = appendUnicodeEscape(b, rune(c))
			default:
				b = append(b, c)
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

// appendEscapedLiteral escapes a literal's lexical form for Turtle/N-Triples.
func appendEscapedLiteral(b []byte, s string) []byte {
	return appendLiteralEscaped(b, s, literalEscape{})
}

// blankNamer maps blank-node labels to fresh, always-valid identifiers (b1, b2, …)
// during one serialization. The mapping is injective, so two distinct blank nodes
// never collapse to the same output label — which a character-sanitizing scheme
// could (e.g. "!x" and "?x" both becoming "_x"), silently merging nodes.
type blankNamer struct {
	m map[string]string
	n int
}

func (bn *blankNamer) name(label string) string {
	if v, ok := bn.m[label]; ok {
		return v
	}
	if bn.m == nil {
		bn.m = make(map[string]string)
	}
	bn.n++
	v := "b" + strconv.Itoa(bn.n)
	bn.m[label] = v
	return v
}

// newScope drops the label→name mappings while keeping the running counter, so
// the next labels begin a fresh blank-node scope yet still receive globally
// unique output names. It lets independently-labeled graphs (e.g. one per
// record) serialize into a single N-Quads document without their blank nodes
// merging.
func (bn *blankNamer) newScope() { bn.m = nil }

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
	b.Grow(len(s)) // the unescaped form is never longer, so one allocation suffices
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
