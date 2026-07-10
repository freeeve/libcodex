# 108 -- rdf.Graph is documented as a set but stores a multiset, so it disagrees with conformant parsers on any input containing a repeated triple

Opened 2026-07-09.

Surfaced while fixing the interop CI failure in 6d11710. That fix stopped *us*
emitting a duplicate triple. It did not address the fact that we happily *read*
one.

`rdf/rdf.go:89` promises:

> Graph is a set of triples with simple lookup helpers built on first use.

It is not a set. `Graph.Triples` is a `[]Triple` and every parser appends without
checking:

```go
nt := "<http://a> <http://p> <http://o> .\n<http://a> <http://p> <http://o> .\n"
g, _ := rdf.ParseNTriples([]byte(nt))
len(g.Triples)  // 2. rdflib, Jena, and every conformant parser read 1.
```

Same for `ParseRDFXML` and `ParseJSONLD`. Repeated statements are legal and
common in hand-written RDF/XML, where one node gets described under two
properties -- which is precisely the shape our own writer was emitting until
6d11710.

That is what the interop test caught, but only from the writer side:

```
interop_test.go:101: RDF/XML triples: ours=92 rdflib=90
```

Those two extra triples were duplicates in our own output. Had they arrived in
someone else's document instead, we would still have counted 92, and no test
would have said a word.

## What is *not* affected

Checked rather than assumed:

- **Canonicalization is immune.** `Graph.Canonical()` on the two-triple graph
  above emits a single line. RDFC-1.0 collapses the duplicate, so canonical
  comparison and `Dataset.Canonical()` already behave as sets.
- **Lookup is immune.** `Objects`/`Object`/`Literal` go through the `spo` index,
  which is subject-keyed and tolerates duplicates; a caller reading a property
  gets the same answer either way. (It does mean `Objects` can return the same
  term twice -- see below.)

So the blast radius is `len(g.Triples)`, direct iteration over `Triples`, and
`Objects` returning repeats.

## The decision this needs

Three options, and choosing among them is a judgement about what `rdf.Graph` is
*for*:

1. **Dedupe on parse.** Honest to the doc and to RDF. Costs a `map[Triple]bool`
   over every parse; measure against `BenchmarkCorpus*` before committing, since
   parse throughput is a stated goal of this package.
2. **Dedupe lazily**, alongside the existing `spo` index build, so only callers
   who ask pay. But then a count has to trigger an index build, which makes a
   cheap accessor quietly expensive.
3. **Change the doc.** Call `Graph` a triple *list* that preserves document
   order, and give callers who want set semantics an explicit `Dedupe()` --
   `Canonical()` already serves the comparison case.

Option 3 is the one I lean toward, and for a concrete reason: document order and
multiplicity are how the writer bug in 6d11710 became visible at all. A set would
have silently swallowed it, and the interop test -- which exists to compare our
count against rdflib's -- would have had nothing to compare. A parser whose job
is faithful round-tripping should probably report what the document said and let
the caller ask for the set.

But option 3 trades away the "agree with rdflib triple-for-triple" property the
interop test asserts, which was a deliberate choice when that test was written.
Reversing it is not mine to do unilaterally.

Leaving pending for Eve.

## Outcome

Eve chose option 3. Done in e132fa6, shipped in v0.24.0.

Research first, since the doc was quoting a spec at us. **RDF 1.1 Concepts §3**, and
the **RDF 1.2 Candidate Recommendation** of April 2026, both say verbatim: "An RDF
graph is a set of RDF triples." N-Triples §8.2 draws the line exactly where it
matters -- the *document* is a "sequence", the graph it denotes is "a set of RDF
triples". So the old doc comment was not loosely worded; it was claiming a
normative property we did not have.

The mainstream architecture is parser-streams-duplicates, store-dedupes-on-insert:
RDF/JS `DatasetCore.add()` ("Existing quads … will be ignored"), rdflib's memory
store (re-adding is a no-op, `len(graph)` does not move), N3.js `addQuad()`
(returns `false` and skips), oxigraph `Store::insert()` (returns `true` only if
"not already" present). But in **Go** the picture is looser, and it is where we
sit: `knakk/rdf` has no graph type at all, just `DecodeAll() []Quad`;
`piprate/json-gold`'s `RDFDataset` is `map[string][]*Quad` with dedup deferred to
an explicit normalization pass; `deiu/rdf2go` keys a map by pointer and is an
accidental multiset. Only Cayley, a real quad store, dedupes on insert.

There is one clean precedent for a duplicate-preserving graph as a first-class
type -- Haskell rdf4h's `TriplesGraph`, which "maintains the triples in the order
that they are given in … especially useful for holding N-Triples" and states that
"duplicate triples are not filtered". Two details of it decided this task: it is
the non-default, explicitly less-efficient option next to a deduplicating
`MGraph`, and **it deduplicates query results**.

### The evidence that settled it

- **Duplicates are the norm, not an artifact.** Every LC BIBFRAME fixture in
  `bibframe/testdata/loc` contains them. Verified textually, no parser involved:
  `2543127.work.nt` is 449 lines for 389 distinct triples. One triple appears
  sixteen times; `organizations/dlc` is restated five times. `marc2bibframe2`
  re-describes a shared node under every property that references it -- the exact
  pattern I removed from our own writer in 6d11710.
- **Dedup-on-parse costs too much.** Measured on the corpus benchmark: parse-only
  78ms / 730 MB/s / **5 allocs**; parse + presized dedup 200ms / 280 MB/s / 331k
  allocs; parse + naturally-grown map 330ms / 175 MB/s. 2.5-4x slower, +63%
  memory, and it destroys the arena the parse path was built around, for a
  property no caller needs.
- **The leak was real and it was in the query surface.** `Objects(dlc,
  rdfs:label)` returned the same label five times; `Objects(subjects, rdf:type)`
  returned nineteen terms. libcat has 24 `Objects()` call sites and
  `ingest/enrich.go:430` reads `s.Items += len(merged.Objects(inst, hasItem))` --
  a user-visible count taken straight off the slice.

### What landed

`Graph` is documented as a document-order list, with the spec's definition and
the reason for departing from it. `Dataset` likewise. `Objects` and
`GraphView.Objects` return each distinct object once; `SubjectsOfType` already
did. `Dedupe()` gives set semantics on request and reports how many it removed --
on LC's file it removes exactly 61, landing on 389, which is what rdflib reports.
`Canonical()` already collapsed duplicates, so graph comparison never needed it.

`Objects` dedupes with a linear scan and promotes to a hash set past 16 results,
sized from the subject's index bucket. Both constants are measured, not guessed:
the first cut sized the map at `2*len(out)` and rehashed its way to 1µs per
object (255µs for 256 objects); sizing from the bucket cut that to 47µs. A sweep
of the promotion threshold at 8 / 16 / 32 put 16 at the optimum for the 16-object
case (1642ns vs 3089 and 1888) with no meaningful loss at 256. At the sizes that
actually occur -- three or four objects for a predicate -- the dedup costs about
3%.

### What it broke, and what that revealed

`Objects` deduping regressed 490 round-tripping, and the regression was the
interesting part. `bf:seriesEnumeration` is positional, aligned index-for-index
with `bf:seriesStatement`, so **two 490s carrying the same `$v` encode to two
identical triples** and deduping dropped one, misaligning both. Caught it by
constructing the case rather than trusting the green suite, which had no test for
it.

The fix here is `ObjectsWithRepeats`, an explicit list view, called from
`allLiteralsOf`. But the underlying mapping encodes positional correspondence in
triple multiplicity, which RDF's abstract syntax cannot carry: that field is
lossless through libcodex and lossy through rdflib, Jena, or anything else that
models a graph as a set. Our own tests could never have seen it, because our own
graph was the one implementation that preserved it. Filed as **110**, with LC's
actual model from `ConvSpec-Process6-Series.xsl` (one `bf:Relation` node per 490,
enumeration attached to the relation, not the Instance) as the proposed fix.

### The interop test

It asserted `len(g.Triples) == rdflib's graph size`, which quietly demanded that
our writers never restate a node -- a demand LC's own converter does not meet.
That assertion, not the writer, is what failed CI for five releases. It now
compares *distinct* triples.

Checked by mutation what the reformulated test can and cannot see: it still
catches a parser that drops a triple (`ours=89 rdflib=90`), it now tolerates a
writer that restates one, and it never could catch a writer that omits one --
because it parses our own output with both parsers, so an omission is invisible to
both. That last property is pre-existing and unchanged; worth knowing, since the
test's name suggests more coverage than it has.
