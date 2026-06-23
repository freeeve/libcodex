package rdf

import (
	"sort"
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

// ParseTurtle parses a Turtle document into a Graph. It supports the constructs
// real RDF uses: @prefix/@base and SPARQL-style PREFIX/BASE, prefixed names and
// IRIs, the `a` keyword, predicate-object lists (`;`) and object lists (`,`),
// blank-node labels, `[ … ]` blank-node property lists, `( … )` collections, and
// string (including triple-quoted), language-tagged, datatyped, numeric and
// boolean literals. Local names are read as the common identifier subset.
func ParseTurtle(data []byte) (*Graph, error) {
	p := &turtleParser{s: string(data), g: &Graph{}, prefixes: map[string]string{}}
	for {
		p.ws()
		if p.pos >= len(p.s) {
			return p.g, nil
		}
		if p.s[p.pos] == '@' || p.peekKeyword("prefix") || p.peekKeyword("base") {
			if !p.directive() {
				return p.g, errTurtle(p)
			}
			continue
		}
		if !p.triples() {
			return p.g, errTurtle(p)
		}
	}
}

type turtleError struct{ pos int }

func (e *turtleError) Error() string  { return "rdf: malformed Turtle" }
func errTurtle(p *turtleParser) error { return &turtleError{p.pos} }

type turtleParser struct {
	s        string
	pos      int
	g        *Graph
	prefixes map[string]string
	base     string
	blanks   int
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
func (p *turtleParser) triples() bool {
	subj, ok := p.subject()
	if !ok {
		return false
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
		p.g.Add(subj, verb, obj)
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
func (p *turtleParser) digitNext() bool {
	return p.pos+1 < len(p.s) && p.s[p.pos+1] >= '0' && p.s[p.pos+1] <= '9'
}

// matchWord matches a bare keyword followed by a non-name character, consuming it.
func (p *turtleParser) matchWord(w string) bool {
	if p.pos+len(w) > len(p.s) || p.s[p.pos:p.pos+len(w)] != w {
		return false
	}
	next := p.pos + len(w)
	if next < len(p.s) && isNameChar(p.s[next]) {
		return false
	}
	p.pos += len(w)
	return true
}

func (p *turtleParser) iriOrPName() (Term, bool) {
	if p.peek() == '<' {
		iri, ok := p.iriRef()
		return NewIRI(iri), ok
	}
	label := p.readPrefixLabelNoColon()
	if p.peek() != ':' {
		return Term{}, false
	}
	p.pos++ // ':'
	local := p.readLocalName()
	base, ok := p.prefixes[label]
	if !ok {
		return Term{}, false
	}
	return NewIRI(base + local), true
}

// iriRef reads `<IRI>` and resolves it against @base when relative.
func (p *turtleParser) iriRef() (string, bool) {
	if p.peek() != '<' {
		return "", false
	}
	i := strings.IndexByte(p.s[p.pos:], '>')
	if i < 0 {
		return "", false
	}
	raw := unescapeRDF(p.s[p.pos+1 : p.pos+i])
	p.pos += i + 1
	if p.base != "" && !strings.Contains(raw, "://") && !strings.HasPrefix(raw, "#") {
		// (only simple base joining is needed for the documents we read)
		raw = p.base + raw
	}
	return raw, true
}

func (p *turtleParser) blankLabel() (Term, bool) {
	if !strings.HasPrefix(p.s[p.pos:], "_:") {
		return Term{}, false
	}
	p.pos += 2
	start := p.pos
	for p.pos < len(p.s) && isNameChar(p.s[p.pos]) {
		p.pos++
	}
	for p.pos > start && p.s[p.pos-1] == '.' { // a label cannot end with '.'
		p.pos--
	}
	if p.pos == start {
		return Term{}, false
	}
	return NewBlank("u" + p.s[start:p.pos]), true
}

// blankPropertyList parses `[ predicateObjectList ]` into a fresh blank node.
func (p *turtleParser) blankPropertyList() (Term, bool) {
	if p.peek() != '[' {
		return Term{}, false
	}
	p.pos++
	node := p.fresh()
	p.ws()
	if p.peek() == ']' {
		p.pos++
		return node, true
	}
	if !p.predicateObjectList(node) {
		return Term{}, false
	}
	p.ws()
	if p.peek() != ']' {
		return Term{}, false
	}
	p.pos++
	return node, true
}

// collection parses `( object* )` into an RDF list, returning its head.
func (p *turtleParser) collection() (Term, bool) {
	if p.peek() != '(' {
		return Term{}, false
	}
	p.pos++
	var items []Term
	for {
		p.ws()
		if p.peek() == ')' {
			p.pos++
			break
		}
		obj, ok := p.object()
		if !ok {
			return Term{}, false
		}
		items = append(items, obj)
	}
	if len(items) == 0 {
		return NewIRI(NilIRI), true
	}
	head := p.fresh()
	node := head
	for i, it := range items {
		p.g.Add(node, NewIRI(FirstIRI), it)
		if i == len(items)-1 {
			p.g.Add(node, NewIRI(RestIRI), NewIRI(NilIRI))
		} else {
			next := p.fresh()
			p.g.Add(node, NewIRI(RestIRI), next)
			node = next
		}
	}
	return head, true
}

// literal parses a quoted string and any @lang or ^^datatype suffix.
func (p *turtleParser) literal() (Term, bool) {
	value, ok := p.quotedString()
	if !ok {
		return Term{}, false
	}
	switch {
	case p.peek() == '@':
		p.pos++
		start := p.pos
		for p.pos < len(p.s) && (isAlpha(p.s[p.pos]) || p.s[p.pos] == '-') {
			p.pos++
		}
		return NewLiteral(value, p.s[start:p.pos], ""), true
	case strings.HasPrefix(p.s[p.pos:], "^^"):
		p.pos += 2
		dt, ok := p.iriOrPName()
		if !ok {
			return Term{}, false
		}
		return NewLiteral(value, "", dt.Value), true
	}
	return NewLiteral(value, "", ""), true
}

// quotedString reads a "…" or '…' string, or a """…""" / ”'…”' long string,
// decoding escapes.
func (p *turtleParser) quotedString() (string, bool) {
	q := p.peek()
	if q != '"' && q != '\'' {
		return "", false
	}
	long := strings.HasPrefix(p.s[p.pos:], string([]byte{q, q, q}))
	delim := string(q)
	if long {
		delim = string([]byte{q, q, q})
	}
	p.pos += len(delim)
	var b strings.Builder
	for p.pos < len(p.s) {
		if strings.HasPrefix(p.s[p.pos:], delim) {
			p.pos += len(delim)
			return b.String(), true
		}
		if p.s[p.pos] == '\\' && p.pos+1 < len(p.s) {
			r, n := unescapeRune(p.s[p.pos:])
			b.WriteRune(r)
			p.pos += n
			continue
		}
		b.WriteByte(p.s[p.pos])
		p.pos++
	}
	return "", false
}

// number reads a numeric literal and assigns its xsd datatype.
func (p *turtleParser) number() (Term, bool) {
	start := p.pos
	if c := p.peek(); c == '+' || c == '-' {
		p.pos++
	}
	hasDot, hasExp := false, false
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch {
		case c >= '0' && c <= '9':
			p.pos++
		case c == '.' && !hasDot && !hasExp && p.digitNext():
			hasDot = true
			p.pos++
		case (c == 'e' || c == 'E') && !hasExp:
			hasExp = true
			p.pos++
			if c2 := p.peek(); c2 == '+' || c2 == '-' {
				p.pos++
			}
		default:
			goto done
		}
	}
done:
	if p.pos == start {
		return Term{}, false
	}
	dt := xsdInteger
	if hasExp {
		dt = xsdDouble
	} else if hasDot {
		dt = xsdDecimal
	}
	return NewLiteral(p.s[start:p.pos], "", dt), true
}

func (p *turtleParser) readBareWord() string {
	start := p.pos
	for p.pos < len(p.s) && isAlpha(p.s[p.pos]) {
		p.pos++
	}
	return p.s[start:p.pos]
}

// readPrefixLabel reads a prefix label up to and including its ':'.
func (p *turtleParser) readPrefixLabel() string {
	label := p.readPrefixLabelNoColon()
	if p.peek() == ':' {
		p.pos++
	}
	return label
}

func (p *turtleParser) readPrefixLabelNoColon() string {
	start := p.pos
	for p.pos < len(p.s) && p.s[p.pos] != ':' && isNameChar(p.s[p.pos]) {
		p.pos++
	}
	return p.s[start:p.pos]
}

func (p *turtleParser) readLocalName() string {
	start := p.pos
	for p.pos < len(p.s) && isNameChar(p.s[p.pos]) {
		p.pos++
	}
	for p.pos > start && p.s[p.pos-1] == '.' { // PN_LOCAL cannot end with '.' (the terminator)
		p.pos--
	}
	return p.s[start:p.pos]
}

func isAlpha(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

// isNameChar reports whether c is allowed in the common subset of prefixed-name
// local parts and blank-node labels handled here.
func isNameChar(c byte) bool {
	return isAlpha(c) || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.'
}

// ---- Turtle serialization ----

// Turtle serializes the graph as Turtle, declaring the given namespace prefixes
// (prefix label -> namespace IRI) and compacting IRIs against them. Triples are
// grouped by subject in first-seen order, using `a` for rdf:type.
func (g *Graph) Turtle(prefixes map[string]string) []byte {
	return append(TurtleHeader(prefixes), g.TurtleBody(prefixes)...)
}

// TurtleHeader returns the @prefix declaration block for the given prefixes,
// terminated by a blank line. Collection writers emit it once.
func TurtleHeader(prefixes map[string]string) []byte {
	if len(prefixes) == 0 {
		return nil
	}
	labels := make([]string, 0, len(prefixes))
	for k := range prefixes {
		labels = append(labels, k)
	}
	sort.Strings(labels)
	var b []byte
	for _, k := range labels {
		b = append(b, "@prefix "...)
		b = append(b, k...)
		b = append(b, ": <"...)
		b = appendEscapedIRI(b, prefixes[k])
		b = append(b, "> .\n"...)
	}
	return append(b, '\n')
}

// TurtleBody serializes the triples grouped by subject, without a prefix header.
func (g *Graph) TurtleBody(prefixes map[string]string) []byte {
	var b []byte
	var order []Term
	seen := map[string]bool{}
	bySubj := map[string][]Triple{}
	for _, t := range g.Triples {
		k := t.S.key()
		if !seen[k] {
			seen[k] = true
			order = append(order, t.S)
		}
		bySubj[k] = append(bySubj[k], t)
	}
	for _, s := range order {
		b = appendTurtleSubject(b, s, bySubj[s.key()], prefixes)
	}
	return b
}

func appendTurtleSubject(b []byte, s Term, triples []Triple, prefixes map[string]string) []byte {
	b = appendTurtleTerm(b, s, prefixes, false)

	var predOrder []Term
	predSeen := map[string]bool{}
	byPred := map[string][]Term{}
	for _, t := range triples {
		k := t.P.key()
		if !predSeen[k] {
			predSeen[k] = true
			predOrder = append(predOrder, t.P)
		}
		byPred[k] = append(byPred[k], t.O)
	}
	for i, pterm := range predOrder {
		if i == 0 {
			b = append(b, ' ')
		} else {
			b = append(b, " ;\n    "...)
		}
		b = appendTurtleTerm(b, pterm, prefixes, true)
		for j, o := range byPred[pterm.key()] {
			if j == 0 {
				b = append(b, ' ')
			} else {
				b = append(b, ", "...)
			}
			b = appendTurtleTerm(b, o, prefixes, false)
		}
	}
	return append(b, " .\n"...)
}

// appendTurtleTerm writes a term in Turtle syntax. In predicate position rdf:type
// is written as `a`.
func appendTurtleTerm(b []byte, t Term, prefixes map[string]string, predicate bool) []byte {
	switch t.Kind {
	case IRI:
		if predicate && t.Value == TypeIRI {
			return append(b, 'a')
		}
		if pn, ok := compactIRI(t.Value, prefixes); ok {
			return append(b, pn...)
		}
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
			b = append(b, "^^"...)
			if pn, ok := compactIRI(t.Datatype, prefixes); ok {
				return append(b, pn...)
			}
			b = append(b, '<')
			b = appendEscapedIRI(b, t.Datatype)
			return append(b, '>')
		}
		return b
	}
}

// compactIRI returns iri as a prefixed name against the longest matching
// namespace, when the remaining local part is a valid bare local name.
func compactIRI(iri string, prefixes map[string]string) (string, bool) {
	bestLabel, bestNS := "", ""
	for label, ns := range prefixes {
		if len(ns) > len(bestNS) && strings.HasPrefix(iri, ns) {
			if local := iri[len(ns):]; validLocal(local) {
				bestLabel, bestNS = label, ns
			}
		}
	}
	if bestNS == "" {
		return "", false
	}
	return bestLabel + ":" + iri[len(bestNS):], true
}

// validLocal reports whether s is a non-empty local name that can be written bare
// (matching what the reader accepts), so a round-trip is lossless.
func validLocal(s string) bool {
	if s == "" || s[len(s)-1] == '.' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isNameChar(s[i]) {
			return false
		}
	}
	return true
}
