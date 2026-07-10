# 114 -- close the 490 flat-shape deprecation window: drop legacySeriesFields and bf:seriesStatement reading, and ping libcat in the release that does it

Opened 2026-07-10.

v0.25.0 (task 110) moved 490 to a `bf:relation` -> `bf:Series` on the Work, and
left `Decode` able to read the old flat `bf:seriesStatement` /
`bf:seriesEnumeration` literals on the Instance when a Work carries no series
relation. That window exists so libcat could migrate on its own schedule. It has
(libcat v0.130.0, task 112).

## What comes out when it closes

- `legacySeriesFields`, `seriesEnumerationsFor`, `seriesField` in
  `bibframe/reader_crosswalk.go`
- `allLiteralsOf`, whose only caller is the legacy path
- `pSeriesStatement` / `qpSeriesEnumeration`'s read-only role
- `TestSeriesEnumerationsFor` and `TestSeriesLegacyFlatShapeDecodes`

`rdf.Graph.ObjectsWithRepeats` then loses its only caller in this repo. Do not
delete it: it is a legitimate accessor on a document-order list (task 108), and
losing its bibframe caller is the *evidence* the series shape is right, not a
reason to remove the API.

## Do not close it quietly

**libcat explicitly asked to be pinged**, and gave the reason: "a graph without
series relations is indistinguishable from a corpus with no 490s." They cannot
detect the window closing by observing behavior -- their fallback simply stops
finding anything, silently, on exactly the corpora that still need it. File a task
in their repo in the same release.

Their adoption report also named the failure mode this window does *not* protect
against, which is worth restating here because it is counter-intuitive: a consumer
whose tests are hand-written fixtures of the old shape gets **a green build and
empty data**. Their whole suite passed after bumping to v0.25.0, while a record
with two 490s projected `series=[] enum=""`. The compatibility window did nothing
for them, because their archived graphs kept decoding while their newly produced
graphs quietly lost the field. Any release note here should lead with that, not
with the API break.

## When

Not before libcat's archived grains are re-ingested, since a graph written before
v0.25.0 keeps the flat literals forever. Ask before scheduling; there is no
technical deadline, and the legacy path costs one `literalsOf` call on records
whose Work has no series relation.
