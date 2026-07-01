# 047 -- bibframe: IRI collisions, lossy reverse crosswalk, 264 semantics

## Motivation

Review found the streaming graph writers silently merge distinct records,
and the reader (reverse crosswalk) loses the source-qualified data that
commit 64907fc (task 037) just added -- the round-trip tests pass only
because their sample records don't exercise the feature.

## Problems

1. **IRI collision across 001-less records in the stream writers** (high --
   serialize.go:61, :95, :133, :38 with graph.go:13-15).
   `NTriplesWriter.Write`, `TurtleWriter.Write`, and `NQuadsWriter.Write`
   all call `graphFromRecord(r)`, which hardcodes `resolveBase(r, 0)`; every
   record lacking a 001 gets base `"r0"`, so their `#r0Work`/`#r0Instance`
   triples silently merge into one resource in the output document. The
   shared encoder keeps *blank* labels distinct but not these IRIs.
   `RecordGraph` (serialize.go:38-40) likewise maps all 001-less records to
   named graph `#r0`. The RDF/XML and JSON-LD writers already thread
   `wr.idx` (bibframe.go:515, :583) -- the serialize.go writers regressed
   relative to them. Existing tests only use records with 001s. Fix: give
   the three writers an index counter; give `RecordGraph` an index-aware
   variant or document the 001 requirement.
2. **Reader drops task-037's source-qualified data on Decode**
   (reader.go:385-400, :363-382). `classificationFields` handles only
   LCC/DDC, so a generic `bf:Classification` node (072, e.g. BISAC with
   source) vanishes entirely; `identifierFields` never reads `pSource`
   (constant defined at reader.go:59, unused), so a sourced 024 loses its
   `$2`. `Decode(Encode(r))` silently loses the whole feature. Fix: add a
   generic `Classification` case mapping source to 072 `$2`, and read
   `pSource` into `$2` on 020/022/024. Extend round-trip tests with `$2`
   samples.
3. **`controlNumber` fabricates 001s from fallback bases**
   (reader.go:570-576). A 001-less record written as `#r0Work` decodes with
   an invented `001 r0`; every such record in separate documents gets the
   *same* invented control number. Treat `^r[0-9]+$` fallback bases as
   "no control number" (or mint a distinguishable prefix the reader strips).
4. **`sniffFormat` misdetects real inputs** (reader.go:139-163). N-Triples
   whose first subject is non-hierarchical (`<urn:isbn:123> ...`) falls to
   the RDF/XML branch and `Decode` returns 0 records with nil error; Turtle
   beginning with `[ a bf:Work ]` is classified JSON-LD. Strengthen the
   `<` case (look for a second term after `>`) and the `[` case (require
   JSON-looking content after).
5. **264 second indicator ignored** (bibframe.go:170-171, :347-357).
   `264 _4 $c ©2015` fills `Provision.Date`, emitted as `bf:date` on a
   `bf:Publication` node -- a copyright date presented as publication date
   (`cleanDate` keeps the `©`); first-wins merging also mixes subfields
   from different 260/264 fields into one Publication. Skip or separately
   map ind2='4' (bf:Copyright), prefer ind2='1' statements. (Same defect
   exists in mods -- see task 050; consider fixing both against one shared
   rule.)

## Acceptance

- [ ] Streaming two 001-less records yields disjoint Work/Instance IRIs in
      N-Triples/Turtle/N-Quads and distinct named graphs from
      `RecordGraph`; regression test without 001s.
- [ ] `Decode(Encode(r))` preserves 020/022/024 `$2` and 072 fields;
      source round-trip tests added.
- [ ] 001-less round trip does not invent a `001 r0` field.
- [ ] `urn:`-subject N-Triples and `[`-leading Turtle decode correctly.
- [ ] `264 _4` no longer emits bf:date on bf:Publication.
