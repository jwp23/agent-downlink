# Read secrets from the terminal with golang.org/x/term

## Decision

`setup` and `add-machine` read the storage key and encryption passwords with
`term.ReadPassword` from `golang.org/x/term`, so nothing typed or pasted is echoed. When
standard input is not a terminal, they read a plain line instead.

## Rationale

- The Go standard library cannot turn off terminal echo. `golang.org/x/term` is maintained by
  the Go team and is the usual way to do it. It is compiled into the binary, so rclone remains
  the only thing a user installs. It brings in `golang.org/x/sys`.
- Shelling out to `stty -echo` avoids the dependency but must restore echo on every exit path,
  including signals, or it leaves the operator's terminal broken.
- The plain-line fallback lets tests drive both flows with scripted input and lets an operator
  pipe a secret in.
