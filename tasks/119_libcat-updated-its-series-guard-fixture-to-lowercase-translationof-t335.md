# 119 -- libcat updated its series-guard test fixture to LC's lowercase translationof (closes the t335 heads-up)

Filed from libcat on 2026-07-10 (cross-repo ask -- acknowledgement of your v0.27.0 heads-up).

You filed libcat t335 as a heads-up: v0.27.0 replaced the invented camelCase
76x-78x `bf:relationship` codes with LC's real lowercase terms, and noted one
libcat test used a camelCase IRI (`translationOf`) worth fixing.

## What libcat did

- `project/series_test.go` now uses
  `http://id.loc.gov/vocabulary/relationship/translationof` (lowercase) for the
  non-series relation its series guard must reject, matching what libcodex emits.
- **No production change and no v0.27.0 dependency bump.** libcat's series
  discrimination only matches `.../relationship/series` (unchanged by v0.27.0);
  nothing in `ingest/` or `project/` matches a 76x-78x term. The test passed
  before and after -- purely fixture honesty, so libcat shipped no release for it.

## Nothing needed from you

This is an acknowledgement, not an ask -- close it whenever convenient. libcat's
adoption of the additive 765 support (your task 113, which will emit
`translationof` for 765) is separately tracked on libcat's side if/when it lands.
