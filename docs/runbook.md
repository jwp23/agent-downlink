# Operator runbook

`agent-downlink` provisions nothing for you: no administrative storage credential ever rests on
disk (see `docs/designs/agent-downlink.md`, "Runbook"). Each procedure below is something you
run by hand, once, from your own machine.

Every `rclone` command here was verified against `rclone --help`, `rclone help backend b2`,
`rclone help flags`, or the named subcommand's own `--help`, using the rclone version installed
while this runbook was written (`rclone v1.75.1`). Every command carries its own source note.
The web-console steps are described in general terms, not as a click-by-click walkthrough, since
Backblaze's own console changes over time; consult Backblaze's current documentation for the
exact screen.

Placeholders: `<bucket>` is your bucket's name, `<machine>` is a machine name (lowercase
letters, digits, and hyphens — `workstation` and `laptop` are used as examples), `<key ID>` and
`<application key>` are one B2 application key's two parts.

## 1. Provision the bucket

1. In the B2 web console, create a new bucket and set it private. B2 buckets keep every
   superseded version of a file by default; nothing further is needed to turn that on.
2. Set a lifecycle rule so a superseded version is eventually deleted rather than kept forever
   (ADR-002 calls for keeping it 90 days). Set this in the B2 web console's lifecycle settings
   for the bucket, not with `rclone backend lifecycle`: that command takes the account's master
   application key on its command line, and another local account on your machine can read a
   process's arguments. The per-machine key from step 2 below is deliberately too restricted to
   change a bucket's settings, so it cannot be used here instead.

   B2 hides a file's previous version the moment it is overwritten; the "days from hiding to
   deleting" setting is what eventually expires a hidden version. Confirm it took effect by
   reopening the bucket's lifecycle settings and checking the value shown.

## 2. Create a machine's storage key

Each live machine's key needs list, read, and write capabilities on the bucket, and must
**not** have delete capability (ADR-002): a compromised machine must not be able to destroy or
rewrite the archive, only versioning-protected overwrites. ADR-002 records that a key without
delete cannot be created in the B2 web console — only the `b2` command-line tool or the B2 API
can create one.

This runbook cannot give you the exact `b2` command here: the `b2` tool is not installed in the
environment this runbook was written in, and there was no way to check its current flags against
Backblaze's live documentation either. Before running anything, verify the exact invocation
against `b2 create-key --help` (or whatever the installed `b2` version's key-creation command is
called) or Backblaze's current command-line tools documentation. What the key must have when you
create it:

- Scoped to `<bucket>` only.
- Capabilities: list, read, and write. Not delete.

Save the resulting key ID and application key in your password manager under `<machine>`; you
will paste them into `agent-downlink setup` next.

## 3. Add a machine

1. Install `agent-downlink` (`go install github.com/jwp23/agent-downlink@latest`) and make sure
   `rclone` is installed.
2. Run `agent-downlink setup`. It asks for a machine name (default: the hostname), the bucket
   name, the storage key from step 2, and an encryption password — press Enter to generate one.
   It writes the config files and installs the hourly timer.
3. Save what it prints: if it generated a password, save the machine name and the encryption
   password in your password manager now. It is not shown again, and without it this machine's
   own records cannot be read by anyone, including you.
4. Run `agent-downlink run` to make the first transfer.
5. Run `agent-downlink status` and check the `push` step shows a recent `LAST SUCCESS` and state
   `ok`.

## 4. Add a reader

On the machine that should be able to read another machine's records:

1. Run `agent-downlink add-machine <machine>`, giving `<machine>`'s encryption password from
   your password manager.
2. It prints that `<machine>` is now readable here.
3. The next `agent-downlink run` (or `agent-downlink pull`) pulls `<machine>`'s records into the
   mirror. Check with `agent-downlink status`.

## 5. Retire a machine

At the time this was written, whether a B2 application key restricted to a file-name prefix
(`<machine>/`) works with rclone had not yet been confirmed — the end-to-end suite that answers
this (`agent-downlink-eu4.9`) had not yet been run against a real bucket. Until an ADR records
that a prefix-restricted key works, use the fallback: a whole-bucket key without delete,
deleted immediately after the machine's one final push. If a later ADR confirms the
prefix-restricted key works, prefer scoping the key to `<machine>/` instead, since it cannot
touch any other machine's data even briefly.

1. Create a storage key exactly as in section 2 (whole-bucket, list/read/write, no delete).
2. On the retiring machine, run `agent-downlink setup --no-timer`, giving that key and the
   machine's *existing* encryption password (so it replaces the same machine's area rather than
   starting a new one) — `setup` asks to confirm before replacing an existing configuration.
3. Run `agent-downlink push` to send anything not yet uploaded.
4. Delete the key you created in step 1 from the B2 web console.
5. If this machine ever had the hourly timer installed, run `agent-downlink timer remove`.

## 6. A machine is lost or stolen

1. In the B2 web console, delete that machine's storage key. Backblaze does not document how
   quickly a deleted key stops working; ADR-002 plans for up to a day.
2. That machine's encryption password only ever protected its own area of the archive — it
   cannot be used to read any other machine's records.
3. Full-disk encryption, if it was enabled on the lost machine, protects the mirror and the
   `~/.config/agent-downlink` files on that disk from someone who has the disk but not the
   unlock passphrase. It does not undo a compromise that happened while the machine was
   unlocked.

## 7. Recover from tampering

The tool does not detect tampering; this is a runbook procedure using the bucket's retained
versions.

1. List a file's versions:

   ```sh
   rclone --config <copy of rclone.conf> lsl crypt-<machine>: --b2-versions
   ```

   Verified against: `rclone lsl --help` and `rclone help backend b2` (`--b2-versions`:
   "Include old versions in directory listings").

2. Restore the archive as of a specific time into a **fresh, empty directory** — never back
   into the live mirror, so a bad restore cannot be mistaken for current data:

   ```sh
   rclone --config <copy of rclone.conf> copy crypt-<machine>: <fresh directory> --b2-version-at 2024-01-15T00:00:00Z
   ```

   Verified against: `rclone copy --help`, and `rclone help flags` / `rclone help backend b2`
   for `--b2-version-at` ("Show file versions as they were at the specified time"). `crypt-`
   remotes in the tool's `rclone.conf` wrap a `b2` remote (`internal/config/rcloneconf.go`), and
   rclone applies a backend flag like `--b2-version-at` to every remote of that backend type
   used in the command, including one reached through `crypt`, not only a remote named `b2:`
   directly — this environment has no live B2 bucket to exercise end to end, so confirm the
   restored content is what you expect before relying on it.

Always use `--config` pointing at a **copy** of `~/.config/agent-downlink/rclone.conf`, never
the live file, so a mistake here cannot affect the running tool. Putting a password on the
command line is never needed for this: the config file already carries every encryption
password it knows.

## 8. What cannot be recovered

- A lost encryption password. By design, nothing but that password and a copy of it in the
  operator's password manager can decrypt that machine's area.
- A version of a file older than the lifecycle rule's retention window (90 days here). B2 has
  already deleted it.
- Records an agent pruned from its own directory before any run had copied them into the
  mirror. The tool only ever preserves what it has already seen.

## 9. The scheduler while logged out

Neither scheduler runs while the user is logged out.

- On Linux, `loginctl enable-linger` keeps a user's session (and its systemd user timers)
  running after logout. Verified against `loginctl --help` and its man page (`enable-linger`:
  "a user manager is spawned for the user at boot and kept around after logouts").
- A launchd agent cannot be made to run while its user is logged out; launchd's per-user
  `gui/<uid>` domain only exists while that user has a session.
- Either way, catch-up covers a gap: the systemd timer uses `Persistent=true` and the launchd
  agent uses `StartCalendarInterval`, both of which run once for a missed schedule as soon as
  the user is back, rather than skipping it (`internal/scheduler/scheduler.go`).

The log is always at `~/.local/state/agent-downlink/agent-downlink.log`, regardless of what the
scheduler does with a job's own output. Run by the timer, the tool prints nothing and relies
entirely on this file; run by hand, it also prints errors to the terminal.
