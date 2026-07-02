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
	Media                   string // RDA media type (bf:media), e.g. "unmediated", "audio", "computer"
	Carrier                 string // RDA carrier type (bf:carrier), e.g. "volume", "online resource", "audio disc"
	Identifiers             []Identifier
	ElectronicLocator       []string
	Admin                   *AdminMetadata
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
	Class  string // "ClassificationLcc", "ClassificationDdc" or "Classification"
	Value  string
	Source string // classification scheme (bf:source), e.g. "bisacsh"; optional
}

// Identifier is a typed identifier carried by the Instance.
type Identifier struct {
	Class  string // "Isbn", "Issn" or "Identifier"
	Value  string
	Source string // identifier scheme (bf:source), e.g. a provider code; optional
}

// Provision is a bf:Publication (place / publisher / date).
type Provision struct {
	Place     string
	Publisher string
	Date      string
}

// AdminMetadata is administrative provenance about the record's description —
// the BIBFRAME bf:AdminMetadata carrying the record control number, the
// cataloging conventions, the last-change date, and the generation process that
// produced the RDF. It is what the LoC/BIBFRAME ecosystem reads for provenance.
type AdminMetadata struct {
	ControlNumber          string // field 001
	ChangeDate             string // field 005, as an xsd:dateTime string
	DescriptionConventions string // field 040 $e (e.g. "rda")
}

// generatorLabel names this library as the bf:GenerationProcess that produced
// the RDF, recorded in every record's bf:AdminMetadata.
const generatorLabel = "libcodex"

// FromRecord maps a MARC record to a BIBFRAME Work/Instance pair in a single
// pass over the fields, following the common-field crosswalk.
func FromRecord(r *codex.Record) *BIBFRAME {
	g := &BIBFRAME{}
	g.Work.Class = workClass(r.Leader().RecordType())
	fields := r.Fields()
	g.presize(fields)
	var transcribed, uniform Title
	var provFields []codex.Field
	var descConventions string
	for _, f := range fields {
		switch f.Tag {
		case "040":
			descConventions = trimISBD(f.SubfieldValue('e'))
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
			provFields = append(provFields, f)
		case "300":
			if e := extent(f); e != "" {
				g.Instance.Extent = append(g.Instance.Extent, e)
			}
		case "337":
			if g.Instance.Media == "" {
				g.Instance.Media = trimISBD(f.SubfieldValue('a')) // RDA media type
			}
		case "338":
			if g.Instance.Carrier == "" {
				g.Instance.Carrier = trimISBD(f.SubfieldValue('a')) // RDA carrier type
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
			g.appendClassification(joinSub(f, "ab", " "), "ClassificationLcc", "")
		case "082":
			g.appendClassification(trimISBD(f.SubfieldValue('a')), "ClassificationDdc", "")
		case "072":
			// Subject category code (e.g. BISAC): a source-qualified classification.
			g.appendClassification(trimISBD(f.SubfieldValue('a')), "Classification", trimISBD(f.SubfieldValue('2')))
		case "084":
			// Other classification number (e.g. BISAC in MARC Express): repeated $a
			// codes qualified by the $2 scheme, mirroring the 072 source handling.
			source := trimISBD(f.SubfieldValue('2'))
			for _, sf := range f.Subfields {
				if sf.Code == 'a' {
					g.appendClassification(trimISBD(sf.Value), "Classification", source)
				}
			}
		case "020":
			g.appendIdentifiers(f, 'a', "Isbn")
		case "022":
			g.appendIdentifiers(f, 'a', "Issn")
		case "024":
			g.appendIdentifiers(f, 'a', "Identifier")
		case "037":
			// Source of acquisition (e.g. the OverDrive Reserve ID): the $a value,
			// with the supplying scheme ($2) or agency ($b) kept as the identifier
			// source so it is not dropped on the MARC import path.
			source := trimISBD(f.SubfieldValue('2'))
			if source == "" {
				source = trimISBD(f.SubfieldValue('b'))
			}
			g.appendIdentifier("Identifier", trimISBD(f.SubfieldValue('a')), source)
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

	prov := provisionStatement(provFields)
	if prov.Date == "" {
		prov.Date = date008(r)
	}
	if prov.Place != "" || prov.Publisher != "" || prov.Date != "" {
		g.Instance.Provision = &prov
	}
	// Every record carries admin metadata: the generation process marks it as
	// libcodex output, alongside the control number, change date and cataloging
	// conventions the record itself provides.
	g.Instance.Admin = &AdminMetadata{
		ControlNumber:          r.ControlField("001"),
		ChangeDate:             formatMARC005(r.ControlField("005")),
		DescriptionConventions: descConventions,
	}
	g.addLanguages(r)
	return g
}

// formatMARC005 converts a field 005 timestamp (yyyymmddhhmmss.f) to an
// xsd:dateTime string, or "" when the field is absent or malformed.
func formatMARC005(s string) string {
	if len(s) < 14 {
		return ""
	}
	for i := range 14 {
		if s[i] < '0' || s[i] > '9' {
			return ""
		}
	}
	return s[0:4] + "-" + s[4:6] + "-" + s[6:8] + "T" + s[8:10] + ":" + s[10:12] + ":" + s[12:14]
}

// ---- crosswalk helpers ----

// presize pre-allocates the multi-valued Work/Instance slices to the number of
// contributing fields, so the crosswalk pass appends without regrowing them. Only
// categories with two or more fields are sized -- for one field an append to nil
// allocates exactly once anyway, and this way a lone empty (skipped) field never
// forces a wasted allocation. The counting pass is a tag switch, far cheaper than
// the growslice-and-copy it removes.
func (g *BIBFRAME) presize(fields []codex.Field) {
	var contrib, subj, classif, ident int
	for i := range fields {
		switch fields[i].Tag {
		case "100", "110", "111", "700", "710", "711":
			contrib++
		case "650", "651", "600", "610", "611":
			subj++
		case "050", "082", "072", "084":
			classif++
		case "020", "022", "024", "037":
			ident++
		}
	}
	if contrib >= 2 {
		g.Work.Contributions = make([]Contribution, 0, contrib)
	}
	if subj >= 2 {
		g.Work.Subjects = make([]Subject, 0, subj)
	}
	if classif >= 2 {
		g.Work.Classifications = make([]Classification, 0, classif)
	}
	if ident >= 2 {
		g.Instance.Identifiers = make([]Identifier, 0, ident)
	}
}

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

func (g *BIBFRAME) appendClassification(value, class, source string) {
	if value != "" {
		g.Work.Classifications = append(g.Work.Classifications, Classification{Class: class, Value: value, Source: source})
	}
}

func (g *BIBFRAME) appendIdentifiers(f codex.Field, code byte, class string) {
	source := trimISBD(f.SubfieldValue('2')) // $2 names the identifier scheme (bf:source)
	for _, sf := range f.Subfields {
		if sf.Code == code {
			g.appendIdentifier(class, trimISBD(sf.Value), source)
		}
	}
}

// appendIdentifier records one Instance identifier when the value is non-empty.
func (g *BIBFRAME) appendIdentifier(class, value, source string) {
	if value != "" {
		g.Instance.Identifiers = append(g.Instance.Identifiers, Identifier{Class: class, Value: value, Source: source})
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

// provisionStatement picks the single 260/264 field that best describes
// publication and reads its place/publisher/date. A 264 publication statement
// (2nd indicator '1') is preferred over a legacy 260, which is preferred over
// the other 264 roles (production, distribution, manufacture); a 264 copyright
// statement (2nd indicator '4') is never chosen, so its $c copyright date is not
// emitted as a bf:date. Reading every subfield from one field also avoids mixing
// one statement's place with another's date.
func provisionStatement(fields []codex.Field) Provision {
	var best *codex.Field
	bestRank := 0
	for i := range fields {
		if r := publicationRank(fields[i]); r > bestRank {
			bestRank, best = r, &fields[i]
		}
	}
	if best == nil {
		return Provision{}
	}
	return Provision{
		Place:     trimISBD(best.SubfieldValue('a')),
		Publisher: trimISBD(best.SubfieldValue('b')),
		Date:      cleanDate(best.SubfieldValue('c')),
	}
}

// publicationRank scores a 260/264 field as a source for the bf:Publication
// node; the highest-scoring field wins and a zero score is never chosen.
func publicationRank(f codex.Field) int {
	if f.Tag == "260" {
		return 2
	}
	switch f.Ind2 {
	case '1': // publication
		return 3
	case '4': // copyright notice date -- not a publication statement
		return 0
	default: // production, distribution, manufacture, or unspecified
		return 1
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
