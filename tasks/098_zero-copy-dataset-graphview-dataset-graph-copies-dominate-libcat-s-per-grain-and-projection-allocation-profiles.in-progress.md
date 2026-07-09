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
