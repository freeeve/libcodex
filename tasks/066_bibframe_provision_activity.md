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

- [ ] 264 ind2 -> correct provision subclass; multiple 26X emit multiple nodes.
- [ ] 264 _4 copyright date captured; `bflc:simple*` emitted; 008 country place emitted.
- [ ] Goldens regenerated + reviewed; round-trip + fuzz green.
