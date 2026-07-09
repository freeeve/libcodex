# 098 -- zero-copy Dataset.GraphView: Dataset.Graph copies dominate libcat's per-grain and projection allocation profiles

Filed from libcat on 2026-07-09 (cross-repo ask).

From libcat tasks/121 (profile-first allocation pass; per repo
convention, profiles pointing into libcodex get filed here rather than
patched around).

Measured over a real 62,602-work corpus (Apple M3 Max, alloc_space):

- Per-grain paths, 2,000 real grains: rdf.(*Dataset).Graph is the top
  flat allocator in BOTH identity.ScanGrain (33.7%) and (before a
  local fix) ingest.SummarizeGrain (27.8%) -- these run once per grain
  at workindex boot/refresh and in batch scans. ScanGrain's per-graph
  QUERY semantics are load-bearing (feed vs editorial separation), so
  libcat cannot merge its way around the copy there.
- Corpus scale: one full-catalog Project run allocates 8.6GB, of which
  rdf.parseNQuads is 40% (the Dataset representation itself; input
  copy already avoided via ParseNQuadsShared) and libcat's own
  graph-splitting copy is 30% -- also avoidable with a graph-scoped
  view instead of materialized []Triple.

Ask: a read-only graph view over a Dataset -- Dataset.GraphView(g) or
equivalent -- exposing the Graph query surface (SubjectsOfType,
Object/Objects, Literal, HasType) without materializing a []Triple
copy per graph. Positions-into-Quads (like Graph's int32 index arena)
would fit. If the quads slice were additionally segmented per graph at
parse (canonical N-Quads interleave graphs, so this means bucketing),
views become slices and libcat's splitGraphs disappears too -- but the
query-surface view alone removes the top per-grain allocator.

No urgency signal: libcat's own fixable share is landed (v0.56.0);
this is the remaining headroom, worth it before the 10M-work tier
(libcat tasks/085 sizing: memory is the wall).

## Outcome

Done in 7d71279, shipped in v0.19.0. `Dataset.GraphView(graph Term)`
returns a `*GraphView` answering the full query surface over int32
positions into the dataset's own `Quads` -- the positions-into-Quads
shape the ask suggested. `Dataset.Graph` is untouched and remains
correct when a caller needs to own or mutate the triples.

Measured on the corpus bench (10k works, ~325k quads), split one named
graph and query it:

    scan (SubjectsOfType)    26.4ms  253MB  ->   3.3ms   5.0MB
    subject lookup (Object)  46.6ms  264MB  ->  26.8ms  11.5MB

Both paths are covered, because only measuring the scan would have
flattered the change: ScanGrain leans on Object/Literal, and those do
build an index on both paths.

### One design correction, caught by benchmarking

The obvious implementation -- build the subject index eagerly on first
touch, like `Graph.index()` -- was **slower than `Dataset.Graph`**
(30.8ms vs 23.9ms) despite allocating 14x less. `Graph.SubjectsOfType`
never builds the subject index; it scans. So a view whose every query
forced a `map[Term]` build paid for an index the hot call does not use.

Split the laziness instead: the subject-keyed lookups
(Object/Objects/Literal/HasType) build and cache the index; the
whole-graph scans (Len/Triples/SubjectsOfType) never trigger it and
allocate nothing but their result. That is what turned a 1.3x
regression into an 8x speedup on the scan path.

### Also landed

- `GraphQuery` interface naming the surface `*Graph` and `*GraphView`
  share, with compile-time assertions on both, so libcat can write one
  function over a materialized graph and a view alike.
- `GraphView.Triples()` is an `iter.Seq[Triple]` yielding from the
  dataset's quads with no slice materialized -- this is what lets
  libcat's `splitGraphs` go away without parse-time segmentation.
- Appending to the dataset invalidates a view's cached index; the next
  query rebuilds it. In-place quad mutation does not, the same contract
  `Graph` already has.

### Deliberately not done: parse-time segmentation

The ask floats bucketing quads per graph at parse so views become plain
slices. Skipped: `Dataset.NQuads` writes quads in slice order, so
bucketing would silently reorder the output of
`ParseNQuads(x).NQuads()` for any document whose graphs interleave --
an observable behavior change to serialization, for a second-order
allocation win. The ask itself notes the query-surface view alone
removes the top per-grain allocator, and the numbers above bear that
out. If libcat later wants segmentation, it should be a separate
opt-in constructor rather than a change to how `ParseNQuads` orders a
dataset.

Filed libcat tasks/209 as the notice.
