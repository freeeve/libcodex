# 066 -- bibframe: 264 ind2 provision subclass + copyright + 008 place + bflc simple

Tier 2. From the 059 m2b audit, provision area.
Ref: `docs/bibframe_m2b_audit.md` section 5; m2b `ConvSpec-250-270.xsl`,
`ConvSpec-Process8-ProvAct.xsl`.

## Motivation

We always emit a single `bf:Publication` from the best-ranked 26X and discard the
rest. m2b maps 264 ind2 to the provision-activity subclass and emits one node per
26X: ind2 0->`bf:Production`, 1->`bf:Publication`, 2->`bf:Distribution`,
3->`bf:Manufacture`, 4->`bf:copyrightDate` (on the Instance). It also emits an
008/15-17 country `bf:place` IRI unconditionally and the transcribed $a/$b/$c as
`bflc:simplePlace/simpleAgent/simpleDate` (which our reader already parses).

## Scope

1. Emit one provision node per 260/264, typed by 264 ind2 (fallback Publication
   for 260 / blank ind2).
2. 264 _4 $c -> Instance copyright date (`bf:copyrightDate`).
3. Emit `bflc:simplePlace/simpleAgent/simpleDate` from 26X $a/$b/$c on the forward
   path (predicates already defined in reader.go/vocab.go for the reverse read).
4. 008/15-17 -> a `bf:place` country IRI on the provision activity, even with no 26X.

## Hazards

- Sample has one 264 _1 -> stays a single `bf:Publication`, but adding `bflc:simple*`
  and an 008 country place WILL change the sample goldens -- regenerate and review.
- Country-code -> IRI needs the MARC country code table; scope that lookup (a small
  static map is fine, mirroring how language codes map to IRIs).
- Keep `provisionStatement`/`publicationRank` behavior for the controlled place/date
  or refactor cleanly; don't double-emit place (controlled IRI vs simple literal).

## Acceptance

- [x] 264 ind2 -> correct provision subclass; multiple 26X emit multiple nodes.
- [x] 264 _4 copyright date captured; `bflc:simple*` emitted; 008 country place emitted.
- [x] Goldens regenerated + reviewed; round-trip + fuzz green.

## Result

`Instance.Provision *Provision` became `Provisions []Provision`; `Provision` gained
`Class` and `Country`, and `Instance` gained `CopyrightDate`. Forward
(`addProvisions`) emits one node per 26X typed by `provisionClass` (264 ind2 0/1/2/3
-> Production/Publication/Distribution/Manufacture, 260/blank -> Publication); a
264 _4 sets `CopyrightDate` instead of a node. The 008/15-17 country (`country008`)
and, absent a 26X date, the 008 date attach to a Publication node, minted when the
record has no usable 26X. `provisionStatement`/`publicationRank` are retired.

Emit: `emitProvision` renders the subclass, the country as a controlled `bf:place`
IRI (LoC countries vocab, `countryIRIVal`), the transcribed place/agent as
`bflc:simplePlace`/`simpleAgent`, and the date as `bf:date` + `bflc:simpleDate`.
Provisions are wrapped in a `bf:provisionActivity` **list** -- the original
single-object emit produced duplicate JSON-LD keys for multiple provisions and
silently dropped all but the last (caught by FuzzDecode on a two-provision LoC
record). Reverse (`provisionFields`) emits one 260 per node (classes collapse to
the flat 260, preserving the transcribed-statement tests), reconstructs a minimal
008 for the country (`control008Country`), and emits 264 _4 for the copyright date;
a controlled country IRI is never mis-read as a transcribed $a.

Dead `qcAgent` removed (publishers now emit as `bflc:simpleAgent`).

Goldens: the sample's single 264 _1 now emits a `bf:provisionActivity` array
carrying the `countries/nyu` place, `bflc:simple*` transcription and `bf:date` +
`bflc:simpleDate`; regenerated both serializations. Tests:
`provision_activity_test.go` (per-field subclasses + copyright + country, forward
simple*/country emit, RDF/XML + JSON-LD round-trip of multiple provisions) and the
updated provision/date-fallback assertions. Suite + FuzzFromMARC + FuzzDecode green.

Deferred: EDTF datatype on dates; 008 date/language positions in the reconstructed
008 (only country is needed for round-trip; language/date already round-trip via
041 / 26X $c).
