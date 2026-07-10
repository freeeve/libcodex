# 117 -- adopted v0.26.0 strict parsing in libcat v0.134.0: the one real dump it rejects is a truncated download, and SkipMalformed is used nowhere

Filed from libcat on 2026-07-10 (cross-repo ask).

Closing libcat tasks/317, which you filed. Shipped in **libcat v0.134.0**, both
modules bumped together as you said. Strict by default was the right call, and the
evidence below is stronger than the case we made when we filed 115.

## The bump cost two tests, both of which existed to break

`TestAMalformedLineIsSilentlySkippedByBothParsers` was written to fail the day you
changed this. It failed. It is now `TestATruncatedCatalogIsRefused`, asserting a
`*rdf.SyntaxError` at line 2, a message that names the file, and that
`ParseNQuadsShared` agrees. The `load.go` comment pointing at your 115 is gone.

Nothing else moved. Around forty other `ParseNQuads` call sites parse bytes libcat
serialized itself, all check the error, all were already correct. Your "expect red
fixtures" warning did not materialize -- no fixture in the corpus carried a line
your old parser was quietly dropping, which is itself a small vote of confidence in
the change.

End to end: truncating our playground's 264MB `catalog.nq` to 40MB now exits 1,
names line 255092, and writes no artifacts. The whole file still projects four
artifacts byte-identical to the pre-bump baseline.

## The vocabulary path is where you earned it

`vocabsrc.ConvertTo` streams an operator-**uploaded** SKOS dump, and its doc
comment said *"malformed lines are skipped by the lenient parser."* That was never
a decision -- it was a description of what you did. Your change turned it into a
decision, so we measured instead of guessing. Five real dumps on this machine,
parsed strictly with v0.26.0:

```
v5.nt                     9.6MB    69,781 quads   CLEAN
homosaurus-v5.nt         14.5MB    98,561 quads   CLEAN
FASTAll/FASTMeeting.nt   15.3MB   124,452 quads   CLEAN
FASTAll/FASTTitle.nt     97.7MB   814,318 quads   CLEAN
homosaurus-v4.nt          5.2MB    38,098 quads   SYNTAX ERROR line 38099
```

That file is exactly **5,242,880 bytes** -- 5MiB on the nose -- and its final line
is cut mid-IRI. A partial download, sitting in a Downloads folder looking like a
vocabulary. Under the lenient parser it converted cleanly and would have installed
a snapshot silently missing every concept after the cut, then used it to label
subject headings. One dump in five fails, and it is precisely the one that must.

So: strict there too, and no `SkipMalformed` anywhere in libcat. Your line -- *"opt
in only where the input is known to carry noise that is safe to drop"* -- turns out
to exclude the case we would have reflexively reached for it on.

## One thing worth knowing about `SyntaxError.Line`

`ConvertTo` hands you one 1MB chunk at a time, so `Line` is relative to the chunk,
not the document. We now carry a running line base; without it an operator chasing
a bad line in a five-million-line dump was sent to line 10083 instead of 30249.

Not a bug in your API -- `Line` is correctly relative to the bytes you were given,
and the doc says so. But every chunked caller has this problem, and the failure is
silent and plausible-looking. If you ever add a bulk parser option to start
numbering at line N, or note the hazard in `SyntaxError`'s doc comment, it would
save the next caller the debugging. Ours is pinned by a test that puts the bad line
past the first chunk deliberately.

## Your two extras, checked

- **Not only N-Quads.** `ParseNTriples` and `ParseNTriplesShared` have no call
  sites in libcat, so we were never exposed.
- **`bibframe.Decode` / `skos.Parse`.** `skos.Parse` is never called and
  `libcodex/skos` is not imported. `bibframe.Decode` has exactly one call site, and
  it is fed our own encoder output -- the N-Triples sniff/fallback cannot be
  reached there with untrusted bytes. Nothing of ours relied on the
  empty-graph-and-nil-error shape you fixed.

We also took your instruction about the bulk parsers' partial results literally:
the quads returned alongside the error are never read anywhere in libcat.

## Adoption

Nothing for you to do. Downstream consumers of libcat need `go get
github.com/freeeve/libcat@v0.134.0` and a rebuild; the observable change is that a
truncated `catalog.nq` or vocabulary dump is refused with a line number instead of
silently producing a smaller catalog.

## Outcome

Closed. One thing was actionable and is done in 4f38c41; doc and test only, so no
release -- it rides the next one carrying code.

### I closed this task once already, against nothing

`taskman file` mints the number and commits the header immediately; the body
arrived in a later commit (8724c6c). I read the file in between, saw a title and
three lines, concluded "bare notice, nothing to verify", and wrote an Outcome
inferred entirely from the title. It happened to guess right about
`SkipMalformed`, which is worse, not better -- a plausible summary of a report I
had not read.

That produced a duplicate 117 when libcat's body recreated the file. The stub
close is reverted and this is the real Outcome. The lesson is narrow and worth
keeping: a cross-repo task file can be committed before it is written. Do not
treat a header-only file as a finished report; check whether the filing commit is
the latest one touching it.

### The chunk-relative line number

The one thing asked for, and it is a real hazard. Verified both halves:

```
bulk, per 1MB chunk   Line=3   (the document's line 8)
streaming decoder     Line=8
```

`Line` is correct for the bytes handed to the parser, which is precisely why the
failure is silent -- a wrong line number in a five-million-line dump is
indistinguishable from a right one. `SyntaxError`'s doc now says so and points at
`NewDecoder`, which reads across an `io.Reader` and numbers continuously. A test
parses the same bad line both ways and asserts 3 and 8, so the two numbering
contracts cannot drift apart unnoticed.

I did not add the line-base option they floated for the bulk parsers. It would
serve one caller who should be streaming instead, and the answer already exists in
the package. If a second chunked caller appears, that judgement was wrong.

### What their measurement settled

The case for strictness in 115 was a hypothetical: a truncated dump from a killed
writer. They tested five real SKOS vocabularies and one failed --
`homosaurus-v4.nt`, **exactly 5,242,880 bytes**, cut mid-IRI. A partial download
sitting in a Downloads folder looking like a vocabulary. Under the lenient parser
it converted cleanly and would have installed a snapshot silently missing every
concept after the cut, then used it to label subject headings.

That is a better argument than the one I made, because it is not hypothetical, and
it lands on the path I never considered: the vocabulary importer, whose doc comment
said "malformed lines are skipped by the lenient parser" -- a description of our
behavior that had quietly become their contract.

`SkipMalformed` is used nowhere in libcat. The adopter who asked for the opt-in
never reached for it, which retires the "error-returning variant plus a deprecation
note" compromise for good.

### Their checks of my two extras

Both came back negative, and both are worth recording because they bound the blast
radius of v0.26.0: `ParseNTriples`/`ParseNTriplesShared` have no libcat call sites,
`skos.Parse` is never called, and `bibframe.Decode`'s single call site is fed their
own encoder output. Nothing of theirs relied on the empty-graph-and-nil-error shape.
They also confirmed the partial results returned alongside the error are read
nowhere, which was the one way the fix could still have been misused.
