# 094 -- 040 to-and-from bf:AdminMetadata: full agency chain modeling and 040 regeneration on decode

Filed from libcat on 2026-07-09 (cross-repo ask).

Context: libcat's marc-fidelity contract lists 040 as lost ("provenance
is modeled as named graphs, not a 040" -- docs/marc-fidelity.md). Eve
wants to revisit: named graphs (statement-level data provenance) and
040 (record-level cataloging-agency chain) are orthogonal axes, so the
plan is to use both -- model the 040 semantically and regenerate it at
export, with libcat deriving the modifying-agency chain from its
editorial graph. libcat side is tracked as their tasks/192, blocked on
this.

Current state here (v0.17 tree): bibframe.AdminMetadata already
captures 001/003/005 and 040 $e (DescriptionConventions -> 
bf:descriptionConventions via shape.go). Two gaps:

1. FromRecord: capture the rest of 040 -- $a (original cataloging
   agency) and $c (transcriber) -> bf:source on the AdminMetadata
   node, $d (modifying agencies, repeatable) ->
   bflc:descriptionModifier, $b -> bf:descriptionLanguage. Mirrors
   LC's marc2bibframe2 mapping, so the RDF shape stays
   ecosystem-readable. Struct fields to match (OrigAgency,
   Transcriber, Modifiers []string, DescriptionLanguage).
2. The decode/ToRecord side: regenerate a 040 from AdminMetadata
   ($a $b $c $d... $e in canonical subfield order). Today nothing
   emits 040, so records that arrived with one only round-trip through
   libcat's verbatim sidecar rather than the model.

Acceptance sketch: MARC with 040 $aDLC $beng $cDLC $dOCLCQ $erda
round-trips field-exact through FromRecord -> RDF -> decode; a
BIBFRAME with no AdminMetadata emits no 040 (libcat synthesizes its
own downstream); admin_completeness_test extended accordingly.

## Outcome

Done in 2c28190. Both acceptance criteria hold, in all four
serializations.

The ask's proposed mapping needed two corrections, both settled by
reading LC's own XSLT (`xsl/ConvSpec-010-048.xsl` in lcnetdev/
marc2bibframe2) rather than inferring:

- `descriptionModifier` lives in the **bf:** namespace, not bflc:.
  Confirmed by LC's published RDF in `bibframe/testdata/loc/`.
- $a maps to **bf:assigner**, not bf:source. $c maps to *nothing* --
  LC has no template for it at all.

That last point is what made "$a and $c both -> bf:source" unworkable:
in the acceptance record both are DLC, so both would emit the identical
triple `<admin> bf:source <organizations/dlc>`, RDF collapses it to
one, and decode cannot tell a $a+$c pair from a bare $a.

LC's actual answer, which we now copy: preserve the whole field as a
`bf:Note` typed `mnotetype/internal` whose rdfs:label is the field in
marcKey form (`040  $aDLC$beng$cDLC$dOCLCQ$erda`), *alongside* the
semantic properties. The note is the round-trip carrier, so $c survives
with no invented vocabulary. Decode prefers the note and falls back to
the modelled properties (recovering everything but $c) for a graph
without one.

Final mapping, all with vocabulary IRI + bf:code:

    $a -> bf:assigner            (bf:Agent, organizations/)
    $b -> bf:descriptionLanguage (bf:Language, languages/)
    $c -> internal note only
    $d -> bf:descriptionModifier (bf:Agent, organizations/) one per $d
    $e -> bf:descriptionConventions           (pre-existing)

Two deliberate divergences from m2b's commented-out code, both
supersets: one bf:descriptionModifier per $d (m2b keeps only the last),
and a bf:assigner IRI for any IRI-safe agency code (m2b mints one only
for DLC).

Validated against LC's published RDF, not just our own output: the
three `bibframe/testdata/loc/*.inst.rdf` fixtures now decode back to
their original 040s ($c included), e.g. 21263493 ->
`040  $aDLC$beng$cDLC$dDLC$egihc`.

Side effects: 040 moved from `lostTags` to `coreTags` in the loss gate;
the JSON-LD @context gained an `mnotetype` prefix (the only golden
change -- RDF/XML writes extra rdf:types as full IRIs, so `sample.rdf`
is untouched). `docs/bibframe_m2b_audit.md` updated.

libcat's tasks/192 is unblocked; filed their tasks/193 as the notice.
