# 065 -- bibframe: classification fidelity (item portion, assigner, 072/037 shape)

Tier 2. From the 059 m2b audit, classification area.
Ref: `docs/bibframe_m2b_audit.md` section 4; m2b `ConvSpec-050-088.xsl`,
`ConvSpec-010-048.xsl`.

## Motivation

Our classification handling collapses several m2b signals:

- 050: `joinSub(f,"ab"," ")` merges $a and $b into a single
  `bf:classificationPortion`; m2b puts $a -> `bf:classificationPortion`, $b ->
  `bf:itemPortion`, one node per $a. 050 ind2/ind1 -> `bf:assigner`/`bf:status` dropped.
- 082: $2 dropped (we pass source=""), and $b -> `bf:itemPortion`, ind1 ->
  `bf:edition` (full/abridged), ind2/$q -> `bf:assigner` unhandled.
- 084: $b -> `bf:itemPortion`, $q -> `bf:assigner` dropped.
- 072: mapped to `bf:Classification`; m2b treats 072 as a `bf:subject`/`bf:Topic`
  category (with `bf:code`). Reconsider whether these belong under bf:subject.
- 037: flattened to `bf:Identifier` with $b as source; m2b builds
  `bf:acquisitionSource -> bf:AcquisitionSource`. Defensible but mislabels agency.

## Scope

- Add `ItemPortion string` to `Classification`; split $b into it for 050/082/084.
- Pass the Dewey $2 through for 082; add `bf:edition` from 082 ind1.
- Capture an assigner (050 ind2=0/DLC, 040 $a) where cheap.
- Decide 072: keep as classification (document divergence) or move to bf:subject
  with a `bf:code`. Align with task 060 if moved.
- Decide 037: keep flat (document) or model `bf:acquisitionSource`.

## Hazards

- Sample 050 is `$a PS3556 $b .E446` -> splitting into item portion changes the
  golden classificationPortion; regenerate deliberately. 082 has no $2 in sample.
- Several of these are low-frequency; prioritize 050 $b split + 082 $2.

## Acceptance

- [ ] 050/082/084 $b -> `bf:itemPortion`; 082 $2 -> `bf:source`.
- [ ] 072/037 divergence either fixed or explicitly documented in the audit note.
- [ ] Goldens regenerated + reviewed; round-trip + fuzz green.
