package codex_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/bibframe"
	"github.com/freeeve/libcodex/citation"
	"github.com/freeeve/libcodex/iso2709"
	"github.com/freeeve/libcodex/marcjson"
	"github.com/freeeve/libcodex/schemaorg"
)

// interopResult is the JSON one-liner testdata/interop.py prints.
type interopResult struct {
	OK      bool   `json:"ok"`
	Records int    `json:"records"`
	Triples int    `json:"triples"`
	Entries int    `json:"entries"`
	Failed  int    `json:"failed"`
	Title   string `json:"title"`
	Error   string `json:"error"`
}

// TestInterop validates the library's generated output by reading it back with
// independent, widely used parsers (pymarc, rdflib, bibtexparser, rispy), over the
// real corpus. It is skipped unless those libraries are importable; the CI interop
// job installs them. Set INTEROP_PYTHON to choose the interpreter.
func TestInterop(t *testing.T) {
	py := os.Getenv("INTEROP_PYTHON")
	if py == "" {
		py = "python3"
	}
	script := filepath.Join("testdata", "interop.py")
	if _, err := runInterop(py, script, "check"); err != nil {
		t.Skipf("interop parsers unavailable (%v); set INTEROP_PYTHON or pip install pymarc rdflib bibtexparser rispy", err)
	}

	recs, err := iso2709.ReadFile(filepath.Join("testdata", "pymarc-sample.mrc"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// The strongest check: our binary MARC, read by pymarc, written back as
	// MARC-in-JSON, then read by our marcjson reader — interop in both directions.
	t.Run("iso2709<->pymarc<->marcjson", func(t *testing.T) {
		mrc := write("corpus.mrc", encodeAll(t, recs))
		jsonPath := filepath.Join(dir, "from-pymarc.json")
		r := mustInterop(t, py, script, "marc-to-json", mrc, jsonPath)
		if r.Records != len(recs) {
			t.Fatalf("pymarc read %d records from our MARC, want %d", r.Records, len(recs))
		}
		back, err := marcjson.ReadFile(jsonPath)
		if err != nil {
			t.Fatalf("our marcjson reader rejected pymarc's output: %v", err)
		}
		if len(back) != len(recs) {
			t.Fatalf("round-trip record count %d != %d", len(back), len(recs))
		}
		if got, want := back[0].SubfieldValue("245", 'a'), recs[0].SubfieldValue("245", 'a'); got != want {
			t.Errorf("round-trip 245$a = %q, want %q", got, want)
		}
	})

	t.Run("marcjson->pymarc", func(t *testing.T) {
		p := write("ours.json", encodeJSON(t, recs))
		r := mustInterop(t, py, script, "read-json", p)
		if r.Records != len(recs) {
			t.Errorf("pymarc read %d records from our marcjson, want %d", r.Records, len(recs))
		}
		// pymarc's .title joins 245 $a and $b, so check ours is contained.
		if a := recs[0].SubfieldValue("245", 'a'); !strings.Contains(r.Title, a) {
			t.Errorf("pymarc title %q does not contain our 245$a %q", r.Title, a)
		}
	})

	t.Run("bibframe-rdfxml->rdflib", func(t *testing.T) {
		b, _ := bibframe.Encode(recs[0])
		r := mustInterop(t, py, script, "parse-rdf", write("bf.rdf", b), "xml")
		if r.Triples == 0 {
			t.Error("rdflib parsed 0 triples from our BIBFRAME RDF/XML")
		}
	})

	t.Run("bibframe-jsonld->rdflib", func(t *testing.T) {
		b, _ := bibframe.EncodeJSONLD(recs[0])
		r := mustInterop(t, py, script, "parse-rdf", write("bf.jsonld", b), "json-ld")
		if r.Triples == 0 {
			t.Error("rdflib parsed 0 triples from our BIBFRAME JSON-LD")
		}
	})

	t.Run("schemaorg->rdflib", func(t *testing.T) {
		b, _ := schemaorg.Encode(recs[0])
		r := mustInterop(t, py, script, "parse-rdf", write("so.jsonld", b), "json-ld")
		if r.Triples == 0 {
			t.Error("rdflib parsed 0 triples from our schema.org JSON-LD")
		}
	})

	t.Run("citation-bibtex->bibtexparser", func(t *testing.T) {
		r := mustInterop(t, py, script, "parse-bibtex", write("c.bib", citationAll(t, recs, false)))
		if r.Entries != len(recs) || r.Failed != 0 {
			t.Errorf("bibtexparser: %d entries, %d failed, want %d/0", r.Entries, r.Failed, len(recs))
		}
	})

	t.Run("citation-ris->rispy", func(t *testing.T) {
		r := mustInterop(t, py, script, "parse-ris", write("c.ris", citationAll(t, recs, true)))
		if r.Entries != len(recs) {
			t.Errorf("rispy: %d entries, want %d", r.Entries, len(recs))
		}
	})
}

func runInterop(py, script string, args ...string) (interopResult, error) {
	out, err := exec.Command(py, append([]string{script}, args...)...).CombinedOutput()
	var r interopResult
	if jerr := json.Unmarshal(bytes.TrimSpace(lastLine(out)), &r); jerr != nil {
		if err != nil {
			return r, err
		}
		return r, jerr
	}
	if err != nil || !r.OK {
		if r.Error != "" {
			return r, &interopError{r.Error}
		}
		return r, err
	}
	return r, nil
}

func mustInterop(t *testing.T, py, script string, args ...string) interopResult {
	t.Helper()
	r, err := runInterop(py, script, args...)
	if err != nil {
		t.Fatalf("interop %v failed: %v", args, err)
	}
	return r
}

type interopError struct{ msg string }

func (e *interopError) Error() string { return e.msg }

func lastLine(b []byte) []byte {
	b = bytes.TrimRight(b, "\n")
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
		return b[i+1:]
	}
	return b
}

func encodeAll(t *testing.T, recs []*codex.Record) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, r := range recs {
		b, err := iso2709.Encode(r)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(b)
	}
	return buf.Bytes()
}

func encodeJSON(t *testing.T, recs []*codex.Record) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := marcjson.NewWriter(&buf)
	for _, r := range recs {
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	return buf.Bytes()
}

func citationAll(t *testing.T, recs []*codex.Record, ris bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	var w codex.RecordWriter = citation.NewBibTeXWriter(&buf)
	if ris {
		w = citation.NewRISWriter(&buf)
	}
	for _, r := range recs {
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes()
}
