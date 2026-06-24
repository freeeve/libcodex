# W3C RDF test suite (excerpt)

These Turtle (`.ttl`) and N-Triples (`.nt`) files are a curated subset of the
W3C RDF 1.1 test suite, from <https://github.com/w3c/rdf-tests> (directories
`rdf/rdf11/rdf-turtle` and `rdf/rdf11/rdf-n-triples`). Each `NAME.ttl` eval test
is paired with its expected `NAME.nt` output.

`TestW3CConformance` parses each Turtle file and checks the result matches the
expected N-Triples (up to blank-node isomorphism), and round-trips every file
through our serializer and parser. Point `W3C_RDF_TESTS` at a full local checkout
(its `rdf/rdf11` directory) to run the complete suite.

Tests requiring relative-IRI resolution against a document base URI are omitted:
the parser's `Decode` takes no base, so it does not resolve document-relative
IRIs (a documented limitation).

Licensed under the W3C Test Suite License and the W3C 3-clause BSD License; see
LICENSE.md. Copyright © W3C and contributors.
