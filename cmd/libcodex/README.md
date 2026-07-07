# libcodex (CLI)

A command-line front-end for the libcodex bibliographic codecs. It inspects,
converts, validates and profiles MARC records and their sibling serializations,
wiring the library's format packages behind four subcommands.

## Install

With Go:

```sh
go install github.com/freeeve/libcodex/cmd/libcodex@latest
```

With Homebrew (this repo is its own tap):

```sh
brew tap freeeve/libcodex https://github.com/freeeve/libcodex
brew install freeeve/libcodex/libcodex
```

Or grab a prebuilt binary for your OS/arch from the
[latest release](https://github.com/freeeve/libcodex/releases/latest)
(linux/macOS/windows, amd64/arm64), or build from a checkout:

```sh
go build -o libcodex ./cmd/libcodex
```

`libcodex version` reports the build version.

## Usage

```
libcodex cat       [-i fmt] [-t tags] [-n N] [--json] [file...]   readable dump
libcodex convert   [-i fmt] -o fmt [file...]                      transcode
libcodex validate  [-i fmt] [file...]                             structural check
libcodex stats     [-i fmt] [file...]                             field/leader report
libcodex skos      [-o fmt] [-n N] [file...]                      SKOS vocab view / to MARC authority
```

`-i` sets the input format; when omitted it is auto-detected from the leading
bytes. With no file arguments a subcommand reads standard input, so commands
compose in a pipe.

### Formats

| Format               | Read | Write | Notes                                  |
|----------------------|:----:|:-----:|----------------------------------------|
| `marc` / `iso2709`   |  ✓   |   ✓   | ISO 2709 binary MARC                   |
| `marcxml` / `xml`    |  ✓   |   ✓   | MARC21slim collection                  |
| `marcjson` / `json`  |  ✓   |   ✓   | MARC-in-JSON                           |
| `mrk`                |  ✓   |   ✓   | MARCMaker line format                  |
| `unimarc`            |  ✓   |       | read-only                              |
| `bibframe`           |  ✓   |   ✓   | BIBFRAME RDF/XML                       |
| `dublincore`         |      |   ✓   | write-only, lossy display projection   |
| `mods`               |      |   ✓   | write-only, lossy display projection   |
| `schemaorg`          |      |   ✓   | write-only, schema.org JSON-LD         |
| `ris`                |      |   ✓   | write-only, reference-manager RIS      |
| `bibtex`             |      |   ✓   | write-only, BibTeX                     |

Auto-detection recognizes an ISO 2709 leader (five leading digits), an XML
document (BIBFRAME RDF/XML vs plain MARCXML), a JSON array/object, and the
`=LDR` line of the mrk format. A leading UTF-8 BOM is tolerated. A stream that
cannot be identified is an error rather than a silent zero-record read; pass
`-i` to set the format explicitly.

## Subcommands

### cat -- readable dump

Prints each record in the mrk line format, or MARC-in-JSON with `--json`.

- `-t tags` keeps only the listed fields, comma-separated (`-t 084,650`).
- `-n N` stops after N records across all inputs.

```sh
libcodex cat -t 084,650 -n 2 records.mrc
```

```
=LDR  02260nam  2200361Ka 4500
=084  \\$aYAF010010$aYAF010140$aYAF010170$2bisacsh
=650  17$aYoung Adult Fiction.$2OverDrive
=650  \7$aLGBTQIA+ (Fiction).$2OverDrive
```

### convert -- transcode

Reads every record and writes it in the `-o` output format to standard output.
Any input format converts to any output format.

```sh
libcodex convert -o marcxml   records.mrc  > records.xml
libcodex convert -o bibframe  records.mrc  > records.rdf
libcodex convert -o marcjson  records.mrc  | libcodex cat -t 245 -n 1
```

### validate -- structural check

Runs the record-model structural check on every record, printing the position
and 001 of each failure and a summary line. Exits non-zero if any record is
invalid or a stream fails to parse, so it works as a gate in scripts.

```sh
libcodex validate records.mrc
```

```
checked 67 record(s), 0 invalid
```

### stats -- field and leader profile

Reports the record count, the UTF-8 / MARC-8 encoding split, the leader/06
record-type and leader/07 bibliographic-level breakdowns, and a per-tag field
frequency.

```sh
libcodex stats records.mrc
```

```
records: 67
encoding: 0 unicode, 67 marc-8

record type (leader/06):
  a  67

bibliographic level (leader/07):
  m  67

fields:
  001  67
  084  67
  245  67
  650  225
  856  266
```

### skos -- SKOS concept scheme view / MARC authority crosswalk

Reads a SKOS concept scheme (a controlled vocabulary such as homosaurus published
as RDF -- N-Triples/N-Quads/Turtle/RDF-XML/JSON-LD, autodetected). By default it
prints a readable concept view led by a summary header; with `-o` it crosswalks
each `skos:Concept` to a MARC **authority** record and writes it in that output
format (`marc`, `marcxml`, `marcjson`, `mrk`). `-n N` limits the concept count.

```sh
libcodex skos homosaurus.nt              # concept view
libcodex skos -o marcxml homosaurus.nt   # -> MARC authority records
```

The summary header surfaces the IRI base(s), the `skos:inScheme` value(s) and the
label languages, so a version or scheme mismatch is visible at a glance:

```
# 4160 concepts
# IRI base:    https://homosaurus.org/v4/ (4160)
# inScheme:    https://homosaurus.org/v3 (3744), https://homosaurus.org/v4 (416)  [mixed]
# label langs: en (4088), es (3011), en-gb (80), ...

homoit0000001  5-alpha reductase deficiency [en]
  uri: https://homosaurus.org/v4/homoit0000001
  alt: 5-ARD [en], 5-ARD [es]
  broader: Intersex variations (homoit0000669)
```

The crosswalk maps `skos:prefLabel` (English-preferred) to the `150` established
heading, other labels to `450` see-from tracings, `broader`/`narrower`/`related`
to `550` see-also tracings (`$w g`/`$w h` for the hierarchy, with the target
heading and `$0` IRI), the concept IRI to `024 7 … $2 uri`, and scope notes to
`680`.
