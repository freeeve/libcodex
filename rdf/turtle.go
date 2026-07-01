package rdf

import (
	"fmt"
	"strconv"
	"strings"
)

// XSD datatypes used by Turtle's literal shorthands.
const (
	xsdBoolean = "http://www.w3.org/2001/XMLSchema#boolean"
	xsdInteger = "http://www.w3.org/2001/XMLSchema#integer"
	xsdDecimal = "http://www.w3.org/2001/XMLSchema#decimal"
	xsdDouble  = "http://www.w3.org/2001/XMLSchema#double"
)

// maxParseDepth bounds nesting in the recursive-descent parsers — Turtle
// blank-node property lists (`[ … ]`) and collections (`( … )`), and RDF/XML
// node elements — so adversarial deeply nested input fails with an error
// instead of overflowing the goroutine stack, which is a fatal, unrecoverable
// runtime crash (recover cannot catch it). No real document nests this deep.
const maxParseDepth = 1 << 13

// ParseTurtle parses a Turtle document into a Graph. It supports the constructs
// real RDF uses: @prefix/@base and SPARQL-style PREFIX/BASE, prefixed names and
// IRIs, the `a` keyword, predicate-object lists (`;`) and object lists (`,`),
// blank-node labels, `[ … ]` blank-node property lists, `( … )` collections, and
// string (including triple-quoted), language-tagged, datatyped, numeric and
// boolean literals. Local names are read as the common identifier subset.
func ParseTurtle(data []byte) (*Graph, error) {
	g := &Graph{Triples: make([]Triple, 0, len(data)/32)}
	p := &turtleParser{
		s:        string(data),
		emit:     g.Add,
		prefixes: map[string]string{},
		iriCache: map[string]map[string]string{},
		strs:     &arena{},
	}
	for {
		ok, done := p.stmt()
		if done {
			return g, nil
		}
		if !ok {
			return g, errTurtle(p)
		}
	}
}

// stmt parses one directive or triple statement from the current buffer, emitting
// its triples. done is true at end of input; ok is false on a malformed statement.
// It is shared by the whole-document parser and the streaming decoder (which feeds
// one statement at a time).
func (p *turtleParser) stmt() (ok, done bool) {
	p.ws()
	if p.pos >= len(p.s) {
		return true, true
	}
	if p.s[p.pos] == '@' || p.peekKeyword("prefix") || p.peekKeyword("base") {
		return p.directive(), false
	}
	return p.triples(), false
}

type turtleError struct{ pos, line, col int }

// Error reports the failure with its position. For a whole-document parse the
// line and column locate the byte in the source; when streaming, p.s is a single
// statement, so they are relative to the start of that statement.
func (e *turtleError) Error() string {
	return fmt.Sprintf("rdf: malformed Turtle at line %d, column %d (byte offset %d)", e.line, e.col, e.pos)
}

func errTurtle(p *turtleParser) error {
	line, col := lineCol(p.s, p.pos)
	return &turtleError{pos: p.pos, line: line, col: col}
}

// lineCol returns the 1-based line and column of byte offset pos in s.
func lineCol(s string, pos int) (line, col int) {
	line, col = 1, 1
	if pos > len(s) {
		pos = len(s)
	}
	for i := range pos {
		if s[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

type turtleParser struct {
	s        string
	pos      int
	emit     func(s, pr, o Term) // sink: Graph.Add (whole document) or channel (streaming)
	prefixes map[string]string
	iriCache map[string]map[string]string // prefix -> local -> interned full IRI
	strs     *arena                       // backs expanded IRIs when whole-document; nil when streaming
	base     string
	blanks   int
	depth    int // current `[`/`(` nesting, bounded by maxParseDepth
}

func (p *turtleParser) fresh() Term {
	p.blanks++
	return NewBlank("t" + strconv.Itoa(p.blanks))
}

// ws skips whitespace and # comments.
func (p *turtleParser) ws() {
	for p.pos < len(p.s) {
		switch p.s[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		case '#':
			for p.pos < len(p.s) && p.s[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

func (p *turtleParser) peek() byte {
	if p.pos < len(p.s) {
		return p.s[p.pos]
	}
	return 0
}

// peekKeyword reports whether a case-insensitive keyword (SPARQL PREFIX/BASE)
// begins at the cursor, followed by a non-name character.
func (p *turtleParser) peekKeyword(kw string) bool {
	if p.pos+len(kw) > len(p.s) {
		return false
	}
	if !strings.EqualFold(p.s[p.pos:p.pos+len(kw)], kw) {
		return false
	}
	next := p.pos + len(kw)
	return next >= len(p.s) || isWS(p.s[next])
}

func isWS(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

// directive parses @prefix/@base or SPARQL PREFIX/BASE.
func (p *turtleParser) directive() bool {
	sparql := p.s[p.pos] != '@'
	if !sparql {
		p.pos++ // '@'
	}
	kind := p.readBareWord()
	p.ws()
	switch strings.ToLower(kind) {
	case "prefix":
		label := p.readPrefixLabel()
		p.ws()
		iri, ok := p.iriRef()
		if !ok {
			return false
		}
		p.prefixes[label] = iri
	case "base":
		iri, ok := p.iriRef()
		if !ok {
			return false
		}
		p.base = iri
	default:
		return false
	}
	if !sparql {
		p.ws()
		if p.peek() != '.' {
			return false
		}
		p.pos++
	}
	return true
}

// triples parses a subject and its predicate-object list, terminated by '.'.
// A blankNodePropertyList subject ("[ ... ]") carries its predicate-object pairs
// inside the brackets, so per the Turtle grammar the trailing predicate-object
// list is optional there ("[ a :Work ] ." is a complete statement); every other
// subject requires one.
func (p *turtleParser) triples() bool {
	p.ws()
	bracketed := p.peek() == '['
	subj, ok := p.subject()
	if !ok {
		return false
	}
	p.ws()
	if bracketed && p.peek() == '.' {
		p.pos++
		return true
	}
	if !p.predicateObjectList(subj) {
		return false
	}
	p.ws()
	if p.peek() != '.' {
		return false
	}
	p.pos++
	return true
}

// predicateObjectList parses `verb objectList (';' verb objectList)*`.
func (p *turtleParser) predicateObjectList(subj Term) bool {
	for {
		p.ws()
		verb, ok := p.verb()
		if !ok {
			return false
		}
		if !p.objectList(subj, verb) {
			return false
		}
		p.ws()
		if p.peek() != ';' {
			return true
		}
		for p.peek() == ';' { // allow repeated/empty ';'
			p.pos++
			p.ws()
		}
		// A trailing ';' before '.' or ']' ends the list.
		if c := p.peek(); c == '.' || c == ']' || c == 0 {
			return true
		}
	}
}

func (p *turtleParser) objectList(subj, verb Term) bool {
	for {
		obj, ok := p.object()
		if !ok {
			return false
		}
		p.emit(subj, verb, obj)
		p.ws()
		if p.peek() != ',' {
			return true
		}
		p.pos++
	}
}

func (p *turtleParser) verb() (Term, bool) {
	if p.peek() == 'a' {
		next := p.pos + 1
		if next >= len(p.s) || isWS(p.s[next]) || p.s[next] == '<' {
			p.pos++
			return NewIRI(TypeIRI), true
		}
	}
	return p.iriOrPName()
}

func (p *turtleParser) subject() (Term, bool) {
	p.ws()
	switch p.peek() {
	case '_':
		return p.blankLabel()
	case '[':
		return p.blankPropertyList()
	case '(':
		return p.collection()
	default:
		return p.iriOrPName()
	}
}

func (p *turtleParser) object() (Term, bool) {
	p.ws()
	switch c := p.peek(); {
	case c == '<':
		return p.iriOrPName()
	case c == '"', c == '\'':
		return p.literal()
	case c == '_':
		return p.blankLabel()
	case c == '[':
		return p.blankPropertyList()
	case c == '(':
		return p.collection()
	case c == 't' && p.matchWord("true"):
		return NewLiteral("true", "", xsdBoolean), true
	case c == 'f' && p.matchWord("false"):
		return NewLiteral("false", "", xsdBoolean), true
	case (c >= '0' && c <= '9') || c == '+' || c == '-' || (c == '.' && p.digitNext()):
		return p.number()
	default:
		return p.iriOrPName()
	}
}

// digitNext reports whether a digit follows the cursor (to tell a leading-dot
// number from a statement terminator).
