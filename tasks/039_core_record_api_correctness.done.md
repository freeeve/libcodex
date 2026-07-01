# 039 -- core: Record API correctness and linkage fixes

## Motivation

A quality/performance review of the root package found aliasing, validation,
and parsing defects in the core `Record`/`Field`/linkage API that every codec
builds on.

## Problems

1. **`Fields()` aliases `RemoveFields` compaction** (codex.go:281, 292).
   `Fields()` returns `r.fields` directly and `RemoveFields` compacts that same
   backing array in place (`kept := r.fields[:0]`), so a slice obtained from
   `Fields()` before `RemoveFields` is silently corrupted -- its elements are
   overwritten with shifted survivors. The truncated tail also retains dropped
   `Field` data until reallocation.
2. **`Validate` misses two lossy malformations** (codex.go:393-406). It rejects
   a data field with no subfields but accepts a control field carrying
   `Subfields` and a data field carrying a non-empty `Value` -- both are
   silently dropped by every codec on write, so `Validate` passes on records
   that round-trip lossily.
3. **`numAt` accepts signed input** (codex.go:234-243). `strconv.Atoi` parses
   `"-1234"`/`"+1234"`, so a hostile leader yields a negative `RecordLength()`
   instead of the documented 0 -- a latent panic for callers doing slice
   arithmetic.
4. **`Link` accepts malformed occurrence references** (linkage.go:52-55).
   `len(tagOcc) < 6` allows longer strings, so `"880-012"` silently truncates
   to occurrence `"01"` and can match the wrong 880 partner; non-digit
   occurrences also parse.
5. **`Vernacular` contradicts its doc** (linkage.go:95-104). Doc says "the
   first field with the given tag", but the loop continues past an unlinked
   first match and returns a later same-tag field's vernacular.
6. **Linkage resolution is O(n²) with per-call allocation** (linkage.go:49,
   71-90). `AlternateGraphic` calls `Link()` (which allocates via
   `strings.Split`) for every field in the record, even fields that cannot
   match; `Vernacular` adds another full-record scan.

## Change

- Either document `Fields()` as a live view and clear the dropped tail in
  `RemoveFields`, or return a copy from `Fields()`. Decide and document; clear
  the tail regardless so dropped fields are not retained.
- Extend `Validate`: error on `f.IsControl() && len(f.Subfields) > 0` and on
  `!f.IsControl() && f.Value != ""`.
- Replace `strconv.Atoi` in `numAt` with a digits-only manual parse (also
  removes the string-conversion allocation).
- In `Link`, require exactly `len(tagOcc) == 6` (or byte 6 to be a delimiter)
  and ASCII digits at positions 4-5.
- Align `Vernacular` behavior and doc ("first field with the given tag that
  has a linked 880" is likely intended).
- In `Link`, parse `$6` with `strings.IndexByte` instead of `strings.Split`
  (zero allocation for the common `880-01` case); in `AlternateGraphic`, skip
  fields whose tag can match neither `880` nor `link.Tag` before parsing.
- Add fuzz tests for the root package's parser-shaped code (`Link`,
  `Leader` numeric accessors, `Control008`) -- the only parsers in the repo
  without fuzz coverage.

## Acceptance

- [ ] Test proving a `Fields()` snapshot is not corrupted (or doc + tail-clear
      if the live-view semantics are chosen).
- [ ] `Validate` rejects control-with-subfields and data-with-value; tests.
- [ ] Leader with `-`/`+` in numeric positions returns 0.
- [ ] `Link` rejects `"880-012"` and `"880-xy"`; `Vernacular` matches its doc.
- [ ] Benchmark shows `AlternateGraphic`/`Vernacular` allocation drop.
- [ ] Fuzz tests for linkage and leader parsing added.
