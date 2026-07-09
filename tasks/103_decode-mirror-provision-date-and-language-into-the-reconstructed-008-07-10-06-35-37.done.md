# 103 -- decode: mirror provision date and language into the reconstructed 008 (07-10, 06, 35-37)

Filed from libcat on 2026-07-09 (cross-repo ask).

Found by libcat-e2e while probing our 008 builder (libcat tasks/228
note, diagnosed as libcat tasks/230).

## The asymmetry

Encode reads 008/07-10 as the provision date fallback and 008/35-37
into `Instance.Languages`; decode mirrors only the COUNTRY back
(`control008Country`, reader_crosswalk.go:1078 -- "without fabricating
date or language positions"). The date renders into 260 $c instead,
and language into no 008 position at all.

Observed round trip (libcat playground, a real audiobook record):

    provision date "2010", place nyu, language eng
    -> decode:
    008 "               nyu                      "   (07-10 blank, 35-37 blank)
    260 $a Ashland $b Blackstone... $c 2010

Semantically nothing is lost, but positionally the reconstructed 008
diverges from what encode itself would read back: a cataloger who sets
Date 1 through libcat's 008 builder saves bf:date quads, reloads, and
sees 008/07-10 blank (with the date now in a 260) -- the builder
appears to discard the edit.

## The ask

Extend the reconstruction to mirror what encode reads, at the same
derive-don't-fabricate confidence as the country:

- 008/07-10 <- the provision `bf:date`/`bflc:simpleDate` when it is a
  plain 4-digit year (leave blank otherwise -- no parsing heroics);
  008/06 <- "s" when exactly one such date.
- 008/35-37 <- the first Instance language code, which encode already
  round-trips through 041 -- the 008 slot is the same code.

These are derivations from properties encode itself created from those
positions -- the mirror of `control008Country`, not fabrication. Keep
emitting the 260 $c as today; the date legitimately lives in both.

No urgency: data is preserved either way; this is display/positional
parity for fixed-field editing surfaces. Our fidelity doc now carries
the caveat (libcat docs/marc-fidelity.md) and will drop it on your
release.

## Outcome

Done in ee24945, shipped in v0.22.0. Filed libcat tasks/235.

Implemented as asked. `control008Country` became `control008(g, work,
inst)`, rendering every position `FromRecord` reads out of an 008:

    06/07-10  a provision's bf:date, when it is a bare four-digit year
    15-17     the controlled bf:place country IRI      (unchanged)
    35-37     the Work's first content language

Language lives on the Work in this model, not the Instance as the ask
assumed, so the builder needed both terms; 260/264 stayed in
`provisionFields` and the 008 moved up to `recordFromWorkInstance`.

Verified against the reported audiobook shape: date 2010 / place nyu /
language eng now decodes to `008 "      s2010    nyu                 eng
"` plus its 260, and re-encoding the decoded record reads all three back.

### Two boundaries worth recording

**"[2010]" mirrors, and should.** My first cut of the not-a-year test
asserted a bracketed date stays blank, and it failed -- because
`FromRecord`'s `cleanDate` already strips brackets, so `[2010]` reaches
the graph as the bare year `2010`. Mirroring it is a derivation from a
property encode built, not a parse, which is exactly the confidence bar
this task set. The test was wrong, not the code; it now pins the
behavior so nobody "fixes" it later.

**Disagreeing provisions assert nothing.** The ask said 06 <- "s" when
exactly one such date. Taken literally that would drop the year when two
provisions carry the *same* year (publication + manufacture, both 2001),
which is not ambiguous at all. Implemented as: distinct years disagreeing
-> assert neither; agreeing -> mirror. Both cases pinned.

### Notes

- `mkc008(date, country, lang)` test helper composes the 40-byte field,
  so no test hand-counts filler spaces. The first version of these tests
  did, and one literal was already miscounted.
- Warned libcat that any snapshot asserting those positions are *blank*
  will need updating on their bump.