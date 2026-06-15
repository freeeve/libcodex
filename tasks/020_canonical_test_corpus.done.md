# 020 — Canonical free-licensed test corpus

## Goal
Test the readers against real-world MARC records under a free/open license, not
only hand-built fixtures.

## Status — done
- Added `testdata/pymarc-sample.mrc`: 20 real MARC 21 bibliographic records in
  binary ISO 2709 with **MARC-8** encoding (so it exercises the MARC-8 decode
  path), from the [pymarc](https://gitlab.com/pymarc/pymarc) project's
  `test/marc.dat`. **BSD 2-Clause**; provenance and full license recorded in
  `testdata/README.md`.
- `TestCanonicalCorpus` (in `integration_test.go`) reads all 20 records, spot-
  checks fields, then converts the corpus through **every** format
  (iso2709/marcxml/marcjson/mrk) and asserts the model is preserved — real data
  across the whole pipeline.

## Notes / future
- Sources surveyed: pymarc (BSD-2, used), Library of Congress sample records
  (US-gov public domain), marc4j fixtures. LoC records are public domain if a
  no-attribution source is preferred later.
- Could add canonical MARCXML / MARC-in-JSON input fixtures (not just binary) for
  the text readers; the binary file already round-trips through them in the test.

## Acceptance — met
- A free-licensed, attributed canonical corpus is committed and read by the test
  suite across all formats.

## Depends on
- 002, 006-008
