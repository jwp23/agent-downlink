# ADR-005: Mutation testing runs whole-module on every pull request

## Context

The test suite is small and fast: about 6,200 lines of Go in nine packages, and an uncached
`go test ./...` finishes in under four seconds, rclone included. Line coverage says which
statements the tests execute; it says nothing about whether the tests would notice a wrong
result. Mutation testing measures that directly by changing one operator at a time and
checking that some test fails.

A measurement run with gremlins v0.6.0 over the whole module found 286 mutants and finished
in about two minutes on a developer machine: 247 killed, 14 lived, 25 not covered. Two runs
agreed exactly. The integration tests, which run the real rclone binary over a local
directory, produced no noise once one artifact was removed: gremlins derives its per-mutant
timeout from the duration of its own coverage pass, and that pass hits Go's test cache, so
a warm cache measured the baseline at 56 ms and turned every rclone-backed test into a
phantom timeout. `GOFLAGS=-count=1` bypasses the cache and removed all of them.

## Decision

- Adopt gremlins v0.6.0, installed by commit hash, as a required CI check that mutates the
  whole module on every pull request. No diff scoping, no scheduled sweep, no sharding: the
  whole run costs about as much as the lint job.
- The bar is zero unexcluded mutants in any non-KILLED status. gremlins can only fail a run
  on a kill percentage, so a small program, `tools/gremlinsgate`, reads gremlins' JSON
  report and fails on any mutant that is LIVED, NOT COVERED, TIMED OUT or NOT VIABLE unless
  a reviewed allowlist names it.
- Scope lives in one file, `.gremlins.yaml`, shared by CI and local runs. Excluded: `e2e/`
  (behind a build tag, so gremlins sees no tests for it), `tools/` (development utilities),
  and `main.go` (process bootstrap that the code style already keeps out of tests). Every
  other file, `dispatch` and the `cmd_*.go` files included, is in scope. The
  `increment-decrement` mutator is disabled module-wide: it turns an ordinary counted loop
  into an infinite one, which can only ever time out.
- A mutant that no test can distinguish from the original is an equivalent mutant, not a
  gap. Each one is recorded in `.gremlins-equivalents.json` with a written proof, and
  `gremlinsgate` reports how many entries matched so a stale entry is visible.
- The gate is enforced from its first run. The survivors found by the measurement run are
  triaged before the CI job lands, so the job is green on its first run and a failure means
  something.
- Every run, local or CI, sets `GOFLAGS=-count=1`.

## Trade-offs

- A diff-scoped PR gate plus a weekly full sweep is how projects with hour-long mutation
  runs stay usable. At two minutes whole-module, the layering would add a scheduled
  workflow, a survivor-capture pipeline, and diff configuration for nothing, and a diff
  scope misses a mutant killed only by a test in another package. Rejected; gremlins has
  `--diff` if wall-clock ever forces a retreat.
- gomu applies a richer operator set and is actively developed, but is young and produces
  several times as many mutants per package, which at adoption time is more to burn down
  than it is insight. Rejected for now.
- A kill-rate threshold is what gremlins offers natively and needs no extra tool. It lets
  any individual gap hide inside a percentage, and the percentage drifts as code is added.
  Rejected in favour of the per-mutant bar and allowlist.
- Landing the CI job first as non-blocking and tightening it later was rejected: a gate that
  cannot fail teaches people to ignore it, and the burn-down is small enough to finish first.
- Accepted cost: every pull request pays about two minutes of CI, in parallel with the other
  jobs; the pre-commit hook does not run it. Each survivor a change introduces has to be
  killed or proven equivalent before merge.
