package codex_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/dublincore"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/marcxml"
	"github.com/freeeve/libcodex/mods"
)

// TestXMLSchemaConformance validates the XML exporters against their official
// schemas (vendored in testdata/schema; see that directory's README) with
// xmllint, over the real MARC-8 corpus. It is skipped when xmllint is absent, so
// CI without libxml2 stays green while the check stays reproducible anywhere it is
// installed.
func TestXMLSchemaConformance(t *testing.T) {
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not installed; skipping XML schema conformance")
	}
	recs, err := iso2709.ReadFile(filepath.Join("testdata", "pymarc-sample.mrc"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	collection := func(w codex.RecordWriter, buf *bytes.Buffer) []byte {
		for _, r := range recs {
			if err := w.Write(r); err != nil {
				t.Fatal(err)
			}
		}
		if c, ok := w.(interface{ Close() error }); ok {
			c.Close()
		}
		return buf.Bytes()
	}

	cases := []struct {
		name, schema string
		gen          func() []byte
	}{
		{"marcxml", "MARC21slim.xsd", func() []byte {
			var b bytes.Buffer
			return collection(marcxml.NewWriter(&b), &b)
		}},
		{"mods", "mods-3-8.xsd", func() []byte {
			var b bytes.Buffer
			return collection(mods.NewWriter(&b), &b)
		}},
		{"dublincore", "oai_dc.xsd", func() []byte {
			// oai_dc.xsd validates a single dc element, not the collection wrapper.
			out, _ := dublincore.Encode(recs[0])
			return append([]byte(`<?xml version="1.0" encoding="UTF-8"?>`+"\n"), out...)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), c.name+".xml")
			if err := os.WriteFile(f, c.gen(), 0o644); err != nil {
				t.Fatal(err)
			}
			schema := filepath.Join("testdata", "schema", c.schema)
			out, err := exec.Command(xmllint, "--noout", "--schema", schema, f).CombinedOutput()
			if err != nil {
				t.Errorf("%s did not validate against %s:\n%s", c.name, c.schema, out)
			}
		})
	}
}
