# 078 -- z3950: bib-1 structure/truncation/relation attributes

Tier 1 -- CRITICAL for real-library use. Split from the 075 deferred checklist.
Ref: bib-1 attribute set (types 2-6); `z3950/query.go`.

## Motivation

Queries currently carry only a use attribute (type 1) and rely on server
defaults for everything else. Zebra and yaz default sensibly, but the strict
servers common in libraries (Voyager, Aleph, Symphony) mis-handle or reject
multi-word terms without an explicit structure (word vs phrase), and catalogers'
bread-and-butter searches -- title begins-with, truncated forms -- need the
truncation and position attributes. Without these, everyday searches quietly
return wrong result sets on a large share of real targets.

## Scope

1. Auto-structure: a term containing whitespace gets structure=phrase (4=1),
   a single word structure=word (4=2), unless overridden.
2. Explicit options on the builder, keeping the current API compatible:
   `Term(index, term).Phrase()` / `.Word()` / `.Truncated()` (5=1 right
   truncation) / `.Exact()` (relation 2=3, position 3=1 first-in-field,
   completeness 6=3) -- a small fluent set, not full bib-1 generality.
3. A `*` suffix on the term maps to right-truncation (5=1) with the `*`
   stripped, matching common user expectation; literal asterisks escapable.
4. Keep attribute order deterministic (type ascending) for testability.

## Hazards

- Attribute combinations are server-quirk territory; default to emitting only
  what is asked plus the automatic structure, nothing more.
- Do not break the current single-attribute encoding for existing callers; the
  fake-server tests must assert exact AttributeList contents.
- yaz-ztest ignores most attributes, so interop proves well-formedness only;
  correctness of the mapping is asserted hermetically.

## Acceptance

- [x] Multi-word terms carry structure=phrase automatically; single words
      structure=word.
- [x] Truncation, exact and word/phrase overrides encode the documented
      attribute pairs (hermetic AttributeList assertions).
- [x] yaz-ztest interop and live Koha/LC spot checks still return expected hit
      counts for single-word queries.

## Result

- `Query` gains structure/truncation/exact state with fluent refinements
  (`Phrase`/`Word`/`Truncated`/`Exact`); `Term` and the boolean combinators are
  unchanged for existing callers.
- Auto-structure: whitespace in the term -> structure=phrase (4=1), else
  structure=word (4=2), always emitted -- the gap that made multi-word terms
  brittle on strict servers.
- Trailing `*` strips to a right-truncated stem (5=1); `\*` escapes a literal
  asterisk; a bare `*` errors (empty stem). `.Exact()` adds relation=equal
  (2=3), position=first-in-field (3=1), completeness=complete-field (6=3).
- AttributeElements emit in ascending attribute-type order; `query_test.go`
  pins the exact (type,value) list per builder form plus the boolean branch
  shape.
- Live: Koha Zebra -- phrase "fire island" 2 hits (matching its SRU dc.title
  count), `fire*` 33, author+any boolean 2; LC production -- 318 / 9672 / 17.
  yaz-ztest interop green with the new attributes.

### Deferred

- [ ] Proximity operator; relation attributes beyond equal (ranges, stem).
- [ ] Left/both truncation (5=2/3); regexp truncation (5=102/103).
