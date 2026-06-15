package codex_test

import (
	"bytes"
	"io"
	"log"
	"os"
	"reflect"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/marcjson"
	"github.com/freeeve/libcodex/marcxml"
	"github.com/freeeve/libcodex/mrk"
)

// codecFormat adapts a format's reader/writer constructors to the core
// interfaces, so the conversion matrix can treat every format uniformly.
type codecFormat struct {
	name string
	newR func(io.Reader) codex.RecordReader
	newW func(io.Writer) codex.RecordWriter
}

func formats() []codecFormat {
	return []codecFormat{
		{"iso2709", func(r io.Reader) codex.RecordReader { return iso2709.NewReader(r) }, func(w io.Writer) codex.RecordWriter { return iso2709.NewWriter(w) }},
		{"marcxml", func(r io.Reader) codex.RecordReader { return marcxml.NewReader(r) }, func(w io.Writer) codex.RecordWriter { return marcxml.NewWriter(w) }},
		{"marcjson", func(r io.Reader) codex.RecordReader { return marcjson.NewReader(r) }, func(w io.Writer) codex.RecordWriter { return marcjson.NewWriter(w) }},
		{"mrk", func(r io.Reader) codex.RecordReader { return mrk.NewReader(r) }, func(w io.Writer) codex.RecordWriter { return mrk.NewWriter(w) }},
	}
}

func corpus() []*codex.Record {
	return []*codex.Record{
		codex.NewRecord().
			AddField(codex.NewControlField("001", "ocm12345")).
			AddField(codex.NewControlField("008", "210101s2021    nyu           000 1 eng d")).
			AddField(codex.NewDataField("245", '1', '0',
				codex.NewSubfield('a', "Stone butch blues :"),
				codex.NewSubfield('b', "a novel /"),
				codex.NewSubfield('c', "Leslie Feinberg."))).
			AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Café—Lesbians"))).
			AddField(codex.NewDataField("650", ' ', '0', codex.NewSubfield('a', "Gender identity"))),
		codex.NewRecord().
			AddField(codex.NewControlField("001", "rec-2")).
			AddField(codex.NewDataField("100", '1', ' ', codex.NewSubfield('a', "Author, An,"), codex.NewSubfield('d', "1900-"))),
	}
}

// encode writes records with newW, closing the writer if it buffers a wrapper.
func encode(t *testing.T, newW func(io.Writer) codex.RecordWriter, recs []*codex.Record) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := newW(&buf)
	for _, r := range recs {
		if err := w.Write(r); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if c, ok := w.(interface{ Close() error }); ok {
		if err := c.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}
	return buf.Bytes()
}

func decode(t *testing.T, newR func(io.Reader) codex.RecordReader, b []byte) []*codex.Record {
	t.Helper()
	var out []*codex.Record
	for rec, err := range codex.All(newR(bytes.NewReader(b))) {
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		out = append(out, rec)
	}
	return out
}

// normalize runs the corpus through iso2709 once so the leaders carry their
// computed length and base address; every format then reproduces them exactly.
func normalize(t *testing.T, recs []*codex.Record) []*codex.Record {
	fs := formats()[0] // iso2709
	return decode(t, fs.newR, encode(t, fs.newW, recs))
}

func TestAllFormatsPreserveModel(t *testing.T) {
	canonical := normalize(t, corpus())
	for _, f := range formats() {
		t.Run(f.name, func(t *testing.T) {
			got := decode(t, f.newR, encode(t, f.newW, canonical))
			if !reflect.DeepEqual(canonical, got) {
				t.Errorf("%s round trip changed the model:\n want = %#v\n got  = %#v", f.name, canonical, got)
			}
		})
	}
}

func TestConversionMatrix(t *testing.T) {
	canonical := normalize(t, corpus())
	for _, src := range formats() {
		srcBytes := encode(t, src.newW, canonical)
		for _, dst := range formats() {
			t.Run(src.name+"->"+dst.name, func(t *testing.T) {
				// Convert src -> dst purely through the core interfaces.
				var out bytes.Buffer
				dw := dst.newW(&out)
				if err := codex.Convert(src.newR(bytes.NewReader(srcBytes)), dw); err != nil {
					t.Fatalf("Convert: %v", err)
				}
				if c, ok := dw.(interface{ Close() error }); ok {
					if err := c.Close(); err != nil {
						t.Fatalf("close: %v", err)
					}
				}
				got := decode(t, dst.newR, out.Bytes())
				if !reflect.DeepEqual(canonical, got) {
					t.Errorf("%s->%s did not preserve the model", src.name, dst.name)
				}
			})
		}
	}
}

// ExampleConvert converts a binary ISO 2709 record to MARCMaker text using only
// the format-agnostic RecordReader and RecordWriter interfaces.
func ExampleConvert() {
	rec := codex.NewRecord().
		AddField(codex.NewControlField("001", "ex-1")).
		AddField(codex.NewDataField("245", '0', '0', codex.NewSubfield('a', "Example record")))

	var binary bytes.Buffer
	if err := iso2709.NewWriter(&binary).Write(rec); err != nil {
		log.Fatal(err)
	}

	if err := codex.Convert(iso2709.NewReader(&binary), mrk.NewWriter(os.Stdout)); err != nil {
		log.Fatal(err)
	}
	// Output:
	// =LDR  00074nam a2200049   4500
	// =001  ex-1
	// =245  00$aExample record
	//
}
