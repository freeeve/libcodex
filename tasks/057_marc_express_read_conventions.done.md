# 057: feat(bibframe): read OverDrive MARC Express field conventions on import (037/084/650 _7)

> Filed by the libcatalog session as a cross-repo handoff, **uncommitted** per the
> repo boundary (a concurrent libcodex session owns commits here). Consumer request
> backing libcatalog `tasks/007` (MARC Express reconciliation) and measured by
> libcatalog `tasks/003` (round-trip fidelity).

**Severity: MED** -- `FromRecord` silently drops fields that real OverDrive MARC
Express records (and other ILS exports using these conventions) carry, so a
MARC-import ramp loses the acquisition key and the BISAC classification. No frozen
layout changes; these are crosswalk read-path additions.

## Evidence (measured, libcatalog `tasks/003`)

libcatalog round-trips the vendored OverDrive MARC Express samples
(`od-sample-{ebook,audiobook}.mrc`, 30 records) `MARC --Encode--> BIBFRAME
--Decode--> MARC` and pins the loss. Fields lost that these conventions define:

- **037 (Source of Acquisition) -- the OverDrive Reserve ID.** MARC Express puts it in
  `037 $a` (+ `$b OverDrive, Inc.`). `FromRecord` does not read 037, so the Reserve ID
  -- the client-side availability key -- is dropped on the MARC path. (libcatalog's
  *direct* JSON→BIBFRAME path keeps it as a `bf:source`-tagged identifier, which is why
  OverDrive ingests directly; but any ILS bringing 037 hits this gap.)
- **084 (Other Classification Number) -- BISAC.** MARC Express puts BISAC in
  `084 $a…$a… $2 bisacsh` (repeated `$a`, one field). `FromRecord` reads BISAC only
  from **072** (`bibframe.go` `case "072"`), so 084-encoded BISAC is dropped.
- **650 _7 `$2 OverDrive` (source-specified topical subject).** MARC Express marks its
  uncontrolled subjects with ind2=7 + `$2 OverDrive`. Confirm these are read as
  `bf:subject` (600/610/611/650/651 are handled; verify the ind2=7 + `$2` named-source
  form is not skipped).

## Requested

1. **Read 037** → an Instance identifier (`bf:Identifier`), ideally carrying its
   `$2`/`$b` as the `bf:source`/agency so the scheme is recoverable (mirrors the 072
   `$2` handling). At minimum do not drop it.
2. **Read 084 as a classification** (`bf:Classification`) with `bf:source` from `$2`
   (e.g. `bisacsh`), alongside the existing 072 path. Repeated `$a` → multiple codes.
3. **Confirm/So that 650 _7 `$2 OverDrive`** subjects crosswalk to `bf:subject` (with
   the source named), not silently dropped by an indicator/`$2` check.

## Also (minor, separate)

- **020 `$a` ISBN qualifier.** MARC Express writes `020 $a 9781…842 (electronic bk)`.
  `FromRecord` keeps the whole string -- including `(electronic bk)` -- as the ISBN
  value, so downstream sees a non-normalized ISBN. Consider stripping the trailing
  parenthetical qualifier into a separate field (or `bf:qualifier`) so the ISBN value
  is just the number. Low priority; surfaced by libcatalog MARC ingest.

## Acceptance

- A MARC Express record round-trips 037 and 084 through `FromRecord` (present in the
  resulting BIBFRAME), verified against the vendored samples.
- 650 _7 `$2 OverDrive` subjects appear as `bf:subject`.
- No change to frozen on-disk layouts; add read-path tests per field.

## Outcome (libcodex)

Implemented the forward read path in `FromRecord` (`bibframe/bibframe.go`):

- **037 → Instance `bf:Identifier`:** `$a` value with the source scheme (`$2`) or,
  failing that, the supplying agency (`$b`) recorded as `bf:source`, so the
  OverDrive Reserve ID survives MARC import.
- **084 → `bf:Classification`** per repeated `$a`, with the `$2` scheme (e.g.
  `bisacsh`) as `bf:source`, mirroring the existing 072 handling.
- **650 _7 `$2 <source>`:** confirmed already read as `bf:subject` -- `FromRecord`
  reads 6xx regardless of indicators, so the named-source form is not skipped.

Tests: `bibframe/marc_express_test.go` (037/084/650 _7 plus an empty-field guard).
Existing RDF/XML and JSON-LD goldens are unchanged (the sample record carries none
of these fields); full suite and the FromMARC fuzz target pass. `presize` and an
extracted `appendIdentifier` helper were updated for the new tags.

**Deferred** -- the "Also" item (strip the 020 `$a` ISBN parenthetical qualifier)
is left as-is: it needs a design decision on where the qualifier lands (separate
field vs `bf:qualifier`) and is flagged low priority. Worth a small separate task
if downstream needs a normalized ISBN value.

**Scope note:** this is the read path (MARC → BIBFRAME), which is what the
acceptance asks for ("present in the resulting BIBFRAME"). The reverse crosswalk is
unchanged, so on BIBFRAME → MARC the data survives but under the generic
identifier/classification tags rather than being re-emitted as 037/084 -- exact-tag
round-trip would be a separate reverse-crosswalk task.

## Consumer note (libcatalog)

Unblocks libcatalog `tasks/007` read side (its MARC provider, `ingest/marc`, already
routes MARC through the two-tier identity + clustering pipeline via `FromRecord`; it
inherits whatever `FromRecord` reads). libcatalog `docs/marc-fidelity.md` +
`bibframe/roundtrip_test.go` will register the fidelity improvement when this lands
(the round-trip gate there expects 037/084 in its known-loss set until then).
