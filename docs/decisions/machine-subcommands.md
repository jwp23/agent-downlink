# Machine commands are subcommands of `machine`

## Decision

Commands that manage which machines are readable here are `agent-downlink machine add
<name>`, `agent-downlink machine remove <name>`, and `agent-downlink machine list`. The
earlier spellings `add-machine` and `remove-machine` are gone; they are not kept as aliases,
hidden or listed.

## Rationale

- A noun with verbs under it scales: `list` joined `add` and `remove` without a third
  hyphenated top-level command, and `timer install|remove` already set the pattern.
- Aliases were rejected. The tool had one operator and no release when the spelling changed,
  and an alias kept "for one release" is never removed. Every document, the usage text, and
  `setup`'s closing hint say only the new spelling, so nothing teaches the old one.
