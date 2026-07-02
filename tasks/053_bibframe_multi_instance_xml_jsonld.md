# 053 -- bibframe: multi-instance RDF/XML and JSON-LD output

## Motivation

Task 038 added `WorkInstances.Graph(workBase, instanceBases)` for the priority
`rdf.Graph` / N-Quads path, so a Work with N Instances serializes correctly to
N-Triples, Turtle and N-Quads (which derive from the graph). The two hand-written
emitters -- RDF/XML (`rdfxml.go`) and JSON-LD (`jsonld.go`) -- still only render a
single Work+Instance pair via `appendGraphXML`/`appendGraphJSONLD(b, *BIBFRAME,
base)`. A caller who wants the RDF/XML or JSON-LD serialization of a multi-instance
grain cannot get it today.

## Change

Give the RDF/XML and JSON-LD emitters a multi-instance entry point that mirrors
`WorkInstances.Graph`: the Work element/object emitted once with one
`bf:hasInstance` per Instance, and each Instance emitted under its own IRI with
`bf:instanceOf` back to the Work.

Two viable shapes (author's call):

- Add `appendWorkInstancesXML` / `appendWorkInstancesJSONLD(b, *WorkInstances,
  workBase, instanceBases)` alongside the existing single-pair functions, plus a
  public `EncodeWorkInstancesXML` / `...JSONLD` (or extend the collection
  Writers), or
- Fold this into task 048 problem 1 (unify the three emitters behind one
  authoritative traversal), which removes the parallel-emitter duplication so
  multi-instance support lands once instead of three times.

If 048-P1 is scheduled, prefer doing it there and closing this task as subsumed.

## Requirements

- Work node/object emitted once; N `bf:hasInstance` links; each Instance under its
  own sanitized base with `bf:instanceOf` back.
- Output parses back to a graph isomorphic to `WorkInstances.Graph(...)` for the
  same inputs (extend `TestEncodersIsomorphic`-style coverage to the multi-instance
  case).
- Single-pair `Encode`/`EncodeJSONLD` and their golden output unchanged.

## Acceptance

- [ ] RDF/XML and JSON-LD of a 2-instance grain each parse to a graph isomorphic
      to the N-Quads path's graph.
- [ ] Existing single-instance golden output byte-unchanged.
- [ ] If 048-P1 lands first, this is closed as subsumed with a pointer.

Depends on: 038 (done). Related: 048 problem 1.
