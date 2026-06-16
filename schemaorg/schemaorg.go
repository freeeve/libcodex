// Package schemaorg converts MARC 21 records to schema.org JSON-LD — the
// vocabulary search engines and reading systems consume on the web. Each record
// becomes a Book (or a related CreativeWork subtype) carrying the common
// bibliographic fields plus schema.org accessibility metadata (accessMode,
// accessibilityFeature, accessibilitySummary) derived from the record's 008, 007,
// 341 and 532 fields.
//
// schema.org is a different, flat model, so this is a one-way MARC->schema.org
// crosswalk, not a codec. The Writer emits a JSON array and implements
// codex.RecordWriter, so it plugs into codex.Convert; it must be closed:
//
//	w := schemaorg.NewWriter(out)
//	codex.Convert(iso2709.NewReader(src), w)
//	w.Close()
package schemaorg

import (
	"slices"
	"strings"

	"github.com/freeeve/libcodex"
)

// Book is the schema.org representation of a record. Empty fields are omitted.
type Book struct {
	Type          string
	Name          string
	Authors       []Agent
	Contributors  []Agent
	Publisher     string
	DatePublished string
	ISBN          []string
	ISSN          []string
	InLanguage    []string
	About         []string
	Genre         []string
	Edition       string
	URL           []string
	Description   string

	// Accessibility (schema.org a11y vocabulary).
	AccessMode           []string
	AccessibilityFeature []string
	AccessibilitySummary string
}

// Agent is a schema.org Person or Organization.
type Agent struct {
	Type string // "Person" or "Organization"
	Name string
}

// FromRecord maps a MARC record to a schema.org Book in a single pass.
func FromRecord(r *codex.Record) *Book {
	b := &Book{Type: schemaType(r.Leader().RecordType())}
	for _, f := range r.Fields() {
		switch f.Tag {
		case "245":
			b.Name = joinSub(f, "ab", " ")
		case "100", "110", "111":
			b.Authors = appendAgent(b.Authors, f)
		case "700", "710", "711":
			b.Contributors = appendAgent(b.Contributors, f)
		case "250":
			b.Edition = trimISBD(f.SubfieldValue('a'))
		case "260", "264":
			if b.Publisher == "" {
				b.Publisher = trimISBD(f.SubfieldValue('b'))
			}
			if b.DatePublished == "" {
				b.DatePublished = year(trimISBD(f.SubfieldValue('c')))
			}
		case "020":
			b.ISBN = appendValues(b.ISBN, f, 'a')
		case "022":
			b.ISSN = appendValues(b.ISSN, f, 'a')
		case "600", "610", "611", "630", "650", "651", "653":
			if v := subject(f); v != "" {
				b.About = append(b.About, v)
			}
		case "655":
			if v := trimISBD(f.SubfieldValue('a')); v != "" {
				b.Genre = append(b.Genre, v)
			}
		case "520":
			if b.Description == "" {
				b.Description = strings.TrimRight(f.SubfieldValue('a'), " ")
			}
		case "856":
			b.URL = appendValues(b.URL, f, 'u')
		case "041":
			for _, code := range f.SubfieldValues('a') {
				for i := 0; i+3 <= len(code); i += 3 {
					b.addLanguage(code[i : i+3])
				}
			}
		}
	}
	if c, ok := r.Control008(); ok {
		b.addLanguage(c.Language())
		if b.DatePublished == "" {
			b.DatePublished = year(c.Date1())
		}
	}
	b.applyAccessibility(r.Accessibility())
	return b
}

func (b *Book) addLanguage(code string) {
	code = strings.TrimSpace(code)
	if len(code) == 3 && !slices.Contains(b.InLanguage, bcp47(code)) {
		b.InLanguage = append(b.InLanguage, bcp47(code))
	}
}

// applyAccessibility maps the MARC accessibility facts onto schema.org
// properties. The accessibilityFeature values are from the W3C accessibility
// discoverability vocabulary ("largePrint", "braille", "tactileObject").
func (b *Book) applyAccessibility(a codex.Accessibility) {
	b.AccessMode = a.AccessModes
	if a.LargePrint {
		b.AccessibilityFeature = append(b.AccessibilityFeature, "largePrint")
	}
	if a.Braille {
		b.AccessibilityFeature = append(b.AccessibilityFeature, "braille")
	}
	if a.Tactile && !a.Braille {
		b.AccessibilityFeature = append(b.AccessibilityFeature, "tactileObject")
	}
	b.AccessibilitySummary = strings.Join(a.Notes, " ")
}

// ---- crosswalk helpers ----

// schemaType maps leader byte 6 (type of record) to a schema.org @type.
func schemaType(recordType byte) string {
	switch recordType {
	case 'a', 't':
		return "Book"
	case 'c', 'd':
		return "MusicComposition"
	case 'e', 'f':
		return "Map"
	case 'g':
		return "Movie"
	case 'i':
		return "AudioObject"
	case 'j':
		return "MusicRecording"
	case 'k':
		return "ImageObject"
	case 'm':
		return "SoftwareApplication"
	default:
		return "CreativeWork"
	}
}

func appendAgent(dst []Agent, f codex.Field) []Agent {
	name := trimISBD(f.SubfieldValue('a'))
	if name == "" {
		return dst
	}
	t := "Person"
	switch f.Tag {
	case "110", "710", "111", "711":
		t = "Organization"
	}
	return append(dst, Agent{Type: t, Name: name})
}

func subject(f codex.Field) string {
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

func appendValues(dst []string, f codex.Field, code byte) []string {
	for _, sf := range f.Subfields {
		if sf.Code == code {
			if v := trimISBD(sf.Value); v != "" {
				dst = append(dst, v)
			}
		}
	}
	return dst
}

// year returns the first run of four digits in s, or "".
func year(s string) string {
	for i := 0; i+4 <= len(s); i++ {
		if isDigit(s[i]) && isDigit(s[i+1]) && isDigit(s[i+2]) && isDigit(s[i+3]) {
			return s[i : i+4]
		}
	}
	return ""
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func trimISBD(s string) string {
	s = strings.TrimRight(s, " ")
	if n := len(s); n > 0 && strings.IndexByte("/:;,", s[n-1]) >= 0 {
		s = strings.TrimRight(s[:n-1], " ")
	}
	return s
}

// bcp47 maps a MARC/ISO 639-2/B language code to a BCP-47 tag where a common one
// exists, otherwise returns the code unchanged (schema.org accepts either).
func bcp47(code string) string {
	if tag, ok := iso639[code]; ok {
		return tag
	}
	return code
}

var iso639 = map[string]string{
	"eng": "en", "fre": "fr", "fra": "fr", "ger": "de", "deu": "de",
	"spa": "es", "ita": "it", "por": "pt", "rus": "ru", "chi": "zh",
	"zho": "zh", "jpn": "ja", "kor": "ko", "ara": "ar", "heb": "he",
	"gre": "el", "ell": "el", "lat": "la", "dut": "nl", "nld": "nl",
	"swe": "sv", "nor": "no", "dan": "da", "fin": "fi", "pol": "pl",
	"ukr": "uk", "tur": "tr", "vie": "vi", "tha": "th", "hin": "hi",
}
