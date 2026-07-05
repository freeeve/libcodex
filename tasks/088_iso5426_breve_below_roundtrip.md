# iso5426: 0xF9 mark/graphic disambiguation breaks re-encode round-trip

## Bug

Found by the weekly CI fuzz run (2026-07-05, run 28730521037):
`FuzzEncode` in `internal/iso5426` -- "re-encode of decoded form failed:
iso5426: cannot encode U+032E".

ISO 5426 byte `0xF9` is both the letter `ø` and the combining
breve-below mark (U+032E); `Decode` disambiguates by treating it as a
mark when it composes with the *following* byte (iso5426.go:127-136).
That check ignores marks already pending. For input `"ø̤H"`
(ø + U+0324 combining diaeresis below + H), `Encode` emits
`0xD7 0xF9 0x48` (mark before base, per the format). Decoding that:

1. `0xD7` -> pending = [U+0324]
2. `0xF9` -> `composes(0xF9, 'H')` is true (Ḫ exists), so it is taken
   as a *second* mark: pending = [U+0324, U+032E] -- the ø is stolen
   from its own diacritic
3. `'H'` -> H + U+0324 has no precomposed form, so flush writes
   `H` + U+0324 + U+032E

The decoded string contains a standalone U+032E, which `Encode` cannot
represent: `buildEncode` deliberately skips `0xF9` when building
`encCombining` (iso5426.go:222-227) because a standalone mark there is
ambiguous. So `Encode(Decode(Encode(s)))` fails.

## Repro

Minimized fuzz input (Go corpus format, was
`internal/iso5426/testdata/fuzz/FuzzEncode/3641c89f55d60f26` in the
run's fuzz-crashers artifact):

```
go test fuzz v1
string("ø̤H")
```

Save under `internal/iso5426/testdata/fuzz/FuzzEncode/` and run
`go test ./internal/iso5426 -run='FuzzEncode/3641c89f55d60f26'`.

## Suggested fix

In `Decode`, treat `0xF9` as the graphic `ø` whenever marks are already
pending: a pending mark must attach to a base, and ISO 5426 places marks
*before* their base, so a graphic-capable byte arriving with marks
pending is that base. With that rule, `0xD7 0xF9 0x48` decodes back to
`"ø̤H"` -- the original input -- and the round-trip closes.

Check the adjacent cases while there: `0xF9 0xF9 <base>` (breve-below
on ø), `0xF9` at end of input after pending marks, and the existing
`{0xF9, 0x48} -> Ḫ` seed (iso5426_test.go:27) which must keep decoding
as a mark when nothing is pending. Commit the minimized input above as
a regression seed alongside the fix (committing it before the fix would
fail `go test ./...` and block the pre-push hook).
