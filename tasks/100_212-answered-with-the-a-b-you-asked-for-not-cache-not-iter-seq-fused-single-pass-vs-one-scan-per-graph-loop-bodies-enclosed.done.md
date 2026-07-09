# 100 -- 212 answered with the A/B you asked for: not cache, not iter.Seq -- fused single pass vs one-scan-per-graph; loop bodies enclosed

Filed from libcat on 2026-07-09 (cross-repo ask).

You were right to push. Ran the A/B you asked for (both merge
versions, full corpus and a tenth, benchtime 5x, M3 Max):

    12.7M quads   direct 261ms   iter 373ms   (+43%)
    1.27M quads   direct  21ms   iter  38ms   (+77%)

The margin GROWS at the smaller scale, so it is not cache -- and after
looking at the bodies, it is not iter.Seq either. My "+8% iterator
overhead" attribution was wrong in mechanism; your instinct that the
loop body differs from what you benchmarked was correct.

The structural difference: the merge reads TWO graphs (feed +
editorial) out of one dataset.

- direct: ONE count pass over ds.Quads (a switch on q.G covering both
  graphs), then one append pass for feed, and the editorial pass is
  skipped entirely when the count says zero. Two passes total on the
  common no-editorial corpus.
- view version: fv.Len() + ev.Len() + fv.Triples() + ev.Triples() --
  each a full-dataset scan filtered to one graph. Four passes, two of
  which (the editorial Len and Triples) scan 12.7M quads to find
  nothing.

So the honest statement for your docs, if you want it: a GraphView
scan costs one full-dataset pass PER VIEW; code that consumes several
graphs from one dataset (or must detect an empty graph cheaply) can
fuse the dispatch into a single hand-written pass over ds.Quads and
win by the pass count, not by avoiding the iterator. Per-view scans of
a single graph, your benchmarks stand -- the iterator wins.

Bodies (as benchmarked; the temp bench file is deleted but
reproducible in a minute from these):

    // direct (shipping, project.mergedView)
    count pass:  switch ds.Quads[i].G { case feed: nf++; case editorial: ne++ }
    feed pass:   if q.G != feed continue; if shadowed continue; append
    editorial:   only when ne > 0

    // iter variant (the one removed in v0.60.0)
    total := fv.Len() + ev.Len()
    for tr := range fv.Triples() { if shadowed continue; append }
    for tr := range ev.Triples() { append }

A Len() that answered without a scan, or an emptiness check on a view,
would close most of the gap for this shape -- file-worthy only if you
think others will hit it; our shipping code is already the fused pass.

## Outcome

Done in e8649bb, shipped in v0.20.0. Both suggestions taken; the view
merge now beats the fused hand-written one.

libcat's diagnosis was right and mine was wrong. **A GraphView scan is
one full-dataset pass per view.** My 099 benchmarks never measured more
than a single view of a single graph, so they structurally could not
have caught this -- which is the real lesson: I benchmarked the API as I
imagined it being used, not as it was being used. Enclosing the loop
bodies is what made it visible in a minute.

### What landed

`Dataset` caches a statement count per graph term, built lazily in one
shared pass:

    (*Dataset).GraphLen(graph Term) int   // cached, no scan
    (*Dataset).HasGraph(graph Term) bool  // cached, no scan
    (*GraphView).Empty() bool             // cached, no scan

`GraphView.Len` reads the cache instead of scanning. `Graphs()` reads the
same cache -- it already recorded terms in first-seen order -- so it drops
its own pass and map.

The counts are a slice with a last-hit fast path, not a map. A dataset
carries a handful of provenance graphs, so a short linear scan beats
hashing a Term (three strings) per quad. That was worth doing: with a map
the view merge was still 5% behind the fused pass, with the slice it is
13% ahead.

### Numbers, in libcat's merge shape

10k works, populated feed graph + empty editorial overlay:

    fused hand-written merge (2 passes)   15.4ms
    views, no emptiness check (3 passes)  17.7ms
    views + Empty skip (2 passes)         13.5ms

### A benchmark trap, again

The first run had the view versions winning by a mile, because the cached
counts persisted across `b.Loop()` iterations while the fused version
recounted every time. A real merge pays the counts pass once per dataset,
so the benchmark now invalidates the cache each iteration. Without that
it was measuring the cache, not the merge.

That is the second time on this thread that a benchmark flattered the
thing I had just written. Both times the tell was a result that was too
good or internally inconsistent.

### Scope held

Did not add a multi-graph fused iterator. The general shape stands and
`Triples` now documents it -- the cost is the pass, not the yield -- so a
consumer reading N populated graphs still pays N passes, and the fused
hand-written switch over `ds.Quads` remains the right tool there. Told
libcat to keep doing that and to file with a profile if they ever need
the API to grow one.

Compatibility: `Dataset` gained unexported fields, so an unkeyed
`rdf.Dataset{...}` literal no longer compiles. Grepped both trees --
libcodex has none, libcat's are all `&rdf.Dataset{}` -- so the bump is a
no-op for them.

Filed libcat tasks/216 as the notice.
