package mrk

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
	"github.com/freeeve/libcodex/marcxml"
)

// sample exercises control fields, repeated data fields, blank/non-blank
// indicators, the {dollar}/{lcub}/{rcub} mnemonics and UTF-8.
func sample() *codex.Record {
	return codex.NewRecord().
		AddField(codex.NewControlField("001", "ocm12345")).
		AddField(codex.NewControlField("008", "210101s2021    nyu")).
		AddField(codex.NewDataField("245", '1', '0',
			codex.NewSubfield('a', "Stone butch blues :"),
			codex.NewSubfield('b', "a novel /"),
			codex.NewSubfield('c', "Leslie Feinberg."))).
		AddField(codex.NewDataField("520", ' ', ' ',
			codex.NewSubfield('a', "Cost is $5, see {note} & more"))).
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

func TestEncodeFormat(t *testing.T) {
	b, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, want := range []string{
		"=LDR  00000nam a2200000   4500\n",
		"=001  ocm12345\n",
		"=245  10$aStone butch blues :$ba novel /$cLeslie Feinberg.\n",
		"=520  \\\\$aCost is {dollar}5, see {lcub}note{rcub} & more\n",
		"=650  \\0$aCafé—Lesbians\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestCrossFormatISO2709(t *testing.T) {
	mrc, err := iso2709.Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	fromMRC, _, err := iso2709.Decode(mrc)
	if err != nil {
		t.Fatal(err)
	}
	mrkb, err := Encode(fromMRC)
	if err != nil {
		t.Fatal(err)
	}
	fromMRK, err := Decode(mrkb)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromMRC, fromMRK) {
		t.Errorf("iso2709 record not preserved through mrk")
	}
	back, err := iso2709.Encode(fromMRK)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mrc, back) {
		t.Error("iso2709 -> mrk -> iso2709 is not byte-stable")
	}
}

func TestCrossFormatMARCXML(t *testing.T) {
	xmlb, err := marcxml.Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	fromXML, err := marcxml.Decode(xmlb)
	if err != nil {
		t.Fatal(err)
	}
	mrkb, err := Encode(fromXML)
	if err != nil {
		t.Fatal(err)
	}
	fromMRK, err := Decode(mrkb)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromXML, fromMRK) {
		t.Errorf("marcxml and mrk disagree on the model")
	}
}

func TestReaderMultiple(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(sample()); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(codex.NewRecord().AddField(codex.NewControlField("001", "two"))); err != nil {
		t.Fatal(err)
	}

	recs := readAll(t, NewReader(&buf))
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

func TestReaderTolerance(t *testing.T) {
	// Leading/extra blank lines and a non-"=" comment line are tolerated; the
	// last record need not end in a blank line.
	in := "\n\n; a comment\n=LDR  00000nam a2200000   4500\n=001  a\n\n\n=001  b"
	recs := readAll(t, NewReader(strings.NewReader(in)))
	if len(recs) != 2 {
		t.Fatalf("read %d records, want 2", len(recs))
	}
	if recs[0].ControlField("001") != "a" || recs[1].ControlField("001") != "b" {
		t.Errorf("got 001 = %q, %q", recs[0].ControlField("001"), recs[1].ControlField("001"))
	}
}

func TestDecodeCharReferences(t *testing.T) {
	// Decoding accepts numeric character references even though Encode emits UTF-8.
	rec, err := Decode([]byte("=245  10$aCaf&#xe9; / na&#239;ve"))
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.SubfieldValue("245", 'a'); got != "Café / naïve" {
		t.Errorf("245a = %q, want %q", got, "Café / naïve")
	}
}

func TestBlankIndicators(t *testing.T) {
	rec, err := Decode([]byte("=245  \\\\$aBoth blank\n=100  1\\$aOne blank"))
	if err != nil {
		t.Fatal(err)
	}
	if f, _ := rec.DataField("245"); f.Ind1 != ' ' || f.Ind2 != ' ' {
		t.Errorf("245 indicators = %q %q, want blanks", f.Ind1, f.Ind2)
	}
	if f, _ := rec.DataField("100"); f.Ind1 != '1' || f.Ind2 != ' ' {
		t.Errorf("100 indicators = %q %q, want '1' ' '", f.Ind1, f.Ind2)
	}
}

func TestEncodeRejectsLineBreaks(t *testing.T) {
	cases := map[string]*codex.Record{
		"control value":  codex.NewRecord().AddField(codex.NewControlField("001", "a\nb")),
		"subfield value": codex.NewRecord().AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('a', "x\ry"))),
		"leader":         codex.NewRecord().SetLeader(codex.Leader("00000nam\n2200000   4500")).AddField(codex.NewControlField("001", "x")),
		"indicator CR":   codex.NewRecord().AddField(codex.NewDataField("245", '1', '\r', codex.NewSubfield('a', "x"))),
		"dollar code":    codex.NewRecord().AddField(codex.NewDataField("245", '1', '0', codex.NewSubfield('$', "x"))),
	}
	for name, rec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Encode(rec); err == nil {
				t.Error("expected error for a line break")
			}
			if err := NewWriter(&bytes.Buffer{}).Write(rec); err == nil {
				t.Error("Writer.Write: expected error for a line break")
			}
		})
	}
}

func TestDecodeEmpty(t *testing.T) {
	if _, err := Decode([]byte("")); err == nil {
		t.Error("expected error for empty input")
	}
	if _, err := Decode([]byte("\n\n  \n")); err == nil {
		t.Error("expected error for blank-only input")
	}
}

func TestUnescapeHelpers(t *testing.T) {
	cases := map[string]string{
		"plain":          "plain",
		"{dollar}":       "$",
		"a{lcub}b{rcub}": "a{b}",
		"&#x41;&#66;":    "AB",
		"&#xZZ;":         "&#xZZ;", // invalid hex -> left as-is
		"&#;":            "&#;",    // no digits -> left as-is
		"&#x41":          "&#x41",  // no terminator -> left as-is
	}
	for in, want := range cases {
		if got := unescape(in); got != want {
			t.Errorf("unescape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadWriteFile(t *testing.T) {
	recs := []*codex.Record{sample(), codex.NewRecord().AddField(codex.NewControlField("001", "x"))}
	path := filepath.Join(t.TempDir(), "c.mrk")
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
	if _, err := ReadFile(filepath.Join(t.TempDir(), "missing.mrk")); err == nil {
		t.Error("expected error for missing file")
	}
}

// errWriter fails on the nth write, to exercise the Writer's error path.
type errWriter struct{ n int }

func (e *errWriter) Write(p []byte) (int, error) {
	e.n++
	return 0, fmt.Errorf("boom")
}

func TestWriterError(t *testing.T) {
	if err := NewWriter(&errWriter{}).Write(sample()); err == nil {
		t.Error("expected write error")
	}
	if err := WriteFile(filepath.Join(t.TempDir(), "missing-dir", "x.mrk"), []*codex.Record{sample()}); err == nil {
		t.Error("expected error writing into a nonexistent directory")
	}
}

// selfConsistent reports whether every field's tag-based classification matches
// the attributes it carries, so the record round-trips (see marcxml).
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

// FuzzDecode ensures decoding never panics and that, for inputs that decode,
// Decode->Encode->Decode is stable for self-consistent records.
// FuzzFromMARC ensures the MARC->mrk path produces re-decodable output (or a
// clean error) and never a panic. mrk is byte-transparent, so it should carry
// arbitrary bytes that are not structural.
func FuzzFromMARC(f *testing.F) {
	mrc, _ := iso2709.Encode(sample())
	f.Add(mrc)
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		rec, _, err := iso2709.Decode(data)
		if err != nil || rec == nil {
			return
		}
		b, err := Encode(rec)
		if err != nil {
			return // contains a line break or a $ subfield code
		}
		if _, err := Decode(b); err != nil {
			t.Errorf("re-decode of MARC->mrk output failed: %v\n%q", err, b)
		}
	})
}

func FuzzDecode(f *testing.F) {
	b, _ := Encode(sample())
	f.Add(b)
	f.Add([]byte("=001  x"))
	f.Add([]byte("=245  10$aTitle$bSub"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		rec, err := Decode(data)
		if err != nil || rec == nil {
			return
		}
		out, err := Encode(rec)
		if err != nil {
			return // contains a line break
		}
		rec2, err := Decode(out)
		if err != nil {
			t.Fatalf("re-decode of encoded record failed: %v", err)
		}
		if selfConsistent(rec) && !reflect.DeepEqual(rec, rec2) {
			t.Errorf("round-trip not stable:\n a = %#v\n b = %#v", rec, rec2)
		}
	})
}

func TestGolden(t *testing.T) {
	recs := []*codex.Record{sample(), codex.NewRecord().AddField(codex.NewControlField("001", "x"))}
	path := filepath.Join("testdata", "sample.mrk")

	var buf bytes.Buffer
	w := NewWriter(&buf)
	for _, rec := range recs {
		if err := w.Write(rec); err != nil {
			t.Fatal(err)
		}
	}

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (UPDATE_GOLDEN=1 to create): %v", err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("output differs from %s:\n got:\n%s\n want:\n%s", path, buf.Bytes(), want)
	}
}
