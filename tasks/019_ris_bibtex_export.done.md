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

## Status — done
- One `citation` package (RIS and BibTeX share the MARC extraction). `Entry`
  intermediate; `FromRecord`; `RIS(r)`/`BibTeX(r)` single-record; `NewRISWriter`/
  `NewBibTeXWriter` (both implement `codex.RecordWriter` — self-delimiting, no
  Close); `WriteRISFile`/`WriteBibTeXFile`.
- Crosswalk: leader 06+07 → RIS TY / BibTeX entry type (book/article/inbook/…);
  245→title; 1xx/7xx→author; 260/264→publisher/place/date (year extracted, with
  008 fallback); 250→edition; 020→ISBN; 022→ISSN; 6xx→keywords; 520→abstract;
  856→URL; 008→language. BibTeX cite key = first-author-surname + year + first
  title word (ASCII-sanitized, fallback "ref").
- BibTeX escapes `{ } & % $ # _` and `\ ~ ^` (text commands); RIS keeps values
  plain and folds line breaks to spaces. Both are UTF-8-aware (invalid bytes
  dropped). Added `codex.Leader.BibLevel()` (leader byte 7).

## Performance (done)
| Benchmark | allocs | ns/op |
|-----------|--------|-------|
| RIS    | **14** | 665 |
| BibTeX | **20** | 888 |
| RISWriterStream (100) | ~14/rec | 381 MB/s |

Hand-rolled, append-based; no reflection.

## Fuzz (done)
`FuzzFromMARC`: any decodable MARC record renders to valid-UTF-8 RIS and BibTeX
without panicking. Campaign clean (14M execs).

## Acceptance — met
- MARC→RIS and MARC→BibTeX implemented and documented; golden files
  (`sample.ris`, `sample.bib`); special-character escaping and cite-key
  generation tested. Coverage 84.1%.

## Depends on
- 002

## Per-format requirements (standing directive)
- Add `bench_test.go`; profile and reduce allocations (single-pass mapping; reuse
  the Writer buffer; hand-roll output where it pays off). Document any stdlib
  marshaling floor.
- Add a fuzz target: any decodable MARC record converts without panicking and
  produces valid output. Run a sustained campaign.
- Zero third-party dependencies.
