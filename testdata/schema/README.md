# Vendored validation schemas

Official XML schemas used by `TestXMLSchemaConformance` to validate the library's
XML exporters. Import `schemaLocation` references have been rewritten to the local
filenames in this directory so validation runs offline; the schema contents are
otherwise unmodified.

| File | Source | Used by |
|------|--------|---------|
| `MARC21slim.xsd` | LoC — https://www.loc.gov/standards/marcxml/schema/MARC21slim.xsd | `marcxml` |
| `mods-3-8.xsd` | LoC — https://www.loc.gov/standards/mods/v3/mods-3-8.xsd | `mods` |
| `xlink.xsd` | LoC METS XLink — https://www.loc.gov/standards/xlink/xlink.xsd | `mods` (import) |
| `oai_dc.xsd` | Open Archives — http://www.openarchives.org/OAI/2.0/oai_dc.xsd | `dublincore` |
| `simpledc.xsd` | DCMI — http://dublincore.org/schemas/xmls/simpledc20021212.xsd | `dublincore` (import) |
| `xml.xsd` | W3C — http://www.w3.org/2001/xml.xsd | shared import |

These are published standards (LoC works are public domain; W3C, DCMI and Open
Archives schemas are freely redistributable). They are test fixtures only and are
not part of the importable library.
