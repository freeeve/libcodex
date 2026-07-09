# 097 -- libcat adopted v0.18.0: 040 fidelity row flipped, export-side derivation shipped (v0.47.0)

Filed from libcat on 2026-07-09 (cross-repo ask).

Closing the loop on your 094/our 194 handoff -- libcat v0.47.0:

- Both modules require libcodex v0.18.0. Your loss-gate handshake
  worked exactly as designed: our TestMARCRoundTripLossTableCurrent
  failed with "040 now survives; move it" on the bump, and our sidecar
  test caught the would-be double-040 (sidecar + model), so 040 left
  bibframe.KnownLoss in the same commit.
- The corrected contract you documented ($a bf:assigner, bf: not
  bflc:, $c via the internal marcKey note) is now quoted in
  docs/marc-fidelity.md's kept table.
- On top: DecodeGrainMARCSource derives the deployment's own agency at
  decode time (org code config; editorial-graph statements -> one
  trailing $d, born-digital -> synthesized $a/$c), leaning on your
  per-$d ordering guarantee. Nothing needed from your side; FYI only.

## Outcome

Closed as acknowledged. No code change, no release: the ask is a
closure notice, and its own last line says nothing is needed here.

Verified the one guarantee libcat now builds on -- per-$d ordering of
`bf:descriptionModifier` -- rather than taking it on trust. It is
pinned two ways and both still pass:

- `TestCatalogingSourceFromRecord` asserts the modifier IRIs come back
  in field order (oclcq before ukmgb), so a reordering regression fails
  the suite here rather than silently scrambling libcat's trailing $d.
- The internal marcKey note preserves subfield order verbatim, and
  decode prefers it, so the order survives even when RDF object order
  does not.

No reply task filed in libcat: they asked for nothing, and a
"received" task would be ledger noise. Their tasks/194 (our 094) and
tasks/192 are both settled by this.

Note for future adopters: the 096 fix (deterministic English-preferred
subject heading labels) also shipped in v0.18.0, so libcat's
DecodeGrainMARC prefLabel pre-filter can go too. That was flagged in
their 194 and is not repeated here.
