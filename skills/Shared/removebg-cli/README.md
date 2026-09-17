# removebg-cli

An agent skill for removing image backgrounds with the official [remove.bg](https://www.remove.bg) CLI or HTTP API — with credit-aware defaults so a draft never burns a full credit.

## Features

- **Credit-safe workflow** — inspect dimensions and megapixels first; `preview` size for drafts (0.25 MP cap, free allowance first, 0.25 credit after); `full` only after explicit per-request confirmation; `auto` is never used by default
- **CLI-first, API fallback** — drives the official `removebg` CLI; falls back to direct HTTP calls when the CLI is unavailable or response headers are needed
- **Format guidance** — PNG/RGBA up to 10 MP; WebP or ZIP for 10–50 MP (with `zip2png` when a PNG working file is needed); JPG only when transparency isn't required
- **Mandatory validation** — alpha channel check, inspection on dark and light backgrounds, edge-halo and clipped-margin review, non-destructive output naming
- **Secrets hygiene** — `REMOVE_BG_API_KEY` from the environment only; never printed, persisted, or exposed in output

## Requirements

- `removebg` CLI on `PATH`, or network access to the remove.bg API
- `REMOVE_BG_API_KEY` exported in the environment (e.g. via `~/.zshrc`)

## Install

Copy this folder into your environment's skills directory:

| Environment | Skills directory |
| --- | --- |
| Codex | `~/.codex/skills/` |
| Cursor | `~/.cursor/skills/` (also reads `~/.codex/skills/`, `~/.agents/skills/`, `~/.claude/skills/`) |
| Devin | `~/.config/devin/skills/` |
| Any | `~/.agents/skills/` |

`agents/openai.yaml` is Codex plugin display metadata — harmless in other environments.

## Contents

| Path | Purpose |
| --- | --- |
| `SKILL.md` | The workflow: credit rules, authentication, CLI usage, format selection, validation |
| `references/api.md` | Direct HTTP API syntax, response headers, limits, failure handling |
| `agents/openai.yaml` | Codex plugin display metadata |
