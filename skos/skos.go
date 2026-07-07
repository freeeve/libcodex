// Package skos reads a SKOS concept scheme (a controlled vocabulary such as
// homosaurus or LCSH published as RDF) and crosswalks each skos:Concept to a MARC
// authority record. It parses the common RDF serializations via the rdf package
// and maps prefLabel/altLabel/broader/narrower/related onto the MARC 1xx/4xx/5xx
// authority structure.
package skos

import (
	"io"
	"sort"
	"strings"

	"github.com/freeeve/libcodex/rdf"
)

// SKOS and companion vocabulary IRIs used by the crosswalk.
const (
	skosNS = "http://www.w3.org/2004/02/skos/core#"
	dctNS  = "http://purl.org/dc/terms/"
	rdfsNS = "http://www.w3.org/2000/01/rdf-schema#"

	cConcept    = skosNS + "Concept"
	pPrefLabel  = skosNS + "prefLabel"
	pAltLabel   = skosNS + "altLabel"
	pBroader    = skosNS + "broader"
	pNarrower   = skosNS + "narrower"
	pRelated    = skosNS + "related"
	pInScheme   = skosNS + "inScheme"
	pScopeNote  = skosNS + "scopeNote"
	pIdentifier = dctNS + "identifier"
	pComment    = rdfsNS + "comment"
)

// Label is a text value with an optional BCP-47 language tag.
type Label struct {
	Text string
	Lang string // "" when the source literal had no language tag
}

// Ref is a reference from one concept to another: the target IRI, its short id,
// and its preferred label resolved against the scheme when the target is present.
type Ref struct {
	URI   string
	ID    string
	Label string // "" when the target concept is not in the parsed set
}

// Concept is one skos:Concept with the fields the authority crosswalk uses.
type Concept struct {
	URI      string  // the concept IRI (subject)
	ID       string  // dc:identifier, or the IRI's last path segment
	Scheme   string  // skos:inScheme target IRI; optional
	Pref     []Label // skos:prefLabel (one per language)
	Alt      []Label // skos:altLabel
	Broader  []Ref   // skos:broader
	Narrower []Ref   // skos:narrower
	Related  []Ref   // skos:related
	Notes    []Label // skos:scopeNote and rdfs:comment
}

// PrefLabel returns the concept's English-preferred preferred label, falling back
// to the first prefLabel in document order, or "" when none is present.
func (c Concept) PrefLabel() string { return pickLabel(c.Pref) }

// Read parses a SKOS concept scheme from r. It is Parse over the whole stream.
func Read(r io.Reader) ([]Concept, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse reads a SKOS concept scheme from RDF bytes (serialization autodetected)
// and returns its concepts sorted by id. broader/narrower/related references are
// resolved to the target concept's preferred label when the target is in the set.
func Parse(data []byte) ([]Concept, error) {
	g, err := parseGraph(data)
	if err != nil {
		return nil, err
	}
	subjects := g.SubjectsOfType(cConcept)
	concepts := make([]Concept, 0, len(subjects))
	index := make(map[string]int, len(subjects))
	for _, s := range subjects {
		c := Concept{
			URI:      s.Value,
			ID:       firstLiteral(g, s, pIdentifier),
			Scheme:   firstIRI(g, s, pInScheme),
			Pref:     labelsOf(g, s, pPrefLabel),
			Alt:      labelsOf(g, s, pAltLabel),
			Broader:  refsOf(g, s, pBroader),
			Narrower: refsOf(g, s, pNarrower),
			Related:  refsOf(g, s, pRelated),
			Notes:    append(labelsOf(g, s, pScopeNote), labelsOf(g, s, pComment)...),
		}
		if c.ID == "" {
			c.ID = lastSegment(c.URI)
		}
		index[c.URI] = len(concepts)
		concepts = append(concepts, c)
	}
	for i := range concepts {
		resolveRefs(concepts[i].Broader, concepts, index)
		resolveRefs(concepts[i].Narrower, concepts, index)
		resolveRefs(concepts[i].Related, concepts, index)
	}
	sort.SliceStable(concepts, func(i, j int) bool { return concepts[i].ID < concepts[j].ID })
	return concepts, nil
}

// resolveRefs fills each ref's ID and Label from the target concept when it is in
// the parsed set, else derives a short ID from the IRI's last path segment.
func resolveRefs(refs []Ref, concepts []Concept, index map[string]int) {
	for i := range refs {
		if j, ok := index[refs[i].URI]; ok {
			refs[i].ID = concepts[j].ID
			refs[i].Label = concepts[j].PrefLabel()
		} else {
			refs[i].ID = lastSegment(refs[i].URI)
		}
	}
}

// labelsOf returns the language-tagged literal objects of predicate on subject.
func labelsOf(g *rdf.Graph, subject rdf.Term, predicate string) []Label {
	var out []Label
	for _, o := range g.Objects(subject, predicate) {
		if o.IsLiteral() && o.Value != "" {
			out = append(out, Label{Text: o.Value, Lang: o.Lang})
		}
	}
	return out
}

// refsOf returns the IRI-reference objects of predicate on subject as unresolved
// refs (URI only; ID/Label are filled by resolveRefs).
func refsOf(g *rdf.Graph, subject rdf.Term, predicate string) []Ref {
	var out []Ref
	for _, o := range g.Objects(subject, predicate) {
		if o.IsIRI() && o.Value != "" {
			out = append(out, Ref{URI: o.Value})
		}
	}
	return out
}

// firstLiteral returns the first literal object value of predicate, or "".
func firstLiteral(g *rdf.Graph, subject rdf.Term, predicate string) string {
	for _, o := range g.Objects(subject, predicate) {
		if o.IsLiteral() {
			return o.Value
		}
	}
	return ""
}

// firstIRI returns the first IRI object value of predicate, or "".
func firstIRI(g *rdf.Graph, subject rdf.Term, predicate string) string {
	for _, o := range g.Objects(subject, predicate) {
		if o.IsIRI() {
			return o.Value
		}
	}
	return ""
}

// pickLabel returns the English-preferred label text, else the first label, else "".
func pickLabel(labels []Label) string {
	for _, l := range labels {
		if isEnglish(l.Lang) {
			return l.Text
		}
	}
	if len(labels) > 0 {
		return labels[0].Text
	}
	return ""
}

// isEnglish reports whether a language tag names English ("en" or "en-*").
func isEnglish(lang string) bool {
	return lang == "en" || strings.HasPrefix(lang, "en-")
}

// lastSegment returns the final '/'- or '#'-separated segment of an IRI, the
// conventional short id for a concept.
func lastSegment(iri string) string {
	iri = strings.TrimRight(iri, "/#")
	if i := strings.LastIndexAny(iri, "/#"); i >= 0 {
		return iri[i+1:]
	}
	return iri
}
