# working-with-codex-subagents

A verified field guide to Codex custom subagents — where agent definitions live, how `spawn_agent` binds them, and how model/effort actually resolve. Every claim is tagged *(verified)*, *(binary)* or *(docs)* by how it was established (against codex-cli 0.145.0).

## What's inside

- **Discovery** — `~/.codex/agents/*.toml` registers; `config.toml [agents.<name>]` works; project `.codex/agents/` is advertised-but-not-spawnable; plugin-bundled TOMLs don't register. Discovery happens at startup — `codex exec` is the fast test loop
- **The silent failure** — an undiscovered role doesn't error: `spawn_agent` "succeeds" with a generic child. Binding must be verified by having the child state its identity
- **File format** — `agent_type` is the TOML `name` field, *not* the filename; `developer_instructions` is the only field the child can see
- **Model/effort resolution** — spawn arg > definition pin > `[agents]` defaults > parent > model default. `fork_turns="all"` (the default) **silently voids** spawn `model`/`reasoning_effort` args
- **Concurrency** — the injected slot count is config + 1 (default yields 3 usable subagent slots); overflow is a hard error, `max_depth` defaults to 1
- **The delegation gate** — subagents are off unless the user, `AGENTS.md`, or a skill explicitly asks for delegation
- **Shared filesystem** — all agents share one cwd and filesystem; parallel writers need their own isolation and absolute paths
- **Verification recipes** — `codex debug` commands to re-check every claim on your build, plus a spawn checklist

## Install

From the repository root:

```sh
./bin/aitools install working-with-codex-subagents
```

`Codex` installs this skill for Codex only. `manifest.json` copies `SKILL.md`. See the [skills README](../../README.md).

## Contents

| Path | Purpose |
| --- | --- |
| `SKILL.md` | The full reference: discovery, format, resolution tables, tools, checklists |
