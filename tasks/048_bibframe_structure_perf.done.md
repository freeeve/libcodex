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

- [x] A node-shape change (e.g. a new provision property) lands in exactly
      one place and all three serializations pick it up.
      **(P1 -- moved to task 055 with a sharpened design; see below.)**
- [x] Decode of an aggregated LoC-shaped document shows linear scaling
      (`instanceBackrefs` precomputes the Work->Instance map once; the per-Work
      `SubjectsOfType` scan is gone).
- [x] `Graph("my id")` sanitizes the base with `sanitizeID` -- documented on the
      method.
- [x] Stream-writer allocations drop (`BenchmarkNTriplesWriterStream`:
      6946 KB/op -> 5730 KB/op with the reused buffer).
- [x] bibframe.go and reader.go each under 500 lines (bibframe.go 486 +
      bibframe_writer.go 199; reader.go 259 + reader_crosswalk.go 482).
- [x] Findings 2, 4, 5 cross-referenced from task 038 before it starts.

## Resolution (P2-P7 done; P1 split out to task 055)

Landed the concrete, output-preserving structural fixes:

- **P2 (reader groundwork):** cross-referenced into task 038; `recordFromWork`
  took the precomputed backref map. **Completed by task 054**, which chose
  policy A (one record per Work+Instance pair) and made `Decode` iterate every
  Instance via `instancesByWork` -- the "silently drops N-1 instances" finding
  is fully closed.
- **P3:** `instanceBackrefs` builds the Work->Instance map in one pass; removed
  the O(works x instances) `instanceBackref` scan.
- **P4:** `BIBFRAME.Graph(base)` sanitizes the base (documented).
- **P5:** `graphBuilder.fresh()` namespaces blank labels by the base so
  separately built graphs merge without `_:b1` collisions (output byte-stable --
  the serializers relabel blanks).
- **P6:** the N-Triples/Turtle/N-Quads collection writers reuse a per-writer
  buffer instead of `Append*(nil, ...)` per record.
- **P7:** split `bibframe.go` -> `bibframe.go` + `bibframe_writer.go`, and
  `reader.go` -> `reader.go` + `reader_crosswalk.go`; all four under 500 lines.

**Update (task 053):** the multi-instance RDF/XML and JSON-LD work extracted
shared body helpers (`appendWorkBodyXML`/`appendWorkHeadJSONLD`,
`appendInstanceNodeXML`/`appendInstanceNodeJSONLD`), so the Work/Instance node
shape is now written once per format rather than twice (single- and
multi-instance). This shrinks -- but does not close -- P1: the three formats are
still parallel emitters.

**P1 (unify the three emitters) is split out as task 055.** The original
deferral rationale ("both options change or risk the hand-tuned output") has
been sharpened by the 039/053 work: deriving RDF/XML and JSON-LD generically
from the `rdf.Graph` is *rejected* (a generic serializer cannot reproduce the
curated LoC-shaped output), while a shared shape declaration rendered three
ways *can* stay byte-identical and is the approach task 055 specifies, with
concrete hazards and acceptance criteria. Until 055 lands, a node-shape
addition still touches three files, guarded by `TestEncodersIsomorphic` and
`TestGolden` -- a maintenance tax, not a correctness risk. This task is closed:
P2-P7 are done here (P2 finished by 054) and P1 is tracked in 055.
