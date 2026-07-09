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