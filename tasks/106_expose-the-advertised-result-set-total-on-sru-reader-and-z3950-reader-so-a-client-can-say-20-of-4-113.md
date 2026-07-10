# 106 -- expose the advertised result-set total on sru.Reader and z3950.Reader so a client can say "20 of 4,113"

Filed from libcat on 2026-07-09 (cross-repo ask).

Both protocols tell us how many records the result set holds. Both readers know
it. Neither will say. A `RecordReader` caller that pages to a limit therefore
cannot distinguish *"this catalog holds 20 matches"* from *"this catalog holds
4,113 matches and you have seen 20 of them"* -- and in copy cataloging that is
the whole judgement a person is making.

Observed against **libcodex v0.22.0**.

## What each reader does with the total today

**SRU** parses it and drops it. `sru.Response.NumberOfRecords` exists
(`sru/sru.go:112`, populated at `sru/sru.go:298` from the `numberOfRecords`
element), and `Reader.fetch()` consults it:

```go
// sru/reader.go:92
case rd.fetched < resp.NumberOfRecords:
```

...to decide whether another page is worth requesting. Then `resp` falls out of
scope. `sru.Reader` (`sru/reader.go:16`) has no field to hold it, so there is
nothing for an accessor to return without a small change to the struct.

**Z39.50** keeps it, unexported. `Reader.fetch()` stores it:

```go
// z3950/reader.go:89
rd.total = res.Count
```

and `total` is a private field on `z3950.Reader` (`z3950/reader.go:16`). The
value is public elsewhere -- `Result.Count` (`z3950/z3950.go:193`) -- but only on
the `Search` path, which a streaming caller does not take. The reader is where
the number is, and it is one exported method away.

## The ask

An accessor on both readers, with matching names and matching semantics:

```go
// Total reports the number of records the server said the result set holds, or
// -1 before the first successful fetch (and on servers that do not say).
func (rd *Reader) Total() int
```

Whatever the shape, the two protocols should agree on it, since a caller behind
`codex.RecordReader` chooses between them at runtime and would otherwise need a
type switch to ask the same question.

Three details that matter for our use:

- **A pre-fetch sentinel.** `Total()` before any `Read()` cannot be a real answer.
  `0` is a real answer -- an empty result set -- so it cannot double as "unknown".
  `-1`, or a `(int, bool)` return, keeps the two apart.
- **`numberOfRecords` is optional in SRU 2.0** and some servers omit it. That is
  also "unknown", and should read the same as "not yet fetched" rather than `0`.
- **Adding it to `codex.RecordReader` would be a breaking change** for any other
  implementor. A separate optional interface -- `interface{ Total() int }`, tested
  with a type assertion -- costs callers one `if` and breaks nobody. That is our
  preference, but the call is yours.

## Why libcat wants it

`copycat.readUpTo` drains a target's stream to `searchLimit = 20`. Until
**libcat v0.109.0** a truncated set was indistinguishable from a complete one; it
now returns a `warnings` map naming any target whose answer was cut short (by a
broken stream, or by the limit). That is the honest answer available without the
total, and it is deliberately blunt:

> `loc-sru: result set truncated at the search limit: showing the first 20`

With `Total()` that becomes *"20 of 4,113 -- refine your search"*, which is what
the cataloger actually needs, and it lets us drop the warning entirely on the
common case where a target returned 20 of exactly 20. Right now every
20-record answer warns, because we cannot tell the two apart.

Filed rather than patched: libcat does not modify libcodex.

## Repro

```go
c := sru.NewClient("http://lx2.loc.gov:210/LCDB")
rd := c.NewReader(ctx, `dc.title="ocean"`)
for i := 0; i < 20; i++ { rd.Read() }
// The server said numberOfRecords="4113" on the first page.
// There is no way to ask rd for it.
```

Same shape for `z3950.NewClient(...).NewReader(...)`, where the value is sitting
in `rd.total`.
