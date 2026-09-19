# End-to-end suite

This suite runs the tool against a real Backblaze B2 scratch bucket instead of a local
directory standing in for one. It proves:

- A no-delete key can push.
- Overwriting a file preserves the prior version (B2 buckets keep versions).
- A file that is still being appended to when a push runs is completed by the next run.
- A second machine can pull the first machine's pushed files and decrypt them.

It also settles two open questions:

- Whether rclone works with a B2 application key restricted to a file-name prefix
  (`TestPrefixRestrictedKeyCanPush`). Either outcome is a useful result: if it fails, record
  the fallback (a whole-bucket no-delete key, deleted after the retiring machine's one push)
  in a new ADR that supersedes the relevant clause of ADR-002.
- Whether hyphenated machine names work as rclone remote names against B2. Every test here
  uses names of the form `e2e-<unix time>-a`.

## Why this never runs in CI

This repository is public. A public repository cannot safely give storage credentials to
workflows that run against pull requests from forks, so this suite is not part of `go test
./...` or any CI job. It runs only on an operator's own machine, by hand, against a scratch
bucket the operator controls.

## Environment variables

All six are required. A missing one fails the suite immediately, naming the variable — a green
run must mean every question was actually answered. The suite never prints the value of any of
these.

| Variable | Meaning |
|---|---|
| `AGENT_DOWNLINK_E2E_BUCKET` | A scratch bucket with versioning (B2 buckets always keep versions). |
| `AGENT_DOWNLINK_E2E_KEY_ID`, `AGENT_DOWNLINK_E2E_KEY` | An application key for that bucket without `deleteFiles`. |
| `AGENT_DOWNLINK_E2E_PREFIX`, `AGENT_DOWNLINK_E2E_PREFIX_KEY_ID`, `AGENT_DOWNLINK_E2E_PREFIX_KEY` | A machine name, and a no-delete key restricted to the file-name prefix `<that name>/`. |

Export them in your shell; never write them to a file in this repository.

## Creating the scratch bucket and keys

This section is prose, not exact `b2` command-line invocations: the commands need to be
verified against the installed `b2` tool's `--help` output or Backblaze's current
documentation before they are added here. What needs to exist:

- A private scratch bucket, separate from any bucket used for real data. B2 buckets keep file
  versions by default, which this suite relies on.
- An application key scoped to that bucket with list, read, and write capabilities, and
  without the `deleteFiles` capability. This is the key for `AGENT_DOWNLINK_E2E_KEY_ID` /
  `AGENT_DOWNLINK_E2E_KEY`.
- A second application key, also scoped to that bucket and without `deleteFiles`, further
  restricted to the file-name prefix `<machine>/`, where `<machine>` is the value you set as
  `AGENT_DOWNLINK_E2E_PREFIX`. This is the key for `AGENT_DOWNLINK_E2E_PREFIX_KEY_ID` /
  `AGENT_DOWNLINK_E2E_PREFIX_KEY`.

## Running it

```sh
export AGENT_DOWNLINK_E2E_BUCKET=...
export AGENT_DOWNLINK_E2E_KEY_ID=...
export AGENT_DOWNLINK_E2E_KEY=...
export AGENT_DOWNLINK_E2E_PREFIX=...
export AGENT_DOWNLINK_E2E_PREFIX_KEY_ID=...
export AGENT_DOWNLINK_E2E_PREFIX_KEY=...
go test -tags e2e ./e2e/ -v
```

## Cleanup

A no-delete key cannot clean up after itself, so every run leaves a few small encrypted files
under `e2e-<time>-a/` and `e2e-<time>-b/` in the bucket (and under the prefix machine's name,
for the prefix test). Clear them with the bucket's lifecycle rule, or by deleting the scratch
bucket entirely from the B2 web console.
