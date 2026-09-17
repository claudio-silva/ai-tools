# ai-tools

Custom-built skills, MCP servers, plugins and developer tools for AI environments like Codex, Cursor, Devin, etc.

## MCP servers

| Project | Description |
| --- | --- |
| [imagen](mcp/imagen/) | Lightweight Go MCP server (stdio) for OpenAI **gpt-image-2.5** image generation and editing — `generate_image`, `edit_image`, embedded prompting guide; single static binary |

## Skills

Each skill is a self-contained folder (`SKILL.md` + bundled references, scripts and agent definitions). Install = copy the folder into the target environment's skills directory — no symlinks, so updating an install is always a plain `cp -R` from this repo.

### Cross-environment — `skills/Shared/`

Work in any MCP/agent environment; may carry env-specific metadata (e.g. `agents/openai.yaml` for Codex) that other environments ignore.

| Skill | Description |
| --- | --- |
| [removebg-cli](skills/Shared/removebg-cli/) | remove.bg background removal via CLI/API with credit-aware defaults and mandatory validation |
| [affinity-sdk](skills/Shared/affinity-sdk/) | Verified operating procedure for the official Affinity by Canva MCP |
| [cua-browser-automation](skills/Shared/cua-browser-automation/) | Real-browser E2E testing via the `browse` wrapper over `cua-driver` — snapshot → act → verify, parallel contexts, evidence capture |
| [lite-chrome-automation](skills/Shared/lite-chrome-automation/) | Lightweight CDP control of dedicated Chrome instances — MAIN-world JS eval, cookies, screenshots, extension debugging |

### Codex — `skills/Codex/`

| Skill | Description |
| --- | --- |
| [auto-routing](skills/Codex/auto-routing/) | Cost-aware delegation: routes work to the cheapest pinned `te_*` worker role; installer ships 9 role definitions |
| [codex-remote](skills/Codex/codex-remote/) | Delegate bounded tasks to Codex on SSH servers via a local Luna/low relay subagent; durable job IDs, observe or dispatch-only |
| [who-am-i](skills/Codex/who-am-i/) | Report the agent's real runtime model/effort/thread from read-only local evidence (self-contained script) |
| [working-with-codex-subagents](skills/Codex/working-with-codex-subagents/) | Verified guide to Codex subagent discovery, `spawn_agent` binding, and model/effort resolution |

### Cursor — `skills/Cursor/`

| Skill | Description |
| --- | --- |
| [cursor-plugin-development](skills/Cursor/cursor-plugin-development/) | Verified guide to Cursor plugins: commands, skills, subagents, hooks — including undocumented gotchas |
| [working-with-cursor-subagents](skills/Cursor/working-with-cursor-subagents/) | Verified Task-tool/model-resolution behavior for Cursor subagents |

## Layout

```
mcp/<name>/             # standalone MCP servers — each is its own project
skills/
  Shared/<skill>/       # cross-environment; may carry env-specific metadata
  Codex/<skill>/        # → ~/.codex/skills/  (also read by Cursor as a compat dir)
  Cursor/<skill>/       # → ~/.cursor/skills/
```

`~/.agents/skills/` is the env-neutral global location both Codex-era and Cursor read; `~/.claude/skills/` is also picked up as a compat dir.
