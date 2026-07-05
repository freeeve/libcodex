# marcjson: normalize non-printable indicator bytes on decode

## Bug

Found by the weekly CI fuzz run (2026-07-05, run 28730521037):
`FuzzDecode` round-trip instability in `marcjson`.

`indByte` (marcjson/marcjson.go:111) returns the raw first byte of an
`ind1`/`ind2` JSON string, so `"ind1":"\x00"` decodes to `Ind1: 0x00`.
The encode side treats `0x00` as "unset": `validate` allows it
(`f.Ind1 != 0 && !asciiChar(f.Ind1)`) and `indStr` serializes it as
`" "`, which re-decodes as `0x20`. Decode -> encode -> decode is not a
fixed point:

```
a = Field{Tag:"1", Ind1:0x00, Ind2:0x20}
b = Field{Tag:"1", Ind1:0x20, Ind2:0x20}
```

Subfield codes do not have the same hole: `validate` rejects any
non-printable `Code` outright, so encode errors instead of silently
normalizing.

## Repro

Minimized fuzz input (Go corpus format, was
`marcjson/testdata/fuzz/FuzzDecode/0d6087ee005d56d5` in the run's
fuzz-crashers artifact):

```
go test fuzz v1
[]byte("{\"fields\"[{\"1\"{\"ind1\"\"\x00\"}}]}")
```

Save that file under `marcjson/testdata/fuzz/FuzzDecode/` and run
`go test ./marcjson -run='FuzzDecode/0d6087ee005d56d5'`.

## Suggested fix

Make decode agree with encode's notion of "blank": in `indByte`, fold
any byte that fails `asciiChar` to `' '` (covers `0x00`-`0x1F` and
`0x7F`+). One-line change:

```go
func indByte(s string) byte {
	if s == "" || !asciiChar(s[0]) {
		return ' '
	}
	return s[0]
}
```

Commit the minimized input above as a regression seed alongside the fix
(committing it before the fix would fail `go test ./...` and block the
pre-push hook).
