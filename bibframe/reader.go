package bibframe

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// BIBFRAME vocabulary IRIs used by the reverse crosswalk.
const (
	classWork     = bfNS + "Work"
	classInstance = bfNS + "Instance"

	pType         = rdfNS + "type"
	pLabel        = rdfsNS + "label"
	pValue        = rdfNS + "value"
	pTitle        = bfNS + "title"
	pMainTitle    = bfNS + "mainTitle"
	pSubtitle     = bfNS + "subtitle"
	pPartNumber   = bfNS + "partNumber"
	pPartName     = bfNS + "partName"
	pContribution = bfNS + "contribution"
	pAgent        = bfNS + "agent"
	pRole         = bfNS + "role"
	pSubject      = bfNS + "subject"
	pGenreForm    = bfNS + "genreForm"
	pLanguage     = bfNS + "language"
	pClassif      = bfNS + "classification"
	pClassPortion = bfNS + "classificationPortion"
	pSummary      = bfNS + "summary"
	pHasInstance  = bfNS + "hasInstance"
	pInstanceOf   = bfNS + "instanceOf"
	pRespStmt     = bfNS + "responsibilityStatement"
	pEdition      = bfNS + "editionStatement"
	pProvision    = bfNS + "provisionActivity"
	pPlace        = bfNS + "place"
	pDate         = bfNS + "date"
	pExtent       = bfNS + "extent"
	pMedia        = bfNS + "media"
	pCarrier      = bfNS + "carrier"
	pIdentifiedBy = bfNS + "identifiedBy"
	pLocator      = bfNS + "electronicLocator"
	pCode         = bfNS + "code"

	// Administrative metadata (bf:AdminMetadata) — provenance about the record's
	// description and the process that generated the RDF.
	pAdminMetadata          = bfNS + "adminMetadata"
	pGenerationProcess      = bfNS + "generationProcess"
	pChangeDate             = bfNS + "changeDate"
	pDescriptionConventions = bfNS + "descriptionConventions"
	classAdminMetadata      = bfNS + "AdminMetadata"
	classGenerationProcess  = bfNS + "GenerationProcess"
	classLocal              = bfNS + "Local"

	// A source/scheme node on an identifier or classification.
	pSource     = bfNS + "source"
	classSource = bfNS + "Source"

	// LoC's marc2bibframe2 carries the transcribed publication statement in these
	// bflc properties, alongside the controlled bf:place / bf:date.
	pSimplePlace = bflcNS + "simplePlace"
	pSimpleAgent = bflcNS + "simpleAgent"
	pSimpleDate  = bflcNS + "simpleDate"

	primaryContribution   = bflcNS + "PrimaryContribution"
	bfPrimaryContribution = bfNS + "PrimaryContribution"
)

// agentClasses are the bf agent subclasses, in MARC-tag preference order, used to
// pick the specific class when an agent node also carries the generic bf:Agent
// type (as LoC's marc2bibframe2 output does).
var agentClasses = []string{"Organization", "Meeting", "Person", "Family", "Jurisdiction"}

// Decode parses a BIBFRAME document — RDF/XML, JSON-LD, Turtle or N-Triples,
// autodetected — and reverse-crosswalks every bf:Work (with its linked
// bf:Instance) to a MARC 21 record. It reads the vocabulary the forward crosswalk
// emits and the common shape of LoC marc2bibframe2 output. BIBFRAME is a lossier
// model than MARC, so
// the result carries the crosswalked fields rather than reproducing the original
// record byte for byte; re-encoding it yields an equivalent BIBFRAME graph.
func Decode(data []byte) ([]*codex.Record, error) {
	g, err := parseGraph(data)
	if err != nil {
		return nil, err
	}
	backref := instanceBackrefs(g)
	var out []*codex.Record
	for _, work := range g.SubjectsOfType(classWork) {
		out = append(out, recordFromWork(g, work, backref))
	}
	return out, nil
}

// instanceBackrefs maps each Work to an Instance that names it with bf:instanceOf,
// built in one pass over the triples. It lets recordFromWork resolve a Work that
// lacks bf:hasInstance in O(1) instead of scanning every Instance per Work, so
// Decode of an aggregated document scales linearly rather than quadratically.
func instanceBackrefs(g *rdf.Graph) map[rdf.Term]rdf.Term {
	m := map[rdf.Term]rdf.Term{}
	for _, t := range g.Triples {
		if t.P.Kind == rdf.IRI && t.P.Value == pInstanceOf {
			if _, seen := m[t.O]; !seen { // first Instance wins, matching document order
				m[t.O] = t.S
			}
		}
	}
	return m
}

// parseGraph picks the RDF parser by sniffing the serialization.
func parseGraph(data []byte) (*rdf.Graph, error) {
	switch sniffFormat(data) {
	case formatJSONLD:
		return rdf.ParseJSONLD(data)
	case formatRDFXML:
		return rdf.ParseRDFXML(data)
	case formatTurtle:
		return rdf.ParseTurtle(data)
	default:
		return rdf.ParseNTriples(data)
	}
}

type rdfFormat int

const (
	formatNTriples rdfFormat = iota
	formatJSONLD
	formatRDFXML
	formatTurtle
)

// sniffFormat guesses the RDF serialization from the leading bytes: '{' is
// JSON-LD, and '[' is JSON-LD unless it opens a Turtle blank-node property list;
// '@' or a PREFIX/BASE keyword is Turtle; a leading '<' is RDF/XML when it opens
// an XML start tag and N-Triples/Turtle when it opens an <IRI> subject; the
// line-based remainder is treated as N-Triples (which the Turtle grammar also
// subsumes).
func sniffFormat(data []byte) rdfFormat {
	s := bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // optional UTF-8 BOM
	for {
		s = bytes.TrimLeft(s, " \t\r\n")
		if len(s) > 0 && s[0] == '#' { // skip Turtle/N-Triples comment lines
			if i := bytes.IndexByte(s, '\n'); i >= 0 {
				s = s[i+1:]
				continue
			}
		}
		break
	}
	if len(s) == 0 {
		return formatNTriples
	}
	switch s[0] {
	case '{':
		return formatJSONLD
	case '[':
		// '[' opens either a JSON-LD array (whose first element is a JSON value:
		// an object, a string, or nothing for an empty array) or a Turtle
		// blank-node property list, "[ a bf:Work ]", whose first token is a
		// predicate: the 'a' keyword, a prefixed name, or an <IRI>. A letter,
		// '_', ':' or '<' after the bracket means Turtle; anything else JSON-LD.
		rest := bytes.TrimLeft(s[1:], " \t\r\n")
		if len(rest) > 0 && (rest[0] == '<' || rest[0] == '_' || rest[0] == ':' ||
			(rest[0] >= 'a' && rest[0] <= 'z') || (rest[0] >= 'A' && rest[0] <= 'Z')) {
			return formatTurtle
		}
		return formatJSONLD
	case '@':
		return formatTurtle
	case '<':
		// Distinguish an XML start tag from a leading <IRI>. A processing
		// instruction or doctype is RDF/XML. Otherwise inspect the first
		// angle-bracketed token and what follows it: an attribute ('=') inside the
		// token is an XML start tag (RDF/XML); a following subject-position term
		// ('<', '_', or a quote) or a first token that is an absolute IRI (bearing
		// a scheme ':', path '/', or fragment '#') is N-Triples/Turtle.
		if bytes.HasPrefix(s, []byte("<?")) || bytes.HasPrefix(s, []byte("<!")) {
			return formatRDFXML
		}
		inner := s[1:]
		first, after, _ := bytes.Cut(inner, []byte{'>'})
		rest := bytes.TrimLeft(after, " \t\r\n")
		switch {
		case bytes.IndexByte(first, '=') >= 0:
			return formatRDFXML
		case len(rest) > 0 && (rest[0] == '<' || rest[0] == '_' || rest[0] == '"'):
			return formatNTriples
		case bytes.IndexByte(first, '#') >= 0 || bytes.IndexByte(first, '/') >= 0 || bytes.IndexByte(first, ':') >= 0:
			return formatNTriples
		default:
			return formatRDFXML // a bare element name
		}
	}
	if hasKeyword(s, "prefix") || hasKeyword(s, "base") {
		return formatTurtle
	}
	return formatNTriples
}

// hasKeyword reports whether s begins with the case-insensitive keyword followed
// by whitespace (a SPARQL-style Turtle directive).
func hasKeyword(s []byte, kw string) bool {
	if len(s) <= len(kw) || !strings.EqualFold(string(s[:len(kw)]), kw) {
		return false
	}
	c := s[len(kw)]
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// recordFromWork builds a MARC record from one Work node and the Instance it
// links to, assembling fields in ascending tag order.
// ---- entry points ----

// Reader reads BIBFRAME records from a stream. A BIBFRAME document is a single
// RDF graph, so the first Read parses the whole input; successive calls return
// the reconstructed records in document order, then io.EOF.
type Reader struct {
	src  io.Reader
	recs []*codex.Record
	i    int
	err  error
	done bool
}

// NewReader returns a Reader over r. It implements codex.RecordReader, so a
// BIBFRAME document can be a source for codex.Convert.
func NewReader(r io.Reader) *Reader { return &Reader{src: r} }

// Read returns the next record, or io.EOF when the document is exhausted.
func (rd *Reader) Read() (*codex.Record, error) {
	if !rd.done {
		rd.done = true
		var data []byte
		if data, rd.err = io.ReadAll(rd.src); rd.err == nil {
			rd.recs, rd.err = Decode(data)
		}
	}
	if rd.err != nil {
		return nil, rd.err
	}
	if rd.i >= len(rd.recs) {
		return nil, io.EOF
	}
	rec := rd.recs[rd.i]
	rd.i++
	return rec, nil
}

// ReadFile reads and decodes every BIBFRAME record in the file at path.
func ReadFile(path string) ([]*codex.Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(data)
}
