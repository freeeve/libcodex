# 116 -- every bf:relationship IRI we emit for 76x-78x links 404s at id.loc.gov: LC's terms are lowercase and differently named, and adopting them collapses the 780/785 ind2 round-trip

Opened 2026-07-10.

Found while scoping 113's "nearly mechanical" half -- extending `linkRelations` to
the remaining 76x tags. It is not mechanical, and the existing entries are wrong.

## The finding

`linkRelations` names 18 relationship codes. We emit them as
`http://id.loc.gov/vocabulary/relationship/<code>`. **Every one of them 404s.**

```
$ curl -sLo /dev/null -w '%{http_code}' https://id.loc.gov/vocabulary/relationship/continues.json
404
    /otherPhysicalFormat.json  404      /partOf.json           404
    /absorbed.json             404      /formedByUnionOf.json  404
    /supersedes.json           404      /continuedBy.json      404
```

LC's terms are lowercase and, in several cases, differently named. All of these
resolve 200:

```
partof  otherphysicalformat  continuationof  continuedby  precededby  succeededby
mergerof  absorptionof  absorbedby  translationof  translatedas  supplement
supplementto  part  otheredition  issuedwith  datasource  relatedwork  series
subseries  splitinto  mergedtoform  separatedfrom  continuedinpart  continuedinpartby
```

Read from `xsl/ConvSpec-760-788-Links.xsl` (lines 54-86), then each verified against
id.loc.gov rather than trusted from the XSLT. The camelCase codes were invented
here; nothing in LC's vocabulary carries them.

The `relationship/series` IRI added for 490 in v0.25.0 is **correct** -- it was
copied out of `ConvSpec-Process6-Series.xsl` rather than guessed. That difference in
provenance is the whole story: 110 was read from the source, this was not.

## Why this is not a rename

LC's map is not injective over (tag, ind2). Ours is, deliberately: `linkRelations`
is "the single source of truth for both crosswalk directions", and decode recovers
(tag, ind2) from the code. LC collapses:

| MARC                 | LC relationship     |
|----------------------|---------------------|
| 780 ind2=0           | `continuationof`    |
| 780 ind2=1           | `continuedinpart`   |
| 780 ind2=4           | `mergerof`          |
| 780 ind2=**5 or 6**  | `absorptionof`      |
| 780 ind2=7           | `separatedfrom`     |
| 780 ind2=**2, 3, 8** | `precededby`        |
| 785 ind2=**0 or 8**  | `continuedby`       |
| 785 ind2=1           | `continuedinpartby` |
| 785 ind2=**4 or 5**  | `absorbedby`        |
| 785 ind2=6           | `splitinto`         |
| 785 ind2=7           | `mergedtoform`      |
| 785 ind2=**2, 3**    | `succeededby`       |

Adopting LC's vocabulary makes 780 ind2 5 and 6 indistinguishable on decode, and
likewise 785 4/5, 785 0/8, 780 2/3/8, 785 2/3. Today those round-trip exactly.
`bibframe/lossgate_test.go` holds 773/776/780/785, and its kitchen-sink record
carries `780 ind2=0`.

So the decision is: **dereferenceable, LC-correct IRIs, or an exact 780/785 ind2
round trip.** Today we have the second, bought with seven IRIs that do not exist.

## Options

1. **Adopt LC's IRIs, accept the collapse.** Decode picks a canonical ind2 per code
   (5 for `absorptionof`, 4 for `absorbedby`, 0 for `continuedby`, 2 for
   `precededby`). Honest to BIBFRAME, lossy against MARC, and the loss gate has to
   learn about it.
2. **Adopt LC's IRIs and keep the field verbatim in an internal `bf:Note`**, exactly
   as we already do for 040 (`mnotetype/internal`, `rdfs:label` in marcKey form).
   Lossless *and* correct, at one extra node per linking entry. It is LC's own
   pattern for the same problem, and the precedent is already in this codebase.
3. **Keep the invented IRIs.** Cheapest, and wrong: a consumer resolving
   `bf:relationship` gets a 404, and a consumer matching against LC's vocabulary
   never matches us at all.

Option 2 is what I would do. It costs more than 1, but it is the only one that does
not trade a real property away, and the marcKey note pattern is already carried,
tested and understood here.

Whatever is chosen, 113's remaining-76x-tags half should land in the same change --
extending the table first would cement eleven more IRIs that 404.

## Blast radius

Breaking for any consumer matching on `bf:relationship`. libcat matches only
`relationship/series`, which is correct already and unaffected. Their 76x reader, if
they ever grow one, would want LC's IRIs.

Related: 113 (which this blocks), 110 (the 490 series relation, whose IRI is right).

Leaving pending for Eve.
