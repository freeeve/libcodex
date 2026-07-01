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

- [ ] A Work with 2 Instances -> one Work node + two Instance nodes, correct
      hasInstance/instanceOf both ways, distinct instance IRIs.
- [ ] Independent work/instance bases honored in the node IRIs.
- [ ] Blank labels unique across all instances; RDFC-1.0 output deterministic.
- [ ] Existing single-instance tests and golden output unchanged.

Consumer: libcatalog Phase 1 (`identity/`, `tasks/001`, `tasks/002`).
