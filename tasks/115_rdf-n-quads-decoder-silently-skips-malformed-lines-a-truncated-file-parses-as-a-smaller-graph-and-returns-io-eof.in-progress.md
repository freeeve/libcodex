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
