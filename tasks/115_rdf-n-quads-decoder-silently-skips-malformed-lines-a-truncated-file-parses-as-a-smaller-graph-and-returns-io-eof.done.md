# 115 -- rdf N-Quads decoder silently skips malformed lines: a truncated file parses as a smaller graph and returns io.EOF

Filed from libcat on 2026-07-10 (cross-repo ask). Found while rewriting
`lcat project` to stream `catalog.nq` instead of slurping it (libcat tasks/279).

## What happens

Both N-Quads entry points drop a line they cannot parse and read on. Neither
returns an error, and neither reports what it lost.

```go
raw := []byte("<#w> <http://p> <http://o> <g> .\n<#broken \n")

ds, err := rdf.ParseNQuadsShared(raw)
// err == nil, len(ds.Quads) == 1

dec := rdf.NewDecoder(bytes.NewReader(raw), rdf.NQuads)
for {
    q, err := dec.DecodeQuad()
    if errors.Is(err, io.EOF) { break }   // <- reached, err is io.EOF
    if err != nil { panic(err) }          // <- never reached
    _ = q
}
// one quad decoded, io.EOF, no error
```

A file consisting entirely of `this is not rdf at all` parses as **zero quads
and no error**.

## Why libcat cares

`lcat project` reads a `catalog.nq` that an earlier build step wrote. If that
write was truncated -- disk full, killed process, a partial S3 GET -- the
projector sees a smaller, well-formed graph. It emits a smaller catalog and
exits 0. The build goes green and the site ships with works missing.

libcat refuses that failure class elsewhere on purpose (its tasks/246), so it is
not something we can absorb by convention. And it cannot be recovered
downstream: by the time libcat holds the `Dataset`, the dropped lines are gone
without a trace. A count heuristic ("we expected ~1.7M quads") is the only thing
available to a caller, and that is a guess, not a guard.

The libcat side currently pins the behavior with a test named
`TestAMalformedLineIsSilentlySkippedByBothParsers`, precisely so it fails loudly
the day this changes -- which is the day we want. `project/load.go` carries a
comment pointing here.

## What we'd like

A malformed line should be an error, with the line number, from both
`Decoder.DecodeQuad` and `ParseNQuadsShared`. `io.EOF` should mean end of input,
not "end of input, and by the way some of it wasn't N-Quads".

Skipping is defensible for some corpora, so if callers rely on it, an opt-in is
fine -- `rdf.NewDecoder(r, rdf.NQuads)` strict by default, with something like
`dec.SkipMalformed(true)` for the lenient path. What matters is that the default
does not lose data quietly.

If strict-by-default is too breaking for a minor, an error-returning variant
plus a deprecation note on the lenient one would let libcat move.

## Repro

```go
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/freeeve/libcodex/rdf"
)

func main() {
	raw := []byte("<#w> <http://p> <http://o> <g> .\n<#broken \n")

	ds, err := rdf.ParseNQuadsShared(raw)
	fmt.Printf("ParseNQuadsShared: %d quads, err=%v\n", len(ds.Quads), err)

	dec := rdf.NewDecoder(bytes.NewReader(raw), rdf.NQuads)
	n := 0
	for {
		_, err := dec.DecodeQuad()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			fmt.Println("DecodeQuad error:", err)
			return
		}
		n++
	}
	fmt.Printf("DecodeQuad: %d quads, clean EOF\n", n)
}
```

Prints `1 quads, err=<nil>` then `1 quads, clean EOF`. Expected: an error naming
line 2.

## Outcome

Done in 3daf4c0, shipped in v0.26.0. Strict by default, as asked.

Reproduced first, and it was worse than reported: **`ParseNTriples` and
`ParseNTriplesShared` do it too**, not just the N-Quads pair. All four bulk entry
points and the streaming decoder shared one line parser.

### The bug was a bool

`parseNQuadLine` returned `(Quad, bool)`, and `false` meant two different things:
"this line carries no statement" (blank, comment) and "this line is not
N-Quads". Every caller treated both as skip. That single conflation is the whole
defect -- a truncated document is indistinguishable from a document with a
comment at the end.

It now returns a three-state `lineKind` (`lineStatement` / `lineIgnorable` /
`lineMalformed`). Blank and comment lines stay free; a malformed one becomes:

```
rdf: line 2: malformed N-Triples/N-Quads statement: "<http://broken"
```

a `*rdf.SyntaxError` with a 1-based `Line` field, from `ParseNTriples`,
`ParseNTriplesShared`, `ParseNQuads`, `ParseNQuadsShared` and
`Decoder.DecodeQuad`/`Decode`.

### The API call

Strict by default, with `NewDecoder(r, NQuads).SkipMalformed(true)` for the
lenient path -- the shape suggested. Not the "error-returning variant plus a
deprecation note" fallback: this is pre-1.0, the lenient behavior loses data with
no way for a caller to notice, and a deprecated-but-default footgun would have
left every existing caller in the failure mode. The chainable setter keeps the
opt-in to one line.

`SkipMalformed` lives on the Decoder only. The bulk parsers take no options, and
adding `ParseNQuadsLenient` + a `Shared` twin for each of two formats is four
functions to serve a case that wants streaming anyway. A caller who genuinely
wants to skip noise over a whole document can drain a Decoder.

The bulk parsers still return the statements read before the bad line, so a
caller can report how far it got. The error is what says the graph is not the
document; the partial graph is a diagnostic, not a result. Pinned by a test that
says so.

### Verification

- All four bulk parsers and both decoder paths tested against the truncated
  document, and against `"this is not rdf at all"` (the zero-quads-no-error case).
- Line numbering checked on a document whose bad line is the 10th, so an
  off-by-one cannot hide behind `line 2`.
- Blank/comment lines pinned as non-errors, which is the distinction the bool
  collapsed.
- Three mutations, each reverted: reclassifying `lineMalformed` as ignorable fails
  7 assertions; ignoring `skipBad` fails both lenient tests; a `+1` on the line
  counter fails the late-line test.
- The stream fuzzers assert the bulk and streaming parsers agree triple-for-triple
  on arbitrary input. They still pass, which is the real check that both stop at
  the same line rather than merely both erroring.
- No parse-throughput regression; the `lineKind` return is the same width as the
  bool and the arena is untouched.

### Two callers changed shape

`TestStreamingDecoder` existed to assert skipping, so it now opts in via
`SkipMalformed(true)` and a strict sibling was added. `sharedParseDoc` carried a
trailing `not a statement` line to exercise skipping while really testing
zero-copy semantics; the malformed line is gone from it and strictness is tested
where it belongs.

### Consequence worth naming

`bibframe.Decode` and `skos.Parse` sniff the serialization and fall back to
N-Triples. Feeding either a document that is not RDF used to yield an empty graph
and no error; it now yields a `*SyntaxError`. That is the same bug in a second
place, fixed by the same change, and no test relied on the silence.
