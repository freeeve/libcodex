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

- [x] 500/504/505/546 -> typed `bf:Note` (or dedicated property); round-trip.
- [x] New note tests; suite + fuzz green.
- [x] Remaining 5xx tags enumerated here as a tracked checklist.

## Result

Landed the common note tags with a round-trippable `bf:noteType` vocabulary:

- `Work.Notes []Note{Type,Label}`, `Instance.Notes`, `Work.TableOfContents []string`.
- Forward (`bibframe.go`): 500 -> untyped `bf:Note` (Instance); 504 ->
  `bf:noteType "bibliography"` (Instance); 546 -> `bf:noteType "language"` (Work);
  505 -> `bf:tableOfContents` (Work). `noteTypeForTag`/`tagForNoteType` carry the
  type both ways.
- Emit (`shape.go`): `emitNotes` wraps repeated notes in a `beginList`, and
  `TableOfContents`/`Dimensions` now go through a new `litList` sink method so
  repeated bare literals serialize as a JSON-LD array (`["a","b"]`) instead of
  duplicate object keys, which the JSON-LD decoder silently collapses to the last
  value. This is the same duplicate-key class of bug fixed for provisions in 066.
- Reverse (`reader_crosswalk.go`): `noteFields` regenerates the 5xx tag from each
  note's `bf:noteType`; `TableOfContents` -> 505.
- Hardened `rdaIRIVal` (`isRDACode`): a 336/337/338 `$b` carrying IRI-breaking
  characters now yields a blank RDA node (label-only) instead of an unescaped,
  malformed node IRI in the XML sink -- found by `FuzzFromMARC`. The term still
  round-trips through its `rdfs:label`.

### Golden / tests

- Sample golden moved by one field only: `bf:dimensions` is now `["22 cm"]` (array)
  rather than `"22 cm"` -- a consequence of routing `Dimensions` through `litList`
  to close its latent duplicate-key bug, not of the note infrastructure. No 5xx
  note is present in the sample, so the note paths leave the sample untouched.
- New `note_test.go`: forward routing (500/504/546/505) and a multi-note,
  multi-505 round-trip proving repeated notes/ToC survive the JSON-LD array form.
- `FuzzFromMARC` and `FuzzDecode` green after the `rdaIRIVal` hardening.

### Remaining 5xx checklist (tracked follow-up)

Common note tags landed (500/504/505/546). Still dropped:

- [ ] 508 (creation/production credits) -> `bf:noteType`/`bf:CreditsNote`.
- [x] 511 (participant/performer) -> `bf:noteType "performers"` (Work). [081]
- [ ] 520 ind1 nuance -- currently always `bf:summary`; m2b splits abstract/review.
- [x] 521 (target audience) -> `bf:noteType "audience"` (Work; typed note rather than `bf:intendedAudience`, keeping the flat model). [081]
- [ ] 524 (preferred citation) -> `bf:citation`/note.
- [x] 533 -> `bf:noteType "reproduction"` (Instance; label joins every subfield). [081]
- [ ] 525 (supplement note), 534 (original version note).
- [ ] 502 (dissertation) -> `bf:dissertation`.
- [ ] 506/540 (access/use policy) -> `bf:usageAndAccessPolicy`.
- [x] 538 -> `bf:noteType "systemDetails"` (Instance; typed note rather than `bf:systemRequirement`). [081]
- [ ] 856 ind2 refinement: ind2=2 -> `bf:supplementaryContent`, ToC $3/$a ->
      `bf:tableOfContents`, rather than always `bf:electronicLocator` (separate,
      small; not started).
