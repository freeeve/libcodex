# 060 -- bibframe: subject bf:source (thesaurus) + subdivision reverse fidelity

Tier 1 (high value, low effort). From the 059 m2b audit, subjects area.
Ref: `docs/bibframe_m2b_audit.md` section 3; m2b `ConvSpec-600-662.xsl`.

## Motivation

We drop the subject thesaurus entirely: `appendSubject` records only a label, so
LCSH vs MeSH vs a local scheme is unrecoverable, and the reverse path hardcodes
`ind2='0'` (always LCSH). m2b derives `bf:source` from 6xx ind2 (0=LCSH,
1=children, 2=MeSH, 3=NAL, 5=CASH, 6=RVM) and, for ind2=4/7, from $2. This mirrors
what we already do for `Classification.Source` and `Identifier.Source`.

We intentionally keep the flat `--`-joined subject label rather than adopting
m2b's `madsrdf:ComplexSubject`/`componentList` -- that stays a non-goal. This task
adds only the cheap, faithful signals on top of the flat model.

## Scope

1. Add `Source string` to the `Subject` struct; populate in the 6xx cases from an
   ind2->URI table (and $2 for ind2=4/7). Emit `bf:source` in the subject path of
   `emitLabeled`/`emitWorkBody`, matching the classification source shape.
2. Route a subdivided 655 ($v/$x/$y/$z present) through the subject path instead
   of filing it as a flat `bf:genreForm`; attach the genre scheme source (lcgft).
3. Reverse fidelity: so subdivisions and non-LCSH thesauri round-trip, either
   preserve the subdivision subfield codes (not just `--`) or, at minimum, restore
   ind2/$2 from the recorded `bf:source` in `subjectFields`/`headingField`.

## Hazards

- Sample record has 650 ind2=0 (LCSH) and 655 ind2=7 $2; emitting `bf:source` will
  change `sample.rdf`/`sample.jsonld` -- regenerate goldens deliberately and eyeball.
- Keep the flat label output; do not introduce ComplexSubject nodes.

## Acceptance

- [ ] Subjects carry `bf:source` derived from ind2/$2; round-trips through ind2.
- [ ] Subdivided 655 becomes a subject, not a flat genreForm.
- [ ] New/updated golden + round-trip test; full suite + fuzz green.
