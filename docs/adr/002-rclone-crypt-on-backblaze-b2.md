# ADR-002: rclone crypt on Backblaze B2

## Context

ADR-001 calls for client-side-encrypted object storage with a per-machine key. We need a tool
and a provider. Requirements: a compromised machine must not be able to destroy or silently
rewrite the archive; a lost machine's access must be revocable; session files grow for days
and are pushed hourly; transcripts pruned locally by Claude Code must stay in the archive;
the system must be easy to operate.

## Decision

Use `rclone copy` through an rclone `crypt` remote, one crypt password per machine, onto a
single Backblaze B2 bucket laid out as `<machine>/<tool>/...` (for example
`workstation/claude-code/projects/...`). Files are stored raw in each tool's native tree; there is
no normalized cross-tool schema.

Tamper resistance comes from two things together: B2 application keys created without the
`deleteFiles` capability, and bucket versioning with a lifecycle rule keeping superseded
versions for 90 days. An overwrite by a compromised machine leaves the prior version
recoverable (`--b2-version-at`) inside that window.

Each live machine holds one B2 key: read and write on the whole bucket, no delete. A retiring
machine gets a key scoped to its own prefix, deleted after its one push.

## Trade-offs

- restic uploads only the appended tail of a growing file and has content-addressed integrity,
  but it is a snapshot backup tool: a transcript pruned at the source vanishes from the latest
  snapshot, so an archive must be reassembled across snapshots. It also leaves hidden lock
  files under a no-delete key and cannot read old bucket versions. Rejected.
- kopia has first-class object-lock support but the same snapshot mismatch. Rejected.
- rclone re-uploads a growing file whole on every run. A 50 MB multi-day session produces
  about 1 GB/day of superseded versions until the lifecycle rule expires them. Accepted;
  B2 storage at this scale costs pennies.
- crypt authenticates each file but not the archive as a whole, so a rollback of one file to
  an older version is not detected by rclone itself. Accepted.
- A crypt password cannot be rotated without re-uploading that machine's data. Accepted; the
  data is a few GB.
- Cloudflare R2 has no write-without-delete permission, no object versioning, and no
  prefix-scoped permanent keys. Rejected. AWS S3 meets the requirements but adds IAM policy
  management for no benefit here. Rejected.
- A per-machine split into a prefix-scoped push key plus a read-only key would stop a
  compromised machine overwriting other machines' files. Versioning already makes those
  overwrites recoverable, so one key per live machine was chosen for simplicity.
- No-delete keys cannot be created in the B2 web console; setup needs the `b2` CLI or API.
  Revocation works from the console. Backblaze does not document how quickly a deleted key
  stops working (auth tokens last up to 24 hours), so revocation is planned as up-to-a-day.
