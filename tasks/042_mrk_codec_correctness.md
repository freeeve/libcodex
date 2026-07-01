# 042 -- mrk: escaping, injection, and parsing correctness

## Motivation

Review found the mrk codec silently corrupts or forges data on inputs the
sibling codecs (marcxml, marcjson) handle safely. All lead findings were
verified with runnable reproductions.

## Problems

1. **`&` never escaped -- character-reference-shaped text corrupts on round
   trip** (high -- mrk.go:62-76 vs :161-167). Encode escapes only `$`, `{`,
   `}`; Decode resolves any `&#DDDD;`/`&#xHHHH;`. Verified: value
   `"AT&#38;T and &#x24;5"` encodes verbatim and decodes as `"AT&T and $5"`.
   Also violates the fuzzer's stability property (e.g.
   `=245  10$a&#x26;#x41;` decodes differently twice). Fix: escape `&` on
   encode (MARCMaker's `{amp}` mnemonic is the lossless route) and add
   `{amp}` to `unescape`.
2. **Tag injection -- no tag validation** (high -- mrk.go:99-123, :44-46).
   `validate` never checks `f.Tag`; a tag containing `\n` injects lines.
   Verified: `Tag: "9\n=999"` encodes successfully and decodes as a
   different field (`999`) -- field forgery with no error. Require
   `len(f.Tag) == 3` printable non-`=` ASCII, mirroring marcxml's `validTag`.
3. **Positional indicator misparse** (mrk.go:248-253). Bytes 4-5 are assumed
   to be the two separator spaces and never checked; `=245 10$aTitle` (one
   space) silently decodes with `ind1='0'`, `ind2='$'`, and the two length
   branches disagree on the same malformation. Verify the separator and
   error on mismatch.
4. **`charRef` accepts what the format cannot carry** (mrk.go:180-200).
   `&#10;` decodes to a value containing `\n` that `Encode` immediately
   rejects; surrogate refs become U+FFFD; `&#+65;` slips through
   `ParseInt`'s sign handling. Reject refs producing `\n`/`\r`/surrogates
   and anchor the digit scan.
5. **Backslash indicator conflated with blank** (mrk.go:80-93). `Ind1: '\\'`
   round-trips as `' '` with no error while marcxml/marcjson preserve it;
   converting through mrk silently changes data. Reject `'\\'` in
   `validate` as unrepresentable (same pattern as the `$` subfield-code
   check).
6. **No malformed-line diagnostics** (mrk.go:217-246). Non-`=` lines are
   skipped and short `=` lines silently ignored, so broken input yields
   confidently wrong records where the sibling codecs error. Error on
   structurally bad `=` lines; document comment-line tolerance.
7. **Writer API parity** (mrk.go:322-344). mrk's Writer lacks `Close`, the
   sticky error, and write-after-close rejection that marcxml/marcjson
   writers have; generic write-then-Close code must special-case mrk.
8. **`Decode` copies its input** (mrk.go:289). `strings.NewReader(string(b))`
   is an O(n) copy; use `bytes.NewReader(b)` like the sibling codecs.

Reproductions for 1, 2, 3, 5 exist as throwaway tests written during review;
re-derive them as regression tests.

## Acceptance

- [ ] Round trip preserves `&#...;`-shaped literal text; fuzz stability
      property holds on the previous counterexamples.
- [ ] Tag injection input from problem 2 now returns an error from Encode.
- [ ] One-space separator line errors instead of shifting indicators.
- [ ] `Close`/sticky-error semantics match marcxml/marcjson writers.
- [ ] Regression tests added for each fixed item.
