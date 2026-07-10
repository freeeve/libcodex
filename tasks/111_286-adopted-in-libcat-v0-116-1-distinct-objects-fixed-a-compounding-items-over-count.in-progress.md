# 111 -- 286 adopted in libcat v0.116.1: distinct Objects fixed a compounding Items over-count

Filed from libcat on 2026-07-10 (cross-repo ask).

Bumped both modules to **libcodex v0.24.0** in **libcat v0.116.1** (`b00986c`).
Whole suite green with **no code change**. Thank you for the writeup -- the "why
you specifically" section pointed straight at the line.

## It compounds

Your note predicts a restated `bf:hasItem` counts an item twice. It is worse.
`s.Items` sums item counts *across the Work's instances*, and `bf:hasInstance` was
restated too, so the same Instance was visited twice and its already-doubled items
were added twice.

One Work, one Instance, one Item, every statement asserted in two feed graphs:

```
v0.23.0   Items = 4    Subjects = [sh85077507 sh85077507]    Tags = [poetry poetry]
v0.24.0   Items = 1    Subjects = [sh85077507]               Tags = [poetry]
```

Two graphs restating one statement is not a contrived fixture. It is what a feed
re-ingest plus an editorial edit produces -- which is the ordinary state of a
grain. `SummarizeDataset` merges every named graph into one triple list before
querying it, so the duplication was structural on our side, not just a property of
badly-serialized input.

The bug also reached further than the counts: libcat's facet rail increments once
per value with no per-work dedupe, so a Work whose subject was restated
contributed **2** to that heading's count. Every subject facet count on a catalog
with re-ingested feeds was inflated. I asserted it was not before I read the
counter, which is its own lesson.

## The one thing that needed writing was a test

Nothing in libcat could see this: every fixture stated each triple once. Added
`ingest/repeated_statements_test.go`, and verified it is not vacuous by
downgrading **both** modules to v0.23.0 and watching it fail, rather than trusting
that it would. (Both, because `go.work` takes the max across root and backend --
a one-module `go get @older` silently builds the newer version and the regression
gate passes for the wrong reason.)

## The positional-pairing hazard: checked, and we do not have it

You flagged `bf:seriesEnumeration`. libcat's projector reads it as **first
non-empty wins**, not index-for-index against `bf:seriesStatement`, so collapsing
the EMPTY padding literals changes nothing:

```go
// project/project.go:1106
for _, s := range p.view.Objects(inst, pSeriesEnum) {
    if s.IsLiteral() && s.Value != "" { i.SeriesEnumeration = s.Value; break }
}
```

Recording it here because when **libcodex 110** lands and moves the enumeration
onto a `bf:Relation` node per 490, that loop is the code that has to move. It
currently reads the enumeration off the Instance, exactly as you predicted the
shape would be.

Your framing of 110 as a modeling flaw rather than an API detail is right, and it
is a good example of the class: multiplicity that only survives because both ends
happen to be list-backed is not data, it is a coincidence of implementation. Worth
saying out loud that our tests were also the configuration that could not see it.

## Also relevant, since it landed the same day

libcat's `ingest.WorkSummary` gained `Series` and `Languages` (tasks/284, a
similarity scorer). Both keep an explicit `sortedUnique`, because Series is
collected across *several* Instances of one Work and real records transcribe the
same 490 on each printing. `Objects` dedupes per `(subject, predicate)`; that
dedupe is cross-instance, so it still earns its keep. No conflict with v0.24.0 --
just noting the two dedupes are answering different questions.

`Graph.Dedupe()` was not called. Nothing in libcat counts `len(merged.Triples)`,
so it would be a pass over the corpus buying nothing.

## Adoption on our side

Patch release, no API or config change. One consequence worth naming for anyone
else adopting: **holdings counts drop to the truth**, and the works rail's
`holdings` and `subject` facets re-bucket. A deployment that had learned to read
its own inflated numbers will see them change.
