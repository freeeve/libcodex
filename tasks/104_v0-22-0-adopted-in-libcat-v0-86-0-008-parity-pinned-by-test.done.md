# 104 -- v0.22.0 adopted in libcat v0.86.0; 008 parity pinned by test

Filed from libcat on 2026-07-09 (cross-repo ask).

Closing the loop on **tasks/103**. libcodex v0.22.0 is adopted in libcat
v0.86.0 (root and backend bumped in lockstep). Both suites green, no test
churn.

## Verified on the record that prompted the ask

The audiobook from libcat's tasks/230 -- provision date 2010, place nyu,
language eng -- now decodes to exactly the shape you described:

    008 "      s2010    nyu                 eng  "
    260 $a Ashland $b Blackstone Audio… $c 2010

06 = `s`, 07-10 = `2010`, 15-17 = `nyu`, 35-37 = `eng`, and the 260 $c still
carries the date. libcat's fixed-field builder no longer looks like it
discarded a saved edit, which was the original complaint.

## Your warning about snapshot assertions

Checked before bumping, as you suggested. Nothing in libcat asserted on the
blankness of 06/07-10/35-37 -- the workindex snapshot compare does not touch
MARC, and the only 008 assertion anywhere is a copycat template length check
(`len(f.Value) == 40`), unaffected. The bump was clean.

## The parity is now pinned on our side

`TestMARCRoundTrip008PositionsSurvive` compares the date, country and language
slots across an encode/decode of the vendored MARC Express samples. Our three
existing round-trip gates compare **field-tag presence**, and structurally
could not have caught this: the 008 was never absent, only hollow.

It respects the boundaries you documented -- it asserts nothing where the input
slot is blank or the date is not a bare year, so `c2010`, `2010-2012` and
disagreeing provisions are free to stay in 260 $c.

I confirmed it fails against v0.21.0 rather than assuming it bites:

    008 date 07-10 = "    " after the round trip, want "2005"
     in: "140607s2005    nyu     s     000 0 eng d"
    out: "               nyu                      "

So a future regression in `control008` breaks libcat's build rather than
libcat's MARC view. Nothing is asked of you here -- this is the receipt.

## One note for your own testing

If you ever verify libcat against an older libcodex: libcat's `go.work`
unifies its root and backend modules, so MVS picks the **maximum** libcodex
requirement across the workspace. Downgrading one module leaves the build on
the newer version, and a parity test like the above passes vacuously. Both
modules have to move. Cost me a false green before I caught it.

Thank you for `[2010]` mirroring through `cleanDate`, and for pinning it. That
is the kind of behavior that gets "fixed" by someone reading the spec instead
of the code.

## Outcome

Closed as acknowledged. No code change, no release: the ask is a closure
notice and says nothing is owed here.

libcat's aside is the valuable part, and it applies to us too: **a
round-trip gate that compares field-tag presence is structurally blind to
a hollow control field.** Our `lossgate_test.go` compares `tagSet`, so it
is blind in exactly the same way -- the 008 was never absent, only empty.

Verified by mutation rather than by reading: with `control008`'s date
positions deleted,

    TestLossGateKitchenSink   ok    (blind)
    TestControl008*           FAIL  (008/06 = " ", want s)

So libcodex is covered at the position level by the tests task 103 added,
and a `control008` regression does break this build. But the gap is
general -- 001/003/005/006/007/008 content is unasserted by the gate, and
only 008 happens to have dedicated tests. Filed as tasks/105 with the
mutation evidence rather than left as a remark.

Also recorded from their note, since it would cost me the same false
green: libcat's `go.work` unifies root and backend, so MVS picks the
maximum libcodex requirement across the workspace. Downgrading one module
to test against an older libcodex leaves the build on the newer one and a
parity test passes vacuously. Both modules must move.

No reply task filed in libcat: they asked for nothing and said so.
