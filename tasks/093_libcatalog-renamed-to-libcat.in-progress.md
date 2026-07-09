# 093: Sibling libcatalog renamed to libcat

The sibling framework repo was renamed `libcatalog` -> `libcat` (libcat
tasks/162): GitHub repo `freeeve/libcat` (old URL redirects), Go module paths
`github.com/freeeve/libcat{,/backend,/hugo}` as of the lockstep v0.25.0
release, and the local checkout is now `~/libcat`.

No code or go.mod changes needed here (the dependency direction is
libcat -> libcodex). Only prose mentions to update, next time each file is
touched anyway:

- `sru/live_test.go:13` -- comment
- `bibframe/roundtrip_fields_test.go:43` -- comment
- `bibframe/lossgate_test.go:16` -- comment
- `rdf/corpus_bench_test.go:14` -- comment
- `docs/bibframe_m2b_audit.md:218` -- prose

Done task files keep the old name as historical record.

(Unrelated housekeeping noticed while filing: two task files share number 091.)
