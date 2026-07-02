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

- [ ] 7xx with $t no longer yields a Contribution; the related title is preserved.
- [ ] Plain 7xx (no $t) still yields a Contribution (sample golden unchanged).
- [ ] Test covering a name-title 7xx; suite + fuzz green.
