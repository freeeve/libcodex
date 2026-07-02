package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// TestSpecializedNotesForward covers the 081 note tags: routing (511/521 -> Work,
// 533/538 -> Instance) and the multi-subfield label join.
func TestSpecializedNotesForward(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("511", '0', ' ', codex.NewSubfield('a', "Read by the author.")),
		codex.NewDataField("521", ' ', ' ', codex.NewSubfield('a', "Ages 8-10.")),
		codex.NewDataField("533", ' ', ' ', codex.NewSubfield('a', "Electronic reproduction."),
			codex.NewSubfield('b', "Cleveland :"), codex.NewSubfield('c', "OverDrive, Inc.,"),
			codex.NewSubfield('n', "Mode of access: World Wide Web.")),
		codex.NewDataField("538", ' ', ' ', codex.NewSubfield('a', "Requires OverDrive Read.")),
	))
	workTypes := map[string]string{}
	for _, n := range g.Work.Notes {
		workTypes[n.Type] = n.Label
	}
	instTypes := map[string]string{}
	for _, n := range g.Instance.Notes {
		instTypes[n.Type] = n.Label
	}
	if workTypes["performers"] != "Read by the author." {
		t.Errorf("511 -> work performers note; got %+v", g.Work.Notes)
	}
	if workTypes["audience"] != "Ages 8-10." {
		t.Errorf("521 -> work audience note; got %+v", g.Work.Notes)
	}
	if want := "Electronic reproduction. Cleveland : OverDrive, Inc., Mode of access: World Wide Web."; instTypes["reproduction"] != want {
		t.Errorf("533 joined label = %q, want %q", instTypes["reproduction"], want)
	}
	if instTypes["systemDetails"] != "Requires OverDrive Read." {
		t.Errorf("538 -> instance systemDetails note; got %+v", g.Instance.Notes)
	}
}

// TestSpecializedNotesRoundTrip encodes the full extended note family and asserts
// each returns to its original tag on decode (the libcatalog loss-gate check).
func TestSpecializedNotesRoundTrip(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("511", '0', ' ', codex.NewSubfield('a', "Narrator: Jim Dale.")),
		codex.NewDataField("521", ' ', ' ', codex.NewSubfield('a', "Young adult.")),
		codex.NewDataField("533", ' ', ' ', codex.NewSubfield('a', "Electronic reproduction.")),
		codex.NewDataField("538", ' ', ' ', codex.NewSubfield('a', "Mode of access: internet.")),
	)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	got := recs[0]
	for tag, want := range map[string]string{
		"511": "Narrator: Jim Dale.",
		"521": "Young adult.",
		"533": "Electronic reproduction.",
		"538": "Mode of access: internet.",
	} {
		f := firstField(got, tag)
		if f == nil || f.SubfieldValue('a') != want {
			t.Errorf("%s not reconstructed; got %+v", tag, f)
		}
	}
}

// TestSeriesStatementRoundTrip covers 490 -> bf:seriesStatement -> 490, with and
// without a volume designation, and repeated series statements surviving the
// JSON-LD array form.
func TestSeriesStatementRoundTrip(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("490", '0', ' ', codex.NewSubfield('a', "Sally Lockhart mysteries ;"),
			codex.NewSubfield('v', "bk. 2")),
		codex.NewDataField("490", '1', ' ', codex.NewSubfield('a', "Firebrand fiction")),
	)
	g := FromRecord(rec)
	if len(g.Instance.SeriesStatements) != 2 {
		t.Fatalf("series statements = %+v, want 2", g.Instance.SeriesStatements)
	}
	if g.Instance.SeriesStatements[0] != "Sally Lockhart mysteries ; bk. 2" {
		t.Errorf("statement with volume = %q", g.Instance.SeriesStatements[0])
	}

	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	fields := countFields(recs[0], "490")
	if len(fields) != 2 {
		t.Fatalf("490 fields = %+v, want 2", fields)
	}
	var sawVolume, sawPlain bool
	for _, f := range fields {
		switch f.SubfieldValue('a') {
		case "Sally Lockhart mysteries":
			sawVolume = f.SubfieldValue('v') == "bk. 2"
		case "Firebrand fiction":
			sawPlain = f.SubfieldValue('v') == ""
		}
	}
	if !sawVolume || !sawPlain {
		t.Errorf("490 reconstruction: volume=%v plain=%v (%+v)", sawVolume, sawPlain, fields)
	}
}

// TestRelationISBN covers the OverDrive-style 776 that carries only an ISBN in
// $z (no $t/$a/$x), which was dropped before 081.
func TestRelationISBN(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("776", '0', '8', codex.NewSubfield('c', "Original"),
			codex.NewSubfield('z', "9780786803521")),
	)
	g := FromRecord(rec)
	if len(g.Work.Relations) != 1 || g.Work.Relations[0].ISBN != "9780786803521" {
		t.Fatalf("relations = %+v, want one with ISBN", g.Work.Relations)
	}

	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	f := firstField(recs[0], "776")
	if f == nil || f.SubfieldValue('z') != "9780786803521" {
		t.Errorf("776 $z not reconstructed; got %+v", f)
	}
}

// TestCodedFieldsRoundTrip covers 006/007 folding into media/carrier terms and
// their reconstruction on decode (task 082: sound/computer/video categories).
func TestCodedFieldsRoundTrip(t *testing.T) {
	// An electronic-resource shape with no 337/338: the coded fields alone must
	// produce the carrier and media, and decode must rebuild both plus RDA fields.
	rec := recordWith(
		codex.NewControlField("006", "m        d        "),
		codex.NewControlField("007", "cr"),
	)
	g := FromRecord(rec)
	if len(g.Instance.Carrier) != 1 || g.Instance.Carrier[0].Code != "cr" {
		t.Fatalf("carrier from 007 = %+v", g.Instance.Carrier)
	}
	if len(g.Instance.Media) != 1 || g.Instance.Media[0].Code != "c" {
		t.Fatalf("media from 006 = %+v", g.Instance.Media)
	}

	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	got := recs[0]
	if f := firstField(got, "007"); f == nil || f.Value != "cr" {
		t.Errorf("007 = %+v, want cr", f)
	}
	if f := firstField(got, "006"); f == nil || len(f.Value) != 18 || f.Value[0] != 'm' {
		t.Errorf("006 = %+v, want m + fill", f)
	}

	// Explicit 337/338 win: a 007 whose carrier is already present adds nothing.
	g2 := FromRecord(recordWith(
		codex.NewControlField("007", "sd"),
		codex.NewDataField("338", ' ', ' ', codex.NewSubfield('a', "audio disc"),
			codex.NewSubfield('b', "sd"), codex.NewSubfield('2', "rdacarrier")),
	))
	if len(g2.Instance.Carrier) != 1 {
		t.Errorf("carrier deduplication failed: %+v", g2.Instance.Carrier)
	}

	// A computer-file record (leader/06 m) must not gain a redundant 006.
	sw := recordWith(codex.NewDataField("337", ' ', ' ', codex.NewSubfield('b', "c"),
		codex.NewSubfield('2', "rdamedia")))
	sw.SetLeader(codex.Leader("00000nmm a2200000 a 4500"))
	b2, err := Encode(sw)
	if err != nil {
		t.Fatal(err)
	}
	recs2, err := Decode(b2)
	if err != nil || len(recs2) != 1 {
		t.Fatalf("Decode: %v", err)
	}
	if f := firstField(recs2[0], "006"); f != nil {
		t.Errorf("computer-file record gained a redundant 006: %+v", f)
	}
}

// TestDurationAndDigitalCharacteristics covers 306 -> bf:duration and 347 ->
// bf:digitalCharacteristic (FileType/EncodingFormat) both ways.
func TestDurationAndDigitalCharacteristics(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("306", ' ', ' ', codex.NewSubfield('a', "013000"),
			codex.NewSubfield('a', "011500")),
		codex.NewDataField("347", ' ', ' ', codex.NewSubfield('a', "text file"),
			codex.NewSubfield('b', "EPUB"), codex.NewSubfield('2', "rda")),
	)
	g := FromRecord(rec)
	if len(g.Instance.Duration) != 2 || g.Instance.Duration[0] != "013000" {
		t.Fatalf("duration = %+v", g.Instance.Duration)
	}
	if len(g.Instance.DigitalCharacteristics) != 2 ||
		g.Instance.DigitalCharacteristics[0] != (DigitalCharacteristic{Class: "FileType", Label: "text file"}) ||
		g.Instance.DigitalCharacteristics[1] != (DigitalCharacteristic{Class: "EncodingFormat", Label: "EPUB"}) {
		t.Fatalf("digital characteristics = %+v", g.Instance.DigitalCharacteristics)
	}

	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	got := recs[0]
	if f := firstField(got, "306"); f == nil || len(f.Subfields) != 2 || f.SubfieldValue('a') != "013000" {
		t.Errorf("306 not reconstructed; got %+v", f)
	}
	if f := firstField(got, "347"); f == nil || f.SubfieldValue('a') != "text file" || f.SubfieldValue('b') != "EPUB" {
		t.Errorf("347 not reconstructed; got %+v", f)
	}
}
