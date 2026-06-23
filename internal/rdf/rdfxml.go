package rdf

import (
	"encoding/xml"
	"io"
	"strconv"
	"strings"
)

const xmlNS = "http://www.w3.org/XML/1998/namespace"

// ParseRDFXML parses an RDF/XML document into a Graph. It supports the striped
// syntax real bibliographic RDF uses: typed node elements and rdf:Description,
// rdf:about / rdf:nodeID / rdf:ID subjects, rdf:resource and rdf:nodeID object
// references, nested node elements, literal property values with rdf:datatype and
// xml:lang, property attributes, and rdf:parseType="Resource". It does not handle
// RDF containers, reification, or rdf:parseType="Literal"/"Collection".
func ParseRDFXML(data []byte) (*Graph, error) {
	p := &xmlParser{dec: xml.NewDecoder(strings.NewReader(string(data))), g: &Graph{}}
	for {
		tok, err := p.dec.Token()
		if err == io.EOF {
			return p.g, nil
		}
		if err != nil {
			return p.g, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		// The document root is rdf:RDF (its children are node elements) or, when a
		// single node is the root, the node element itself.
		if se.Name.Space == NS && se.Name.Local == "RDF" {
			if err := p.parseNodeChildren(se); err != nil {
				return p.g, err
			}
			continue
		}
		if _, err := p.parseNode(se); err != nil {
			return p.g, err
		}
	}
}

type xmlParser struct {
	dec    *xml.Decoder
	g      *Graph
	blanks int
}

func (p *xmlParser) fresh() Term {
	p.blanks++
	return NewBlank("b" + strconv.Itoa(p.blanks))
}

func iriOf(n xml.Name) string { return n.Space + n.Local }

func attr(se xml.StartElement, space, local string) (string, bool) {
	for _, a := range se.Attr {
		if a.Name.Space == space && a.Name.Local == local {
			return a.Value, true
		}
	}
	return "", false
}

// parseNode parses a node element (already read as a StartElement), emits its
// type and property-attribute triples, recurses into property children, and
// returns the node's subject term. It consumes the matching EndElement.
func (p *xmlParser) parseNode(se xml.StartElement) (Term, error) {
	subject := p.subjectOf(se)
	if !(se.Name.Space == NS && se.Name.Local == "Description") {
		p.g.Add(subject, NewIRI(TypeIRI), NewIRI(iriOf(se.Name)))
	}
	// Property attributes: rdf attributes other than the identity ones become
	// literal-valued statements (rdf:type becomes a type IRI).
	for _, a := range se.Attr {
		if isStructuralAttr(a) {
			continue
		}
		if a.Name.Space == NS && a.Name.Local == "type" {
			p.g.Add(subject, NewIRI(TypeIRI), NewIRI(a.Value))
			continue
		}
		p.g.Add(subject, NewIRI(iriOf(a.Name)), NewLiteral(a.Value, "", ""))
	}
	if err := p.parseProperties(se, subject); err != nil {
		return subject, err
	}
	return subject, nil
}

// parseNodeChildren reads the child node elements of a container (rdf:RDF) until
// its EndElement.
func (p *xmlParser) parseNodeChildren(container xml.StartElement) error {
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if _, err := p.parseNode(t); err != nil {
				return err
			}
		case xml.EndElement:
			if t.Name == container.Name {
				return nil
			}
		}
	}
}

// parseProperties reads the property-element children of a node until its
// EndElement.
func (p *xmlParser) parseProperties(node xml.StartElement, subject Term) error {
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if err := p.parseProperty(subject, t); err != nil {
				return err
			}
		case xml.EndElement:
			if t.Name == node.Name {
				return nil
			}
		}
	}
}

// parseProperty parses one property element and emits the (subject, predicate,
// object) triple, consuming the property's content and EndElement.
func (p *xmlParser) parseProperty(subject Term, se xml.StartElement) error {
	pred := NewIRI(iriOf(se.Name))

	if res, ok := attr(se, NS, "resource"); ok {
		p.g.Add(subject, pred, NewIRI(res))
		return p.skipTo(se.Name)
	}
	if nid, ok := attr(se, NS, "nodeID"); ok {
		p.g.Add(subject, pred, NewBlank(nid))
		return p.skipTo(se.Name)
	}
	if pt, ok := attr(se, NS, "parseType"); ok && pt == "Resource" {
		obj := p.fresh()
		p.g.Add(subject, pred, obj)
		return p.parseProperties(se, obj)
	}
	datatype, _ := attr(se, NS, "datatype")
	lang, _ := attr(se, xmlNS, "lang")

	// Look at the content: a nested node element, or literal text.
	var text strings.Builder
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, err := p.parseNode(t)
			if err != nil {
				return err
			}
			p.g.Add(subject, pred, child)
			return p.skipTo(se.Name) // consume any trailing whitespace + EndElement
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if t.Name == se.Name {
				p.g.Add(subject, pred, NewLiteral(text.String(), lang, datatype))
				return nil
			}
		}
	}
}

// skipTo consumes tokens until the EndElement matching name.
func (p *xmlParser) skipTo(name xml.Name) error {
	depth := 0
	for {
		tok, err := p.dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			if depth == 0 && t.Name == name {
				return nil
			}
			depth--
		}
	}
}

// subjectOf returns a node element's subject from rdf:about / rdf:nodeID / rdf:ID,
// or a fresh blank node.
func (p *xmlParser) subjectOf(se xml.StartElement) Term {
	if v, ok := attr(se, NS, "about"); ok {
		return NewIRI(v)
	}
	if v, ok := attr(se, NS, "nodeID"); ok {
		return NewBlank(v)
	}
	if v, ok := attr(se, NS, "ID"); ok {
		return NewIRI("#" + v)
	}
	return p.fresh()
}

// isStructuralAttr reports whether an attribute carries node identity or XML
// machinery rather than a property statement.
func isStructuralAttr(a xml.Attr) bool {
	if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" || a.Name.Space == xmlNS {
		return true
	}
	if a.Name.Space == NS {
		switch a.Name.Local {
		case "about", "nodeID", "ID", "resource", "datatype", "parseType":
			return true
		}
	}
	return false
}
