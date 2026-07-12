# libcodex

A dependency-free Go module for reading, writing and converting
[MARC 21](https://www.loc.gov/marc/) bibliographic records across four
serializations: binary [ISO 2709](https://en.wikipedia.org/wiki/ISO_2709)
(`.mrc`), [MARCXML](https://www.loc.gov/standards/marcxml/), MARC-in-JSON, and
the [MARCMaker](https://www.loc.gov/marc/makrbrkr.html) mnemonic text format
(`.mrk`).

- Module path: `github.com/freeeve/libcodex`
- Core package `codex`: the format-agnostic MARC model and the codec interfaces
- One subpackage per serialization, all sharing the same surface
- **Standard library only — no third-party dependencies**
- Go 1.25+

The model is domain-agnostic: it exposes leaders, fields, subfields and
indicators, and leaves interpretation of specific tags to the caller.

## Install

```sh
go get github.com/freeeve/libcodex
```

```go
import (
	"github.com/freeeve/libcodex"          // package codex — the model + interfaces
	"github.com/freeeve/libcodex/iso2709"  // a format codec (one of four)
)
```

(The core import path ends in `libcodex`; the package identifier is `codex`.)

## Architecture

`codex` holds the shared in-memory model — `Record`, `Field`, `Subfield`,
`Leader` — and two interfaces:

```go
type RecordReader interface{ Read() (*Record, error) }  // io.EOF at end of stream
type RecordWriter interface{ Write(*Record) error }
```

Each serialization is a subpackage that maps that one model onto a wire format,
so the same record round-trips through any of them and converts between any two.

| Package    | Format                         | Notes                                         |
|------------|--------------------------------|-----------------------------------------------|
| `iso2709`  | binary ISO 2709 (`.mrc`)       | the interchange format; fastest path          |
| `marcxml`  | MARCXML (LoC slim schema)      | `Writer` wraps a `<collection>`; needs `Close` |
| `marcjson` | MARC-in-JSON (pymarc layout)   | `Writer` emits a JSON array; needs `Close`    |
| `mrk`      | MARCMaker mnemonic text        | human-readable, line-based                     |

Every codec exposes the same surface:

```
NewReader(io.Reader) *Reader        Decode([]byte) (*codex.Record, …)
NewWriter(io.Writer) *Writer        Encode(*codex.Record) ([]byte, error)
ReadFile(path) ([]*codex.Record, …) WriteFile(path, []*codex.Record) error
(*Reader).All() iter.Seq2[*codex.Record, error]
```

## Reading

Readers stream one record at a time. The API is identical for every format —
just pick the package:

```go
f, _ := os.Open("catalog.mrc")
defer f.Close()

r := iso2709.NewReader(f) // or marcxml.NewReader, marcjson.NewReader, mrk.NewReader
for rec, err := range r.All() {
	if err != nil {
		log.Println("bad record:", err) // iteration stops at the first error
		break
	}
	title := rec.SubfieldValue("245", 'a')
	author := rec.SubfieldValue("100", 'a')
	fmt.Printf("%s — %s (id %s)\n", title, author, rec.ControlField("001"))
	for _, subject := range rec.SubfieldValues("650", 'a') {
		fmt.Println("  subject:", subject)
	}
}
```

Convenience helpers per format: `iso2709.ReadFile(path)` and
`iso2709.Decode(raw)` (the binary `Decode` also returns a `lossy bool` — see
[Character encoding](#character-encoding)).

## Writing

Build a record from scratch and serialize it. Builders are chainable:

```go
rec := codex.NewRecord().
	AddField(codex.NewControlField("001", "qll-00042")).
	AddField(codex.NewDataField("245", '1', '0',
		codex.NewSubfield('a', "Stone butch blues :"),
		codex.NewSubfield('b', "a novel"),
		codex.NewSubfield('c', "Leslie Feinberg."))).
	AddField(codex.NewDataField("650", ' ', '0',
		codex.NewSubfield('a', "Lesbians")))

raw, err := iso2709.Encode(rec) // or marcxml.Encode, marcjson.Encode, mrk.Encode
```

The `marcxml` and `marcjson` writers buffer a wrapper element, so call `Close`
when finished:

```go
w := marcxml.NewWriter(out)
for _, rec := range recs {
	w.Write(rec)
}
w.Close() // emits </collection>
```

## Converting between formats

Because the codecs share interfaces, `codex.Convert` pipes any reader into any
writer — that is the whole conversion engine:

```go
// Binary MARC on stdin → MARCXML on stdout.
w := marcxml.NewWriter(os.Stdout)
if err := codex.Convert(iso2709.NewReader(os.Stdin), w); err != nil {
	log.Fatal(err)
}
w.Close()
```

Any of the 4 × 4 source/target combinations works and preserves the model.

## Command-line tool (`libcodex`)

The same codecs are wrapped in a small CLI for shell use — inspecting,
transcoding, validating and profiling records without writing Go:

```sh
go install github.com/freeeve/libcodex/cmd/libcodex@latest
```

```sh
libcodex cat       [-i fmt] [-t tags] [-n N] [--json] [file...]   readable dump
libcodex convert   [-i fmt] -o fmt [file...]                      transcode
libcodex validate  [-i fmt] [file...]                             structural check
libcodex stats     [-i fmt] [file...]                             field/leader report
libcodex skos      [-o fmt] [-n N] [file...]                      SKOS vocab view / to MARC authority
```

The input format is auto-detected from the leading bytes when `-i` is omitted,
and with no file arguments each subcommand reads stdin, so commands compose in a
pipe:

```sh
libcodex cat -t 084,650 catalog.mrc          # dump only classification/subject fields
libcodex convert -o bibframe catalog.mrc     # MARC → BIBFRAME RDF/XML on stdout
libcodex convert -o marcjson catalog.mrc | libcodex stats   # transcode, then profile
```

`convert` targets every registered output format (the four MARC serializations
plus `bibframe`, `mods`, `dublincore`, `schemaorg`, `ris` and `bibtex`). See
[`cmd/libcodex/README.md`](cmd/libcodex/README.md) for the full format table and
per-subcommand detail.

## Export converters

Beyond the four round-trip serializations, the library exports to formats that
use a *different*, lossy model — a documented MARC→target crosswalk, not a codec
(`bibframe` also reads its graph back, see below).
Their `Writer`s implement `codex.RecordWriter`, so they also work with
`codex.Convert`:

| Package      | Target                                      |
|--------------|---------------------------------------------|
| `mods`       | MODS (LoC XML, near-lossless)               |
| `dublincore` | Dublin Core — `oai_dc` XML and DC-JSON      |
| `citation`   | RIS and BibTeX (reference managers)         |
| `bibframe`   | BIBFRAME 2.0 — RDF/XML, JSON-LD, Turtle, N-Triples (reads + writes) |
| `schemaorg`  | schema.org JSON-LD (`Book`, with a11y)      |

```go
// Binary MARC → BibTeX for a reference manager.
codex.Convert(iso2709.NewReader(in), citation.NewBibTeXWriter(out))
// Or a single record: b, _ := mods.Encode(rec) / dublincore.Encode(rec) / citation.RIS(rec)
```

`bibframe` is the one that changes data *model*, not just serialization: each MARC
record becomes a small RDF graph of a `bf:Work` (intellectual content) and a
`bf:Instance` (a publication of it), linked by `bf:instanceOf`/`bf:hasInstance`.
It reads and writes **four RDF serializations** — RDF/XML, JSON-LD, Turtle and
N-Triples — all hand-written with the standard library, including the RDF parsers
(no RDF dependency):

```go
// Binary MARC → BIBFRAME, in any serialization.
b, _ := bibframe.Encode(rec)         // RDF/XML (canonical)
b, _ := bibframe.EncodeJSONLD(rec)   // JSON-LD
b, _ := bibframe.EncodeTurtle(rec)   // Turtle
b, _ := bibframe.EncodeNTriples(rec) // N-Triples
// Streaming collections: bibframe.NewWriter / NewJSONLDWriter / NewTurtleWriter /
// NewNTriplesWriter (the XML and JSON-LD writers wrap a container, so close them).
```

Unlike the other converters, `bibframe` also **reads** — a dependency-free RDF
parser (all four serializations, autodetected) plus a BIBFRAME→MARC 21 reverse
crosswalk turn a BIBFRAME graph back into records. It reads the vocabulary this
library emits and the common shape of LoC `marc2bibframe2` output:

```go
recs, _ := bibframe.Decode(b)       // []*codex.Record; the format is detected
recs, _ := bibframe.ReadFile(path)
// As a Convert source: codex.Convert(bibframe.NewReader(in), iso2709.NewWriter(out))
```

BIBFRAME is a lossier model than MARC, so a decoded record carries the
crosswalked fields rather than the original byte-for-byte; re-encoding it yields
an equivalent graph. The other four converters (`mods`, `dublincore`, `citation`,
`schemaorg`) are export-only and carry only the common fields; each package
documents its crosswalk.

## RDF toolkit (`rdf`)

The RDF machinery behind BIBFRAME is a standalone, dependency-free package you can
use directly: the triple model (`Term`, `Triple`, `Graph`) and parsers and
serializers for RDF/XML, JSON-LD, Turtle and N-Triples. In benchmarks it parses
several times faster than the common third-party Go RDF libraries.

```go
g, _ := rdf.ParseNTriples(data)          // also ParseTurtle / ParseRDFXML / ParseJSONLD
out := g.Turtle(map[string]string{...})  // serialize: NTriples() / Turtle(prefixes)
```

For inputs too large to hold in memory — multi-gigabyte dumps like the Library of
Congress authority files — a streaming `Decoder` reads **N-Triples, N-Quads,
RDF/XML or Turtle** from an `io.Reader` one triple at a time in **constant
memory**:

```go
d := rdf.NewDecoder(file, rdf.NTriples) // or rdf.RDFXML / rdf.Turtle / rdf.NQuads
for tr := range d.All() {               // or d.Decode() (rdf.Triple, error)
    // process tr; the whole graph is never materialized
}
```

It streams the 3.3 GB LCSH N-Triples file (23M triples) at ~800 MB/s with a live
heap of a few megabytes, and holds RDF/XML and Turtle to a few MB regardless of
file size. (JSON-LD is whole-document only — its tree must be materialized.)

## Fetching records over SRU (`sru`)

[SRU](https://www.loc.gov/standards/sru/) (Search/Retrieve via URL) is the HTTP
search protocol library catalogs expose for copy-cataloging and discovery — the
web successor to Z39.50. The `sru` client runs a `searchRetrieve` over `net/http`,
parses the response, and hands the embedded records to the decoders above. Its
`Reader` implements `codex.RecordReader`, so a catalog search is a `Convert`
source. This is the one package that reaches the network; it remains standard
library only.

```go
c := sru.NewClient("http://lx2.loc.gov:210/lcdb")
rd := c.NewReader(ctx, `dc.title = `+sru.Quote("moby dick")) // CQL; pages automatically
codex.Convert(rd, marcjson.NewWriter(os.Stdout))            // stream hits into any format

// Or one page at a time, with counts and diagnostics:
resp, _ := c.SearchRetrieve(ctx, sru.Request{Query: `bath.isbn = "9780142437247"`})
rec, _ := resp.Records[0].Decode() // MARCXML -> *codex.Record
```

MARCXML records decode to `*codex.Record`; records in other schemas (MODS, Dublin
Core) are returned as raw XML in `Record.Data` (those crosswalks are export-only).
The client targets SRU 1.1/1.2.

## Fetching records over Z39.50 (`z3950`)

For the many targets that predate SRU — legacy OPACs and classic ILS installs —
the `z3950` client speaks the original [Z39.50](https://www.loc.gov/z3950/agency/)
protocol (ISO 23950): BER-encoded APDUs over TCP, implemented from the published
standard in pure Go. It runs Initialize/Search/Present sessions with Type-1 RPN
queries over the bib-1 attribute set, built with a small query builder:

```go
c := z3950.NewClient("lx2.loc.gov:210/LCDB") // host:port/database
rd := c.NewReader(ctx, z3950.Term("title", "moby dick"))
codex.Convert(rd, marcjson.NewWriter(os.Stdout)) // same pipeline as sru

// Or session-level control:
conn, _ := c.Connect(ctx)
defer conn.Close()
res, _ := conn.Search(ctx, z3950.And(z3950.Term("author", "melville"), z3950.Term("any", "whale")))
recs, _ := conn.Present(ctx, 1, 10) // fetch records 1-10
```

Records decode by their record syntax: MARC21 via `iso2709` (including MARC-8
transcoding), UNIMARC via `unimarc`, MARCXML via `marcxml`; SUTRS text is exposed
raw. Requesting `Syntax: "opac"` returns each bib record with its **holdings**
(location, call number, circulation availability) attached as `Record.Holdings`
-- the interlibrary-loan "who holds this" query. Query access points: `any`, `title`, `author`, `subject`, `isbn`, `issn`,
`lccn`, `id`, combined with `And`/`Or`/`AndNot`. Multi-word terms automatically
search as phrases, a trailing `*` right-truncates (`"mob*"` finds moby), and
`.Exact()`/`.Phrase()`/`.Word()`/`.Truncated()` refine a term for strict servers.
Guarded targets authenticate via `Client.User`/`Password`/`Group` (idPass) or
`Client.AuthOpen`. Interop is tested against YAZ's `yaz-ztest` (skipped when YAZ
is not installed) and, opt-in via `Z3950_LIVE_TARGET`, any live target.

## Real-time availability over SIP2 (`sip2`)

Where SRU and Z39.50 answer "what does this library hold", [SIP2](https://en.wikipedia.org/wiki/Standard_Interchange_Protocol)
(3M Standard Interchange Protocol v2) answers "is this copy on the shelf right
now". The `sip2` client implements the discovery slice -- optional Login (93/94)
and Item Information (17/18) over a TCP session -- for a real-time availability
bridge; it does not cover checkout, holds, fees or patron messages.

```go
c := &sip2.Client{Address: "acs.example.org:6001", User: "term", Password: "pw"}
conn, _ := c.Connect(ctx) // dials and (since User is set) logs in
defer conn.Close()
info, _ := conn.ItemInformation(ctx, "30000123456789") // barcode
avail := sip2.CirculationStatusLabel(info.CirculationStatus) == "available"
```

`ItemInfo` surfaces the fixed status header (circulation status, security marker,
fee type, transaction date) and the variable fields a discovery caller reads --
item id (AB), title (AJ), due date (AH), current/permanent location (AP/AQ), the 3M
call-number extension (CS) and hold-queue length (CF) -- with every field also in
`ItemInfo.Fields`. `CirculationStatusLabel` exposes the 01-13 status table so a
caller folds it into its own availability rollup. Error detection (the AY/AZ
checksum) is opt-in via `Client.ErrorDetection` and always tolerated inbound, and
`Client.Dial` is a seam for tests.

## Reading UNIMARC

[UNIMARC](https://www.ifla.org/g/unimarc-rg/) (IFLA, used widely in Europe) shares
the ISO 2709 container with MARC 21 but uses a different data dictionary and
declares its character set in field 100, not the leader. The `unimarc` package
reads it, selecting the encoding (UTF-8, or legacy **ISO 5426** transcoded to
UTF-8 like MARC-8) and mapping it to the MARC 21 model so it flows into every
exporter above:

```go
recs, _ := unimarc.ReadFile("catalog.unimarc")
title := unimarc.Title(recs[0])     // 200 $a, non-sort markers stripped
authors := unimarc.Authors(recs[0]) // 700/701/710 …

m := unimarc.ToMARC21(recs[0])      // re-tag 200→245, 010→020, 700→100, …
b, _ := schemaorg.Encode(m)         // …then any exporter accepts it
```

## Reading SKOS vocabularies (`skos`)

Controlled vocabularies — subject thesauri like [homosaurus](https://homosaurus.org),
LCSH or FAST — are published as [SKOS](https://www.w3.org/TR/skos-reference/)
concept schemes in RDF, and they are the authority side of the subject headings
the crosswalk reads in a bib record. The `skos` package parses a concept scheme
(any RDF serialization, autodetected via the `rdf` package) and crosswalks each
`skos:Concept` to a MARC 21 **authority** record:

```go
concepts, _ := skos.Parse(data)     // []skos.Concept, broader/narrower resolved
rec := concepts[0].Record()         // MARC authority: 150 heading, 450/550 tracings, 024 URI
recs := skos.Records(concepts)      // …then any exporter accepts them
```

`skos:prefLabel` (English-preferred) becomes the `150` established heading, other
labels `450` see-from tracings, `broader`/`narrower`/`related` the `550` see-also
tracings (`$w g`/`$w h` for the hierarchy, with the target heading and `$0` IRI),
and the concept IRI a `024 … $2 uri`. The `libcodex skos` command prints a concept
view (with a summary header that surfaces IRI/scheme version drift) or converts to
MARC authority records in any output format.

## Accessors

On `*Record`:

- `Leader() Leader`, `Encoding() byte`, `Fields() []Field`
- `ControlField(tag string) string`
- `DataField(tag string) (Field, bool)`, `DataFields(tag string) []Field`
- `SubfieldValue(tag string, code byte) string`, `SubfieldValues(tag string, code byte) []string`
- Building/editing (chainable): `AddField`, `InsertField` (tag-ordered),
  `ReplaceField`, `RemoveFields(tag)`, `SetLeader`
- `Validate() error` — structural checks (24-byte leader, 3-byte tags, data
  fields have subfields)

On `Field`: `IsControl()`, `Indicators() (byte, byte)`, `Subfield(code)`,
`SubfieldValue(code)`, `SubfieldValues(code)`.

On `Leader`: `RecordStatus()` (byte 5), `RecordType()` (byte 6), `Encoding()`
(byte 9), `IsUnicode()`, `RecordLength()` (`[0:5]`), `BaseAddress()` (`[12:17]`).

### Fixed fields, multilingual linkage and accessibility

Higher-level accessors interpret the harder-to-parse parts of a record:

- **`Control008()`** — typed access to the positional 008 fixed field: `Date1`,
  `Date2`, `Language`, `Place`, and the material-aware `FormOfItem` (with
  `IsLargePrint` / `IsBraille`).
- **Vernacular (alternate-script) linkage** — MARC pairs a romanized field with
  its original-script `880` via subfield `$6`. `Field.Link()` parses that linkage
  (tag, occurrence, script code, right-to-left), `Record.AlternateGraphic(field)`
  returns the linked partner in either direction, and `Record.Vernacular(tag,
  code)` reads the original-script value directly:

  ```go
  title := rec.SubfieldValue("245", 'a')   // romanized
  original := rec.Vernacular("245", 'a')   // e.g. the Cyrillic or CJK form
  ```

- **`Accessibility()`** — gathers the record's accessibility metadata (008 form
  of item, 007 tactile category, the 341 Accessibility Content and 532
  Accessibility Note fields). The `schemaorg` exporter maps it to schema.org
  `accessMode` / `accessibilityFeature` / `accessibilitySummary` so reading
  systems and search engines can surface large-print, braille and captioned
  editions.

## Performance

Encoding hand-builds output into a reusable buffer, so writers stream at roughly
zero allocations per record. Decoding cost tracks the wire format. Indicative
numbers for a ~10-field record (Apple M3 Max):

| Format    | Decode (allocs / MB·s) | Encode (allocs) | Streaming write |
|-----------|------------------------|-----------------|-----------------|
| `iso2709` | 4 / 864                | 1               | ~0 allocs/record |
| `mrk`     | 35 / 200               | 7               | ~0 allocs/record |
| `marcxml` | 374 / 60               | 9               | ~0 allocs/record |
| `marcjson`| 566 / 40               | 7               | ~0 allocs/record |

The binary codec is the fast path for bulk work; the XML/JSON decoders are bound
by the standard library's `encoding/xml` and `encoding/json` tokenizers (a
deliberate correctness-over-speed, zero-dependency choice). Run `go test -bench=.`
in any subpackage to reproduce.

## Character encoding

MARC 21 with **UTF-8** (leader byte 9 == `'a'`) is the primary, preferred form.
The text formats (MARCXML, MARC-in-JSON, `.mrk`) are UTF-8 throughout.

For **older MARC-8** binary records (leader byte 9 == blank), `iso2709` transcodes
values to UTF-8 on read, so every value the API exposes is a UTF-8 Go string
regardless of source encoding. **Every MARC-8 graphic character set is supported:**

- **Basic Latin** (ASCII) and **ANSEL Extended Latin**, including spacing
  graphics and **combining diacritics**.
- **Basic and Extended Cyrillic, Basic and Extended Arabic, Basic Hebrew, Basic
  Greek, Greek Symbols, Subscripts and Superscripts.**
- the multibyte **East Asian (CJK) set, EACC** — all ~15,700 ideographs.

MARC-8 follows ISO 2022: a primary set in G0 governs bytes `0x21–0x7E` and an
extension set in G1 governs `0xA1–0xFE`, with escape sequences re-designating
either; combining marks are stored *before* their base (the reverse of Unicode),
and the decoder reorders them, composing common Latin pairs to NFC (e.g. combining
acute + `e` → `é`). The non-Latin and EACC tables are generated directly from the
[LoC MARC-8 code tables](https://www.loc.gov/marc/specifications/codetables.xml)
(`go generate ./internal/marc8`); the hand-maintained ANSEL table is verified
against them, including the 2005 alif remapping and the euro/eszett additions.

`iso2709` also **writes** legacy MARC-8 (leader byte 9 = blank) via
`iso2709.EncodeMARC8`, the inverse of the read path across all sets — emitting the
ISO 2022 escape sequences to switch sets and returning to the defaults at the end
of each value. It returns an error only if a value contains a character no MARC-8
set can represent (e.g. an emoji), so you never get a record that claims MARC-8
but holds untranscodable data.

A decode still **never crashes** on malformed input: an unrecognized set
designation or an unmapped EACC triple passes through best-effort, and
`iso2709.Decode` returns a `lossy bool` (and `iso2709.Reader.Lossy()` reports the
last read) so callers can detect it and avoid re-serializing mojibake as clean
UTF-8. Precomposed accented *non-Latin* text (e.g. NFC Greek `ά`) has no single
MARC-8 code; supply it in decomposed (NFD) form to encode, which is exactly what
decoding produces.

UNIMARC records use **ISO 5426** (extended Latin) rather than MARC-8; the
`unimarc` reader transcodes it the same way (combining mark before base, composed
to NFC), selecting it from field 100. See [Reading UNIMARC](#reading-unimarc).

## What each format rejects

A record is rejected on encode when a format cannot represent it, rather than
producing corrupt output:

- `iso2709`: a value, indicator or subfield code containing a reserved delimiter
  byte (`0x1d`/`0x1e`/`0x1f`); a field over 9999 bytes or a record over 99999.
- `marcxml`: any character XML 1.0 forbids (control characters such as NUL).
- `mrk`: a line break in any datum, or `$` used as a subfield code.
- `marcjson`: nothing — JSON can represent every character.

## Compression

Compression composes through `io`; it is not built in. Wrap the stream:

```go
gz, _ := gzip.NewReader(f)              // compress/gzip, stdlib
for rec, err := range iso2709.NewReader(gz).All() { … }
```

## Adding your own format

Implement `codex.RecordReader` and/or `codex.RecordWriter` over `*codex.Record`
in your own package — no changes to this module required. Your type then works
with `codex.Convert`, `codex.All`, and everything else built on the interfaces.
The four bundled codecs are the reference implementations.

## Tolerance of malformed input

- Decoders never panic on arbitrary bytes (verified by sustained fuzzing of every
  format).
- `iso2709.Reader.Read` returns a non-EOF error for a malformed record but
  advances past it using the declared length, so reading can continue. Directory
  entries pointing outside the field data are skipped rather than failing the
  record.
- The text readers skip blank/separator lines and unknown elements/keys.
- `ReadFile` stops at the first malformed record and returns the records parsed
  so far together with the error.

## License

MIT — see [LICENSE](LICENSE). Copyright Eve Freeman.
