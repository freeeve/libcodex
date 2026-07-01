# 036: RDF dataset canonicalization (RDFC-1.0)

Promote the `canonicalTriples` test helper (`rdf/w3c_conformance_test.go:16`) into
a production canonicalization API, so consumers get **isomorphism-invariant**
N-Quads: an unchanged graph re-serializes byte-for-byte, and two graphs differing
only in statement order or blank-node labels produce identical output. This is the
clean-diff / git-review guarantee libcatalog's per-Work grains depend on
(libcatalog ROADMAP Phase 0), and the one piece 0.4.0's N-Quads I/O does not yet
provide.

## Why the current output is not canonical
- `Dataset.NQuads()` / `Graph.NQuads()` (`rdf/quad.go`) emit quads in
  **insertion/slice order** -- no sort.
- `blankNamer` (`rdf/ntriples.go:265`) assigns `b1, b2, …` in **first-encounter
  order**: deterministic within one serialization pass, but not
  isomorphism-invariant. Reorder the same logical graph and the blank labels shift.
- BIBFRAME leans on blank nodes (provisionActivity, titles, notes), so grains
  built in a different statement order would diff spuriously.
- The one path that sorts + canonically labels blanks, `canonicalTriples`, is a
  **test helper** over a `*Graph` (triples only), not an exported dataset API.

## Pieces
1. **Canonical blank-node labeling: RDFC-1.0 (URDNA2015).** In-house, honoring the
   zero-dependency rule (as with the RDF parsers in `035`): first-degree hashing,
   then the n-degree hash / permutation step for blank-node cycles and hash ties.
   Guard the known exponential worst case on adversarial "poison" graphs with a
   bounded work / call limit (fits the existing `adversary_test.go` posture).
2. **Dataset scope, not just triples.** Grains are **quads** (named graphs
   `feed:<provider>` / `editorial:`), so canonicalization runs at the **Dataset**
   level: the graph term participates in both the blank labeling and the final
   sort. Generalizes the triple-level `canonicalTriples`.
3. **Canonical statement sort.** After relabeling, sort quads by canonical
   `(g, s, p, o)` in N-Quads term order.
4. **Public API.** e.g. `func (d *Dataset) Canonical() []byte` (canonical N-Quads
   bytes) and/or `func Canonicalize(d *Dataset) *Dataset` (relabeled + sorted).
   Pick the surface; keep the existing non-canonical `NQuads()` for the
   streaming/bulk fast path.

## Validation
- **W3C rdf-canon test suite** (the official RDFC-1.0 vectors) under the existing
  `w3c_conformance_test.go` harness.
- **Isomorphism invariance** (property/fuzz): shuffle a dataset's quads and
  relabel its blanks, assert `Canonical` is byte-identical.
- **Idempotence:** `Canonical(Canonical(x)) == Canonical(x)`.
- **No-op diff:** decode a canonical grain and re-`Canonical` it -> identical bytes.
- Re-express the current `canonicalTriples` helper on the new API, or keep it as an
  independent oracle.

## Out of scope
- RDF-star / RDF 1.2 triple terms.
- Any remote / network processing.

## Consumer
libcatalog Phase 0: per-Work N-Quads grains are written through this API so git
diffs are clean and PR-reviewable. See libcatalog ROADMAP Phase 0 and its
`tasks/002` (the identity map round-trips through the same canonical path).

## Status
Complete. `rdf/canon.go` implements RDFC-1.0 (URDNA2015) over a Dataset:
`Dataset.Canonical() ([]byte, error)`, `Canonicalize(*Dataset) (*Dataset, error)`
and `Graph.Canonical()`, with first-degree + n-degree hashing (SHA-256), the
graph term participating in blank labeling and the final sort, and a work budget
(`ErrCanonComplexity`) guarding the exponential worst case. Duplicate quads are
dropped (a dataset is a set). Validated against all 65 official W3C rdf-canon
RDFC-1.0 vectors (including the negative/poison graph), plus isomorphism
invariance, idempotence and a no-panic fuzz target. The `canonicalTriples` test
helper is retained as an independent oracle.
