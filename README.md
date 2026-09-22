# agent-downlink

`agent-downlink` gathers AI-agent session records from several personal machines into one
client-side-encrypted archive, and keeps a decrypted local mirror of that archive for other
tools to read. It is transport and archive only. Dashboards and transcript-review workflows are
separate consumers that read the mirror.

```
 each machine                        object storage (ciphertext)            each reader machine
 agent's native records ──push──▶  bucket/<machine>/<tool>/...  ──pull──▶  mirror/<machine>/<tool>/...
                                     versioned; old versions                 plain files, native layout
                                     kept 90 days                            ▲ consumers read here
```

See `docs/designs/agent-downlink.md` for the full design and `docs/adr/` for the decisions
behind it.

## Prerequisites

- [rclone](https://rclone.org/install/), the only runtime dependency; the tool wraps it.
- Go, to build or install the binary.
- A Backblaze B2 account.
- Linux or macOS. There is no Windows support.

## Install

```sh
go install github.com/jwp23/agent-downlink@latest
```

## Quick start

1. Provision a bucket and a machine's storage key (see `docs/runbook.md`, sections "Provision
   the bucket" and "Create a machine's storage key").
2. On the first machine, run `agent-downlink setup` and answer its prompts.
3. Save what it prints in your password manager now — the encryption password is shown once.
4. Run `agent-downlink run` to make the first transfer, then `agent-downlink status` to see
   that it succeeded.
5. On a second machine, run `agent-downlink setup` there too, then
   `agent-downlink add-machine <first machine's name>` with that machine's encryption password.
   The next run pulls its records into the mirror.

## Commands

| Command | Purpose |
|---|---|
| `agent-downlink setup [--no-timer]` | Once per machine. Asks for a machine name, the bucket name, and the storage key, generates the machine's encryption password (or accepts an existing one, for a machine that replaces one of the same name), writes the config files, installs the hourly timer, and prints what to save in the password manager. `--no-timer` skips the timer, for a machine making a single push. |
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

## Reading the mirror

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

The local machine's folder is as fresh as the last run. A consumer that needs live data for the
local machine reads the agent's own directory.

### Plugin version timeline

For Claude Code, each run stores a copy of `installed_plugins.json` under a UTC-timestamped
name (`20060102T150405Z.json`, which sorts by time), only when its content differs from the
latest stored copy. A consumer joins a transcript's timestamp against these copies to learn
which plugin versions were installed when the session ran.

A consumer may also read `~/.local/state/agent-downlink/status.json`, described below, to learn
the age of each machine's data.

## Where things live

| Path | Contents |
|---|---|
| `~/.config/agent-downlink/` | Created by the tool with mode `0700`. |
| `~/.config/agent-downlink/config.toml` | Non-secret settings: machine name, storage location, mirror path, tools to archive. Written by `setup` with a comment on each field. |
| `~/.config/agent-downlink/rclone.conf` | The storage key and one encryption password per readable machine. Mode `0600`; the tool refuses to run if permissions are looser. rclone is always pointed at this file, so the user's own rclone configuration is never touched. |
| `~/.local/share/agent-downlink/mirror/` | The mirror. The path is configurable so it can sit on an encrypted volume. |
| `~/.local/state/agent-downlink/status.json` | Per step and per machine: last attempt, last success, error text. Consumers may read it. |
| `~/.local/state/agent-downlink/agent-downlink.log` | The history: one line per step per run, plus rclone's full error output for a failed step. |

### `config.toml`

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

A worked example, adding a second tool alongside the built-in `claude-code`:

```toml
# This machine's name. Its encrypted area in the bucket sits under this name.
machine = 'workstation'
# The rclone path of the bucket: b2:<bucket-name>.
storage = 'b2:<bucket>'
# Directory holding the decrypted mirror of every readable machine. Move it to an encrypted volume if you have one.
mirror = '/home/user/.local/share/agent-downlink/mirror'
# Absolute path of the rclone binary. Schedulers run with a minimal PATH. Empty means search PATH.
rclone = '/usr/bin/rclone'
# Built-in agent tools whose records are archived from this machine.
tools = ['claude-code']

# Agent tools that are not built in.
[[custom_tools]]
# Folder name for this tool in the mirror: lowercase letters, digits, and hyphens.
name = 'another-agent'
# Absolute path of the tool's data directory.
root = '/home/user/.another-agent'
# Sub-paths of root to archive. Each is copied to <mirror>/<machine>/<name>/<path>.
paths = ['sessions']
# Optional file under root; a dated copy is kept whenever its content changes.
plugin_manifest = 'plugins.json'
```

This example archives `~/.another-agent/sessions` to `mirror/workstation/another-agent/sessions`
and keeps a dated timeline of `~/.another-agent/plugins.json` whenever it changes.

## Security model

| Threat | Control |
|---|---|
| Bucket leak or provider compromise | Client-side encryption of contents and names. |
| A compromised machine deletes or rewrites the archive | Storage keys without delete capability, plus bucket versioning with old versions kept 90 days. |
| A compromised push-only machine reads other machines' records | Per-machine encryption passwords; a machine holds only the passwords it was given. |
| A lost or stolen machine | Delete its storage key in the provider's web console; plan for up to a day before it stops working. Full-disk encryption protects the mirror and config on the lost disk. |
| Loss of every machine | Master copies of all passwords in the operator's password manager. Without them the archive is unreadable, by design. |

Full-disk encryption is recommended on every reader machine, most of all on laptops that leave
the house, but the tool does not check for or require it.

Out of scope: an attacker with live access to a machine already has that machine's plaintext
records. Records are not scrubbed of secrets; encryption is the control. Full-disk encryption is
recommended on readers and not enforced.

## Development

Install rclone and golangci-lint before running tests (integration tests fail without rclone).

```sh
go build .                          # builds ./agent-downlink
gofmt -l .                          # must print nothing
go vet ./...
golangci-lint run                   # config in .golangci.yml
go test ./...                       # unit and integration tests
go test -tags e2e ./e2e/ -v         # end-to-end against a real B2 bucket (see e2e/README.md)
```

The first four commands run in the pre-commit hook. The e2e suite requires real B2 credentials
and never runs in CI. CI runs the same checks on Linux and macOS.
