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
