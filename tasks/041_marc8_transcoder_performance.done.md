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

- [x] `internal/marc8 BenchmarkDecode` throughput improves at least 3x; no
      behavior change on the conformance/fuzz suites. Achieved ~4.3x (see below).
- [x] MARC-8 encode path drops the per-subfield rune-slice allocation
      (verify with `-benchmem`). `BenchmarkEncode` is now 1 alloc/op (the output
      buffer only).
- [x] `benchstat` before/after captured in the task/commit message for
      iso2709 `BenchmarkDecode`/`BenchmarkEncode`/`BenchmarkReaderStream`.
- [x] Generated-table format change (if any) reproducible via the gen
      programs. N/A -- the dense arrays are built at init from the existing
      generated maps, so the generated file format is unchanged.

## Resolution

Changes made (Apple M3 Max, `-count` 5-8):

- **marc8 dense arrays.** The single-byte sets' `dec` map became a `*[256]rune`
  built at init (`densify`); ANSEL's two maps merged into one dense `anselTable`.
  This speeds the non-Latin decode paths (map probe -> indexed load).
- **marc8 isCombining fast path.** The dominant decode cost was `unicode.In` on
  every output rune. No combining mark exists below U+0300, so a range guard
  skips the table scan for all ASCII/Latin text (56% -> 8% of profile).
- **marc8 inline ASCII decode + WriteByte.** A plain ASCII byte in the default G0
  set with no buffered mark is written with a single `WriteByte`, skipping the
  per-byte decodeChar/isCombining/flush calls -- the largest single win.
- **marc8/iso5426 encode.** `Encode` no longer materializes `[]rune(s)`; it
  iterates the string with `utf8.DecodeRuneInString` and a one-rune peek.
- **marc8 decoder scratch reuse.** `pending` moved onto the `Decoder` (reset per
  Decode), so a reused decoder (the iso2709 stream path) stops re-allocating it.
- **iso2709 bytes.Count.** Subfield presizing uses `bytes.Count` (SIMD
  bytealg.Count) with a hoisted needle instead of a manual byte loop.
- **iso2709 fused directory pass.** The encoder reserves the directory region in
  place and derives each field length from `appendField`'s byte delta, dropping
  the duplicate `fieldLen` walk; still 1 alloc/record.

Benchmarks (ns/op, before -> after):

- marc8 `BenchmarkDecode`:  1063 -> 247   (60 -> 258 MB/s, ~4.3x)
- marc8 `BenchmarkEncode`:  (new) 567, 1 alloc/op
- iso2709 `BenchmarkDecode`:  692 -> 587   (862 -> 970 MB/s)
- iso2709 `BenchmarkEncode`:  740 -> 684   (1 alloc/op unchanged)
- iso2709 `BenchmarkReaderStream`:  74000 -> 63200  (806 -> 901 MB/s)
- iso5426 `BenchmarkDecode`: unchanged (Decode not in scope; Encode `[]rune`
  removal is not benchmarked but mirrors the proven marc8 change).

No behavior change: full test suite, FuzzEncode (marc8, iso5426), FuzzDecode and
FuzzEncodeRoundTrip (iso2709) all pass.
