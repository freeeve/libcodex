# 091 -- codex CLI (inspect / convert / validate / stats)

Filed and completed 2026-07-06, out of the task 090 investigation: poking at a
real OverDrive .mrc export by hand made a small command-line front-end for the
library's codecs obviously useful.

## What shipped

`cmd/codex` -- a single binary wiring the existing format codecs behind four
subcommands. Input format auto-detects from the leading bytes when `-i` is
omitted; with no file arguments each subcommand reads stdin.

- `codex cat [-i fmt] [-t tags] [-n N] [--json] [file...]` -- readable dump in
  the mrk line format (or MARC-in-JSON with `--json`). `-t` keeps only the
  listed tags (e.g. `-t 084,650`); `-n` caps the record count.
- `codex convert [-i fmt] -o fmt [file...]` -- transcode between any registered
  input and output formats via codex.Convert; finalizes wrapper-buffering
  writers (marcxml/marcjson/bibframe) with codex.Close.
- `codex validate [-i fmt] [file...]` -- run Record.Validate on every record,
  report the position + 001 of each failure, non-zero exit if any invalid.
- `codex stats [-i fmt] [file...]` -- record count, encoding split, leader/06
  record-type and leader/07 bib-level breakdowns, and per-tag frequency.

## Registry

- Input formats: marc/iso2709, marcxml/xml, marcjson/json, mrk, unimarc,
  bibframe.
- Output formats: the above MARC round-trippers plus the write-only display
  projections dublincore, mods, schemaorg.
- Format detection: ISO 2709 (5-digit leader), XML (marcxml vs bibframe RDF/XML
  by an `RDF`/`bf:` probe), JSON array/object (marcjson), mrk (`=LDR`); UTF-8
  BOM tolerated.

## Files

- cmd/codex/{main,registry,input,cat,convert,validate,stats}.go
- cmd/codex/codex_test.go -- 82% statement coverage; exercised against the real
  ME-15711 OverDrive export during development.

## Possible follow-ups

- Autodetect output format from a `-o file.ext` extension.
- `stats --subfields` for a subfield-level histogram.
- A `--pretty`/aligned mrk mode.
