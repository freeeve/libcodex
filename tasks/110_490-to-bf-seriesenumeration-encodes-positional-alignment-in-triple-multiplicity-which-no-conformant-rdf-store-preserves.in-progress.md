# 110 -- 490 to bf:seriesEnumeration encodes positional alignment in triple multiplicity, which no conformant RDF store preserves

Opened 2026-07-10.

Found while implementing 108. Not caused by it: 108 only made it visible.

## The bug

`emitInstanceBody` emits `bf:seriesStatement` and `bf:seriesEnumeration` as two
parallel lists of plain literals on the Instance, aligned index for index, with an
empty-string literal standing in for a 490 that carried no `$v`. Decoding zips
them back together by position.

Position is not something RDF can express. The triples are an unordered set, and
identical triples collapse. So two 490s that share a `$v` -- or that both lack one
-- emit the *same* triple twice, and the alignment is destroyed by any consumer
that models a graph the way RDF 1.1 defines it.

```go
rec := recordWith(
    codex.NewDataField("490", '1', ' ', codex.NewSubfield('a', "Series One"), codex.NewSubfield('v', "v. 2")),
    codex.NewDataField("490", '1', ' ', codex.NewSubfield('a', "Series Two"), codex.NewSubfield('v', "v. 2")),
)
```

encodes to (among other triples):

```
<inst> bf:seriesStatement  "Series One" .
<inst> bf:seriesStatement  "Series Two" .
<inst> bf:seriesEnumeration "v. 2" .
<inst> bf:seriesEnumeration "v. 2" .   # identical to the line above
```

rdflib, Jena, oxigraph and N3.js all read three of those four lines. Decode then
sees one enumeration for two statements and drops both `$v`. libcodex itself
round-trips it only because `rdf.Graph` keeps the document's list -- which is why
`allLiteralsOf` now calls `ObjectsWithRepeats` explicitly, with a comment
pointing here. `TestSeriesIdenticalEnumerationRoundTrip` pins the current
behavior.

So the field survives libcodex -> libcodex and dies on libcodex -> anything else.
That is the worst shape for a bug: our own tests are exactly the configuration
that cannot see it.

## What LC does

Read `xsl/ConvSpec-Process6-Series.xsl` from lcnetdev/marc2bibframe2 rather than
inferring. LC does not put series literals on the Instance at all. Each 490 gets
its own node, and the enumeration hangs off *that* node:

```xml
<bf:relation>
  <bf:Relation>
    <bf:relationship>
      <bf:Relationship rdf:about="http://id.loc.gov/vocabulary/relationship/series">
        <rdfs:label>series</rdfs:label>
      </bf:Relationship>
    </bf:relationship>
    <bf:associatedResource>
      <bf:Series>
        <bf:status>            <!-- mstatus/t transcribed; mstatus/tr traced when ind1=1 -->
        <bf:title>...</bf:title>
        <bf:identifiedBy>...</bf:identifiedBy>
      </bf:Series>
    </bf:associatedResource>
    <bf:seriesEnumeration>v. 2</bf:seriesEnumeration>
  </bf:Relation>
</bf:relation>
```

`grouped490Info` groups the subfields of each 490 by `groupNum`, then emits one
`bf:relation` per group. The enumeration is a property of the relation, not of the
Instance, so two 490s with the same `$v` yield two distinct blank-node subjects
carrying `bf:seriesEnumeration "v. 2"` -- two different triples. No alignment, no
multiplicity, nothing for a set to collapse.

It also picks up two things the current mapping drops: `ind1=1` becomes
`mstatus/tr` ("traced") alongside the always-present `mstatus/t`
("transcribed"), and `$x` becomes `bf:identifiedBy` (an ISSN) on the Series.

## Proposed fix

Model each 490 as `bf:relation` -> `bf:Relation` with `bf:associatedResource` ->
`bf:Series`, per LC. Decode reads one 490 per Relation node, taking `$a` from the
Series title, `$v` from the Relation's `bf:seriesEnumeration`, `$x` from the
Series' `bf:identifiedBy`, and `ind1` from the presence of `mstatus/tr`.
`Instance.SeriesStatements` / `SeriesEnumerations` collapse into one
`[]SeriesRelation` and stop being positionally coupled.

`ObjectsWithRepeats` then has no caller in bibframe, which is the tell that this
is the right shape.

## Known consumers of the flat shape

Confirmed by reading libcat at v0.116.1 (via 111), not assumed. Three read sites,
all reading the literals straight off the Instance:

- `project/project.go:1097` -- `Objects(inst, bf:seriesStatement)` -> `i.Series`
- `project/project.go:1106` -- `Objects(inst, bf:seriesEnumeration)`, "first
  non-empty wins", with a comment naming libcodex v0.21.0's positional padding
- `ingest/enrich.go:458` -- `Objects(inst, bf:seriesStatement)` -> `WorkSummary.Series`

Two things follow. libcat never adopted the positional pairing -- it takes the
first non-empty enumeration and discards the rest -- so no consumer today depends
on the multiplicity this task is about, and nobody is currently getting the right
answer for a record with two 490s either. And the migration is bounded: three
loops, one of which already carries a comment saying it is working around this
mapping.

## Why this is not mine to just do

It changes the emitted RDF for every record with a 490 -- a visible, breaking
change to the serialization, not an internal refactor. The three read sites above
would need to move to the Relation shape in the same release, and 102 chose the
flat literals deliberately. It also wants `bf:Series` and `bf:Relation` classes plus the
`mstatus` and `relationship` vocabularies added to the writer, and a decision on
whether to keep reading the old flat shape for a release or two.

Recommendation: do it, in a release that says so. Leaving pending for Eve.
