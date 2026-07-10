package bibframe

import (
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// TestAdminAssignerAndConventions covers 003 -> bf:assigner on the 001 bf:Local, and
// multiple 040 $e -> bf:DescriptionConventions nodes with vocabulary IRIs (task 069).
func TestAdminAssignerAndConventions(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "12345")).
		AddField(codex.NewControlField("003", "DLC")).
		AddField(codex.NewDataField("040", ' ', ' ', codex.NewSubfield('e', "rda"), codex.NewSubfield('e', "pn"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))

	g := FromRecord(rec)
	if g.Instance.Admin.ControlOrg != "DLC" {
		t.Errorf("ControlOrg = %q, want DLC", g.Instance.Admin.ControlOrg)
	}
	if len(g.Instance.Admin.DescriptionConventions) != 2 {
		t.Errorf("descriptionConventions = %+v, want 2", g.Instance.Admin.DescriptionConventions)
	}

	graph, _ := rdf.ParseNTriples(mustEncodeNT(t, rec))
	// The bf:Local (001) carries a bf:assigner agent with the organizations IRI.
	local := graph.SubjectsOfType(classLocal)
	if len(local) != 1 {
		t.Fatalf("want 1 bf:Local, got %d", len(local))
	}
	assigner, ok := graph.Object(local[0], bfNS+"assigner")
	if !ok {
		t.Fatal("bf:Local has no bf:assigner")
	}
	if !assigner.IsIRI() || assigner.Value != "http://id.loc.gov/vocabulary/organizations/dlc" {
		t.Errorf("assigner = %+v, want organizations/dlc IRI", assigner)
	}
	if v, _ := graph.Literal(assigner, bfNS+"code"); v != "DLC" {
		t.Errorf("assigner bf:code = %q, want DLC", v)
	}
	// Two bf:DescriptionConventions nodes, one carrying the rda vocabulary IRI.
	if n := len(graph.SubjectsOfType(bfNS + "DescriptionConventions")); n != 2 {
		t.Errorf("want 2 DescriptionConventions nodes, got %d", n)
	}
	var sawRDA bool
	for _, o := range graph.Objects(graph.SubjectsOfType(classAdminMetadata)[0], bfNS+"descriptionConventions") {
		if o.Value == "http://id.loc.gov/vocabulary/descriptionConventions/rda" {
			sawRDA = true
		}
	}
	if !sawRDA {
		t.Error("no descriptionConventions node with the rda vocabulary IRI")
	}
}

// field040 returns the record's 040 rendered as a marcKey string, or "" when the
// record has none, so a round-trip can be asserted field-exact in one comparison.
func field040(rec *codex.Record) string {
	for _, f := range rec.Fields() {
		if f.Tag != "040" {
			continue
		}
		var b strings.Builder
		b.WriteString("040")
		b.WriteByte(f.Ind1)
		b.WriteByte(f.Ind2)
		for _, s := range f.Subfields {
			b.WriteByte('$')
			b.WriteByte(s.Code)
			b.WriteString(s.Value)
		}
		return b.String()
	}
	return ""
}

// catalogingSource040 is the acceptance record for task 094: a full 040 agency
// chain, with a repeated modifying agency.
func catalogingSource040() *codex.Record {
	return codex.NewRecord().
		AddField(codex.NewControlField("001", "12345")).
		AddField(codex.NewDataField("040", ' ', ' ',
			codex.NewSubfield('a', "DLC"),
			codex.NewSubfield('b', "eng"),
			codex.NewSubfield('c', "DLC"),
			codex.NewSubfield('d', "OCLCQ"),
			codex.NewSubfield('d', "UKMGB"),
			codex.NewSubfield('e', "rda"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))
}

// TestCatalogingSourceFromRecord covers 040 -> AdminMetadata: the full agency chain
// lands in the model, and the graph carries LoC's properties -- $a bf:assigner,
// $b bf:descriptionLanguage, $d bf:descriptionModifier (one per $d) -- alongside
// the internal bf:Note that holds the field verbatim (task 094).
func TestCatalogingSourceFromRecord(t *testing.T) {
	g := FromRecord(catalogingSource040())
	am := g.Instance.Admin
	if am.OrigAgency != "DLC" || am.DescriptionLanguage != "eng" || am.Transcriber != "DLC" {
		t.Errorf("040 $a/$b/$c = %q/%q/%q, want DLC/eng/DLC", am.OrigAgency, am.DescriptionLanguage, am.Transcriber)
	}
	if len(am.Modifiers) != 2 || am.Modifiers[0] != "OCLCQ" || am.Modifiers[1] != "UKMGB" {
		t.Errorf("040 $d = %q, want [OCLCQ UKMGB]", am.Modifiers)
	}

	graph, _ := rdf.ParseNTriples(mustEncodeNT(t, catalogingSource040()))
	admin := graph.SubjectsOfType(classAdminMetadata)
	if len(admin) != 1 {
		t.Fatalf("want 1 bf:AdminMetadata, got %d", len(admin))
	}
	// $a -> bf:assigner, carrying the organizations IRI and the raw code.
	assigner, ok := graph.Object(admin[0], pAssigner)
	if !ok || assigner.Value != orgVocab+"dlc" {
		t.Errorf("bf:assigner = %+v, want %s", assigner, orgVocab+"dlc")
	}
	if v, _ := graph.Literal(assigner, pCode); v != "DLC" {
		t.Errorf("assigner bf:code = %q, want DLC", v)
	}
	// $b -> bf:descriptionLanguage on the languages vocabulary.
	lang, ok := graph.Object(admin[0], pDescriptionLanguage)
	if !ok || lang.Value != langVocab+"eng" {
		t.Errorf("bf:descriptionLanguage = %+v, want %seng", lang, langVocab)
	}
	// Each $d -> its own bf:descriptionModifier (LoC's XSLT keeps only the last).
	mods := graph.Objects(admin[0], pDescriptionModifier)
	if len(mods) != 2 {
		t.Fatalf("want 2 bf:descriptionModifier, got %d", len(mods))
	}
	if mods[0].Value != orgVocab+"oclcq" || mods[1].Value != orgVocab+"ukmgb" {
		t.Errorf("descriptionModifier IRIs = %q, %q", mods[0].Value, mods[1].Value)
	}
	// The whole field also survives as an internal bf:Note in marcKey form.
	var key string
	for _, n := range graph.Objects(admin[0], pNote) {
		if graph.HasType(n, internalNoteType) {
			key, _ = graph.Literal(n, pLabel)
		}
	}
	if want := "040  $aDLC$beng$cDLC$dOCLCQ$dUKMGB$erda"; key != want {
		t.Errorf("internal note = %q, want %q", key, want)
	}
}

// TestCatalogingSourceRoundTrip is task 094's acceptance: a full 040 survives
// FromRecord -> RDF -> Decode field-exact, in every serialization.
func TestCatalogingSourceRoundTrip(t *testing.T) {
	rec := catalogingSource040()
	want := field040(rec)

	x, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	j, err := EncodeJSONLD(rec)
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"rdfxml": x, "jsonld": j, "ntriples": mustEncodeNT(t, rec)} {
		recs, err := Decode(b)
		if err != nil {
			t.Fatalf("%s decode: %v", name, err)
		}
		if len(recs) != 1 {
			t.Fatalf("%s: want 1 record, got %d", name, len(recs))
		}
		if got := field040(recs[0]); got != want {
			t.Errorf("%s: 040 = %q, want %q", name, got, want)
		}
	}
}

// TestCatalogingSourceAbsent confirms a record without an 040 decodes without one:
// AdminMetadata still carries the 001, but nothing fabricates a cataloging agency.
func TestCatalogingSourceAbsent(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "12345")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))

	x, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(x), "mnotetype/internal") {
		t.Errorf("record with no 040 emitted an internal note:\n%s", x)
	}
	recs, err := Decode(x)
	if err != nil {
		t.Fatal(err)
	}
	if got := field040(recs[0]); got != "" {
		t.Errorf("040 = %q, want none", got)
	}
}

// TestCatalogingSourceFromProperties covers the fallback for a third-party BIBFRAME
// that models 040 semantically but carries no internal note: every subfield but $c
// is recovered, in canonical order.
func TestCatalogingSourceFromProperties(t *testing.T) {
	const doc = `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="` + rdfNS + `" xmlns:rdfs="` + rdfsNS + `" xmlns:bf="` + bfNS + `">
  <bf:Work rdf:about="#w1">
    <bf:hasInstance rdf:resource="#i1"/>
  </bf:Work>
  <bf:Instance rdf:about="#i1">
    <bf:adminMetadata>
      <bf:AdminMetadata>
        <bf:assigner><bf:Agent rdf:about="` + orgVocab + `dlc"/></bf:assigner>
        <bf:descriptionLanguage><bf:Language rdf:about="` + langVocab + `eng"/></bf:descriptionLanguage>
        <bf:descriptionModifier><bf:Agent><bf:code>OCLCQ</bf:code></bf:Agent></bf:descriptionModifier>
        <bf:descriptionConventions><bf:DescriptionConventions rdf:about="` + conventionsVocab + `rda"/></bf:descriptionConventions>
      </bf:AdminMetadata>
    </bf:adminMetadata>
  </bf:Instance>
</rdf:RDF>`

	recs, err := Decode([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	// $a and the language come back from their vocabulary IRIs, $d from its bf:code.
	if got, want := field040(recs[0]), "040  $aDLC$beng$dOCLCQ$erda"; got != want {
		t.Errorf("040 = %q, want %q", got, want)
	}
}

// TestChangeDateTyped confirms 005 is emitted as an xsd:dateTime typed literal in
// all serializations and survives a JSON-LD round-trip (task 069).
func TestChangeDateTyped(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "12345")).
		AddField(codex.NewControlField("005", "19931204093000.0")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))

	x, _ := Encode(rec)
	if !strings.Contains(string(x), `<bf:changeDate rdf:datatype="http://www.w3.org/2001/XMLSchema#dateTime">1993-12-04T09:30:00</bf:changeDate>`) {
		t.Errorf("RDF/XML changeDate not xsd:dateTime typed:\n%s", x)
	}
	j, _ := EncodeJSONLD(rec)
	if !strings.Contains(string(j), `"bf:changeDate":{"@value":"1993-12-04T09:30:00","@type":"http://www.w3.org/2001/XMLSchema#dateTime"}`) {
		t.Errorf("JSON-LD changeDate not xsd:dateTime typed:\n%s", j)
	}
	// The datatype survives parsing in every serialization.
	for name, b := range map[string][]byte{"rdfxml": x, "jsonld": j} {
		g, err := parseGraph(b)
		if err != nil {
			t.Fatalf("%s parse: %v", name, err)
		}
		am := g.SubjectsOfType(classAdminMetadata)
		if len(am) != 1 {
			t.Fatalf("%s: want 1 AdminMetadata, got %d", name, len(am))
		}
		found := false
		for _, o := range g.Objects(am[0], bfNS+"changeDate") {
			if o.Datatype == xsdDateTime {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: changeDate lost its xsd:dateTime datatype", name)
		}
	}
}

// TestCatalogingSourceRepeatedAgencyDescribedOnce guards a divergence from every
// conformant RDF parser. A 040 routinely names one agency in several roles --
// "$aDLC$cDLC$dDLC" is the commonest 040 in the LC corpus -- and describing that
// agency's node under each role emits the same triples more than once. An RDF
// graph is a set, so a conformant parser reads back fewer triples than we wrote,
// and our own Graph, which is a slice, disagreed with rdflib on the count.
func TestCatalogingSourceRepeatedAgencyDescribedOnce(t *testing.T) {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "12345")).
		AddField(codex.NewDataField("040", ' ', ' ',
			codex.NewSubfield('a', "DLC"),
			codex.NewSubfield('c', "DLC"),
			codex.NewSubfield('d', "DLC"),
			codex.NewSubfield('d', "OCLCQ"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))

	x, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	j, err := EncodeJSONLD(rec)
	if err != nil {
		t.Fatal(err)
	}
	nt := mustEncodeNT(t, rec)
	for name, parse := range map[string]func() (*rdf.Graph, error){
		"rdfxml":   func() (*rdf.Graph, error) { return rdf.ParseRDFXML(x) },
		"jsonld":   func() (*rdf.Graph, error) { return rdf.ParseJSONLD(j) },
		"ntriples": func() (*rdf.Graph, error) { return rdf.ParseNTriples(nt) },
	} {
		g, err := parse()
		if err != nil {
			t.Fatalf("%s parse: %v", name, err)
		}
		seen := make(map[rdf.Triple]bool, len(g.Triples))
		for _, tr := range g.Triples {
			if seen[tr] {
				t.Errorf("%s: duplicate triple %v %v %v", name, tr.S, tr.P, tr.O)
			}
			seen[tr] = true
		}

		// The repeated agency must still be described once, not dropped: a bare
		// reference is only sound because the first mention carries bf:code.
		agent := rdf.NewIRI(orgVocab + "dlc")
		if got := g.Objects(agent, bfNS+"code"); len(got) != 1 || got[0].Value != "DLC" {
			t.Errorf("%s: bf:code on the shared agency = %v, want one \"DLC\"", name, got)
		}
	}

	// And the whole field still round-trips, so deduping cost no subfield.
	recs, err := Decode(x)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := field040(recs[0]), field040(rec); got != want {
		t.Errorf("040 round-trip = %q, want %q", got, want)
	}
}
