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
