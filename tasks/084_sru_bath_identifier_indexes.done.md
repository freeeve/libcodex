# SRU CQL builder: map isbn/issn to bath.* instead of dc.*

`sru/cql.go`'s `cqlIndex` maps the builder access points `isbn` -> `dc.isbn`
and `issn` -> `dc.issn`. Dublin Core defines no identifier indexes -- the Bath
profile does (`bath.isbn`, `bath.issn`, `bath.lccn`). LOC's SRU server
(`lx2.loc.gov:210/LCDB`) rejects the dc forms with diagnostic 1/16
"Unsupported index: isbn"; the bath forms return records (verified live
2026-07-04).

Proposed change:

- `"isbn": "bath.isbn"`, `"issn": "bath.issn"` in `cqlIndex`.
- Consider adding `"lccn": "bath.lccn"` while at it -- today `lccn` is not a
  builder access point at all and only works via verbatim dotted pass-through.
- Update the `Term` doc comment and `cql_test.go` pins.

Context: libcatalog worked around this in its `copycat.sruIndex` shim (maps
isbn/issn/lccn -> bath.*) so it is not blocked; the shim can shrink once this
lands and libcatalog bumps.
