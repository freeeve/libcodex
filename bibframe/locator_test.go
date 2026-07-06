package bibframe

import (
	"testing"

	"github.com/freeeve/libcodex"
)

// TestLocatorFromRecord covers parsing an 856 into an ElectronicLocator: $u URL,
// $3 materials, $z note and $y link text (task 086).
func TestLocatorFromRecord(t *testing.T) {
	g := FromRecord(recordWith(
		codex.NewDataField("856", '4', ' ', codex.NewSubfield('3', "Image"),
			codex.NewSubfield('u', "https://img.example.org/cover.jpg"),
			codex.NewSubfield('z', "Large cover image")),
		codex.NewDataField("856", '4', '0', codex.NewSubfield('u', "https://example.org/read"),
			codex.NewSubfield('y', "Click to read"), codex.NewSubfield('z', "Access the title")),
	))
	if n := len(g.Instance.ElectronicLocator); n != 2 {
		t.Fatalf("locators = %d, want 2: %+v", n, g.Instance.ElectronicLocator)
	}
	cover := g.Instance.ElectronicLocator[0]
	if cover.URL != "https://img.example.org/cover.jpg" || cover.Materials != "Image" || cover.Note != "Large cover image" {
		t.Errorf("cover locator = %+v", cover)
	}
	read := g.Instance.ElectronicLocator[1]
	if read.URL != "https://example.org/read" || read.LinkText != "Click to read" || read.Note != "Access the title" {
		t.Errorf("read locator = %+v", read)
	}
}

// TestLocatorRoundTrip confirms all four 856 subfields survive Encode -> Decode
// through the bf:electronicLocator node shape (materials -> rdfs:label, note ->
// literal bf:note, link text -> a typed bf:note node).
func TestLocatorRoundTrip(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("856", '4', '0', codex.NewSubfield('u', "https://example.org/read"),
			codex.NewSubfield('3', "Excerpt"),
			codex.NewSubfield('z', "Sample chapter"),
			codex.NewSubfield('y', "Read a sample")),
	)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	f := firstField(recs[0], "856")
	if f == nil {
		t.Fatal("856 missing after round-trip")
	}
	for code, want := range map[byte]string{
		'u': "https://example.org/read",
		'3': "Excerpt",
		'z': "Sample chapter",
		'y': "Read a sample",
	} {
		if got := f.SubfieldValue(code); got != want {
			t.Errorf("856 $%c = %q, want %q", code, got, want)
		}
	}
}

// TestLocatorURLOnly confirms a bare 856 $u round-trips through the empty
// rdf:Description node without inventing subfields.
func TestLocatorURLOnly(t *testing.T) {
	rec := recordWith(
		codex.NewDataField("856", '4', '0', codex.NewSubfield('u', "https://example.org/item")),
	)
	encoded, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Decode(encoded)
	if err != nil || len(recs) != 1 {
		t.Fatalf("Decode: %v (%d records)", err, len(recs))
	}
	f := firstField(recs[0], "856")
	if f == nil || f.SubfieldValue('u') != "https://example.org/item" {
		t.Fatalf("856 $u round-trip; got %+v", f)
	}
	for _, code := range []byte{'3', 'z', 'y'} {
		if got := f.SubfieldValue(code); got != "" {
			t.Errorf("856 $%c = %q, want empty for a URL-only locator", code, got)
		}
	}
}
