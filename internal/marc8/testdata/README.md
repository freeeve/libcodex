# MARC-8 test data

## `pymarc_marc8.txt`

Real-world MARC-8 multiscript text used to exercise the decoder on authentic
data. Each line is a raw MARC-8 byte sequence (with ISO 2022 escape designations)
covering four character sets: Basic Latin (ASCII), the multibyte CJK set (EACC),
Basic Arabic and Basic Hebrew.

Source: the [pymarc](https://gitlab.com/pymarc/pymarc) project's
`test/test_marc8.txt`, copyright Ed Summers and contributors, licensed BSD-2-Clause.
Unmodified, redistributed here under that license for testing purposes.
