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

// mkc008 composes a 40-byte 008 from the positions this package reads, so a test
// never has to hand-count filler spaces. An empty date, country or language leaves
// its positions blank.
func mkc008(date, country, lang string) string {
	b := []byte(strings.Repeat(" ", 40))
	copy(b[0:6], "920219")
	if date != "" {
		b[6] = 's'
		copy(b[7:11], date)
	}
	copy(b[15:18], country)
	copy(b[35:38], lang)
	return string(b)
}

// decoded008 round-trips a record through RDF/XML and returns the reconstructed
// 008, or "" when none was emitted.
func decoded008(t *testing.T, rec *codex.Record) string {
	t.Helper()
	b, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(b)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	for _, f := range recs[0].Fields() {
		if f.Tag == "008" {
			if len(f.Value) != 40 {
				t.Errorf("008 is %d bytes, want 40: %q", len(f.Value), f.Value)
			}
			return f.Value
		}
	}
	return ""
}

// at008 reads a slice of a reconstructed 008, "" when the field is absent.
func at008(c string, start, end int) string {
	if len(c) < end {
		return ""
	}
	return c[start:end]
}

// TestControl008Mirror is task 103: decode must return the publication date,
// country and language to the same 008 positions the forward crosswalk reads them
// from, not only the country.
func TestControl008Mirror(t *testing.T) {
	rec := bookWith008(mkc008("2010", "nyu", "eng"),
		codex.NewDataField("260", ' ', ' ', codex.NewSubfield('a', "Ashland"),
			codex.NewSubfield('b', "Blackstone,"), codex.NewSubfield('c', "2010.")))

	c := decoded008(t, rec)
	for _, tc := range []struct{ name, got, want string }{
		{"06 date type", at008(c, 6, 7), "s"},
		{"07-10 date 1", at008(c, 7, 11), "2010"},
		{"15-17 country", at008(c, 15, 18), "nyu"},
		{"35-37 language", at008(c, 35, 38), "eng"},
	} {
		if tc.got != tc.want {
			t.Errorf("008/%s = %q, want %q (full: %q)", tc.name, tc.got, tc.want, c)
		}
	}

	// The date legitimately lives in both places; the 260 keeps it.
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	var saw260c string
	for _, f := range recs[0].Fields() {
		if f.Tag == "260" {
			saw260c = f.SubfieldValue('c')
		}
	}
	if saw260c != "2010" {
		t.Errorf("260 $c = %q, want 2010 (the date stays transcribed too)", saw260c)
	}

	// And the forward crosswalk reads the reconstruction back to the same values.
	g := FromRecord(recs[0])
	if p := g.publicationProvision(); p == nil || p.Country != "nyu" || p.Date != "2010" {
		t.Errorf("re-encoded publication = %+v, want country nyu date 2010", p)
	}
	if len(g.Work.Languages) != 1 || g.Work.Languages[0] != "eng" {
		t.Errorf("re-encoded languages = %v, want [eng]", g.Work.Languages)
	}
}

// TestControl008BracketedDateIsAYear pins a boundary the naive reading gets wrong:
// FromRecord's cleanDate already strips brackets from a transcribed date, so a
// "[2010]" in the 260 $c reaches the graph as the bare year 2010 and mirroring it
// into 008/07-10 is a derivation, not a parse.
func TestControl008BracketedDateIsAYear(t *testing.T) {
	rec := bookWith008(mkc008("", "nyu", "eng"),
		codex.NewDataField("260", ' ', ' ', codex.NewSubfield('a', "X"), codex.NewSubfield('c', "[2010]")))
	c := decoded008(t, rec)
	if got := at008(c, 7, 11); got != "2010" {
		t.Errorf("008/07-10 = %q, want 2010 (cleanDate normalized the brackets)", got)
	}
	if got := at008(c, 6, 7); got != "s" {
		t.Errorf("008/06 = %q, want s", got)
	}
}

// TestControl008DateNotAYear covers the derive-don't-fabricate boundary: a
// provision date that is not a bare four-digit year is left to the 260 $c rather
// than parsed into the fixed field.
func TestControl008DateNotAYear(t *testing.T) {
	for _, date := range []string{"c2010", "2010-2012", "2010 printing", "201"} {
		rec := bookWith008(mkc008("", "nyu", "eng"),
			codex.NewDataField("260", ' ', ' ', codex.NewSubfield('a', "X"), codex.NewSubfield('c', date)))
		c := decoded008(t, rec)
		if got := at008(c, 7, 11); got != "    " {
			t.Errorf("date %q: 008/07-10 = %q, want blank", date, got)
		}
		if got := at008(c, 6, 7); got != " " {
			t.Errorf("date %q: 008/06 = %q, want blank", date, got)
		}
		// The country and language still mirror.
		if got := at008(c, 15, 18); got != "nyu" {
			t.Errorf("date %q: country = %q, want nyu", date, got)
		}
	}
}

// TestControl008AmbiguousYear covers two provisions naming different years: the
// reconstruction cannot say which one the 008 meant, so it asserts neither.
func TestControl008AmbiguousYear(t *testing.T) {
	disagree := bookWith008(mkc008("", "nyu", "eng"),
		codex.NewDataField("264", ' ', '1', codex.NewSubfield('a', "London"), codex.NewSubfield('c', "2001")),
		codex.NewDataField("264", ' ', '3', codex.NewSubfield('a', "Boston"), codex.NewSubfield('c', "2005")))
	if got := at008(decoded008(t, disagree), 7, 11); got != "    " {
		t.Errorf("disagreeing years: 008/07-10 = %q, want blank", got)
	}

	// Two provisions agreeing on one year are not ambiguous.
	agree := bookWith008(mkc008("", "nyu", "eng"),
		codex.NewDataField("264", ' ', '1', codex.NewSubfield('a', "London"), codex.NewSubfield('c', "2001")),
		codex.NewDataField("264", ' ', '3', codex.NewSubfield('a', "Boston"), codex.NewSubfield('c', "2001")))
	c := decoded008(t, agree)
	if got := at008(c, 7, 11); got != "2001" {
		t.Errorf("agreeing years: 008/07-10 = %q, want 2001", got)
	}
	if got := at008(c, 6, 7); got != "s" {
		t.Errorf("agreeing years: 008/06 = %q, want s", got)
	}
}

// TestControl008LanguageOfOriginal confirms 008/35-37 takes the content language,
// never the 041 $h language of the original, which the 008 slot does not hold.
func TestControl008LanguageOfOriginal(t *testing.T) {
	rec := bookWith008(mkc008("2010", "nyu", "eng"),
		codex.NewDataField("041", '1', ' ', codex.NewSubfield('a', "eng"), codex.NewSubfield('h', "ger")))
	if got := at008(decoded008(t, rec), 35, 38); got != "eng" {
		t.Errorf("008/35-37 = %q, want eng (not the language of the original)", got)
	}
}

// TestControl008Absent confirms nothing is fabricated: a graph naming no date,
// country or language yields no 008 at all.
func TestControl008Absent(t *testing.T) {
	rec := codex.NewRecord().
		SetLeader(codex.Leader("00000nam a2200000 a 4500")).
		AddField(codex.NewControlField("001", "x")).
		AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "T")))
	if c := decoded008(t, rec); c != "" {
		t.Errorf("008 = %q, want none", c)
	}
}

// TestControl008CountryOnly is the pre-existing behavior, preserved: a graph with
// only a country still reconstructs the country position.
func TestControl008CountryOnly(t *testing.T) {
	rec := bookWith008(mkc008("", "nyu", ""))
	c := decoded008(t, rec)
	if got := at008(c, 15, 18); got != "nyu" {
		t.Errorf("008/15-17 = %q, want nyu (full: %q)", got, c)
	}
	if got := at008(c, 7, 11); got != "    " {
		t.Errorf("008/07-10 = %q, want blank", got)
	}
}
