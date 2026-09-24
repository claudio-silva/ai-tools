# who-am-i

Report the invoking Codex agent's **actual** runtime identity — model, reasoning effort, thread/session, provider — from read-only local evidence. A model asked directly cannot reliably name itself; this utility correlates the live thread against persisted Codex state instead.

## Features

- **Runtime truth, not configuration** — distinguishes `effective-runtime-value` from `configured-default`, heuristic and unknown, with per-field confidence and evidence sources
- **Thread correlation** — resolves the current thread via `CODEX_THREAD_ID` against rollout/session state; never substitutes "most recently updated thread" for the current one
- **Read-only** — never modifies Codex config, SQLite databases, rollouts or UI settings
- **Self-contained** — the utility ships in `scripts/` inside the skill; no external installation needed

## Usage

```sh
scripts/codex-who-am-i            # JSON — for tools and scripts
scripts/codex-who-am-i --human    # human-readable text
scripts/codex-who-am-i --debug    # resolution decisions on stderr (no conversation content)
```

`--allow-latest-fallback` opts into a low-confidence "latest persisted thread" heuristic when no runtime ID exists — only for users who explicitly accept it.

## Requirements

- Python 3
- A Codex home with config and session state (`$CODEX_HOME` or `~/.codex`)

## Install

From the repository root:

```sh
./bin/aitools install who-am-i
```

`Codex` installs this skill for Codex only. `manifest.json` copies `SKILL.md`, `scripts/`, and `agents/`. Restart Codex afterwards. See the [skills README](../../README.md).

## Contents

| Path | Purpose |
| --- | --- |
| `SKILL.md` | Invocation contract and reporting rules for the agent |
| `scripts/codex-who-am-i` | The utility (stdlib-only Python, executable) |
| `agents/openai.yaml` | Codex plugin display metadata |
