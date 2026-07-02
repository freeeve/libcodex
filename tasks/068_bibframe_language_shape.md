# 068 -- bibframe: bf:Language node shape (bf:code/bf:part), 041 roles

Tier 2 (low/med). From the 059 m2b audit, language area.
Ref: `docs/bibframe_m2b_audit.md` section 5; m2b `ConvSpec-006,008.xsl`,
`ConvSpec-010-048.xsl` (parse041).

## Motivation

Our `bf:Language` node stamps `rdfs:label` with the 3-letter code, which is
non-idiomatic (a code is not a human label). m2b emits either a bare
`<bf:language rdf:resource=".../languages/xxx"/>` (from 008) or a `bf:Language`
carrying `bf:code` and a `bf:part` role -- never `rdfs:label`=code. We also read
only 041 $a; $h (translated-from) -> `bf:accompaniedBy` work is dropped.

## Scope

1. Drop `rdfs:label`=code from the language node; keep the language IRI, and add
   `bf:code` where a node is emitted (or emit the bare resource form from 008).
2. Decide node-vs-bare form to match m2b: 008 -> bare `bf:language` resource;
   041 -> `bf:Language` with `bf:code`.
3. 041 $h (translated-from) -> a related work (`bf:accompaniedBy`) or, minimally,
   stop dropping it silently. $b (summary language) handling optional.

## Hazards

- Sample has 008 lang "eng" and 041 $a "engfre" -> language nodes appear in all
  three goldens; changing the node shape WILL move them. Regenerate and review.
- Reverse `languageField`/`langCode` reads the current node shape -- update it in
  lockstep so 041/008 still round-trip.

## Acceptance

- [ ] Language node no longer uses `rdfs:label`=code; uses IRI (+ `bf:code`).
- [ ] 041 $h no longer silently dropped.
- [ ] Reverse still reconstructs 008/041; goldens regenerated; suite + fuzz green.
