# 118 -- ConvertTo streams through rdf.NewDecoder now (libcat v0.136.1); the line base is gone -- two notes on Decoder

Filed from libcat on 2026-07-10 (cross-repo ask).

Closing libcat tasks/320, which you filed. Nothing is asked of you either, but two
things about `Decoder` are worth your attention -- one is a doc gap that your own
snippet walks into, and one is a tag you may have meant to push.

## Took the streaming option

`vocabsrc.ConvertTo` decodes statement-at-a-time through `rdf.NewDecoder` now.
`lineBase`, the 1MB chunk, and the `ReadSlice`/`ErrBufferFull` read loop are gone.
Shipped in **libcat v0.136.1**. It was the only chunked caller in the tree --
`subsetFromDump` hands `ParseNQuads` the whole dump, so its `Line` was always the
document's.

You framed this as tidying. It is also a memory win. Converting 200k statements:

| | chunked | streaming |
|---|---|---|
| wall clock | 147.6 ms | 151.9 ms (+2.9%) |
| allocated | 138.3 MB | 86.5 MB (**-37%**) |
| allocations | 600,715 | 800,585 (+33%) |

The extra allocations are `ReadString`'s string per line. The 52MB saved is the
chunk plus the chunk's whole parsed `[]Quad`, neither of which exists any more.
Peak memory is now one statement plus the concept set. I would take that trade
again at twice the wall clock.

## Note 1: `Decoder` reads lines with an unbounded `ReadString`

`decoder.go:73` is `br.ReadString('\n')`. A body with no newline in it grows one
"line" until the process dies. That is fine as a library default -- but it means
**the snippet in your 320 is not a safe drop-in for `ConvertTo`**, and the reason
is invisible from the doc.

`ConvertTo` has two defensive ceilings (decompressed bytes, and bytes since the
last newline). The old read loop enforced both itself, precisely because it did its
own `ReadSlice`. Handing the upload straight to `NewDecoder` silently drops the
line ceiling. I moved both ceilings *ahead* of the decoder into a `cappedReader`,
which is the right shape anyway -- ceilings are about bytes, parsing is about
syntax.

Worth one sentence on `NewDecoder`: *the line-based formats accumulate a line
without bound; wrap r if the input is untrusted.* That is the whole fix. I am not
asking for a `MaxLine` option -- your argument against a second knob in 320 applies
here too, and a wrapping reader is four lines.

### The ordering rule that fell out of it

Worth knowing because it makes a correct parser report the wrong cause:

> A breached ceiling cuts the reader off **mid-line**. `Decoder` then does exactly
> what it should -- returns a `*SyntaxError` about the truncated tail. So a 5GB
> upload got blamed on *"line 6676 is truncated or corrupt"* instead of on the size
> cap that truncated it.

`ConvertTo` now consults the reader's sticky error before classifying any decode
failure, so the ceiling outranks the syntax error it caused. Mutation-checked:
stubbing that check reproduces the message above; stubbing the line ceiling makes
`Decoder` allocate the entire 6MB line into `SyntaxError.Text`, which is Note 1
demonstrated.

Nothing for you to change. `Decoder` reporting what it can see is correct; only the
caller knows the truncation was self-inflicted.

## Note 2: the doc change in 320 is committed but not tagged

`4f38c41 docs(rdf): warn that SyntaxError.Line is relative to the bytes handed to
the parser` sits after `v0.26.0` on main. `go list -m -versions` and your own tags
both top out at v0.26.0, so no adopter can see that warning yet.

Nothing here depends on it -- the decoder's continuous line numbering is v0.26.0
behavior, and I verified it directly rather than trusting the doc:

```
streaming decoder: 7 quads, SyntaxError.Line=8   (the document's line)
bulk, per chunk:   SyntaxError.Line=2            (chunk-relative)
```

Flagging it only so the warning does not get assumed shipped. If it lands in the
same release as the `NewDecoder` sentence from Note 1, that release note writes
itself.

## Kept the test

`TestAMalformedLineIsReportedAtItsLineInTheWholeDump` stays, as you suggested. Its
filler now guards a different mistake than the one it was written for -- a
regression to chunked parsing rather than a missing base -- so its comment says
which. It fails identically under either.

Also used `rdf.NQuads` rather than the `rdf.NTriples` in your snippet: the importer
takes both, and the NQuads decoder parses a three-term line fine. The graph term is
overwritten with `authority:<scheme>` regardless.

## Adoption

None. Reported for the record, and because Note 1 is a doc gap that the next
streaming adopter will also fall into.
