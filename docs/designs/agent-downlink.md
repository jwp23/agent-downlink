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

Supporting another agent tool means adding a source path and a `<tool>` name. Records are
stored raw; there is no cross-tool schema.

### Plugin version timeline

For Claude Code, each run stores a copy of `installed_plugins.json` under a UTC-timestamped
name, only when its content differs from the latest stored copy. A consumer joins a
transcript's timestamp against these copies to learn which plugin versions were installed when
the session ran.

## Commands

| Command | Purpose |
|---|---|
| `agent-downlink setup` | Once per machine. Asks for a machine name and storage key, generates the machine's encryption password, writes the config files, installs the hourly timer, and prints what to save in the password manager. A flag omits the timer for a machine making a single push. |
| `agent-downlink add-machine <name>` | On a reader. Takes another machine's encryption password and makes that machine readable here. |
| `agent-downlink run` | What the timer invokes: the full cycle below. |
| `agent-downlink push` | The cycle's local-copy and push steps, on demand. |
| `agent-downlink pull` | The cycle's pull step, on demand. |
| `agent-downlink status` | Age of each machine's last successful push and pull, and any recorded errors. |

### One run

1. Store a dated copy of the plugin manifest if it changed.
2. Copy this machine's records into `mirror/<this-machine>/`.
3. Push `mirror/<this-machine>/` to the bucket.
4. Pull every other readable machine from the bucket into the mirror.

Every copy uses `rclone copy`, which never deletes at the destination.

## Files on disk

| Path | Contents |
|---|---|
| `~/.config/agent-downlink/config.toml` | Non-secret settings: machine name, mirror path, sources to push. Written by `setup` with a comment on each field. |
| `~/.config/agent-downlink/rclone.conf` | The storage key and one encryption password per readable machine. Mode `0600`; the tool refuses to run if permissions are looser. rclone is always pointed at this file, so the user's own rclone configuration is never touched. |
| `~/.local/share/agent-downlink/mirror/` | The mirror. The path is configurable so it can sit on an encrypted volume. |
| `~/.local/share/agent-downlink/status.json` | Per step and per machine: last attempt, last success, error text. Consumers may read it. |

## Components

| Component | Responsibility |
|---|---|
| `config` | Read and write `config.toml`; enforce and check file permissions. |
| `rclone` | The only code that executes rclone. Builds argument lists; interprets exit codes and output. |
| `transfer` | The run cycle: local copy, push, pull. |
| `plugintimeline` | Detect a changed plugin manifest and store a dated copy. |
| `scheduler` | Generate and install a systemd user timer or a launchd agent that invokes `run` hourly. |
| `status` | Record step outcomes; render the status report. |
| `setup` | The interactive `setup` and `add-machine` flows, composed from the components above. |

## Failure handling

- Steps fail independently. A failed push does not block pull, and one machine's failed pull
  does not block the others. The run exits non-zero if any step failed, and `status.json`
  records the detail.
- A lock prevents overlapping runs; a run that finds the lock held exits quietly.
- With no network, a run fails fast, records the failure, and prints nothing. The next run
  retries.
- The timer definitions request catch-up of runs missed while the machine was off or asleep.
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
