# End-to-end suite

This suite runs the tool against a real Backblaze B2 scratch bucket instead of a local
directory standing in for one. It proves:

- A no-delete key can push.
- Overwriting a file preserves the prior version (B2 buckets keep versions).
- Content appended to a file between two runs is fully copied by the next run.
- A second machine can pull the first machine's pushed files and decrypt them.

It settled two questions the design left open, both affirmatively, against B2 with rclone
v1.75.1. The tests stay in the suite so a later rclone or a change at B2 cannot quietly take
either answer back:

- A B2 application key restricted to the file-name prefix `<machine>/` pushes correctly
  (`TestPrefixRestrictedKeyCanPush`), so ADR-002's prefix-scoped retirement key stands and the
  runbook uses it. Had it failed, the fallback (a whole-bucket no-delete key, deleted after the
  retiring machine's one push) would have needed a new ADR superseding that clause of ADR-002.
- Hyphenated machine names work as rclone remote names against B2. Every test here uses names
  of the form `e2e-<unix time>-a`, which exercises this on every run.

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
| `AGENT_DOWNLINK_E2E_KEY_ID`, `AGENT_DOWNLINK_E2E_KEY` | An application key for that bucket with `listFiles,readFiles,writeFiles` and without `deleteFiles`. |
| `AGENT_DOWNLINK_E2E_PREFIX`, `AGENT_DOWNLINK_E2E_PREFIX_KEY_ID`, `AGENT_DOWNLINK_E2E_PREFIX_KEY` | A machine name, and a no-delete key restricted to the file-name prefix `<that name>/`. |

Export them in your shell; never write them to a file in this repository.

## Creating the scratch bucket and keys

Use a private scratch bucket, separate from any bucket holding real data. B2 keeps file
versions by default, which this suite relies on.

The commands below are the `b2` tool's current noun-verb form; 3.x releases spell the same three
`b2 authorize-account`, `b2 create-bucket`, and `b2 create-key`. Confirm against what you have
installed with `b2 version` and `b2 key create --help`.

```sh
export B2_ACCOUNT_INFO="$(mktemp -d)/account_info"
b2 account authorize
b2 bucket create <bucket> allPrivate
b2 key create --bucket <bucket> e2e-suite listFiles,readFiles,writeFiles
b2 key create --bucket <bucket> --name-prefix <machine>/ e2e-suite-prefix listFiles,readFiles,writeFiles
rm -rf "$(dirname "$B2_ACCOUNT_INFO")"
```

Give `b2 account authorize` nothing on the command line. It prompts for the master key ID and
reads the key without echoing it, so neither reaches your shell history or a process's
arguments, where another local account could read them. Creating a key needs the master key's
`writeKeys` capability, which neither of these two keys has.

`b2 account authorize` caches the key it authorized with in a SQLite file, `~/.b2_account_info`
unless `B2_ACCOUNT_INFO` says otherwise, and leaves it there; the temporary path above and the
`rm` that follows keep the master key from resting on disk. `b2 account clear` is not a
substitute — it empties the cache without removing the file (Backblaze advisory
GHSA-8wr4-2wm6-w3pr).

The first key is `AGENT_DOWNLINK_E2E_KEY_ID` / `AGENT_DOWNLINK_E2E_KEY` and the second is
`AGENT_DOWNLINK_E2E_PREFIX_KEY_ID` / `AGENT_DOWNLINK_E2E_PREFIX_KEY`; `<machine>` is the value
you set as `AGENT_DOWNLINK_E2E_PREFIX`, and the trailing slash on the prefix is what confines
that key to the machine's own area. Each `key create` prints the key ID and the application key,
the only time the key itself is ever shown — put both straight into your password manager.

`listFiles` is what makes a key able to push at all: rclone lists the destination before
copying, and a key without it gets `401 unauthorized` from B2 on every run. A key missing it
fails every test in this suite, including the two that use no prefix restriction — so read a
suite-wide 401 as a key problem, not as an answer to either question above. `deleteFiles` is
absent on purpose: the suite's first claim is that a no-delete key can push. `listBuckets` is
not needed, because a key scoped to one bucket carries that bucket's name and ID in its
authorization response, which is where rclone reads them. That last one does mean the `b2` tool
will not authorize with these keys — `b2 account authorize` itself requires `listBuckets`.

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
