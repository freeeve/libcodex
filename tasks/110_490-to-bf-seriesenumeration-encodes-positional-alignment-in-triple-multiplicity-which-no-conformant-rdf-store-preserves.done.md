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

## Outcome

Eve said go ahead, with the deprecation window. Done in fe93acf, shipped in
v0.25.0.

### One correction to this task file

It says the relation goes on the Instance. It does not. `ConvSpec-Process6-Series.xsl`
matches 490 in `mode="work"`, and so does its 8XX template -- the `bf:relation`
hangs off the **Work**. I had written that section from the excerpt I first
grepped rather than from the template's mode, and only caught it on re-reading
before writing code. Everything else in the sketch held up.

### What shipped

```
<#Work> bf:relation _:rel .
_:rel   rdf:type bf:Relation ;
        bf:relationship <.../vocabulary/relationship/series> ;
        bf:associatedResource _:series ;
        bf:seriesEnumeration "bk. 2" .        # on the relation, not the series
_:series rdf:type bf:Series ;
        bf:status <.../mstatus/t> ;           # transcribed, always
        bf:status <.../mstatus/tr> ;          # traced, when ind1=1
        bf:title [ rdf:type bf:Title ; bf:mainTitle "Firebrand fiction" ] ;
        bf:identifiedBy [ rdf:type bf:Issn ; rdf:value "0075-2118" ] .
```

`Instance.SeriesStatements` and `Instance.SeriesEnumerations` are gone, replaced
by `Work.Series []Series` (Title, Enumeration, ISSN, Traced). Series relations
share the Work's existing `bf:relation` list with the 76x-78x linking entries;
the relationship IRI tells them apart, and the linking-entry decoder already
skipped anything whose code is not one of its own, so nothing had to change there.

Reading the XSLT rather than trusting this task's own summary also recovered two
subfields the flat mapping silently dropped: `$x` -> `bf:Issn` on the series, and
`ind1=1` -> the `mstatus/tr` status. Still not carried: `$n`/`$p`, `$l`
classification, `$3` `bflc:appliesTo`, and the 880 parallel grouping. Recorded in
the audit doc.

### The regression the new test caught

`TestSeriesIdenticalEnumerationDistinctTriples` asserts `Dedupe()` removes nothing
from a two-490 record. It failed on the first run: two `bf:Series` nodes each
described the shared `mstatus/t` status node, emitting its `rdf:type` and
`rdfs:label` twice. Identical in kind to the 040 agency bug of 6d11710, found the
same way, fixed the same way -- describe on first mention, reference by IRI after.

Worth stating plainly, because it is the second time: **any shared IRI node
reachable from a repeated structure will be described once per occurrence unless
the emitter is told otherwise.** That is now true of agencies (040) and statuses
(490). The next one will not announce itself either. A `Dedupe() == 0` assertion
is the cheap general guard, and it belongs on any new emitter that references a
vocabulary IRI from a repeatable field.

### Verification

- Both halves mutation-checked. Describing the status per-series fails the dedupe
  test; moving `bf:seriesEnumeration` from the relation onto the series fails both
  the shape test and the identical-`$v` round trip -- which is the defect this task
  was opened about, so the test genuinely pins the fix rather than the shape.
- The interop suite passes against real rdflib, so the emitted graph parses and
  counts the same set-for-set.
- No LC fixture in `bibframe/testdata/loc` carries a 490, so there was nothing to
  validate the shape against empirically; the XSLT is the only authority here, and
  that is worth knowing rather than assuming the fixtures cover it.

### The deprecation window

`legacySeriesFields` reads the old flat literals when a Work carries no series
relation, so a graph libcodex wrote before v0.25.0 still decodes to 490s. It
inherits the defect -- two 490s sharing a `$v` were already one triple in those
graphs, and nothing can recover the second.

This also means `rdf.Graph.ObjectsWithRepeats` keeps a caller, contrary to the
prediction above that losing its last one would be "the tell that this is the
right shape". The legacy path needs it precisely because it decodes the shape that
depended on multiplicity. When the window closes, `allLiteralsOf`,
`seriesEnumerationsFor`, `seriesField` and that call all go together.
