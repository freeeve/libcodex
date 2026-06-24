package rdf

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// TestArenaSurvivesGC parses graphs large enough to span many arena chunks, then
// forces several GC cycles and checks the arena-backed (unsafe.String) IRIs and
// literals are still intact — a regression guard for the string arena.
func TestArenaSurvivesGC(t *testing.T) {
	const n = 50000 // ~2 MB of materialized strings -> dozens of 64 KiB chunks
	var nt, ttl strings.Builder
	ttl.WriteString("@prefix ex: <http://example.org/> .\n")
	for i := range n {
		fmt.Fprintf(&nt, "<http://example.org/s%d> <http://example.org/p%d> \"v%d say \\\"hi\\\" \\u00e9\" .\n", i, i%50, i)
		fmt.Fprintf(&ttl, "ex:s%d ex:p%d \"v%d say \\\"hi\\\" \\u00e9\" .\n", i, i%50, i)
	}
	wantObj := func(i int) string { return fmt.Sprintf("v%d say \"hi\" é", i) }
	wantSubj := func(i int) string { return fmt.Sprintf("http://example.org/s%d", i) }

	for _, tc := range []struct {
		name  string
		parse func([]byte) (*Graph, error)
		data  string
	}{
		{"ntriples", ParseNTriples, nt.String()},
		{"turtle", ParseTurtle, ttl.String()},
	} {
		g, err := tc.parse([]byte(tc.data))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(g.Triples) != n {
			t.Fatalf("%s: %d triples, want %d", tc.name, len(g.Triples), n)
		}
		for range 5 {
			runtime.GC()
		}
		for _, i := range []int{0, n / 2, n - 1} {
			if got := g.Triples[i].O.Value; got != wantObj(i) {
				t.Fatalf("%s: literal %d corrupted after GC:\n got %q\nwant %q", tc.name, i, got, wantObj(i))
			}
			if got := g.Triples[i].S.Value; got != wantSubj(i) {
				t.Fatalf("%s: subject %d corrupted after GC: %q", tc.name, i, got)
			}
		}
	}
}
