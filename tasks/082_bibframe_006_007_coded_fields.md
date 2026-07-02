# 082 -- bibframe: 006/007 coded fields -> typed properties -> packed positions

Tier 3 -- split from 081 scope item 4 (its 306/347 portion landed with 081).
Ref: MARC 006 (additional material characteristics) and 007 (physical
description fixed field); m2b ConvSpec-006,007 tables; the 008 partial
reconstruction pattern (`control008Country`).

## Motivation

006 and 007 carry material characteristics as packed byte positions keyed by a
leading category code (007/00: s sound, v video, c electronic, ...). They are
dropped entirely today, so downstream fidelity gates count them lost. The 008
work showed the viable shape: map the positions this model already speaks
(media/carrier, sound characteristics, color, dimensions...) to typed
properties on Encode, and rebuild a minimal, category-correct packed field on
Decode from whatever typed properties survive -- without fabricating positions
the graph does not know.

## Scope

1. 007/00-01 (category + specific material designation): correlate with the RDA
   media/carrier codes already modeled from 337/338 -- the RDA carrier
   vocabulary was designed to align (e.g. carrier `cr` <-> 007 "cr" online
   resource, `sd` <-> "sd" audio disc). Rebuild a 2-byte 007 when the record's
   carrier implies one; extend per-category positions only where a typed
   property exists to carry them.
2. 006: the material-specific 008 positions for secondary material types;
   reconstruct only the leading type byte plus positions with graph-native
   sources, mirroring `control008Country`'s minimal-fill approach.
3. Table-driven: one category table shared by both directions, like
   `linkRelations`.
4. Loss gate: move 006/007 from unlisted to coreTags (or transformed) in
   `bibframe/lossgate_test.go` as they land; downstream's stale guard will
   prompt its table update.

## Hazards

- The full 007 position tables are large; land category by category (electronic
  `c` and sound `s` first -- the OverDrive/audiobook shapes downstream cares
  about), leaving the rest enumerated here.
- Do not fabricate: positions with no typed source stay fill characters ('|' or
  ' '), exactly as the 008 reconstruction does.
- Category/carrier correlation is not 1:1 everywhere (carrier `cd` vs 007
  "co"); use an explicit table, not string surgery.

## Acceptance

- [ ] 007 for electronic and sound categories round-trips category + SMD (and
      any position with a typed source) on the kitchen-sink and realdata gates.
- [ ] 006 leading byte round-trips for records carrying one.
- [ ] Loss-gate tables updated; suite + fuzz green.
