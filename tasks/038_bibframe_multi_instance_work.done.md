# 038 -- BIBFRAME: a Work with multiple Instances, caller-controlled ids

## Motivation

`bibframe.BIBFRAME` models exactly one Instance per Work
(`struct { Work Work; Instance Instance }`), and `BIBFRAME.Graph(base)` ties both
nodes to a single base (`#<base>Work`, `#<base>Instance`). That fits a single MARC
record, but not the consumer's core model.

libcatalog's identity layer (ARCHITECTURE §4) is two-tier: **one Work groups many
Instances** (editions, formats, translations), each with its own opaque, minted
id, independent of the Work's id. It needs to serialize a Work with N Instances
into one grain, supplying its own ids for the Work **and** each Instance -- which
the current single-base, single-Instance API cannot express. This blocks
libcatalog Phase 1 (clustering + stable identity, its `tasks/001`/`tasks/002`).

## Change

Let a caller assemble a Work with 0..N Instances into an `rdf.Graph`, controlling
the id of every node. Suggested shape (final form is the author's call):

```go
// A Work and the Instances that realize it.
type WorkInstances struct {
    Work      Work
    Instances []Instance
}

// Graph assembles the Work at #<workBase>Work and each instance at
// #<instanceBases[i]>Instance, linked bf:hasInstance / bf:instanceOf.
// len(instanceBases) must equal len(Instances).
func (wi *WorkInstances) Graph(workBase string, instanceBases []string) *rdf.Graph
```

Requirements:

- Work-level triples emitted **once**; each Instance's triples emitted under its
  own IRI; `bf:hasInstance` (Work->each Instance) and `bf:instanceOf` (each
  Instance->Work) links present.
- Work id and Instance ids are **independent** (distinct bases), so a caller can
  use minted opaque ids at both tiers.
- Blank-node labels stay **unique across the whole grain** (one graphBuilder /
  blank counter for the Work + all Instances), so RDFC-1.0 canonicalization is
  stable.
- **Backward compatible**: existing `BIBFRAME`, `FromRecord`, and `Graph(base)`
  unchanged. `FromRecord(r).Graph(base)` keeps emitting identical bytes.
- Priority target is the `rdf.Graph` / N-Quads path (`graph.go`) -- that is what
  libcatalog serializes. RDF/XML and JSON-LD multi-instance output can follow;
  note if deferred.

## Acceptance

- [x] A Work with 2 Instances -> one Work node + two Instance nodes, correct
      hasInstance/instanceOf both ways, distinct instance IRIs.
      (`TestWorkInstancesGraphStructure`.)
- [x] Independent work/instance bases honored in the node IRIs, each sanitized.
      (`TestWorkInstancesGraphStructure`, `TestWorkInstancesBaseSanitized`.)
- [x] Blank labels unique across all instances; RDFC-1.0 output deterministic.
      (`TestWorkInstancesBlankNodesDistinct`, `TestWorkInstancesCanonicalDeterministic`.)
- [x] Existing single-instance tests and golden output unchanged (the `work()`
      refactor keeps `bf:hasInstance` the last Work-subject triple, so the
      single-instance graph is byte-identical; full suite + `TestGolden` pass).

## Resolution

- Added `WorkInstances{Work, []Instance}` and
  `(*WorkInstances).Graph(workBase string, instanceBases []string) *rdf.Graph`
  (graph.go): the Work is emitted once, each Instance under its own sanitized base
  with `bf:hasInstance` / `bf:instanceOf` both ways, all built with one
  `graphBuilder` so blank labels are unique across the grain.
- Refactored `graphBuilder.work` to take `*Work` and no longer emit
  `bf:hasInstance` (the caller emits one per Instance); `instance` now takes
  `*Instance`. The single-instance `graphFromBIBFRAME` path is byte-identical.
- Scope: this is the priority `rdf.Graph` / N-Quads path. Multi-instance RDF/XML
  and JSON-LD output is deferred to **task 053**; decoding a multi-instance
  document back to MARC (needs an N-records-vs-merge policy decision) is **task
  054**. `BIBFRAME`, `FromRecord`, and `Graph(base)` are unchanged.

Consumer: libcatalog Phase 1 (`identity/`, `tasks/001`, `tasks/002`).

## Prerequisites from task 048 (structural groundwork, done)

Task 048 landed the structural fixes that make this task safe to start:

- **Finding 048-4 (base sanitization):** `BIBFRAME.Graph(base)` now applies
  `sanitizeID`, so caller-supplied bases at both tiers cannot mint invalid node
  IRIs. `WorkInstances.Graph(workBase, instanceBases)` must apply the same rule to
  every base it accepts.
- **Finding 048-5 (blank-label collisions):** `graphBuilder.fresh()` namespaces
  blank labels by the node base. A grain assembled from separate `Graph` calls
  would still collide, so this task must build the Work + all its Instances with
  **one** `graphBuilder` (one blank counter) as the requirements above state.
- **Finding 048-2 (reader binds one Instance per Work):** `recordFromWork` still
  reads a single Instance (via the precomputed `instanceBackrefs` map). Once this
  task emits one-Work/N-Instance grains, `Decode` must iterate
  `g.Objects(work, pHasInstance)` and this task's acceptance must fix the policy
  (one record per Work+Instance pair, or merge). Decide it here.
