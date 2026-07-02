# 067 -- bibframe: 336 content type, RDA media/carrier IRIs, 300 extent split

Tier 2. From the 059 m2b audit, physical/RDA area.
Ref: `docs/bibframe_m2b_audit.md` section 5; m2b `ConvSpec-3XX.xsl`, `ConvSpec-LDR.xsl`.

## Motivation

- 336 (content type) is not handled at all -- we emit no `bf:content`. m2b emits
  `bf:content -> bf:Content` with the RDA `contentTypes` IRI from $b (or $a-label
  map) and always backfills a `bf:content` IRI from leader/06 when 336 is absent,
  so every m2b Work carries a content IRI and ours carry none.
- 337/338 emit label-only `bf:Media`/`bf:Carrier`; m2b builds the RDA
  `mediaTypes`/`carriers` IRI from $b (or $a-map) and allows repeats.
- 300 conflates $a/$b/$c/$e into one `bf:Extent` label; m2b keeps $a(+$f/$g) as the
  extent and routes $b/$c/$e to separate notes/dimensions.

## Scope

1. Read 336 $a/$b -> `bf:content` IRI on the Work, with a leader/06 fallback IRI.
2. Build 337/338 IRIs from $b (or $a-label map); allow repeated media/carrier.
3. Split 300: extent label from $a(+$f/$g); route $b/$c/$e out of the label
   (dimensions/notes) -- or, minimally, stop swallowing $c dimensions into extent.

## Hazards

- Sample has 337/338 (media/carrier text) and 300 -> all three goldens will move.
  Regenerate deliberately and review the RDA IRIs.
- Needs the RDA content/media/carrier code->IRI maps; use small static tables
  (mirror the language-code IRI approach). Scope $a-label lookups carefully.
- Leader/06 content fallback interacts with `workClass`; keep the Work rdf:type
  subclass and add `bf:content` as a separate property.

## Acceptance

- [x] Work carries `bf:content` (from 336 or leader/06 fallback).
- [x] 337/338 carry RDA vocab IRIs and are repeatable.
- [x] 300 no longer conflates dimensions/notes into the extent label.
- [x] Goldens regenerated + reviewed; round-trip + fuzz green.

## Result

Added `Work.Content` (RDA content code), `Instance.Dimensions []string`, and made
`Instance.Media`/`Carrier` `[]RDATerm` ({Code,Label}). Forward: 336 $b -> Content,
falling back to `content06(leader/06)` so every Work carries a content term; 337/338
-> `rdaTerm` ($b code, $a label), repeatable; 300 extent from $a/$b/$f/$g with $c
routed to Dimensions.

Emit: `bf:content` -> `bf:Content` IRI in the RDA contentTypes vocabulary
(`rdaIRIVal`), `bf:media`/`bf:carrier` as lists of IRI-or-blank nodes (`emitRDA`),
and `bf:dimensions` literals. Reverse: `contentField` -> 336 $b + $2 rdacontent,
`rdaFields` -> 337/338 $a/$b/$2 (code from the vocabulary IRI local name,
`rdaValue`), `physicalFields` -> 300 with $a per extent and $c dimensions on the
first. `extent` now excludes $c/$e.

Goldens: the sample's leader/06 'a' adds `bf:content` contentTypes/txt on the Work;
300 `$a 301 pages $c 22 cm` splits into `bf:extent "301 pages"` +
`bf:dimensions "22 cm"`. Regenerated both serializations. Tests:
`rda_content_test.go` (336 content + leader fallback, repeatable 337/338 RDA codes,
extent/dimension split, RDF/XML + JSON-LD round-trip). Suite + FuzzFromMARC +
FuzzDecode green.

Deferred (documented): 300 $e accompanying material, $a-label -> code lookup when a
33X carries no $b, EDTF date datatypes.
