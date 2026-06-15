# 019 — RIS and BibTeX export (citation formats)

## Goal
Export records to RIS and BibTeX, the formats reference managers (Zotero,
EndNote, Mendeley) and LaTeX consume — a frequently-requested export target that
opens the researcher/citation audience.

## Why
These are small, high-utility, citation-centric formats. The mapping from MARC is
lossy (a citation is a flat subset of a bibliographic record), but export is in
high demand and the formats are simple text.

## Scope
- `ris` and `bibtex` packages (or one `citation` package), text output only,
  stdlib only.
- MARC→RIS: `TY` from leader/008 record type; `TI`←245; `AU`←100/700; `PY`←date;
  `PB`←264$b; `SN`←020/022; `KW`←6xx; `ER` terminator.
- MARC→BibTeX: choose entry type (`@book`/`@article`/…) from the record type;
  `title`, `author` (joined with " and "), `year`, `publisher`, `isbn`, etc.;
  escape BibTeX special characters; generate a stable cite key.
- Export only (these are lossy targets); a converter API, not
  `RecordReader`/`RecordWriter`.

## Acceptance
- MARC→RIS and MARC→BibTeX over a corpus; output parses in a reference manager /
  BibTeX; golden-file tests; special-character escaping covered.

## Depends on
- 002

## Per-format requirements (standing directive)
- Add `bench_test.go`; profile and reduce allocations (single-pass mapping; reuse
  the Writer buffer; hand-roll output where it pays off). Document any stdlib
  marshaling floor.
- Add a fuzz target: any decodable MARC record converts without panicking and
  produces valid output. Run a sustained campaign.
- Zero third-party dependencies.
