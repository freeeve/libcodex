package rdf

import "strings"

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
	return NewIRI(p.expandPName(label, base, local)), true
}

// expandPName returns base+local, interning the result per (prefix, local) so a
// repeated prefixed name — predicates and types recur heavily — allocates the
// full IRI only on its first occurrence.
func (p *turtleParser) expandPName(label, base, local string) string {
	m := p.iriCache[label]
	if m == nil {
		m = make(map[string]string)
		p.iriCache[label] = m
	}
	if full, ok := m[local]; ok {
		return full
	}
	var full string
	if p.strs != nil {
		full = p.strs.concat(base, local) // arena-backed (whole document)
	} else {
		full = base + local // streaming: a plain string the triple owns
	}
	// Cache to dedup repeated names (predicates, types, common subjects), but cap
	// the size: a file with millions of distinct subjects would otherwise grow an
	// unbounded map of entries that never see a second lookup. The vocabulary that
	// actually repeats is small and appears early, so a bound costs nothing real.
	if len(m) < maxIRICacheEntries {
		m[local] = full
	}
	return full
}

// maxIRICacheEntries bounds each prefix's expansion cache.
const maxIRICacheEntries = 1 << 16

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
	if p.base != "" && !hasScheme(raw) {
		// Simple base joining for relative references. An absolute IRI -- any
		// scheme, including non-hierarchical ones like urn:/mailto:/info: that
		// carry no "://" -- is left untouched.
		raw = p.base + raw
	}
	return raw, true
}

// hasScheme reports whether ref begins with an IRI scheme ("scheme:"): an ASCII
// letter followed by letters, digits, "+", "-" or "." up to a ":", before any
// "/", "?" or "#". Such a reference is absolute and must not be base-joined.
func hasScheme(ref string) bool {
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		if c == ':' {
			return i > 0
		}
		alpha := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
		if i == 0 {
			if !alpha {
				return false
			}
			continue
		}
		if !alpha && !(c >= '0' && c <= '9') && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	return false
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
	if p.depth >= maxParseDepth {
		return Term{}, false
	}
	p.depth++
	defer func() { p.depth-- }()
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
	if p.depth >= maxParseDepth {
		return Term{}, false
	}
	p.depth++
	defer func() { p.depth-- }()
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
		p.emit(node, NewIRI(FirstIRI), it)
		if i == len(items)-1 {
			p.emit(node, NewIRI(RestIRI), NewIRI(NilIRI))
		} else {
			next := p.fresh()
			p.emit(node, NewIRI(RestIRI), next)
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
		n := langTagLen(p.s[p.pos:])
		lang := p.s[p.pos : p.pos+n]
		p.pos += n
		return NewLiteral(value, lang, ""), true
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
	delimLen := 1
	if p.pos+2 < len(p.s) && p.s[p.pos+1] == q && p.s[p.pos+2] == q {
		delimLen = 3 // a """ or ''' long string
	}
	p.pos += delimLen
	start := p.pos
	hasEsc := false
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if c == '\\' {
			hasEsc = true
			p.pos += 2 // skip the escaped character
			continue
		}
		if c == q && (delimLen == 1 || (p.pos+2 < len(p.s) && p.s[p.pos+1] == q && p.s[p.pos+2] == q)) {
			content := p.s[start:p.pos] // zero-copy when unescaped
			p.pos += delimLen
			if hasEsc {
				if p.strs != nil {
					return p.strs.unescape(content), true
				}
				return unescapeRDF(content), true
			}
			return content, true
		}
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
	digits, expDigits := 0, 0
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch {
		case c >= '0' && c <= '9':
			digits++
			if hasExp {
				expDigits++
			}
			p.pos++
		case c == '.' && !hasDot && !hasExp && p.digitNext():
			hasDot = true
			p.pos++
		case (c == 'e' || c == 'E') && !hasExp && digits > 0:
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
	// Require at least one digit overall, and (for a number with an exponent) at
	// least one digit after the e/E, so a bare sign or "1e" is rejected.
	if digits == 0 || (hasExp && expDigits == 0) {
		p.pos = start
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
