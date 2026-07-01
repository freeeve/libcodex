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

- [ ] 264 ind2 table test: production/publication/distribution/manufacture/
      copyright each map correctly; no duplicate `<place>` for RDA hybrids.
- [ ] `author = {{Food and Agriculture Organization}}` (or equivalent
      brace-protected form) in BibTeX output; parser-perspective test.
- [ ] 245 with only $k/$n/$p yields a non-empty MODS titleInfo.
- [ ] mods WriterStream allocs/op reduced materially (benchstat recorded);
      citation newline copy gone.
- [ ] API decision on `RIS`/`BibTeX` naming documented in the package doc.
