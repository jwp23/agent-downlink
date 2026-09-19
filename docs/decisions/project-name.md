# Project and command name: agent-downlink

## Decision

The repository, the Go module, and the installed command are all named `agent-downlink`.

## Rationale

- The tool does one job: machines send their agent records down to a common archive, the way
  spacecraft downlink telemetry to a ground station. Dashboards and review workflows are
  separate consumers, so the name describes transport only.
- One name everywhere keeps `go install` simple (Go names a binary after its main package's
  directory), is self-explanatory in process lists and timer definitions, and cannot collide
  with another command. The command is run by a timer far more often than it is typed.
- `agent-debrief` was rejected: an existing npm package of that name, in the same niche,
  already installs a `debrief` command.
- `agent-intel` was rejected: searches for it are dominated by Intel Corporation's agent
  products.
- `agent-intelligence` had no collisions but is long and unsearchable among results about
  intelligent agents.
