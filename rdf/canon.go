package rdf

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"sort"
	"strconv"
)

// ErrCanonComplexity is returned by the canonicalization functions when a graph's
// blank-node structure would require more work than maxCanonWork permits — the
// guard against the algorithm's exponential worst case on adversarial "poison"
// datasets. Well-formed data never approaches the limit.
var ErrCanonComplexity = errors.New("rdf: canonicalization exceeded complexity budget")

// maxCanonWork bounds the total permutations examined across the n-degree hashing
// recursion, and maxCanonDepth bounds that recursion's depth. Real datasets finish
// in near-linear work at shallow depth (the whole W3C rdf-canon suite peaks at
// ~2880 permutations and depth 6); only graphs crafted to be maximally symmetric —
// or a long chain of indistinguishable blank nodes, which would otherwise recurse
// one frame per link and overflow the goroutine stack — approach these limits, and
// are rejected quickly with ErrCanonComplexity rather than hanging or crashing.
const (
	maxCanonWork  = 1 << 18
	maxCanonDepth = 1 << 10
)

// Canonical returns the dataset serialized as canonical N-Quads per the W3C RDF
// Dataset Canonicalization algorithm RDFC-1.0 (URDNA2015): blank nodes are
// relabeled to isomorphism-invariant _:c14nN identifiers and the quads are sorted,
// so two datasets that differ only in statement order or blank-node labels produce
// byte-identical output. It returns ErrCanonComplexity for a graph whose symmetry
// exceeds the work budget.
func (d *Dataset) Canonical() ([]byte, error) {
	_, out, err := canonicalize(d.Quads)
	return out, err
}

// Canonicalize returns a new Dataset with blank nodes relabeled to their canonical
// _:c14nN identifiers and quads in canonical order, leaving d untouched.
func Canonicalize(d *Dataset) (*Dataset, error) {
	quads, _, err := canonicalize(d.Quads)
	if err != nil {
		return nil, err
	}
	return &Dataset{Quads: quads}, nil
}

// Canonical returns the graph as canonical N-Quads (its triples in the default
// graph), per RDFC-1.0. See Dataset.Canonical.
func (g *Graph) Canonical() ([]byte, error) {
	quads := make([]Quad, len(g.Triples))
	for i, t := range g.Triples {
		quads[i] = Quad{S: t.S, P: t.P, O: t.O}
	}
	_, out, err := canonicalize(quads)
	return out, err
}

// identifierIssuer issues canonical or temporary blank-node labels (c14n0, c14n1,
// … or b0, b1, …) in first-issue order, per RDFC-1.0's Issue Identifier algorithm.
type identifierIssuer struct {
	prefix string
	issued map[string]string
	order  []string
}

func newIssuer(prefix string) *identifierIssuer {
	return &identifierIssuer{prefix: prefix, issued: map[string]string{}}
}

func (is *identifierIssuer) clone() *identifierIssuer {
	c := &identifierIssuer{
		prefix: is.prefix,
		issued: make(map[string]string, len(is.issued)),
		order:  append([]string(nil), is.order...),
	}
	maps.Copy(c.issued, is.issued)
	return c
}

// issue returns the label for id, minting a new one on first sight.
func (is *identifierIssuer) issue(id string) string {
	if v, ok := is.issued[id]; ok {
		return v
	}
	v := is.prefix + strconv.Itoa(len(is.order))
	is.issued[id] = v
	is.order = append(is.order, id)
	return v
}

func (is *identifierIssuer) get(id string) (string, bool) { v, ok := is.issued[id]; return v, ok }

type canonicalizer struct {
	quads       []Quad
	blankQuads  map[string][]int // blank id -> indices of quads it occurs in (once per occurrence)
	canonical   *identifierIssuer
	firstDegree map[string]string // memoized first-degree hashes (a pure function of the quad set)
	work        int
}

// spend charges n units of work against the complexity budget, panicking with
// ErrCanonComplexity when the budget is exhausted. Charges are proportional to the
// real cost of a step (quads serialized, issuer entries cloned) so an adversarial
// graph that is cheap per permutation but expensive per step still fails fast.
func (c *canonicalizer) spend(n int) {
	c.work += n
	if c.work > maxCanonWork {
		panic(ErrCanonComplexity)
	}
}

// canonicalize runs RDFC-1.0 over quads, returning the relabeled+sorted quads and
// the canonical N-Quads bytes.
func canonicalize(quads []Quad) (result []Quad, out []byte, err error) {
	c := &canonicalizer{
		quads:       quads,
		blankQuads:  map[string][]int{},
		canonical:   newIssuer("c14n"),
		firstDegree: map[string]string{},
	}
	for i, q := range quads {
		for _, t := range [3]Term{q.S, q.O, q.G} {
			if t.Kind == Blank {
				c.blankQuads[t.Value] = append(c.blankQuads[t.Value], i)
			}
		}
	}

	defer func() {
		if r := recover(); r != nil {
			if r == ErrCanonComplexity {
				result, out, err = nil, nil, ErrCanonComplexity
				return
			}
			panic(r)
		}
	}()

	// First-degree hashing: group blank nodes by their neighbourhood hash.
	hashToBlanks := map[string][]string{}
	for bn := range c.blankQuads {
		h := c.hashFirstDegree(bn)
		hashToBlanks[h] = append(hashToBlanks[h], bn)
	}
	hashes := make([]string, 0, len(hashToBlanks))
	for h := range hashToBlanks {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)

	// Unique first-degree hashes get canonical ids immediately, in hash order.
	var ambiguous []string
	for _, h := range hashes {
		if len(hashToBlanks[h]) == 1 {
			c.canonical.issue(hashToBlanks[h][0])
		} else {
			ambiguous = append(ambiguous, h)
		}
	}

	// Remaining (hash-colliding) blank nodes are disambiguated by n-degree hashing.
	for _, h := range ambiguous {
		var results []ndegreeResult
		for _, bn := range hashToBlanks[h] {
			if _, done := c.canonical.get(bn); done {
				continue
			}
			tmp := newIssuer("b")
			tmp.issue(bn)
			hash, iss := c.hashNDegree(bn, tmp, 0)
			results = append(results, ndegreeResult{hash: hash, issuer: iss})
		}
		sort.Slice(results, func(i, j int) bool { return results[i].hash < results[j].hash })
		for _, r := range results {
			for _, id := range r.issuer.order {
				c.canonical.issue(id)
			}
		}
	}

	return c.serialize()
}

// relabel maps a blank term to its canonical form; non-blank terms pass through.
func (c *canonicalizer) relabel(t Term) Term {
	if t.Kind == Blank {
		if id, ok := c.canonical.get(t.Value); ok {
			return NewBlank(id)
		}
	}
	return t
}

// serialize relabels every quad's blank nodes, sorts the quads by their canonical
// N-Quads line, and returns both the sorted quads and the concatenated bytes.
func (c *canonicalizer) serialize() ([]Quad, []byte, error) {
	type lined struct {
		q    Quad
		line []byte
	}
	rows := make([]lined, len(c.quads))
	for i, q := range c.quads {
		rq := Quad{S: c.relabel(q.S), P: c.relabel(q.P), O: c.relabel(q.O), G: c.relabel(q.G)}
		rows[i] = lined{q: rq, line: canonLine(nil, rq, canonAppendTerm)}
	}
	sort.Slice(rows, func(i, j int) bool { return string(rows[i].line) < string(rows[j].line) })
	quads := make([]Quad, 0, len(rows))
	var out []byte
	var prev string
	for i, r := range rows {
		if i > 0 && string(r.line) == prev { // an RDF dataset is a set: drop duplicate quads
			continue
		}
		prev = string(r.line)
		quads = append(quads, r.q)
		out = append(out, r.line...)
	}
	return quads, out, nil
}

// hashFirstDegree returns the hash of a blank node's immediate neighbourhood: each
// of its quads serialized with the node itself as _:a and any other blank as _:z,
// sorted and hashed.
func (c *canonicalizer) hashFirstDegree(bn string) string {
	if h, ok := c.firstDegree[bn]; ok {
		return h
	}
	term := func(b []byte, t Term) []byte {
		if t.Kind == Blank {
			if t.Value == bn {
				return append(b, "_:a"...)
			}
			return append(b, "_:z"...)
		}
		return canonAppendTerm(b, t)
	}
	lines := make([]string, 0, len(c.blankQuads[bn]))
	for _, qi := range c.blankQuads[bn] {
		lines = append(lines, string(canonLine(nil, c.quads[qi], term)))
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
	}
	sum := hex.EncodeToString(h.Sum(nil))
	c.firstDegree[bn] = sum
	return sum
}

// hashRelated hashes a related blank node's contribution from one quad position,
// preferring an already-issued canonical or temporary id and otherwise its
// first-degree hash.
func (c *canonicalizer) hashRelated(related string, q Quad, iss *identifierIssuer, position byte) string {
	input := []byte{position}
	if position != 'g' {
		input = append(input, '<')
		input = append(input, q.P.Value...)
		input = append(input, '>')
	}
	if id, ok := c.canonical.get(related); ok {
		input = append(input, "_:"...)
		input = append(input, id...)
	} else if id, ok := iss.get(related); ok {
		input = append(input, "_:"...)
		input = append(input, id...)
	} else {
		input = append(input, c.hashFirstDegree(related)...)
	}
	sum := sha256.Sum256(input)
	return hex.EncodeToString(sum[:])
}

type ndegreeResult struct {
	hash   string
	issuer *identifierIssuer
}

// hashNDegree computes the n-degree hash of a blank node, distinguishing it from
// others that share a first-degree hash by exploring its blank-node neighbourhood
// (with permutations) — RDFC-1.0's Hash N-Degree Quads.
func (c *canonicalizer) hashNDegree(bn string, iss *identifierIssuer, depth int) (string, *identifierIssuer) {
	if depth > maxCanonDepth {
		panic(ErrCanonComplexity)
	}
	related := map[string][]string{}
	for _, qi := range c.blankQuads[bn] {
		q := c.quads[qi]
		c.addRelated(bn, q.S, q, iss, 's', related)
		c.addRelated(bn, q.O, q, iss, 'o', related)
		c.addRelated(bn, q.G, q, iss, 'g', related)
	}

	var data []byte
	hashes := make([]string, 0, len(related))
	for h := range related {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)

	for _, h := range hashes {
		data = append(data, h...)
		var chosenPath []byte
		var chosenIssuer *identifierIssuer

		permute(related[h], func(perm []string) {
			// Each visit clones the issuer (O(|issued|)) and may serialize quads, so
			// charge for that real cost rather than a flat unit per permutation --
			// otherwise a graph cheap in permutations but expensive per permutation
			// evades the budget.
			c.spend(len(iss.issued) + 1)
			issuerCopy := iss.clone()
			var path []byte
			var recursion []string
			skip := false
			for _, r := range perm {
				if id, ok := c.canonical.get(r); ok {
					path = append(path, "_:"...)
					path = append(path, id...)
				} else {
					if _, seen := issuerCopy.get(r); !seen {
						recursion = append(recursion, r)
					}
					path = append(path, "_:"...)
					path = append(path, issuerCopy.issue(r)...)
				}
				if chosenPath != nil && len(path) >= len(chosenPath) && string(path) > string(chosenPath) {
					skip = true
					break
				}
			}
			if !skip {
				for _, r := range recursion {
					rh, ri := c.hashNDegree(r, issuerCopy, depth+1)
					path = append(path, "_:"...)
					path = append(path, issuerCopy.issue(r)...)
					path = append(path, '<')
					path = append(path, rh...)
					path = append(path, '>')
					issuerCopy = ri
					if chosenPath != nil && len(path) >= len(chosenPath) && string(path) > string(chosenPath) {
						skip = true
						break
					}
				}
			}
			if !skip && (chosenPath == nil || string(path) < string(chosenPath)) {
				chosenPath = path
				chosenIssuer = issuerCopy
			}
		})

		data = append(data, chosenPath...)
		iss = chosenIssuer
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), iss
}

// addRelated records a blank node related to bn through one quad component.
func (c *canonicalizer) addRelated(bn string, t Term, q Quad, iss *identifierIssuer, pos byte, related map[string][]string) {
	if t.Kind != Blank || t.Value == bn {
		return
	}
	h := c.hashRelated(t.Value, q, iss, pos)
	related[h] = append(related[h], t.Value)
}

// permute calls visit for every permutation of list (Heap's algorithm). visit must
// not retain the slice: it is mutated in place between calls.
func permute(list []string, visit func([]string)) {
	if len(list) == 0 {
		visit(list)
		return
	}
	a := append([]string(nil), list...)
	var gen func(k int)
	gen = func(k int) {
		if k == 1 {
			visit(a)
			return
		}
		for i := range k {
			gen(k - 1)
			if k%2 == 0 {
				a[i], a[k-1] = a[k-1], a[i]
			} else {
				a[0], a[k-1] = a[k-1], a[0]
			}
		}
	}
	gen(len(a))
}

// ---- canonical N-Quads term serialization ----

// canonLine appends one quad as a canonical N-Quads line (terms rendered by term),
// terminated by " .\n". A zero-value graph term is the default graph (three terms).
func canonLine(b []byte, q Quad, term func([]byte, Term) []byte) []byte {
	b = term(b, q.S)
	b = append(b, ' ')
	b = term(b, q.P)
	b = append(b, ' ')
	b = term(b, q.O)
	if q.G != (Term{}) {
		b = append(b, ' ')
		b = term(b, q.G)
	}
	return append(b, ' ', '.', '\n')
}

// canonAppendTerm writes a term in canonical N-Quads form.
func canonAppendTerm(b []byte, t Term) []byte {
	switch t.Kind {
	case IRI:
		b = append(b, '<')
		b = appendEscapedIRI(b, t.Value)
		return append(b, '>')
	case Blank:
		b = append(b, '_', ':')
		return append(b, t.Value...)
	default:
		b = append(b, '"')
		b = canonAppendLiteral(b, t.Value)
		b = append(b, '"')
		if t.Lang != "" {
			b = append(b, '@')
			return append(b, t.Lang...)
		}
		if t.Datatype != "" && t.Datatype != XSDString {
			b = append(b, '^', '^', '<')
			b = appendEscapedIRI(b, t.Datatype)
			return append(b, '>')
		}
		return b
	}
}

// canonAppendLiteral escapes a literal's lexical form for canonical N-Quads: the
// named escapes \b \t \n \f \r \" \\, and \u00XX (uppercase) for other C0 controls
// and U+007F; all other bytes pass through as UTF-8. It shares the one literal
// escaper with the N-Triples serializer (see appendLiteralEscaped), configured for
// the RDFC-1.0 profile.
func canonAppendLiteral(b []byte, s string) []byte {
	return appendLiteralEscaped(b, s, literalEscape{namedBF: true, escapeDEL: true})
}
