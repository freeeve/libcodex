# 076 -- sru: cap searchRetrieve response body reads

Small hardening task. From the 074 code review.
Ref: `sru/sru.go` `SearchRetrieve` (`io.ReadAll(resp.Body)`).

## Motivation

`SearchRetrieve` reads the whole HTTP response body unbounded. Every other
unbounded read in the library is from a local file the caller chose; this one is
from a remote server, so a misbehaving or hostile endpoint can hold the connection
open and stream bytes until the process OOMs. A network client should bound what
it is willing to buffer.

## Scope

1. Add a `MaxResponseBytes int64` knob on `Client` (0 -> a generous default, e.g.
   64 MiB; negative -> unlimited for callers who genuinely need it).
2. Wrap the body in `io.LimitReader(resp.Body, cap+1)`; if more than cap bytes
   arrive, fail with a clear error naming the limit and the knob -- do not
   silently truncate into an XML parse error.
3. Document the default on the field and in the package doc.

## Hazards

- A silently truncated body would surface as a confusing `sru: parse response`
  error; the limit check must produce its own error instead.
- Pick the default so no legitimate SRU page trips it (pages are bounded by
  `maximumRecords`; even fat full-MARCXML pages are a few MiB -- 64 MiB is far
  above any real response while still bounding memory).
- The same knob should be honored by any future transport (Z39.50 [075]
  preferredMessageSize negotiation is that protocol's native equivalent).

## Acceptance

- [x] Oversized response -> a distinct "response exceeds N bytes" error, not a
      parse error; body memory bounded by the cap.
- [x] Default generous (no fixture or realistic page trips it); 0/negative
      semantics tested.
- [x] Suite green; no API break for existing callers.

## Result

`Client.MaxResponseBytes` (0 -> 64 MiB default, negative -> unlimited) bounds
the body read via `io.LimitReader(body, limit+1)` in a new `readBody` helper; a
response over the limit fails with "response exceeds N bytes; raise
Client.MaxResponseBytes..." rather than a confusing truncated-XML parse error.
`TestMaxResponseBytes` covers the distinct error, the default, unlimited, and
an exact-size limit.
