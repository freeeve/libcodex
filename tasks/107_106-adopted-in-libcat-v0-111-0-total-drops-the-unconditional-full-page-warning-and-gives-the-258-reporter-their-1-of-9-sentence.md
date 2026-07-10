# 107 -- 106 adopted in libcat v0.111.0: Total() drops the unconditional full-page warning and gives the 258 reporter their "1 of 9" sentence

Filed from libcat on 2026-07-09 (cross-repo ask).

Adopted in **libcat v0.111.0** (`3a5974b`), on **libcodex v0.23.0**, bumped in
both libcat modules together (a one-module `go get` builds green under `go.work`
and proves nothing). This closes libcodex 106 and libcat 274.

Nothing needed changing on your side. Both design calls were right, and both
paid off in ways worth naming.

## `-1` over `(int, bool)`

You said the sentinel "keeps the accessor a plain getter, which is what lets it
sit behind an interface." That is exactly what happened. The whole adoption on
our side is:

```go
func advertisedTotal(rd codex.RecordReader) int {
    rc, ok := rd.(codex.RecordCounter)
    if !ok {
        return unknownTotal
    }
    return rc.Total()
}
```

A `(int, bool)` return would have made `RecordCounter` awkward to assert against
and pushed the two-value handling into every call site. And keeping `Total()` off
`codex.RecordReader` meant our fake readers in tests -- which have no result set
to size -- keep compiling untouched, and correctly report "unknown" rather than
being forced to invent a number.

## The wrinkle earned its warning

`Total() == -1` after a successful fetch is the case I would have got wrong
without the note. Our `cappedError` switches three ways:

```go
switch {
case total > got:   return fmt.Errorf("%w: showing %d of %d matches -- refine your search", ErrCapped, got, total)
case total == got:  return nil                                    // complete answer, no warning
default:            return fmt.Errorf("%w: showing the first %d", ErrCapped, limit)  // -1, or a contradictory count
}
```

`total == 0` reaches `case total == got` only when nothing arrived, which is the
true "empty result set" reading. An omitted `numberOfRecords` reaches `default`
and gets the honest pre-274 warning. A test pins all six boundaries, including
the one where a server's advertised count contradicts its own stream (`total <
got`) -- we print the fallback rather than a nonsense "3 of 2".

Your related parser fix -- omitted count and empty result set both unmarshalling
to `0` -- is the reason any of this is expressible. Thank you for finding it
while you were in there; we would have inherited it silently.

## What it bought

**The unconditional full-page warning is gone.** libcat 258 had shipped a warning
on *every* search that filled `searchLimit` (20), because 20 matches and "the
first 20 of 4,113" were indistinguishable. That was noise on the commonest path
and no help on the path that mattered. Now:

```
target says 4113 exist, 20 returned  ->  "showing 20 of 4113 matches -- refine your search"
target says 20 exist,   20 returned  ->  (silence: this is the whole answer)
target says nothing,    20 returned  ->  "showing the first 20"
```

Verified against a live SRU stub, not just in unit tests.

**And a broken stream now names the total too.** `PartialError` carries `Total`,
so the sentence the original bug reporter wrote by hand comes out of the code:

```
partial results: the stream broke after 1 of 9 record(s): sru: parse response: XML syntax error
```

## Nothing to do

No libcodex change requested. `sru.Response.NumberOfRecords` is untouched for our
direct `SearchRetrieve` callers, as you said.

One observation, offered rather than filed: `z3950.Reader.Total()` is documented
as always `>= 0` after a successful fetch, and `sru.Reader.Total()` may be `-1`
forever. Callers behind `codex.RecordCounter` cannot tell which reader they hold,
so they must handle `-1` regardless -- which is the right outcome, and the
interface doc already says so. Recording it only because a reader of the z3950
doc alone might conclude otherwise.
