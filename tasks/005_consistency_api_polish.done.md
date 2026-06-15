# 005 — Consistency & API polish

## Goal
Close the small consistency and ergonomics gaps surfaced in review and make the
public surface uniform across every format subpackage.

## Scope
- Move the `escape` constant out of core into the MARC-8 code (it is MARC-8-only).
- Normalize indicator defaults: parse and marshal agree on blank (`' '`) for unset
  data-field indicators; control fields carry none.
- Cover the gaps: `Leader.String()`, `SetLeader` (currently 0%) and the untested
  error branches in `numAt` / `indicator`.
- `Record` mutation helpers beyond `AddField`: `RemoveFields(tag)`,
  `ReplaceField`, ordered insert.
- Go 1.23 iterator per format: `func (r *Reader) All() iter.Seq2[*Record, error]`
  for `for rec, err := range r.All()`.
- `Record.Validate() error`: optional structural checks (leader length, 3-byte
  tags, data fields carry ≥1 subfield).
- Verify every format subpackage exposes the same surface:
  `NewReader` / `NewWriter` / `Decode` / `Encode` / `ReadFile` / `WriteFile`.

## Status — done
- [x] `escape` constant lives only in `internal/marc8` (moved during the 002
      refactor); verified no other package references it.
- [x] Indicator convention normalized + documented on `Field`/`NewDataField`: a
      blank indicator is `' '`; an unset (zero) data-field indicator serializes as
      blank; control fields carry none. Decode emits `' '` for missing indicators
      and Encode maps `0 → ' '`. Tested by `TestEncodeZeroIndicators`.
- [x] Coverage gaps closed: `Leader.String()`/`SetLeader` (core 100%); the
      `indicator`, `writeLeader` default-template branch, `atoiBytes`,
      `leaderDigit`, `prealloc`, `Read`/`readBody`/`ReadFile` error branches, and
      `Reader.Lossy()` are now tested.
- [x] `Record` mutators added: `RemoveFields(tag)`, `ReplaceField` (replace-or-
      append), `InsertField` (tag-ordered) — all chainable, tested.
- [x] Iterator: `codex.All(RecordReader) iter.Seq2[*Record, error]` (one
      implementation for every format) plus a thin `iso2709.Reader.All()`.
      Tested for success, stop-at-error, and early-break.
- [x] `Record.Validate() error`: leader length, 3-byte tags, data fields have
      ≥1 subfield. Tested (valid + each error).
- [x] Surface contract on `iso2709`: `NewReader` / `NewWriter` / `Decode` /
      `Encode` / `EncodeInto` / `ReadFile` / `WriteFile` — `WriteFile` added.
      Future format subpackages follow the same names.

## Acceptance — met
- Coverage: core 100%, marc8 97.1%, iso2709 97.8%; vet + gofmt clean; new helpers
  and the iterator tested. README accessor list updated.

## Depends on
- 002 (the naming contract here is the template every future format follows)
