// Package rdf is a small, dependency-free RDF toolkit: a triple model and parsers
// for the two RDF serializations BIBFRAME uses, RDF/XML and JSON-LD. It targets
// the constructs real bibliographic RDF uses rather than the whole of RDF — see
// the parser docs for what is and isn't handled.
package rdf

import "strings"

// Well-known IRIs.
const (
	NS         = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	TypeIRI    = NS + "type"
	FirstIRI   = NS + "first"
	RestIRI    = NS + "rest"
	NilIRI     = NS + "nil"
	langString = NS + "langString"
	XSDString  = "http://www.w3.org/2001/XMLSchema#string"
)

// Kind distinguishes the three RDF term types.
type Kind uint8

const (
	IRI Kind = iota
	Blank
	Literal
)

// Term is an RDF term: an IRI, a blank node, or a literal.
type Term struct {
	Kind     Kind
	Value    string // IRI, blank-node identifier, or a literal's lexical form
	Lang     string // literal language tag (Literal only)
	Datatype string // literal datatype IRI (Literal only)
}

// NewIRI, NewBlank and NewLiteral construct the three term kinds.
func NewIRI(s string) Term    { return Term{Kind: IRI, Value: s} }
func NewBlank(id string) Term { return Term{Kind: Blank, Value: id} }

// NewLiteral makes a literal; an empty datatype defaults per RDF (xsd:string, or
// rdf:langString when a language tag is present).
func NewLiteral(value, lang, datatype string) Term {
	if datatype == "" {
		if lang != "" {
			datatype = langString
		} else {
			datatype = XSDString
		}
	}
	return Term{Kind: Literal, Value: value, Lang: lang, Datatype: datatype}
}

// IsIRI, IsBlank and IsLiteral report the term kind.
func (t Term) IsIRI() bool     { return t.Kind == IRI }
func (t Term) IsBlank() bool   { return t.Kind == Blank }
func (t Term) IsLiteral() bool { return t.Kind == Literal }

// key returns a comparable identity for indexing (kind, value, and for literals
// the language and datatype).
func (t Term) key() string {
	switch t.Kind {
	case IRI:
		return "<" + t.Value
	case Blank:
		return "_" + t.Value
	default:
		return "\"" + t.Value + "\x00" + t.Lang + "\x00" + t.Datatype
	}
}

// Triple is an RDF statement.
type Triple struct {
	S, P, O Term
}

// Graph is a set of triples with simple lookup helpers built on first use.
type Graph struct {
	Triples []Triple

	spo map[string][]Triple // subject key -> triples
}

// Add appends a triple.
func (g *Graph) Add(s, p, o Term) {
	g.Triples = append(g.Triples, Triple{s, p, o})
	g.spo = nil // invalidate the index
}

func (g *Graph) index() {
	if g.spo != nil {
		return
	}
	g.spo = make(map[string][]Triple, len(g.Triples))
	for _, t := range g.Triples {
		k := t.S.key()
		g.spo[k] = append(g.spo[k], t)
	}
}

// Objects returns the objects of every triple with the given subject and
// predicate IRI, in document order.
func (g *Graph) Objects(subject Term, predicate string) []Term {
	g.index()
	var out []Term
	for _, t := range g.spo[subject.key()] {
		if t.P.Kind == IRI && t.P.Value == predicate {
			out = append(out, t.O)
		}
	}
	return out
}

// Object returns the first object for (subject, predicate), or false.
func (g *Graph) Object(subject Term, predicate string) (Term, bool) {
	if objs := g.Objects(subject, predicate); len(objs) > 0 {
		return objs[0], true
	}
	return Term{}, false
}

// HasType reports whether the subject has rdf:type typeIRI.
func (g *Graph) HasType(subject Term, typeIRI string) bool {
	for _, o := range g.Objects(subject, TypeIRI) {
		if o.Kind == IRI && o.Value == typeIRI {
			return true
		}
	}
	return false
}

// SubjectsOfType returns every subject with rdf:type typeIRI, in document order
// (deduplicated).
func (g *Graph) SubjectsOfType(typeIRI string) []Term {
	var out []Term
	seen := map[string]bool{}
	for _, t := range g.Triples {
		if t.P.Kind == IRI && t.P.Value == TypeIRI && t.O.Kind == IRI && t.O.Value == typeIRI {
			if k := t.S.key(); !seen[k] {
				seen[k] = true
				out = append(out, t.S)
			}
		}
	}
	return out
}

// LocalName returns the fragment or last path segment of an IRI (the part after
// the final '#' or '/').
func LocalName(iri string) string {
	if i := strings.LastIndexByte(iri, '#'); i >= 0 {
		return iri[i+1:]
	}
	if i := strings.LastIndexByte(iri, '/'); i >= 0 {
		return iri[i+1:]
	}
	return iri
}
