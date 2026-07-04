# SRU: normalize "MARC21-xml" recordSchema to marcxml

## Bug

`sru.normalizeSchema` (sru/sru.go) folds `marcxml`, the MARC21 slim
namespace URI, and `info:srw/schema/1/marcxml*` to the canonical
`"marcxml"` token -- but not `MARC21-xml`, the identifier the Deutsche
Nationalbibliothek stamps on every record (`recordSchema=MARC21-xml` is
also the only schema name DNB's server accepts for MARC21 requests;
`marcxml` gets a `requestedRecordSchema` diagnostic).

Because the label passes through unrecognized, `Reader.Read` skips every
DNB record at its `rec.Schema != "marcxml"` gate and `Record.Decode`
refuses them, even though the payloads are ordinary MARC21 slim XML
(`xmlns="http://www.loc.gov/MARC21/slim"`). Net effect: a Reader over a
DNB search silently yields zero records with no error.

## Repro

```
https://services.dnb.de/sru/dnb?operation=searchRetrieve&version=1.1
  &query=dnb.num%20%3D%20%229783446235755%22
  &maximumRecords=2&recordSchema=MARC21-xml&recordPacking=xml
```

returns `numberOfRecords=3` with `<recordSchema>MARC21-xml</recordSchema>`
records; `NewClient(...)` with `Schema: "MARC21-xml"` + `NewReader` over
that query reads straight to io.EOF.

## Suggested fix

Add the label to the marcxml case in `normalizeSchema`, ideally
case-insensitively -- variants seen in the wild include `MARC21-xml`
(DNB), `marc21`, and `MARC21plus-1-xml` (DNB's extended schema, also slim
XML). At minimum:

```go
case s == "marcxml" || strings.EqualFold(s, "MARC21-xml") || ...
```

Consider whether `Record.Decode`'s schema gate should sniff the payload
namespace as a fallback instead of trusting the label alone.

## Context

Found while seeding DNB as a default copycat SRU target in libcatalog
(libcatalog tasks/087). libcatalog works around it for now by calling
`SearchRetrieve` + `marcxml.Decode` directly in
`backend/copycat/copycat.go` (`sruSearch`); once this is fixed that
workaround can go back to the Reader.
