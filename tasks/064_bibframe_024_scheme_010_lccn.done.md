# 064 -- bibframe: 024 ind1 scheme typing + forward 010 -> bf:Lccn

Tier 1 (high value). From the 059 m2b audit, identifiers area.
Ref: `docs/bibframe_m2b_audit.md` section 4; m2b `ConvSpec-010-048.xsl` mode
`instanceId`.

## Motivation

Two related typing gaps:

1. **024 ind1 -> scheme.** We flatten every 024 to a generic `bf:Identifier` with
   $2 as source, discarding ind1. m2b types by ind1: 0->`bf:Isrc`, 1->`bf:Upc`,
   2->`bf:Ismn`, 3->`bf:Ean`, 7->a $2-keyed scheme (Doi/Isni/Gtin14/...). Because
   the forward path never types 024, the reverse path hardcodes `ind1='8'`, so
   ingesting a real m2b graph (`bf:Upc`/`bf:Ean`/`bf:Doi`) loses the scheme.
2. **Forward 010 -> bf:Lccn missing.** There is no "010" case in `FromRecord`, so
   we never produce a `bf:Lccn`; the reverse path maps Lccn->010 but nothing
   round-trips into it.

## Scope

- 024: switch on `f.Ind1` to choose the identifier class (consult $2 when ind1=7);
  keep generic `bf:Identifier` only as the fallback.
- Reverse `identifierField`: map the bf class back to 024 ind1 (Isrc->0, Upc->1,
  Ismn->2, Ean->3) or ind1=7 + $2 for doi/isni; stop the blanket ind1='8'.
- Add a "010" case: `appendIdentifier("Lccn", trimISBD($a), ...)` (and $z ->
  invalid status once 063 lands, or leave a TODO referencing 063).

## Hazards

- Sample 024 is `urn:isbn:...` with no ind1/$2 -> stays generic `bf:Identifier`;
  confirm the golden 024 node is unchanged for the sample.
- Sample has no 010, so adding the case shouldn't move the sample golden.
- Coordinate the class<->ind1 table between forward and reverse so they invert.

## Acceptance

- [x] 024 typed by ind1/$2 forward; reverse reconstructs the matching ind1.
- [x] 010 $a produces `bf:Lccn`; round-trips to 010.
- [x] Tests for a UPC/DOI 024 and an LCCN; suite + fuzz green.

## Result

`identifier024` types a 024 by ind1 (0->Isrc, 1->Upc, 2->Ismn, 3->Ean, 4->Sici) or,
for ind1='7', by $2 via `scheme024ByCode` (doi->Doi, isni->Isni,
gtin-14->Gtin14Number, ... the full m2b set); an unrecognized ind1='7' scheme is
kept as a generic `bf:Identifier` carrying the $2 as bf:source, and ind1 '8'/blank
stays generic. `appendIdentifiers` now takes an explicit source so the 024 $2 acts
as the scheme selector rather than always becoming the source. The reverse
(`identifier024Field` + inverted `ind1ByScheme024`/`codeByScheme024`) restores the
matching ind1 / $2, replacing the old blanket ind1='8'.

Added a forward "010" case producing `bf:Lccn` ($a valid, $z canceled via
bf:status), with full-space trimming for the LCCN's positional leading spaces; the
reverse Lccn path now emits $z for a canceled status. `presize` counts 010.

Goldens unchanged: the sample's 024 is ind1=blank (generic Identifier, no source)
and it has no 010. Tests: `identifier_scheme_test.go` (024 ind1/$2 typing across
9 cases, 024 round-trip, 010->Lccn incl. canceled $z). Full suite + fuzz green.

Class names taken verbatim from marc2bibframe2 ConvSpec-010-048.xsl (the 024
`vIdentifier` choose), so we don't invent vocabulary.
