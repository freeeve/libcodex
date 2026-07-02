# 054 -- bibframe: decode a Work with multiple Instances

## Motivation

Task 038 lets a caller emit one Work with N Instances into a single grain
(`WorkInstances.Graph`). The reverse crosswalk does not yet round-trip that shape:
`recordFromWork` (reader_crosswalk.go) binds a single Instance per Work -- via
`g.Object(work, pHasInstance)`, falling back to the precomputed `instanceBackrefs`
map -- so decoding a multi-instance document silently drops all but one Instance
(finding 048-2).

## Decision required before implementing

MARC is one-record-per-resource, so a one-Work/N-Instance graph has no single
obvious MARC form. Pick the policy:

- **A. One record per Work+Instance pair** -- each Instance yields its own MARC
  record carrying the shared Work fields plus that Instance's fields (260/300/020/
  856/...). N Instances -> N records. Closest to how union catalogs hold
  manifestation-level records; duplicates the Work fields across records.
- **B. Merge into one record** -- a single MARC record with repeated
  instance-level fields (multiple 260/300/020/...). One Work -> one record.
  Compact, but conflates distinct manifestations and is lossy on which subfields
  belong to which Instance.

Recommendation: **A** (one record per pair), matching the forward grain's intent
and libcatalog's manifestation-level identity model.

**Decision: A (one record per Work+Instance pair).** Chosen on the task's own
recommendation; can be revisited if a consumer needs the merged form. A Work with
no Instance still yields a Work-only record, and single-Instance decode is
unchanged.

## Change (once the policy is chosen)

- `Decode`: iterate `g.Objects(work, pHasInstance)` (and the `instanceBackrefs`
  map, extended to `map[Work][]Instance`) rather than taking the first Instance.
- Emit records per the chosen policy; a Work with zero Instances still yields a
  Work-only record (current behavior).
- Keep single-instance decoding byte-for-byte unchanged.

## Acceptance

- [x] Policy A chosen and recorded above.
- [x] `Decode` of a `WorkInstances.Graph` (and `WorkInstances.RDFXML`) document
      with 2 Instances yields one record per Instance, each with the shared Work
      fields plus its own (`TestDecodeMultiInstance`).
- [x] Existing single-instance decode tests and `FuzzDecode` unchanged/passing.

Depends on: 038 (done). Cross-references: 048 problem 2.

## Resolution

`instanceBackrefs` (map[Work]Instance, first-wins) is replaced by
`instancesByWork` (map[Work][]Instance), which unions the bf:hasInstance and
bf:instanceOf links in one pass, deduplicated and in document order. `Decode`
iterates a Work's Instances and emits one record per Instance via the renamed
`recordFromWorkInstance(g, work, inst, hasInst)`; a Work with zero Instances
emits a Work-only record. Single-Instance decode is byte-for-byte unchanged (one
Work + one Instance still yields exactly one record built from the same pair).
