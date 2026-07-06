# codex

A command-line front-end for the libcodex bibliographic codecs. It inspects,
converts, validates and profiles MARC records and their sibling serializations,
wiring the library's format packages behind four subcommands.

## Install

```sh
go install github.com/freeeve/libcodex/cmd/codex@latest
```

Or build from a checkout:

```sh
go build -o codex ./cmd/codex
```

## Usage

```
codex cat       [-i fmt] [-t tags] [-n N] [--json] [file...]   readable dump
codex convert   [-i fmt] -o fmt [file...]                      transcode
codex validate  [-i fmt] [file...]                             structural check
codex stats     [-i fmt] [file...]                             field/leader report
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
| `schemaorg`          |      |   ✓   | write-only, lossy display projection   |

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
codex cat -t 084,650 -n 2 records.mrc
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
codex convert -o marcxml   records.mrc  > records.xml
codex convert -o bibframe  records.mrc  > records.rdf
codex convert -o marcjson  records.mrc  | codex cat -t 245 -n 1
```

### validate -- structural check

Runs the record-model structural check on every record, printing the position
and 001 of each failure and a summary line. Exits non-zero if any record is
invalid or a stream fails to parse, so it works as a gate in scripts.

```sh
codex validate records.mrc
```

```
checked 67 record(s), 0 invalid
```

### stats -- field and leader profile

Reports the record count, the UTF-8 / MARC-8 encoding split, the leader/06
record-type and leader/07 bibliographic-level breakdowns, and a per-tag field
frequency.

```sh
codex stats records.mrc
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
