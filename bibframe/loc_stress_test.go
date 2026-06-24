package bibframe

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/freeeve/libcodex"
	"github.com/freeeve/libcodex/rdf"
)

// TestLoCStress runs the parser and reverse crosswalk over real Library of
// Congress BIBFRAME (id.loc.gov), which is far richer than this library's own
// output: nine-plus namespaces, blank nodes throughout, external IRIs, xml:lang,
// typed literals, admin metadata and the full bf:/bflc: vocabulary. It is gated on
// BIBFRAME_LOC_DIR pointing at a directory of <id>.work.rdf / <id>.inst.rdf /
// <id>.work.json files, so it never runs in normal CI.
//
// Checks: every document parses to a non-empty graph (RDF/XML and JSON-LD); each
// Work+Instance pair crosswalks to a record with a title; and the reconstructed
// 245/1xx are cross-checked against LoC's own bflc:marcKey, which records the
// source MARC field verbatim.
func TestLoCStress(t *testing.T) {
	dir := os.Getenv("BIBFRAME_LOC_DIR")
	if dir == "" {
		dir = filepath.Join("testdata", "loc")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("no BIBFRAME sample directory at %s", dir)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	ids := map[string]bool{}
	rdfDocs, jsonDocs := 0, 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		base := filepath.Base(f)
		id := strings.SplitN(base, ".", 2)[0]
		switch {
		case strings.HasSuffix(f, ".rdf"):
			g, err := rdf.ParseRDFXML(data)
			if err != nil {
				t.Errorf("%s: RDF/XML parse error: %v", base, err)
				continue
			}
			if len(g.Triples) == 0 {
				t.Errorf("%s: 0 triples", base)
			}
			rdfDocs++
			ids[id] = true
		case strings.HasSuffix(f, ".json"):
			g, err := rdf.ParseJSONLD(data)
			if err != nil {
				t.Errorf("%s: JSON-LD parse error: %v", base, err)
				continue
			}
			if len(g.Triples) == 0 {
				t.Errorf("%s: 0 triples", base)
			}
			// The JSON-LD path must also decode without panicking.
			if recs, _ := Decode(data); len(recs) == 0 {
				t.Logf("%s: JSON-LD decoded 0 records (work-only graph)", base)
			}
			jsonDocs++
		}
	}
	t.Logf("parsed %d RDF/XML and %d JSON-LD documents across %d records", rdfDocs, jsonDocs, len(ids))

	sorted := make([]string, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)

	for _, id := range sorted {
		wb, e1 := os.ReadFile(filepath.Join(dir, id+".work.rdf"))
		ib, e2 := os.ReadFile(filepath.Join(dir, id+".inst.rdf"))
		if e1 != nil || e2 != nil {
			continue
		}
		wg, _ := rdf.ParseRDFXML(wb)
		ig, _ := rdf.ParseRDFXML(ib)
		merged := &rdf.Graph{Triples: append(append([]rdf.Triple{}, wg.Triples...), ig.Triples...)}

		works := merged.SubjectsOfType(classWork)
		if len(works) == 0 {
			t.Errorf("%s: no bf:Work in merged graph", id)
			continue
		}
		rec := recordFromWork(merged, works[0])
		title := subfield(rec, "245", 'a')
		if title == "" {
			t.Errorf("%s: reconstructed 245 has empty $a", id)
		}

		// Cross-check contributions (and any 245) against LoC's own bflc:marcKey,
		// which records the source MARC field verbatim.
		for _, tag := range []string{"245", "100", "110", "111"} {
			ours := subfield(rec, tag, 'a')
			key := marcKey(merged, tag)
			if ours == "" || key == "" {
				continue
			}
			if want := marcKeySubfield(key, 'a'); want != "" && !sameTitle(want, ours) {
				t.Errorf("%s: %s $a %q does not match marcKey %q", id, tag, ours, want)
			}
		}

		// A bf:language with a resolvable code must produce an 041.
		if hasLanguage(merged, works[0]) && subfield(rec, "041", 'a') == "" {
			t.Errorf("%s: bf:language present but 041 missing", id)
		}
		// The transcribed publication place/agent must surface in 260.
		if hasLiteralPred(merged, pSimplePlace) && subfield(rec, "260", 'a') == "" {
			t.Errorf("%s: bflc:simplePlace present but 260 $a missing", id)
		}
		if hasLiteralPred(merged, pSimpleAgent) && subfield(rec, "260", 'b') == "" {
			t.Errorf("%s: bflc:simpleAgent present but 260 $b missing", id)
		}
		t.Logf("%-9s [%s] %q  author=%q  pub=%q/%q lang=%q lccn=%q subj=%d",
			id, string(rec.Leader().RecordType()), title,
			firstNonEmpty(subfield(rec, "100", 'a'), subfield(rec, "110", 'a'), subfield(rec, "111", 'a'),
				subfield(rec, "700", 'a'), subfield(rec, "710", 'a'), subfield(rec, "711", 'a')),
			subfield(rec, "260", 'a'), subfield(rec, "260", 'b'), subfield(rec, "041", 'a'), subfield(rec, "010", 'a'),
			count(rec, "650")+count(rec, "651")+count(rec, "600")+count(rec, "610")+count(rec, "611"))
	}
}

// hasLanguage reports whether the work has a bf:language node with a resolvable
// three-letter code.
func hasLanguage(g *rdf.Graph, work rdf.Term) bool {
	for _, l := range g.Objects(work, pLanguage) {
		if langCode(g, l) != "" {
			return true
		}
	}
	return false
}

// hasLiteralPred reports whether any triple uses the predicate with a literal.
func hasLiteralPred(g *rdf.Graph, predicate string) bool {
	for _, tr := range g.Triples {
		if tr.P.IsIRI() && tr.P.Value == predicate && tr.O.IsLiteral() {
			return true
		}
	}
	return false
}

// subfield returns the first value of code in the first field with tag, or "".
func subfield(r *codex.Record, tag string, code byte) string {
	if f := firstField(r, tag); f != nil {
		return f.SubfieldValue(code)
	}
	return ""
}

// count returns how many fields have the tag.
func count(r *codex.Record, tag string) int {
	n := 0
	for _, f := range r.Fields() {
		if f.Tag == tag {
			n++
		}
	}
	return n
}

// marcKey returns the first bflc:marcKey literal in the graph whose MARC tag (its
// first three characters) matches tag, or "".
func marcKey(g *rdf.Graph, tag string) string {
	for _, tr := range g.Triples {
		if tr.P.IsIRI() && tr.P.Value == bflcNS+"marcKey" && tr.O.IsLiteral() {
			if strings.HasPrefix(tr.O.Value, tag) {
				return tr.O.Value
			}
		}
	}
	return ""
}

// marcKeySubfield extracts a subfield from a bflc:marcKey string, whose body uses
// the standard "$<code>value" delimiter convention.
func marcKeySubfield(key string, code byte) string {
	for i := 0; i+1 < len(key); i++ {
		if key[i] == '$' && key[i+1] == code {
			rest := key[i+2:]
			before, _, _ := strings.Cut(rest, "$")
			return strings.TrimRight(before, " /:;,.")
		}
	}
	return ""
}

// sameTitle compares two titles ignoring case and trailing ISBD punctuation, since
// the crosswalk trims punctuation the marcKey keeps.
func sameTitle(a, b string) bool {
	norm := func(s string) string {
		return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), " /:;,."))
	}
	na, nb := norm(a), norm(b)
	return na == nb || strings.HasPrefix(na, nb) || strings.HasPrefix(nb, na)
}
