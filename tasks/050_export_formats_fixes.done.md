# 050 -- mods/dublincore/citation/schemaorg: mapping and API fixes

## Motivation

Review findings in the export packages: RDA 264 handling produces wrong
dates and duplicate places, BibTeX corporate authors are structurally
misparsed by consumers, archival titles vanish, and two writers do
avoidable per-record work.

## Problems

1. **mods `mergeOrigin` ignores 264 ind2 and duplicates places**
   (mods.go:159-160, :235-245). A record whose only 264 is ind2='4'
   (copyright) gets `dateIssued` = "©2016"; a 264_2 distributor becomes
   `<publisher>`; and `o.Place = append(...)` runs for every 260/264 while
   publisher/date are first-wins, so RDA hybrids (260+264, 264_1+264_4)
   emit duplicate `<place>` elements. Prefer 264 ind2='1' (or 260) for
   publisher/date/place, map 264_4 $c to MODS `<copyrightDate>`, dedupe
   Place. (bibframe has the same ind2 defect -- task 047; align the rule.)
2. **BibTeX corporate authors joined with " and " and unprotectable**
   (citation.go:48-51, :259-261, :303). 110/710 names go into the same
   `Authors []string`; BibTeX splits on the word "and", so
   `Food and Agriculture Organization` parses as two authors, and the
   standard `{...}` brace protection is impossible because `appendBibTeX`
   escapes braces in values. Track corporateness per author (e.g.
   `Author{Name string; Corporate bool}`) and emit corporate names in
   literal braces.
3. **mods drops the whole 245 titleInfo when $a is absent**
   (mods.go:143-146). The `t.Title != ""` guard discards $b/$n/$p too;
   records titled only via $k/$n/$p (archival/multipart material) lose
   their title entirely while dublincore keeps it. Keep the element when
   any of Title/SubTitle/PartNumber/PartName is non-empty.
4. **Writer inefficiencies** (mods.go:437; citation.go:409).
   `mods.Writer.Write` runs reflection-based `xml.MarshalIndent` per record
   then reallocates to append '\n' (dublincore/schemaorg reuse `wr.buf` for
   the same job) -- mods WriterStream benches at ~154 MB/s with 4,903
   allocs/op vs dublincore's ~678 MB/s. `BibTeXWriter` copies each rendered
   entry just to prepend a newline; write the separator first.
5. **API consistency** (citation.go). citation exposes `RIS`/`BibTeX` where
   every other package uses `Encode`, and their `error` returns are always
   nil. Either add `Encode` aliases or document the divergence; drop or
   justify the dead error returns (breaking-change call -- pre-1.0 window
   is the time).

## Acceptance

- [x] 264 ind2 table test: production/publication/distribution/manufacture/
      copyright each map correctly; no duplicate `<place>` for RDA hybrids.
      (`TestOrigin264Indicators`, `TestRDAHybridNoDuplicatePlace`.)
- [x] `author = {{Food and Agriculture Organization}}` (or equivalent
      brace-protected form) in BibTeX output; parser-perspective test.
      (`TestBibTeXCorporateAuthor`.)
- [x] 245 with only $n/$p yields a non-empty MODS titleInfo.
      (`TestTitleFromPartOnly`.)
- [x] mods WriterStream allocs/op reduced materially; citation newline copy
      gone. See benchstat below.
- [x] API decision on `RIS`/`BibTeX` naming documented in the package doc.

## Resolution

1. **mods origin** -- `originFromPublication` ranks the 260/264 fields
   (264 ind2='1' > 260 > other 264 roles; copyright never chosen), reads
   place/publisher/date from the single best field (no cross-field place
   duplication), and maps a 264 ind2='4' $c to the new
   `OriginInfo.CopyrightDate`. Ranking matches bibframe's `provisionStatement`.
2. **BibTeX corporate authors** -- `Entry.Authors` is now `[]Author{Name,
   Corporate}`; corporate/conference names (110/710/111/711) are wrapped in a
   literal brace group so BibTeX does not split them on an internal "and".
3. **mods title** -- the 245 titleInfo is kept when any of
   Title/SubTitle/PartNumber/PartName is present, not only $a.
4. **Writer efficiency** -- mods now hand-rolls its serializer
   (`serialize.go`) into a reused per-Writer buffer instead of reflection-based
   `xml.MarshalIndent` + a `\n` realloc; output is byte-identical (golden
   unchanged). The `BibTeXWriter` writes the inter-entry newline separately
   rather than prepending it to a fresh slice. The XML text escaper is now
   shared (`crosswalk.AppendXMLText`, used by mods and dublincore).
5. **citation API** -- the package doc explains why citation exposes format-named
   `RIS`/`BibTeX` (two formats, not one `Encode`) and keeps the always-nil error
   for signature parity with the other exporters.

### benchstat (Apple Silicon, -benchtime=200ms)

| bench                    | before                     | after                       |
|--------------------------|----------------------------|-----------------------------|
| mods Encode              | 17364 ns, 8066 B, 51 allocs | 9423 ns, 4568 B, 28 allocs |
| mods WriterStream        | 62.6 MB/s, 4903 allocs/op  | 198.6 MB/s, 1912 allocs/op  |
