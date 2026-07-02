package bibframe

import "unicode/utf8"

const (
	xmlHeader = `<?xml version="1.0" encoding="UTF-8"?>`
	rdfOpen   = `<rdf:RDF xmlns:rdf="` + rdfNS + `" xmlns:rdfs="` + rdfsNS + `"` +
		` xmlns:bf="` + bfNS + `" xmlns:bflc="` + bflcNS + `">`
	rdfClose = `</rdf:RDF>`
)

// appendGraphXML appends the Work and Instance nodes for one record. base is the
// local identifier stem shared by the two nodes. The node shape comes from
// shape.go; xmlNode does the formatting.
func appendGraphXML(b []byte, g *BIBFRAME, base string) []byte {
	s := newXMLSink(b)
	emitWork(s, &g.Work, base, instanceIRIVal(base), nil)
	emitInstance(s, &g.Instance, base, base)
	return s.b
}

// appendWorkInstancesXML emits a Work with N Instances: the Work node once with
// one bf:hasInstance per Instance, then each Instance under its own IRI. It
// mirrors WorkInstances.Graph, so the two denote the same graph.
func appendWorkInstancesXML(b []byte, wi *WorkInstances, workBase string, instanceBases []string) []byte {
	wb := sanitizeID(workBase)
	s := newXMLSink(b)
	emitWork(s, &wi.Work, wb, iriVal{}, instanceIRIs(instanceBases))
	for i := range wi.Instances {
		emitInstance(s, &wi.Instances[i], sanitizeID(instanceBases[i]), wb)
	}
	return s.b
}

// ---- text escaping ----

// appendXMLText appends s as XML character data, escaping the markup-significant
// characters, dropping invalid UTF-8 and control bytes XML 1.0 cannot represent.
// Runs of ordinary characters are copied in one append, so clean text (the common
// case) costs a single copy.
func appendXMLText(b []byte, s string) []byte {
	last := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c >= 0x80 {
			if r, size := utf8.DecodeRuneInString(s[i:]); r == utf8.RuneError && size == 1 {
				b = append(b, s[last:i]...)
				i++
				last = i
			} else {
				i += size
			}
			continue
		}
		var esc string
		switch c {
		case '&':
			esc = "&amp;"
		case '<':
			esc = "&lt;"
		case '>':
			esc = "&gt;"
		case '\r':
			esc = "&#xD;"
		default:
			if c >= 0x20 || c == '\t' || c == '\n' {
				i++
				continue
			}
		}
		b = append(b, s[last:i]...)
		if esc != "" {
			b = append(b, esc...)
		}
		i++
		last = i
	}
	return append(b, s[last:]...)
}

// appendXMLAttr appends s as an XML attribute value (also escaping quotes and
// whitespace), copying runs of ordinary characters in one append.
func appendXMLAttr(b []byte, s string) []byte {
	last := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c >= 0x80 {
			if r, size := utf8.DecodeRuneInString(s[i:]); r == utf8.RuneError && size == 1 {
				b = append(b, s[last:i]...)
				i++
				last = i
			} else {
				i += size
			}
			continue
		}
		var esc string
		switch c {
		case '&':
			esc = "&amp;"
		case '<':
			esc = "&lt;"
		case '>':
			esc = "&gt;"
		case '"':
			esc = "&quot;"
		case '\r':
			esc = "&#xD;"
		case '\n':
			esc = "&#xA;"
		case '\t':
			esc = "&#x9;"
		default:
			if c >= 0x20 {
				i++
				continue
			}
		}
		b = append(b, s[last:i]...)
		if esc != "" {
			b = append(b, esc...)
		}
		i++
		last = i
	}
	return append(b, s[last:]...)
}
