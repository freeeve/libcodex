# 055 -- bibframe: single source of truth for the Work/Instance node shape

Split out of 048 problem 1, which is otherwise closed. Scope sharpened with
what tasks 039 and 053 taught about the emitter surface.

## Motivation

The BIBFRAME node shape is hand-written three times: `graph.go` (the
`rdf.Graph` builder feeding N-Triples/Turtle/N-Quads), `rdfxml.go`, and
`jsonld.go` -- today ~1,000 lines and ~50 shape-carrying functions kept
consistent only by convention and by `TestEncodersIsomorphic`. The tax is
real and recurring:

- commit 64907fc added `bf:source` in six places;
- task 039 added media/carrier in three places;
- task 053 collapsed the single- vs multi-instance duplication *within* each
  format (shared body helpers), but the three formats remain parallel.

The isomorphism and golden tests make drift loud, so this is a maintenance
tax, not a correctness risk -- but every future shape change (new provision
property, new identifier qualifier) pays it.

## Approach decided: shared shape declaration (option b)

Deriving RDF/XML and JSON-LD generically from the `rdf.Graph` (048's option
a) is **rejected**, not deferred: a generic serializer cannot reproduce the
hand-tuned, LoC-shaped output, so it would trade the curated golden documents
for arbitrary ones. The fix is to declare the node shape once and render it
three ways, keeping every serialization byte-identical.

Two viable shapes (implementer's call):

1. **Intermediate node tree.** One traversal builds a small shape tree per
   record -- `node{kind (iri|blank|labeled), class, id, props []prop}` where
   `prop{pred, kind (literal|ref|child|group), value, children}` -- and each
   format walks the tree: `graphBuilder` adds triples, the XML renderer
   tracks indent depth, the JSON-LD renderer opens arrays for grouped
   (repeated) predicates. The tree *is* the shape declaration; formatting
   stays in the renderers. Reuse the tree's backing slices per writer to keep
   the P6 allocation win.
2. **Visitor/sink callbacks.** A `shapeSink` interface
   (`openNode/typeRef/leaf/ref/beginGroup/openChild/label/.../close`) with one
   `visitWork`/`visitInstance` traversal and three sink implementations. No
   intermediate allocation, but repetition/grouping must be threaded through
   the callback protocol for JSON-LD's arrays.

Either way the traversal owns *order* (which is JSON-LD key order and XML
element order) and the renderers own *formatting* (indent depth, `,`
placement, `rdf:about` vs `@id`), which is what makes byte-fidelity
achievable.

## Known hazards (from 039/053 spelunking)

- JSON-LD emits repeated predicates as arrays (`"bf:title":[...]`) and
  single-valued ones inline -- the shape declaration must distinguish them.
- Not every child is a blank labeled node: languages are IRI nodes with
  fixed labels, electronic locators are bare IRI refs, admin metadata nests
  an identifiedBy child, contributions pick their wrapper class by
  `Primary`.
- The multi-instance entry points (053) must keep their independent
  workBase/instanceBases handling.
- XML text vs attribute escaping differ (`appendXMLText` vs
  `appendXMLAttr`); the language code path deliberately skips escaping.

## Acceptance

- [ ] A node-shape change lands in exactly one place and all serializations
      pick it up -- demonstrated by adding a trial property in one place and
      seeing it in RDF/XML, JSON-LD, and N-Triples output (the test may live
      on a branch or be reverted; the point is the mechanism).
- [ ] `TestGolden` passes **without** `UPDATE_GOLDEN` -- byte-identical
      RDF/XML and JSON-LD.
- [ ] `TestEncodersIsomorphic`, `TestWorkInstancesEncodersIsomorphic`, all
      round-trip and fuzz targets pass unchanged.
- [ ] No benchmark regression beyond noise on the collection writers
      (`bench_test.go`, `rdfio_bench_test.go`); the reused-buffer pattern is
      preserved.
- [ ] Net shape-declaration surface shrinks (today ~1,000 lines across
      graph.go/rdfxml.go/jsonld.go); files stay under the 500-line
      convention.

Origin: 048 problem 1 (closed). Related: 053's shared body helpers, which
this subsumes.
