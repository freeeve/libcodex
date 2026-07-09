# 105 -- lossgate compares tag presence only: control-field content regressions are invisible

Opened 2026-07-09. Surfaced by libcat's tasks/104 receipt, which noted their
own three round-trip gates "compare field-tag presence, and structurally could
not have caught this: the 008 was never absent, only hollow." The same is true
of ours.

## The gap

`bibframe/lossgate_test.go` round-trips the kitchen-sink record through all four
serializations and asserts, per tag, that it survives / transforms / stays lost.
Every comparison goes through `tagSet(r) map[string]bool`. Nothing looks at a
field's *value*.

So a control field can come back present and empty and the gate is happy. That
is exactly the shape of the bug libcat reported as tasks/103: decode emitted an
008 carrying only the country, with 06, 07-10 and 35-37 blank. The gate passed
throughout.

## Evidence, not assumption

Deleting the date positions from `control008` (reader_crosswalk.go):

    TestLossGateKitchenSink   ok     <- blind
    TestControl008*           FAIL   <- 008/06 = " ", want s

Task 103's dedicated tests do bite, so 008 specifically is covered. The gap is
that this is the only control field with such tests, and it only got them
because a downstream consumer noticed. 001/003/005/006/007 content is asserted
nowhere in the gate.

(003 and 005 are in `lostTags` and deliberately not reverse-crosswalked, so they
are out of scope. 001, 006, 007, 008 are all in `coreTags`.)

## The ask

Extend the loss gate from tag presence to a value comparison for control fields
in `coreTags`, at the same derive-don't-fabricate confidence the crosswalk
itself keeps:

- 001 -- exact match.
- 006/007 -- compare the positions `applyCodedFields` actually round-trips;
  `codedFields` already reconstructs a partial field, so assert on the populated
  positions rather than the whole 18/23 bytes.
- 008 -- assert the positions the crosswalk claims (06, 07-10, 15-17, 35-37) and
  ignore the rest, mirroring what `control008` renders. Do not assert on
  positions the graph cannot speak to; that is a documented non-goal, not a bug.

A stale-guard in the same spirit as `lostTags` would be ideal: if a position
starts surviving that the table says is blank, fail and force the table to move,
the way the tag table already does. That is what caught 040 in task 094.

## Not urgent

No live bug. 008 is pinned by task 103's tests and by libcat's
`TestMARCRoundTrip008PositionsSurvive` downstream. This is about the gate
catching the next one without a downstream consumer having to.

## Note for anyone verifying against an older libcodex

From libcat's 104: their `go.work` unifies root and backend, so MVS picks the
maximum libcodex requirement across the workspace. Downgrading one module leaves
the build on the newer version and a parity test passes vacuously. Both modules
have to move. Same trap applies to any workspace check here.
