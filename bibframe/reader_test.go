package bibframe

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// normalize sorts every slice in a BIBFRAME so two graphs that carry the same
// statements in a different order compare equal. BIBFRAME is an unordered RDF
// graph, so order-independence is the right notion of equivalence for the
// reverse-crosswalk round-trip.
func normalize(g *BIBFRAME) *BIBFRAME {
	// AdminMetadata is provenance about the conversion (generation process, the
	// source's control number and change date), regenerated on each encode and
	// deliberately not reverse-crosswalked to MARC, so it is not part of the
	// bibliographic crosswalk this stability check covers.
	g.Instance.Admin = nil
	sort.Slice(g.Work.Titles, func(i, j int) bool { return titleKey(g.Work.Titles[i]) < titleKey(g.Work.Titles[j]) })
	sort.Slice(g.Instance.Titles, func(i, j int) bool { return titleKey(g.Instance.Titles[i]) < titleKey(g.Instance.Titles[j]) })
	sort.Slice(g.Work.Contributions, func(i, j int) bool { return contKey(g.Work.Contributions[i]) < contKey(g.Work.Contributions[j]) })
	sort.Slice(g.Work.Subjects, func(i, j int) bool {
		return g.Work.Subjects[i].Class+g.Work.Subjects[i].Label < g.Work.Subjects[j].Class+g.Work.Subjects[j].Label
	})
	sort.Slice(g.Work.Classifications, func(i, j int) bool {
		return g.Work.Classifications[i].Class+g.Work.Classifications[i].Value < g.Work.Classifications[j].Class+g.Work.Classifications[j].Value
	})
	sort.Slice(g.Instance.Identifiers, func(i, j int) bool {
		return g.Instance.Identifiers[i].Class+g.Instance.Identifiers[i].Value < g.Instance.Identifiers[j].Class+g.Instance.Identifiers[j].Value
	})
	sort.Strings(g.Work.GenreForms)
	sort.Strings(g.Work.Languages)
	sort.Strings(g.Work.Summary)
	sort.Strings(g.Instance.Extent)
	sort.Strings(g.Instance.ElectronicLocator)
	return g
}

func titleKey(t Title) string {
	return t.Type + "|" + t.MainTitle + "|" + t.Subtitle + "|" + t.PartNumber + "|" + t.PartName
}

func contKey(c Contribution) string {
	p := "0"
	if c.Primary {
		p = "1"
	}
	key := p + "|" + c.Class + "|" + c.Label
	for _, r := range c.Roles {
		key += "|" + r.IRI + "=" + r.Term
	}
	return key
}

// roundTrip checks that decoding an encoded record and re-running the forward
// crosswalk reproduces the original BIBFRAME graph (the reverse crosswalk is a
// right inverse of the forward one).
func roundTrip(t *testing.T, encoded []byte) {
	t.Helper()
	recs, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("decoded %d records, want 1", len(recs))
	}
	got := normalize(FromRecord(recs[0]))
	want := normalize(FromRecord(sample()))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip graph differs:\n got %+v\nwant %+v", got, want)
	}
}

// TestRoundTripRDFXML and TestRoundTripJSONLD exercise both serializations
// through Encode -> Decode -> forward crosswalk.
func TestRoundTripRDFXML(t *testing.T) {
	b, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, b)
}

func TestRoundTripJSONLD(t *testing.T) {
	b, err := EncodeJSONLD(sample())
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, b)
}

// TestDecodeFields spot-checks that specific MARC fields are reconstructed with
// the expected values and subfields.
func TestDecodeFields(t *testing.T) {
	b, _ := Encode(sample())
	recs, err := Decode(b)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	rec := recs[0]

	want := map[string]string{
		"001": "92005291",
		"245": "Stone butch blues",
		"100": "Feinberg, Leslie",
		"020": "0786803525",
		"260": "Ithaca, New York",
		"650": "Lesbians",
	}
	for tag, val := range want {
		f := firstField(rec, tag)
		if f == nil {
			t.Errorf("%s: missing", tag)
			continue
		}
		if tag == "001" {
			if f.Value != val {
				t.Errorf("001 = %q, want %q", f.Value, val)
			}
			continue
		}
		if got := f.SubfieldValue('a'); got != val {
			t.Errorf("%s $a = %q, want %q", tag, got, val)
		}
	}

	// 650 keeps its subdivision as $x; the language field carries both codes.
	if f := firstField(rec, "650"); f != nil && f.SubfieldValue('x') != "Fiction" {
		t.Errorf("650 $x = %q, want Fiction", f.SubfieldValue('x'))
	}
	if f := firstField(rec, "041"); f != nil {
		var codes []string
		for _, sf := range f.Subfields {
			codes = append(codes, sf.Value)
		}
		if strings.Join(codes, ",") != "eng,fre" {
			t.Errorf("041 = %v, want [eng fre]", codes)
		}
	} else {
		t.Error("041 missing")
	}

	if rec.Leader().RecordType() != 'a' {
		t.Errorf("leader type = %c, want a", rec.Leader().RecordType())
	}
}

// TestReadFileGolden reads the checked-in golden serializations and confirms they
// reconstruct the sample graph, exercising the on-disk path for both formats.
func TestReadFileGolden(t *testing.T) {
	for _, path := range []string{"testdata/sample.rdf", "testdata/sample.jsonld"} {
		recs, err := ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		if len(recs) != 1 {
			t.Fatalf("%s: %d records, want 1", path, len(recs))
		}
		got := normalize(FromRecord(recs[0]))
		want := normalize(FromRecord(sample()))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: graph differs from sample", path)
		}
	}
}

// TestReaderStream checks the streaming Reader yields each record then io.EOF, and
// that it satisfies codex.RecordReader for use as a Convert source.
func TestReaderStream(t *testing.T) {
	// Distinct control numbers so the two records are distinct RDF resources
	// (records sharing a 001 share a Work IRI and collapse, as RDF intends).
	second := sample().RemoveFields("001").AddField(codex.NewControlField("001", "99999999"))
	var buf bytes.Buffer
	w := NewWriter(&buf)
	for _, r := range []*codex.Record{sample(), second} {
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	var rd codex.RecordReader = NewReader(&buf)
	n := 0
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if rec.Leader().RecordType() != 'a' {
			t.Errorf("record %d: leader type %c", n, rec.Leader().RecordType())
		}
		n++
	}
	if n != 2 {
		t.Errorf("read %d records, want 2", n)
	}
}

// TestDecodeForeignShape reads BIBFRAME that links Work and Instance only by
// bf:instanceOf and uses an external IRI, the shape other producers emit.
func TestDecodeForeignShape(t *testing.T) {
	doc := `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	            xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
	            xmlns:bf="http://id.loc.gov/ontologies/bibframe/">
	  <bf:Work rdf:about="http://example.org/works/42">
	    <bf:title><bf:Title><bf:mainTitle>Foreign record</bf:mainTitle></bf:Title></bf:title>
	    <bf:contribution><bf:Contribution><bf:agent><bf:Person>
	      <rdfs:label>Doe, Jane</rdfs:label></bf:Person></bf:agent></bf:Contribution></bf:contribution>
	  </bf:Work>
	  <bf:Instance rdf:about="http://example.org/instances/42">
	    <bf:instanceOf rdf:resource="http://example.org/works/42"/>
	    <bf:title><bf:Title><bf:mainTitle>Foreign record</bf:mainTitle></bf:Title></bf:title>
	  </bf:Instance>
	</rdf:RDF>`
	recs, err := Decode([]byte(doc))
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	rec := recs[0]
	if f := firstField(rec, "245"); f == nil || f.SubfieldValue('a') != "Foreign record" {
		t.Errorf("245 = %+v", f)
	}
	if f := firstField(rec, "700"); f == nil || f.SubfieldValue('a') != "Doe, Jane" {
		t.Errorf("contribution = %+v, want 700 Doe, Jane", f)
	}
}

// TestEncodersIsomorphic confirms all four serializations of one record, run
// through their separate parsers, yield isomorphic RDF graphs — a cross-check of
// every encoder and parser at once, independent of blank node labelling and
// statement order.
func TestEncodersIsomorphic(t *testing.T) {
	x, _ := Encode(sample())
	j, _ := EncodeJSONLD(sample())
	nt, _ := EncodeNTriples(sample())
	ttl, _ := EncodeTurtle(sample())

	graphs := map[string]*rdf.Graph{}
	for name, p := range map[string]func() (*rdf.Graph, error){
		"rdfxml":   func() (*rdf.Graph, error) { return rdf.ParseRDFXML(x) },
		"jsonld":   func() (*rdf.Graph, error) { return rdf.ParseJSONLD(j) },
		"ntriples": func() (*rdf.Graph, error) { return rdf.ParseNTriples(nt) },
		"turtle":   func() (*rdf.Graph, error) { return rdf.ParseTurtle(ttl) },
	} {
		g, err := p()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		graphs[name] = g
	}

	want := canonGraph(graphs["rdfxml"])
	for name, g := range graphs {
		if got := canonGraph(g); !reflect.DeepEqual(want, got) {
			t.Errorf("%s graph differs from RDF/XML:\n want %s\n got  %s",
				name, strings.Join(want, "\n  "), strings.Join(got, "\n  "))
		}
	}
}

// TestInstanceCarrierMedia confirms an Instance's RDA media/carrier (337/338) is
// rendered on the Instance node in every serialization, that the four encoders
// stay isomorphic on it, and that it round-trips back to 337/338.
func TestInstanceCarrierMedia(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nim a2200000 a 4500")).
		AddField(codex.NewControlField("001", "carrier1")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Recorded talk"))).
		AddField(codex.NewDataField("337", ' ', ' ', codex.NewSubfield('a', "audio"))).
		AddField(codex.NewDataField("338", ' ', ' ', codex.NewSubfield('a', "audio disc")))

	bib := FromRecord(rec)
	if len(bib.Instance.Media) != 1 || bib.Instance.Media[0].Label != "audio" ||
		len(bib.Instance.Carrier) != 1 || bib.Instance.Carrier[0].Label != "audio disc" {
		t.Fatalf("FromRecord media/carrier = %+v/%+v, want audio/audio disc", bib.Instance.Media, bib.Instance.Carrier)
	}

	x, _ := Encode(rec)
	j, _ := EncodeJSONLD(rec)
	nt, _ := EncodeNTriples(rec)
	ttl, _ := EncodeTurtle(rec)
	graphs := map[string]*rdf.Graph{}
	for name, p := range map[string]func() (*rdf.Graph, error){
		"rdfxml":   func() (*rdf.Graph, error) { return rdf.ParseRDFXML(x) },
		"jsonld":   func() (*rdf.Graph, error) { return rdf.ParseJSONLD(j) },
		"ntriples": func() (*rdf.Graph, error) { return rdf.ParseNTriples(nt) },
		"turtle":   func() (*rdf.Graph, error) { return rdf.ParseTurtle(ttl) },
	} {
		g, err := p()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		graphs[name] = g
	}
	want := canonGraph(graphs["rdfxml"])
	for name, g := range graphs {
		if got := canonGraph(g); !reflect.DeepEqual(want, got) {
			t.Errorf("%s graph differs from RDF/XML:\n want %s\n got  %s", name, strings.Join(want, "\n  "), strings.Join(got, "\n  "))
		}
	}
	if joined := strings.Join(want, "\n"); !strings.Contains(joined, bfNS+"media") || !strings.Contains(joined, bfNS+"carrier") {
		t.Errorf("graph missing bf:media/bf:carrier:\n%s", joined)
	}

	recs, err := Decode(x)
	if err != nil {
		t.Fatal(err)
	}
	if v := recs[0].SubfieldValue("337", 'a'); v != "audio" {
		t.Errorf("337$a = %q, want audio", v)
	}
	if v := recs[0].SubfieldValue("338", 'a'); v != "audio disc" {
		t.Errorf("338$a = %q, want 'audio disc'", v)
	}
}

// TestRoundTripNTriples and TestRoundTripTurtle exercise the new serializations
// through Encode -> Decode -> forward crosswalk.
func TestRoundTripNTriples(t *testing.T) {
	b, err := EncodeNTriples(sample())
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, b)
}

func TestRoundTripTurtle(t *testing.T) {
	b, err := EncodeTurtle(sample())
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, b)
}

// TestCollectionWritersNTTurtle checks the N-Triples and Turtle collection
// writers emit a document that decodes back to every record.
func TestCollectionWritersNTTurtle(t *testing.T) {
	second := sample().RemoveFields("001").AddField(codex.NewControlField("001", "99999999"))
	for _, tc := range []struct {
		name string
		mk   func(*bytes.Buffer) interface {
			Write(*codex.Record) error
			Close() error
		}
	}{
		{"ntriples", func(b *bytes.Buffer) interface {
			Write(*codex.Record) error
			Close() error
		} {
			return NewNTriplesWriter(b)
		}},
		{"turtle", func(b *bytes.Buffer) interface {
			Write(*codex.Record) error
			Close() error
		} {
			return NewTurtleWriter(b)
		}},
	} {
		var buf bytes.Buffer
		w := tc.mk(&buf)
		for _, r := range []*codex.Record{sample(), second} {
			if err := w.Write(r); err != nil {
				t.Fatalf("%s write: %v", tc.name, err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatalf("%s close: %v", tc.name, err)
		}
		recs, err := Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("%s decode: %v", tc.name, err)
		}
		if len(recs) != 2 {
			t.Errorf("%s: decoded %d records, want 2", tc.name, len(recs))
		}
	}
}

// canonGraph returns a label-independent canonical signature for every IRI
// subject in the graph: blank-node objects are expanded recursively so two
// isomorphic graphs compare equal regardless of blank ids or triple order. The
// graph is a forest of blank-node trees hanging off named (IRI) resources, so
// only IRI objects terminate the recursion — which also breaks the Work<->Instance
// reference cycle.
func canonGraph(g *rdf.Graph) []string {
	var sigs []string
	seen := map[string]bool{}
	for _, tr := range g.Triples {
		if tr.S.IsIRI() && !seen[tr.S.Value] {
			seen[tr.S.Value] = true
			sigs = append(sigs, sigOf(g, tr.S))
		}
	}
	sort.Strings(sigs)
	return sigs
}

func sigOf(g *rdf.Graph, s rdf.Term) string {
	var parts []string
	for _, tr := range g.Triples {
		if tr.S == s {
			parts = append(parts, tr.P.Value+"->"+objSig(g, tr.O))
		}
	}
	sort.Strings(parts)
	return termLabel(s) + "{" + strings.Join(parts, "|") + "}"
}

func objSig(g *rdf.Graph, o rdf.Term) string {
	if o.IsBlank() {
		return sigOf(g, o)
	}
	return termLabel(o)
}

func termLabel(t rdf.Term) string {
	switch {
	case t.IsIRI():
		return "<" + t.Value + ">"
	case t.IsBlank():
		return "_" // identity-independent
	default:
		if t.Lang != "" {
			return `"` + t.Value + `"@` + t.Lang
		}
		return `"` + t.Value + `"`
	}
}

// TestDecodeLoCContribution covers the contribution shape LoC's marc2bibframe2
// emits, which differs from this library's own output: the contribution is typed
// bf:PrimaryContribution (not bflc:), and the agent carries the generic bf:Agent
// type alongside the specific bf:Person/bf:Organization. The reader must still
// route the main entry to 1xx and pick the specific agent class.
func TestDecodeLoCContribution(t *testing.T) {
	const bf = "http://id.loc.gov/ontologies/bibframe/"
	doc := `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	            xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
	            xmlns:bf="` + bf + `">
	  <bf:Work rdf:about="http://ex/w">
	    <bf:title><bf:Title><bf:mainTitle>A study</bf:mainTitle></bf:Title></bf:title>
	    <bf:contribution><bf:Contribution>
	      <rdf:type rdf:resource="` + bf + `PrimaryContribution"/>
	      <bf:agent><bf:Agent rdf:about="http://id.loc.gov/rwo/agents/n1">
	        <rdf:type rdf:resource="` + bf + `Person"/>
	        <rdfs:label>Doe, Jane</rdfs:label></bf:Agent></bf:agent>
	      <bf:role><bf:Role><rdfs:label>author</rdfs:label></bf:Role></bf:role>
	    </bf:Contribution></bf:contribution>
	    <bf:contribution><bf:Contribution>
	      <bf:agent><bf:Agent>
	        <rdf:type rdf:resource="` + bf + `Organization"/>
	        <rdfs:label>Acme Corp</rdfs:label></bf:Agent></bf:agent>
	    </bf:Contribution></bf:contribution>
	  </bf:Work></rdf:RDF>`
	recs, err := Decode([]byte(doc))
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	rec := recs[0]
	if f := firstField(rec, "100"); f == nil || f.SubfieldValue('a') != "Doe, Jane" || f.SubfieldValue('e') != "author" {
		t.Errorf("primary person: want 100 $a 'Doe, Jane' $e 'author', got %+v", f)
	}
	if firstField(rec, "700") != nil {
		t.Error("primary contribution must not also appear as 700")
	}
	if f := firstField(rec, "710"); f == nil || f.SubfieldValue('a') != "Acme Corp" {
		t.Errorf("added organization: want 710 $a 'Acme Corp', got %+v", f)
	}
}

// TestDecodeLoCFields locks the field shapes LoC's marc2bibframe2 emits that
// differ from this library's own output: the language code in bf:code (not the
// rdfs:label human name), the transcribed publication statement in
// bflc:simplePlace/simpleAgent (the controlled bf:place being an authority form),
// and an LCCN as bf:Lccn (-> 010, not 024).
func TestDecodeLoCFields(t *testing.T) {
	const bf, bflc = "http://id.loc.gov/ontologies/bibframe/", "http://id.loc.gov/ontologies/bflc/"
	doc := `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	            xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#"
	            xmlns:bf="` + bf + `" xmlns:bflc="` + bflc + `">
	  <bf:Work rdf:about="http://ex/w">
	    <bf:title><bf:Title><bf:mainTitle>T</bf:mainTitle></bf:Title></bf:title>
	    <bf:language><bf:Language rdf:about="http://id.loc.gov/vocabulary/languages/fre">
	      <rdfs:label xml:lang="en">French</rdfs:label>
	      <bf:code rdf:datatype="http://www.w3.org/2001/XMLSchema#string">fre</bf:code>
	    </bf:Language></bf:language>
	    <bf:hasInstance rdf:resource="http://ex/i"/>
	  </bf:Work>
	  <bf:Instance rdf:about="http://ex/i">
	    <bf:instanceOf rdf:resource="http://ex/w"/>
	    <bf:title><bf:Title><bf:mainTitle>T</bf:mainTitle></bf:Title></bf:title>
	    <bf:provisionActivity><bf:ProvisionActivity>
	      <rdf:type rdf:resource="` + bf + `Publication"/>
	      <bf:date rdf:datatype="http://id.loc.gov/datatypes/edtf">1995</bf:date>
	      <bf:place><bf:Place rdf:about="http://id.loc.gov/vocabulary/countries/miu">
	        <rdfs:label>Michigan</rdfs:label></bf:Place></bf:place>
	      <bflc:simplePlace>Ann Arbor</bflc:simplePlace>
	      <bflc:simpleAgent>University of Michigan Press</bflc:simpleAgent>
	    </bf:ProvisionActivity></bf:provisionActivity>
	    <bf:identifiedBy><bf:Lccn><rdf:value>   94036501 </rdf:value></bf:Lccn></bf:identifiedBy>
	  </bf:Instance></rdf:RDF>`
	recs, err := Decode([]byte(doc))
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	rec := recs[0]
	if f := firstField(rec, "041"); f == nil || f.SubfieldValue('a') != "fre" {
		t.Errorf("language: want 041 $a 'fre' from bf:code, got %+v", f)
	}
	f := firstField(rec, "260")
	if f == nil || f.SubfieldValue('a') != "Ann Arbor" {
		t.Errorf("place: want 260 $a 'Ann Arbor' from bflc:simplePlace, got %+v", f)
	}
	if f != nil && f.SubfieldValue('b') != "University of Michigan Press" {
		t.Errorf("publisher: want 260 $b from bflc:simpleAgent, got %q", f.SubfieldValue('b'))
	}
	if f != nil && f.SubfieldValue('c') != "1995" {
		t.Errorf("date: want 260 $c '1995', got %q", f.SubfieldValue('c'))
	}
	if f := firstField(rec, "010"); f == nil || f.SubfieldValue('a') != "94036501" {
		t.Errorf("lccn: want 010 $a '94036501', got %+v", f)
	}
	if firstField(rec, "024") != nil {
		t.Error("LCCN must not also appear as 024")
	}
}

// firstField returns the first field with the tag, or nil.
func firstField(r *codex.Record, tag string) *codex.Field {
	for i := range r.Fields() {
		if r.Fields()[i].Tag == tag {
			return &r.Fields()[i]
		}
	}
	return nil
}

// FuzzDecode fuzzes the reader. Beyond never panicking, it asserts the crosswalk
// is a stable right inverse of the forward one: re-encoding a decoded record and
// decoding it again must reproduce the same BIBFRAME graph, for both
// serializations. It is seeded with this library's own output and the real LoC
// corpus so mutation explores realistic graphs.
func FuzzDecode(f *testing.F) {
	if b, err := Encode(sample()); err == nil {
		f.Add(b)
	}
	if j, err := EncodeJSONLD(sample()); err == nil {
		f.Add(j)
	}
	if paths, err := filepath.Glob(filepath.Join("testdata", "loc", "*")); err == nil {
		for _, p := range paths {
			if d, err := os.ReadFile(p); err == nil {
				f.Add(d)
			}
		}
	}
	f.Add([]byte("<rdf:RDF>"))
	f.Add([]byte("{}"))
	f.Fuzz(func(t *testing.T, data []byte) {
		recs, err := Decode(data)
		if err != nil {
			return
		}
		for _, rec := range recs {
			want := normalize(FromRecord(rec))
			for _, enc := range [][]byte{mustReencode(t, rec, false), mustReencode(t, rec, true)} {
				back, err := Decode(enc)
				if err != nil {
					t.Fatalf("re-decode of encoded record failed: %v", err)
				}
				if len(back) != 1 {
					t.Fatalf("re-decode produced %d records, want 1", len(back))
				}
				if got := normalize(FromRecord(back[0])); !reflect.DeepEqual(want, got) {
					t.Fatalf("crosswalk not stable under encode/decode:\n want %+v\n got  %+v", want, got)
				}
			}
		}
	})
}

// mustReencode serializes a record to RDF/XML (jsonld=false) or JSON-LD, failing
// the test on error.
func mustReencode(t *testing.T, rec *codex.Record, jsonld bool) []byte {
	t.Helper()
	var b []byte
	var err error
	if jsonld {
		b, err = EncodeJSONLD(rec)
	} else {
		b, err = Encode(rec)
	}
	if err != nil {
		t.Fatalf("re-encode failed: %v", err)
	}
	return b
}
