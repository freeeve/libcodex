package bibframe

import (
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
)

// bookWith008 builds a record with the given 008 and extra fields.
func bookWith008(c008 string, fields ...codex.Field) *codex.Record {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewControlField("008", c008)).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))
	for _, f := range fields {
		rec.AddField(f)
	}
	return rec
}

// TestProvisionSubclassPerField covers one provision node per 26X typed by 264 ind2,
// with 264 _4 -> copyright date and the 008 country as a controlled place (task 066).
func TestProvisionSubclassPerField(t *testing.T) {
	// 008/15-17 = "enk" (England).
	c008 := "701231s2001    enk           000 0 eng d"
	g := FromRecord(bookWith008(c008,
		codex.NewDataField("264", ' ', '0', codex.NewSubfield('a', "Studio City"), codex.NewSubfield('b', "Maker")),
		codex.NewDataField("264", ' ', '1', codex.NewSubfield('a', "London"), codex.NewSubfield('b', "Verso"), codex.NewSubfield('c', "2001")),
		codex.NewDataField("264", ' ', '2', codex.NewSubfield('a', "Boston"), codex.NewSubfield('b', "Distributor")),
		codex.NewDataField("264", ' ', '3', codex.NewSubfield('a', "Ann Arbor"), codex.NewSubfield('b', "Printer")),
		codex.NewDataField("264", ' ', '4', codex.NewSubfield('c', "2001")),
	))
	classes := make(map[string]Provision)
	for _, p := range g.Instance.Provisions {
		classes[p.Class] = p
	}
	for _, want := range []string{"Production", "Publication", "Distribution", "Manufacture"} {
		if _, ok := classes[want]; !ok {
			t.Errorf("missing provision class %q; got %+v", want, g.Instance.Provisions)
		}
	}
	if p := classes["Publication"]; p.Date != "2001" || p.Country != "enk" {
		t.Errorf("publication node = %+v, want date 2001 + country enk", p)
	}
	if g.Instance.CopyrightDate != "2001" {
		t.Errorf("264 _4 copyright date = %q, want 2001", g.Instance.CopyrightDate)
	}
}

// TestProvisionSimpleAndCountry covers the forward emit: transcribed place/agent go
// to bflc:simple*, and the 008 country becomes a controlled bf:place IRI (task 066).
func TestProvisionSimpleAndCountry(t *testing.T) {
	c008 := "701231s2001    enk           000 0 eng d"
	b, err := Encode(bookWith008(c008,
		codex.NewDataField("264", ' ', '1', codex.NewSubfield('a', "London"), codex.NewSubfield('b', "Verso"), codex.NewSubfield('c', "2001"))))
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, want := range []string{
		`<bflc:simplePlace>London</bflc:simplePlace>`,
		`<bflc:simpleAgent>Verso</bflc:simpleAgent>`,
		`<bf:Place rdf:about="http://id.loc.gov/vocabulary/countries/enk">`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RDF/XML missing %q\n%s", want, out)
		}
	}
}

// TestProvisionRoundTrip confirms multiple provisions, the copyright date and the
// 008 country survive Encode -> Decode (task 066).
func TestProvisionRoundTrip(t *testing.T) {
	c008 := "701231s2001    enk           000 0 eng d"
	rec := bookWith008(c008,
		codex.NewDataField("264", ' ', '1', codex.NewSubfield('a', "London"), codex.NewSubfield('b', "Verso"), codex.NewSubfield('c', "2001")),
		codex.NewDataField("264", ' ', '2', codex.NewSubfield('a', "Boston"), codex.NewSubfield('b', "Distributor")),
		codex.NewDataField("264", ' ', '4', codex.NewSubfield('c', "2001")),
	)
	for _, jsonld := range []bool{false, true} {
		var b []byte
		var err error
		if jsonld {
			b, err = EncodeJSONLD(rec)
		} else {
			b, err = Encode(rec)
		}
		if err != nil {
			t.Fatal(err)
		}
		recs, err := Decode(b)
		if err != nil || len(recs) != 1 {
			t.Fatalf("Decode (jsonld=%v): %v (%d records)", jsonld, err, len(recs))
		}
		g := FromRecord(recs[0])
		if len(g.Instance.Provisions) != 2 {
			t.Errorf("jsonld=%v: provisions = %+v, want 2", jsonld, g.Instance.Provisions)
		}
		if p := g.publicationProvision(); p == nil || p.Country != "enk" || p.Date != "2001" {
			t.Errorf("jsonld=%v: publication = %+v, want country enk date 2001", jsonld, p)
		}
		if g.Instance.CopyrightDate != "2001" {
			t.Errorf("jsonld=%v: copyright date = %q, want 2001", jsonld, g.Instance.CopyrightDate)
		}
	}
}
