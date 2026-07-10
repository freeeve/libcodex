# 112 -- libcat v0.129.0 adopts the v0.25.0 series relation: schema v12, per-490 enumeration, ISSN and traced carried

Filed from libcat on 2026-07-10 (cross-repo ask).

Closes your ask (libcat tasks/309). Shipped in libcat **v0.130.0**, commits
`fdeb664` + `79921b3`. All three read sites moved; both modules bumped to
`libcodex@v0.25.0` together, per your note about `go.work` taking the max.

(v0.129.0 carried the same graph work but exposed the projected series to Hugo
under a param name adopters already own. Nothing to do with libcodex; superseded
minutes later by v0.130.0.)

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

## Outcome

Closed. Nothing was asked; two of the three things they flagged turned out to be
actionable here, and one of their conclusions is wrong.

### Their correction is right, their conclusion is not

"libcodex v0.25.0 emits no `bf:relation` for 765 or 830" -- confirmed.
`FromRecord` routes only `773, 776, 780, 785` to `appendRelation`, and
`linkRelations` maps only those tags.

But "no MARC record can exercise that guard" does not follow. **780 emits a
relation**, and a record carrying a 490 and a 780 exercises the discrimination
exactly as intended:

```
_:b2 bf:relationship <.../relationship/continues> .
_:b5 bf:relationship <.../relationship/series> .
```

They picked one of the four unmapped tags. The guard is real and reachable; their
nquads fixture was a sound fallback, but a real record would have worked.

Our suite had no such test either, so I added one (`490` + `780`, forward and
decode) and mutation-checked it: deleting the `relationshipCode(...) != series`
check turns the 780's `$t` into a spurious `490 $a "Old Title"`. That is precisely
the failure they predicted, and it is now pinned by a real record on our side.

Their advice -- warn consumers who walk `bf:relation` without checking
`bf:relationship` -- is taken, but a release note is the wrong home for it, since
it is a standing property of the shape rather than news about one version. It now
sits on the public `Work.Relations` / `Work.Series` fields and in the audit doc.
Note the hazard predates v0.25.0: 773/776/780/785 have shared that list all along;
490 only made it likelier to matter.

### 830 is a dangling reference we created

A traced 490 (ind1=1) asserts that an 8xx exists carrying the controlled series
heading. Since v0.25.0 we emit `mstatus/tr` saying so -- and drop the 830 it points
at. The graph claims "traced" and offers nothing to trace to. Filed as **113**,
with LC's `mode="work8XX"` template (`bf:Hub` as the associated resource, not
`bf:Series`) as the thing to read before designing it. The remaining 76x tags
(765 `translationOf`, 767, 775, ...) are the mechanical half of the same task.

### The lesson they wrote down is the important one

> a consumer whose tests are fixture-shaped gets a **green build and empty data**

Their whole suite passed after bumping, while a record with two 490s projected
`series=[] enum=""`. Every series test they had was a hand-written nquads fixture
of the flat shape; the fixture agreed with their reader, and neither agreed with
libcodex. The compatibility window protected exactly nothing for them, because
archived graphs kept decoding while new graphs silently lost the field.

That is the failure mode to lead with when the window closes, not the API break.
Recorded in **114**, along with their request to be pinged -- they cannot detect
the closure by observation, since a graph with no series relations is
indistinguishable from a corpus with no 490s.

### Also worth noting

They dropped the cross-instance `sortedUnique` on `WorkSummary.Series`, on the
grounds that a Work's membership in a series "was never a property of the carrier
you happened to borrow." That is the right reading of why LC puts the relation on
the Work, and it is a better justification for the shape than the one I gave in
110, which was only that distinct subjects stop the triples collapsing.
