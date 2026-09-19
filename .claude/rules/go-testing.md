---
paths:
  - "**/*.go"
---
# Go Testing

- Unit tests are table-driven and work in `t.TempDir()`. Never touch the real home directory,
  `~/.config/agent-downlink`, or the operator's own rclone configuration.
- Integration tests run the real rclone binary with real `crypt` encryption over a local
  directory standing in for the bucket. Do not mock rclone. When rclone is absent they fail;
  they never call `t.Skip`.
- Captured rclone output used as a fixture lives in the package's `testdata/`.
- End-to-end tests against a real bucket carry the `e2e` build tag and live in `e2e/`. Plain
  `go test ./...` must never compile them, and they never run in CI.
- Test output must be pristine. A test that provokes an error captures the error text and
  asserts on it.
- Generated systemd and launchd files are compared against expected files, not substrings.
- Fixtures use placeholder machine names (`workstation`, `laptop`) and throwaway passwords
  generated in the test.
