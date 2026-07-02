package mods

import "github.com/freeeve/libcodex/internal/crosswalk"

// appendMODS serializes m as an indented <mods> element. base is the
// indentation of the <mods> line (empty for a standalone document, one level in
// for a collection member); step is one indent level. This hand-rolled writer
// replaces reflection-based xml.Marshal so the Writer allocates a single reused
// buffer per record instead of ~50 allocations, matching the dublincore and
// schemaorg serializers. The element and attribute names mirror the struct's xml
// tags, so the output still round-trips through xml.Unmarshal.
func appendMODS(b []byte, m *MODS, base, step string) []byte {
	in1 := base + step
	in2 := in1 + step
	in3 := in2 + step

	b = append(b, base...)
	b = append(b, "<mods"...)
	b = attr(b, "xmlns", m.Xmlns)
	b = attr(b, "version", m.Version)
	b = append(b, ">\n"...)

	for _, t := range m.TitleInfo {
		b = append(b, in1...)
		b = append(b, "<titleInfo"...)
		b = attr(b, "type", t.Type)
		b = append(b, ">\n"...)
		b = elem(b, in2, "title", t.Title)
		b = elem(b, in2, "subTitle", t.SubTitle)
		b = elem(b, in2, "partNumber", t.PartNumber)
		b = elem(b, in2, "partName", t.PartName)
		b = closeElem(b, in1, "titleInfo")
	}
	for _, n := range m.Name {
		b = appendNameElem(b, n, in1, step)
	}
	b = elem(b, in1, "typeOfResource", m.TypeOfResource)
	if o := m.OriginInfo; o != nil {
		b = append(b, in1...)
		b = append(b, "<originInfo>\n"...)
		for _, p := range o.Place {
			b = append(b, in2...)
			b = append(b, "<place>\n"...)
			b = elemAttr(b, in3, "placeTerm", "type", p.PlaceTerm.Type, p.PlaceTerm.Value)
			b = closeElem(b, in2, "place")
		}
		b = elem(b, in2, "publisher", o.Publisher)
		b = elem(b, in2, "dateIssued", o.DateIssued)
		b = elem(b, in2, "copyrightDate", o.CopyrightDate)
		b = elem(b, in2, "edition", o.Edition)
		b = closeElem(b, in1, "originInfo")
	}
	for _, l := range m.Language {
		b = append(b, in1...)
		b = append(b, "<language>\n"...)
		b = append(b, in2...)
		b = append(b, "<languageTerm"...)
		b = attr(b, "type", l.LanguageTerm.Type)
		b = attr(b, "authority", l.LanguageTerm.Authority)
		b = append(b, '>')
		b = crosswalk.AppendXMLText(b, l.LanguageTerm.Value)
		b = append(b, "</languageTerm>\n"...)
		b = closeElem(b, in1, "language")
	}
	if p := m.PhysicalDesc; p != nil {
		b = append(b, in1...)
		b = append(b, "<physicalDescription>\n"...)
		b = elem(b, in2, "extent", p.Extent)
		b = closeElem(b, in1, "physicalDescription")
	}
	for _, nt := range m.Note {
		b = elemAttr(b, in1, "note", "type", nt.Type, nt.Value)
	}
	for _, s := range m.Subject {
		b = appendSubject(b, s, in1, step)
	}
	for _, id := range m.Identifier {
		b = elemAttr(b, in1, "identifier", "type", id.Type, id.Value)
	}
	if ri := m.RecordInfo; ri != nil {
		b = append(b, in1...)
		b = append(b, "<recordInfo>\n"...)
		b = elem(b, in2, "recordIdentifier", ri.RecordIdentifier)
		b = closeElem(b, in1, "recordInfo")
	}

	b = append(b, base...)
	return append(b, "</mods>"...)
}

// appendName writes a <name> element (also used for a subject's name).
func appendNameElem(b []byte, n Name, indent, step string) []byte {
	in2 := indent + step
	in3 := in2 + step
	b = append(b, indent...)
	b = append(b, "<name"...)
	b = attr(b, "type", n.Type)
	b = append(b, ">\n"...)
	for _, np := range n.NamePart {
		b = elemAttr(b, in2, "namePart", "type", np.Type, np.Value)
	}
	if n.Role != nil {
		b = append(b, in2...)
		b = append(b, "<role>\n"...)
		b = elemAttr(b, in3, "roleTerm", "type", n.Role.RoleTerm.Type, n.Role.RoleTerm.Value)
		b = closeElem(b, in2, "role")
	}
	return closeElem(b, indent, "name")
}

// appendSubject writes a <subject> element.
func appendSubject(b []byte, s Subject, indent, step string) []byte {
	in2 := indent + step
	b = append(b, indent...)
	b = append(b, "<subject"...)
	b = attr(b, "authority", s.Authority)
	b = append(b, ">\n"...)
	for _, v := range s.Topic {
		b = elem(b, in2, "topic", v)
	}
	for _, v := range s.Geographic {
		b = elem(b, in2, "geographic", v)
	}
	for _, v := range s.Temporal {
		b = elem(b, in2, "temporal", v)
	}
	for _, v := range s.Genre {
		b = elem(b, in2, "genre", v)
	}
	if s.Name != nil {
		b = appendNameElem(b, *s.Name, in2, step)
	}
	return closeElem(b, indent, "subject")
}

// elem writes <name>text</name> at indent (with a trailing newline) when value
// is non-empty, matching the omitempty struct tags.
func elem(b []byte, indent, name, value string) []byte {
	if value == "" {
		return b
	}
	b = append(b, indent...)
	b = append(b, '<')
	b = append(b, name...)
	b = append(b, '>')
	b = crosswalk.AppendXMLText(b, value)
	b = append(b, "</"...)
	b = append(b, name...)
	return append(b, ">\n"...)
}

// elemAttr writes <name attr="attrVal">text</name> when value is non-empty; the
// attribute is omitted when attrVal is empty.
func elemAttr(b []byte, indent, name, attrName, attrVal, value string) []byte {
	if value == "" {
		return b
	}
	b = append(b, indent...)
	b = append(b, '<')
	b = append(b, name...)
	b = attr(b, attrName, attrVal)
	b = append(b, '>')
	b = crosswalk.AppendXMLText(b, value)
	b = append(b, "</"...)
	b = append(b, name...)
	return append(b, ">\n"...)
}

// attr writes ` name="value"` when value is non-empty, matching attr,omitempty.
func attr(b []byte, name, value string) []byte {
	if value == "" {
		return b
	}
	b = append(b, ' ')
	b = append(b, name...)
	b = append(b, '=', '"')
	b = crosswalk.AppendXMLText(b, value)
	return append(b, '"')
}

// closeElem writes a closing </name> tag at indent with a trailing newline.
func closeElem(b []byte, indent, name string) []byte {
	b = append(b, indent...)
	b = append(b, "</"...)
	b = append(b, name...)
	return append(b, ">\n"...)
}
