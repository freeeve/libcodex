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

## Export converters (one-way)

Beyond the four round-trip serializations, the library exports to formats that
use a *different*, lossy model — a documented MARC→target crosswalk, not a codec.
Their `Writer`s implement `codex.RecordWriter`, so they also work with
`codex.Convert`:

| Package      | Target                                      |
|--------------|---------------------------------------------|
| `mods`       | MODS (LoC XML, near-lossless)               |
| `dublincore` | Dublin Core — `oai_dc` XML and DC-JSON      |
| `citation`   | RIS and BibTeX (reference managers)         |
| `bibframe`   | BIBFRAME 2.0 — RDF/XML and JSON-LD          |

```go
// Binary MARC → BibTeX for a reference manager.
codex.Convert(iso2709.NewReader(in), citation.NewBibTeXWriter(out))
// Or a single record: b, _ := mods.Encode(rec) / dublincore.Encode(rec) / citation.RIS(rec)
```

`bibframe` is the one that changes data *model*, not just serialization: each MARC
record becomes a small RDF graph of a `bf:Work` (intellectual content) and a
`bf:Instance` (a publication of it), linked by `bf:instanceOf`/`bf:hasInstance`.
Both serializations are hand-written with the standard library — no RDF dependency:

```go
// Binary MARC → BIBFRAME RDF/XML (canonical) or JSON-LD.
b, _ := bibframe.Encode(rec)        // RDF/XML
b, _ := bibframe.EncodeJSONLD(rec)  // JSON-LD
// Streaming collections (must be closed): bibframe.NewWriter / NewJSONLDWriter.
```

These are export-only (the targets cannot round-trip back to full MARC) and carry
only the common fields; each package documents its crosswalk.

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
values to UTF-8 on read for the common Western subset, so every value the API
exposes is a UTF-8 Go string regardless of source encoding:

- **Basic Latin** (ASCII, the default G0 set).
- **ANSEL Extended Latin** (the default G1 set), including spacing graphics and
  **combining diacritics**. MARC-8 stores a combining mark *before* its base
  character (the reverse of Unicode); the decoder reorders it and composes common
  base+mark pairs to a precomposed (NFC) code point (e.g. combining acute + `e` →
  `é`). The ANSEL table is verified against the LoC code tables, including the
  2005 alif remapping and the euro/eszett additions.

`iso2709` can also **write** legacy MARC-8 (leader byte 9 = blank) via
`iso2709.EncodeMARC8`, the inverse of the read path over the same Western subset.
It returns an error if a value contains a character outside that subset, so you
never get a record that claims MARC-8 but holds untranscodable data.

**Out of scope** (best-effort pass-through, never a crash): EACC/CJK, Cyrillic,
Greek, Hebrew, Arabic and the subscript/superscript/Greek-symbol sets. On read,
their escape designations are recognized only well enough to skip them; their
bytes pass through as Latin-1 rather than being transcoded, and `iso2709.Decode`
returns a `lossy bool` (and `iso2709.Reader.Lossy()` reports the last read) so
callers can detect this and avoid re-serializing mojibake as clean UTF-8. On
write, `EncodeMARC8` rejects them outright.

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

MIT — see [LICENSE](LICENSE). Copyright Queer Liberation Library.
