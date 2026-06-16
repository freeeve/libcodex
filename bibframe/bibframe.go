// Package bibframe converts MARC 21 records to BIBFRAME 2.0, the Library of
// Congress linked-data model that replaces the flat MARC record with an RDF
// graph of related resources: a bf:Work (the intellectual content) and a
// bf:Instance (a particular publication of it), linked by bf:instanceOf /
// bf:hasInstance.
//
// BIBFRAME is a different data model than MARC — a graph, not a leader+fields
// record — so this is a one-way MARC->BIBFRAME crosswalk, not a codec, following
// a subset of the LoC marc2bibframe2 mapping. Two serializations are produced,
// both hand-written with the standard library only (no RDF dependency):
//
//   - RDF/XML  (Encode / Writer / WriteFile) — the canonical LoC serialization.
//   - JSON-LD  (EncodeJSONLD / JSONLDWriter / WriteJSONLDFile).
//
// The collection Writers implement codex.RecordWriter, so they plug into
// codex.Convert as conversion targets; both wrap their records in a container
// (an rdf:RDF element or a JSON-LD @graph) and must be closed:
//
//	w := bibframe.NewWriter(out) // RDF/XML; or NewJSONLDWriter for JSON-LD
//	codex.Convert(iso2709.NewReader(src), w)
//	w.Close()
//
// The crosswalk covers the common fields (titles, contributions, subjects,
// language, classification, summary on the Work; title, provision, extent,
// edition, identifiers and electronic locator on the Instance). Fields outside
// that set are not carried, and BIBFRAME cannot round-trip back to full MARC.
package bibframe

import (
	"errors"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/freeeve/libcodex"
)

// errWriteAfterClose is returned by a Writer's Write once Close has run.
var errWriteAfterClose = errors.New("bibframe: Write after Close")

// RDF vocabulary namespaces used by both serializations.
const (
	bfNS      = "http://id.loc.gov/ontologies/bibframe/"
	bflcNS    = "http://id.loc.gov/ontologies/bflc/"
	rdfNS     = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	rdfsNS    = "http://www.w3.org/2000/01/rdf-schema#"
	langVocab = "http://id.loc.gov/vocabulary/languages/"
)

// BIBFRAME is the Work/Instance pair derived from one MARC record.
type BIBFRAME struct {
	Work     Work
	Instance Instance
}

// Work is the intellectual content (bf:Work) plus a specific content class.
type Work struct {
	Class           string // bf class refining bf:Work (e.g. "Text"), or ""
	Titles          []Title
	Contributions   []Contribution
	Subjects        []Subject
	GenreForms      []string
	Languages       []string // ISO 639-2 codes
	Classifications []Classification
	Summary         []string
}

// Instance is a particular publication of the Work (bf:Instance).
type Instance struct {
	Titles                  []Title
	ResponsibilityStatement string
	EditionStatement        string
	Provision               *Provision
	Extent                  []string
	Identifiers             []Identifier
	ElectronicLocator       []string
}

// Title is a bf:Title with its component portions.
type Title struct {
	Type       string // "" for the transcribed title, "uniform" for 130/240
	MainTitle  string
	Subtitle   string
	PartNumber string
	PartName   string
}

// Contribution links an Agent to the Work with an optional role.
type Contribution struct {
	Primary bool   // a bflc:PrimaryContribution (1xx) vs a plain bf:Contribution (7xx)
	Class   string // agent class: "Person", "Organization" or "Meeting"
	Label   string // agent name
	Role    string // role term, or ""
}

// Subject is a topical, geographic or name access point on the Work.
type Subject struct {
	Class string // "Topic", "Place", "Person", "Organization" or "Meeting"
	Label string
}

// Classification is a call number with its BIBFRAME class.
type Classification struct {
	Class string // "ClassificationLcc" or "ClassificationDdc"
	Value string
}

// Identifier is a typed identifier carried by the Instance.
type Identifier struct {
	Class string // "Isbn", "Issn" or "Identifier"
	Value string
}

// Provision is a bf:Publication (place / publisher / date).
type Provision struct {
	Place     string
	Publisher string
	Date      string
}

// FromRecord maps a MARC record to a BIBFRAME Work/Instance pair in a single
// pass over the fields, following the common-field crosswalk.
func FromRecord(r *codex.Record) *BIBFRAME {
	g := &BIBFRAME{}
	g.Work.Class = workClass(r.Leader().RecordType())
	var transcribed, uniform Title
	prov := Provision{}
	for _, f := range r.Fields() {
		switch f.Tag {
		case "245":
			transcribed = Title{
				MainTitle:  trimISBD(f.SubfieldValue('a')),
				Subtitle:   trimISBD(f.SubfieldValue('b')),
				PartNumber: trimISBD(f.SubfieldValue('n')),
				PartName:   trimISBD(f.SubfieldValue('p')),
			}
			g.Instance.ResponsibilityStatement = trimISBD(f.SubfieldValue('c'))
		case "130", "240":
			uniform = Title{Type: "uniform", MainTitle: trimISBD(f.SubfieldValue('a'))}
		case "100", "700":
			g.appendContribution(f, "Person", f.Tag == "100")
		case "110", "710":
			g.appendContribution(f, "Organization", f.Tag == "110")
		case "111", "711":
			g.appendContribution(f, "Meeting", f.Tag == "111")
		case "250":
			g.Instance.EditionStatement = trimISBD(f.SubfieldValue('a'))
		case "260", "264":
			mergeProvision(&prov, f)
		case "300":
			if e := extent(f); e != "" {
				g.Instance.Extent = append(g.Instance.Extent, e)
			}
		case "520":
			if v := strings.TrimRight(f.SubfieldValue('a'), " "); v != "" {
				g.Work.Summary = append(g.Work.Summary, v)
			}
		case "650":
			g.appendSubject(subdivided(f), "Topic")
		case "651":
			g.appendSubject(subdivided(f), "Place")
		case "655":
			if v := trimISBD(f.SubfieldValue('a')); v != "" {
				g.Work.GenreForms = append(g.Work.GenreForms, v)
			}
		case "600":
			g.appendSubject(trimISBD(f.SubfieldValue('a')), "Person")
		case "610":
			g.appendSubject(trimISBD(f.SubfieldValue('a')), "Organization")
		case "611":
			g.appendSubject(trimISBD(f.SubfieldValue('a')), "Meeting")
		case "050":
			g.appendClassification(joinSub(f, "ab", " "), "ClassificationLcc")
		case "082":
			g.appendClassification(trimISBD(f.SubfieldValue('a')), "ClassificationDdc")
		case "020":
			g.appendIdentifiers(f, 'a', "Isbn")
		case "022":
			g.appendIdentifiers(f, 'a', "Issn")
		case "024":
			g.appendIdentifiers(f, 'a', "Identifier")
		case "856":
			for _, sf := range f.Subfields {
				if sf.Code == 'u' {
					if v := strings.TrimRight(sf.Value, " "); v != "" {
						g.Instance.ElectronicLocator = append(g.Instance.ElectronicLocator, v)
					}
				}
			}
		}
	}

	// The Work's preferred title is the uniform title when present, else the
	// transcribed title; the Instance always carries the transcribed title.
	if uniform.MainTitle != "" {
		g.Work.Titles = append(g.Work.Titles, uniform)
	} else if transcribed.MainTitle != "" {
		g.Work.Titles = append(g.Work.Titles, transcribed)
	}
	if transcribed.MainTitle != "" {
		g.Instance.Titles = append(g.Instance.Titles, transcribed)
	}

	if prov.Date == "" {
		prov.Date = date008(r)
	}
	if prov.Place != "" || prov.Publisher != "" || prov.Date != "" {
		g.Instance.Provision = &prov
	}
	g.addLanguages(r)
	return g
}

// ---- crosswalk helpers ----

func (g *BIBFRAME) appendContribution(f codex.Field, class string, primary bool) {
	label := trimISBD(f.SubfieldValue('a'))
	if label == "" {
		return
	}
	role := f.SubfieldValue('e')
	if role == "" {
		role = f.SubfieldValue('4')
	}
	g.Work.Contributions = append(g.Work.Contributions, Contribution{
		Primary: primary, Class: class, Label: label, Role: trimISBD(role),
	})
}

func (g *BIBFRAME) appendSubject(label, class string) {
	if label != "" {
		g.Work.Subjects = append(g.Work.Subjects, Subject{Class: class, Label: label})
	}
}

func (g *BIBFRAME) appendClassification(value, class string) {
	if value != "" {
		g.Work.Classifications = append(g.Work.Classifications, Classification{Class: class, Value: value})
	}
}

func (g *BIBFRAME) appendIdentifiers(f codex.Field, code byte, class string) {
	for _, sf := range f.Subfields {
		if sf.Code == code {
			if v := trimISBD(sf.Value); v != "" {
				g.Instance.Identifiers = append(g.Instance.Identifiers, Identifier{Class: class, Value: v})
			}
		}
	}
}

func (g *BIBFRAME) addLanguages(r *codex.Record) {
	// Languages number a handful at most, so a linear dedup over the accumulator
	// avoids allocating a set.
	add := func(code string) {
		if code = strings.TrimSpace(code); isLangCode(code) && !slices.Contains(g.Work.Languages, code) {
			g.Work.Languages = append(g.Work.Languages, code)
		}
	}
	if c := r.ControlField("008"); len(c) >= 38 {
		add(c[35:38])
	}
	for _, code := range r.SubfieldValues("041", 'a') {
		for i := 0; i+3 <= len(code); i += 3 {
			add(code[i : i+3])
		}
	}
}

// isLangCode reports whether s is a syntactically valid ISO 639-2 language code
// (three lowercase ASCII letters). Codes that fail are dropped rather than emitted
// into a vocabulary URI, so a malformed 008/041 cannot produce an unsafe IRI.
func isLangCode(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := range 3 {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

// subdivided joins a 6xx access point and its subdivisions with "--".
func subdivided(f codex.Field) string {
	var parts []string
	for _, sf := range f.Subfields {
		switch sf.Code {
		case 'a', 'x', 'y', 'z', 'v':
			if v := strings.TrimRight(sf.Value, " "); v != "" {
				parts = append(parts, v)
			}
		}
	}
	return strings.Join(parts, "--")
}

func mergeProvision(p *Provision, f codex.Field) {
	if p.Place == "" {
		p.Place = trimISBD(f.SubfieldValue('a'))
	}
	if p.Publisher == "" {
		p.Publisher = trimISBD(f.SubfieldValue('b'))
	}
	if p.Date == "" {
		p.Date = cleanDate(f.SubfieldValue('c'))
	}
}

// cleanDate strips the brackets and trailing punctuation MARC transcribes around
// a publication date (e.g. "[1993]." -> "1993"), leaving a tidier bf:date.
func cleanDate(s string) string {
	s = strings.Trim(s, " []()")
	return strings.TrimRight(s, " .,;:")
}

func extent(f codex.Field) string {
	parts := make([]string, 0, 4)
	for _, code := range []byte{'a', 'b', 'c', 'e'} {
		if v := trimISBD(f.SubfieldValue(code)); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

func joinSub(f codex.Field, codes, sep string) string {
	var parts []string
	for _, sf := range f.Subfields {
		if strings.IndexByte(codes, sf.Code) >= 0 {
			if v := trimISBD(sf.Value); v != "" {
				parts = append(parts, v)
			}
		}
	}
	return strings.Join(parts, sep)
}

// workClass maps leader byte 6 (type of record) to a BIBFRAME content class
// refining bf:Work, or "" when no specific class applies.
func workClass(recordType byte) string {
	switch recordType {
	case 'a', 't':
		return "Text"
	case 'c', 'd':
		return "NotatedMusic"
	case 'e', 'f':
		return "Cartography"
	case 'g':
		return "MovingImage"
	case 'i', 'j':
		return "Audio"
	case 'k':
		return "StillImage"
	case 'm':
		return "Multimedia"
	case 'o', 'p':
		return "MixedMaterial"
	case 'r':
		return "Object"
	default:
		return ""
	}
}

func date008(r *codex.Record) string {
	if c := r.ControlField("008"); len(c) >= 11 {
		d := strings.TrimRight(c[7:11], " |")
		if d != "" && d != "0000" {
			return d
		}
	}
	return ""
}

// trimISBD strips trailing whitespace and a single trailing ISBD separator.
func trimISBD(s string) string {
	s = strings.TrimRight(s, " ")
	if n := len(s); n > 0 && strings.IndexByte("/:;,", s[n-1]) >= 0 {
		s = strings.TrimRight(s[:n-1], " ")
	}
	return s
}

// resolveBase returns the local identifier stem for a record's nodes: the
// sanitized 001 control number, or "rN" using the collection index when absent.
func resolveBase(r *codex.Record, idx int) string {
	if id := sanitizeID(r.ControlField("001")); id != "" {
		return id
	}
	return "r" + strconv.Itoa(idx)
}

// sanitizeID keeps the characters valid in an IRI fragment, dropping the rest.
func sanitizeID(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '.' || c == '-' || c == '_':
			b.WriteByte(c)
		}
	}
	return b.String()
}

func workURI(base string) string     { return "#" + base + "Work" }
func instanceURI(base string) string { return "#" + base + "Instance" }

// ---- entry points ----

// Encode converts a record to a standalone BIBFRAME RDF/XML document.
func Encode(r *codex.Record) ([]byte, error) {
	b := make([]byte, 0, 4096)
	b = append(b, xmlHeader...)
	b = append(b, '\n')
	b = append(b, rdfOpen...)
	b = append(b, '\n')
	b = appendGraphXML(b, FromRecord(r), resolveBase(r, 0))
	b = append(b, rdfClose...)
	return append(b, '\n'), nil
}

// EncodeJSONLD converts a record to a standalone BIBFRAME JSON-LD document.
func EncodeJSONLD(r *codex.Record) ([]byte, error) {
	b := make([]byte, 0, 2048)
	b = append(b, jsonldContext...)
	b = append(b, `,"@graph":[`...)
	b = appendGraphJSONLD(b, FromRecord(r), resolveBase(r, 0))
	return append(b, "]}"...), nil
}

// ---- RDF/XML writer ----

// Writer converts records and writes them as an rdf:RDF graph. Close must be
// called to emit the closing tag.
type Writer struct {
	w      io.Writer
	buf    []byte
	idx    int
	opened bool
	closed bool
	err    error
}

var _ codex.RecordWriter = (*Writer)(nil)

// NewWriter returns a Writer that writes a BIBFRAME RDF/XML graph to w.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// Write converts one record and writes its Work and Instance nodes.
func (wr *Writer) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return errWriteAfterClose
	}
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte(xmlHeader + "\n" + rdfOpen + "\n")); err != nil {
			return err
		}
	}
	wr.buf = appendGraphXML(wr.buf[:0], FromRecord(r), resolveBase(r, wr.idx))
	wr.idx++
	return wr.writeAll(wr.buf)
}

// Close writes the closing </rdf:RDF> tag.
func (wr *Writer) Close() error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return nil
	}
	wr.closed = true
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte(xmlHeader + "\n" + rdfOpen + "\n")); err != nil {
			return err
		}
	}
	return wr.writeAll([]byte(rdfClose + "\n"))
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

// ---- JSON-LD writer ----

// JSONLDWriter converts records and writes them into a JSON-LD @graph array.
// Close must be called to terminate the document.
type JSONLDWriter struct {
	w      io.Writer
	buf    []byte
	idx    int
	opened bool
	closed bool
	err    error
}

var _ codex.RecordWriter = (*JSONLDWriter)(nil)

// NewJSONLDWriter returns a Writer that writes a BIBFRAME JSON-LD document to w.
func NewJSONLDWriter(w io.Writer) *JSONLDWriter { return &JSONLDWriter{w: w} }

func (wr *JSONLDWriter) Write(r *codex.Record) error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return errWriteAfterClose
	}
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte(jsonldContext + `,"@graph":[`)); err != nil {
			return err
		}
	}
	wr.buf = wr.buf[:0]
	if wr.idx > 0 {
		wr.buf = append(wr.buf, ',')
	}
	wr.buf = appendGraphJSONLD(wr.buf, FromRecord(r), resolveBase(r, wr.idx))
	wr.idx++
	return wr.writeAll(wr.buf)
}

func (wr *JSONLDWriter) Close() error {
	if wr.err != nil {
		return wr.err
	}
	if wr.closed {
		return nil
	}
	wr.closed = true
	if !wr.opened {
		wr.opened = true
		if err := wr.writeAll([]byte(jsonldContext + `,"@graph":[`)); err != nil {
			return err
		}
	}
	return wr.writeAll([]byte("]}\n"))
}

func (wr *JSONLDWriter) writeAll(b []byte) error {
	if wr.err != nil {
		return wr.err
	}
	if _, err := wr.w.Write(b); err != nil {
		wr.err = err
	}
	return wr.err
}

// ---- file helpers ----

// WriteFile converts every record to a BIBFRAME RDF/XML file.
func WriteFile(path string, records []*codex.Record) error {
	return writeFile(path, records, func(w io.Writer) closableWriter { return NewWriter(w) })
}

// WriteJSONLDFile converts every record to a BIBFRAME JSON-LD file.
func WriteJSONLDFile(path string, records []*codex.Record) error {
	return writeFile(path, records, func(w io.Writer) closableWriter { return NewJSONLDWriter(w) })
}

type closableWriter interface {
	Write(*codex.Record) error
	Close() error
}

func writeFile(path string, records []*codex.Record, newW func(io.Writer) closableWriter) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := newW(f)
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
