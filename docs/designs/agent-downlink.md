# agent-downlink

`agent-downlink` gathers AI-agent session records from several personal machines into one
client-side-encrypted archive, and keeps a decrypted local mirror of that archive for other
tools to read. It is transport and archive only. Dashboards and transcript-review workflows
are separate consumers that read the mirror.

Decisions and their rejected alternatives live in `docs/adr/` and `docs/decisions/`. This
document describes the design as it stands.

## Goals

- Collect records from machines that are not networked to each other and are never presumed
  to be on at the same time.
- Nothing to host: no server, no always-on machine.
- The storage provider holds only ciphertext.
- A compromised or lost machine cannot destroy the archive, and its access can be revoked.
- Easy to use: one command to set up a machine, then a timer does the rest.

## Architecture

```
 each machine                        object storage (ciphertext)            each reader machine
 agent's native records ──push──▶  bucket/<machine>/<tool>/...  ──pull──▶  mirror/<machine>/<tool>/...
                                     versioned; old versions                 plain files, native layout
                                     kept 90 days                            ▲ consumers read here
```

The tool is a single Go binary that wraps rclone. rclone's `crypt` remote encrypts file
contents and names before upload to a Backblaze B2 bucket. Every machine has its own area of
the bucket and its own encryption password. A machine needs only its own password to push. It
becomes a reader of another machine by being given that machine's password.

Reading the archive takes two independent secrets: a storage key, revocable from the
provider's web console, and the encryption password.

## The layout contract

The mirror's layout is the tool's public interface. Consumers read files in this layout and
never import the tool's code; the tool knows nothing about its consumers.

```
mirror/
  <machine>/
    <tool>/                                  e.g. claude-code
      projects/...                           the tool's native tree, byte for byte
      history.jsonl                          cross-project prompt history, byte for byte
      plugins/installed_plugins/<UTC timestamp>.json
```

Consumers may rely on the following:

- The path is always `<machine>/<tool>/` followed by the tool's native layout, unmodified.
- Every machine appears in the mirror as ordinary files, including the machine the mirror is
  on. There are no symbolic links.
- This tool never removes or renames a file. Records the agent later prunes from its own
  directory remain in the mirror and the bucket.
- A file still being written by an agent may end in a partial line. A later run completes it.
- Consumers read the mirror and never write to it.

The local machine's folder is as fresh as the last run. A consumer that needs live data for
the local machine reads the agent's own directory.

Machine names are lowercase letters, digits, and hyphens, chosen at setup and defaulting to
the hostname. The machine name is the one thing the storage provider sees in plaintext, since
each machine's encrypted area sits under it.

The tool knows where each supported agent keeps its records, so the operator never has to.
Claude Code is the first built-in. An operator can describe a tool that is not yet built in
with a `[[custom_tools]]` table in `config.toml`, without a code change. Records are stored
raw; there is no cross-tool schema.

### Plugin version timeline

For Claude Code, each run stores a copy of `installed_plugins.json` under a UTC-timestamped
name (`20060102T150405Z.json`, which sorts by time), only when its content differs from the
latest stored copy. A consumer joins a
transcript's timestamp against these copies to learn which plugin versions were installed when
the session ran.

## Commands

| Command | Purpose |
|---|---|
| `agent-downlink setup` | Once per machine. Asks for a machine name, the bucket name, and the storage key, generates the machine's encryption password (or accepts an existing one, for a machine that replaces one of the same name), writes the config files, installs the hourly timer, and prints what to save in the password manager. A flag omits the timer for a machine making a single push. |
| `agent-downlink add-machine <name>` | On a reader. Takes another machine's encryption password and makes that machine readable here. |
| `agent-downlink run` | What the timer invokes: the full cycle below. |
| `agent-downlink push` | The cycle's local-copy and push steps, on demand. |
| `agent-downlink pull [--transfers=N]` | The cycle's pull step, on demand. `--transfers=N` overrides the configured copy parallelism for this pull alone, for tuning one pull without editing `config.toml`. |
| `agent-downlink timer install` / `timer remove` | Install or remove the hourly schedule on its own: for a machine set up without it, or one being retired. |
| `agent-downlink status` | Age of each machine's last successful push and pull, the most recent error of any failing step, and the path of the log file. |

### One run

1. Store a dated copy of the plugin manifest if it changed.
2. Copy this machine's records into `mirror/<this-machine>/`.
3. Push `mirror/<this-machine>/` to the bucket.
4. Pull every other readable machine from the bucket into the mirror.

Every copy uses `rclone copy`, which never deletes at the destination.

The push uploads from the mirror copy made in step 2, not from the agent's live directory, so
an upload never reads a file that an agent is writing. Only the local copy in step 2 can meet
a file that changes while it is read. rclone abandons that file, the run records a warning
rather than a failure, and the next run copies it.

## Files on disk

| Path | Contents |
|---|---|
| `~/.config/agent-downlink/` | Created by the tool with mode `0700`. |
| `~/.config/agent-downlink/config.toml` | Non-secret settings: machine name, storage location, mirror path, tools to archive. Written by `setup` with a comment on each field. |
| `~/.config/agent-downlink/rclone.conf` | The storage key and one encryption password per readable machine. Mode `0600`; the tool refuses to run if permissions are looser. rclone is always pointed at this file, so the user's own rclone configuration is never touched. |
| `~/.local/share/agent-downlink/mirror/` | The mirror. The path is configurable so it can sit on an encrypted volume. |
| `~/.local/state/agent-downlink/status.json` | Per step and per machine: last attempt, last success, error text. Consumers may read it. |
| `~/.local/state/agent-downlink/agent-downlink.log` | The history: one line per step per run, plus rclone's full error output for a failed step. |

### config.toml

| Field | Meaning |
|---|---|
| `machine` | This machine's name. |
| `storage` | The rclone path of the bucket, `b2:<bucket>`. Each machine's encrypted area is `<storage>/<machine>`. Tests point it at a local directory. |
| `mirror` | The mirror directory. |
| `rclone` | Absolute path of the rclone binary, found by `setup`. Schedulers run jobs with a minimal `PATH` that may not include the directory rclone was installed to. |
| `tools` | Names of the built-in tools to archive. `setup` writes `["claude-code"]`. The list is explicit so that a tool built in later is not switched on by an upgrade. |
| `transfers` | Optional. Files rclone copies at the same time. Omitted means rclone's own default of 4. Raising it shortens a pull that moves many files; a B2 upload holds up to `transfers` times `--b2-upload-concurrency` chunks in memory, so the trade is deliberate. |
| `checkers` | Optional. Files rclone compares at the same time to decide what to copy. Omitted means rclone's own default of 8. |
| `[[custom_tools]]` | Optional. A tool that is not built in: `name`, `root` (absolute path of its data directory), `paths` (sub-paths of `root` to archive), and optionally `plugin_manifest` (a file under `root` to keep a timeline of). |

Each archived path is copied to `mirror/<machine>/<tool>/<path>`.

### rclone.conf

The storage remote is named `b2`. Each readable machine has a `crypt` remote named
`crypt-<machine>` whose `remote` is `<storage>/<machine>`. The machines a pull reads are the
`crypt-` remotes other than this machine's own.

### Logging

The tool writes its own log file so that the answer to "where are the logs" is the same on
every OS, whatever the scheduler does with a job's output. At about 5 MB the log is renamed to
`agent-downlink.log.1`, replacing any earlier one, and a new log is started. Run by the timer,
the tool prints nothing and relies on the file. Run by hand, it also prints errors to the
terminal.

The log holds file names, which include project paths and session identifiers. It never holds
secrets.

### Handling secrets

Secrets never appear in the log or on a command line; other users on a machine can read a
process's arguments. `setup` generates each encryption password, obscures it by piping it to
`rclone obscure -` on standard input, and writes `rclone.conf` itself rather than passing
passwords to `rclone config create`.

## Components

| Component | Responsibility |
|---|---|
| `config` | Read and write `config.toml` and `rclone.conf`; create the config directory; enforce and check permissions. |
| `rclone` | The only code that executes rclone. Builds argument lists; interprets exit codes and output. |
| `transfer` | The run cycle: local copy, push, pull. |
| `plugintimeline` | Detect a changed plugin manifest and store a dated copy. |
| `scheduler` | Generate and install a systemd user timer or a launchd agent that invokes `run` hourly. |
| `status` | Record step outcomes; render the status report. |
| `runlog` | Append to the log file and rotate it. |
| `setup` | The interactive `setup` and `add-machine` flows, composed from the components above. |

## Failure handling

- Steps fail independently. A failed push does not block pull, and one machine's failed pull
  does not block the others. The run exits non-zero if any step failed, and `status.json`
  records the detail.
- A lock prevents overlapping runs; a run that finds the lock held exits quietly.
- With no network, a run fails fast, records the failure, and prints nothing. The next run
  retries.
- Runs missed while the machine was off or asleep are caught up with a single run. The systemd
  timer uses a calendar schedule with `Persistent=true`. The launchd agent uses
  `StartCalendarInterval`, because `StartInterval` skips runs that fall during sleep.
- Neither scheduler runs while the user is logged out. On Linux the operator can change that
  with `loginctl enable-linger`; a launchd agent cannot. Agents run while the user is logged
  in, and catch-up covers the gap.
- The tool does not alert on staleness, because a machine that is off looks like a machine
  that is broken. `status` shows ages and leaves the judgment to the operator.
- The tool does not detect tampering. Recovery uses the bucket's retained versions and is a
  runbook procedure.
- A replacement machine set up under a previous machine's name and password adds to that
  machine's area and cannot damage it, because nothing deletes.

## Security model

| Threat | Control |
|---|---|
| Bucket leak or provider compromise | Client-side encryption of contents and names. |
| A compromised machine deletes or rewrites the archive | Storage keys without delete capability, plus bucket versioning with old versions kept 90 days. |
| A compromised push-only machine reads other machines' records | Per-machine encryption passwords; a machine holds only the passwords it was given. |
| A lost or stolen machine | Delete its storage key in the provider's web console; plan for up to a day before it stops working. Full-disk encryption protects the mirror and config on the lost disk. |
| Loss of every machine | Master copies of all passwords in the operator's password manager. Without them the archive is unreadable, by design. |

Out of scope: an attacker with live access to a machine already has that machine's plaintext
records. Records are not scrubbed of secrets; encryption is the control. Full-disk encryption
is recommended on readers and not enforced.

## Testing

| Layer | Approach |
|---|---|
| Unit | Table-driven tests against temporary directories. |
| Timer definitions | Generated systemd and launchd files compared with expected files. Installation is verified by hand once per OS. |
| Integration | The real rclone binary with real `crypt` encryption over a local directory standing in for the bucket. No network, no mocks. These tests fail, rather than skip, when rclone is absent. |
| End-to-end | Behind a build tag, against a real scratch B2 bucket. Proves that a no-delete key can push, that an overwrite preserves the prior version, that a file appended to during upload is completed by the next run, and that a second machine can pull and decrypt. |
| CI | Unit and integration layers on Linux and macOS. The end-to-end layer runs only on the operator's machines, since a public repository cannot safely give storage credentials to pull requests from forks. |

Fixtures use placeholder machine names.

## Runbook

Provisioning is documented, not automated, so that no administrative storage credential ever
rests on disk. The runbook covers: provisioning the bucket, lifecycle rule, and per-machine
no-delete keys; adding a machine; adding a reader; retiring a machine; responding to a lost
machine; recovering from tampering; and what cannot be recovered.

## Distribution

`go install github.com/jwp23/agent-downlink@latest`. rclone is the only other prerequisite.
