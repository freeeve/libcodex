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
and libcatalog's manifestation-level identity model. Confirm before coding.

## Change (once the policy is chosen)

- `Decode`: iterate `g.Objects(work, pHasInstance)` (and the `instanceBackrefs`
  map, extended to `map[Work][]Instance`) rather than taking the first Instance.
- Emit records per the chosen policy; a Work with zero Instances still yields a
  Work-only record (current behavior).
- Keep single-instance decoding byte-for-byte unchanged.

## Acceptance

- [ ] Policy A or B chosen and recorded here.
- [ ] `Decode` of a `WorkInstances.Graph` document with 2 Instances yields the
      chosen record shape; round-trip test added.
- [ ] Existing single-instance decode tests and `FuzzDecode` unchanged/passing.

Depends on: 038 (done). Cross-references: 048 problem 2.
