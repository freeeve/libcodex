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
