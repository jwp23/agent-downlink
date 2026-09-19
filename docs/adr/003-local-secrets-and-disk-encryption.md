# ADR-003: Local secrets in 0600 files; disk encryption recommended, not enforced

## Context

Pushes and pulls run unattended on a timer, so each machine must read its storage credential
and encryption keys without prompting. Reader machines also hold a decrypted mirror of every
machine's transcripts, so a stolen reader exposes the whole archive, not just its own data.
Full-disk encryption is the only control for that case, and not every machine has it:
retrofitting it onto an installed Linux system usually means backup, reinstall, and restore.

## Decision

Secrets live in a config file with `0600` permissions in the user's home directory, the same
model as SSH private keys. The tool refuses to run if the permissions are looser. Master
copies of the keys belong in the operator's password manager.

Full-disk encryption is recommended on every reader machine, most of all on laptops that
leave the house, but the tool does not check for or require it. Running a reader on an
unencrypted disk is a risk the operator accepts per machine; the exposure is physical theft,
not remote attack.

The mirror location is configurable so it can be moved onto an encrypted volume without a
redesign. The response to a lost machine is to delete its storage key from the provider's web
console.

## Trade-offs

- OS keychains (macOS Keychain, Secret Service) encrypt secrets at rest, but add a second
  password to remember and differ per OS. Any process running as the user can already read
  `~/.claude` in plaintext, so a `0600` file matches the sensitivity of the data. Rejected.
- A password-manager CLI at runtime needs a long-lived token on disk for unattended use, which
  is a `0600` file with extra steps. Rejected.
- Enforcing full-disk encryption would block the tool on machines where retrofitting it is
  impractical. Rejected in favor of a documented recommendation.
- An encrypted mirror directory unlocked at login (gocryptfs with `pam_mount`) is a documented
  pattern on Linux and would be frictionless, but it means editing PAM configuration, where a
  mistake can lock out login. fscrypt is not an option on btrfs. Left to the operator, enabled
  by the configurable mirror path.
