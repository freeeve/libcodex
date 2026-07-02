# 058 -- bibframe: lift ISBN qualifying information into bf:qualifier

Spun out of the "Also (minor, separate)" note on task 057. MARC 020 (ISBN)
carries qualifying information -- a physical-form or volume note -- either as a
trailing parenthetical in $a ("9781234567842 (electronic bk)") or, in RDA
records, as a discrete $q. BIBFRAME models this as bf:qualifier on the
bf:Identifier, distinct from rdf:value (the bare number) and bf:source (scheme).

## Prior art

LC's marc2bibframe2 (ConvSpec-010-048.xsl) is the reference: for 020 it sets
rdf:value to the ISBN with any parenthetical removed
(`concat(substring-before(.,'('),substring-after(.,')'))`) and emits bf:qualifier
from both the parenthetical text and any $q. bibframe.rdf defines bf:qualifier as
an owl:DatatypeProperty (range rdfs:Literal), "qualifying information associated
with an identifier". This task follows that split exactly.

## Done

- `Identifier` gains a `Qualifier` field (bibframe.go).
- `appendIdentifiers` takes a `qualified` flag (020 -> true; 022/024 -> false).
  When set, it strips a trailing "(...)" from $a via a new `splitParenthetical`
  helper and reads $q, preferring an explicit $q over the parenthetical.
- `emitIdentifier` (shape.go) emits `bf:qualifier` after rdf:value across all
  three sinks; `qpQualifier`/`pQualifier` added to vocab.go/reader.go.
- Reverse crosswalk (`identifierFields`/`identifierField`) reads bf:qualifier back
  into 020 $q -- the qualifier round-trips, normalized from a parenthetical into
  the modern $q subfield.
- Tests: `isbn_qualifier_test.go` (forward parenthetical/$q, splitParenthetical
  edge cases, Encode->Decode round-trip). Goldens unchanged (the sample ISBN has
  no qualifier); FuzzFromMARC and FuzzDecode pass.

## Scope notes

- Only ISBN (020) is qualified here; 022/024 pass `qualified=false` (their $q/$c
  semantics differ and no request covers them yet).
- 010 (Lccn) reverse stays $a-only; the forward path never sets a Lccn qualifier.
