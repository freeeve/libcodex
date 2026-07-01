# 045 -- rdf: parser correctness (base IRIs, blank-label collisions, streaming)

## Motivation

Review of the rdf package found two high-severity parser bugs that silently
corrupt bibliographic identifiers and merge distinct nodes, plus several
smaller conformance gaps. The RDFC-1.0 canonicalizer itself, the arena, and
N-Triples escaping all checked out clean (full W3C rdf-canon vectors pass).

## Problems

1. **Turtle base resolution mangles non-hierarchical absolute IRIs** (high --
   turtle.go:347). Absoluteness is tested with `strings.Contains(raw, "://")`,
   so with `@base` declared, `<urn:isbn:0451450523>`, `mailto:`, `doi:`,
   `info:lccn` etc. get base-prefixed: subject becomes
   `http://ex/urn:isbn:...`. For a bibliographic library these IRIs are core
   data. The `#`-prefix branch also keeps fragment references relative
   instead of resolving them. Fix: treat `[A-Za-z][A-Za-z0-9+.-]*:` as
   absolute; base-join only genuinely relative references.
2. **RDF/XML and JSON-LD fresh blank labels collide with document labels**
   (high -- rdfxml.go:71 vs :232, jsonld.go:48 vs :221). `fresh()` mints
   `"b"+n` / `"j"+n` while `rdf:nodeID` / `"@id": "_:..."` values are used
   verbatim, so `rdf:nodeID="b1"` plus one anonymous node silently merge
   into a single `_:b1`. The Turtle parser already maintains the injectivity
   invariant (`"u"`/`"t"` prefixes, turtle.go:82-84); apply the same scheme
   to both parsers.
3. **Streaming Turtle drops statements at `.` without following whitespace**
   (decoder.go:296-304 with :210-215). `statementEnd` requires whitespace
   after `.`, so `<s> <p> <o>.<s2> ...` puts two statements in one chunk and
   `streamTurtle` parses only the first -- stream and whole-document parse
   diverge. FuzzStreamTurtle currently waives the differential check
   (decoder_fuzz_test.go:133-135). Fix the terminator recognition and
   re-enable the differential.
4. **JSON-LD triple order is map-iteration nondeterministic** (jsonld.go:136).
   `for key, val := range obj` makes serialized output differ run-to-run
   (blank labels are numbered by encounter order), breaking reproducible
   diffs. Sort the non-`@` keys.
5. **JSON-LD numbers always become `xsd:double` in non-canonical form**
   (jsonld.go:166-167). `5` should map to `xsd:integer` per the
   JSON-LD-to-RDF algorithm; `"5"^^xsd:double` is also not a canonical
   double lexical form, so a Turtle round trip changes the datatype.
6. **N-Triples blank label swallows the terminating dot** (ntriples.go:75-80).
   `... _:y.` (valid, no space) parses as label `y.` -- one node silently
   split into two. Trim the trailing `.` as the Turtle path does
   (turtle.go:363).
7. **Turtle `number()` accepts bare signs and empty exponents**
   (turtle.go:528-531). `+`/`-`/`1e` are accepted as literals with invalid
   lexical forms instead of erroring.

## Acceptance

- [ ] `@base` + `urn:`/`mailto:`/`info:` subjects parse unchanged; fragment
      refs resolve against base; W3C Turtle suite still passes.
- [ ] Document-supplied `nodeID`/`_:` labels can never merge with generated
      ones in any parser (test with the colliding shapes above).
- [ ] Stream vs whole-document Turtle differential fuzz check re-enabled and
      passing.
- [ ] Same JSON-LD document yields byte-identical N-Quads across runs;
      integer-valued numbers map to `xsd:integer`.
- [ ] Regression tests for the `_:y.` and bare-sign cases.
