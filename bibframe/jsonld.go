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
// for one record.
func appendGraphJSONLD(b []byte, g *BIBFRAME, base string) []byte {
	b = appendWorkJSONLD(b, g, base)
	b = append(b, ',')
	b = appendInstanceJSONLD(b, g, base)
	return b
}

func appendWorkJSONLD(b []byte, g *BIBFRAME, base string) []byte {
	b = append(b, `{"@id":`...)
	b = appendJSONString(b, workURI(base))
	if g.Work.Class != "" {
		b = append(b, `,"@type":["bf:Work","bf:`...)
		b = append(b, g.Work.Class...)
		b = append(b, `"]`...)
	} else {
		b = append(b, `,"@type":"bf:Work"`...)
	}
	if len(g.Work.Titles) > 0 {
		b = append(b, `,"bf:title":[`...)
		for i, t := range g.Work.Titles {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendTitleJSON(b, t)
		}
		b = append(b, ']')
	}
	if len(g.Work.Contributions) > 0 {
		b = append(b, `,"bf:contribution":[`...)
		for i, c := range g.Work.Contributions {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendContributionJSON(b, c)
		}
		b = append(b, ']')
	}
	b = labeledArrayJSON(b, "bf:subject", subjectClasses(g.Work.Subjects), subjectLabels(g.Work.Subjects))
	b = simpleLabeledArrayJSON(b, "bf:genreForm", "bf:GenreForm", g.Work.GenreForms)
	if len(g.Work.Languages) > 0 {
		b = append(b, `,"bf:language":[`...)
		for i, code := range g.Work.Languages {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"@id":"`...)
			b = append(b, langVocab...)
			b = append(b, code...)
			b = append(b, `","rdfs:label":"`...)
			b = append(b, code...)
			b = append(b, `"}`...)
		}
		b = append(b, ']')
	}
	if len(g.Work.Classifications) > 0 {
		b = append(b, `,"bf:classification":[`...)
		for i, c := range g.Work.Classifications {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"@type":"bf:`...)
			b = append(b, c.Class...)
			b = append(b, `","bf:classificationPortion":`...)
			b = appendJSONString(b, c.Value)
			b = append(b, '}')
		}
		b = append(b, ']')
	}
	b = simpleLabeledArrayJSON(b, "bf:summary", "bf:Summary", g.Work.Summary)
	b = append(b, `,"bf:hasInstance":{"@id":`...)
	b = appendJSONString(b, instanceURI(base))
	return append(b, "}}"...)
}

func appendInstanceJSONLD(b []byte, g *BIBFRAME, base string) []byte {
	b = append(b, `{"@id":`...)
	b = appendJSONString(b, instanceURI(base))
	b = append(b, `,"@type":"bf:Instance","bf:instanceOf":{"@id":`...)
	b = appendJSONString(b, workURI(base))
	b = append(b, '}')
	if len(g.Instance.Titles) > 0 {
		b = append(b, `,"bf:title":[`...)
		for i, t := range g.Instance.Titles {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendTitleJSON(b, t)
		}
		b = append(b, ']')
	}
	if g.Instance.ResponsibilityStatement != "" {
		b = append(b, `,"bf:responsibilityStatement":`...)
		b = appendJSONString(b, g.Instance.ResponsibilityStatement)
	}
	if g.Instance.EditionStatement != "" {
		b = append(b, `,"bf:editionStatement":`...)
		b = appendJSONString(b, g.Instance.EditionStatement)
	}
	if p := g.Instance.Provision; p != nil {
		b = append(b, `,"bf:provisionActivity":{"@type":"bf:Publication"`...)
		if p.Place != "" {
			b = append(b, `,"bf:place":`...)
			b = appendLabeledJSON(b, "bf:Place", p.Place)
		}
		if p.Publisher != "" {
			b = append(b, `,"bf:agent":`...)
			b = appendLabeledJSON(b, "bf:Agent", p.Publisher)
		}
		if p.Date != "" {
			b = append(b, `,"bf:date":`...)
			b = appendJSONString(b, p.Date)
		}
		b = append(b, '}')
	}
	b = simpleLabeledArrayJSON(b, "bf:extent", "bf:Extent", g.Instance.Extent)
	if len(g.Instance.Identifiers) > 0 {
		b = append(b, `,"bf:identifiedBy":[`...)
		for i, id := range g.Instance.Identifiers {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"@type":"bf:`...)
			b = append(b, id.Class...)
			b = append(b, `","rdf:value":`...)
			b = appendJSONString(b, id.Value)
			b = append(b, '}')
		}
		b = append(b, ']')
	}
	if len(g.Instance.ElectronicLocator) > 0 {
		b = append(b, `,"bf:electronicLocator":[`...)
		for i, u := range g.Instance.ElectronicLocator {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"@id":`...)
			b = appendJSONString(b, u)
			b = append(b, '}')
		}
		b = append(b, ']')
	}
	return append(b, '}')
}

// ---- node fragments ----

func appendTitleJSON(b []byte, t Title) []byte {
	b = append(b, `{"@type":"bf:Title","bf:mainTitle":`...)
	b = appendJSONString(b, t.MainTitle)
	if t.Subtitle != "" {
		b = append(b, `,"bf:subtitle":`...)
		b = appendJSONString(b, t.Subtitle)
	}
	if t.PartNumber != "" {
		b = append(b, `,"bf:partNumber":`...)
		b = appendJSONString(b, t.PartNumber)
	}
	if t.PartName != "" {
		b = append(b, `,"bf:partName":`...)
		b = appendJSONString(b, t.PartName)
	}
	return append(b, '}')
}

func appendContributionJSON(b []byte, c Contribution) []byte {
	if c.Primary {
		b = append(b, `{"@type":"bflc:PrimaryContribution"`...)
	} else {
		b = append(b, `{"@type":"bf:Contribution"`...)
	}
	b = append(b, `,"bf:agent":{"@type":"bf:`...)
	b = append(b, c.Class...)
	b = append(b, `","rdfs:label":`...)
	b = appendJSONString(b, c.Label)
	b = append(b, '}')
	if c.Role != "" {
		b = append(b, `,"bf:role":{"@type":"bf:Role","rdfs:label":`...)
		b = appendJSONString(b, c.Role)
		b = append(b, '}')
	}
	return append(b, '}')
}

func appendLabeledJSON(b []byte, class, label string) []byte {
	b = append(b, `{"@type":"`...)
	b = append(b, class...)
	b = append(b, `","rdfs:label":`...)
	b = appendJSONString(b, label)
	return append(b, '}')
}

// simpleLabeledArrayJSON appends `,"key":[ {labeled} … ]` for a slice of labels
// that all share one class, or nothing when the slice is empty.
func simpleLabeledArrayJSON(b []byte, key, class string, labels []string) []byte {
	if len(labels) == 0 {
		return b
	}
	b = append(b, ',')
	b = appendJSONString(b, key)
	b = append(b, ':', '[')
	for i, l := range labels {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendLabeledJSON(b, class, l)
	}
	return append(b, ']')
}

// labeledArrayJSON appends `,"key":[ … ]` for parallel class/label slices.
func labeledArrayJSON(b []byte, key string, classes, labels []string) []byte {
	if len(labels) == 0 {
		return b
	}
	b = append(b, ',')
	b = appendJSONString(b, key)
	b = append(b, ':', '[')
	for i := range labels {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendLabeledJSON(b, classes[i], labels[i])
	}
	return append(b, ']')
}

func subjectClasses(s []Subject) []string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = "bf:" + s[i].Class
	}
	return out
}

func subjectLabels(s []Subject) []string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = s[i].Label
	}
	return out
}

const hexDigits = "0123456789abcdef"

// appendJSONString appends s as a quoted JSON string, escaping control and
// markup-significant characters and dropping invalid UTF-8.
func appendJSONString(b []byte, s string) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			i++
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
				if c < 0x20 {
					b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
				} else {
					b = append(b, c)
				}
			}
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
	return append(b, '"')
}
