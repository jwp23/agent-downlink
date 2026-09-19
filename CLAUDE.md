# Project Instructions for AI Agents

This file provides instructions and context for AI coding agents working on this project.

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

The Go module does not exist yet. The feature that creates it (`agent-downlink-eu4.1`) adds the
build, test, and lint commands here. rclone must be installed to run the integration tests.

## Architecture Overview

`agent-downlink` is a single Go binary that wraps rclone. Each machine pushes its AI-agent
session records to a client-side-encrypted Backblaze B2 bucket, and keeps a decrypted local
mirror of every machine's records for other tools to read. It is transport and archive only.

- `docs/designs/agent-downlink.md` is the living design. Read it before changing behavior, and
  update it in place when the design changes.
- `docs/adr/` and `docs/decisions/` record why. An ADR is history: supersede it with a new ADR,
  never rewrite it.

## Conventions & Patterns

**This repository is public.** Treat everything stored in beads as public too, including
`bd remember`, because the beads database syncs to the git remote. Every checked-in file and
every bead describes a generic operator. Never record details
of a real deployment: machine names, operating systems of particular machines, which disks are
encrypted, bucket names, which password manager is used, or how many machines exist. Examples
and test fixtures use placeholder machine names such as `workstation` and `laptop`.

**Secrets never reach a log, a process argument, or test output.** Other users on a machine
can read a process's arguments. Passwords go to rclone on standard input or through the
tool's own `rclone.conf`.

**Nothing deletes.** Every transfer is `rclone copy`. No code path removes or renames a file
in a source directory, the mirror, or the bucket.

**The mirror layout is the public interface.** Consumers depend on
`<machine>/<tool>/<the tool's native tree>` as documented in the design. Changing it needs an
ADR. The tool never imports or knows about a consumer.

**One package executes rclone.** All other code goes through it.

**Tests.** Follow test-driven development. Integration tests run the real rclone binary with
real `crypt` encryption over a local directory; they do not mock rclone, and they fail rather
than skip when rclone is absent. Tests against a real bucket sit behind a build tag and never
run in CI. Test output must be pristine.

**Verify, don't guess.** Confirm rclone flags, scheduler settings, and `b2` commands against
primary documentation or the installed tool's `--help` before relying on them.
