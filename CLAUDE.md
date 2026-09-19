# agent-downlink

A single Go binary that wraps rclone. Each machine pushes its AI-agent session records to a
client-side-encrypted Backblaze B2 bucket and keeps a decrypted local mirror of every readable
machine's records for other tools to read. Transport and archive only. Linux and macOS.

## Tech Stack

Go (version from `go.mod`), standard library first. Dependencies: `github.com/pelletier/go-toml/v2`
(config), `golang.org/x/term` (secret input). rclone is the only runtime prerequisite. systemd
user timer on Linux, launchd agent on macOS. GitHub Actions CI on both.

## What This Project Does NOT Do

Refuse these, or raise them with the operator, rather than building them:

- No consumer features: no dashboards, transcript parsing, cross-tool schema, or scrubbing of
  records. The tool never imports or knows about a consumer.
- No deletion, sync, or pruning of a source directory, the mirror, or the bucket.
- No automated provisioning of buckets, lifecycle rules, or storage keys. That is a runbook
  procedure, so no administrative credential ever rests on disk.
- No server, daemon, staleness alerting, or tamper detection.
- No Windows support.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:6cd5cc61 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->


## Build & Test

rclone and golangci-lint must be installed; integration tests fail, rather than skip, without rclone.
The pre-commit hook in `.beads/hooks/pre-commit` runs the first four commands below on every commit.

```bash
go build .                      # builds ./agent-downlink
gofmt -l .                      # must print nothing
go vet ./...
golangci-lint run               # config in .golangci.yml
go test ./...                   # unit and integration tests (real rclone, local directory bucket)
go test -tags e2e ./e2e/ -v     # end-to-end against a real B2 scratch bucket; never in CI; see e2e/README.md
```

## Invariants

Each of these is a security or data-loss guardrail with no exceptions.

- **This repository is public, and so is everything in beads** (`bd remember` included; the
  beads database syncs to the git remote). Describe a generic operator. Never record a real
  deployment: machine names, which OS a machine runs, which disks are encrypted, bucket names,
  the password manager in use, or how many machines exist. Examples and fixtures use
  placeholder machine names such as `workstation` and `laptop`.
- **Secrets never reach a log, a process argument, or test output.** Other users on a machine
  can read a process's arguments.
- **Nothing deletes.** Every transfer is `rclone copy`. No code path removes or renames a file
  in a source directory, the mirror, or the bucket.
- **The mirror layout is the public interface**: `<machine>/<tool>/<the tool's native tree>`.
  Changing it needs an ADR.
- **Only `internal/rclone` executes rclone.** All other code goes through it.

## Code Style

- `package main` at the repo root holds dispatch and one `cmd_<area>.go` per command group.
  Components are `internal/<name>` packages named as in the design's Components table.
- Commands take everything from the process through the `env` struct and return an exit
  status, so tests substitute all of it. Do not read `os.Stdin`, `os.Args`, or the home
  directory anywhere but `main`.
- Verify, don't guess: confirm rclone flags, systemd and launchd settings, and `b2` commands
  against primary documentation or the installed tool's `--help` before relying on them.
- When conventions and simplicity conflict, simplicity wins.

## Testing

Use red/green TDD for every feature and bugfix: write a failing test, watch it fail, write the
minimum code to pass, refactor while green. Markdown and config files are exempt. Go-specific
test rules load from `.claude/rules/go-testing.md` when you touch Go files.

## Dependencies

rclone is the only thing a user installs; every Go dependency is compiled into the binary.
Adding one needs a decision record in `docs/decisions/` that names the alternatives rejected.

## Reference Documents

**IMPORTANT:** Before starting any task, identify which docs below are relevant and read them first. Load the full context before making changes.

- `docs/designs/agent-downlink.md` — Read before changing any behavior. The living design:
  layout contract, commands, run cycle, files on disk, components, failure handling, security
  model, test layers. Update it in place when the design changes.
- `docs/adr/*.md` — Read when a change touches encryption, storage provider, local secrets, or
  the language and distribution choice. An ADR is history: supersede it with a new ADR, never
  rewrite it.
- `docs/decisions/*.md` — Read when working on naming, config or status formats, or terminal
  secret input. Lighter decisions: a Decision section and a Rationale section.
