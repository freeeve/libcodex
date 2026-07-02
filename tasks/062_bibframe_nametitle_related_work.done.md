# 062 -- bibframe: 7xx $t name-title -> related work, not contribution

Tier 1 (correctness). From the 059 m2b audit, contributions area.
Ref: `docs/bibframe_m2b_audit.md` section 2; m2b `ConvSpec-1XX,7XX,8XX-names.xsl`
mode `work7XX`.

## Motivation

A 7xx added entry that carries a $t is a name-title (author + work title) pointing
at a *related work* -- e.g. 700 1 2 $a Author $t Title. We unconditionally build a
`bf:Contribution` from $a, producing a spurious contributor and silently dropping
the related work. m2b routes a 7xx-with-$t to a related-work relation, not a
contribution.

## Scope

- In the 100/110/111/700/710/711 handling, when the field has a $t, do **not**
  emit a Contribution.
- Model the related work at the flat level this library already uses: a related
  title (and the name as its creator label) under an appropriate predicate. Decide
  the target during implementation -- a lightweight `bf:relatedTo`/related-work
  title is acceptable; full `bf:relation`/Hub is a non-goal (that is task 073's
  linking-entry shape and remains out of scope here). At minimum, stop emitting the
  bogus contributor and preserve the title text so it is not lost.
- Reverse: reconstruct the 7xx $a/$t from whatever related-work shape is chosen.

## Hazards

- Sample 700 has no $t (it is a plain added entry), so the common path is
  unaffected and goldens should not move for the sample.
- Coordinate the chosen related-work predicate with task 073 so the two don't
  model relations two different ways.

## Acceptance

- [x] 7xx with $t no longer yields a Contribution; the related title is preserved.
- [x] Plain 7xx (no $t) still yields a Contribution (sample golden unchanged).
- [x] Test covering a name-title 7xx; suite + fuzz green.

## Result

`appendContribution` now branches on a $t: the field routes to `appendRelatedWork`
(a new `Work.RelatedWorks []RelatedWork`) instead of emitting a contributor. The
name label is the subfields before the first $t (`agentLabel` gained a stop-at-$t
that is a no-op for plain contributions, which never carry $t), the title is $t.

Shape: `emitRelatedWork` emits `bf:relatedTo -> bf:Work` carrying the linking name
as a (primary for 1xx) `bf:contribution` and the referenced title as `bf:title`,
reusing `emitContribution`/`emitTitle`. Reverse: `relatedWorkFields` reads each
bf:relatedTo work's creator agent (label -> $a, class -> tag/ind1 via
`contribTag`/`ind1ForClass`, primary typing -> 1xx vs 7xx) and title mainTitle ->
$t.

Decode fix: `Decode` iterates every bf:Work as a record, so a nested related work
would surface as a spurious second record. `relatedWorkSet` collects bf:relatedTo
objects and the Decode loop skips them -- this also front-runs task 073's linking
targets. Round-trip stays one record.

Scope kept to the flat model: the related work's title is $t only (partNumber/part
subfields $n/$p left for later to avoid the x10/x11 name-vs-title $n ambiguity);
full bf:relation/Hub remains task 073. Predicate chosen is `bf:relatedTo` -- 073
should model 76x-78x linking entries under the same relation predicate rather than
a second scheme.

Sample golden unchanged (no name-title $t in the sample). Tests:
`related_work_test.go` (7xx $t -> related work not contribution, plain 7xx
untouched, 100/710 name-title round-trip + single-record check). Suite +
FuzzFromMARC + FuzzDecode green.
