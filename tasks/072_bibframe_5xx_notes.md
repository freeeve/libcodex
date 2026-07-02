# 072 -- bibframe: 5xx note family -> bf:Note

Tier 3 (high breadth). From the 059 m2b audit, notes area.
Ref: `docs/bibframe_m2b_audit.md` section 6; m2b `ConvSpec-5XX.xsl`,
`ConvSpec-841-887.xsl` (856).

## Motivation

We read only 520 (-> `bf:summary`). The entire rest of the 5xx family is dropped:
500 (general note, ubiquitous), 504 (bibliography), 505 (`bf:tableOfContents`),
546 (language note), 508/511/524/525/533/534/546/... each with an m2b
`.../vocabulary/mnotetype/*` subtype, plus non-note 5xx that map to dedicated
properties (521 intendedAudience, 538 systemRequirement, 540/506
usageAndAccessPolicy, 502 dissertation, ...). This is the single biggest breadth
gap in the crosswalk.

## Scope

1. Add a general `Notes []Note{Type, Label}` on `Work` and/or `Instance`; route by
   m2b's work/instance split (505/508/511/546/520 -> Work; 500/504/... -> Instance).
2. Map the common tags first: 500 (general), 504 (biblio), 505
   (`bf:tableOfContents`), 546 (language note). Emit `bf:note -> bf:Note` with the
   `mnotetype/*` type IRI (or the dedicated property where m2b uses one).
3. Reverse: regenerate the 5xx tag from Note.Type.
4. 856 ind2 (separate, small): ind2=2 -> `bf:supplementaryContent`, ToC $3/$a ->
   `bf:tableOfContents`, rather than always `bf:electronicLocator`.

## Hazards

- Sample has a 520 (already handled) but no other 5xx, so adding the note
  infrastructure should NOT move the sample golden until a note tag is present in
  a test -- verify.
- Scope creep risk: this is a large family. Land the common tags (500/504/505/546)
  first; leave the long tail as a follow-up checklist in this file.
- A round-trippable Note.Type vocabulary is needed; base it on m2b's mnotetype codes.

## Acceptance

- [ ] 500/504/505/546 -> typed `bf:Note` (or dedicated property); round-trip.
- [ ] Sample golden unchanged; new note tests; suite + fuzz green.
- [ ] Remaining 5xx tags enumerated here as a tracked checklist.
