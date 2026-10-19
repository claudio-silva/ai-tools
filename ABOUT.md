# Repository format

Any git repository — or local directory — that contains skills or MCP servers is an `aitools` source. Users point the tool at it (`aitools use <repo>`, or a per-command first argument) and get `list`, `install`, `update`, and `uninstall` for everything it publishes. There is no central registry: your repository is the package index.

The layout is free-form. No directory names are required, and no metadata is needed for the common case: a skill is found by its `SKILL.md` and installed to every platform. An optional `aitools.json` adds what a tool can't express on its own: platform limits, files placed outside the skill directory, files left out, and MCP server settings.

## Skills

A skill is a directory containing `SKILL.md`. Discovery matches the [`skills`](https://github.com/vercel-labs/skills) CLI (`npx skills add`), so a repository that works with one works with the other:

- A `SKILL.md` at the repository root makes the whole repository one skill.
- Otherwise these containers are searched up to three levels deep: `skills/`, `skills/.curated/`, `skills/.experimental/`, `skills/.system/`, `.agents/skills/`, `.claude/skills/`. Group skills however you like: `skills/<name>/` and `skills/<group>/<name>/` both work.
- If the containers hold no skills, the whole repository is searched.
- A skill directory hides any `SKILL.md` nested inside it. Hidden directories, `node_modules`, and `vendor` are skipped.

`SKILL.md` needs YAML frontmatter with `name` (lowercase letters, digits, and hyphens; it names the installed directory) and `description`. A skill missing either is skipped with a warning. Quote values that contain `: `. aitools reads them anyway, but warns, because strict YAML parsers (including `npx skills`) skip the skill.

By default, the whole skill directory is installed — every file except dotfiles, `.DS_Store`, `__pycache__`, `.pyc` files, and `aitools.json` — to every platform. If the skill ships helper scripts or binaries, document in `SKILL.md` how the agent should invoke them; aitools preserves permissions.

### Skill metadata: aitools.json

Put an `aitools.json` next to `SKILL.md`, or describe the skill under `skills.<name>` in the repository's root `aitools.json` (one or the other, not both). Every key is optional:

```json
{
  "platforms": ["codex"],
  "exclude": ["tests", "token-economy-support"],
  "files": [
    { "from": "te-agents/*.toml", "to": "$CODEX_HOME/agents/" },
    { "from": "agents/openai.yaml", "platforms": ["codex"] }
  ],
  "remove": ["$SKILLS_DIR/token-economy", "$CODEX_HOME/agents/te_debug.toml"]
}
```

- `platforms` — the platforms this skill supports: `cursor`, `codex`, `claude-code`, `devin`. Omitted means all.
- `exclude` — paths or globs, relative to the skill directory, left out of the default copy. A match on a directory excludes everything under it.
- `files` — placements beyond the default copy. Each entry is `{"from", "to", "platforms"}`; only `from` is required:
  - `from` is a file, a directory, or a glob such as `te-agents/*.toml`. It stays inside the skill directory and cannot contain `..` or `**`. Its files are taken out of the default copy and installed only as this entry says.
  - `to` is the destination. A glob copies each matched file into the `to` directory under its base name. A trailing slash on `to` does the same for one file. A directory `from` copies that directory's contents into `to`. Omitted, `to` is `from`'s own path inside the skill directory — useful together with `platforms`.
  - `platforms` limits the entry to some of the skill's platforms, e.g. a Codex-only metadata file in an otherwise cross-platform skill.
- `remove` — paths deleted on install and on uninstall when present. Use it for retired skill directories and obsolete files a previous version installed. A path another recorded install still uses is kept.

`SKILL.md` must end up in the destination skill directory. Unknown keys are rejected.

Variables in `to` and `remove` (longer names expand first):

| Variable | Meaning |
| --- | --- |
| `$SKILL_DIR` | Destination skill directory |
| `$SKILLS_DIR` | Platform skills directory for this install |
| `$PLATFORM_HOME` | Config root for this platform and scope (e.g. global cursor is `~/.cursor`, local codex is `<project>/.codex`) |
| `$CODEX_HOME` | Codex user config directory, in both scopes |
| `$HOME` | Home directory |

`to` and `remove` must expand to an absolute path inside the selected platform directories, `$CODEX_HOME`, or the project directory — never the home directory, a platform config root, a skills directory, or `$CODEX_HOME/agents` itself.

Other installers ignore `aitools.json`: `npx skills add` copies the skill directory as-is and leaves `files` placements to the skill itself. A skill that needs a placed file can check for it on first use and place it from its own directory.

## MCP servers

An MCP server is a directory whose `aitools.json` has a `settings` key — any directory except the repository root or a skill directory. The directory name is the server name. Alternatively, describe it under `mcp.<name>` in the root `aitools.json`, with an optional `path` to its directory (default: the repository root).

```json
{
  "settings": {
    "command": "$BINARY",
    "env": { "OPENAI_API_KEY": "$OPENAI_API_KEY" }
  },
  "files": ["server.py", "lib"],
  "message": "Restart the client after installing."
}
```

- `settings` (required) — the object written into each platform's MCP config under `mcpServers.<name>` (JSON) or `mcp_servers.<name>` (Codex TOML).
- `platforms` (optional) — subset of `cursor`, `codex`, `claude-code`, `devin`. Omitted means all.
- `files` (optional) — payload paths relative to the server directory, copied to `~/.local/share/ai-tools/mcp/<name>/`. Entries are strings or `{"from", "to"}` objects, where `to` may use `$SERVER_DIR` and `$HOME`.
- `message` (optional) — shown after install.
- `bin/<name>` (optional) — a shipped binary, copied to `~/bin` or `~/.local/bin` on install.

Three shapes, in order of how much they ship:

1. **Binary server** — ship `bin/<name>`; use `$BINARY` in `settings.command`. The binary is copied to the user's bin directory.
2. **Script server** — ship sources via `files`; use `$SERVER_DIR` in `settings` (e.g. `"command": "python3", "args": ["$SERVER_DIR/server.py"]`).
3. **External server** — no `bin/`, no `files`; `settings` alone describes the launcher (`npx`, `uvx`, a remote `url`, …). These fit well in the root `aitools.json`.

Placeholders in `settings` string values: `$BINARY` is the installed binary path, `$SERVER_DIR` the managed payload directory. Any other `$NAME` is filled from the environment at install time — on a TTY the user is prompted for missing values (an empty answer leaves the placeholder), and `--raw` writes placeholders as-is.

Unknown keys are rejected.

## The root aitools.json

Optional. It can hold three keys:

```json
{
  "exclude": ["README.md"],
  "skills": { "auto-routing": { "platforms": ["codex"] } },
  "mcp": { "fetch": { "settings": { "command": "uvx", "args": ["mcp-server-fetch"] } } }
}
```

- `exclude` — applied to every skill, before the skill's own `exclude`. Handy for leaving per-skill READMEs out of installs.
- `skills` — skill metadata by skill name, as an alternative to per-skill files.
- `mcp` — MCP servers by name, each with an optional `path`.

## Example

```text
aitools.json                  {"exclude": ["README.md"]}
skills/
  lite-chrome-automation/     SKILL.md, cdp.mjs, README.md        → all platforms, no metadata
  auto-routing/               SKILL.md, aitools.json, te-agents/   → codex, roles placed in $CODEX_HOME/agents
mcp/
  imagen/                     aitools.json, bin/imagen
```

A working example lives at [claudio-silva/ai-toolbox](https://github.com/claudio-silva/ai-toolbox).

## Where tools land

Skills:

| Platform | Global | Local (project) |
| --- | --- | --- |
| cursor | `~/.cursor/skills` | `.cursor/skills` |
| codex | `$CODEX_HOME/skills` | `.agents/skills` |
| claude-code | `~/.claude/skills` | `.claude/skills` |
| devin | `~/.config/devin/skills` | `.devin/skills` |

MCP configs:

| Platform | Global | Local (project) |
| --- | --- | --- |
| cursor | `~/.cursor/mcp.json` | `.cursor/mcp.json` |
| codex | `$CODEX_HOME/config.toml` | `.codex/config.toml` |
| claude-code | `~/.claude.json` | `.mcp.json` |
| devin | `~/.config/devin/mcp_config.json` | `.devin/mcp_config.json` |

`$CODEX_HOME` defaults to `~/.codex`. Shipped binaries go to `~/bin` when it exists and is writable, else `~/.local/bin`. Multi-file MCP payloads (Python/JS trees, data files) go to the managed directory `~/.local/share/ai-tools/mcp/<name>/` — never into the user's bin directories — and `settings` reference it as `$SERVER_DIR`.

## Install semantics

- `install` copies the skill's files and records the install in `~/.local/state/ai-tools/installs.json` (including which source repository it came from).
- A destination that already exists and wasn't installed by aitools is left alone — the install fails for that destination rather than overwriting foreign files.
- Reinstalling replaces managed files and deletes files inside the skill directory that the repository no longer ships.
- `uninstall` deletes recorded paths and the skill's `remove` paths, keeping paths still used by another recorded install. Deletions go to `~/.Trash` when possible, and are deleted permanently when it can't take them (e.g. a different volume). Paths outside the platform tool directories are never removed, and a symlinked install aborts the operation.
- An MCP entry in a platform config that aitools didn't record is left in place. `uninstall` removes the recorded binary/payload copies when no other install needs them; for a foreign entry it only removes the config key — never files it didn't write. `install --force`/`uninstall --force` override the foreign-entry guards.
- `update` reinstalls tools whose repository copy is newer — comparing the repository files' modification times against the installed copies.

## Versions

A tool's version is `v` plus the local modification time of its newest installed file, to the minute: `vYYMMDDHHmm`. There is no counter to bump — editing a file in the repository is the version bump. `about` prints the repository version; `installed --version` prints the installed copy, and `▲` marks tools where the repository is newer:

```text
◉ auto-routing - v2608151725
▲ imagen - v2601010900 < v2609241803
```

## Commands and options

```text
aitools use [<repo>]
aitools install [<repo>] (<tool>...|--all) [opts]
aitools update [<repo>] [<tool>...|--all] [opts]
aitools uninstall [<repo>] (<tool>...|--all) [opts]
aitools list [<repo>] [<tool>...]
aitools installed [<tool>...] [--version]
aitools about|info|show [<repo>] <tool>
aitools pull [<repo>]
aitools cache | cache clear [<repo>]
aitools setup [--remove]
```

| Option | Meaning |
| --- | --- |
| `-g`, `--global` | User-level directories (default for install/update/uninstall) |
| `-l`, `--local` | Project directories |
| `-C`, `--directory DIR` | Project directory for `--local` (default: cwd) |
| `-p`, `--platform NAME` | `cursor`, `codex`, `claude-code`, `devin` — repeat or comma-separate |
| `-a`, `--all` | Every matching tool (install), every outdated install (update), every recorded install (uninstall) |
| `-n`, `--dry-run` | Print planned changes without touching files |
| `-f`, `--force` | `install`: replace a foreign tool occupying the destination. `uninstall`: also remove copies with no install record |
| `--version` | With `installed`, append each tool's version |
| `--raw` | With `install`/`update`, write `$NAME` placeholders instead of filling them |
| `-h`, `--help` | Command and option summary |

A named tool is never installed to a platform it doesn't support. With `--all`, unsupported platforms are skipped.
