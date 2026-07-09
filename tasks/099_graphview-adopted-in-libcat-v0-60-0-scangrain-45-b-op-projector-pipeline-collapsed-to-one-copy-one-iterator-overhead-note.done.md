# 099 -- GraphView adopted in libcat v0.60.0: ScanGrain -45% B/op, projector pipeline collapsed to one copy; one iterator-overhead note

Filed from libcat on 2026-07-09 (cross-repo ask).

Closing the 098/209 loop -- libcat v0.60.0 is on v0.19.0:

- identity.ScanGrain per-graph queries ride GraphView; rdf.GraphQuery
  let the four scan helpers retype without duplication. Real-grain
  bench: 89,666 -> 49,171 B/op (-45%), allocs 36 -> 31, time flat --
  your Object-heavy caveat reproduced exactly on our corpus.
- The projector's splitGraphs/mergeGraphs collapsed into one
  exactly-sized merge pass; the editorial/override path drops from up
  to three corpus copies to one.
- One field note you may want on GraphView.Triples() docs: at 12.7M
  quads the iter.Seq per-triple call overhead cost us +8% wall against
  a direct quads loop in the no-editorial common case, so the merge
  iterates ds.Quads directly and uses the view for Len() sizing. For
  per-grain sizes (~200 quads) the iterator is immaterial.
- Parse-segmentation pushback accepted; nothing further filed. If the
  10M tier wants more we will bring a fresh profile from the
  view-based code, per your closing note.

Verified: both suites green; the 62,602-grain workindex snapshot is
byte-identical to the pre-adoption baseline.

## Outcome

Done in 52d5dd2, shipped in v0.19.1 (docs + benchmarks, no behavior
change).

The only actionable item was the doc note on `GraphView.Triples`. I
tried to reproduce the +8% before writing it down, and **could not**.
Six new benchmarks (Corpus/Grain/SingleGraph x Iter/Direct), Go 1.25.6
on an M3 Max -- the same CPU family libcat reported on:

    corpus (250k quads viewed)   iter 4.13ms  vs  loop 5.79ms   (-29%)
    per-grain (150 quads viewed) iter 2.33us  vs  loop 3.16us   (-26%)
    single-graph, unfiltered loop iter 3.91ms vs  loop 4.96ms   (-21%)

The iterator wins all three. The third pairing exists to test my
hypothesis for their number: that the cost was not `iter.Seq` but the
per-quad `q.G == v.graph` compare (a Term is three strings; differing
pointers mean a memcmp per quad), which `Triples` must pay and their
no-editorial direct loop skips. Wrong -- even against a single-graph
dataset whose direct loop has no filter at all, the iterator still wins.

### Two ways I fooled myself first

- Comparing `Triples()` against a loop reading `q.S.Value` through a
  pointer is not equal work: the iterator yields a whole 168-byte
  `Triple` by value. That first cut had the iterator faster at corpus
  scale and 2x slower per-grain -- an incoherent result that was
  entirely the unfair comparison, not a real effect.
- Storing the yielded `Triple` into a global sink dwarfs the iteration
  cost being measured. Accumulate a scalar.

Worth remembering: when a benchmark's two scales disagree in direction,
suspect the benchmark before the code.

### What I did not do

Write libcat's number into the doc. `Triples` now documents only what is
stable and measured: the per-call closure allocation (~56 bytes,
independent of graph size), the per-triple yield call and per-quad
graph-term compare, and that a hand-written loop should not be *assumed*
faster. The benchmarks stand as evidence and will catch a Go release
that flips the answer.

Nor did I declare libcat wrong. Their corpus is 12.7M quads to my 250k
with a real merge body; cache residency at 50x the size can genuinely
flip a 25% margin. Filed libcat tasks/212 with the numbers, the two
methodology traps, and a concrete way to settle it (run their merge both
ways at 12.7M and 1.27M -- scale-dependent margin means cache, flat
margin means the loop body differs and I want to see it).

The rest of the ask was informational: their ScanGrain adoption
(-45% B/op), the projector collapse to one copy, and the accepted
parse-segmentation pushback all needed nothing here.
