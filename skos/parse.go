package skos

import (
	"bytes"

	"github.com/freeeve/libcodex/rdf"
)

// parseGraph parses RDF into a graph, choosing the parser by sniffing the
// serialization: JSON-LD, RDF/XML, Turtle, or (the default) N-Triples, which the
// Turtle parser also accepts. N-Quads collapses to its default graph.
func parseGraph(data []byte) (*rdf.Graph, error) {
	switch sniffFormat(data) {
	case formatJSONLD:
		return rdf.ParseJSONLD(data)
	case formatRDFXML:
		return rdf.ParseRDFXML(data)
	case formatTurtle:
		return rdf.ParseTurtle(data)
	default:
		return rdf.ParseNTriples(data)
	}
}

type rdfFormat int

const (
	formatNTriples rdfFormat = iota
	formatJSONLD
	formatRDFXML
	formatTurtle
)

// sniffFormat guesses the RDF serialization from the leading bytes: '{' (or '['
// that does not open a Turtle collection) is JSON-LD; '@prefix'/'@base'/
// 'PREFIX'/'BASE' is Turtle; a '<' opening an XML tag is RDF/XML while a '<IRI>'
// subject is N-Triples/Turtle; everything else is treated as N-Triples.
func sniffFormat(data []byte) rdfFormat {
	s := bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // optional UTF-8 BOM
	for {
		s = bytes.TrimLeft(s, " \t\r\n")
		if len(s) > 0 && s[0] == '#' { // skip Turtle/N-Triples comment lines
			if i := bytes.IndexByte(s, '\n'); i >= 0 {
				s = s[i+1:]
				continue
			}
		}
		break
	}
	if len(s) == 0 {
		return formatNTriples
	}
	switch s[0] {
	case '{':
		return formatJSONLD
	case '[':
		rest := bytes.TrimLeft(s[1:], " \t\r\n")
		if len(rest) > 0 && (rest[0] == '<' || rest[0] == '_' || rest[0] == ':' ||
			isASCIILetter(rest[0])) {
			return formatTurtle
		}
		return formatJSONLD
	case '@':
		return formatTurtle
	case '<':
		// An RDF term IRI (<scheme://...>) opens N-Triples/Turtle; an XML name
		// opens RDF/XML.
		first := s
		if i := bytes.IndexAny(s, "> \t\r\n"); i >= 0 {
			first = s[:i]
		}
		if bytes.Contains(first, []byte("://")) || bytes.HasPrefix(first, []byte("<urn:")) {
			return formatNTriples
		}
		return formatRDFXML
	}
	if bytes.HasPrefix(s, []byte("PREFIX")) || bytes.HasPrefix(s, []byte("BASE")) {
		return formatTurtle
	}
	return formatNTriples
}

// isASCIILetter reports whether b is an ASCII letter.
func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
