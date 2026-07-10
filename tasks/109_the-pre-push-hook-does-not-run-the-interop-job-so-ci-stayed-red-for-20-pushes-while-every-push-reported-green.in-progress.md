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
