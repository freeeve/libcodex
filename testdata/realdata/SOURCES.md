# Real records (Library of Congress)

Authentic bibliographic records fetched from the LoC LCCN permalink service:

- `<lccn>.marcxml` — `https://lccn.loc.gov/<lccn>/marcxml` (the source record)
- `<lccn>.mods`    — `https://lccn.loc.gov/<lccn>/mods`    (LoC's own MODS crosswalk)

`TestRealData` reads each MARCXML record and exercises the whole library on it:
the four codecs (ISO 2709, MARCXML, MARC-in-JSON, MARCMaker) round-trip it
losslessly; every exporter (MODS, Dublin Core, RIS, BibTeX, schema.org, and
BIBFRAME in RDF/XML, JSON-LD, Turtle and N-Triples) produces well-formed output;
the BIBFRAME serializations decode back to a titled record; and our MODS titles
are cross-checked against LoC's own MODS.

The set spans material types — books (leader/06 `a`), notated music (`c`) and a
map (`e`) — and Unicode transcription (e.g. romanized Punjabi with diacritics).
These descriptive records are works of the U.S. federal government.
