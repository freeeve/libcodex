# 102 -- 490$v: encode/decode bf:seriesEnumeration alongside seriesStatement

Filed from libcat on 2026-07-09 (cross-repo ask).

libcat v0.72.0 (tasks/221/222) lets catalogers record a series
enumeration -- the 490$v volume/part -- as `bf:seriesEnumeration` on the
Instance, next to the `bf:seriesStatement` you already round-trip as
490. Today the crosswalk carries only $a:

- encode: `seriesStatement(f)` (bibframe.go:1403) renders a 490 into one
  seriesStatement literal; $v is not read.
- decode: `reader_crosswalk.go:62` emits `seriesField(stmt)` with $a
  only, so an editorial enumeration never reaches exported MARC.

The ask, both directions:

- **encode** (MARC -> BF): 490$v -> `bf:seriesEnumeration` literal on
  the Instance ($a keeps its current shape; repeat $v handling is your
  call -- first-wins matches how libcat's profile bounds the field to
  max 1).
- **decode** (BF -> MARC): when the Instance carries seriesEnumeration,
  append `$v` to the 490 the seriesStatement already produces; a bare
  enumeration with no statement can stay unemitted.

Predicate IRI: `http://id.loc.gov/ontologies/bibframe/seriesEnumeration`
(BF 2.0 core). No urgency -- our fidelity table does not gate 490$v, and
the editor field works end-to-end in the grain, catalog.json, and the
OPAC without it. It only matters for MARC export parity.

## Outcome

Done in 8c28e36, shipped in v0.21.0. Filed libcat tasks/226.

### The ask's premise was wrong, and hid a bug

"$v is not read" -- it was. `seriesStatement()` packed it into the single
`bf:seriesStatement` literal after an ISBD `" ; "`, and `seriesField()`
split it back out. `490 $aFirebrand fiction ;$vbk. 2` already
round-tripped. I checked before writing code, which is the only reason
the next part surfaced.

The packing was silently corrupting data. A series title containing
`" ; "` of its own got split on it, inventing a $v from the second half
of the title:

    490 $aAims ; and methods   (no $v at all)
      -> decode: 490 $aAims $vand methods

Corruption on a record that never had a $v -- worse than the loss the ask
was actually about. Pinned by `TestSeriesTitleContainingSeparator`.

### What landed

LC's `ConvSpec-Process6-Series.xsl` maps 490 $v to `bf:seriesEnumeration`
as its own literal and never packs, so doing the ask properly is the fix.
`bf:seriesStatement` is now the title alone; decode splits nothing;
`splitSeriesStatement` is gone.

### The alignment problem, and why placeholders

Flat sibling literals cannot say which statement an enumeration belongs
to. My first cut dropped enumerations whenever the counts did not line
up -- and that regressed the repo's existing `TestSeriesStatementRoundTrip`,
which has two 490s of which only one carries a $v. The old packing paired
those correctly, by accident of carrying the pair inside one literal.

I did not weaken the test to fit the implementation. Instead: emit one
`bf:seriesEnumeration` per statement in the same order, including an
*empty* literal for a 490 with no $v, so position pairs them. Nothing is
emitted when no 490 had a $v, so clean records stay clean. Needed a
`allLiteralsOf` reader, since `literalsOf` filters empty literals out.
Verified the placeholder survives all four serializations, not just the
RDF/XML the existing test used.

Decode pairs by position when counts match, pairs a lone statement with a
lone enumeration (a hand-written or third-party graph, including libcat's
editor), and otherwise drops rather than attributing to the wrong series.
`TestSeriesEnumerationsFor` pins all six cases.

### Judgement call, flagged not taken

m2b's real 490 shape is a grouped series entity (`bf:title`/`bf:Title`
with a `groupNum` attribute pairing the enumeration). That pairs
unambiguously and needs no placeholders. I did not take it: it moves the
enumeration off the Instance and breaks the flat predicate libcat's
v0.72.0 editor writes and explicitly asked for. Offered it to them in 226
as a breaking change they can request; not doing it on speculation.

### Consumer-visible changes, told to libcat

- `bf:seriesStatement` no longer contains the volume designation.
- Graphs from <= v0.20.0 decode with the packed string intact in $a and
  no $v, rather than split on their separator.
- Empty `bf:seriesEnumeration` literals are positional placeholders and
  must be ignored as data.