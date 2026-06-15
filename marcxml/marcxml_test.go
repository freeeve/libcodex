package marcxml

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/iso2709"
)

// sample builds a record exercising control fields, repeated data fields, a
// blank indicator, a trailing-space value, XML special characters and UTF-8.
func sample() *codex.Record {
	return codex.NewRecord().
		AddField(codex.NewControlField("001", "ocm12345")).
		AddField(codex.NewControlField("008", "210101s2021    nyu")).
		AddField(codex.NewDataField("245", '1', '0',
			codex.NewSubfield('a', "Stone butch blues :"),
			codex.NewSubfield('b', "a novel /"),
			codex.NewSubfield('c', "Leslie Feinberg."))).
		AddField(codex.NewDataField("520", ' ', ' ',
			codex.NewSubfield('a', `Tom & Jerry <best> "friends"`))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Café—Lesbians"))).
		AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Gender identity")))
}

func readAll(t *testing.T, r *Reader) []*codex.Record {
	t.Helper()
	var out []*codex.Record
	for rec, err := range r.All() {
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		out = append(out, rec)
	}
	return out
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	rec := sample()
	b, err := Encode(rec)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(rec, got) {
		t.Errorf("round trip mismatch:\n in  = %#v\n out = %#v", rec, got)
	}
}

func TestEncodeEscapesAndNamespace(t *testing.T) {
	b, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, `xmlns="`+Namespace+`"`) {
		t.Error("standalone record missing namespace declaration")
	}
	if !strings.Contains(out, "Tom &amp; Jerry &lt;best&gt;") {
		t.Errorf("XML special characters not escaped:\n%s", out)
	}
	if !strings.Contains(out, `ind1=" " ind2=" "`) {
		t.Error("blank indicators not emitted as spaces")
	}
}

func TestCrossFormatISO2709(t *testing.T) {
	// An iso2709 record must survive a marcxml round trip unchanged.
	mrc, err := iso2709.Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	fromMRC, _, err := iso2709.Decode(mrc)
	if err != nil {
		t.Fatal(err)
	}
	xmlb, err := Encode(fromMRC)
	if err != nil {
		t.Fatal(err)
	}
	fromXML, err := Decode(xmlb)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromMRC, fromXML) {
		t.Errorf("iso2709 record not preserved through marcxml:\n mrc = %#v\n xml = %#v", fromMRC, fromXML)
	}
	// And re-encoding to iso2709 is byte-stable.
	back, err := iso2709.Encode(fromXML)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mrc, back) {
		t.Error("iso2709 -> marcxml -> iso2709 is not byte-stable")
	}
}

func TestWriterCollectionRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(sample()); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(codex.NewRecord().AddField(codex.NewControlField("001", "two"))); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, xmlHeader) {
		t.Error("missing XML declaration")
	}
	if !strings.Contains(out, collectionOpen) || !strings.Contains(out, collectionClose) {
		t.Error("missing collection wrapper")
	}

	recs := readAll(t, NewReader(strings.NewReader(out)))
	if len(recs) != 2 {
		t.Fatalf("read %d records, want 2", len(recs))
	}
	if got := recs[0].SubfieldValue("245", 'a'); got != "Stone butch blues :" {
		t.Errorf("record 0 245a = %q", got)
	}
	if got := recs[1].ControlField("001"); got != "two" {
		t.Errorf("record 1 001 = %q, want two", got)
	}
}

func TestEncodeRejectsInvalidXML(t *testing.T) {
	// XML 1.0 cannot carry control characters (e.g. NUL); Encode must reject
	// rather than emit invalid XML.
	cases := map[string]*codex.Record{
		"control value":  codex.NewRecord().AddField(codex.NewControlField("001", "a\x00b")),
		"subfield value": codex.NewRecord().AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "x\x0by"))),
		"subfield code":  codex.NewRecord().AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield(0x00, "x"))),
		"leader":         codex.NewRecord().SetLeader(codex.Leader("00000nam a2200000 \x00 4500")).AddField(codex.NewControlField("001", "x")),
	}
	for name, rec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Encode(rec); err == nil {
				t.Error("expected error for an XML-invalid character")
			}
			if err := NewWriter(&bytes.Buffer{}).Write(rec); err == nil {
				t.Error("Writer.Write: expected error for an XML-invalid character")
			}
		})
	}
}

func TestWriteAfterCloseFails(t *testing.T) {
	w := NewWriter(&bytes.Buffer{})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(sample()); err == nil {
		t.Error("expected error writing after Close")
	}
}

func TestDecodeNamespaceVariants(t *testing.T) {
	cases := map[string]struct {
		xml      string
		tag      string
		code     byte
		wantData string
		wantCtrl string
	}{
		"namespaced bare record": {
			xml:      `<record xmlns="` + Namespace + `"><leader>00000nam a2200000   4500</leader><datafield tag="245" ind1="1" ind2="0"><subfield code="a">T</subfield></datafield></record>`,
			tag:      "245",
			code:     'a',
			wantData: "T",
		},
		"no namespace bare record": {
			xml:      `<record><controlfield tag="001">x</controlfield></record>`,
			wantCtrl: "x",
		},
		"prefixed collection": {
			xml:      `<marc:collection xmlns:marc="` + Namespace + `"><marc:record><marc:controlfield tag="001">y</marc:controlfield></marc:record></marc:collection>`,
			wantCtrl: "y",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rec, err := Decode([]byte(c.xml))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if c.wantData != "" {
				if got := rec.SubfieldValue(c.tag, c.code); got != c.wantData {
					t.Errorf("%s$%c = %q, want %q", c.tag, c.code, got, c.wantData)
				}
			}
			if c.wantCtrl != "" {
				if got := rec.ControlField("001"); got != c.wantCtrl {
					t.Errorf("001 = %q, want %q", got, c.wantCtrl)
				}
			}
		})
	}
}

func TestDecodeIgnoresUnknownElements(t *testing.T) {
	// Unknown elements at the record and datafield levels are skipped.
	x := `<record>
		<note>ignore me</note>
		<leader>00000nam a2200000   4500</leader>
		<controlfield tag="001">x</controlfield>
		<datafield tag="245" ind1="1" ind2="0">
			<extra>skip</extra>
			<subfield code="a">Title</subfield>
		</datafield>
	</record>`
	rec, err := Decode([]byte(x))
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.ControlField("001"); got != "x" {
		t.Errorf("001 = %q", got)
	}
	if got := rec.SubfieldValue("245", 'a'); got != "Title" {
		t.Errorf("245a = %q", got)
	}
}

func TestDecodeLeafWithChildElement(t *testing.T) {
	// A child element inside a leaf (subfield) is skipped; surrounding text joins.
	rec, err := Decode([]byte(`<record><datafield tag="245" ind1="0" ind2="0"><subfield code="a">a<b>x</b>c</subfield></datafield></record>`))
	if err != nil {
		t.Fatal(err)
	}
	if v := rec.SubfieldValue("245", 'a'); v != "ac" {
		t.Errorf("245a = %q, want ac", v)
	}
}

func TestDecodeTruncated(t *testing.T) {
	// A record truncated before </record> ends the stream cleanly; Decode then
	// reports that no complete record was found.
	if _, err := Decode([]byte(`<collection><record><controlfield tag="001">x</controlfield>`)); err == nil {
		t.Error("expected error for truncated record")
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := Decode([]byte(`<collection></collection>`)); err == nil {
		t.Error("expected error for collection with no record")
	}
	if _, err := Decode([]byte(`<record><leader>x</leader`)); err == nil {
		t.Error("expected error for malformed XML")
	}
}

func TestReadWriteFile(t *testing.T) {
	recs := []*codex.Record{
		sample(),
		codex.NewRecord().AddField(codex.NewControlField("001", "x")),
	}
	path := filepath.Join(t.TempDir(), "c.xml")
	if err := WriteFile(path, recs); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d records, want 2", len(got))
	}
	if v := got[0].SubfieldValue("650", 'a'); v != "Café—Lesbians" {
		t.Errorf("UTF-8 not preserved: %q", v)
	}

	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing.xml")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestEncodeDecodeHelpers(t *testing.T) {
	if got := string(appendInd(nil, 0)); got != " " {
		t.Errorf("appendInd(0) = %q, want space", got)
	}
	if got := string(appendInd(nil, '1')); got != "1" {
		t.Errorf("appendInd('1') = %q", got)
	}
	if got := string(appendAttrByte(nil, '"')); got != "&quot;" {
		t.Errorf(`appendAttrByte('"') = %q`, got)
	}
	if got := string(appendAttrByte(nil, '<')); got != "&lt;" {
		t.Errorf(`appendAttrByte('<') = %q`, got)
	}
	if got := string(appendAttrByte(nil, '&')); got != "&amp;" {
		t.Errorf(`appendAttrByte('&') = %q`, got)
	}
	if got := string(appendChardata(nil, "a&b<c>d\re")); got != "a&amp;b&lt;c&gt;d&#xD;e" {
		t.Errorf("appendChardata = %q", got)
	}
	if got := indByte(""); got != ' ' {
		t.Errorf("indByte(empty) = %q, want space", got)
	}
	if got := codeByte(""); got != 0 {
		t.Errorf("codeByte(empty) = %d, want 0", got)
	}
}

func TestCarriageReturnRoundTrip(t *testing.T) {
	// A carriage return must be escaped on write (so it survives the decoder's
	// XML line-ending normalization) and restored on read.
	rec := codex.NewRecord().AddField(codex.NewDataField("500", ' ', ' ',
		codex.NewSubfield('a', "line1\rline2")))
	b, err := Encode(rec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "&#xD;") {
		t.Errorf("carriage return not escaped:\n%s", b)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if v := got.SubfieldValue("500", 'a'); v != "line1\rline2" {
		t.Errorf("carriage return not preserved: %q", v)
	}
}

func TestDecodeMissingAttrs(t *testing.T) {
	// A datafield without indicator attributes and a subfield without a code
	// must decode to blank indicators rather than failing.
	rec, err := Decode([]byte(`<record><datafield tag="500"><subfield code="a">note</subfield></datafield></record>`))
	if err != nil {
		t.Fatal(err)
	}
	f, ok := rec.DataField("500")
	if !ok {
		t.Fatal("500 not found")
	}
	if i1, i2 := f.Indicators(); i1 != ' ' || i2 != ' ' {
		t.Errorf("indicators = %q %q, want blanks", i1, i2)
	}
}

// errWriter fails on the nth Write, to exercise the Writer's error paths.
type errWriter struct {
	failAt int
	n      int
}

func (e *errWriter) Write(p []byte) (int, error) {
	e.n++
	if e.n > e.failAt {
		return 0, fmt.Errorf("boom")
	}
	return len(p), nil
}

func TestWriterErrorSticky(t *testing.T) {
	w := NewWriter(&errWriter{failAt: 0}) // fail on the header write
	if err := w.Write(sample()); err == nil {
		t.Error("expected error on first write")
	}
	if err := w.Write(sample()); err == nil {
		t.Error("expected sticky error on second write")
	}
	if err := w.Close(); err == nil {
		t.Error("expected sticky error on Close")
	}
}

func TestCloseIdempotent(t *testing.T) {
	w := NewWriter(&bytes.Buffer{})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
}

func TestWriteFileError(t *testing.T) {
	err := WriteFile(filepath.Join(t.TempDir(), "missing-dir", "x.xml"), []*codex.Record{sample()})
	if err == nil {
		t.Error("expected error writing into a nonexistent directory")
	}
}

// FuzzDecode ensures decoding never panics and that, for inputs that decode,
// Decode->Encode->Decode is stable (marcxml does not normalize the leader).
func FuzzDecode(f *testing.F) {
	b, _ := Encode(sample())
	f.Add(b)
	f.Add([]byte(`<record><controlfield tag="001">x</controlfield></record>`))
	f.Add([]byte(`<collection><record><leader>z</leader></record></collection>`))
	f.Add([]byte(``))
	f.Fuzz(func(t *testing.T, data []byte) {
		rec, err := Decode(data)
		if err != nil || rec == nil {
			return
		}
		// Always exercise encode/decode (no panic). Assert byte-for-byte stability
		// only for self-consistent records: a <datafield> with a control-range or
		// empty tag is misclassified as control by the tag-based model and is not
		// expected to round-trip (malformed input).
		b1, err := Encode(rec)
		if err != nil {
			return // contains a character XML cannot represent
		}
		rec2, err := Decode(b1)
		if err != nil {
			t.Fatalf("re-decode of encoded record failed: %v", err)
		}
		if selfConsistent(rec) && !reflect.DeepEqual(rec, rec2) {
			t.Errorf("round-trip not stable:\n a = %#v\n b = %#v", rec, rec2)
		}
	})
}

// selfConsistent reports whether every field's tag-based classification agrees
// with the attributes it carries, so the record round-trips. The model derives
// control-vs-data from the tag, but XML/JSON carry an explicit element type, so a
// control field with a data-range tag (or vice versa) is malformed and not
// expected to be stable: a control field must have zero indicators and no
// subfields, and a data field must have non-zero (set) indicators, since an
// unset indicator serializes as a blank space.
func selfConsistent(rec *codex.Record) bool {
	for _, f := range rec.Fields() {
		if f.IsControl() {
			if f.Ind1 != 0 || f.Ind2 != 0 || len(f.Subfields) > 0 {
				return false
			}
		} else if f.Ind1 == 0 || f.Ind2 == 0 {
			return false
		}
	}
	return true
}

func TestGoldenCollection(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(sample()); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	golden := filepath.Join("testdata", "sample.xml")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 to create): %v", err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("output differs from %s:\n got:\n%s\n want:\n%s", golden, buf.Bytes(), want)
	}
}
