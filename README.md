# ai-tools

Custom-built skills, MCP servers, plugins and developer tools for AI environments like Codex, Cursor, Devin, etc.

## Layout

```
skills/
  Shared/   # skills that work across environments (install anywhere)
  Codex/    # Codex-specific skills
  Cursor/   # Cursor-specific skills
  Devin/    # Devin-specific skills
```

Each skill is a folder containing a `SKILL.md` plus any supporting files
(`references/`, `scripts/`, `agents/`, …). Skills under `Shared/` may still
carry environment-specific metadata (e.g. `agents/openai.yaml` for Codex
plugin display) — it is ignored by environments that don't use it.
