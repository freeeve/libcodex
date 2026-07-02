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
	b = appendWorkHeadJSONLD(b, &g.Work, base)
	b = append(b, `,"bf:hasInstance":{"@id":`...)
	b = appendJSONString(b, instanceURI(base))
	return append(b, "}}"...)
}

// appendWorkHeadJSONLD opens the Work object and emits its @type and child
// properties up to (but not including) bf:hasInstance and the closing brace,
// shared by the single-pair and multi-instance encoders.
func appendWorkHeadJSONLD(b []byte, w *Work, workBase string) []byte {
	b = append(b, `{"@id":`...)
	b = appendJSONString(b, workURI(workBase))
	if w.Class != "" {
		b = append(b, `,"@type":["bf:Work","bf:`...)
		b = append(b, w.Class...)
		b = append(b, `"]`...)
	} else {
		b = append(b, `,"@type":"bf:Work"`...)
	}
	if len(w.Titles) > 0 {
		b = append(b, `,"bf:title":[`...)
		for i, t := range w.Titles {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendTitleJSON(b, t)
		}
		b = append(b, ']')
	}
	if len(w.Contributions) > 0 {
		b = append(b, `,"bf:contribution":[`...)
		for i, c := range w.Contributions {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendContributionJSON(b, c)
		}
		b = append(b, ']')
	}
	b = labeledArrayJSON(b, "bf:subject", subjectClasses(w.Subjects), subjectLabels(w.Subjects))
	b = simpleLabeledArrayJSON(b, "bf:genreForm", "bf:GenreForm", w.GenreForms)
	if len(w.Languages) > 0 {
		b = append(b, `,"bf:language":[`...)
		for i, code := range w.Languages {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"@id":"`...)
			b = append(b, langVocab...)
			b = append(b, code...)
			b = append(b, `","@type":"bf:Language","rdfs:label":"`...)
			b = append(b, code...)
			b = append(b, `"}`...)
		}
		b = append(b, ']')
	}
	if len(w.Classifications) > 0 {
		b = append(b, `,"bf:classification":[`...)
		for i, c := range w.Classifications {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"@type":"bf:`...)
			b = append(b, c.Class...)
			b = append(b, `","bf:classificationPortion":`...)
			b = appendJSONString(b, c.Value)
			if c.Source != "" {
				b = append(b, `,"bf:source":`...)
				b = appendLabeledJSON(b, "bf:Source", c.Source)
			}
			b = append(b, '}')
		}
		b = append(b, ']')
	}
	return simpleLabeledArrayJSON(b, "bf:summary", "bf:Summary", w.Summary)
}

func appendInstanceJSONLD(b []byte, g *BIBFRAME, base string) []byte {
	return appendInstanceNodeJSONLD(b, &g.Instance, base, base)
}

// appendInstanceNodeJSONLD emits one bf:Instance node object under instBase,
// linked bf:instanceOf back to workBase.
func appendInstanceNodeJSONLD(b []byte, in *Instance, instBase, workBase string) []byte {
	b = append(b, `{"@id":`...)
	b = appendJSONString(b, instanceURI(instBase))
	b = append(b, `,"@type":"bf:Instance","bf:instanceOf":{"@id":`...)
	b = appendJSONString(b, workURI(workBase))
	b = append(b, '}')
	if len(in.Titles) > 0 {
		b = append(b, `,"bf:title":[`...)
		for i, t := range in.Titles {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendTitleJSON(b, t)
		}
		b = append(b, ']')
	}
	if in.ResponsibilityStatement != "" {
		b = append(b, `,"bf:responsibilityStatement":`...)
		b = appendJSONString(b, in.ResponsibilityStatement)
	}
	if in.EditionStatement != "" {
		b = append(b, `,"bf:editionStatement":`...)
		b = appendJSONString(b, in.EditionStatement)
	}
	if p := in.Provision; p != nil {
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
	b = simpleLabeledArrayJSON(b, "bf:extent", "bf:Extent", in.Extent)
	if in.Media != "" {
		b = append(b, `,"bf:media":`...)
		b = appendLabeledJSON(b, "bf:Media", in.Media)
	}
	if in.Carrier != "" {
		b = append(b, `,"bf:carrier":`...)
		b = appendLabeledJSON(b, "bf:Carrier", in.Carrier)
	}
	if len(in.Identifiers) > 0 {
		b = append(b, `,"bf:identifiedBy":[`...)
		for i, id := range in.Identifiers {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"@type":"bf:`...)
			b = append(b, id.Class...)
			b = append(b, `","rdf:value":`...)
			b = appendJSONString(b, id.Value)
			if id.Source != "" {
				b = append(b, `,"bf:source":`...)
				b = appendLabeledJSON(b, "bf:Source", id.Source)
			}
			b = append(b, '}')
		}
		b = append(b, ']')
	}
	if len(in.ElectronicLocator) > 0 {
		b = append(b, `,"bf:electronicLocator":[`...)
		for i, u := range in.ElectronicLocator {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"@id":`...)
			b = appendJSONString(b, u)
			b = append(b, '}')
		}
		b = append(b, ']')
	}
	b = appendAdminMetadataJSON(b, in.Admin)
	return append(b, '}')
}

// appendWorkInstancesJSONLD emits a Work node with one bf:hasInstance per
// Instance, then each Instance node object, mirroring WorkInstances.Graph.
func appendWorkInstancesJSONLD(b []byte, wi *WorkInstances, workBase string, instanceBases []string) []byte {
	wb := sanitizeID(workBase)
	b = appendWorkHeadJSONLD(b, &wi.Work, wb)
	if len(instanceBases) > 0 {
		b = append(b, `,"bf:hasInstance":[`...)
		for i, ib := range instanceBases {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"@id":`...)
			b = appendJSONString(b, instanceURI(sanitizeID(ib)))
			b = append(b, '}')
		}
		b = append(b, ']')
	}
	b = append(b, '}') // close the Work object
	for i := range wi.Instances {
		b = append(b, ',')
		b = appendInstanceNodeJSONLD(b, &wi.Instances[i], sanitizeID(instanceBases[i]), wb)
	}
	return b
}

// appendAdminMetadataJSON renders the bf:AdminMetadata node, mirroring the graph
// builder's adminMetadata triples.
func appendAdminMetadataJSON(b []byte, am *AdminMetadata) []byte {
	if am == nil {
		return b
	}
	b = append(b, `,"bf:adminMetadata":{"@type":"bf:AdminMetadata"`...)
	b = append(b, `,"bf:generationProcess":{"@type":"bf:GenerationProcess","rdfs:label":`...)
	b = appendJSONString(b, generatorLabel)
	b = append(b, '}')
	if am.ChangeDate != "" {
		b = append(b, `,"bf:changeDate":`...)
		b = appendJSONString(b, am.ChangeDate)
	}
	if am.DescriptionConventions != "" {
		b = append(b, `,"bf:descriptionConventions":`...)
		b = appendJSONString(b, am.DescriptionConventions)
	}
	if am.ControlNumber != "" {
		b = append(b, `,"bf:identifiedBy":{"@type":"bf:Local","rdf:value":`...)
		b = appendJSONString(b, am.ControlNumber)
		b = append(b, '}')
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
