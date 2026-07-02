# 077 -- z3950: authentication (idPass / open)

Tier 1 -- CRITICAL for real-library use. Split from the 075 deferred checklist.
Ref: Z39.50-1995 InitializeRequest idAuthentication [7]; `z3950/apdu.go`
`encodeInitRequest`.

## Motivation

Many everyday Z39.50 targets require login: OCLC, commercial ILS installs
(Voyager, Symphony, Sierra), and peer libraries that gate their catalogs. The
client currently omits idAuthentication entirely, so those targets reject the
session at Initialize. This is the single biggest reachability gap for real-world
use, and it is one optional field in one APDU.

## Scope

1. `Client.User`, `Client.Password`, `Client.Group` (optional) fields.
2. Encode idAuthentication [7] in the InitializeRequest:
   - idPass form (SEQUENCE of groupId [0] / userId [1] / password [2]) when a
     user or password is set;
   - open form (a bare VisibleString "user/password") as a fallback knob for the
     odd server that only takes it -- `Client.AuthOpen string`.
3. Surface a rejected Initialize (result=false) with any userInformationField /
   implementation string the server returns, so a bad password reads as such.
4. Never log or echo the password (keep it out of error strings).

## Hazards

- idAuthentication sits between exceptionalRecordSize [6] and implementationId
  [110] -- BER SEQUENCEs are order-sensitive (the 075 Present bug was exactly
  this class); add a fake-server test asserting field order.
- yaz-ztest accepts any credentials, so the interop test can only prove the
  field is well-formed, not that auth semantics work; the fake server should
  assert the decoded user/password match.

## Acceptance

- [ ] idPass and open forms encoded correctly (fake-server assertions on order
      and content); anonymous behavior unchanged when unset.
- [ ] yaz-ztest interop still green with credentials set.
- [ ] Passwords absent from all error strings.
