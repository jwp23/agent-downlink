# ADR-004: Go, distributed as a single binary

## Context

The tool is a thin wrapper: it runs rclone, writes a config file and timer definitions, and
records run status. Any mainstream language can do that. What differs is what the choice asks
of the people who install it, and what it couples the tool to. The tool is intended to be
usable by others, and its consumers (dashboards, transcript review) are separate projects.

## Decision

Write the tool in Go and distribute it as a single self-contained binary per OS and
architecture. rclone is the only other thing a user installs.

Consumers integrate through the mirror's on-disk layout, which is documented as the tool's
public contract. No consumer imports the tool's code, and the tool knows nothing about any
consumer.

## Trade-offs

- Node would share a runtime and test setup with one existing consumer and could share layout
  code with it. That sharing is coupling we do not want, and Node tools are sensitive to the
  Node version a user happens to have installed. Rejected.
- Python's standard library fits the job, but recent macOS has no usable Python until the
  developer tools are installed, and version drift is the same problem as Node. Rejected.
- Rust gives the same single-binary result with more friction for a small wrapper. Rejected.
- Bash suits a wrapper, but macOS ships bash 3.2 and the tool has logic (timer generation,
  status parsing, change detection) that is painful to test-drive in shell. Rejected.
- Go costs a build per OS and architecture, and a documented layout contract that must be
  kept stable for consumers instead of a shared library.
