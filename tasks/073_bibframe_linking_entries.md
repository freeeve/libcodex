# 073 -- bibframe: 76x-78x linking entries -> bf:relation

Tier 3 (high effort, structural). From the 059 m2b audit, linking area.
Ref: `docs/bibframe_m2b_audit.md` section 6; m2b `ConvSpec-760-788-Links.xsl`.

## Motivation

The whole 76x-78x linking-entry family is unhandled in both directions, so series,
preceding/succeeding titles, supplements, host items, and other work-to-work
relationships are lost on import. m2b models each as
`bf:relation -> bf:Relation` carrying a `.../vocabulary/relationship/<code>` IRI
(e.g. 780 ind2 -> precededby/mergerof/..., 785 ind2 -> succeededby/splitinto/...)
plus `bf:associatedResource` pointing at a related `bf:Work` (with nested
`bf:hasInstance`/`bf:Instance`, title from $s/$t/245, contributor from $a, ISSN
from $x). It is explicitly NOT bare `bf:precededBy`/`bf:succeededBy` predicates.

## Scope

1. Add a related-resource model: a `Relation{Relationship, Title, ...}` on the Work,
   emitting `bf:relation -> bf:Relation` with the relationship-vocab IRI and a
   `bf:associatedResource -> bf:Work` (flat: title + optional ISSN + creator label).
2. Map the common tags: 780/785 (continuation, ind2 -> relationship code), 773
   (host), 776 (other physical format), 490/8xx series -> `bf:hasSeries` (may be a
   separate task).
3. This requires minting per-relation IRIs for the related Work/Instance; today the
   IRI scheme (`shape.go`) has only one Work/Instance slot. Extend IRI minting to
   support `#Work760-1`-style fragments.
4. Reverse: reconstruct the 76x-78x field from the relation node.
5. Coordinate with task 062 (7xx $t name-title) so related works are modeled one way.

## Hazards

- Structural: touches IRI minting, not just the field switch -- the largest of the
  audit tasks. Land the relationship model + IRI scheme before adding many tags.
- Follow m2b's `bf:relation`/relationship-vocab shape; do not invent `bf:precededBy`.
- Sample has no 76x-78x, so goldens shouldn't move until a test adds one -- verify.

## Acceptance

- [ ] 780/785 (at least) -> `bf:relation` + relationship IRI + `bf:associatedResource`.
- [ ] Per-relation related-work IRIs minted; round-trips to the 76x-78x field.
- [ ] Sample golden unchanged; new linking tests; suite + fuzz green.
