# affinity-sdk

An agent skill that overrides the official **Affinity by Canva MCP** instructions — the verified procedure for operating Affinity documents (files, layers, spreads, rendering, scripting) while avoiding the traps baked into the SDK examples.

## Features

- **Mandatory first-actions sequence** — read the MCP `preamble` topic, run the bundled inspect script unchanged, then `render_spread` with its `sessionUuid`
- **Execution contract** — `execute_script` runs payloads as scripts, not modules: `require('/document')` with leading slash, no `module.exports`, results via `console.log` JSON blobs
- **Verified field map** — `Document.current`, `doc.layers` (iterable) vs `doc.layers.all` (nested), display name is `userDescription`/`description` (`layer.name` is undefined), bottom-to-top iteration order
- **Known traps documented** — `doc.properties` doesn't exist, `Document.opened` doesn't exist, `NOT_ALLOWED` means Affinity settings blocked the capability (not a missing API), filesystem access is Desktop-only
- **Visual confirmation discipline** — `execute_script` returns no session UUID in metadata; a fabricated one fails with `No document with that Uuid exists`

## Requirements

- Affinity by Canva installed
- The official Affinity MCP server configured in the MCP client
- AI / filesystem / network permissions enabled in Affinity settings, as needed

## Install

Copy this folder into any MCP-capable environment's skills directory — `~/.cursor/skills/`, `~/.codex/skills/`, `~/.config/devin/skills/`, or `~/.agents/skills/`.

## Contents

| Path | Purpose |
| --- | --- |
| `SKILL.md` | The operating procedure: first actions, execution contract, inspect script, verified behaviors and prohibitions |
