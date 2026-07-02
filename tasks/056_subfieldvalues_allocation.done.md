# 056 -- codex: cut SubfieldValues slice allocation on crosswalk hot paths

Deferred follow-up surfaced while profiling task 055. Not urgent -- the
crosswalk hot paths are otherwise at or below their pre-refactor allocation
counts after 055 and the FromRecord pre-sizing.

## Motivation

`Field.SubfieldValues` and `Record.SubfieldValues` (codex.go:205, codex.go:431)
build their result with `var out []string; out = append(out, ...)` from nil, so a
field with several matching subfields regrows the slice (nil->1->2->4). Allocation
profiling of the BIBFRAME writers (055) showed `(*Record).SubfieldValues` as a
`growslice` source, reached through the crosswalk helpers (`joinSub`, `extent`,
`subdivided`, `provisionStatement`, `addLanguages`). These methods are general
`Record` API and are called on every crosswalk's hot path (bibframe, mods,
dublincore, marcxml, ...), so a win here helps every exporter, not just BIBFRAME.

## Approach (implementer's call)

1. **Pre-size** `out` to `len(f.Subfields)` (a cheap upper bound) so the common
   case allocates once with no regrowth. Over-allocates when few subfields match;
   trivial change, no API churn, benefits all callers immediately.
2. **Count-then-fill:** one pass to count matches, then `make([]string, 0, n)`.
   Exact sizing, one extra loop; worth it only if (1)'s over-allocation shows up.
3. **Append/iterator variant** for hot callers that already have a scratch slice
   or only iterate: e.g. `AppendSubfieldValues(dst []string, code byte) []string`
   or a `range`-func iterator, so a caller can reuse a buffer and avoid the slice
   entirely. Bigger API surface; reserve for callers that measurably need it.

Prefer (1) as the baseline (smallest, broadest); add (3) only for a caller a
profile still flags after (1).

## Hazards

- `SubfieldValues` is public, general-purpose API -- keep the returned slice's
  observable behavior identical (values in subfield order, empty values included
  exactly as today; callers do their own trimming/skip).
- Don't return a slice aliasing a reused scratch buffer from the plain
  `SubfieldValues`; a reuse/append variant must be a separate method so existing
  callers keep ownership of their result.

## Acceptance

- [ ] `SubfieldValues` (Field and Record) allocates at most once for any input;
      no regrowth on multi-subfield fields.
- [ ] All crosswalk round-trip and golden tests unchanged (output byte-identical);
      exporter fuzz targets pass.
- [ ] A measurable allocation drop on `BenchmarkFromRecord` and the mods/dublincore
      encode benchmarks; no CPU regression beyond noise.

Origin: 055 profiling follow-up (the remaining per-record `growslice` source after
FromRecord pre-sizing). Lives in the root `codex` package, not the emitters.
