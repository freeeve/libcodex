package rdf

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// ParseJSONLD parses a JSON-LD document into a Graph. It expands compact IRIs and
// terms against an inline @context (prefix maps and term definitions, including
// "@type":"@id" coercion), and handles @graph, @id, @type, @value/@language/@type
// literals, @list, nested node objects, and arrays of any of these. It does not
// fetch remote @context documents or run full JSON-LD 1.1 framing.
func ParseJSONLD(data []byte) (*Graph, error) {
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	p := &jsonldParser{g: &Graph{}, ctx: map[string]string{}, coerce: map[string]string{}, expanded: map[string]string{}}

	// A document is a node, an array of nodes, or an object wrapping @graph.
	switch d := doc.(type) {
	case map[string]any:
		p.loadContext(d["@context"])
		if g, ok := d["@graph"]; ok {
			p.eachNode(g)
		} else {
			p.node(d)
		}
	case []any:
		for _, n := range d {
			p.node(n)
		}
	}
	return p.g, nil
}

type jsonldParser struct {
	g        *Graph
	ctx      map[string]string // prefix or term -> IRI
	coerce   map[string]string // term -> "@id" (value is an IRI reference, not a literal)
	expanded map[string]string // compact IRI/term -> interned full IRI
	blanks   int
}

func (p *jsonldParser) fresh() Term {
	p.blanks++
	return NewBlank("j" + strconv.Itoa(p.blanks))
}

// loadContext records prefix and term mappings from an inline @context. String
// (remote) contexts are skipped.
func (p *jsonldParser) loadContext(c any) {
	switch v := c.(type) {
	case []any:
		for _, e := range v {
			p.loadContext(e)
		}
	case map[string]any:
		for k, val := range v {
			if strings.HasPrefix(k, "@") {
				continue
			}
			switch t := val.(type) {
			case string:
				p.ctx[k] = t
			case map[string]any:
				if id, ok := t["@id"].(string); ok {
					p.ctx[k] = id
				}
				if ty, ok := t["@type"].(string); ok && ty == "@id" {
					p.coerce[k] = "@id"
				}
			}
		}
	}
}

// expand resolves a compact IRI or term to a full IRI.
func (p *jsonldParser) expand(s string) string {
	if s == "" || s[0] == '@' {
		return s
	}
	// Compact IRIs and terms recur heavily (every property, every type); intern the
	// expansion so it is built only once. The cache key is the existing JSON string,
	// so the lookup itself allocates nothing.
	if full, ok := p.expanded[s]; ok {
		return full
	}
	full := p.expandUncached(s)
	p.expanded[s] = full
	return full
}

func (p *jsonldParser) expandUncached(s string) string {
	if prefix, suffix, ok := strings.Cut(s, ":"); ok {
		if !strings.HasPrefix(suffix, "//") {
			if base, ok := p.ctx[prefix]; ok {
				return base + suffix
			}
		}
		return s // already an absolute IRI
	}
	if base, ok := p.ctx[s]; ok {
		return base
	}
	return s
}

func (p *jsonldParser) eachNode(v any) {
	if arr, ok := v.([]any); ok {
		for _, n := range arr {
			p.node(n)
		}
		return
	}
	p.node(v)
}

// node emits triples for a node object and returns its subject term.
func (p *jsonldParser) node(v any) Term {
	obj, ok := v.(map[string]any)
	if !ok {
		return Term{}
	}
	subject := p.subjectOf(obj)

	if t, ok := obj["@type"]; ok {
		for _, ty := range asSlice(t) {
			if s, ok := ty.(string); ok {
				p.g.Add(subject, NewIRI(TypeIRI), NewIRI(p.expand(s)))
			}
		}
	}
	for key, val := range obj {
		if strings.HasPrefix(key, "@") {
			continue // @id and @type handled; other keywords carry no statement here
		}
		pred := NewIRI(p.expand(key))
		idRef := p.coerce[key] == "@id"
		for _, o := range p.values(val, idRef) {
			p.g.Add(subject, pred, o)
		}
	}
	return subject
}

// values turns a JSON-LD value (or array) into the object terms it denotes,
// emitting triples for any nested nodes.
func (p *jsonldParser) values(v any, idRef bool) []Term {
	switch t := v.(type) {
	case []any:
		var out []Term
		for _, e := range t {
			out = append(out, p.values(e, idRef)...)
		}
		return out
	case string:
		if idRef {
			return []Term{NewIRI(p.expand(t))}
		}
		return []Term{NewLiteral(t, "", "")}
	case bool:
		return []Term{NewLiteral(strconv.FormatBool(t), "", "http://www.w3.org/2001/XMLSchema#boolean")}
	case float64:
		return []Term{NewLiteral(formatNumber(t), "", "http://www.w3.org/2001/XMLSchema#double")}
	case map[string]any:
		return p.objectValue(t)
	}
	return nil
}

// objectValue interprets a JSON-LD object as a literal (@value), an RDF list
// (@list), a bare reference (@id only), or a nested node.
func (p *jsonldParser) objectValue(m map[string]any) []Term {
	if val, ok := m["@value"]; ok {
		lang, _ := m["@language"].(string)
		dt, _ := m["@type"].(string)
		if dt != "" {
			dt = p.expand(dt)
		}
		return []Term{NewLiteral(scalarString(val), lang, dt)}
	}
	if lst, ok := m["@list"]; ok {
		return []Term{p.list(lst)}
	}
	if id, ok := m["@id"].(string); ok && onlyKeys(m, "@id") {
		return []Term{NewIRI(p.expand(id))}
	}
	return []Term{p.node(m)}
}

// list materializes a JSON-LD @list as an RDF collection and returns its head
// (rdf:nil for the empty list).
func (p *jsonldParser) list(v any) Term {
	items := asSlice(v)
	if len(items) == 0 {
		return NewIRI(NilIRI)
	}
	head := p.fresh()
	node := head
	for i, it := range items {
		for _, o := range p.values(it, false) {
			p.g.Add(node, NewIRI(FirstIRI), o)
		}
		if i == len(items)-1 {
			p.g.Add(node, NewIRI(RestIRI), NewIRI(NilIRI))
		} else {
			next := p.fresh()
			p.g.Add(node, NewIRI(RestIRI), next)
			node = next
		}
	}
	return head
}

func (p *jsonldParser) subjectOf(obj map[string]any) Term {
	if id, ok := obj["@id"].(string); ok {
		if strings.HasPrefix(id, "_:") {
			return NewBlank(id[2:])
		}
		return NewIRI(p.expand(id))
	}
	return p.fresh()
}

func asSlice(v any) []any {
	if arr, ok := v.([]any); ok {
		return arr
	}
	return []any{v}
}

func onlyKeys(m map[string]any, allowed ...string) bool {
	for k := range m {
		if !slices.Contains(allowed, k) {
			return false
		}
	}
	return true
}

func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return formatNumber(t)
	}
	return ""
}

// formatNumber renders a JSON number without a trailing ".0" for integers.
func formatNumber(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}
