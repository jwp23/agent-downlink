# ADR-001: Client-side-encrypted object storage with per-machine keys

## Context

Agent transcripts (`~/.claude/projects`) live on several personal machines (macOS and Linux)
that are not networked to each other, and none can be presumed always-on. Transcripts contain whatever the agent read: source, `.env` contents,
pasted tokens. The goals are minimal hosting work, easy use, and a high standard of security.

## Decision

Machines meet at cloud object storage. Every machine encrypts before upload, so the provider
only ever holds ciphertext. Consumers (MISSION-CONTROL, transcript review) never read the
bucket; they read a local decrypted mirror that reader machines pull down.

Each machine encrypts into its own area of the bucket with its own symmetric key. A machine
needs only its own key to push. A machine becomes a reader of another machine's data by being
given that machine's key, so read access is granted per machine rather than implied by being
a pusher. Master copies of all keys live in the operator's password manager.

Reading the archive requires two independent things: a storage credential (revocable from the
provider's web console) and the encryption key.

## Trade-offs

- A LAN-only store (NAS or always-on box) keeps data in the house but makes us own uptime,
  backups, and disk failure, and contradicts "no machine is presumed always-on". Rejected.
- Peer-to-peer sync (Syncthing) needs both machines awake at once. Rejected.
- Asymmetric encryption (`age`: pushers hold only a public key) gives the strongest pusher
  isolation but means DIY tooling with no incremental upload. Per-machine symmetric keys give
  the same blast-radius property for push-only machines with mature tooling. Rejected.
- One shared key for all machines is simplest but lets any compromised pusher decrypt every
  machine's data. Rejected.
- Compromising a machine that reads everything exposes everything. That is the cost of
  reading there; per-machine keys pay off for push-only and retired machines.
- Secrets inside transcripts are not scrubbed in the archive. Regex scrubbing is lossy and
  gives false confidence; encryption is the control.
