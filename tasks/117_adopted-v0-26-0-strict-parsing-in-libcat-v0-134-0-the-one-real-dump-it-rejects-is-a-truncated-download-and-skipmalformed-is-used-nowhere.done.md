# 117 -- adopted v0.26.0 strict parsing in libcat v0.134.0: the one real dump it rejects is a truncated download, and SkipMalformed is used nowhere

Filed from libcat on 2026-07-10 (cross-repo ask).

## Outcome

Closed. Nothing asked, and the title carries the whole report: v0.26.0 adopted in
libcat v0.134.0, `SkipMalformed` used nowhere, and the single real-world dump the
strict parser rejects turned out to be a truncated download.

That last part is the result worth recording. The strictness was argued for on a
hypothetical -- a truncated `catalog.nq` from a killed writer or a partial S3 GET --
and the first thing it caught in their corpus was exactly that, in a file that had
been parsing "successfully" until now. The lenient parser had been handing them a
short graph, and nothing had ever said so.

`SkipMalformed` going unused across the adopter that asked for the opt-in is the
other half of the answer: strict-by-default was the right call, not the
deprecation-note fallback they offered as a compromise. No libcodex change.
