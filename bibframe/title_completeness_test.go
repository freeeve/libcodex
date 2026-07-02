package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// TestNonSortNumAndUniformParts covers 245 ind2 -> bflc:nonSortNum and 130/240 $n/$p
// -> uniform title part number/name (task 071).
func TestNonSortNumAndUniformParts(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("240", '1', '0', codex.NewSubfield('a', "Works"),
			codex.NewSubfield('n', "no. 1"), codex.NewSubfield('p', "Sonata")),
	))
	// The Work carries the uniform title with its part number/name.
	found := false
	for _, tt := range g.Work.Titles {
		if tt.Type == "uniform" && tt.PartNumber == "no. 1" && tt.PartName == "Sonata" {
			found = true
		}
	}
	if !found {
		t.Errorf("uniform title $n/$p not carried: %+v", g.Work.Titles)
	}

	// 245 ind2='4' -> nonSortNum "4" (skip "The ").
	g2 := FromRecord(recordWith2("245", '1', '4', codex.NewSubfield('a', "The Title")))
	if len(g2.Instance.Titles) != 1 || g2.Instance.Titles[0].NonSortNum != "4" {
		t.Errorf("245 ind2=4 nonSortNum = %+v, want 4", g2.Instance.Titles)
	}
}

// recordWith2 builds a record whose sole extra field is the given data field.
func recordWith2(tag string, ind1, ind2 byte, subs ...codex.Subfield) *codex.Record {
	return codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField(tag, ind1, ind2, subs...))
}

// TestVariantTitle covers 246 -> bf:VariantTitle / bf:ParallelTitle on the correct
// entity, and the 246 round-trip (task 071).
func TestVariantTitle(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "Main title")),
		codex.NewDataField("246", '1', '1', codex.NewSubfield('a', "Parallel title")), // parallel -> Work
		codex.NewDataField("246", '1', '4', codex.NewSubfield('a', "Cover title")),    // cover -> Instance
		codex.NewDataField("246", '1', '0', codex.NewSubfield('a', "Portion title")),  // variant -> Work
	)
	g := FromRecord(rec)
	if len(g.Work.VariantTitles) != 2 {
		t.Errorf("work variant titles = %+v, want 2 (parallel + portion)", g.Work.VariantTitles)
	}
	if len(g.Instance.VariantTitles) != 1 || g.Instance.VariantTitles[0].VariantType != "cover" {
		t.Errorf("instance variant titles = %+v, want 1 cover", g.Instance.VariantTitles)
	}
	var sawParallel bool
	for _, vt := range g.Work.VariantTitles {
		if vt.Parallel && vt.MainTitle == "Parallel title" {
			sawParallel = true
		}
	}
	if !sawParallel {
		t.Error("246 ind2=1 did not yield a parallel title")
	}

	// Round-trip: the three 246 fields come back with matching indicators.
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
		byInd2 := map[byte]string{}
		for _, f := range recs[0].Fields() {
			if f.Tag == "246" {
				byInd2[f.Ind2] = f.SubfieldValue('a')
			}
		}
		if byInd2['1'] != "Parallel title" || byInd2['4'] != "Cover title" || byInd2['0'] != "Portion title" {
			t.Errorf("jsonld=%v: 246 round-trip = %+v", jsonld, byInd2)
		}
	}
}
