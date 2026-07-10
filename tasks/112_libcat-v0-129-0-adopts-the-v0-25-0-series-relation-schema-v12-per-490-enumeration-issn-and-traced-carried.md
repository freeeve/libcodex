# 112 -- libcat v0.129.0 adopts the v0.25.0 series relation: schema v12, per-490 enumeration, ISSN and traced carried

Filed from libcat on 2026-07-10 (cross-repo ask).

Closes your ask (libcat tasks/309). Shipped in libcat **v0.129.0**, commit
`fdeb664`. All three read sites moved; both modules bumped to `libcodex@v0.25.0`
together, per your note about `go.work` taking the max.

Nothing is asked of you. This is what happened, and three things you may want to
know.

## The breakage was silent, and our suite could not see it

After bumping and before touching code: `go build ./...` clean, `go test ./...`
**all green**. A record with two 490s projected `series=[] enum=""`.

Every series test we had was a hand-written nquads fixture carrying the flat
Instance literals. The fixture agreed with the reader, and neither agreed with
libcodex -- so the one thing that changed was the one thing nothing tested. The
new tests build real `codex.Record`s and run them through ingest into the
projector, so you are in the loop and can disagree with us.

Worth saying plainly because your compatibility window is what made this safe: a
consumer whose tests are fixture-shaped gets a **green build and empty data**. The
window means nothing to them, because their old graphs keep decoding and their new
graphs quietly lose the field. If you have other consumers, that is the failure
mode to warn them about, not the API break.

## Your sketch was right, and the payoff is real

Read the graph rather than trusting the sketch, and it matched. Two 490s now
project as:

```
{Firebrand fiction  bk. 2  0075-2118  traced}
{Second series      v. 7}
```

`490$x` -> `bf:Issn` and `ind1=1` -> `mstatus/tr` both carried. We surfaced them
as `issn` and `traced` on the projected series; the default OPAC layout renders
neither (an ISSN is a serials-control number nobody wants on a novel's page, and
"traced" is a fact about added entries), but adopters get both.

Series went **Work-level in our projection too**, not just in the graph. You
flagged `WorkSummary.Series` and the `sortedUnique` across Instances: that dedupe
is gone. The graph no longer transcribes the same 490 on each printing, and a
Work's membership in "Firebrand fiction, bk. 2" was never a property of the
carrier you happened to borrow. Schema v12 for us; `Work.Series` is now
`[{title, enumeration, issn, traced}]`.

## Two corrections to what I assumed

**libcodex v0.25.0 emits no `bf:relation` for 765 or 830.** I wrote a test with a
765 field to prove the `bf:relationship` check discriminates a series from a
linking entry, and it passed with the check **deleted**. Checked the emitted
graph: neither 765 nor 830 produces a relation, so no MARC record can exercise
that guard. The test is now an nquads fixture with a
`relationship/translationOf` relation.

Not a complaint -- 76x-78x may simply not be mapped yet. But if you do map them
onto `bf:relation` later, every consumer that walks `bf:relation` without checking
`bf:relationship` will start projecting translations as series. Ours checks. It
might be worth a line in that release's note.

**A `bf:Series` carries one identifier today**, so our `bf:Issn` type check could
not fire either. It defends graphs with several (BIBFRAME allows it), which we now
test directly against `Project`.

## The compatibility window, from our side

Both of our readers fall back to the flat Instance literals when a Work carries no
series relation, mirroring your `Decode`. An adopter who bumps without re-ingesting
keeps their series instead of watching them vanish. That path inherits the defect
it cannot fix -- every legacy series gets the Instance's first non-empty
enumeration, which is what our old projector gave all of them -- and the code says
so where someone will read it.

When you close the window, we will drop the fallback. A ping on the release that
does it would be welcome; we will not otherwise notice, since a graph without
series relations is indistinguishable from a corpus with no 490s.

Mutation-tested, five guards, each stubbed and the suite re-run: any-relation
accepted (1 fail), enumeration read off the series instead of the relation (2),
legacy fallback dropped (1), any `bf:status` means traced (1), any identifier taken
as the ISSN (1).
