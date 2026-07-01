package rdf

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// isomorphicVariant returns a dataset denoting the same graph as d but with its
// blank-node labels renamed and its quads reversed, so a correct canonicalization
// must produce byte-identical output for both.
func isomorphicVariant(d *Dataset) *Dataset {
	rename := map[string]string{}
	relabel := func(t Term) Term {
		if t.Kind != Blank {
			return t
		}
		n, ok := rename[t.Value]
		if !ok {
			n = "renamed" + strconv.Itoa(len(rename)) + "x"
			rename[t.Value] = n
		}
		return NewBlank(n)
	}
	out := &Dataset{Quads: make([]Quad, len(d.Quads))}
	for i, q := range d.Quads {
		// reverse order and relabel blanks
		out.Quads[len(d.Quads)-1-i] = Quad{relabel(q.S), relabel(q.P), relabel(q.O), relabel(q.G)}
	}
	return out
}

// TestCanonInvariance checks that every rdf-canon input canonicalizes to the same
// bytes as an isomorphic variant (blanks renamed, quads reordered).
func TestCanonInvariance(t *testing.T) {
	ins, _ := filepath.Glob(filepath.Join(rdfCanonDir, "*-in.nq"))
	if len(ins) == 0 {
		t.Skip("rdf-canon vectors not present")
	}
	for _, in := range ins {
		name := strings.TrimSuffix(filepath.Base(in), "-in.nq")
		if name == "test074" || name == "test075" {
			continue // poison graph / SHA-384
		}
		data, _ := os.ReadFile(in)
		ds, _ := ParseNQuads(data)
		a, err := ds.Canonical()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, err := isomorphicVariant(ds).Canonical()
		if err != nil {
			t.Fatalf("%s variant: %v", name, err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s: canonical form not invariant under relabel+reorder", name)
		}
	}
}

// TestCanonIdempotence checks that re-canonicalizing canonical output — including
// after a parse round trip — reproduces it byte-for-byte.
func TestCanonIdempotence(t *testing.T) {
	ins, _ := filepath.Glob(filepath.Join(rdfCanonDir, "*-in.nq"))
	if len(ins) == 0 {
		t.Skip("rdf-canon vectors not present")
	}
	for _, in := range ins {
		name := strings.TrimSuffix(filepath.Base(in), "-in.nq")
		if name == "test074" || name == "test075" {
			continue
		}
		data, _ := os.ReadFile(in)
		ds, _ := ParseNQuads(data)
		once, err := ds.Canonical()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		reparsed, _ := ParseNQuads(once)
		twice, _ := reparsed.Canonical()
		if !bytes.Equal(once, twice) {
			t.Errorf("%s: canonicalization not idempotent", name)
		}
	}
}

const rdfCanonDir = "testdata/rdf-canon/rdfc10"

// TestRDFCanonConformance runs the official W3C rdf-canon (RDFC-1.0) evaluation
// vectors: each testNNN-in.nq must canonicalize byte-for-byte to testNNN-rdfc10.nq.
// test074 is the negative (poison) case that must exceed the complexity budget;
// test075 uses SHA-384, which this implementation does not target.
func TestRDFCanonConformance(t *testing.T) {
	ins, _ := filepath.Glob(filepath.Join(rdfCanonDir, "*-in.nq"))
	if len(ins) == 0 {
		t.Skip("rdf-canon vectors not present")
	}
	sort.Strings(ins)
	passed := 0
	for _, in := range ins {
		name := strings.TrimSuffix(filepath.Base(in), "-in.nq")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			ds, _ := ParseNQuads(data)

			if name == "test074" { // negative: symmetric poison graph
				if _, err := ds.Canonical(); err != ErrCanonComplexity {
					t.Fatalf("want ErrCanonComplexity, got %v", err)
				}
				passed++
				return
			}
			if name == "test075" { // SHA-384 variant, out of scope
				t.Skip("SHA-384 hash variant")
			}

			want, err := os.ReadFile(filepath.Join(rdfCanonDir, name+"-rdfc10.nq"))
			if err != nil {
				t.Skipf("no expected result: %v", err)
			}
			got, err := ds.Canonical()
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if string(got) != string(want) {
				t.Fatalf("canonical output differs\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
			passed++
		})
	}
	t.Logf("rdf-canon: %d vectors checked", passed)
}

// TestCanonDeepChainGuard checks that a long chain of indistinguishable blank
// nodes — which drives the n-degree hashing recursion arbitrarily deep — is
// rejected with ErrCanonComplexity rather than overflowing the goroutine stack.
func TestCanonDeepChainGuard(t *testing.T) {
	var b strings.Builder
	for i := range 50000 {
		b.WriteString("_:b" + strconv.Itoa(i) + " <http://p> _:b" + strconv.Itoa(i+1) + " .\n")
	}
	ds, _ := ParseNQuads([]byte(b.String()))
	if _, err := ds.Canonical(); err != ErrCanonComplexity {
		t.Fatalf("deep blank chain: got %v, want ErrCanonComplexity", err)
	}
}

// FuzzCanonInvariance asserts canonicalization is isomorphism-invariant on
// arbitrary input: an input and its relabeled, reordered variant canonicalize to
// identical bytes, and canonicalization never panics.
func FuzzCanonInvariance(f *testing.F) {
	f.Add([]byte("_:a <u:p> _:b .\n_:b <u:p> _:a .\n"))
	f.Add([]byte("<u:s> <u:p> _:x <u:g> .\n_:x <u:q> \"v\" .\n"))
	for _, seed := range []string{"test016", "test020", "test038", "test044"} {
		if b, err := os.ReadFile(filepath.Join(rdfCanonDir, seed+"-in.nq")); err == nil {
			f.Add(b)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		ds, _ := ParseNQuads(data)
		a, err := ds.Canonical()
		if err != nil {
			return // complexity budget hit; acceptable
		}
		b, err := isomorphicVariant(ds).Canonical()
		if err != nil {
			return
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("canonical form not invariant:\n a=%s\n b=%s", a, b)
		}
	})
}
