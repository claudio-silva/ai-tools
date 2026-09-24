# working-with-cursor-subagents

Verified behavior of Cursor subagents: where agent files live, how the Task tool's `model` argument resolves against agent frontmatter, and the discovery gotchas that bite during development.

## What's inside

- **Locations** — `<workspace>/.cursor/agents/`, `~/.cursor/agents/`, plugin `agents/` — one `.md` file per agent, frontmatter fields (`name`, `description`, `model`, `readonly`, `is_background`)
- **Discovery lag** — a newly created agent can be missing from the Task `subagent_type` enum on the same turn; retry on a later turn (edits hot-reload, new names need rediscovery)
- **Model resolution matrix** — empirically verified: a frontmatter pin beats the parent model, Task `"inherit"` still resolves to the pin, an explicit allow-listed slug wins, `""` is rejected. Task `model` accepts only flat slugs — bracket syntax (`claude-sonnet-4-6[thinking=…,effort=…,context=…]`) belongs on frontmatter
- **What the child can see** — its effective running model (injected identity), but **not** the Task `model` arg or its own frontmatter; pass those in the Task `prompt` if they matter
- **Resume** — keeps the subagent's existing model; don't pass `model` with it
- **Authoring patterns** — product-ready recipes for "inherit the user's model" vs "pin a fixed model", plus a spawn checklist

## Install

From the repository root:

```sh
./bin/aitools install working-with-cursor-subagents
```

`Cursor` installs this skill for Cursor only. `manifest.json` copies `SKILL.md`. See the [skills README](../../README.md).

## Contents

| Path | Purpose |
| --- | --- |
| `SKILL.md` | The full reference: locations, resolution tables, model syntax, visibility rules, checklist |
