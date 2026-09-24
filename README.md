# ai-tools

Custom-built skills, MCP servers, plugins and developer tools for AI environments like Codex, Cursor, Devin, etc.

## Install

Clone the repository and symlink `bin/aitools` onto your `PATH`. The link is followed to the real script, and that script's repository is the one the tool manages.

```sh
git clone https://github.com/claudio-silva/ai-tools.git
ln -s "$(pwd)/ai-tools/bin/aitools" /usr/local/bin/aitools
```

`/usr/local/bin` is the usual directory. `/usr/bin` is not a place to install this on macOS. To avoid writing outside your home directory, link it into a directory already on `PATH`, such as `~/.local/bin`.

`aitools pull` runs `git pull` in the clone and prints the current commit. That commit is the version of this repository. A zip download has no commit to compare or update; clone it instead.

## MCP servers

| Project | Description |
| --- | --- |
| [imagen](mcp/imagen/) | Lightweight Go MCP server (stdio) for OpenAI **gpt-image-2.5** image generation and editing — `generate_image`, `edit_image`, embedded prompting guide; single static binary |

## Skills

Each skill is a directory under `skills/<Platform>/<name>/` with a `SKILL.md` and a `manifest.json`. The platform folder decides where it can be installed. Manage them with [`bin/aitools`](skills/README.md) — it copies the files the manifest names (no symlinks), updates a copy when the repository is newer, and can delete retired paths:

```sh
aitools list
aitools install --all
aitools installed
aitools update
aitools pull
```

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
bin/aitools                 # install, uninstall, and list skills
mcp/<name>/                 # standalone MCP servers — each is its own project
skills/
  Shared/<skill>/           # cursor, codex, claude-code, and devin
  Codex/<skill>/            # codex
  Cursor/<skill>/           # cursor
  Claude/<skill>/           # claude-code (none yet)
  Devin/<skill>/            # devin (none yet)
```

Install paths, scope flags, and the manifest format are documented in [skills/README.md](skills/README.md).
