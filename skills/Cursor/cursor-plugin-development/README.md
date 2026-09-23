# cursor-plugin-development

A verified guide to developing Cursor plugins — commands, skills, subagents and hooks — capturing the behaviors and gotchas that aren't obvious from the official documentation.

## What's inside

- **The #1 gotcha** — Settings → "Include third-party Plugins, Skills, and other configs" must be ON or commands/hooks silently never load (skills and subagents still appear, which makes it look like a plugin bug)
- **Plugin anatomy** — `~/.cursor/plugins/local/<name>/` layout, manifest files, auto-reload on edit, the symlink rejection (use `rsync`, or `workspaceOpen` + `pluginPaths`)
- **Commands** — the markdown body is the prompt; `$ARGUMENTS`/`$N` macros are substituted to *values* before the model sees them, so bodies must describe values, never tokens
- **Skills** — `description` is pre-loaded into every context (keep it short); `disable-model-invocation`, `paths` glob-scoping, self-location via the injected SKILL.md path, no `` !`cmd` `` injection
- **Subagents** — `agents/<name>.md` format, `name` is the Task `subagent_type` identity, always load even with the setting off
- **Hooks** — the native flat camelCase `hooks.json` format (NOT Claude-Code's nested PascalCase shape), exit codes, matchers, `failClosed`, reload behavior, the full event map, and the env vars that are actually reliable (`CURSOR_PROJECT_DIR`, `${CLAUDE_PLUGIN_ROOT}` — never `pwd`)
- **Bootstrap pattern** — capturing the plugin path via a `workspaceOpen`/`sessionStart` hook writing a project state file
- **Decision tables** — which surface to use for a given need, plus a local dev checklist

## Requirements

- Cursor (recent build — the third-party plugins setting exists)

## Install

From the repository root:

```sh
./bin/skills install cursor-plugin-development
```

`Cursor` installs this skill for Cursor only. `manifest.json` copies `SKILL.md` and `references/`. See the [skills README](../../README.md).

## Contents

| Path | Purpose |
| --- | --- |
| `SKILL.md` | The full guide: surfaces, formats, gotchas, checklists |
| `references/hook-events.md` | Per-event input/output schemas for every hook event |
