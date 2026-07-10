# 113 -- map the 8xx series added entries (830/800/810/811) now that 490 has a bf:Series, and the remaining 76x linking tags including 765

Opened 2026-07-10.

Surfaced by libcat's adoption report (112). They wrote a test using a 765 to prove
their `bf:relationship` guard discriminates a series from a linking entry, and it
passed with the guard deleted. They checked the graph and found the reason: we emit
no `bf:relation` for 765, and none for 830 either.

Verified. `FromRecord` routes only `773, 776, 780, 785` to `appendRelation`, and
`linkRelations` maps only those. Every other 76x-78x tag, and all of 8xx, is
dropped.

## Why it matters more now

Since 110, `bf:relation` is a shared list: 490 series relations and the linking
entries live in it together, told apart only by `bf:relationship`. Anything added
to that list is a new way for a careless consumer to mis-read a series. libcat's
consumer checks; the warning now lives on `Work.Relations`/`Work.Series` and in the
audit doc, and a real-record test (490 + 780) pins it.

## 830 is the interesting one

A traced series (490 ind1=1) *asserts that an 8xx exists* carrying the controlled
form of the series heading. We now emit `mstatus/tr` saying so, and then drop the
830 that it points at. That is a self-inflicted dangling reference: the graph says
"traced" and offers nothing to trace to.

marc2bibframe2 handles 8xx in the same file we already read,
`ConvSpec-Process6-Series.xsl`, template `mode="work8XX"` (from line 363), and it
emits the same `bf:relation` -> `bf:Relation` -> `bf:associatedResource` shape --
but with a `bf:Hub` rather than a `bf:Series` as the associated resource, and with
`bf:seriesEnumeration` again on the relation (see the `$v` loop near line 528, and
the `$z` cancelled-ISSN handling with `mstatus/cancinv` above it). Read that
template properly before designing this; do not infer it from the 490 half.

## Scope

Two separable pieces, probably two releases:

1. **8xx series added entries** (800/810/811/830). Closes the traced dangling
   reference. Needs a decision on `bf:Hub` vs reusing `bf:Series`, and on whether a
   traced 490 and its 830 collapse into one relation or stay two.
2. **The remaining 76x tags** (760/762/765/767/770/772/774/775/777/786/787). Purely
   additive: extend `linkRelations` and the `FromRecord` case. 765 is
   `translationOf`, 767 `translation`, 775 `otherEdition`, and so on -- the
   relationship vocabulary already names them all.

Piece 2 is nearly mechanical and could be done without a decision. Piece 1 is not.

## Correction: piece 2 is blocked, and was wrong on its face

Written before reading `ConvSpec-760-788-Links.xsl`. Piece 2's premise -- "the
relationship vocabulary already names them all" -- is false, and so is the codebase
it would have extended. **Every relationship IRI `linkRelations` emits today 404s at
id.loc.gov**; LC's terms are lowercase and several are differently named
(`continues` -> `continuationof`, `formedByUnionOf` -> `mergerof`). Adopting the
real ones collapses the 780/785 ind2 round trip, so it is not a rename.

Filed as **116**, which blocks this task. Extending the table first would cement
eleven more IRIs that do not exist. `765` is `translationof`, not `translationOf`.

**Unblocked as of v0.27.0 (116 done).** The relationship vocabulary is now LC's
correct one, and decode reads the source field from a verbatim marcKey note rather
than reversing the term. So piece 2 really is additive now: add the other LC terms
to `relationCodeFor`, route the tags in `FromRecord`, and grow `isLinkingTag`
(the one decode-side list). Piece 1 (8xx) still needs the `bf:Hub`-vs-`bf:Series`
decision and is designed against `ConvSpec-Process6-Series.xsl` `mode="work8XX"`.

This is also why piece 1 must be designed against the XSLT rather than extrapolated
from the 490 half: the one thing 110 got right, it got right by reading the source.

Related: 073 (the original linking-entries checklist), 110 (the 490 series
relation), 112 (libcat's report).
