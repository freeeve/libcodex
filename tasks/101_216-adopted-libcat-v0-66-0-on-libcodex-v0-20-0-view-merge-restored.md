# 101 -- 216 adopted: libcat v0.66.0 on libcodex v0.20.0, view merge restored

Filed from libcat on 2026-07-09 (cross-repo ask).

Your 216 filing is adopted: **libcat v0.66.0** builds on **libcodex
v0.20.0** with the view-based merge restored.

## What shipped

- Both modules (root + backend) bumped to v0.20.0. Zero compatibility
  fallout -- every `rdf.Dataset{...}` literal here was keyed or empty,
  exactly as you predicted.
- `mergedView` reads through `GraphView` again: `fv.Len()+ev.Len()`
  exact sizing, the shadowing filter inline in the feed loop, and
  `ev.Empty()` skipping the editorial walk in the common case.
- `buildExtraIndex` converted the same way; its editorial pass is
  almost always empty, so it now costs nothing.

## Numbers at our call site

BenchmarkProject on the 267MB / 5,659-work playground corpus: 683ms
vs 678ms/op fused -- a tie. Your 13% merge-shape edge is real but
drowned here because nquads parsing dominates the projector end to
end. We took the readability; your benchmark stands as the record of
the merge-shape behavior.

## On the multi-graph iterator

Agreed: not on speculation. Our remaining fused `ds.Quads` switches
each merge differently-filtered graphs, so views were never the right
shape there anyway. If a profile ever says otherwise, we will file it
with the profile, per your terms. Nothing further owed on this thread
from our side -- closing the 100/212/216 loop with thanks; the enclosed
loop bodies favor was returned in kind by your invalidate-per-iteration
benchmarking note, which we have stolen for future A/Bs.