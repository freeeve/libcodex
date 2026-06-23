package bibframe

import (
	"bytes"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/internal/rdf"
)

// normalize sorts every slice in a BIBFRAME so two graphs that carry the same
// statements in a different order compare equal. BIBFRAME is an unordered RDF
// graph, so order-independence is the right notion of equivalence for the
// reverse-crosswalk round-trip.
func normalize(g *BIBFRAME) *BIBFRAME {
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
	return p + "|" + c.Class + "|" + c.Label + "|" + c.Role
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

// TestEncodersIsomorphic confirms the RDF/XML and JSON-LD serializations of one
// record, run through their separate parsers, yield isomorphic RDF graphs — a
// cross-check of both encoders and both parsers at once, independent of blank
// node labelling and statement order.
func TestEncodersIsomorphic(t *testing.T) {
	x, _ := Encode(sample())
	j, _ := EncodeJSONLD(sample())
	gx, err := rdf.ParseRDFXML(x)
	if err != nil {
		t.Fatal(err)
	}
	gj, err := rdf.ParseJSONLD(j)
	if err != nil {
		t.Fatal(err)
	}
	cx, cj := canonGraph(gx), canonGraph(gj)
	if !reflect.DeepEqual(cx, cj) {
		t.Errorf("RDF/XML and JSON-LD graphs differ:\n RDF/XML: %s\n JSON-LD: %s",
			strings.Join(cx, "\n  "), strings.Join(cj, "\n  "))
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

// firstField returns the first field with the tag, or nil.
func firstField(r *codex.Record, tag string) *codex.Field {
	for i := range r.Fields() {
		if r.Fields()[i].Tag == tag {
			return &r.Fields()[i]
		}
	}
	return nil
}

// FuzzDecode asserts the reader never panics on arbitrary input.
func FuzzDecode(f *testing.F) {
	b, _ := Encode(sample())
	f.Add(b)
	j, _ := EncodeJSONLD(sample())
	f.Add(j)
	f.Add([]byte("<rdf:RDF>"))
	f.Add([]byte("{}"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Decode(data)
	})
}
