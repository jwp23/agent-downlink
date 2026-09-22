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

Each live machine's key needs exactly the capabilities `listFiles`, `readFiles`, and
`writeFiles` on the bucket, and must **not** have `deleteFiles` (ADR-002): a compromised machine
must not be able to destroy or rewrite the archive, only versioning-protected overwrites.
ADR-002 records that a key without delete cannot be created in the B2 web console — only the
`b2` command-line tool or the B2 API can create one.

`listFiles` is not optional. Every transfer is an `rclone copy`, and rclone lists the
destination before copying anything; B2 answers that listing from a key without `listFiles` with
`401 unauthorized` and an empty message, which rclone reports as `Unknown 401  (401
unauthorized)`. `listBuckets` is not needed: a key scoped to one bucket carries that bucket's
name and ID in its authorization response, which is where rclone reads them.

```sh
export B2_ACCOUNT_INFO="$(mktemp -d)/account_info"
b2 account authorize
b2 key create --bucket <bucket> <machine> listFiles,readFiles,writeFiles
rm -rf "$(dirname "$B2_ACCOUNT_INFO")"
```

These are the `b2` tool's current noun-verb commands; 3.x releases spell them
`b2 authorize-account` and `b2 create-key`. Confirm against what you have installed with
`b2 version` and `b2 key create --help`.

Give `b2 account authorize` nothing on the command line. It prompts for the master key ID and
reads the key without echoing it, so neither reaches your shell history or a process's
arguments, where another local account could read them. Creating a key needs the master key's
`writeKeys` capability, which the machine key deliberately does not have.

`b2 account authorize` caches the key it authorized with in a SQLite file, `~/.b2_account_info`
unless `B2_ACCOUNT_INFO` says otherwise, and leaves it there. Pointing that at a fresh private
directory and deleting the directory afterwards is what keeps the master key from resting on
disk, the same reason section 1 sets the lifecycle rule in the web console rather than from the
command line. Do not reach for `b2 account clear` instead: it empties the cache but does not
remove the file (Backblaze advisory GHSA-8wr4-2wm6-w3pr).

`key create` prints the key ID and the application key, the only time the key itself is ever
shown. Save both in your password manager under `<machine>`; you will paste them into
`agent-downlink setup` next.

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

A B2 application key restricted to the file-name prefix `<machine>/` pushes correctly with
rclone: the end-to-end suite confirmed it against a real bucket, so scope the retiring machine's
key to its own prefix rather than to the whole bucket. Such a key cannot touch another machine's
data even briefly, which is why ADR-002 calls for it.

1. Create a storage key as in section 2, restricted to the file-name prefix `<machine>/` as well
   as to the bucket. The trailing slash is what confines the key to that machine's area.

   ```sh
   export B2_ACCOUNT_INFO="$(mktemp -d)/account_info"
   b2 account authorize
   b2 key create --bucket <bucket> --name-prefix <machine>/ <machine>-retire listFiles,readFiles,writeFiles
   rm -rf "$(dirname "$B2_ACCOUNT_INFO")"
   ```
2. On the retiring machine, run `agent-downlink setup --no-timer`, giving that key and the
   machine's *existing* encryption password (so it replaces the same machine's area rather than
   starting a new one) — `setup` asks to confirm before replacing an existing configuration.
3. Run `agent-downlink push` to send anything not yet uploaded.
4. Delete both keys now that the retirement key has taken over: the original key from section 2
   and the retirement key from step 1. Leaving the original key alive defeats the point of
   retiring the machine — it still has `writeFiles` on the whole bucket. Neither key appears on
   the web console's App Keys page — that page only lists keys created there — so delete both
   the same way they were made:

   ```sh
   export B2_ACCOUNT_INFO="$(mktemp -d)/account_info"
   b2 account authorize
   b2 key delete <original key ID>
   b2 key delete <retirement key ID>
   rm -rf "$(dirname "$B2_ACCOUNT_INFO")"
   ```

   3.x releases spell this `b2 delete-key`. Confirm against what you have installed with
   `b2 key delete --help`.
5. If this machine ever had the hourly timer installed, run `agent-downlink timer remove`.

## 6. A machine is lost or stolen

1. Delete that machine's storage key. It was made with `b2 key create` (section 2), so it will
   not appear on the web console's App Keys page — that page only lists keys created there —
   delete it from the command line instead:

   ```sh
   export B2_ACCOUNT_INFO="$(mktemp -d)/account_info"
   b2 account authorize
   b2 key delete <key ID>
   rm -rf "$(dirname "$B2_ACCOUNT_INFO")"
   ```

   Backblaze does not document how quickly a deleted key stops working; ADR-002 plans for up to
   a day.
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
