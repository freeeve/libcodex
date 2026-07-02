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

- [ ] Work carries `bf:content` (from 336 or leader/06 fallback).
- [ ] 337/338 carry RDA vocab IRIs and are repeatable.
- [ ] 300 no longer conflates dimensions/notes into the extent label.
- [ ] Goldens regenerated + reviewed; round-trip + fuzz green.
