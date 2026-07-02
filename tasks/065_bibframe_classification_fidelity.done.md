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

- [x] 050/082/084 $b -> `bf:itemPortion`; 082 $2 -> `bf:source`.
- [x] 072/037 divergence either fixed or explicitly documented in the audit note.
- [x] Goldens regenerated + reviewed; round-trip + fuzz green.

## Result

`Classification` gained `ItemPortion` (bf:itemPortion) and `Edition` (bf:edition).
Forward: 050 and 082 go through a new `appendCallNumber` ($a -> portion, $b -> item
portion); 082 also passes its $2 as source and maps ind1 0/1 -> full/abridged via
`deweyEdition`. 084 keeps its repeated-$a loop but now carries $b as the item
portion and $2 as source. `emitClassification` emits bf:itemPortion and bf:edition
when present. Reverse (`classificationFields` + `callNumberSubs` + `deweyInd1`)
restores $a/$b, the 082 $2, and the 082 edition indicator. The old `joinSub("ab")`
merge is gone (helper retired).

Decisions documented in the audit (section 4), not restructured:
- 072 stays a source-qualified `bf:Classification` (moving to bf:subject/bf:Topic +
  bf:code would need subject-path plumbing our flat model does not carry).
- 037 stays flat `bf:Identifier` with $b as source (bf:acquisitionSource node
  deferred). Both preserve the data and round-trip.
- Deferred, low-frequency: 050/082 assigner/status, 084 $q, 020 $c, 022 $l/$m,
  dereferenceable source IRIs.

Goldens: sample 050 `$a PS3556 $b .E446` now splits into
`bf:classificationPortion PS3556` + `bf:itemPortion .E446` (was the merged
"PS3556 .E446"); 082 has no $b/$2 so it is unchanged. Regenerated the RDF/XML and
JSON-LD goldens and updated the well-formed assertion. Tests:
`classification_fidelity_test.go` (050/082/084 split + source + edition, plus a
050/082 round-trip incl. the abridged indicator). Suite + FuzzFromMARC + FuzzDecode
green.
