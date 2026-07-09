# 096 -- native SKOS subjects: one heading per term, English-preferred label

Filed from libcatalog (tasks/147, 2026-07-06) while adopting v0.15.0's
native SKOS subject reading (your 089).

## Bug

A bf:subject IRI node carrying prefLabels in several languages mints one
6xx PER prefLabel, and the language pick is effectively arbitrary. Repro
(libcatalog's TestDecodeGrainMARCControlledSubjects, grain shape):

    <term> skos:prefLabel "Queer joy"@en
    <term> skos:prefLabel "Queer joy (es)"@es

decodes to TWO 650s ($a "Queer joy" and $a "Queer joy (es)"), where the
089 spec said "prefLabel (English first) as the heading" -- one heading
per term, English preferred, then untagged, then a deterministic fallback
(e.g. first language tag sorted).

rdfs:label nodes may have the same issue -- worth covering both in
subjectFields' label pick.

## Workaround downstream

libcatalog's DecodeGrainMARC pre-filters prefLabels to the preferred
language decode-locally (bibframe/marcverbatim.go, tasks/147) -- delete
that filter once this lands.

## Outcome

Done in bd9740c.

Reproduced first, and the libcodex-side symptom differs from the report:
a term with prefLabels in several languages decoded to *one* 650, but
with an arbitrary label -- `subjectFields` took whichever literal the
graph listed first, so Spanish won over English. (The two-650s shape in
libcat's repro comes from their grain minting a separate concept node
per label; from libcodex's side each node yields one heading already.)
rdfs:label had the identical defect, as suspected.

New `preferredLabel` picks exactly one label per term:

    1. exact "en" language tag (case-insensitive)
    2. any "en-*" subtag
    3. an untagged literal
    4. the lowest language tag lexicographically

Steps 3 and 4 are what make a label-set with no English member
deterministic -- RDF document order carries no meaning, so without them
the heading would depend on the serializer. Both rdfs:label and
skos:prefLabel go through it, keeping the existing rdfs:label-before-
prefLabel precedence.

Also deduped the bf:subject edge itself: RDF is a set, so a repeated
edge is one subject, not one heading per repeat.

libcat can delete the DecodeGrainMARC prefLabel pre-filter; noted in
their tasks/194.
