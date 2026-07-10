# 109 -- the pre-push hook does not run the interop job, so CI stayed red for 20 pushes while every push reported green

Opened 2026-07-09.

The bug fixed in 6d11710 was introduced in 2c28190 and shipped in five releases
(v0.19.0 through v0.23.0) before anyone looked at the CI tab. Every one of those
pushes printed:

```
pre-push: all checks passed
```

That message was true about the checks the hook runs, and useless. The hook's own
doc comment says it mirrors "the static checks of CI's build job" so that "a push
that would fail CI is stopped locally instead" -- but CI has a second job,
`interop`, which installs pymarc/rdflib/bibtexparser/rispy and reads our output
back with them. The hook never runs it.

Worse, the hook *appears* to. `go test ./...` includes `TestInterop`, which does:

```go
if _, err := runInterop(py, script, "check"); err != nil {
    t.Skipf("interop parsers unavailable (%v); ...", err)
}
```

A skip is not a failure, and `go test ./...` prints `ok` for the package. So the
one test that would have caught the regression ran on every push, decided it
could not, and said nothing that `-v` would have to be passed to see.

## Confirmed

The interop suite passes locally once the parsers exist, which is how 6d11710 was
verified before pushing:

```
python3 -m venv venv && venv/bin/pip install pymarc rdflib bibtexparser rispy
INTEROP_PYTHON=venv/bin/python go test -run TestInterop -v .
```

So the hook *could* run it. The question is whether it should.

## The decision this needs

Requiring Python and four pip packages to push a zero-dependency Go library is a
real cost, and this repo has been deliberate about not accumulating dependencies.
Options:

1. **Run interop in the hook when the parsers happen to be importable, and skip
   loudly when they are not.** Cheap, adds no requirement, and turns a silent skip
   into `pre-push: interop SKIPPED (CI will run it)`. Does not stop the bad push,
   but stops the false assurance.
2. **Require the parsers**, documented in the hook's enable instructions, with
   `PREPUSH_SKIP_INTEROP=1` as the escape hatch. Actually prevents the regression;
   taxes every contributor.
3. **Leave the hook alone and fix the feedback loop instead** -- have whatever
   works these tasks check `gh run list` after each push, which is how this was
   finally caught, twenty pushes late.

Option 1 is the floor and should probably happen regardless: the hook's last line
currently overstates what it verified, and the overstatement is the part that
actually misled. Whether to go on to option 2 is a contributor-experience call.

Related: 108, the other half of what the interop test was trying to say.

Leaving pending for Eve.

## Outcome

Option 1 landed in 326a60c. Option 2 is deliberately not taken; see below.

Implementing it corrected the premise of this task. The title says the hook "does
not run the interop job". It does -- `go test ./...` includes `TestInterop`, and
when the parsers are importable it runs and it fails. Proved by reintroducing the
6d11710 bug and running the hook with a venv on `INTEROP_PYTHON`:

```
pre-push: go test
    interop_test.go:101: RDF/XML triples: ours=92 rdflib=90
HOOK EXIT=1
```

The hook stopped that push at the `go test` stage, before reaching anything I had
added. So the coverage was never missing. What was missing was the truth: with no
parsers installed, `TestInterop` calls `t.Skipf`, `go test` prints `ok` for the
package, and the hook concluded `all checks passed`. The hook was not failing to
check. It was failing to admit it had not checked.

That reframing shrank the fix. My first cut added an explicit
`go test -run TestInterop` stage; it never once executed, because `go test ./...`
had already caught the mutant four seconds earlier. Re-running the suite to prove
a thing the suite had just proved is cost without coverage, so it came out. What
remains is a probe and two honest messages:

```
pre-push: interop parsers present (python3); go test will exercise them
...
pre-push: all checks passed
```

```
pre-push: interop will SKIP -- python3 cannot import the third-party parsers:
pre-push:   {"ok": false, "error": "ModuleNotFoundError: No module named 'bibtexparser'"}
pre-push:   CI runs this job regardless. To run it here:
pre-push:   pip install pymarc rdflib bibtexparser rispy  (or set INTEROP_PYTHON)
...
pre-push: all checks passed (interop SKIPPED)
```

The probe reuses `testdata/interop.py check`, which already exits non-zero and
names the first missing module, so the hook adds no new detection logic. It costs
one Python start-up; the parsers-present path re-runs nothing. All three states
were exercised against the real venv: absent parsers (exit 0, loud), present with
correct code (exit 0, quiet), present with the bug reintroduced (exit 1).

The header comment also claimed the hook "mirrors the static checks of CI's build
job", which is what made the interop gap invisible on reading. It now says what it
actually does.

**Option 2 (require the parsers) not taken.** It would have prevented the original
regression outright, but it taxes every contributor of a zero-dependency Go
library with a Python toolchain, and the regression is now caught two other ways:
by `TestCatalogingSourceRepeatedAgencyDescribedOnce`, which is pure Go and needs
no parsers, and by CI. A push that skips interop is now a push that says so. That
is enough for the failure mode this task was opened about. If a second interop
regression ever reaches main, revisit -- the evidence will be here.

**Option 3** happens to be in force already: the loop working these tasks now
checks `gh run list` after each push, which is what surfaced this.
