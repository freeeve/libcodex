# 051 -- tests: close the assertions that pass on broken output

## Motivation

Several cross-format and differential tests contain no-op or too-lenient
assertions, so specific regressions would go green today. Fuzzers in three
packages carve out known-bad cases instead of enforcing their properties
(those carve-outs are removed by their fix tasks; this task covers the
rest).

## Problems

1. **`utf8NonEmpty` is a no-op** (export_test.go:89). Returns nil
   unconditionally, so the RIS/BibTeX targets get zero format-specific
   assertions. Replace with real checks (RIS `TY  -`/`ER  -` markers,
   BibTeX leading `@`, as realdata_test.go:99-104 already does) or rename
   honestly.
2. **`xmlWellFormed` passes empty input** (realdata_test.go:90-95 with
   export_test.go:71-82). First `dec.Token()` returning `io.EOF` yields
   nil, and realdata's exporter checks have no non-empty guard -- a
   regression to empty mods/dublincore output goes green. Track a
   `seen bool` or check `len(b) == 0`.
3. **Differential test swallows every decode error**
   (bench/marc_test.go:59-72). `readAllStream` compares
   `err.Error() == "EOF"` (breaks on wrapped errors) and returns
   `out, nil` on both branches, so a genuine mid-file parse bug surfaces
   only as a confusing record-count mismatch. Use `errors.Is(err, io.EOF)`
   and propagate real errors. Also the mismatch counter (marc_test.go:33-52)
   breaks only the inner subfield loop, overcounting per-field.
4. **Ignored errors in conformance harness** (conformance_test.go:39, :59).
   `c.Close()` error discarded; `out, _ := dublincore.Encode(...)` -- a
   flush failure or Encode error misattributes as an xmllint schema
   message. Check both.
5. **Fuzz coverage gaps.**
   - Root package: `Link`, leader numeric accessors, `Control008` have no
     fuzz tests (covered by task 039's acceptance; listed here for the
     inventory).
   - iso5426 `FuzzEncode` never checks against the *original* Unicode
     input, which is how task 040's mark-order bug survived; add an
     encode-decode-canonical-equivalence property.
   - `FuzzStreamTurtle` waives the stream-vs-parse differential
     (decoder_fuzz_test.go:133-135); re-enable once task 045 fixes
     `statementEnd`.
   - rdf adversary tests: add the canon cost-per-work-unit and RDF/XML
     unbounded-literal cases (task 046).

## Acceptance

- [x] Deliberately breaking each guarded property (empty exporter output,
      corrupt RIS, mid-file decode error) makes the suite fail. `utf8NonEmpty`
      is replaced by `risValid`/`bibtexValid`; `xmlWellFormed` rejects empty
      and element-less input; the differential propagates real decode errors.
- [x] No ignored error returns remain in the harness paths called out:
      `conformance_test.go` now checks `dublincore.Encode`; `readAllStream`
      returns real errors instead of swallowing them.
- [x] Fuzz seeds added for the review counterexamples: iso5426 stacked marks
      (NFC/NFD ế, mixed-class); `_:y.` and `.#` Turtle terminators. (mrk
      `&#x26;#x41;` and the rdf canon/RDF-XML-literal adversary cases already
      landed with tasks 046.)
- [x] Modified fuzzers (`FuzzStreamTurtle`, mrk `FuzzDecode`, iso5426
      `FuzzEncode`) run clean locally.

## Resolution

1. **`utf8NonEmpty` no-op** -- replaced by `risValid` (asserts `TY  -`/`ER  -`)
   and `bibtexValid` (asserts a leading `@`) in `export_test.go`.
2. **`xmlWellFormed` passed empty input** -- now errors on empty input and on
   input with no elements (tracks a `StartElement`), so a regression to empty
   mods/dublincore output fails instead of going green.
3. **Differential test swallowed errors** -- `readAllStream` uses
   `errors.Is(err, io.EOF)` and propagates genuine parse errors; the mismatch
   loop delegates to `recordsEqual` so a differing record is counted once, not
   once per field.
4. **Conformance harness** -- the dublincore case checks `Encode`'s error.
5. **Fuzz gaps** -- iso5426 stacked-mark seeds added; a targeted
   `TestStreamTerminatorEdges` asserts the streaming Turtle decoder matches the
   parser for the `.`-before-comment and blank-label-terminator cases task 045
   fixed, and those inputs are seeded into the stream fuzzers. A *blanket*
   stream-vs-parse differential over arbitrary fuzz input stays waived: the
   waiver's reasons (the `.` ambiguity in prefixed local names, lenient
   empty-IRI triples) are independent of statementEnd and remain valid.

## Deferred

- Root-package fuzz for `Link`, the leader numeric accessors, and `Control008`
  is part of task 039's acceptance and is added there, not here.
