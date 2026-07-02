package bibframe

import (
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
)

// TestLanguageNodeShape confirms a bf:Language node carries bf:code (not an
// rdfs:label=code) and that a 041 $h original language gets a bf:part role (task 068).
func TestLanguageNodeShape(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
		AddField(codex.NewDataField("041", '1', ' ', codex.NewSubfield('a', "eng"), codex.NewSubfield('h', "fre")))

	g := FromRecord(rec)
	if len(g.Work.Languages) != 1 || g.Work.Languages[0] != "eng" {
		t.Errorf("content languages = %+v, want [eng]", g.Work.Languages)
	}
	if len(g.Work.OriginalLangs) != 1 || g.Work.OriginalLangs[0] != "fre" {
		t.Errorf("original languages = %+v, want [fre]", g.Work.OriginalLangs)
	}

	b, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if strings.Contains(out, "<rdfs:label>eng</rdfs:label>") {
		t.Errorf("language node must not stamp rdfs:label=code:\n%s", out)
	}
	for _, want := range []string{
		`<bf:Language rdf:about="http://id.loc.gov/vocabulary/languages/eng">`,
		`<bf:code>eng</bf:code>`,
		`<bf:part>original</bf:part>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RDF/XML missing %q\n%s", want, out)
		}
	}
}

// TestLanguageRoundTrip confirms content and original (041 $h) languages survive
// Encode -> Decode in both serializations (task 068).
func TestLanguageRoundTrip(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
		AddField(codex.NewDataField("041", '1', ' ', codex.NewSubfield('a', "eng"),
			codex.NewSubfield('a', "spa"), codex.NewSubfield('h', "fre")))
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
			t.Fatalf("Decode (jsonld=%v): %v (%d)", jsonld, err, len(recs))
		}
		f := firstField(recs[0], "041")
		if f == nil {
			t.Fatalf("jsonld=%v: 041 missing", jsonld)
		}
		gotA := f.SubfieldValues('a')
		if len(gotA) != 2 || gotA[0] != "eng" || gotA[1] != "spa" {
			t.Errorf("jsonld=%v: 041 $a = %v, want [eng spa]", jsonld, gotA)
		}
		if h := f.SubfieldValue('h'); h != "fre" {
			t.Errorf("jsonld=%v: 041 $h = %q, want fre", jsonld, h)
		}
	}
}
