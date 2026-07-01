# 046 -- rdf: canonicalization cost accounting, adversarial gaps, cleanup

## Motivation

Commit db71490 bounded canonicalization recursion, but the review found the
complexity budget counts the wrong unit, leaving a CPU-exhaustion gap; the
streaming decoder's "constant memory" claim has an RDF/XML hole; and a few
hot-path and diagnosability issues remain.

## Problems

1. **Canon `work` budget counts permutations, not cost** (canon.go:293-298,
   :251-255). Each `permute` visit charges 1 work unit but performs an
   O(|issued|) `iss.clone()` and can trigger full uncached
   `hashFirstDegree` serializations of all a node's quads. Adversarial
   shape: m disjoint 2-node gadgets whose nodes each occur in E quads share
   one first-degree hash; ~8 MB of input (m=256, E=1000) burns ~5x10^8 quad
   serializations while staying far under `maxCanonWork` -- minutes of CPU,
   no `ErrCanonComplexity`. Fix: memoize first-degree hashes on the
   canonicalizer (pure function of the immutable quad set; also speeds up
   benign data) and charge `work` proportionally to quads serialized /
   issuer entries cloned.
2. **RDF/XML streaming buffers unbounded literals** (rdfxml.go:180-201).
   The Turtle path enforces `maxStatementBytes`, but `parseProperty`
   accumulates CharData without limit -- one multi-gigabyte literal in a
   streamed dump buffers wholly in memory, contradicting decoder.go:28-31's
   constant-memory claim. Cap accumulated CharData per property.
3. **Adversary test gaps** (adversary_test.go). Covers depth chains and
   permutation blowup (test074) but neither the cost-per-work-unit gap (1)
   nor the RDF/XML literal case (2). Add both.
4. **`turtleError` discards position** (turtle.go:65-68). `pos` is captured
   but `Error()` returns the constant `"rdf: malformed Turtle"` -- parse
   failures on multi-megabyte documents are undiagnosable. Include byte
   offset (ideally line/column).
5. **Streaming splitter allocates 64 KiB per read** (decoder.go:255-257).
   A fresh `chunk` is allocated each iteration then copied into `s.buf` --
   one garbage allocation plus a full extra copy per 64 KiB of a multi-GB
   stream. Reuse the chunk or read directly into `s.buf`'s spare capacity.
6. **File split and escaper dedup** (turtle.go, 775 lines -- over the
   500-line convention). Parser (1-585) and serializer (587-775) are
   unrelated units; split into `turtle.go`/`turtle_write.go`. Relatedly,
   `canonAppendLiteral` (canon.go:428-466) near-clones
   `appendEscapedLiteral` (ntriples.go:220-254) and `canonLine` duplicates
   `Encoder.AppendQuad`; parameterize one escaper by table to stop the two
   paths drifting.

## Acceptance

- [x] The gadget-graph adversarial input either canonicalizes quickly
      (memoized) or fails fast with `ErrCanonComplexity`; W3C rdf-canon
      suite still passes byte-for-byte. (4.4 MB gadget graph: 112 ms.)
- [x] Oversized RDF/XML literal in streaming mode returns an error at a
      documented cap; adversary tests added for both new cases.
- [x] Turtle parse errors report position.
- [x] Splitter allocation drop visible in `-benchmem` on the streaming
      benchmark (1134 KB/op -> 831 KB/op).
- [x] turtle.go under 500 lines after split; one shared literal escaper.

## Resolution

1. **Canon cost accounting + memoization.** `hashFirstDegree` is memoized on the
   canonicalizer (a pure function of the immutable quad set), collapsing the
   gadget-graph's repeated re-serializations; a `spend` helper charges the work
   budget proportionally to issuer entries cloned per permutation, so a graph that
   is cheap in permutations but expensive per permutation still fails fast. W3C
   rdf-canon still passes byte-for-byte.
2. **RDF/XML literal cap.** `parseProperty` caps accumulated CharData at
   `maxLiteralBytes` (16 MiB, matching the Turtle statement cap) and returns
   `errLiteralTooLarge`.
3. **Adversary tests.** `TestCanonGadgetGraphMemoized` (200 symmetric 2-node
   gadgets, 500 quads/node) and `TestStreamingRDFXMLLiteralBounded`.
4. **Turtle error position.** `turtleError` now carries line/column and byte
   offset (`lineCol`); the message locates the failure.
5. **Splitter allocation.** `turtleSplitter.next` reads into `buf`'s spare
   capacity (`slices.Grow`) instead of allocating a fresh 64 KiB chunk per read;
   `BenchmarkStreamTurtle` shows 1134 KB/op -> 831 KB/op.
6. **File split + escaper dedup.** `turtle.go` (846 lines) split into `turtle.go`
   (grammar, 306), `turtle_lex.go` (term/lexical parsing, 350) and
   `turtle_write.go` (serialization, 191). `canonAppendLiteral` and
   `appendEscapedLiteral` now delegate to one `appendLiteralEscaped` parameterized
   by a `literalEscape` config, so the two profiles cannot drift.
