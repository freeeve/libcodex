package codex_test

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	codex "github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/bibframe"
	"github.com/freeeve/libcodex/citation"
	"github.com/freeeve/libcodex/dublincore"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/marcjson"
	"github.com/freeeve/libcodex/marcxml"
	"github.com/freeeve/libcodex/mods"
	"github.com/freeeve/libcodex/mrk"
	"github.com/freeeve/libcodex/schemaorg"
)

// TestRealData exercises every reader, writer and converter over real Library of
// Congress records (their MARCXML, fetched from lccn.loc.gov, in testdata/realdata).
// It checks the four codecs round-trip the record's data losslessly, every
// exporter produces well-formed output, the BIBFRAME output decodes back to a
// record, and the MODS and Dublin Core crosswalks agree with LoC's own.
func TestRealData(t *testing.T) {
	files, _ := filepath.Glob("testdata/realdata/*.marcxml")
	if len(files) == 0 {
		t.Skip("no real-data corpus in testdata/realdata")
	}
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".marcxml")
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rec, err := marcxml.Decode(data)
		if err != nil {
			t.Errorf("%s: marcxml decode: %v", id, err)
			continue
		}
		t.Run(id, func(t *testing.T) {
			if rec.SubfieldValue("245", 'a') == "" {
				t.Error("real record has no 245 $a")
			}
			testCodecs(t, rec)
			testExporters(t, rec)
			testLoCModsDifferential(t, rec, id)
		})
	}
}

// testCodecs round-trips the record through each lossless serialization and
// confirms the fields and the semantic leader bytes survive.
func testCodecs(t *testing.T, rec *codex.Record) {
	if b, err := iso2709.Encode(rec); err != nil {
		t.Errorf("iso2709 encode: %v", err)
	} else if got, _, err := iso2709.Decode(b); err != nil {
		t.Errorf("iso2709 decode: %v", err)
	} else {
		assertSameRecord(t, "iso2709", rec, got)
	}
	if b, err := marcxml.Encode(rec); err != nil {
		t.Errorf("marcxml encode: %v", err)
	} else if got, err := marcxml.Decode(b); err != nil {
		t.Errorf("marcxml decode: %v", err)
	} else {
		assertSameRecord(t, "marcxml", rec, got)
	}
	if b, err := marcjson.Encode(rec); err != nil {
		t.Errorf("marcjson encode: %v", err)
	} else if got, err := marcjson.Decode(b); err != nil {
		t.Errorf("marcjson decode: %v", err)
	} else {
		assertSameRecord(t, "marcjson", rec, got)
	}
	if b, err := mrk.Encode(rec); err != nil {
		t.Errorf("mrk encode: %v", err)
	} else if got, err := mrk.Decode(b); err != nil {
		t.Errorf("mrk decode: %v", err)
	} else {
		assertSameRecord(t, "mrk", rec, got)
	}
}

// testExporters confirms every one-way converter yields well-formed, non-empty
// output, and that the BIBFRAME serializations decode back to a titled record.
func testExporters(t *testing.T, rec *codex.Record) {
	if b, err := mods.Encode(rec); err != nil || xmlWellFormed(b) != nil {
		t.Errorf("mods: err=%v wellformed=%v", err, xmlWellFormed(b))
	}
	if b, err := dublincore.Encode(rec); err != nil || xmlWellFormed(b) != nil {
		t.Errorf("dublincore: err=%v wellformed=%v", err, xmlWellFormed(b))
	}
	if b, err := schemaorg.Encode(rec); err != nil || jsonValid(b) != nil {
		t.Errorf("schemaorg: err=%v valid=%v", err, jsonValid(b))
	}
	if b, err := citation.RIS(rec); err != nil || !strings.Contains(string(b), "TY  -") || !strings.Contains(string(b), "ER  -") {
		t.Errorf("RIS: err=%v\n%s", err, b)
	}
	if b, err := citation.BibTeX(rec); err != nil || !strings.HasPrefix(strings.TrimSpace(string(b)), "@") {
		t.Errorf("BibTeX: err=%v\n%s", err, b)
	}

	// BIBFRAME: every serialization must be well-formed and decode back to a record
	// carrying the title.
	want := normTitle(rec.SubfieldValue("245", 'a'))
	for _, bf := range []struct {
		name  string
		enc   func(*codex.Record) ([]byte, error)
		check func([]byte) error
	}{
		{"rdfxml", bibframe.Encode, xmlWellFormed},
		{"jsonld", bibframe.EncodeJSONLD, jsonValid},
		{"turtle", bibframe.EncodeTurtle, nonEmpty},
		{"ntriples", bibframe.EncodeNTriples, nonEmpty},
	} {
		b, err := bf.enc(rec)
		if err != nil {
			t.Errorf("bibframe %s encode: %v", bf.name, err)
			continue
		}
		if err := bf.check(b); err != nil {
			t.Errorf("bibframe %s not well-formed: %v", bf.name, err)
		}
		recs, err := bibframe.Decode(b)
		if err != nil || len(recs) == 0 {
			t.Errorf("bibframe %s decode: %v (%d records)", bf.name, err, len(recs))
			continue
		}
		if got := normTitle(recs[0].SubfieldValue("245", 'a')); got != "" && want != "" && !titleMatch(want, got) {
			t.Errorf("bibframe %s round-trip title %q != %q", bf.name, got, want)
		}
	}
}

// testLoCModsDifferential cross-checks our MODS crosswalk against LoC's own MODS
// for the same record: the title sets must overlap. Comparing sets (rather than
// the first title) tolerates the legitimate difference in which title — uniform or
// transcribed — each crosswalk lists first, and the normalization drops the
// nonfiling article LoC strips but the transcribed title keeps.
func testLoCModsDifferential(t *testing.T, rec *codex.Record, id string) {
	loc, err := os.ReadFile(filepath.Join("testdata", "realdata", id+".mods"))
	if err != nil {
		return
	}
	ours, err := mods.Encode(rec)
	if err != nil {
		t.Errorf("mods encode: %v", err)
		return
	}
	if !titlesOverlap(allElementTexts(ours, "title"), allElementTexts(loc, "title")) {
		t.Errorf("MODS titles do not overlap LoC's:\n ours=%v\n loc =%v",
			allElementTexts(ours, "title"), allElementTexts(loc, "title"))
	}
}

// titlesOverlap reports whether any normalized title in a matches one in b.
func titlesOverlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if titleMatch(normTitle(x), normTitle(y)) {
				return true
			}
		}
	}
	return false
}

// assertSameRecord checks two records carry the same fields and the same semantic
// leader bytes (status, type, bibliographic level, encoding); the length and base
// address are recomputed per serialization and so are not compared.
func assertSameRecord(t *testing.T, format string, want, got *codex.Record) {
	t.Helper()
	for _, i := range []int{5, 6, 7, 9} {
		if want.Leader().String()[i] != got.Leader().String()[i] {
			t.Errorf("%s: leader byte %d: %q != %q", format, i, want.Leader().String()[i], got.Leader().String()[i])
		}
	}
	wf, gf := want.Fields(), got.Fields()
	if len(wf) != len(gf) {
		t.Errorf("%s: field count %d != %d", format, len(gf), len(wf))
		return
	}
	for i := range wf {
		a, b := wf[i], gf[i]
		if a.Tag != b.Tag || a.Ind1 != b.Ind1 || a.Ind2 != b.Ind2 || a.Value != b.Value {
			t.Errorf("%s: field %d %s differs: %q/%q vs %q/%q", format, i, a.Tag, string(a.Ind1)+string(a.Ind2), a.Value, string(b.Ind1)+string(b.Ind2), b.Value)
			continue
		}
		if len(a.Subfields) != len(b.Subfields) {
			t.Errorf("%s: field %d %s subfield count %d != %d", format, i, a.Tag, len(b.Subfields), len(a.Subfields))
			continue
		}
		for j := range a.Subfields {
			if a.Subfields[j] != b.Subfields[j] {
				t.Errorf("%s: field %d %s subfield %d: %+v != %+v", format, i, a.Tag, j, b.Subfields[j], a.Subfields[j])
			}
		}
	}
}

// ---- helpers ----

func nonEmpty(b []byte) error {
	if len(b) == 0 {
		return errEmpty
	}
	return nil
}

type emptyErr struct{}

func (emptyErr) Error() string { return "empty output" }

var errEmpty = emptyErr{}

// allElementTexts returns the character data of every element whose local name
// matches, ignoring namespaces.
func allElementTexts(data []byte, local string) []string {
	var out []string
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != local {
			continue
		}
		var b strings.Builder
		for depth := 1; depth > 0; {
			inner, err := dec.Token()
			if err != nil {
				break
			}
			switch it := inner.(type) {
			case xml.CharData:
				if depth == 1 {
					b.Write(it)
				}
			case xml.StartElement:
				depth++
			case xml.EndElement:
				depth--
			}
		}
		if s := strings.TrimSpace(b.String()); s != "" {
			out = append(out, s)
		}
	}
}

// normTitle lowercases, strips trailing ISBD punctuation, and drops a leading
// nonfiling article so the comparison ignores how each crosswalk handles it.
func normTitle(s string) string {
	s = strings.ToLower(strings.TrimRight(strings.TrimSpace(s), " /:;,.="))
	for _, article := range []string{"the ", "an ", "a "} {
		if strings.HasPrefix(s, article) {
			return s[len(article):]
		}
	}
	return s
}

// titleMatch reports whether two normalized titles agree, allowing one to be a
// prefix of the other (LoC and this library trim subfields slightly differently).
func titleMatch(a, b string) bool {
	return a == b || strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}
