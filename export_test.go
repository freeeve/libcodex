package codex_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/bibframe"
	"github.com/freeeve/libcodex/citation"
	"github.com/freeeve/libcodex/dublincore"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/mods"
	"github.com/freeeve/libcodex/schemaorg"
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
		{"citation-ris", func(w io.Writer) codex.RecordWriter { return citation.NewRISWriter(w) }, risValid},
		{"citation-bibtex", func(w io.Writer) codex.RecordWriter { return citation.NewBibTeXWriter(w) }, bibtexValid},
		{"bibframe-rdfxml", func(w io.Writer) codex.RecordWriter { return bibframe.NewWriter(w) }, xmlWellFormed},
		{"bibframe-jsonld", func(w io.Writer) codex.RecordWriter { return bibframe.NewJSONLDWriter(w) }, jsonValid},
		{"schemaorg", func(w io.Writer) codex.RecordWriter { return schemaorg.NewWriter(w) }, jsonValid},
	} {
		t.Run(tgt.name, func(t *testing.T) {
			var out bytes.Buffer
			w := tgt.newW(&out)
			for _, r := range recs {
				if err := w.Write(r); err != nil {
					t.Fatalf("write: %v", err)
				}
			}
			if err := codex.Close(w); err != nil {
				t.Fatalf("close: %v", err)
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

// xmlWellFormed reports an error if b is not well-formed XML, or if it contains
// no element at all -- so a regression to empty exporter output fails the test
// instead of passing (io.EOF on the first token would otherwise read as "valid").
func xmlWellFormed(b []byte) error {
	if len(b) == 0 {
		return fmt.Errorf("empty XML output")
	}
	seen := false
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			if !seen {
				return fmt.Errorf("XML output has no elements")
			}
			return nil
		}
		if err != nil {
			return err
		}
		if _, ok := tok.(xml.StartElement); ok {
			seen = true
		}
	}
}

func jsonValid(b []byte) error {
	var v any
	return json.Unmarshal(b, &v)
}

// risValid checks the RIS type and end-of-record markers are present.
func risValid(b []byte) error {
	s := string(b)
	if !strings.Contains(s, "TY  -") || !strings.Contains(s, "ER  -") {
		return fmt.Errorf("RIS output missing TY/ER markers")
	}
	return nil
}

// bibtexValid checks the output begins with a BibTeX entry.
func bibtexValid(b []byte) error {
	if !strings.HasPrefix(strings.TrimSpace(string(b)), "@") {
		return fmt.Errorf("BibTeX output does not start with @")
	}
	return nil
}
