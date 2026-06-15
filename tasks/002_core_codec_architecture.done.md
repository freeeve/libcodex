# 002 — Core model + per-format codec architecture

## Goal
Establish the seam that lets formats be added without editing a monolith: keep
the shared MARC data model in the core `codex` package, define `RecordReader` /
`RecordWriter` interfaces, and move the ISO 2709 binary codec into its own
`iso2709` subpackage.

## Why
MARCXML, MARC-in-JSON and `.mrk` are all serializations of the SAME model
(`Leader` + control/data `Field`s + `Subfield`s + indicators). Today
`ParseRecord` / `MarshalRecord` / `Reader` / `Writer` are generic names hardwired
to ISO 2709 — a trap as soon as a second format exists. Subpackages + shared
interfaces let third parties add a format by implementing an interface.

## Design
Core package `codex` keeps:
- `Record`, `Field`, `Subfield`, `Leader` and all accessors/builders.
- Interfaces:
  ```go
  // RecordReader yields records one at a time; io.EOF marks end of stream.
  type RecordReader interface{ Read() (*Record, error) }
  // RecordWriter serializes records to its underlying stream.
  type RecordWriter interface{ Write(*Record) error }
  ```
- MARC-8/ANSEL decoding: move to `internal/marc8` (shared by `iso2709` and
  `mrk`) or keep as an internal helper in core. It is NOT iso2709-specific.

New subpackage `iso2709` (implements the core interfaces):
- `iso2709.NewReader(io.Reader) *Reader`  — was `codex.NewReader`
- `iso2709.NewWriter(io.Writer) *Writer`  — was `codex.NewWriter`
- `iso2709.Decode([]byte) (*codex.Record, error)`  — was `ParseRecord`
- `iso2709.Encode(*codex.Record) ([]byte, error)`  — was `MarshalRecord`
- `iso2709.ReadFile(path) ([]*codex.Record, error)`

## Naming contract (every format subpackage follows this)
`NewReader` / `NewWriter` / `Decode` / `Encode` / `ReadFile` / `WriteFile`.

## Steps
- Create `iso2709/`; move reader.go/writer.go logic there; re-point at
  `codex.Record`.
- Decide the MARC-8 home (recommend `internal/marc8`).
- Migrate tests; preserve round-trip + fuzz coverage.
- Add a short MIGRATION note to the README (`codex.ParseRecord` → `iso2709.Decode`).

## Acceptance
- `codex` exports only the model + interfaces (+ shared MARC-8 if kept there).
- `iso2709` satisfies `codex.RecordReader` / `codex.RecordWriter`; all existing
  behavior preserved; tests green.
- A doc comment shows a third party implementing `codex.RecordReader` for a new
  format.

## Depends on
- 001
