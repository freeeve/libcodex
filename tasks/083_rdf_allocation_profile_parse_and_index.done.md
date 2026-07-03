# 083 -- rdf: allocation profile of ParseNQuads and Graph.index at corpus scale

## Context (filed from libcatalog)

libcatalog benchmarked its projector over a real 51MB / ~325k-quad corpus
(its `project/bench_test.go`, `LCAT_BENCH_CATALOG=...`). After it fixed its
own graph assembly (exact-capacity merges instead of per-triple Add growth:
800ms -> 349ms, 1.46GB -> 0.56GB per projection), the remaining allocation
is rdf-internal:

- `Graph.index` -- 52% of remaining bytes (~280MB/run): the lazy
  `map[Term][]Triple` subject index over a corpus-scale graph allocates one
  slice per subject with append growth. Possible shapes: count-first
  two-pass build (size each subject's slice exactly), or one shared
  `[]Triple` arena sorted/grouped by subject with the map holding
  sub-slices.
- `ParseNQuads` -- 28% (~150MB/run): per-term string allocations. The
  existing `arena.go` machinery may already be adaptable; an opt-in
  arena-backed parse (`ParseNQuadsArena`?) would let hot read-only
  consumers (projector, exporters) skip most per-term allocations, at the
  documented cost of the arena's lifetime rules.

Neither is a correctness issue; both are steady-state GC pressure for any
consumer that parses and indexes whole corpora. Numbers reproduce with:

    cd ../libcatalog
    LCAT_BENCH_CATALOG=<corpus>/catalog.nq \
      go test ./project/ -run xxx -bench BenchmarkProject -benchmem \
      -memprofile mem.prof

## Acceptance

- A corpus-scale benchmark in rdf (parse + index) with -benchmem numbers
  before/after.
- Whichever of the two shapes above (or better) lands, libcatalog's
  BenchmarkProject should show the improvement with no API break -- or the
  opt-in arena API documented for it to adopt.

## Results (2026-07-02)

Profiling showed the parse side was not per-term strings -- the arena already
covers those; whole-corpus ParseNQuads was 5 allocs/op: the private
`string(data)` copy plus the returned Quads slice. The index side was as
suspected: per-subject `[]Triple` append growth plus an oversized map.

What landed:

- `Graph.index` rebuilt: two-pass count-first build over one shared `[]int32`
  arena, buckets carved at exact size, holding positions into `g.Triples`
  instead of Triple copies. No API change.
- `ParseNQuadsShared` / `ParseNTriplesShared`: opt-in zero-copy variants that
  back terms with the caller's buffer instead of a private copy; contract
  documented (caller must not modify data while the result is live).
  Differential fuzz target (FuzzParseNQuadsShared) guards the unsafe path.

Corpus-scale benchmarks (rdf/corpus_bench_test.go, ~330k quads / ~57MB,
Apple M3 Max):

    BenchmarkCorpusParseNQuads          109ms   131.6MB/op        5 allocs   (before == after; semantics unchanged)
    BenchmarkCorpusParseNQuadsShared     74ms    74.1MB/op        4 allocs   (-44% bytes vs ParseNQuads)
    BenchmarkCorpusIndex        before   95ms   156.1MB/op   131,026 allocs
    BenchmarkCorpusIndex        after    55ms    16.2MB/op       388 allocs  (-90% bytes, -40% time)

libcatalog BenchmarkProject (real 430K QLL catalog.nq, provider "marc",
replace -> local libcodex, no libcatalog code change):

    before  4.87MB/op   3,407 allocs/op
    after   3.06MB/op     778 allocs/op   (-37% bytes, -77% allocs)

Adoption of ParseNQuadsShared (the remaining input-copy saving) filed as a
task in libcatalog.
