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

- [ ] The gadget-graph adversarial input either canonicalizes quickly
      (memoized) or fails fast with `ErrCanonComplexity`; W3C rdf-canon
      suite still passes byte-for-byte.
- [ ] Oversized RDF/XML literal in streaming mode returns an error at a
      documented cap; adversary tests added for both new cases.
- [ ] Turtle parse errors report position.
- [ ] Splitter allocation drop visible in `-benchmem` on the streaming
      benchmarks.
- [ ] turtle.go under 500 lines after split; one shared literal escaper.
