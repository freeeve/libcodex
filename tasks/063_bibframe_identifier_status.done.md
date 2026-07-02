# 063 -- bibframe: bf:status for canceled/invalid identifiers

Tier 1 (high value). From the 059 m2b audit, identifiers area.
Ref: `docs/bibframe_m2b_audit.md` section 4; m2b `ConvSpec-010-048.xsl` mode
`instanceId`.

## Motivation

We read only $a on 020/022/024, so canceled/invalid identifier numbers are dropped
entirely. m2b walks $a|$y|$z and emits `bf:status -> bf:Status` with a
`.../vocabulary/mstatus/<code>` IRI: $z -> `cancinv` (canceled/invalid), 022 $y ->
`incorrect`. Losing these means a downstream system can't tell a live ISBN from a
canceled one.

## Scope

1. Add `Status string` to the `Identifier` struct (e.g. "cancinv", "incorrect").
2. In `appendIdentifiers`, also emit identifiers from $z (all of 020/022/024) with
   Status="cancinv", and 022 $y with Status="incorrect".
3. Emit `bf:status -> bf:Status` (mstatus IRI + label) in `emitIdentifier`.
4. Reverse: read `bf:status` back to $z/$y in `identifierFields`.

## Hazards

- Sample identifiers are all valid $a, so goldens only change if a test adds a $z.
- Keep the existing $a + qualifier + source behavior intact.
- Order in the identifier node: value, qualifier, status, source (pick one and
  keep it stable so goldens are deterministic).

## Acceptance

- [x] 020/024 $z and 022 $y/$z become identifiers carrying `bf:status`.
- [x] Status round-trips ($z/$y restored on reverse).
- [x] Round-trip test for a canceled ISBN/ISSN; suite + fuzz green.

## Result

Added `Identifier.Status` (`statusCancInv`/`statusIncorrect` constants).
`appendIdentifiers` now walks $z (all -> cancinv) and 022 $y (-> incorrect)
alongside $a, so canceled/invalid numbers are kept rather than dropped.
`emitIdentifier` emits `bf:status -> bf:Status` (labeled node, consistent with the
source node) in order value/qualifier/status/source. Reverse (`statusLabel`,
`identifierField`) restores the number into $z (cancinv) or $y (incorrect), $a
otherwise. Node shape uses `rdfs:label`=code like the other nodes; IRI-ifying the
status/source vocabulary is a separate cross-cutting divergence, out of scope here.

Goldens unchanged (sample has no $z/$y, status only emitted when present). Tests:
`identifier_status_test.go` (forward $z/$y/$z classification, $z/$y round-trip).
Full suite + FuzzFromMARC/FuzzDecode green.

Note: a single 020 with $a + $z becomes two `bf:identifiedBy` nodes and reverses
to two 020 fields (data-preserving, not byte-identical to a combined input field).
