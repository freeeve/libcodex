package codex_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/bibframe"
	"github.com/freeeve/libcodex/citation"
	"github.com/freeeve/libcodex/dublincore"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/mods"
)

// TestExportConvertersCanonical runs every one-way export converter over the
// real MARC-8 corpus through codex.Convert, confirming each produces well-formed,
// valid-UTF-8 output on authentic data (diacritics included). These targets are
// lossy crosswalks, so they are checked for validity, not round-trip equality.
func TestExportConvertersCanonical(t *testing.T) {
	recs, err := iso2709.ReadFile(filepath.Join("testdata", "pymarc-sample.mrc"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	for _, tgt := range []struct {
		name  string
		newW  func(io.Writer) codex.RecordWriter
		check func([]byte) error
	}{
		{"mods", func(w io.Writer) codex.RecordWriter { return mods.NewWriter(w) }, xmlWellFormed},
		{"dublincore-xml", func(w io.Writer) codex.RecordWriter { return dublincore.NewWriter(w) }, xmlWellFormed},
		{"dublincore-json", func(w io.Writer) codex.RecordWriter { return dublincore.NewJSONWriter(w) }, jsonValid},
		{"citation-ris", func(w io.Writer) codex.RecordWriter { return citation.NewRISWriter(w) }, utf8NonEmpty},
		{"citation-bibtex", func(w io.Writer) codex.RecordWriter { return citation.NewBibTeXWriter(w) }, utf8NonEmpty},
		{"bibframe-rdfxml", func(w io.Writer) codex.RecordWriter { return bibframe.NewWriter(w) }, xmlWellFormed},
		{"bibframe-jsonld", func(w io.Writer) codex.RecordWriter { return bibframe.NewJSONLDWriter(w) }, jsonValid},
	} {
		t.Run(tgt.name, func(t *testing.T) {
			var out bytes.Buffer
			w := tgt.newW(&out)
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
			if out.Len() == 0 {
				t.Fatal("no output")
			}
			if !utf8.Valid(out.Bytes()) {
				t.Errorf("%s output is not valid UTF-8", tgt.name)
			}
			if err := tgt.check(out.Bytes()); err != nil {
				t.Errorf("%s: %v", tgt.name, err)
			}
		})
	}
}

func xmlWellFormed(b []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func jsonValid(b []byte) error {
	var v any
	return json.Unmarshal(b, &v)
}

func utf8NonEmpty(b []byte) error { return nil }
