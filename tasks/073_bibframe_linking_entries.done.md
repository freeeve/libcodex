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

- [x] 780/785 (at least) -> `bf:relation` + relationship IRI + `bf:associatedResource`.
- [x] Related-work associated resource round-trips to the 76x-78x field.
- [x] Sample golden unchanged; new linking tests; suite + fuzz green.

## Result

Landed the common linking entries with a bidirectional relationship table:

- `Work.Relations []Relation{Relationship,Name,Title,ISSN}` (`bibframe.go`), fed by
  `appendRelation` from 773/776/780/785. A single `linkRelations` table is the
  source of truth for both directions: `relationCodeFor(tag,ind2)` (forward) and
  `relationField(code)` (reverse). 780 and 785 refine the relationship by second
  indicator (continues/continuesInPart/supersedes/absorbed/... and the succeeding
  inverses continuedBy/supersededBy/splitInto/...); 773 -> `partOf`, 776 ->
  `otherPhysicalFormat`.
- Emit (`shape.go`, `emitRelation`): `bf:relation -> bf:Relation` with a
  `bf:relationship` vocabulary IRI (`relationshipIRIVal`) and a
  `bf:associatedResource -> bf:Work` carrying the linked resource's title, an
  optional creator contribution ($a) and an optional `bf:Issn` ($x).
- Reverse (`reader_crosswalk.go`, `relationFields` + `relationshipCode`,
  `associatedName`, `associatedISSN`): recovers the tag/ind2 from the relationship
  IRI and the subfields from the associated Work. `relatedWorkSet` (reader.go) now
  also skips `bf:associatedResource` targets so a linked resource is not emitted as
  its own record.
- `normalize` sorts `Work.Relations` (reconstructed fields are tag-sorted, so
  relation order can differ across a round-trip).

### Divergence from m2b (deliberate)

m2b mints per-field Work/Instance IRIs (`#Work760-1`) for each linked resource.
This crosswalk keeps the flat model: the `bf:associatedResource` is a blank labeled
`bf:Work`, exactly like the name-title `bf:relatedTo` handling from 062. The IRI
scheme in `shape.go` was therefore left with its single Work/Instance slot -- the
structural IRI-minting work the task flagged is not needed under the flat model.

### Golden / tests

- Sample carries no 76x-78x, so the sample golden is byte-unchanged (verified with
  `UPDATE_GOLDEN=1`).
- New `linking_entry_test.go`: forward routing (780/785 ind2, 773, 776) and a
  round-trip proving each link returns to its tag with indicators and $a/$t/$x
  intact, and that no linked resource surfaces as its own record.
- Full suite plus `FuzzFromMARC` and `FuzzDecode` green.

### Remaining linking checklist (tracked follow-up)

Common tags landed (773/776/780/785). Still dropped:

- [ ] 760/762 (main/subseries), 765 (original language), 767 (translation).
- [ ] 770/772 (supplement / parent of supplement), 774 (constituent unit),
      775 (other edition), 777 (issued with).
- [ ] 786 (data source), 787 (generic relationship).
- [ ] 490/8xx series -> `bf:hasSeries` (separate; series statement, not a 76x link).
- [ ] 773 `$g`/`$q` enumeration -> `bf:part` on the associated resource.
