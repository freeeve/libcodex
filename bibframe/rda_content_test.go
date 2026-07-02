package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// TestContentFromField covers 336 $b -> Work content code, and TestContentFallback
// the leader/06 fallback (task 067).
func TestContentFromField(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("336", ' ', ' ', codex.NewSubfield('a', "performed music"),
			codex.NewSubfield('b', "prm"), codex.NewSubfield('2', "rdacontent")),
	))
	if g.Work.Content != "prm" {
		t.Errorf("336 $b content = %q, want prm", g.Work.Content)
	}
}

func TestContentFallback(t *testing.T) {
	// leader/06 = 'k' (still image) -> sti; no 336.
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nkm a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))
	if g := FromRecord(rec); g.Work.Content != "sti" {
		t.Errorf("leader/06 k content fallback = %q, want sti", g.Work.Content)
	}
}

// TestMediaCarrierRDA covers repeatable 337/338 with $b codes mapping to RDA IRIs
// (task 067).
func TestMediaCarrierRDA(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("337", ' ', ' ', codex.NewSubfield('a', "audio"), codex.NewSubfield('b', "s"), codex.NewSubfield('2', "rdamedia")),
		codex.NewDataField("337", ' ', ' ', codex.NewSubfield('a', "unmediated"), codex.NewSubfield('b', "n"), codex.NewSubfield('2', "rdamedia")),
		codex.NewDataField("338", ' ', ' ', codex.NewSubfield('a', "audio disc"), codex.NewSubfield('b', "sd"), codex.NewSubfield('2', "rdacarrier")),
	))
	if len(g.Instance.Media) != 2 || g.Instance.Media[0].Code != "s" || g.Instance.Media[1].Code != "n" {
		t.Errorf("337 media = %+v, want two terms s,n", g.Instance.Media)
	}
	if len(g.Instance.Carrier) != 1 || g.Instance.Carrier[0].Code != "sd" || g.Instance.Carrier[0].Label != "audio disc" {
		t.Errorf("338 carrier = %+v, want sd/audio disc", g.Instance.Carrier)
	}
}

// TestExtentDimensionsSplit covers 300 $a as extent and $c routed to dimensions,
// not swallowed into the extent label (task 067).
func TestExtentDimensionsSplit(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("300", ' ', ' ', codex.NewSubfield('a', "301 pages ;"), codex.NewSubfield('c', "22 cm")),
	))
	if len(g.Instance.Extent) != 1 || g.Instance.Extent[0] != "301 pages" {
		t.Errorf("extent = %+v, want [301 pages] (no dimensions)", g.Instance.Extent)
	}
	if len(g.Instance.Dimensions) != 1 || g.Instance.Dimensions[0] != "22 cm" {
		t.Errorf("dimensions = %+v, want [22 cm]", g.Instance.Dimensions)
	}
}

// TestRDARoundTrip confirms content, RDA media/carrier IRIs and the extent/dimension
// split survive Encode -> Decode in both serializations (task 067).
func TestRDARoundTrip(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nim a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T"))).
		AddField(codex.NewDataField("336", ' ', ' ', codex.NewSubfield('b', "spw"), codex.NewSubfield('2', "rdacontent"))).
		AddField(codex.NewDataField("337", ' ', ' ', codex.NewSubfield('b', "s"), codex.NewSubfield('2', "rdamedia"))).
		AddField(codex.NewDataField("338", ' ', ' ', codex.NewSubfield('b', "sd"), codex.NewSubfield('2', "rdacarrier"))).
		AddField(codex.NewDataField("300", ' ', ' ', codex.NewSubfield('a', "1 audio disc"), codex.NewSubfield('c', "12 cm")))
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
		got := recs[0]
		if f := firstField(got, "336"); f == nil || f.SubfieldValue('b') != "spw" {
			t.Errorf("jsonld=%v: 336 $b not round-tripped; got %+v", jsonld, f)
		}
		if f := firstField(got, "337"); f == nil || f.SubfieldValue('b') != "s" {
			t.Errorf("jsonld=%v: 337 $b not round-tripped; got %+v", jsonld, f)
		}
		if f := firstField(got, "338"); f == nil || f.SubfieldValue('b') != "sd" {
			t.Errorf("jsonld=%v: 338 $b not round-tripped; got %+v", jsonld, f)
		}
		if f := firstField(got, "300"); f == nil || f.SubfieldValue('a') != "1 audio disc" || f.SubfieldValue('c') != "12 cm" {
			t.Errorf("jsonld=%v: 300 $a/$c not round-tripped; got %+v", jsonld, f)
		}
	}
}
