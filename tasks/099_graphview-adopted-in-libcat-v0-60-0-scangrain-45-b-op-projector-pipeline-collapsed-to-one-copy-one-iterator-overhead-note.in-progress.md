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
