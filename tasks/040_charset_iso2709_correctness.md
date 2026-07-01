# 040 -- iso2709/marc8/iso5426: correctness and lossiness-contract fixes

## Motivation

Review of the binary record layer found one confirmed round-trip corruption
bug, two violations of the documented `Lossy()` contract, and validation gaps
in the writer. The lead findings were reproduced with runnable tests.

## Problems

1. **ISO 5426 stacked combining marks swap on round trip** (high --
   internal/iso5426/iso5426.go:93-104 vs :249-257). `Encode` emits marks in
   input order before the base; `Decode`'s `flush` composes the base with
   `pending[last]` and emits earlier marks after it. Verified:
   `Encode("ế")` (NFD of Vietnamese ế) yields `C3 C2 65`, which
   decodes to `é + combining circumflex` -- diacritics swapped, not canonically
   equivalent. marc8's decoder (marc8.go:216-223) does this correctly; mirror
   it (`flush` composes with `pending[0]`, emits `pending[1:]` after).
2. **MARC-8 `ESC s` unrecognized** (internal/marc8/scripts.go:68-91).
   `setByFinal` handles technique-1 finals `g`/`b`/`p` but not `s`
   ("reinstate Basic Latin in G0" -- the standard return-to-ASCII escape), so
   clean records decode correctly but are flagged `Lossy()`. Add
   `case 's': return csASCII`.
3. **Malformed escape consumes a data byte silently**
   (internal/marc8/marc8.go:289-294). The `final == 0, n == 1` path returns 2,
   discarding the byte after ESC without setting `d.lossy` -- silent data loss
   that contradicts the Lossy contract. Return 1 and set `d.lossy = true` on
   any malformed/unterminated sequence.
4. **Writer never validates tag content; data-field `Value` silently dropped**
   (iso2709/writer.go:172-191, 210-218). A 3-byte tag containing
   0x1D/0x1E/0x1F passes validation and embeds structural delimiters in the
   directory, misframing the record for terminator-scanning parsers (including
   this package's own fallback paths). Separately, `Field{Tag:"245",
   Value:"x"}` encodes to bare indicators -- the value vanishes with no error
   (also covered from the `Validate` side in task 039).
5. **Truncated data field rescans indicator bytes as data**
   (iso2709/iso2709.go:159-169). When `hi-lo < indCount`, `p` stays at `lo`,
   so a stray 0x1F among would-be indicator bytes fabricates a subfield. Skip
   the field or set `p = hi`.
6. **iso5426 has no lossiness signal** (internal/iso5426/iso5426.go:149-154).
   Unmapped high bytes silently pass through as Latin-1, inconsistent with
   marc8's `Lossy()` API. Mirror the marc8 decoder pattern.
7. **Table generators swallow read errors** (internal/marc8/gen/main.go:186-195,
   internal/iso5426/gen/main.go:139-148). Hand-rolled body reads `break` on any
   error; a mid-stream network error can yield a shorter but valid-looking
   table. Use `io.ReadAll`, fail on error, assert a minimum entry count.

## Acceptance

- [ ] `Decode(Encode(s))` canonically equivalent to `s` for multi-mark NFD
      input in iso5426; fuzz `Encode` against the *original* input (the
      current FuzzEncode only checks decode-side stability).
- [ ] `ESC s` decodes clean (`Lossy() == false`); malformed escapes set
      `Lossy()` and drop no data bytes.
- [ ] Writer rejects tags containing delimiter/non-printable bytes; data-field
      `Value` either errors or is explicitly documented.
- [ ] Truncated-extent field decodes with no fabricated subfields; regression
      test with a 0x1F in the indicator zone.
- [ ] iso5426 exposes a lossiness signal; generators fail loudly on partial
      downloads.
