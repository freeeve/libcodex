# Real BIBFRAME samples (Library of Congress)

These files are authentic BIBFRAME 2.0 records fetched from the Library of
Congress linked-data service, <https://id.loc.gov/>, by content negotiation:

- `<id>.work.rdf`  — `https://id.loc.gov/resources/works/<id>.rdf`     (RDF/XML)
- `<id>.inst.rdf`  — `https://id.loc.gov/resources/instances/<id>.rdf` (RDF/XML)
- `<id>.work.json` — `https://id.loc.gov/resources/works/<id>.json`    (JSON-LD)

They are produced by LoC's official `marc2bibframe2` converter, so they exercise
the full bf:/bflc: vocabulary, blank nodes, external authority IRIs, `xml:lang`
and typed literals, and admin metadata — structure richer than this library's own
output. `TestLoCStress` parses them and runs the reverse crosswalk against them.

The records span record types and agent kinds:

| id        | type       | note                                   |
|-----------|------------|----------------------------------------|
| 2543127   | Text       | personal author; also has JSON-LD here |
| 17234468  | Text       | corporate author; accented French      |
| 21263493  | StillImage | photograph collection                  |
| 5500000   | Cartography| map; corporate author                  |

LoC asserts these descriptive records are free of known copyright restrictions
(works of the U.S. federal government); see <https://id.loc.gov/>.
