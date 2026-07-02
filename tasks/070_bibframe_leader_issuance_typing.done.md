# 070 -- bibframe: leader/07 issuance + Monograph/Serial, audio subclasses

Tier 2 (low/med). From the 059 m2b audit, leader typing area.
Ref: `docs/bibframe_m2b_audit.md` section 5; m2b `ConvSpec-LDR.xsl`.

## Motivation

`workClass` maps leader/06 to the Work content class (mostly matching m2b), but:

- leader/06 i/j collapse to a single `Audio`; m2b distinguishes i->`bf:NonMusicAudio`,
  j->`bf:MusicAudio`. "Audio" is coarser and loses the music distinction.
- leader/06 q -> nothing (m2b types q as `bf:Hub`); manuscript leaders (d/f/t) get
  no secondary `bf:Manuscript` type.
- leader/07 issuance is unused: m2b adds a Work rdf:type
  `bf:Monograph`/`Serial`/`Collection`/`Integrating` and an Instance `bf:issuance` IRI.

## Scope

1. Split i/j into NonMusicAudio/MusicAudio in `workClass` (and the reverse
   `leaderForClass`/`recordType` map).
2. leader/07 -> Work `bf:Monograph`/`Serial`/... type and Instance `bf:issuance`.
3. Optional: q -> a Hub type is a non-goal (no Hub model); instead leave q mapped to
   a sensible default and document. Secondary `bf:Manuscript` for d/f/t is cheap --
   add if it doesn't complicate the reverse map.

## Hazards

- Sample leader/06 is 'a' (Text) and /07 'm' (Monograph) -> adding issuance/type
  changes goldens; regenerate and review.
- The reverse `leaderForClass`/`recordType` must invert any new class exactly, or
  round-trip breaks. Audio subclasses need distinct reverse entries.

## Acceptance

- [x] i/j -> NonMusicAudio/MusicAudio, round-tripping through the leader.
- [x] leader/07 -> issuance type + `bf:issuance`.
- [x] Goldens regenerated + reviewed; round-trip + fuzz green.

## Result

`workClass` splits leader/06 'i'/'j' into `NonMusicAudio`/`MusicAudio` (was a single
`Audio`); the reverse `recordType` inverts them ('i'/'j') and still maps a stray
`Audio` to 'i' for external input. Added `Instance.Issuance` from leader/07
(`issuanceForLevel`: m->mono, s->serl, i->intg, c/d->coll), emitted as a
`bf:issuance` reference to the LoC issuance-vocabulary IRI. The reverse
(`leaderFor` + `issuanceCode`) sets leader byte 7 back from the issuance IRI, so
the mode of issuance round-trips.

Design choice: issuance is carried only as the standard Instance `bf:issuance` IRI,
not as a second Work rdf:type -- the single-subclass reverse (`typeExcept`) expects
exactly one content-class type beside `bf:Work`, and bf:Monograph/Serial are
non-standard. Documented in the audit. q->Hub and secondary bf:Manuscript stay out
of scope.

Goldens: the sample (leader/06 'a', /07 'm') gains `bf:issuance` issuance/mono on
the Instance; regenerated both serializations. Tests: `leader_issuance_test.go`
(i/j audio subclasses + leader round-trip, four issuance levels + leader/07
round-trip) and the updated `TestWorkClass`. Suite + FuzzFromMARC + FuzzDecode green.
