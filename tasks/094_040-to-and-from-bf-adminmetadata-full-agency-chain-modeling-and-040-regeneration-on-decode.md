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
