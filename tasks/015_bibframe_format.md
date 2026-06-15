# 015 — BIBFRAME mapping (stretch, post-v0.1.0)

## Goal
Add BIBFRAME 2.0 support: convert between `codex.Record` (MARC) and BIBFRAME
RDF (Work / Instance / Item resources).

## Why this is different from the other formats
`iso2709`, `marcxml`, `marcjson` and `mrk` are all **serializations of the same
MARC model**, so they implement `codex.RecordReader` / `codex.RecordWriter`
directly over `*codex.Record`. BIBFRAME is **a different data model** — an RDF
graph of linked resources, not a flat leader+fields record. It does NOT fit the
same interface; it needs a *mapping/transform layer*, and the MARC↔BIBFRAME
mapping is inherently lossy and opinionated (cf. the LoC `marc2bibframe2`
converter and the reverse `bibframe2marc`).

Because of that, this is scoped as a stretch item AFTER the v0.1.0 release of the
four MARC serializations — not a blocker for publishing.

## Open questions to resolve before starting
- Direction: MARC→BIBFRAME only, or round-trip both ways? (Round-trip is hard and
  lossy.)
- RDF serialization target: RDF/XML, Turtle, JSON-LD, N-Triples? (Pulls in an RDF
  story — likely the first place the "stdlib-only" constraint is challenged.)
- Dependency policy: a pure-stdlib JSON-LD/Turtle emitter vs. an external RDF
  library. Decide explicitly given the project's "prefer fewer dependencies" rule.
- Scope of the vocabulary mapping (which MARC fields → which BIBFRAME classes
  and properties); whether to mirror LoC's conversion specs or a subset.

## Related non-MARC models (also out of the same-model pattern)
If BIBFRAME lands, MODS and Dublin Core are natural neighbors — same "needs a
mapping layer, not just a codec" shape. Track separately if wanted.

## Acceptance
- Decided direction + RDF serialization + dependency policy documented.
- A `bibframe` package converting `*codex.Record` → BIBFRAME resources in the
  chosen serialization, with a documented, tested mapping for the common fields.

## Depends on
- 002 (model), and ideally 006-008 so the MARC side is settled. Not a v0.1.0 blocker.
