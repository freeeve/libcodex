# 071 -- bibframe: title completeness (245 nonSortNum, uniform $n/$p, 246)

Tier 2. From the 059 m2b audit, titles area.
Ref: `docs/bibframe_m2b_audit.md` section 1; m2b `ConvSpec-200-247not240-Titles.xsl`,
`ConvSpec-240andX30-UnifTitle.xsl`.

## Motivation

The 245 -> Instance path matches, but title coverage is incomplete:

- 245 ind2 (nonfiling characters) ignored; m2b emits `bflc:nonSortNum` = ind2 when
  ind2 in 1-9.
- 130/240 uniform title captures $a only; $n/$p (partNumber/partName) dropped
  (the `Title` struct already has PartNumber/PartName fields).
- 246 variant titles unhandled entirely -- no `bf:VariantTitle`/`bf:ParallelTitle`,
  no Instance cover/spine title.

## Scope

1. 245 ind2 1-9 -> a `NonSortNum` on `Title` -> `bflc:nonSortNum`.
2. Populate uniform-title PartNumber/PartName from 130/240 $n/$p.
3. Add a 246 case: `bf:VariantTitle` (ind2 space/0/2/3/5/6/7) or `bf:ParallelTitle`
   (ind2=1) on the Work; cover(4)/spine(8) on the Instance; $a/$b -> mainTitle/
   subtitle, $n/$p -> part, with a variant-type rdf:type from ind2.
4. Reverse: reconstruct 246 and the nonfiling indicator.

## Hazards

- Sample 245 ind2='0' and 240 has no $n/$p and there is no 246, so the sample
  goldens should NOT move -- verify byte-identical after (1)-(2). A 246 test record
  is needed for (3).
- Uniform title is a deliberate direct `bf:title` on the Work (no Hub); keep that.
- 246 placement Work-vs-Instance by ind2 must round-trip.

## Acceptance

- [ ] nonSortNum emitted for 245 ind2 1-9; uniform $n/$p carried.
- [ ] 246 -> VariantTitle/ParallelTitle on the correct entity; round-trips.
- [ ] Sample goldens unchanged for (1)-(2); new 246 test; suite + fuzz green.
