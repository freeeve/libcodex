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
	skosNS    = "http://www.w3.org/2004/02/skos/core#"
	langVocab = "http://id.loc.gov/vocabulary/languages/"
	// relatorVocab is the LoC relator-term vocabulary; a three-letter $4 relator
	// code names a term IRI beneath it (e.g. .../relators/aut).
	relatorVocab = "http://id.loc.gov/vocabulary/relators/"
	// countriesVocab is the LoC country vocabulary; an 008/15-17 MARC country code
	// names a place IRI beneath it (e.g. .../countries/nyu).
	countriesVocab = "http://id.loc.gov/vocabulary/countries/"
	// The RDA content/media/carrier vocabularies name a term IRI from the code in
	// 336/337/338 $b (e.g. .../contentTypes/txt, .../mediaTypes/n, .../carriers/nc).
	contentVocab = "http://id.loc.gov/vocabulary/contentTypes/"
	mediaVocab   = "http://id.loc.gov/vocabulary/mediaTypes/"
	carrierVocab = "http://id.loc.gov/vocabulary/carriers/"
	// orgVocab names a cataloging-agency IRI from a MARC organization code (003,
	// 040 $a); conventionsVocab names a description-convention IRI from 040 $e.
	orgVocab         = "http://id.loc.gov/vocabulary/organizations/"
	conventionsVocab = "http://id.loc.gov/vocabulary/descriptionConventions/"
	// mnotetypeNS types the internal bf:Note that carries a MARC field verbatim
	// (LoC marc2bibframe2 records the whole 040 this way, in marcKey form).
	mnotetypeNS = "http://id.loc.gov/vocabulary/mnotetype/"
	// issuanceVocab names a mode-of-issuance IRI from a leader/07 bibliographic level
	// (e.g. .../issuance/mono, .../issuance/serl).
	issuanceVocab = "http://id.loc.gov/vocabulary/issuance/"
	// relationshipVocab names a work-to-work relationship IRI from a 76x-78x linking
	// entry (e.g. .../relationship/continues for a 780 preceding entry).
	relationshipVocab = "http://id.loc.gov/vocabulary/relationship/"
)

// BIBFRAME is the Work/Instance pair derived from one MARC record.
type BIBFRAME struct {
	Work     Work
	Instance Instance
}

// Work is the intellectual content (bf:Work) plus a specific content class.
type Work struct {
	Class           string // bf class refining bf:Work (e.g. "Text"), or ""
	Content         string // RDA content-type code (336 $b or leader/06 fallback) -> bf:content IRI; optional
	Titles          []Title
	VariantTitles   []VariantTitle // 246 variant/parallel titles (non-cover/spine)
	Contributions   []Contribution
	RelatedWorks    []RelatedWork
	Relations       []Relation // 76x-78x linking entries -> bf:relation
	Subjects        []Subject
	GenreForms      []string
	Languages       []string // content languages: ISO 639-2 codes from 008/35-37 and 041 $a
	OriginalLangs   []string // languages of the original (041 $h) -> bf:part "original"
	Classifications []Classification
	Summary         []string
	Notes           []Note   // 5xx notes routed to the Work (e.g. 546 language)
	TableOfContents []string // 505 -> bf:tableOfContents
}

// Note is a typed 5xx note: a bf:noteType token (empty for a general 500 note) and
// the note text.
type Note struct {
	Type  string // bf:noteType token (e.g. "bibliography", "language"); "" for a general note
	Label string // the note text
}

// RelatedWork is a name-title access point (a 1xx/7xx carrying a $t): a related
// bf:Work, reached by bf:relatedTo, that pairs the linking name (its creator) with
// the referenced work's title. This is the flat, label-oriented stand-in for m2b's
// Hub-routed name-title relation.
type RelatedWork struct {
	Primary bool   // from a 1xx name-title main entry vs a 7xx added entry
	Class   string // creator agent class: Person/Family/Organization/Jurisdiction/Meeting
	Name    string // creator name (the subfields before $t)
	Title   Title  // the related work's title ($t)
}

// Relation is a 76x-78x work-to-work linking entry (preceding/succeeding title,
// host item, other physical format). It emits a bf:relation -> bf:Relation node
// carrying a bf:relationship vocabulary IRI and a bf:associatedResource -> bf:Work,
// modeled flat (a blank associated Work labeled with the linked resource's title)
// rather than m2b's IRI-minted Hub target.
type Relation struct {
	Relationship string // bf:relationship code (e.g. "continues", "otherPhysicalFormat")
	Name         string // linked resource creator ($a); optional
	Title        string // linked resource title ($t, or $s); the primary access point
	ISSN         string // linked resource ISSN ($x) -> bf:Issn; optional
	ISBN         string // linked resource ISBN ($z) -> bf:Isbn; optional (776 print/ebook pairing)
}

// Instance is a particular publication of the Work (bf:Instance).
type Instance struct {
	Titles                  []Title
	VariantTitles           []VariantTitle // 246 cover/spine titles
	ResponsibilityStatement string
	EditionStatement        string
	SeriesStatements        []string // 490 $a -> bf:seriesStatement
	// SeriesEnumerations holds each 490's volume designation (490 $v ->
	// bf:seriesEnumeration), positionally aligned with SeriesStatements.
	SeriesEnumerations     []string
	Provisions             []Provision
	CopyrightDate          string // 264 _4 $c -> bf:copyrightDate; optional
	Extent                 []string
	Dimensions             []string                // 300 $c -> bf:dimensions
	Duration               []string                // 306 $a playing times -> bf:duration
	Media                  []RDATerm               // RDA media types (337) -> bf:media
	Carrier                []RDATerm               // RDA carrier types (338) -> bf:carrier
	DigitalCharacteristics []DigitalCharacteristic // 347 -> bf:digitalCharacteristic
	Issuance               string                  // mode of issuance (leader/07) -> bf:issuance IRI; optional
	Notes                  []Note                  // 5xx notes routed to the Instance (e.g. 500 general, 504 bibliography)
	Identifiers            []Identifier
	ElectronicLocator      []ElectronicLocator
	Admin                  *AdminMetadata
}

// ElectronicLocator is one 856 access link: the URL plus the display context real
// vendor records carry alongside it. On the graph it is an rdf:Description node
// (the URL as its IRI) hanging off bf:electronicLocator, with Materials as
// rdfs:label, Note as a literal bf:note, and LinkText as a bf:note node typed
// bf:noteType "link text".
type ElectronicLocator struct {
	URL       string // $u -- the access URL (the node IRI)
	Materials string // $3 -- materials specified, e.g. "Image", "Thumbnail", "Excerpt"; the display label
	Note      string // $z -- public note
	LinkText  string // $y -- link text to display in place of the URL
}

// DigitalCharacteristic is one 347 digital-file characteristic: the bflc class
// refining it (FileType for $a, EncodingFormat for $b) and its label.
type DigitalCharacteristic struct {
	Class string // "FileType" or "EncodingFormat"
	Label string
}

// Title is a bf:Title with its component portions.
type Title struct {
	Type       string // "" for the transcribed title, "uniform" for 130/240
	MainTitle  string
	Subtitle   string
	PartNumber string
	PartName   string
	NonSortNum string // 245 ind2 nonfiling character count (1-9) -> bflc:nonSortNum
}

// VariantTitle is a 246 variant or parallel title access point.
type VariantTitle struct {
	Parallel    bool   // 246 ind2=1 -> bf:ParallelTitle, else bf:VariantTitle
	VariantType string // note-type token from 246 ind2 (cover/spine/...) -> bf:variantType; optional
	MainTitle   string
	Subtitle    string
	PartNumber  string
	PartName    string
}

// Contribution links an Agent to the Work with zero or more roles.
type Contribution struct {
	Primary bool   // a bflc:PrimaryContribution (1xx) vs a plain bf:Contribution (7xx)
	Class   string // agent class: "Person", "Family", "Organization", "Jurisdiction" or "Meeting"
	Label   string // agent name
	Roles   []Role // controlled and/or literal roles, in field order
}

// Role is a single contributor role: a relator IRI (from a $4 relator code or
// URI) with an optional label, or a bare literal term (from $e/$j).
type Role struct {
	IRI  string // relator IRI (LoC relators vocabulary or a verbatim URI); "" for a literal role
	Term string // rdfs:label text -- the role term, or the relator code when only a $4 code is present
}

// Subject is a topical, geographic or name access point on the Work.
type Subject struct {
	Class     string // "Topic", "Place", "Person", "Organization" or "Meeting"
	Label     string
	Source    string // subject thesaurus code (bf:source), e.g. "lcsh", "mesh"; optional
	Authority string // authority IRI ($0); when set the bf:subject node is this IRI, not a blank node; optional
}

// Classification is a call number with its BIBFRAME class.
type Classification struct {
	Class       string // "ClassificationLcc", "ClassificationDdc" or "Classification"
	Value       string // classification portion ($a)
	Label       string // human display text for the coded Value (rdfs:label); display-only, optional
	ItemPortion string // item/cutter portion (bf:itemPortion, $b); optional
	Edition     string // Dewey edition (bf:edition): "full" or "abridged"; optional
	Source      string // classification scheme (bf:source), e.g. "bisacsh"; optional
}

// Identifier is a typed identifier carried by the Instance.
type Identifier struct {
	Class     string // "Isbn", "Issn" or "Identifier"
	Value     string
	Source    string // identifier scheme (bf:source), e.g. a provider code; optional
	Qualifier string // qualifying note (bf:qualifier), e.g. "electronic bk"; optional
	Status    string // bf:status: statusCancInv or statusIncorrect; "" when valid
}

// Identifier status codes (bf:status), following marc2bibframe2's mstatus values.
const (
	statusCancInv   = "cancinv"   // canceled or invalid ($z)
	statusIncorrect = "incorrect" // incorrect ($y, ISSN)
)

// Provision is a provision-activity node (bf:Publication / bf:Production /
// bf:Distribution / bf:Manufacture) carrying the transcribed place / agent / date
// and, on the publication node, the 008 country as a controlled bf:place IRI.
type Provision struct {
	Class     string // "Publication", "Production", "Distribution" or "Manufacture"
	Place     string // transcribed place ($a) -> bflc:simplePlace
	Publisher string // transcribed agent ($b) -> bflc:simpleAgent
	Date      string // date ($c or 008) -> bf:date + bflc:simpleDate
	Country   string // 008/15-17 country code -> controlled bf:place IRI; optional
}

// RDATerm is an RDA content/media/carrier term: a code ($b, driving the vocabulary
// IRI) and/or a transcribed label ($a).
type RDATerm struct {
	Code  string // RDA code ($b) -> vocabulary IRI; "" for a label-only term
	Label string // RDA term ($a) -> rdfs:label
}

// AdminMetadata is administrative provenance about the record's description —
// the BIBFRAME bf:AdminMetadata carrying the record control number, the
// cataloging conventions, the last-change date, and the generation process that
// produced the RDF. It is what the LoC/BIBFRAME ecosystem reads for provenance.
type AdminMetadata struct {
	ControlNumber          string   // field 001
	ControlOrg             string   // field 003 -- the agency that assigned the 001
	ChangeDate             string   // field 005, as an xsd:dateTime string
	OrigAgency             string   // field 040 $a -- the original cataloging agency
	DescriptionLanguage    string   // field 040 $b -- the language of the description
	Transcriber            string   // field 040 $c -- the transcribing agency
	Modifiers              []string // field 040 $d (repeatable) -- the modifying agencies
	DescriptionConventions []string // field 040 $e (e.g. "rda"), one per $e
}

// hasCatalogingSource reports whether the AdminMetadata carries any part of a
// field 040, i.e. whether decoding should regenerate one.
func (am *AdminMetadata) hasCatalogingSource() bool {
	return am.OrigAgency != "" || am.DescriptionLanguage != "" || am.Transcriber != "" ||
		len(am.Modifiers) > 0 || len(am.DescriptionConventions) > 0
}

// generatorLabel names this library as the bf:GenerationProcess that produced
// the RDF, recorded in every record's bf:AdminMetadata.
const generatorLabel = "libcodex"

// FromRecord maps a MARC record to a BIBFRAME Work/Instance pair in a single
// pass over the fields, following the common-field crosswalk.
func FromRecord(r *codex.Record) *BIBFRAME {
	g := &BIBFRAME{}
	g.Work.Class = workClass(r.Leader().RecordType())
	g.Instance.Issuance = issuanceForLevel(r.Leader().BibLevel())
	fields := r.Fields()
	g.presize(fields)
	var transcribed, uniform Title
	var provFields []codex.Field
	var coded006, coded007 []string
	var catSource AdminMetadata
	for _, f := range fields {
		switch f.Tag {
		case "040":
			catSource.OrigAgency = trimISBD(f.SubfieldValue('a'))
			catSource.DescriptionLanguage = trimISBD(f.SubfieldValue('b'))
			catSource.Transcriber = trimISBD(f.SubfieldValue('c'))
			for _, d := range f.SubfieldValues('d') {
				if v := trimISBD(d); v != "" {
					catSource.Modifiers = append(catSource.Modifiers, v)
				}
			}
			for _, e := range f.SubfieldValues('e') {
				if v := trimISBD(e); v != "" {
					catSource.DescriptionConventions = append(catSource.DescriptionConventions, v)
				}
			}
		case "245":
			transcribed = Title{
				MainTitle:  trimISBD(f.SubfieldValue('a')),
				Subtitle:   trimISBD(f.SubfieldValue('b')),
				PartNumber: trimISBD(f.SubfieldValue('n')),
				PartName:   trimISBD(f.SubfieldValue('p')),
				NonSortNum: nonSortNum(f.Ind2),
			}
			g.Instance.ResponsibilityStatement = trimISBD(f.SubfieldValue('c'))
		case "130", "240":
			uniform = Title{
				Type:       "uniform",
				MainTitle:  trimISBD(f.SubfieldValue('a')),
				PartNumber: trimISBD(f.SubfieldValue('n')),
				PartName:   trimISBD(f.SubfieldValue('p')),
			}
		case "246":
			g.addVariantTitle(f)
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
			for _, d := range f.SubfieldValues('c') { // dimensions, not part of the extent
				if v := trimISBD(d); v != "" {
					g.Instance.Dimensions = append(g.Instance.Dimensions, v)
				}
			}
		case "336":
			if c := rdaCode(f); c != "" {
				g.Work.Content = c
			}
		case "337":
			g.Instance.Media = append(g.Instance.Media, rdaTerm(f))
		case "338":
			g.Instance.Carrier = append(g.Instance.Carrier, rdaTerm(f))
		case "773", "776", "780", "785":
			g.appendRelation(f)
		case "500", "504", "511", "521", "533", "538", "546":
			// Notes about the content (546 language, 511 performers, 521
			// audience) describe the Work; the rest (500 general, 504
			// bibliography, 533 reproduction, 538 system details) the Instance.
			if v := noteLabel(f); v != "" {
				n := Note{Type: noteTypeForTag(f.Tag), Label: v}
				switch f.Tag {
				case "511", "521", "546":
					g.Work.Notes = append(g.Work.Notes, n)
				default:
					g.Instance.Notes = append(g.Instance.Notes, n)
				}
			}
		case "490":
			// The enumeration is recorded only alongside a statement, so the two
			// slices stay positionally aligned and the reverse crosswalk can pair them.
			if stmt := seriesStatement(f); stmt != "" {
				g.Instance.SeriesStatements = append(g.Instance.SeriesStatements, stmt)
				g.Instance.SeriesEnumerations = append(g.Instance.SeriesEnumerations, trimISBD(f.SubfieldValue('v')))
			}
		case "306":
			for _, d := range f.SubfieldValues('a') {
				if v := strings.TrimSpace(d); v != "" {
					g.Instance.Duration = append(g.Instance.Duration, v)
				}
			}
		case "347":
			g.appendDigitalCharacteristics(f)
		case "006":
			coded006 = append(coded006, f.Value)
		case "007":
			coded007 = append(coded007, f.Value)
		case "505":
			if v := strings.TrimRight(f.SubfieldValue('a'), " "); v != "" {
				g.Work.TableOfContents = append(g.Work.TableOfContents, v)
			}
		case "520":
			if v := strings.TrimRight(f.SubfieldValue('a'), " "); v != "" {
				g.Work.Summary = append(g.Work.Summary, v)
			}
		case "650":
			g.appendSubject(subdivided(f), "Topic", subjectSource(f), subjectAuthority(f))
		case "651":
			g.appendSubject(subdivided(f), "Place", subjectSource(f), subjectAuthority(f))
		case "655":
			// A subdivided 655 is a topical subject in m2b; a plain genre term
			// stays a bf:genreForm (which our model carries as a bare label).
			if hasSubdivision(f) {
				g.appendSubject(subdivided(f), "Topic", subjectSource(f), subjectAuthority(f))
			} else if v := trimISBD(f.SubfieldValue('a')); v != "" {
				g.Work.GenreForms = append(g.Work.GenreForms, v)
			}
		case "600":
			g.appendSubject(trimISBD(f.SubfieldValue('a')), "Person", subjectSource(f), subjectAuthority(f))
		case "610":
			g.appendSubject(trimISBD(f.SubfieldValue('a')), "Organization", subjectSource(f), subjectAuthority(f))
		case "611":
			g.appendSubject(trimISBD(f.SubfieldValue('a')), "Meeting", subjectSource(f), subjectAuthority(f))
		case "050":
			g.appendCallNumber(f, "ClassificationLcc", "", "")
		case "082":
			g.appendCallNumber(f, "ClassificationDdc", trimISBD(f.SubfieldValue('2')), deweyEdition(f.Ind1))
		case "072":
			// Subject category code (e.g. BISAC): a source-qualified classification.
			g.appendClassification(trimISBD(f.SubfieldValue('a')), "Classification", trimISBD(f.SubfieldValue('2')))
		case "084":
			// Other classification number (e.g. BISAC in MARC Express): repeated $a
			// codes qualified by the $2 scheme, with $b as the item portion.
			source, item := trimISBD(f.SubfieldValue('2')), trimISBD(f.SubfieldValue('b'))
			for _, a := range f.SubfieldValues('a') {
				if v := trimISBD(a); v != "" {
					g.Work.Classifications = append(g.Work.Classifications, Classification{
						Class: "Classification", Value: v, ItemPortion: item, Source: source,
					})
				}
			}
		case "010":
			// Library of Congress Control Number: $a valid, $z canceled/invalid.
			// LCCNs carry positional leading spaces, so trim all surrounding space.
			g.appendIdentifier("Lccn", strings.TrimSpace(f.SubfieldValue('a')), "", "", "")
			for _, sf := range f.Subfields {
				if sf.Code == 'z' {
					g.appendIdentifier("Lccn", strings.TrimSpace(sf.Value), "", "", statusCancInv)
				}
			}
		case "020":
			g.appendIdentifiers(f, 'a', "Isbn", trimISBD(f.SubfieldValue('2')), true)
		case "022":
			g.appendIdentifiers(f, 'a', "Issn", trimISBD(f.SubfieldValue('2')), false)
		case "024":
			class, source := identifier024(f)
			g.appendIdentifiers(f, 'a', class, source, false)
		case "037":
			// Source of acquisition (e.g. the OverDrive Reserve ID): the $a value,
			// with the supplying scheme ($2) or agency ($b) kept as the identifier
			// source so it is not dropped on the MARC import path.
			source := trimISBD(f.SubfieldValue('2'))
			if source == "" {
				source = trimISBD(f.SubfieldValue('b'))
			}
			g.appendIdentifier("Identifier", trimISBD(f.SubfieldValue('a')), source, "", "")
		case "856":
			g.appendLocator(f)
		}
	}

	g.applyCodedFields(coded006, coded007)

	// Every Work carries an RDA content type: 336 when present, else a fallback
	// derived from leader/06, so the content signal is never absent (mirroring m2b).
	if g.Work.Content == "" {
		g.Work.Content = content06(r.Leader().RecordType())
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

	g.addProvisions(r, provFields)
	// Every record carries admin metadata: the generation process marks it as
	// libcodex output, alongside the control number, change date and cataloging
	// conventions the record itself provides.
	admin := catSource
	admin.ControlNumber = r.ControlField("001")
	admin.ControlOrg = strings.TrimSpace(r.ControlField("003"))
	admin.ChangeDate = formatMARC005(r.ControlField("005"))
	g.Instance.Admin = &admin
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
		case "010", "020", "022", "024", "037":
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

// appendContribution builds a Contribution from a 1xx/7xx field: the agent label
// concatenates the tag-appropriate name subfields, the class is refined from ind1
// (Family, Jurisdiction), and roles come from $4 (relator code/URI, controlled)
// ahead of the literal role subfield ($e for names, $j for meetings).
func (g *BIBFRAME) appendContribution(f codex.Field, class string, primary bool) {
	labelCodes, roleSub := "abcdqjk", byte('e')
	switch class {
	case "Organization":
		labelCodes = "abcdngk"
	case "Meeting":
		labelCodes, roleSub = "acdengq", 'j'
	}
	// A $t makes this a name-title access point pointing at a related work, not a
	// contributor to this work.
	if f.SubfieldValue('t') != "" {
		g.appendRelatedWork(f, class, labelCodes, primary)
		return
	}
	label := agentLabel(f, labelCodes)
	if label == "" {
		return
	}
	g.Work.Contributions = append(g.Work.Contributions, Contribution{
		Primary: primary,
		Class:   agentSubclass(class, f.Ind1),
		Label:   label,
		Roles:   contribRoles(f, roleSub),
	})
}

// appendRelatedWork records a name-title field ($t present) as a related bf:Work:
// the subfields before $t form the creator name, $t the related work's title.
func (g *BIBFRAME) appendRelatedWork(f codex.Field, class, labelCodes string, primary bool) {
	name := agentLabel(f, labelCodes)
	title := trimISBD(f.SubfieldValue('t'))
	if name == "" && title == "" {
		return
	}
	g.Work.RelatedWorks = append(g.Work.RelatedWorks, RelatedWork{
		Primary: primary,
		Class:   agentSubclass(class, f.Ind1),
		Name:    name,
		Title:   Title{MainTitle: title},
	})
}

// appendRelation records a 76x-78x linking entry as a Relation: the relationship
// code from the tag (and, for 780/785, the second indicator), the linked resource's
// title ($t, or $s), creator ($a) and ISSN ($x). A field with no relationship code
// or no access-point content is skipped.
func (g *BIBFRAME) appendRelation(f codex.Field) {
	code, ok := relationCodeFor(f.Tag, f.Ind2)
	if !ok {
		return
	}
	title := trimISBD(f.SubfieldValue('t'))
	if title == "" {
		title = trimISBD(f.SubfieldValue('s'))
	}
	name := trimISBD(f.SubfieldValue('a'))
	issn := trimISBD(f.SubfieldValue('x'))
	isbn := trimISBD(f.SubfieldValue('z'))
	if title == "" && name == "" && issn == "" && isbn == "" {
		return
	}
	g.Work.Relations = append(g.Work.Relations, Relation{
		Relationship: code,
		Name:         name,
		Title:        title,
		ISSN:         issn,
		ISBN:         isbn,
	})
}

// carrier007 pairs an RDA carrier code (338 $b) with the 007/00-01 category +
// specific-material-designation bytes it corresponds to, for the sound, computer
// and video categories. One table drives both directions: the forward pass folds
// a 007 into a carrier term, the reverse pass rebuilds a minimal 2-byte 007 from
// the carrier -- the same derive-don't-fabricate shape as the partial 008
// reconstruction. The audio and video codes align byte-for-byte (RDA carriers
// were designed against 007); computer discs differ (carrier "cd" is 007 "co",
// optical disc).
var carrier007 = []struct{ carrier, coded string }{
	{"sd", "sd"}, {"si", "si"}, {"sq", "sq"}, {"ss", "ss"}, {"st", "st"}, {"sz", "sz"},
	{"cr", "cr"}, {"cd", "co"}, {"ca", "ca"}, {"cb", "cb"}, {"ce", "ce"},
	{"cf", "cf"}, {"ch", "ch"}, {"ck", "ck"}, {"cz", "cz"},
	{"vd", "vd"}, {"vf", "vf"}, {"vr", "vr"}, {"vc", "vc"}, {"vz", "vz"},
}

// carrierFor007 returns the RDA carrier code for a 007's leading bytes.
func carrierFor007(coded string) (string, bool) {
	for _, m := range carrier007 {
		if m.coded == coded {
			return m.carrier, true
		}
	}
	return "", false
}

// f007ForCarrier inverts carrierFor007.
func f007ForCarrier(carrier string) (string, bool) {
	for _, m := range carrier007 {
		if m.carrier == carrier {
			return m.coded, true
		}
	}
	return "", false
}

// applyCodedFields folds 006/007 coded values into the RDA media/carrier terms
// after the field pass, so explicit 337/338 fields win and the coded fields only
// add what is missing. A 007 whose category+SMD maps to a carrier adds that
// carrier; a 006 leading byte 'm' (computer-file aspect, the electronic-resource
// shape) adds the computer media type.
func (g *BIBFRAME) applyCodedFields(coded006, coded007 []string) {
	haveCarrier := map[string]bool{}
	for _, c := range g.Instance.Carrier {
		haveCarrier[c.Code] = true
	}
	for _, v := range coded007 {
		if len(v) < 2 {
			continue
		}
		if code, ok := carrierFor007(v[:2]); ok && !haveCarrier[code] {
			haveCarrier[code] = true
			g.Instance.Carrier = append(g.Instance.Carrier, RDATerm{Code: code})
		}
	}
	for _, v := range coded006 {
		if len(v) == 0 || v[0] != 'm' {
			continue // only the computer-file aspect is modeled; see tasks/082
		}
		haveMedia := false
		for _, m := range g.Instance.Media {
			if m.Code == "c" {
				haveMedia = true
			}
		}
		if !haveMedia {
			g.Instance.Media = append(g.Instance.Media, RDATerm{Code: "c"})
		}
	}
}

// linkRelation pairs a 76x-78x linking tag (and, for the continuation entries, its
// second indicator) with its bf:relationship vocabulary code. It is the single
// source of truth for both crosswalk directions: the forward pass reads a code from
// (tag, ind2), the reverse pass recovers (tag, ind2) from a code.
type linkRelation struct {
	tag  string
	ind2 byte
	code string
}

// linkRelations maps the supported 76x-78x linking entries to relationship codes.
// 780 (preceding) and 785 (succeeding) refine the code by their second indicator;
// 773 (host item) and 776 (other physical format) map by tag alone (ind2 ' ').
var linkRelations = []linkRelation{
	{"780", '0', "continues"},
	{"780", '1', "continuesInPart"},
	{"780", '2', "supersedes"},
	{"780", '3', "supersedesInPart"},
	{"780", '4', "formedByUnionOf"},
	{"780", '5', "absorbed"},
	{"780", '6', "absorbedInPart"},
	{"780", '7', "separatedFrom"},
	{"785", '0', "continuedBy"},
	{"785", '1', "continuedInPartBy"},
	{"785", '2', "supersededBy"},
	{"785", '3', "supersededInPartBy"},
	{"785", '4', "absorbedBy"},
	{"785", '5', "absorbedInPartBy"},
	{"785", '6', "splitInto"},
	{"785", '7', "mergedToForm"},
	{"785", '8', "changedBackTo"},
	{"773", ' ', "partOf"},
	{"776", ' ', "otherPhysicalFormat"},
}

// relationCodeFor returns the bf:relationship code for a linking field and whether
// the tag is supported. 773/776 map by tag; 780/785 map by second indicator,
// falling back to the tag's base (ind2 '0') code for an unrecognized indicator.
func relationCodeFor(tag string, ind2 byte) (string, bool) {
	switch tag {
	case "773", "776":
		for _, lr := range linkRelations {
			if lr.tag == tag {
				return lr.code, true
			}
		}
	case "780", "785":
		for _, lr := range linkRelations {
			if lr.tag == tag && lr.ind2 == ind2 {
				return lr.code, true
			}
		}
		for _, lr := range linkRelations {
			if lr.tag == tag && lr.ind2 == '0' {
				return lr.code, true
			}
		}
	}
	return "", false
}

// relationField inverts relationCodeFor, recovering the 76x-78x tag and second
// indicator that produced a relationship code.
func relationField(code string) (string, byte, bool) {
	for _, lr := range linkRelations {
		if lr.code == code {
			return lr.tag, lr.ind2, true
		}
	}
	return "", 0, false
}

// agentLabel joins, in field order, the values of the name subfields named in codes
// (one space between them) up to the first $t (which starts the title portion of a
// name-title field), then trims a trailing ISBD mark.
func agentLabel(f codex.Field, codes string) string {
	var parts []string
	for _, s := range f.Subfields {
		if s.Code == 't' {
			break
		}
		if strings.IndexByte(codes, s.Code) >= 0 {
			if v := strings.TrimSpace(s.Value); v != "" {
				parts = append(parts, v)
			}
		}
	}
	return trimISBD(strings.Join(parts, " "))
}

// agentSubclass refines a tag-derived agent class using ind1: an x00 with ind1=3
// is a Family, an x10 with ind1=1 is a Jurisdiction. Other indicators keep the
// tag-derived class.
func agentSubclass(class string, ind1 byte) string {
	switch {
	case class == "Person" && ind1 == '3':
		return "Family"
	case class == "Organization" && ind1 == '1':
		return "Jurisdiction"
	}
	return class
}

// contribRoles collects a field's roles: every $4 (a relator code -> relators IRI,
// a URI verbatim, else a literal) followed by every literal role subfield, each
// split on the ", and &" compound-role delimiters.
func contribRoles(f codex.Field, roleSub byte) []Role {
	var roles []Role
	for _, v := range f.SubfieldValues('4') {
		if v = strings.TrimSpace(v); v != "" {
			roles = append(roles, relatorRole(v))
		}
	}
	for _, v := range f.SubfieldValues(roleSub) {
		for _, term := range splitRoleTerms(trimISBD(v)) {
			roles = append(roles, Role{Term: term})
		}
	}
	return roles
}

// relatorRole classifies a $4 value: a three-letter lowercase relator code becomes
// a relators-vocabulary IRI labeled with the code; an XML-safe absolute URI is used
// verbatim; anything else becomes a literal role term.
func relatorRole(v string) Role {
	switch {
	case isRelatorCode(v):
		return Role{IRI: relatorVocab + v, Term: v}
	case isSafeIRI(v):
		return Role{IRI: v}
	default:
		return Role{Term: v}
	}
}

// isRelatorCode reports whether v is a three-letter lowercase MARC relator code.
func isRelatorCode(v string) bool {
	if len(v) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if v[i] < 'a' || v[i] > 'z' {
			return false
		}
	}
	return true
}

// isSafeIRI reports whether v is an absolute URI carrying no characters that would
// break the unescaped node-IRI paths in the serializers (so it can be emitted as a
// role IRI without further sanitizing).
func isSafeIRI(v string) bool {
	if !strings.Contains(v, "://") {
		return false
	}
	for i := 0; i < len(v); i++ {
		switch c := v[i]; {
		case c <= ' ', c == '<', c == '>', c == '"', c == '&', c == '\'', c == 0x7f:
			return false
		}
	}
	return true
}

// splitRoleTerms splits one literal role string into its component terms on the
// ", and &" delimiters, trimming each and dropping empties.
func splitRoleTerms(s string) []string {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	s = strings.NewReplacer("&", ",", " and ", ",").Replace(s)
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (g *BIBFRAME) appendSubject(label, class, source, authority string) {
	if label != "" {
		g.Work.Subjects = append(g.Work.Subjects, Subject{Class: class, Label: label, Source: source, Authority: authority})
	}
}

// subjectAuthority returns the authority IRI a 6xx carries in $0, or "" when
// absent or not IRI-shaped. A $0 that is a record control number (e.g.
// "(DLC)sh85..." rather than a URI) is left out, since bf:subject mints an IRI
// object only from an actual authority link.
func subjectAuthority(f codex.Field) string {
	for _, v := range f.SubfieldValues('0') {
		v = strings.TrimRight(v, " ")
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return v
		}
	}
	return ""
}

// subjectThesaurusByInd2 maps a 6xx second indicator to the conventional $2
// thesaurus code that names the controlled vocabulary, following marc2bibframe2's
// subjectThesaurus mapping. Indicator '4' (source unspecified) and '7' (source in
// $2) are handled by subjectSource reading $2 directly.
var subjectThesaurusByInd2 = map[byte]string{
	'0': "lcsh",   // Library of Congress Subject Headings
	'1': "lcshac", // LCSH for children's literature
	'2': "mesh",   // Medical Subject Headings
	'3': "nal",    // National Agricultural Library
	'5': "cash",   // Canadian Subject Headings
	'6': "rvm",    // Répertoire de vedettes-matière
}

// subjectSource returns the thesaurus code (bf:source) for a 6xx field: a
// conventional code from the second indicator, or the $2 value when the indicator
// defers to it (ind2 '4' or '7').
func subjectSource(f codex.Field) string {
	if code, ok := subjectThesaurusByInd2[f.Ind2]; ok {
		return code
	}
	return trimISBD(f.SubfieldValue('2'))
}

// hasSubdivision reports whether a 6xx field carries a $x/$y/$z/$v subdivision,
// which turns a 655 genre heading into a topical subject (matching m2b, where a
// subdivided 655 is a bf:subject rather than a bf:genreForm).
func hasSubdivision(f codex.Field) bool {
	for _, sf := range f.Subfields {
		switch sf.Code {
		case 'x', 'y', 'z', 'v':
			if strings.TrimRight(sf.Value, " ") != "" {
				return true
			}
		}
	}
	return false
}

// appendLocator records an 856 access link: one ElectronicLocator per $u, each
// carrying the field's $3 materials, $z public note and $y link text.
func (g *BIBFRAME) appendLocator(f codex.Field) {
	materials := strings.TrimRight(f.SubfieldValue('3'), " ")
	note := strings.TrimRight(f.SubfieldValue('z'), " ")
	linkText := strings.TrimRight(f.SubfieldValue('y'), " ")
	for _, u := range f.SubfieldValues('u') {
		if u = strings.TrimRight(u, " "); u != "" {
			g.Instance.ElectronicLocator = append(g.Instance.ElectronicLocator, ElectronicLocator{
				URL:       u,
				Materials: materials,
				Note:      note,
				LinkText:  linkText,
			})
		}
	}
}

func (g *BIBFRAME) appendClassification(value, class, source string) {
	if value != "" {
		g.Work.Classifications = append(g.Work.Classifications, Classification{Class: class, Value: value, Source: source})
	}
}

// appendCallNumber records a call-number classification from a 050/082 field: $a is
// the classification portion, $b the item (cutter) portion, plus an optional scheme
// source and Dewey edition.
func (g *BIBFRAME) appendCallNumber(f codex.Field, class, source, edition string) {
	value := trimISBD(f.SubfieldValue('a'))
	if value == "" {
		return
	}
	g.Work.Classifications = append(g.Work.Classifications, Classification{
		Class:       class,
		Value:       value,
		ItemPortion: trimISBD(f.SubfieldValue('b')),
		Source:      source,
		Edition:     edition,
	})
}

// deweyEdition maps an 082 first indicator (type of edition) to the bf:edition
// value: 0 -> full, 1 -> abridged, else none.
func deweyEdition(ind1 byte) string {
	switch ind1 {
	case '0':
		return "full"
	case '1':
		return "abridged"
	}
	return ""
}

// scheme024ByInd1 maps a 024 first indicator (0-4) to its bf identifier class,
// following marc2bibframe2. Indicator '7' resolves the class from $2 via
// scheme024ByCode; other indicators fall back to the generic bf:Identifier.
var scheme024ByInd1 = map[byte]string{'0': "Isrc", '1': "Upc", '2': "Ismn", '3': "Ean", '4': "Sici"}

// scheme024ByCode maps a 024 $2 scheme code (used when ind1='7') to its bf
// identifier class, following marc2bibframe2's 024 handling.
var scheme024ByCode = map[string]string{
	"ansi": "Ansi", "doi": "Doi", "gtin-14": "Gtin14Number", "hdl": "Hdl",
	"isan": "Isan", "isni": "Isni", "iso": "Iso", "istc": "Istc", "iswc": "Iswc",
	"matrix-number": "MatrixNumber", "music-plate": "MusicPlate",
	"music-publisher": "MusicPublisherNumber", "stock-number": "StockNumber",
	"urn": "Urn", "videorecording-identifier": "VideoRecordingNumber",
}

// identifier024 resolves a 024 field to its bf identifier class and bf:source: the
// class comes from ind1 (0-4) or, for ind1='7', from the $2 scheme code; an
// unrecognized ind1='7' scheme is kept as a generic identifier with $2 as source.
func identifier024(f codex.Field) (class, source string) {
	if c, ok := scheme024ByInd1[f.Ind1]; ok {
		return c, ""
	}
	if f.Ind1 == '7' {
		s := trimISBD(f.SubfieldValue('2'))
		if c, ok := scheme024ByCode[s]; ok {
			return c, ""
		}
		return "Identifier", s
	}
	return "Identifier", "" // ind1 '8' or blank: unspecified standard number
}

// appendIdentifiers records one Instance identifier per matching subfield, with the
// given class and bf:source. When qualified is set (ISBN-style fields), a trailing
// parenthetical on the value and any $q subfield are split into the identifier's
// bf:qualifier; a canceled ($z) or incorrect ISSN ($y) number is kept flagged
// bf:status -- all matching marc2bibframe2.
func (g *BIBFRAME) appendIdentifiers(f codex.Field, code byte, class, source string, qualified bool) {
	qualifier := ""
	if qualified {
		qualifier = trimISBD(f.SubfieldValue('q'))
	}
	for _, sf := range f.Subfields {
		switch sf.Code {
		case code:
			value := trimISBD(sf.Value)
			q := qualifier
			if qualified {
				var paren string
				value, paren = splitParenthetical(value)
				if q == "" {
					q = paren
				}
			}
			g.appendIdentifier(class, value, source, q, "")
		case 'z':
			// A canceled/invalid number ($z) is kept as an identifier flagged
			// bf:status, not dropped, matching marc2bibframe2.
			g.appendIdentifier(class, trimISBD(sf.Value), source, "", statusCancInv)
		case 'y':
			if class == "Issn" { // 022 $y is an incorrect ISSN
				g.appendIdentifier(class, trimISBD(sf.Value), source, "", statusIncorrect)
			}
		}
	}
}

// appendIdentifier records one Instance identifier when the value is non-empty.
func (g *BIBFRAME) appendIdentifier(class, value, source, qualifier, status string) {
	if value != "" {
		g.Instance.Identifiers = append(g.Instance.Identifiers, Identifier{
			Class: class, Value: value, Source: source, Qualifier: qualifier, Status: status,
		})
	}
}

// splitParenthetical separates a trailing "(qualifier)" note from an identifier
// value, returning the bare value and the qualifier text (empty when absent). It
// mirrors marc2bibframe2, which lifts the parenthetical out of an 020 $a into a
// bf:qualifier: "0781234567 (v.1)" -> "0781234567", "v.1".
func splitParenthetical(s string) (value, qualifier string) {
	open := strings.IndexByte(s, '(')
	if open < 0 {
		return s, ""
	}
	end := strings.IndexByte(s[open:], ')')
	if end < 0 {
		return s, ""
	}
	end += open
	qualifier = strings.TrimSpace(s[open+1 : end])
	value = strings.TrimSpace(s[:open] + s[end+1:])
	return value, qualifier
}

func (g *BIBFRAME) addLanguages(r *codex.Record) {
	// Languages number a handful at most, so a linear dedup over each accumulator
	// avoids allocating a set.
	addTo := func(dst *[]string, code string) {
		if code = strings.TrimSpace(code); isLangCode(code) && !slices.Contains(*dst, code) {
			*dst = append(*dst, code)
		}
	}
	if c := r.ControlField("008"); len(c) >= 38 {
		addTo(&g.Work.Languages, c[35:38])
	}
	for _, code := range r.SubfieldValues("041", 'a') {
		for i := 0; i+3 <= len(code); i += 3 {
			addTo(&g.Work.Languages, code[i:i+3])
		}
	}
	for _, code := range r.SubfieldValues("041", 'h') { // language of the original
		for i := 0; i+3 <= len(code); i += 3 {
			addTo(&g.Work.OriginalLangs, code[i:i+3])
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

// addProvisions builds one provision-activity node per 260/264, typed by the 264
// second indicator (0 Production, 1 Publication, 2 Distribution, 3 Manufacture; a
// 260 or blank indicator is a Publication). A 264 _4 copyright statement is not a
// provision -- its $c becomes the Instance copyright date. The 008/15-17 country
// (and, absent a 26X date, the 008 date) attaches to a Publication node, minted
// when the record has no usable 26X.
func (g *BIBFRAME) addProvisions(r *codex.Record, fields []codex.Field) {
	for i := range fields {
		f := fields[i]
		if f.Tag == "264" && f.Ind2 == '4' {
			if d := cleanDate(f.SubfieldValue('c')); d != "" && g.Instance.CopyrightDate == "" {
				g.Instance.CopyrightDate = d
			}
			continue
		}
		p := Provision{
			Class:     provisionClass(f),
			Place:     trimISBD(f.SubfieldValue('a')),
			Publisher: trimISBD(f.SubfieldValue('b')),
			Date:      cleanDate(f.SubfieldValue('c')),
		}
		if p.Place != "" || p.Publisher != "" || p.Date != "" {
			g.Instance.Provisions = append(g.Instance.Provisions, p)
		}
	}
	country, fallbackDate := country008(r), date008(r)
	pub := g.publicationProvision()
	if pub == nil && (country != "" || (len(g.Instance.Provisions) == 0 && fallbackDate != "")) {
		g.Instance.Provisions = append(g.Instance.Provisions, Provision{Class: "Publication"})
		pub = &g.Instance.Provisions[len(g.Instance.Provisions)-1]
	}
	if pub != nil {
		if country != "" {
			pub.Country = country
		}
		if pub.Date == "" {
			pub.Date = fallbackDate
		}
	}
}

// publicationProvision returns a pointer to the first Publication provision, or nil.
func (g *BIBFRAME) publicationProvision() *Provision {
	for i := range g.Instance.Provisions {
		if g.Instance.Provisions[i].Class == "Publication" {
			return &g.Instance.Provisions[i]
		}
	}
	return nil
}

// provisionClass maps a 26X field to its provision-activity subclass.
func provisionClass(f codex.Field) string {
	if f.Tag == "264" {
		switch f.Ind2 {
		case '0':
			return "Production"
		case '2':
			return "Distribution"
		case '3':
			return "Manufacture"
		}
	}
	return "Publication" // 260, 264 _1, or unspecified
}

// country008 reads the 008/15-17 MARC country code, or "" when absent/invalid.
func country008(r *codex.Record) string {
	if c := r.ControlField("008"); len(c) >= 18 {
		if code := strings.TrimSpace(c[15:18]); isCountryCode(code) {
			return code
		}
	}
	return ""
}

// isCountryCode reports whether s is a syntactically valid MARC country code (two
// or three lowercase ASCII letters), so a malformed 008 cannot mint an unsafe IRI.
func isCountryCode(s string) bool {
	if len(s) < 2 || len(s) > 3 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

// cleanDate strips the brackets and trailing punctuation MARC transcribes around
// a publication date (e.g. "[1993]." -> "1993"), leaving a tidier bf:date.
func cleanDate(s string) string {
	s = strings.Trim(s, " []()")
	return strings.TrimRight(s, " .,;:")
}

// extent renders a 300 field's extent label from $a plus the $b other-physical and
// $f/$g size subfields; $c (dimensions) and $e (accompanying material) are routed
// elsewhere so they no longer inflate the extent.
func extent(f codex.Field) string {
	parts := make([]string, 0, 4)
	for _, code := range []byte{'a', 'b', 'f', 'g'} {
		if v := trimISBD(f.SubfieldValue(code)); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

// rdaCode returns the RDA code of a 33X field: $b (the controlled code) when
// present, else "".
func rdaCode(f codex.Field) string {
	return trimISBD(f.SubfieldValue('b'))
}

// rdaTerm reads a 337/338 field into an RDATerm: $b as the code, $a as the label.
func rdaTerm(f codex.Field) RDATerm {
	return RDATerm{Code: rdaCode(f), Label: trimISBD(f.SubfieldValue('a'))}
}

// content06 maps leader/06 (type of record) to the fallback RDA content-type code
// used when the record has no 336, or "" when no single content type applies.
func content06(recordType byte) string {
	switch recordType {
	case 'a', 't':
		return "txt" // text
	case 'c', 'd':
		return "ntm" // notated music
	case 'e', 'f':
		return "cri" // cartographic image
	case 'g':
		return "tdi" // two-dimensional moving image
	case 'i':
		return "spw" // spoken word
	case 'j':
		return "prm" // performed music
	case 'k':
		return "sti" // still image
	case 'm':
		return "cop" // computer program
	case 'r':
		return "tdf" // three-dimensional form
	default:
		return ""
	}
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
	case 'i':
		return "NonMusicAudio"
	case 'j':
		return "MusicAudio"
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

// nonSortNum returns the 245 nonfiling character count for a second indicator in
// 1-9 (the count of leading characters to ignore in sorting), or "" otherwise.
func nonSortNum(ind2 byte) string {
	if ind2 >= '1' && ind2 <= '9' {
		return string(ind2)
	}
	return ""
}

// addVariantTitle records a 246 variant title: a parallel title (ind2=1) or a typed
// variant (cover/spine on the Instance, others on the Work).
func (g *BIBFRAME) addVariantTitle(f codex.Field) {
	vt := VariantTitle{
		Parallel:    f.Ind2 == '1',
		VariantType: variantType(f.Ind2),
		MainTitle:   trimISBD(f.SubfieldValue('a')),
		Subtitle:    trimISBD(f.SubfieldValue('b')),
		PartNumber:  trimISBD(f.SubfieldValue('n')),
		PartName:    trimISBD(f.SubfieldValue('p')),
	}
	if vt.MainTitle == "" && vt.Subtitle == "" {
		return
	}
	switch f.Ind2 {
	case '4', '8': // cover / spine titles describe the physical Instance
		g.Instance.VariantTitles = append(g.Instance.VariantTitles, vt)
	default:
		g.Work.VariantTitles = append(g.Work.VariantTitles, vt)
	}
}

// variantType maps a 246 second indicator to its bf:variantType token (empty for a
// blank indicator or the parallel form, which the Parallel flag already carries).
func variantType(ind2 byte) string {
	switch ind2 {
	case '0':
		return "portion"
	case '2':
		return "distinctive"
	case '3':
		return "other"
	case '4':
		return "cover"
	case '5':
		return "added title page"
	case '6':
		return "caption"
	case '7':
		return "running"
	case '8':
		return "spine"
	default: // blank (no type) or '1' (parallel, carried by the Parallel flag)
		return ""
	}
}

// ind2ForVariant inverts variantType/Parallel back to a 246 second indicator.
func ind2ForVariant(vt VariantTitle) byte {
	if vt.Parallel {
		return '1'
	}
	switch vt.VariantType {
	case "portion":
		return '0'
	case "distinctive":
		return '2'
	case "other":
		return '3'
	case "cover":
		return '4'
	case "added title page":
		return '5'
	case "caption":
		return '6'
	case "running":
		return '7'
	case "spine":
		return '8'
	default:
		return ' '
	}
}

// noteLabel renders a 5xx note's text: the field's subfield values in order,
// space-joined, skipping the linkage subfields ($6/$8). A single-$a note reads
// exactly as its $a; multi-subfield notes (533 reproduction details) keep every
// part rather than dropping all but $a.
func noteLabel(f codex.Field) string {
	var parts []string
	for _, s := range f.Subfields {
		if s.Code == '6' || s.Code == '8' {
			continue
		}
		if v := strings.TrimRight(s.Value, " "); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

// seriesStatement renders a 490's series title ($a) as one bf:seriesStatement
// literal. The volume designation ($v) is a separate bf:seriesEnumeration, as in
// LoC marc2bibframe2, rather than being packed in after an ISBD " ; " separator:
// a series title may itself contain " ; ", which a packed statement cannot be
// split back apart on. A repeated $v keeps the first, which is what pairs one
// enumeration with one statement.
func seriesStatement(f codex.Field) string {
	return trimISBD(f.SubfieldValue('a'))
}

// appendDigitalCharacteristics reads a 347 field's file type ($a) and encoding
// format ($b) values as typed digital characteristics.
func (g *BIBFRAME) appendDigitalCharacteristics(f codex.Field) {
	for _, s := range f.Subfields {
		var class string
		switch s.Code {
		case 'a':
			class = "FileType"
		case 'b':
			class = "EncodingFormat"
		default:
			continue
		}
		if v := trimISBD(s.Value); v != "" {
			g.Instance.DigitalCharacteristics = append(g.Instance.DigitalCharacteristics, DigitalCharacteristic{Class: class, Label: v})
		}
	}
}

// noteTypeForTag maps a 5xx note tag to its bf:noteType token ("" for a general
// 500 note).
func noteTypeForTag(tag string) string {
	switch tag {
	case "504":
		return "bibliography"
	case "511":
		return "performers"
	case "521":
		return "audience"
	case "533":
		return "reproduction"
	case "538":
		return "systemDetails"
	case "546":
		return "language"
	default: // 500
		return ""
	}
}

// tagForNoteType inverts noteTypeForTag, mapping a bf:noteType token back to its 5xx
// tag (500 for a general/unknown note).
func tagForNoteType(noteType string) string {
	switch noteType {
	case "bibliography":
		return "504"
	case "performers":
		return "511"
	case "audience":
		return "521"
	case "reproduction":
		return "533"
	case "systemDetails":
		return "538"
	case "language":
		return "546"
	default:
		return "500"
	}
}

// issuanceForLevel maps leader/07 (bibliographic level) to a mode-of-issuance code
// in the LoC issuance vocabulary, or "" when no mode applies.
func issuanceForLevel(level byte) string {
	switch level {
	case 'm':
		return "mono" // monograph / single unit
	case 's':
		return "serl" // serial
	case 'i':
		return "intg" // integrating resource
	case 'c', 'd':
		return "coll" // collection / subunit
	default:
		return ""
	}
}

// levelForIssuance inverts issuanceForLevel back to a leader/07 byte, or 0 when the
// code is unknown.
func levelForIssuance(code string) byte {
	switch code {
	case "mono":
		return 'm'
	case "serl":
		return 's'
	case "intg":
		return 'i'
	case "coll":
		return 'c'
	default:
		return 0
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

func instanceURI(base string) string { return "#" + base + "Instance" }
