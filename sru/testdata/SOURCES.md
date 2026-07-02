# SRU test fixtures

These are hand-authored SRU 1.2 `searchRetrieveResponse` documents modeled on the
shape returned by the Library of Congress SRU service
(`http://lx2.loc.gov:210/lcdb`, `zs:` = `http://www.loc.gov/zing/srw/`). They are
synthetic -- the embedded MARCXML records carry illustrative bibliographic data,
not verbatim LC catalog records -- and are captured here so tests never touch the
network.

- `search_page1.xml` -- two MARCXML records (`recordPacking="xml"`, schema given as
  the `info:srw/schema/1/marcxml-v1.1` URI), `numberOfRecords=3`,
  `nextRecordPosition=3` (a further page follows).
- `search_page2.xml` -- the third record, no `nextRecordPosition` (result set
  exhausted). Together with page 1 this exercises the paging `Reader`.
- `diagnostic.xml` -- a zero-record response carrying an SRU `<diagnostic>`,
  exercising `DiagnosticsError`.
- `string_packing.xml` -- one record with `recordPacking="string"` (the MARCXML is
  XML-escaped text rather than inline markup), exercising unescaping.
