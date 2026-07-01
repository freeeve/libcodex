# 043 -- marcjson/marcxml: validation alignment and wire-type contradictions

## Motivation

The three MARC text codecs disagree on what they reject: records that error
in marcxml silently corrupt in marcjson, and both decoders accept documents
whose wire type contradicts the tag range, producing silent data loss on
re-encode. The fuzz tests currently carve these cases out via
`selfConsistent` exemptions instead of fixing them.

## Problems

1. **marcjson corrupts non-ASCII indicators and subfield codes**
   (marcjson.go:102-107, :62, :136-154). `string(b)` on a byte >= 0x80 emits
   the multi-byte UTF-8 encoding, while decode's `indByte`/`codeByte` take
   `s[0]`. Verified: `Ind1: 0xE9` encodes as `"é"` and round-trips back as
   0xC3. marcxml's `validate` rejects exactly these bytes; marcjson's
   checks only value UTF-8. Align marcjson's `validate` with marcxml's
   printable-ASCII rule for indicators and codes.
2. **marcjson never validates `f.Tag`** (marcjson.go:136-154, :47). A tag
   containing 0xFF encodes to output that is not legal JSON (RFC 8259
   requires UTF-8); Go's lenient decoder silently mutates it to U+FFFD,
   stricter consumers reject the document. Add `utf8.ValidString(f.Tag)`
   (and reasonably `len == 3`) to `validate`.
3. **Wire-type vs tag-range contradiction drops data in both codecs**
   (marcxml.go:270-282, marcjson.go:271-283). `<controlfield tag="245">x`
   decodes into `Field{Tag:"245", Value:"x"}`, which `IsControl()` treats as
   a data field, so re-encode silently drops the value (and `{"001":{...}}`
   drops its subfields). Have the decoders return an error when the wire
   type contradicts the tag range -- and remove the corresponding
   `selfConsistent` fuzz exemptions so the property is actually enforced.

## Acceptance

- [ ] The same malformed record errors identically (or corrupts nowhere)
      across marcxml, marcjson, and mrk.
- [ ] `Ind1: 0xE9` and `Tag: "2\xff5"` return Encode errors in marcjson.
- [ ] Control/data wire-type contradictions error on Decode in both codecs;
      round-trip through `codex.Convert` is lossless or loud.
- [ ] `selfConsistent` carve-outs for these cases deleted from both fuzzers;
      fuzz suites still pass.
