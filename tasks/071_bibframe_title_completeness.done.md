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

- [x] nonSortNum emitted for 245 ind2 1-9; uniform $n/$p carried.
- [x] 246 -> VariantTitle/ParallelTitle on the correct entity; round-trips.
- [x] Sample goldens unchanged for (1)-(2); new 246 test; suite + fuzz green.

## Result

`Title` gained `NonSortNum` (245 ind2 1-9 -> `bflc:nonSortNum`), and 130/240 now
populate `PartNumber`/`PartName` from $n/$p (the struct already had the fields; the
reverse `titleSubfields` already emitted $n/$p, so uniform parts round-trip for
free). The 245 reverse sets the second indicator from `NonSortNum` (`titleInd2`).

New `VariantTitle` type and `Work.VariantTitles`/`Instance.VariantTitles`. 246 ->
`addVariantTitle`: a `bf:ParallelTitle` (ind2=1) or `bf:VariantTitle` (else) with a
`bf:variantType` token from ind2 (`variantType`); cover(4)/spine(8) go on the
Instance, the rest on the Work. `emitVariantTitle` renders them under `bf:title`
alongside the main titles. Reverse `variantTitleFields` collects the
variant/parallel nodes into 246 with the indicator restored (`ind2ForVariant`); the
main-title reader `firstTitle` now iterates and skips variant/parallel-typed nodes
so a 246 never masquerades as the 245/130/240.

Goldens unchanged: the sample's 245 ind2='0' (no nonSortNum), 240 has no $n/$p, and
there is no 246. Tests: `title_completeness_test.go` (nonSortNum + uniform parts,
246 variant/parallel/cover placement + RDF/XML + JSON-LD round-trip). Suite +
FuzzFromMARC + FuzzDecode green.

Deferred (documented): 130/240 $l/$f/$s/$m/$r/$o; 210/222/242/243/247; 245 $f/$g/$s.
