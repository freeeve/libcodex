// Package mods converts MARC 21 records to MODS (Metadata Object Description
// Schema), the Library of Congress XML standard that is richer than MARCXML and
// near-lossless from MARC.
//
// MODS is a different data model than MARC — titleInfo, name, originInfo,
// subject, … — so this is a one-way mapping (a crosswalk following the LoC
// MARC-to-MODS mapping), not a codex.RecordReader. The Writer does implement
// codex.RecordWriter, so it plugs into codex.Convert as a conversion target:
//
//	w := mods.NewWriter(out)
//	codex.Convert(iso2709.NewReader(src), w)
//	w.Close()
//
// The crosswalk covers the common fields (title, names, type, origin, physical
// description, subjects, identifiers, language, notes, record info); fields and
// subfields outside that set are not carried.
package mods

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/crosswalk"
)

// Namespace is the MODS version 3 namespace.
const Namespace = "http://www.loc.gov/mods/v3"

const (
	xmlHeader       = `<?xml version="1.0" encoding="UTF-8"?>`
	collectionOpen  = `<modsCollection xmlns="` + Namespace + `">`
	collectionClose = `</modsCollection>`
)

// MODS is a <mods> record. Empty fields are omitted from the output.
type MODS struct {
	XMLName        xml.Name     `xml:"mods"`
	Xmlns          string       `xml:"xmlns,attr,omitempty"`
	Version        string       `xml:"version,attr,omitempty"`
	TitleInfo      []TitleInfo  `xml:"titleInfo"`
	Name           []Name       `xml:"name"`
	TypeOfResource string       `xml:"typeOfResource,omitempty"`
	OriginInfo     *OriginInfo  `xml:"originInfo,omitempty"`
	Language       []Language   `xml:"language"`
	PhysicalDesc   *Physical    `xml:"physicalDescription,omitempty"`
	Note           []Note       `xml:"note"`
	Subject        []Subject    `xml:"subject"`
	Identifier     []Identifier `xml:"identifier"`
	RecordInfo     *RecordInfo  `xml:"recordInfo,omitempty"`
}

type TitleInfo struct {
	Type       string `xml:"type,attr,omitempty"`
	Title      string `xml:"title,omitempty"`
	SubTitle   string `xml:"subTitle,omitempty"`
	PartNumber string `xml:"partNumber,omitempty"`
	PartName   string `xml:"partName,omitempty"`
}

type Name struct {
	Type     string     `xml:"type,attr,omitempty"`
	NamePart []NamePart `xml:"namePart"`
	Role     *Role      `xml:"role,omitempty"`
}

type NamePart struct {
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:",chardata"`
}

type Role struct {
	RoleTerm RoleTerm `xml:"roleTerm"`
}

type RoleTerm struct {
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:",chardata"`
}

type OriginInfo struct {
	Place      []Place `xml:"place"`
	Publisher  string  `xml:"publisher,omitempty"`
	DateIssued string  `xml:"dateIssued,omitempty"`
	Edition    string  `xml:"edition,omitempty"`
}

type Place struct {
	PlaceTerm PlaceTerm `xml:"placeTerm"`
}

type PlaceTerm struct {
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:",chardata"`
}

type Language struct {
	LanguageTerm LanguageTerm `xml:"languageTerm"`
}

type LanguageTerm struct {
	Type      string `xml:"type,attr,omitempty"`
	Authority string `xml:"authority,attr,omitempty"`
	Value     string `xml:",chardata"`
}

type Physical struct {
	Extent string `xml:"extent,omitempty"`
}

type Note struct {
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:",chardata"`
}

type Subject struct {
	Authority  string   `xml:"authority,attr,omitempty"`
	Topic      []string `xml:"topic"`
	Geographic []string `xml:"geographic"`
	Temporal   []string `xml:"temporal"`
	Genre      []string `xml:"genre"`
	Name       *Name    `xml:"name,omitempty"`
}

type Identifier struct {
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:",chardata"`
}

type RecordInfo struct {
	RecordIdentifier string `xml:"recordIdentifier,omitempty"`
}

// FromRecord maps a MARC record to MODS following the common-field crosswalk. It
// makes a single pass over the fields, dispatching by tag.
func FromRecord(r *codex.Record) *MODS {
	m := &MODS{TypeOfResource: typeOfResource(r.Leader().RecordType())}
	origin := OriginInfo{}
	for _, f := range r.Fields() {
		switch f.Tag {
		case "245":
			if t := titleFrom(f); t.Title != "" {
				m.TitleInfo = append(m.TitleInfo, t)
			}
		case "130", "240":
			if v := crosswalk.TrimISBD(f.SubfieldValue('a')); v != "" {
				m.TitleInfo = append(m.TitleInfo, TitleInfo{Type: "uniform", Title: v})
			}
		case "100", "700":
			appendName(&m.Name, f, "personal")
		case "110", "710":
			appendName(&m.Name, f, "corporate")
		case "111", "711":
			appendName(&m.Name, f, "conference")
		case "250":
			origin.Edition = crosswalk.TrimISBD(f.SubfieldValue('a'))
		case "260", "264":
			mergeOrigin(&origin, f)
		case "300":
			if m.PhysicalDesc == nil {
				if e := extentFrom(f); e != "" {
					m.PhysicalDesc = &Physical{Extent: e}
				}
			}
		case "500":
			if v := f.SubfieldValue('a'); v != "" {
				m.Note = append(m.Note, Note{Value: v})
			}
		case "520":
			if v := f.SubfieldValue('a'); v != "" {
				m.Note = append(m.Note, Note{Type: "summary", Value: v})
			}
		case "650":
			if s, ok := topicSubject(f); ok {
				m.Subject = append(m.Subject, s)
			}
		case "651":
			if v := crosswalk.TrimISBD(f.SubfieldValue('a')); v != "" {
				m.Subject = append(m.Subject, Subject{Authority: authority(f.Ind2), Geographic: []string{v}})
			}
		case "655":
			if v := crosswalk.TrimISBD(f.SubfieldValue('a')); v != "" {
				m.Subject = append(m.Subject, Subject{Authority: authority(f.Ind2), Genre: []string{v}})
			}
		case "600", "610", "611":
			if n, ok := buildName(f, nameType(f.Tag)); ok {
				n.Role = nil
				m.Subject = append(m.Subject, Subject{Authority: authority(f.Ind2), Name: &n})
			}
		case "020":
			appendIDs(&m.Identifier, f, 'a', "isbn")
		case "022":
			appendIDs(&m.Identifier, f, 'a', "issn")
		case "024":
			appendIDs(&m.Identifier, f, 'a', "other")
		case "856":
			appendIDs(&m.Identifier, f, 'u', "uri")
		}
	}

	if origin.DateIssued == "" {
		origin.DateIssued = date008(r)
	}
	if len(origin.Place) > 0 || origin.Publisher != "" || origin.DateIssued != "" || origin.Edition != "" {
		m.OriginInfo = &origin
	}
	addLanguages(m, r)
	if id := r.ControlField("001"); id != "" {
		m.RecordInfo = &RecordInfo{RecordIdentifier: id}
	}
	return m
}

func titleFrom(f codex.Field) TitleInfo {
	return TitleInfo{
		Title:      crosswalk.TrimISBD(f.SubfieldValue('a')),
		SubTitle:   crosswalk.TrimISBD(f.SubfieldValue('b')),
		PartNumber: crosswalk.TrimISBD(f.SubfieldValue('n')),
		PartName:   crosswalk.TrimISBD(f.SubfieldValue('p')),
	}
}

func extentFrom(f codex.Field) string {
	parts := make([]string, 0, 4)
	for _, code := range []byte{'a', 'b', 'c', 'e'} {
		if v := crosswalk.TrimISBD(f.SubfieldValue(code)); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

func mergeOrigin(o *OriginInfo, f codex.Field) {
	if p := crosswalk.TrimISBD(f.SubfieldValue('a')); p != "" {
		o.Place = append(o.Place, Place{PlaceTerm: PlaceTerm{Type: "text", Value: p}})
	}
	if o.Publisher == "" {
		o.Publisher = crosswalk.TrimISBD(f.SubfieldValue('b'))
	}
	if o.DateIssued == "" {
		o.DateIssued = crosswalk.TrimISBD(f.SubfieldValue('c'))
	}
}

func topicSubject(f codex.Field) (Subject, bool) {
	s := Subject{Authority: authority(f.Ind2)}
	for _, sf := range f.Subfields {
		switch sf.Code {
		case 'a', 'x':
			s.Topic = appendNonEmpty(s.Topic, sf.Value)
		case 'z':
			s.Geographic = appendNonEmpty(s.Geographic, sf.Value)
		case 'y':
			s.Temporal = appendNonEmpty(s.Temporal, sf.Value)
		case 'v':
			s.Genre = appendNonEmpty(s.Genre, sf.Value)
		}
	}
	return s, len(s.Topic)+len(s.Geographic)+len(s.Temporal)+len(s.Genre) > 0
}

func appendIDs(dst *[]Identifier, f codex.Field, code byte, idType string) {
	for _, sf := range f.Subfields {
		if sf.Code == code {
			if v := crosswalk.TrimISBD(sf.Value); v != "" {
				*dst = append(*dst, Identifier{Type: idType, Value: v})
			}
		}
	}
}

func nameType(tag string) string {
	switch tag {
	case "110", "710", "610":
		return "corporate"
	case "111", "711", "611":
		return "conference"
	default:
		return "personal"
	}
}

func appendName(dst *[]Name, f codex.Field, nameType string) {
	if n, ok := buildName(f, nameType); ok {
		*dst = append(*dst, n)
	}
}

func buildName(f codex.Field, nameType string) (Name, bool) {
	a := crosswalk.TrimISBD(f.SubfieldValue('a'))
	if a == "" {
		return Name{}, false
	}
	n := Name{Type: nameType, NamePart: []NamePart{{Value: a}}}
	if d := strings.TrimRight(f.SubfieldValue('d'), ", "); d != "" {
		n.NamePart = append(n.NamePart, NamePart{Type: "date", Value: d})
	}
	role := f.SubfieldValue('e')
	if role == "" {
		role = f.SubfieldValue('4')
	}
	if role = crosswalk.TrimISBD(role); role != "" {
		n.Role = &Role{RoleTerm: RoleTerm{Type: "text", Value: role}}
	}
	return n, true
}

func addLanguages(m *MODS, r *codex.Record) {
	seen := map[string]bool{}
	add := func(code string) {
		if code = strings.TrimSpace(code); len(code) == 3 && !seen[code] {
			seen[code] = true
			m.Language = append(m.Language, Language{LanguageTerm: LanguageTerm{
				Type: "code", Authority: "iso639-2b", Value: code,
			}})
		}
	}
	if c := control008(r); len(c) >= 38 {
		add(c[35:38])
	}
	for _, code := range r.SubfieldValues("041", 'a') {
		// 041$a may pack several 3-letter codes together.
		for i := 0; i+3 <= len(code); i += 3 {
			add(code[i : i+3])
		}
	}
}

// typeOfResource maps leader byte 6 (type of record) to a MODS typeOfResource.
func typeOfResource(recordType byte) string {
	switch recordType {
	case 'a', 't':
		return "text"
	case 'c', 'd':
		return "notated music"
	case 'e', 'f':
		return "cartographic"
	case 'g':
		return "moving image"
	case 'i':
		return "sound recording-nonmusical"
	case 'j':
		return "sound recording-musical"
	case 'k':
		return "still image"
	case 'm':
		return "software, multimedia"
	case 'o', 'p':
		return "mixed material"
	case 'r':
		return "three dimensional object"
	default:
		return "text"
	}
}

// authority maps a 6xx second indicator to a MODS subject authority.
func authority(ind2 byte) string {
	switch ind2 {
	case '0':
		return "lcsh"
	case '1':
		return "lcshac"
	case '2':
		return "mesh"
	case '7':
		return "" // source in $2, not carried here
	default:
		return ""
	}
}

func control008(r *codex.Record) string { return r.ControlField("008") }

func date008(r *codex.Record) string {
	if c := control008(r); len(c) >= 11 {
		d := strings.TrimRight(c[7:11], " |")
		if d != "" && d != "0000" {
			return d
		}
	}
	return ""
}

// trimISBD strips trailing whitespace and a single trailing ISBD separator.
func appendNonEmpty(dst []string, v string) []string {
	if v = strings.TrimRight(v, " "); v != "" {
		return append(dst, v)
	}
	return dst
}

// Encode converts a record to a standalone MODS document.
func Encode(r *codex.Record) ([]byte, error) {
	m := FromRecord(r)
	m.Xmlns = Namespace
	m.Version = "3.7"
	return xml.MarshalIndent(m, "", "  ")
}

// Writer converts records and writes them into a <modsCollection>. Close must be
// called to emit the closing tag.
type Writer struct {
	w      io.Writer
	opened bool
	closed bool
	err    error
}

// compile-time assertion that Writer satisfies the core interface.
var _ codex.RecordWriter = (*Writer)(nil)

// NewWriter returns a Writer that writes a MODS collection to w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Write converts one record and writes its <mods> element within the collection.
func (wr *Writer) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return fmt.Errorf("mods: Write after Close")
	}
	if err := wr.open(); err != nil {
		return err
	}
	b, err := xml.MarshalIndent(FromRecord(r), "  ", "  ")
	if err != nil {
		return err
	}
	return wr.writeAll(append(b, '\n'))
}

// Close writes the closing </modsCollection> tag.
func (wr *Writer) Close() error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return nil
	}
	wr.closed = true
	if err := wr.open(); err != nil {
		return err
	}
	return wr.writeAll([]byte(collectionClose + "\n"))
}

func (wr *Writer) open() error {
	if wr.opened {
		return wr.err
	}
	wr.opened = true
	return wr.writeAll([]byte(xmlHeader + "\n" + collectionOpen + "\n"))
}

func (wr *Writer) writeAll(b []byte) error {
	if wr.err != nil {
		return wr.err
	}
	if _, err := wr.w.Write(b); err != nil {
		wr.err = err
	}
	return wr.err
}

// WriteFile converts every record to a MODS collection file.
func WriteFile(path string, records []*codex.Record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := NewWriter(f)
	for _, rec := range records {
		if err := w.Write(rec); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
