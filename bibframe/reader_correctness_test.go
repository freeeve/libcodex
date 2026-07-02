package bibframe

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// TestSourceQualifiedRoundTrip checks the reverse crosswalk restores the scheme
// ($2) that source-qualified identifiers (020/022/024) and classifications (072)
// carry, so Decode(Encode(r)) does not silently drop it. Every serialization is
// exercised.
func TestSourceQualifiedRoundTrip(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewControlField("001", "src-1")).
		AddField(codex.NewDataField("020", ' ', ' ', codex.NewSubfield('a', "0786803525"), codex.NewSubfield('2', "isbn"))).
		AddField(codex.NewDataField("024", '7', ' ', codex.NewSubfield('a', "10.1000/xyz"), codex.NewSubfield('2', "doi"))).
		AddField(codex.NewDataField("072", ' ', '7', codex.NewSubfield('a', "FIC000000"), codex.NewSubfield('2', "bisacsh"))).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))

	for _, tc := range []struct {
		name string
		enc  func(*codex.Record) ([]byte, error)
	}{
		{"rdfxml", Encode},
		{"jsonld", EncodeJSONLD},
		{"ntriples", EncodeNTriples},
		{"turtle", EncodeTurtle},
	} {
		b, err := tc.enc(rec)
		if err != nil {
			t.Fatalf("%s encode: %v", tc.name, err)
		}
		recs, err := Decode(b)
		if err != nil || len(recs) != 1 {
			t.Fatalf("%s decode: %v (%d records)", tc.name, err, len(recs))
		}
		got := recs[0]
		checkSub := func(tag string, code byte, want string) {
			f := firstField(got, tag)
			if f == nil {
				t.Errorf("%s: %s missing", tc.name, tag)
				return
			}
			if v := f.SubfieldValue(code); v != want {
				t.Errorf("%s: %s $%c = %q, want %q", tc.name, tag, code, v, want)
			}
		}
		checkSub("020", '2', "isbn")
		checkSub("024", '2', "doi")
		checkSub("072", 'a', "FIC000000")
		checkSub("072", '2', "bisacsh")

		if g, want := normalize(FromRecord(got)), normalize(FromRecord(rec)); !reflect.DeepEqual(g, want) {
			t.Errorf("%s: source-qualified round-trip differs:\n got %+v\nwant %+v", tc.name, g, want)
		}
	}
}

// TestStreamingNoControlNumberDisjoint checks that two records without a 001 get
// distinct Work/Instance IRIs and distinct provenance graphs across the streaming
// writers, rather than colliding on a shared "r0" base, and that decoding does
// not fabricate a shared 001.
func TestStreamingNoControlNumberDisjoint(t *testing.T) {
	mk := func(title string) *codex.Record {
		return codex.NewRecord().
			SetLeader(codex.Leader("00925cam a2200277 a 4500")).
			AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', title)))
	}
	recs := []*codex.Record{mk("First"), mk("Second")}

	for _, tc := range []struct {
		name string
		enc  func([]*codex.Record) []byte
	}{
		{"ntriples", func(rs []*codex.Record) []byte {
			var buf bytes.Buffer
			w := NewNTriplesWriter(&buf)
			for _, r := range rs {
				_ = w.Write(r)
			}
			_ = w.Close()
			return buf.Bytes()
		}},
		{"turtle", func(rs []*codex.Record) []byte {
			var buf bytes.Buffer
			w := NewTurtleWriter(&buf)
			for _, r := range rs {
				_ = w.Write(r)
			}
			_ = w.Close()
			return buf.Bytes()
		}},
	} {
		doc := tc.enc(recs)
		// The second record must use the r1 base -- had the writers regressed to a
		// fixed r0, only #r0Work would appear and both records would merge.
		if !bytes.Contains(doc, []byte("#r0Work")) || !bytes.Contains(doc, []byte("#r1Work")) {
			t.Errorf("%s: want distinct #r0Work and #r1Work bases:\n%s", tc.name, doc)
		}
		back, err := Decode(doc)
		if err != nil {
			t.Fatalf("%s decode: %v", tc.name, err)
		}
		if len(back) != 2 {
			t.Errorf("%s: decoded %d records, want 2", tc.name, len(back))
		}
		for i, r := range back {
			if id := r.ControlField("001"); id != "" {
				t.Errorf("%s: record %d fabricated 001 %q for a 001-less input", tc.name, i, id)
			}
		}
	}

	// N-Quads: RecordGraph must map the two records to distinct named graphs, and
	// each record's statements must land in its own graph.
	gA, gB := RecordGraph(recs[0], 0), RecordGraph(recs[1], 1)
	if gA == gB {
		t.Fatalf("RecordGraph mapped two 001-less records to the same graph %v", gA)
	}
	var buf bytes.Buffer
	w := NewNQuadsWriter(&buf, RecordGraph)
	for _, r := range recs {
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	ds, err := rdf.ParseNQuads(buf.Bytes())
	if err != nil {
		t.Fatalf("parse n-quads: %v", err)
	}
	if len(ds.Graph(gA).Triples) == 0 || len(ds.Graph(gB).Triples) == 0 {
		t.Errorf("n-quads: expected non-empty distinct graphs, got A=%d B=%d",
			len(ds.Graph(gA).Triples), len(ds.Graph(gB).Triples))
	}
}

// TestSniffFormat locks the serialization sniffing for inputs the earlier
// heuristic misclassified: a urn:-subject N-Triples line (no '/' in the subject
// IRI) and a Turtle document opening with a blank-node property list.
func TestSniffFormat(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want rdfFormat
	}{
		{"urn-ntriples", "<urn:isbn:123> <http://p/x> <http://o/y> .\n", formatNTriples},
		{"path-ntriples", "<http://s/x> <http://p/y> <http://o/z> .\n", formatNTriples},
		{"turtle-bnode", "[ a <http://ex/Work> ] .\n", formatTurtle},
		{"turtle-prefix", "@prefix ex: <http://e/> .\n", formatTurtle},
		{"jsonld-array", `[ { "@id": "x" } ]`, formatJSONLD},
		{"jsonld-object", `{ "@id": "x" }`, formatJSONLD},
		{"rdfxml-decl", `<?xml version="1.0"?><rdf:RDF/>`, formatRDFXML},
		{"rdfxml-root", `<rdf:RDF xmlns:bf="http://x/">`, formatRDFXML},
	}
	for _, c := range cases {
		if got := sniffFormat([]byte(c.in)); got != c.want {
			t.Errorf("%s: sniffFormat = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestDecodeURNSubjectNTriples covers the reader bug where N-Triples whose first
// subject IRI has no '/' (a urn:) was sniffed as RDF/XML and silently decoded to
// zero records.
func TestDecodeURNSubjectNTriples(t *testing.T) {
	const bf = "http://id.loc.gov/ontologies/bibframe/"
	doc := "<urn:x:w> <" + rdfNS + "type> <" + bf + "Work> .\n" +
		"<urn:x:w> <" + bf + "title> _:t .\n" +
		"_:t <" + rdfNS + "type> <" + bf + "Title> .\n" +
		"_:t <" + bf + "mainTitle> \"Urn Title\" .\n"
	recs, err := Decode([]byte(doc))
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	if f := firstField(recs[0], "245"); f == nil || f.SubfieldValue('a') != "Urn Title" {
		t.Errorf("245 = %+v, want $a 'Urn Title'", f)
	}
}

// TestDecodeTurtleBlankNodeStart covers the reader bug where a Turtle document
// opening with a blank-node property list ("[ ... ]") was sniffed as JSON-LD.
func TestDecodeTurtleBlankNodeStart(t *testing.T) {
	const bf = "http://id.loc.gov/ontologies/bibframe/"
	doc := "[ a <" + bf + "Work> ;\n" +
		"  <" + bf + "title> [ a <" + bf + "Title> ; <" + bf + "mainTitle> \"Bnode Title\" ] ] .\n"
	if got := sniffFormat([]byte(doc)); got != formatTurtle {
		t.Fatalf("sniffFormat = %d, want Turtle", got)
	}
	recs, err := Decode([]byte(doc))
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	if f := firstField(recs[0], "245"); f == nil || f.SubfieldValue('a') != "Bnode Title" {
		t.Errorf("245 = %+v, want $a 'Bnode Title'", f)
	}
}

// TestProvision264Copyright checks a 264 copyright statement (2nd indicator '4')
// does not populate the bf:Publication date, and that a 264 publication statement
// ('1') is preferred as the provision source.
func TestProvision264Copyright(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
		AddField(codex.NewDataField("264", ' ', '1',
			codex.NewSubfield('a', "London :"),
			codex.NewSubfield('b', "Verso,"),
			codex.NewSubfield('c', "2015"))).
		AddField(codex.NewDataField("264", ' ', '4', codex.NewSubfield('c', "©2015")))

	g := FromRecord(rec)
	if len(g.Instance.Provisions) != 1 {
		t.Fatalf("expected one provision, got %+v", g.Instance.Provisions)
	}
	prov := g.Instance.Provisions[0]
	if prov.Class != "Publication" || prov.Date != "2015" {
		t.Errorf("provision = %+v, want Publication date '2015' (from 264 _1, not the copyright)", prov)
	}
	if prov.Place != "London" {
		t.Errorf("provision place = %q, want 'London'", prov.Place)
	}
	if prov.Publisher != "Verso" {
		t.Errorf("provision publisher = %q, want 'Verso'", prov.Publisher)
	}
	if g.Instance.CopyrightDate == "" {
		t.Errorf("264 _4 copyright date not captured")
	}

	// A copyright statement alone must not become a publication date, but its
	// date is still captured as the copyright date.
	only := codex.NewRecord().
		SetLeader(codex.Leader("00925cam a2200277 a 4500")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
		AddField(codex.NewDataField("264", ' ', '4', codex.NewSubfield('c', "©2015")))
	og := FromRecord(only)
	for _, p := range og.Instance.Provisions {
		if p.Date != "" {
			t.Errorf("copyright-only 264 _4 leaked the copyright date as a provision date: %+v", p)
		}
	}
	if og.Instance.CopyrightDate == "" {
		t.Errorf("copyright-only 264 _4 should still capture the copyright date")
	}
}
