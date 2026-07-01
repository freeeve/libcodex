# 048 -- bibframe: single graph authority, reader indexing, 038 groundwork

## Motivation

Structural findings from the review that directly affect task 038
(multi-instance Works): three hand-written emitters must be changed in
lockstep, blank-node counters and caller-supplied bases are unsafe to
compose, and the reader assumes exactly one Instance per Work. Plus two
measured hot-path costs.

## Problems

1. **Three parallel emitters encode the same shape by convention**
   (rdfxml.go:14-76, jsonld.go:15-192 vs graph.go:67-162; graph.go:11-12
   admits they must "mirror ... exactly"). Commit 64907fc had to add
   `bf:source` in six places. Every 038 change lands three times or gets
   deferred per format. Fix: make graph.go the single authoritative
   traversal and derive RDF/XML and JSON-LD from the `rdf.Graph` (the rdf
   package already has per-format serializers), or extract the node-shape
   declaration so it is written once. `TestEncodersIsomorphic` remains as
   the safety net during the migration.
2. **Reader binds exactly one Instance per Work** (reader.go:187-190).
   `g.Object(work, pHasInstance)` takes the first `bf:hasInstance`; once
   038 emits one-Work/N-Instance grains, `Decode` of libcodex's own output
   silently drops N-1 instances. Decide the policy (one record per
   Work+Instance pair, or merge) as part of 038's acceptance; implement
   `g.Objects` iteration here.
3. **`instanceBackref` is O(works x triples)** (reader.go:517-524). Per
   Work lacking `bf:hasInstance`, a full `SubjectsOfType` scan plus fresh
   map/slice allocations (rdf.go:155-167). Precompute one Work-to-Instance
   map from a single pass over `g.Triples` in `Decode`.
4. **Exported `BIBFRAME.Graph(base)` (f55ae1b) does not sanitize the base**
   (graph.go:32-34). A base containing space/`#`/`/` produces invalid IRIs
   (`#my idWork`) and breaks `controlNumber` recovery. 038 multiplies the
   exposure (caller ids at both tiers). Apply `sanitizeID` or reject bad
   bases; same rule for the future `WorkInstances.Graph`.
5. **Per-builder blank counters collide on graph merge** (graph.go:41-44).
   Every `graphBuilder` restarts at `b1`, so merging two built graphs'
   triples (exactly what a 038 grain assembled from separate `Graph(base)`
   calls would do) merges unrelated `_:b1` nodes. 038 already requires one
   builder per grain; additionally consider base-prefixed blank labels so
   independently built graphs stay merge-safe.
6. **Stream writers allocate a fresh buffer per record** (serialize.go:61,
   :95, :133). `AppendNTriples(nil, ...)` etc.; the RDF/XML and JSON-LD
   writers already reuse `wr.buf` (bibframe.go:515, :579-583). Add the same
   reused buffer to the three writers.
7. **File-length convention** (bibframe.go 649 lines, reader.go 658).
   Clean seams exist: crosswalk vs writer boilerplate in bibframe.go
   (writers are near-identical structs whose `writeAll`/`Close` can
   collapse behind one embedded type); sniffing/entry points vs reverse
   crosswalk in reader.go. Pure moves, no API change.

## Acceptance

- [ ] A node-shape change (e.g. a new provision property) lands in exactly
      one place and all three serializations pick it up;
      `TestEncodersIsomorphic` passes throughout.
- [ ] Decode of an aggregated LoC-shaped document shows linear scaling
      (benchmark before/after on loc_stress corpus).
- [ ] `Graph("my id")` either sanitizes or errors -- documented.
- [ ] Stream-writer allocations drop in `BenchmarkWriterStream`
      (`-benchmem` before/after).
- [ ] bibframe.go and reader.go each under 500 lines.
- [ ] Findings 2, 4, 5 cross-referenced from task 038 before it starts.
