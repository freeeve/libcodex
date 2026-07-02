# 069 -- bibframe: AdminMetadata completeness (003, 040 $e node, 005 datatype)

Tier 2. From the 059 m2b audit, admin metadata area.
Ref: `docs/bibframe_m2b_audit.md` section 6; m2b `ConvSpec-001-007.xsl`,
`ConvSpec-010-048.xsl` (040 mode adminmetadata).

## Motivation

Our AdminMetadata is close on node shape but thin:

- 003 is not read, so the `bf:Local` (001) identifier has no `bf:assigner`
  (DLC IRI when 003=DLC/empty, else `bf:code`=003).
- 040 $e emitted as a plain literal and only the first $e; m2b emits a
  `bf:DescriptionConventions` node (vocab IRI + code) per $e.
- 005 emitted as a bare literal; m2b tags it `rdf:datatype=xsd:dateTime`.
- 040 $b (`bf:descriptionLanguage`) and 042 (`bf:descriptionAuthentication`)
  unhandled.

## Scope

1. Read 003; attach `bf:assigner` to the 001 `bf:Local` (DLC IRI or `bf:code`).
2. 040 $e -> a `bf:DescriptionConventions` node (IRI + code) per $e; loop all $e.
3. Tag 005 (`bf:changeDate`) with `xsd:dateTime` -- requires the emitter/serializer
   to support a typed literal (check `shape_render.go`/`vocab.go` datatype support;
   this may be the first typed literal in the crosswalk).
4. Optional: 040 $b -> `bf:descriptionLanguage`; 042 -> `bf:descriptionAuthentication`.

## Hazards

- Sample has 001, 005, and 040 $e -> emitting the assigner/typed-date/node form
  WILL change goldens; regenerate and review.
- The `xsd:dateTime` datatype needs literal-with-datatype plumbing in the sinks
  (graph/xml/json). Confirm it exists or add it minimally; this is the riskiest part.
- Keep `bf:generationProcess` = libcodex (intentional, identifies our converter).

## Acceptance

- [x] 001 `bf:Local` carries `bf:assigner` from 003.
- [x] 040 $e -> DescriptionConventions node(s); 005 typed `xsd:dateTime`.
- [x] Goldens regenerated + reviewed; round-trip + fuzz green.

## Result

`AdminMetadata` gained `ControlOrg` (003) and `DescriptionConventions` became
`[]string` (all 040 $e). Forward reads 003 and every $e. Emit:
- `emitAssigner` attaches a `bf:assigner` `bf:Agent` to the 001 `bf:Local`, with an
  organizations-vocabulary IRI (`orgIRIVal`, lowercased/validated) and the raw code
  as `bf:code`; emitted only when 003 is present (no LoC-centric DLC default).
- each 040 $e -> a `bf:DescriptionConventions` node (RDA conventions IRI + `bf:code`).
- 005 `bf:changeDate` is an `xsd:dateTime` typed literal.

New sink capability: `litTyped(pred, text, datatype)` on the sink interface and all
three renderers -- graph (`rdf.NewLiteral` datatype), RDF/XML
(`rdf:datatype="..."`), JSON-LD (`{"@value":..,"@type":..}`). This is the
crosswalk's first typed literal; both parsers already round-trip datatypes.
Re-added the `qcAgent` class (retired in 066) for the assigner.

No golden change: the sample carries no 003/005/040, and AdminMetadata is
forward-only provenance (`normalize` nils it, the reverse never reconstructs it),
so nothing round-trips through it. Tests: `admin_completeness_test.go` (003 assigner
IRI + code, two 040 $e conventions nodes, typed 005 in RDF/XML + JSON-LD with the
datatype surviving a parse) and the updated `admin_test.go`. Suite + FuzzFromMARC +
FuzzDecode green.

Deferred (documented): 040 $b descriptionLanguage, 042 descriptionAuthentication.
