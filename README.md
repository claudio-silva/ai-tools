# ai-tools

Custom-built skills, MCP servers, plugins and developer tools for AI environments like Codex, Cursor, Devin, etc.

## Install

Clone the repository, then run `aitools setup` from the clone (or `./bin/aitools setup` before the link exists). It symlinks the script to `~/bin/aitools` when that directory is on `PATH`, otherwise to `~/.local/bin/aitools`. The link is followed to the real script, and that script's repository is the one the tool manages.

```sh
git clone https://github.com/claudio-silva/ai-tools.git
cd ai-tools
./bin/aitools setup
```

If neither `~/bin` nor `~/.local/bin` is on `PATH`, create one, add it to `PATH`, and run `setup` again. `aitools setup --remove` deletes a link in either directory when it points at this repository.

`aitools pull` runs `git pull` in the clone and prints the current commit. That commit is the version of this repository. A zip download has no commit to compare or update; clone it instead.

## MCP servers

| Project | Description |
| --- | --- |
| [imagen](mcp/imagen/) | Lightweight Go MCP server (stdio) for OpenAI **gpt-image-2.5** image generation and editing — `generate_image`, `edit_image`, embedded prompting guide; single static binary |

## Skills

Each skill is a directory under `skills/<Platform>/<name>/` with a `SKILL.md` and a `manifest.json`. The platform folder decides where it can be installed. Manage tools with [`bin/aitools`](ABOUT.md) — skills are copied from the manifest, MCP servers are copied as shipped binaries, and both can be listed, updated, and removed:

```sh
aitools list
aitools install --all
aitools mcp list
aitools mcp install imagen
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
bin/aitools                 # manage skills and MCP servers
mcp/<name>/                 # MCP server with manifest.json and shipped bin/<name>
skills/
  Shared/<skill>/           # cursor, codex, claude-code, and devin
  Codex/<skill>/            # codex
  Cursor/<skill>/           # cursor
  Claude/<skill>/           # claude-code (none yet)
  Devin/<skill>/            # devin (none yet)
```

Install paths, scope flags, the manifest format, and the imagen server are documented in [ABOUT.md](ABOUT.md).
