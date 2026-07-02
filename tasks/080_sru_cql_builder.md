# 080 -- sru: typed CQL query builder

Tier 2 -- ergonomics; raw CQL + Quote already works. Split from the 074 deferred
checklist. Ref: `sru/cql.go`; CQL 1.2 (LoC).

## Motivation

The sru package passes CQL through verbatim with only a Quote escaper, while
z3950 grew a typed `Term`/`And`/`Or`/`AndNot` builder. Callers switching between
the two transports (the same catalogs expose both) should be able to build one
query shape and run it anywhere; hand-concatenated CQL is where injection-ish
quoting bugs live.

## Scope

1. Mirror the z3950 builder surface in sru: `Term(index, term)`, `And`, `Or`,
   `Not`, rendering to CQL text (`dc.title = "moby dick" and dc.author =
   "melville"`).
2. Map the common access points to the default context set names the LC/Koha/
   K10plus deployments actually index (dc.title, dc.author/dc.creator, dc.subject,
   dc.isbn/bath.isbn...) -- pick one mapping, document it, and let callers pass a
   raw index name through unchanged.
3. Keep `Quote` and raw-string queries working unchanged.
4. Consider (cheap now, decide then): a shared query interface both packages
   accept, so one query value drives either transport.

## Hazards

- CQL context sets vary by server (dc vs bath vs cql); the mapping is a
  convention, not a truth -- document that plainly and keep the raw escape hatch.
- Do not build a CQL parser; this is a writer only.

## Acceptance

- [ ] Builder renders correct, properly quoted CQL for terms and boolean
      combinations (table-driven).
- [ ] Works live against Koha (dc.title) and yaz-ztest via the interop test.
- [ ] Raw CQL strings keep working unchanged.
