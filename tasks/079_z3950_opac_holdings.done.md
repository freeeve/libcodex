# 079 -- z3950: OPAC record syntax (holdings)

Tier 2 -- important for ILL workflows, not for bib retrieval. Split from the 075
deferred checklist. Ref: Z39.50 OPAC record syntax (OID 1.2.840.10003.5.102);
`z3950/apdu.go` `parseExternal`.

## Motivation

Interlibrary loan and collection-check workflows ask "who holds this and at what
call number", which Z39.50 answers with the OPAC record syntax: a bibliographic
record plus holdings data. The client currently tags such records "opac" and
exposes raw BER bytes -- unusable without a decoder.

## Scope

1. Parse the OPAC record structure: the embedded bibliographicRecord EXTERNAL
   (decode via the existing syntax dispatch) plus holdingsData entries.
2. A `Holdings` struct on `Record` (location, call number, circulation status --
   the commonly populated fields), leaving rare fields for later.
3. `Syntax: "opac"` requestable via `Client.Syntax`.

## Hazards

- OPAC holdings encoding varies wildly by ILS; parse defensively and keep
  unknown members raw. Capture fixtures from at least two different server
  types before trusting the shape.
- yaz-ztest can serve OPAC test records (database "Default" with preferred
  syntax OPAC) -- use it for interop; hermetic fixtures from captured bytes.

## Acceptance

- [x] OPAC records decode into bib record + holdings list against yaz-ztest.
- [x] Unknown holdings members skipped defensively, never a parse failure.
- [x] Hermetic tests (both EXTERNAL tagging variants); suite green.

## Result

- `Client.Syntax = "opac"` requests the OPAC record syntax; `parseExternal`
  routes it to `parseOPAC`, which unwraps the OPACRecord: the embedded
  bibliographicRecord EXTERNAL becomes the Record's Syntax/Data (so `Decode()`
  works transparently -- syntax reads "marc21") and holdingsData becomes
  `Record.Holdings`.
- `Holding{NUCCode, LocalLocation, ShelvingLocation, CallNumber, CopyNumber,
  PublicNote, EnumAndChron, Circulation}` and `Circulation{AvailableNow,
  AvailabilityDate, ItemID, Renewable, OnHold}` carry the commonly populated
  members; unknown tags are skipped, never a failure.
- `externalIn` accepts both bibliographicRecord tagging variants (explicit
  wrap of a universal EXTERNAL, or implicit tag replacement) -- servers emit
  both; the hermetic tests encode each.
- MARC-holdings-format HoldingsRecords ([1] EXTERNAL) are skipped for now; an
  OPAC record with holdings but no bib stays syntax "opac" with Data unset.
- Interop: yaz-ztest's canned OPAC record round-trips with location, call
  number and an available circulation item, matching yaz-client's own view.
