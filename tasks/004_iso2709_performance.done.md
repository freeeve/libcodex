# 004 — iso2709 read/write: zero/amortized allocation + throughput

## Goal
Drive read/write allocations toward zero/amortized and improve throughput,
guided by the benchmarks (`bench_test.go`) and pprof.

## Baseline
Apple M3 Max, 15-field UTF-8 record, `go test -bench=. -benchmem`:

| Benchmark              | ns/op | B/op  | allocs/op    |
|------------------------|-------|-------|--------------|
| ParseRecord            | 2453  | 4984  | 92           |
| MarshalRecord          | 2091  | 2457  | 36           |
| ReaderStream (100 rec) | 271k  | 569k  | 9404 (~94/r) |
| WriterStream (100 rec) | 209k  | 245k  | 3601 (~36/r) |
| RoundTrip              | 4648  | 7437  | 128          |
| DecodeMARC8            | 402   | 256   | 6            |

## Targets & low-hanging fruit
ParseRecord 92 → <10 allocs:
- Convert the field-data region to a string ONCE (`string(body)`), then take
  tag/value/subfield substrings as zero-copy slices of that single string
  (slicing a string does not copy). Collapses ~45 per-value allocations into 1.
- No-alloc fixed-width decimal parser for the base address and each directory
  entry's length/start (drop `strconv.Atoi(string(...))`): ~46 allocs gone.
- Preallocate `fields` to the directory-entry count; count `0x1F` to preallocate
  `Subfields`; drop `splitByte`'s `[][]byte`.

MarshalRecord 36 → <6 allocs:
- Replace `fmt.Sprintf("%04d%05d", ...)` per field and `"%05d"` (×2) with direct
  fixed-width digit writes into the directory/leader buffers.
- Size `dir` / `data` / `out` from a cheap pre-pass (or one shared buffer).

Streaming amortization:
- Reader: reuse the record read buffer (and the 24-byte leader buffer) across
  `Read` calls — safe because Decode copies values out.
- Writer: encode into a reusable scratch buffer owned by the Writer; add
  `EncodeInto(dst []byte, *Record) []byte` to avoid a fresh slice per record.

## Method
- Iterate against the benchmarks with `-benchmem`; keep a before/after table.
- `-cpuprofile` / `-memprofile` + `go tool pprof` for remaining hot spots after
  the obvious wins.
- Guard correctness with the round-trip + fuzz tests at every step.

## Results (Apple M3 Max, 15-field UTF-8 record)
| Benchmark   | allocs before→after | reduction | ns before→after | speedup | MB/s after |
|-------------|---------------------|-----------|-----------------|---------|------------|
| Decode      | 92 → **4**          | 95.7%     | 2517 → 691      | 3.6×    | 864        |
| Encode      | 36 → **1**          | 97.2%     | 2049 → 478      | 4.3×    | —          |
| ReaderStream (100 rec) | 9404 → **406** (~4/rec) | 95.7% | 263.9k → 76.8k | 3.4× | 778 |
| WriterStream (100 rec) | 3601 → **2** (~0/rec) | 99.9% | 203.5k → 43.2k | 4.7× | 1381 |
| RoundTrip   | 128 → **5**         | 96.1%     | 4679 → 1196     | 3.9×    | 499        |

Decode's 4 allocs are the structural floor: the single record string copy (backs
the leader + all UTF-8 tags/values as zero-copy substrings), the `Record`, its
field slice, and the pooled subfield slice. Encode is a single output allocation,
and streaming writes reuse the Writer's buffer for ~0 allocs/record.

## What was done
- Decode: one `string(b)` copy backs the leader and every UTF-8 tag/value as a
  zero-copy substring; `atoiBytes` replaces `strconv.Atoi(string(...))`; fields
  preallocated via `codex.NewRecordCap`; all subfields drawn from one pooled
  slice; inlined subfield walk (dropped `splitByte`/`trimByte`/`parseField`). A
  `prealloc` cap guards against malformed huge directories.
- Encode: exact one-shot output allocation sized by a `fieldLen` pre-pass;
  `appendFixed`/`writeFixed` replace every `fmt.Sprintf`.
- Reader: reuses one read buffer across calls (safe because Decode copies values
  out); `atoiBytes` for the declared length.
- Writer: added `EncodeInto(dst, *Record) ([]byte, error)` (append-style, grows
  dst once via `slices.Grow`); `Writer` reuses an internal buffer so streaming
  writes allocate only on growth. `Encode` is now `EncodeInto(nil, ...)`.

## Profiling findings
With `-cpuprofile`/`-memprofile` + pprof (single-threaded to remove scheduler
noise):
- Decode is ~5% of CPU; the rest is GC of the allocations. The alloc breakdown is
  exactly `NewRecordCap` (Record + field slice, 2) and Decode (record string copy
  + subfield pool, 2). The reused read buffer does not appear — fully amortized.
- Encode's only allocation was the output buffer (now reused by the Writer).
- Conclusion: both paths are allocation-bound at the structural floor. The
  remaining cost is GC scanning the string headers in Record/Field/Subfield,
  inherent to the string-based model. Going lower would need a Record pool or an
  offset-based (non-string) model — both change the public API/ownership and were
  judged not worth it. No algorithmic hot spots remain.

## Acceptance — met
- Decode/Encode allocations cut **96–97%** (target was >80%); streaming is
  ~4 allocs/record (read) and ~1/record (write); throughput improved 3.4–4.3×;
  all tests pass, fuzz clean (14.5M execs, no panics), coverage core 100% /
  marc8 97.1% / iso2709 91.7%.

## Depends on
- 002
