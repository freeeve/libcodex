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
