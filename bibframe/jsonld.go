package bibframe

import "unicode/utf8"

// jsonldContext is the opening of a JSON-LD document: the object brace and the
// @context prefix map. Callers append `,"@graph":[ … ]}`.
const jsonldContext = `{"@context":{` +
	`"bf":"` + bfNS + `",` +
	`"bflc":"` + bflcNS + `",` +
	`"rdf":"` + rdfNS + `",` +
	`"rdfs":"` + rdfsNS + `"}`

// appendGraphJSONLD appends the Work and Instance node objects (comma-separated)
// for one record. The node shape comes from shape.go; jsonNode does the formatting.
func appendGraphJSONLD(b []byte, g *BIBFRAME, base string) []byte {
	s := newJSONSink(b)
	emitWork(s, &g.Work, base, instanceIRIVal(base), nil)
	s.b = append(s.b, ',')
	emitInstance(s, &g.Instance, base, base)
	return s.b
}

// appendWorkInstancesJSONLD emits a Work node with one bf:hasInstance per
// Instance, then each Instance node object, mirroring WorkInstances.Graph.
func appendWorkInstancesJSONLD(b []byte, wi *WorkInstances, workBase string, instanceBases []string) []byte {
	wb := sanitizeID(workBase)
	s := newJSONSink(b)
	emitWork(s, &wi.Work, wb, iriVal{}, instanceIRIs(instanceBases))
	for i := range wi.Instances {
		s.b = append(s.b, ',')
		emitInstance(s, &wi.Instances[i], sanitizeID(instanceBases[i]), wb)
	}
	return s.b
}

const hexDigits = "0123456789abcdef"

// appendJSONString appends s as a quoted JSON string, escaping control and
// markup-significant characters and dropping invalid UTF-8.
func appendJSONString(b []byte, s string) []byte {
	b = append(b, '"')
	b = appendJSONBody(b, s)
	return append(b, '"')
}

// appendJSONBody appends s escaped as JSON string content, without the surrounding
// quotes, so a caller can build a string from several parts. Escaping is per rune,
// so escaping the parts separately equals escaping their concatenation.
func appendJSONBody(b []byte, s string) []byte {
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
		if c >= 0x20 && c != '"' && c != '\\' {
			i++
			continue
		}
		b = append(b, s[last:i]...)
		switch c {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
		}
		i++
		last = i
	}
	return append(b, s[last:]...)
}
