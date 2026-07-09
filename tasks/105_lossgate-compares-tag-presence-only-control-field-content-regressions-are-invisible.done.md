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

## Outcome

Done in ef80f89. Test-only; no release (see below).

`controlClaims` in `lossgate_test.go` names the positions the reverse crosswalk
reconstructs per control field:

    001   whole value
    006   00 form of material
    007   00 category, 01 specific material designation
    008   06 date type, 07-10 date 1, 15-17 place, 35-37 language

Each claim is checked **both ways**: a claimed position must return the source's
value, and an unclaimed position must return blank. The second half is the stale
guard the ask wanted -- new work populating a position has to move the table, the
same contract `lostTags` has when a tag starts surviving.

### Verified by mutation, not by construction

Three separate regressions, each caught with a located message:

    control008 drops the date       -> 008/06 date type = " ", want "s"
                                       008/07-10 date 1 = "    ", want "1993"
    control008 fills an unclaimed   -> 008 position 38 = '0', which controlClaims
      position                         says the crosswalk cannot reconstruct
    007 category corrupted          -> 007/00 category = "z", want "c"

The first passed the gate before this change, which is the whole point.

006/007 turned out to round-trip byte-exact for the kitchen sink, but the claims
are written to the positions the crosswalk actually *asserts* (006/00, 007/00-01)
rather than to full equality, so the table states the contract instead of
recording an accident.

### No release

Test-only: no API, behavior, or documentation surface changes, so a tag would
carry nothing for consumers. libcodex v0.22.0 remains current. Departing from the
usual ship-on-land rule deliberately, not by omission.

### Scope held

003/005 stay out: they are `lostTags`, carried as AdminMetadata provenance and
deliberately not reverse-crosswalked. Extending the same treatment to *data*
fields (subfield-level value comparison) was not attempted -- the tag tables plus
the per-field round-trip tests cover those, and no evidence says otherwise.
