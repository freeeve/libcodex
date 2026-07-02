package bibframe

import "unicode/utf8"

const (
	xmlHeader = `<?xml version="1.0" encoding="UTF-8"?>`
	rdfOpen   = `<rdf:RDF xmlns:rdf="` + rdfNS + `" xmlns:rdfs="` + rdfsNS + `"` +
		` xmlns:bf="` + bfNS + `" xmlns:bflc="` + bflcNS + `">`
	rdfClose = `</rdf:RDF>`
)

// appendGraphXML appends the Work and Instance nodes for one record. base is the
// local identifier stem shared by the two nodes.
func appendGraphXML(b []byte, g *BIBFRAME, base string) []byte {
	b = appendWorkXML(b, g, base)
	b = appendInstanceXML(b, g, base)
	return b
}

func appendWorkXML(b []byte, g *BIBFRAME, base string) []byte {
	b = openNode(b, "bf:Work", workURI(base))
	b = appendWorkBodyXML(b, &g.Work)
	b = resourceRef(b, "    ", "bf:hasInstance", instanceURI(base))
	return append(b, "  </bf:Work>\n"...)
}

// appendWorkBodyXML emits the Work's child properties (everything inside the
// bf:Work element except the opening tag and the bf:hasInstance links), shared by
// the single-pair and multi-instance encoders.
func appendWorkBodyXML(b []byte, w *Work) []byte {
	if w.Class != "" {
		b = typeRef(b, w.Class)
	}
	for _, t := range w.Titles {
		b = appendTitleXML(b, t)
	}
	for _, c := range w.Contributions {
		b = appendContributionXML(b, c)
	}
	for _, s := range w.Subjects {
		b = labeledXML(b, "bf:subject", "bf:"+s.Class, s.Label)
	}
	for _, gf := range w.GenreForms {
		b = labeledXML(b, "bf:genreForm", "bf:GenreForm", gf)
	}
	for _, lang := range w.Languages {
		b = appendLanguageXML(b, lang)
	}
	for _, c := range w.Classifications {
		b = appendClassificationXML(b, c)
	}
	for _, s := range w.Summary {
		b = labeledXML(b, "bf:summary", "bf:Summary", s)
	}
	return b
}

func appendInstanceXML(b []byte, g *BIBFRAME, base string) []byte {
	return appendInstanceNodeXML(b, &g.Instance, base, base)
}

// appendInstanceNodeXML emits one bf:Instance node under instBase, linked
// bf:instanceOf back to workBase. The two bases are independent so a Work with
// several Instances can give each Instance its own IRI.
func appendInstanceNodeXML(b []byte, in *Instance, instBase, workBase string) []byte {
	b = openNode(b, "bf:Instance", instanceURI(instBase))
	b = resourceRef(b, "    ", "bf:instanceOf", workURI(workBase))
	for _, t := range in.Titles {
		b = appendTitleXML(b, t)
	}
	if in.ResponsibilityStatement != "" {
		b = leafXML(b, "    ", "bf:responsibilityStatement", in.ResponsibilityStatement)
	}
	if in.EditionStatement != "" {
		b = leafXML(b, "    ", "bf:editionStatement", in.EditionStatement)
	}
	if p := in.Provision; p != nil {
		b = appendProvisionXML(b, p)
	}
	for _, e := range in.Extent {
		b = labeledXML(b, "bf:extent", "bf:Extent", e)
	}
	if in.Media != "" {
		b = labeledXML(b, "bf:media", "bf:Media", in.Media)
	}
	if in.Carrier != "" {
		b = labeledXML(b, "bf:carrier", "bf:Carrier", in.Carrier)
	}
	for _, id := range in.Identifiers {
		b = appendIdentifierXML(b, id)
	}
	for _, u := range in.ElectronicLocator {
		b = resourceRef(b, "    ", "bf:electronicLocator", u)
	}
	b = appendAdminMetadataXML(b, in.Admin)
	return append(b, "  </bf:Instance>\n"...)
}

// appendWorkInstancesXML emits a Work with N Instances: the Work node once with
// one bf:hasInstance per Instance, then each Instance under its own IRI. It
// mirrors WorkInstances.Graph, so the two denote the same graph.
func appendWorkInstancesXML(b []byte, wi *WorkInstances, workBase string, instanceBases []string) []byte {
	wb := sanitizeID(workBase)
	b = openNode(b, "bf:Work", workURI(wb))
	b = appendWorkBodyXML(b, &wi.Work)
	for _, ib := range instanceBases {
		b = resourceRef(b, "    ", "bf:hasInstance", instanceURI(sanitizeID(ib)))
	}
	b = append(b, "  </bf:Work>\n"...)
	for i := range wi.Instances {
		b = appendInstanceNodeXML(b, &wi.Instances[i], sanitizeID(instanceBases[i]), wb)
	}
	return b
}

// appendAdminMetadataXML renders the bf:AdminMetadata node, mirroring the graph
// builder's adminMetadata triples.
func appendAdminMetadataXML(b []byte, am *AdminMetadata) []byte {
	if am == nil {
		return b
	}
	b = append(b, "    <bf:adminMetadata>\n      <bf:AdminMetadata>\n"...)
	b = append(b, "        <bf:generationProcess>\n          <bf:GenerationProcess>\n            <rdfs:label>"...)
	b = appendXMLText(b, generatorLabel)
	b = append(b, "</rdfs:label>\n          </bf:GenerationProcess>\n        </bf:generationProcess>\n"...)
	if am.ChangeDate != "" {
		b = leafXML(b, "        ", "bf:changeDate", am.ChangeDate)
	}
	if am.DescriptionConventions != "" {
		b = leafXML(b, "        ", "bf:descriptionConventions", am.DescriptionConventions)
	}
	if am.ControlNumber != "" {
		b = append(b, "        <bf:identifiedBy>\n          <bf:Local>\n            <rdf:value>"...)
		b = appendXMLText(b, am.ControlNumber)
		b = append(b, "</rdf:value>\n          </bf:Local>\n        </bf:identifiedBy>\n"...)
	}
	return append(b, "      </bf:AdminMetadata>\n    </bf:adminMetadata>\n"...)
}

// ---- node fragments ----

// appendTitleXML renders a bf:title. The transcribed and uniform titles both
// serialize as bf:Title; the distinction is carried by which resource (Instance
// vs Work) holds them.
func appendTitleXML(b []byte, t Title) []byte {
	b = append(b, "    <bf:title>\n      <bf:Title>\n"...)
	b = leafXML(b, "        ", "bf:mainTitle", t.MainTitle)
	if t.Subtitle != "" {
		b = leafXML(b, "        ", "bf:subtitle", t.Subtitle)
	}
	if t.PartNumber != "" {
		b = leafXML(b, "        ", "bf:partNumber", t.PartNumber)
	}
	if t.PartName != "" {
		b = leafXML(b, "        ", "bf:partName", t.PartName)
	}
	return append(b, "      </bf:Title>\n    </bf:title>\n"...)
}

func appendContributionXML(b []byte, c Contribution) []byte {
	wrap := "bf:Contribution"
	if c.Primary {
		wrap = "bflc:PrimaryContribution"
	}
	b = append(b, "    <bf:contribution>\n      <"...)
	b = append(b, wrap...)
	b = append(b, ">\n        <bf:agent>\n          <bf:"...)
	b = append(b, c.Class...)
	b = append(b, ">\n            <rdfs:label>"...)
	b = appendXMLText(b, c.Label)
	b = append(b, "</rdfs:label>\n          </bf:"...)
	b = append(b, c.Class...)
	b = append(b, ">\n        </bf:agent>\n"...)
	if c.Role != "" {
		b = append(b, "        <bf:role>\n          <bf:Role>\n            <rdfs:label>"...)
		b = appendXMLText(b, c.Role)
		b = append(b, "</rdfs:label>\n          </bf:Role>\n        </bf:role>\n"...)
	}
	b = append(b, "      </"...)
	b = append(b, wrap...)
	return append(b, ">\n    </bf:contribution>\n"...)
}

func appendLanguageXML(b []byte, code string) []byte {
	b = append(b, "    <bf:language>\n      <bf:Language rdf:about=\""...)
	b = append(b, langVocab...)
	b = append(b, code...) // code is 3 ASCII letters; no escaping needed
	b = append(b, "\">\n        <rdfs:label>"...)
	b = append(b, code...)
	return append(b, "</rdfs:label>\n      </bf:Language>\n    </bf:language>\n"...)
}

func appendClassificationXML(b []byte, c Classification) []byte {
	b = append(b, "    <bf:classification>\n      <bf:"...)
	b = append(b, c.Class...)
	b = append(b, ">\n        <bf:classificationPortion>"...)
	b = appendXMLText(b, c.Value)
	b = append(b, "</bf:classificationPortion>\n"...)
	if c.Source != "" {
		b = labeledXMLAt(b, "        ", "bf:source", "bf:Source", c.Source)
	}
	b = append(b, "      </bf:"...)
	b = append(b, c.Class...)
	return append(b, ">\n    </bf:classification>\n"...)
}

func appendProvisionXML(b []byte, p *Provision) []byte {
	b = append(b, "    <bf:provisionActivity>\n      <bf:Publication>\n"...)
	if p.Place != "" {
		b = labeledXMLAt(b, "        ", "bf:place", "bf:Place", p.Place)
	}
	if p.Publisher != "" {
		b = labeledXMLAt(b, "        ", "bf:agent", "bf:Agent", p.Publisher)
	}
	if p.Date != "" {
		b = leafXML(b, "        ", "bf:date", p.Date)
	}
	return append(b, "      </bf:Publication>\n    </bf:provisionActivity>\n"...)
}

func appendIdentifierXML(b []byte, id Identifier) []byte {
	b = append(b, "    <bf:identifiedBy>\n      <bf:"...)
	b = append(b, id.Class...)
	b = append(b, ">\n        <rdf:value>"...)
	b = appendXMLText(b, id.Value)
	b = append(b, "</rdf:value>\n"...)
	if id.Source != "" {
		b = labeledXMLAt(b, "        ", "bf:source", "bf:Source", id.Source)
	}
	b = append(b, "      </bf:"...)
	b = append(b, id.Class...)
	return append(b, ">\n    </bf:identifiedBy>\n"...)
}

// ---- low-level helpers ----

func openNode(b []byte, class, uri string) []byte {
	b = append(b, "  <"...)
	b = append(b, class...)
	b = append(b, " rdf:about=\""...)
	b = append(b, uri...) // uri is a sanitized fragment; no escaping needed
	return append(b, "\">\n"...)
}

func typeRef(b []byte, class string) []byte {
	b = append(b, "    <rdf:type rdf:resource=\""...)
	b = append(b, bfNS...)
	b = append(b, class...)
	return append(b, "\"/>\n"...)
}

func resourceRef(b []byte, indent, prop, uri string) []byte {
	b = append(b, indent...)
	b = append(b, '<')
	b = append(b, prop...)
	b = append(b, " rdf:resource=\""...)
	b = appendXMLAttr(b, uri)
	return append(b, "\"/>\n"...)
}

// leafXML appends <prop>text</prop> at the given indent.
func leafXML(b []byte, indent, prop, text string) []byte {
	b = append(b, indent...)
	b = append(b, '<')
	b = append(b, prop...)
	b = append(b, '>')
	b = appendXMLText(b, text)
	b = append(b, "</"...)
	b = append(b, prop...)
	return append(b, ">\n"...)
}

// labeledXML appends <prop><class><rdfs:label>label</rdfs:label></class></prop>
// indented four spaces (a direct child of a node).
func labeledXML(b []byte, prop, class, label string) []byte {
	return labeledXMLAt(b, "    ", prop, class, label)
}

func labeledXMLAt(b []byte, indent, prop, class, label string) []byte {
	b = append(b, indent...)
	b = append(b, '<')
	b = append(b, prop...)
	b = append(b, ">\n"...)
	b = append(b, indent...)
	b = append(b, "  <"...)
	b = append(b, class...)
	b = append(b, ">\n"...)
	b = append(b, indent...)
	b = append(b, "    <rdfs:label>"...)
	b = appendXMLText(b, label)
	b = append(b, "</rdfs:label>\n"...)
	b = append(b, indent...)
	b = append(b, "  </"...)
	b = append(b, class...)
	b = append(b, ">\n"...)
	b = append(b, indent...)
	b = append(b, "</"...)
	b = append(b, prop...)
	return append(b, ">\n"...)
}

// appendXMLText appends s as XML character data, escaping the markup-significant
// characters, dropping invalid UTF-8 and control bytes XML 1.0 cannot represent.
func appendXMLText(b []byte, s string) []byte {
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			i++
			switch c {
			case '&':
				b = append(b, "&amp;"...)
			case '<':
				b = append(b, "&lt;"...)
			case '>':
				b = append(b, "&gt;"...)
			case '\r':
				b = append(b, "&#xD;"...)
			default:
				if c >= 0x20 || c == '\t' || c == '\n' {
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
	return b
}

// appendXMLAttr appends s as an XML attribute value (also escaping quotes).
func appendXMLAttr(b []byte, s string) []byte {
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			i++
			switch c {
			case '&':
				b = append(b, "&amp;"...)
			case '<':
				b = append(b, "&lt;"...)
			case '>':
				b = append(b, "&gt;"...)
			case '"':
				b = append(b, "&quot;"...)
			case '\r':
				b = append(b, "&#xD;"...)
			case '\n':
				b = append(b, "&#xA;"...)
			case '\t':
				b = append(b, "&#x9;"...)
			default:
				if c >= 0x20 {
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
	return b
}
