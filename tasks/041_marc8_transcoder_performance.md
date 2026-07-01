# 041 -- marc8/iso5426/iso2709: transcoder hot-path performance

## Motivation

Benchmarks show the character-set transcoder, not the ISO 2709 parser, is the
bottleneck for MARC-8 files: `internal/marc8 BenchmarkDecode` runs at ~50-58
MB/s (~21 ns/byte) while the whole iso2709 record decode runs at ~850 MB/s.
The iso2709 fast path itself is already well tuned (4 allocs/record decode,
1 alloc/record encode); the remaining cost is table lookups and per-call
allocations in the transcoders.

## Changes

1. **Dense arrays instead of `map[byte]rune`** (internal/marc8/marc8.go:55,
   :103; internal/marc8/scripts.go:128-136). `anselGraphic`, `anselCombining`,
   and each single-byte set's `dec` map pay 1-2 hash lookups per non-ASCII
   byte (`anselRune` probes two maps for unmapped G1 bytes). A byte-keyed
   domain of 256 is a textbook dense-array case: generate `[256]rune` arrays
   (0 = unmapped) at init/generation time. Expect a severalfold decode
   speedup.
2. **Stop materializing `[]rune(s)` in Encode** (internal/marc8/marc8.go:397-407
   and internal/iso5426/iso5426.go:225). One O(n) rune-slice allocation per
   subfield; the mark-gathering lookahead only needs the next rune, so iterate
   with `utf8.DecodeRuneInString` and a one-rune peek.
3. **Reuse decoder scratch state** (internal/marc8/marc8.go:205-224). `var
   pending []rune` plus the `flush` closure escape to the heap on every
   `Decode` call (1-2 small allocations per diacritic-bearing subfield). Move
   `pending` onto the `Decoder` (reset with `d.pending[:0]`), make `flush` a
   method. Also add the missing `b.Grow(len(data))` in iso5426
   (iso5426.go:83; marc8 already has it at :207).
4. **`bytes.Count` for subfield presizing** (iso2709/iso2709.go:243-251,
   call site :116). `countByte` is a manual loop over the whole record body;
   `bytes.Count` with a single-byte needle uses the SIMD `bytealg.Count`
   (no allocation) -- roughly an order of magnitude faster per byte.
5. **Drop the duplicate `fieldLen` pass** (iso2709/writer.go:132, :153).
   Computed once in validation and again for the directory, each call
   iterating all subfields; record per-field lengths in a reusable slice or
   fuse the passes.

## Acceptance

- [ ] `internal/marc8 BenchmarkDecode` throughput improves at least 3x; no
      behavior change on the conformance/fuzz suites.
- [ ] MARC-8 encode path drops the per-subfield rune-slice allocation
      (verify with `-benchmem`).
- [ ] `benchstat` before/after captured in the task/commit message for
      iso2709 `BenchmarkDecode`/`BenchmarkEncode`/`BenchmarkReaderStream`.
- [ ] Generated-table format change (if any) reproducible via the gen
      programs.
