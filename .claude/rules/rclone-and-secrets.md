---
paths:
  - "internal/rclone/**"
  - "internal/config/**"
  - "internal/setup/**"
  - "cmd_setup.go"
---
# rclone and Secrets

- Every rclone invocation passes `--config` pointing at the tool's own
  `~/.config/agent-downlink/rclone.conf` and uses the absolute rclone path from `config.toml`.
  The operator's own rclone configuration is never read or written.
- The only transfer verb is `copy`. Never build an argument list containing `sync`, `move`,
  `delete`, `purge`, `rmdir`, `cleanup`, or `dedupe`.
- Obscure a password by piping it to `rclone obscure -` on standard input. Write `rclone.conf`
  directly; never pass a password to `rclone config create` or to any argument list.
- `rclone.conf` is mode `0600` in a `0700` directory. Refuse to run on looser permissions.
- The log and `status.json` may carry file names and rclone's error output. They must never
  carry a storage key, a password, or its obscured form.
- Read secrets with `term.ReadPassword`; fall back to a plain line only when standard input is
  not a terminal.
