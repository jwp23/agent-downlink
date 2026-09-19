# Config in TOML, status in JSON

## Decision

`config.toml` holds the non-secret settings. `status.json` holds the outcome of each run.

## Rationale

- The config file is occasionally edited by a person, long after setup. TOML allows comments,
  so `setup` writes a file that explains each field in place, and TOML has none of JSON's
  trailing-comma fragility or YAML's parsing surprises. The cost is one dependency, compiled
  into the binary.
- The status file is written by the tool and read by the tool or by a consumer. It is never
  hand-edited, and every language parses JSON natively, which keeps consumers decoupled.
- The TOML library is `github.com/pelletier/go-toml/v2`. It has no dependencies of its own and
  can emit a comment above each field when encoding. `github.com/BurntSushi/toml` also has no
  dependencies but cannot write comments.
