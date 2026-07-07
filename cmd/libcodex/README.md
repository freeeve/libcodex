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
