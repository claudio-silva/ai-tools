# Repository format

Any git repository — or local directory — that follows this layout is an `aitools` source. Users point the tool at it (`aitools use <repo>`, or a per-command first argument) and get `list`, `install`, `update`, and `uninstall` for everything it publishes. There is no central registry: your repository is the package index.

A conforming repository has one or both of:

```text
skills/<Platform>/<name>/   skills, grouped by platform folder
mcp/<name>/                 MCP servers
```

## Skills

A skill is a directory under `skills/<Platform>/<name>/` containing `SKILL.md` and `manifest.json`. The platform folder decides which agents the skill can be installed to:

| Folder | Platforms |
| --- | --- |
| `Shared` | cursor, codex, claude-code, devin |
| `Cursor` | cursor |
| `Codex` | codex |
| `Claude` | claude-code |
| `Devin` | devin |

`SKILL.md` carries the usual frontmatter (`name`, `description`) plus the body the agent reads. If the skill ships helper scripts or binaries, include them in the manifest and document in `SKILL.md` how the agent should invoke them — aitools copies whatever the manifest lists, preserving permissions.

### manifest.json

```json
{
  "install": ["SKILL.md", "scripts", "references"],
  "remove": ["$SKILLS_DIR/old-skill-name"]
}
```

`install` is required and non-empty. Each entry is a path relative to the skill directory, or an object `{"from": "...", "to": "..."}`.

- A string copies a file or directory to the same relative path inside the destination skill directory. Directory copies skip dotfiles, `.DS_Store`, `__pycache__`, and `.pyc` files. Name a dotfile explicitly if it must be installed.
- `from` is a file, a directory, or a glob such as `te-agents/*.toml`. It stays inside the skill directory and cannot contain `..` or `**`.
- `to` is the destination. A glob copies each matched file into the `to` directory under its base name. A trailing slash on `to` does the same for one file. A directory `from` copies that directory's contents into `to`.
- The install must place `SKILL.md` in the destination skill directory.

`remove` is optional. Those paths are deleted on install and on uninstall when present — use it for retired skill directories and obsolete files a previous version installed. A path another recorded install still uses is kept.

Variables in `to` and `remove` (longer names expand first):

| Variable | Meaning |
| --- | --- |
| `$SKILL_DIR` | Destination skill directory |
| `$SKILLS_DIR` | Platform skills directory for this install |
| `$PLATFORM_HOME` | Config root for this platform and scope (e.g. global cursor is `~/.cursor`, local codex is `<project>/.codex`) |
| `$CODEX_HOME` | Codex user config directory, in both scopes |
| `$HOME` | Home directory |

`to` and `remove` must expand to an absolute path inside the selected platform directories, `$CODEX_HOME`, or the project directory — never the home directory, a platform config root, a skills directory, or `$CODEX_HOME/agents` itself.

Example — a skill that also installs Codex agent roles and retires old paths:

```json
{
  "install": [
    "SKILL.md",
    "agents",
    "token-economy-validation.md",
    { "from": "te-agents/*.toml", "to": "$CODEX_HOME/agents/" }
  ],
  "remove": [
    "$SKILLS_DIR/token-economy",
    "$CODEX_HOME/agents/te_debug.toml"
  ]
}
```

## MCP servers

An MCP server is `mcp/<name>/` with a `manifest.json`:

```json
{
  "platforms": ["cursor", "codex", "claude-code", "devin"],
  "settings": {
    "command": "$BINARY",
    "env": { "OPENAI_API_KEY": "$OPENAI_API_KEY" }
  },
  "files": ["server.py", "lib"],
  "message": "Restart the client after installing."
}
```

- `platforms` (required) — subset of `cursor`, `codex`, `claude-code`, `devin`.
- `settings` (required) — the object written into each platform's MCP config under `mcpServers.<name>` (JSON) or `mcp_servers.<name>` (Codex TOML).
- `files` (optional) — payload paths relative to `mcp/<name>/`, copied to `~/.local/share/ai-tools/mcp/<name>/`. Same entry forms as skill manifests: strings and `{"from", "to"}` objects, where `to` may use `$SERVER_DIR` and `$HOME`.
- `message` (optional) — shown after install.
- `bin/<name>` (optional) — a shipped binary, copied to `~/bin` or `~/.local/bin` on install.

Three shapes, in order of how much they ship:

1. **Binary server** — ship `bin/<name>`; use `$BINARY` in `settings.command`. The binary is copied to the user's bin directory.
2. **Script server** — ship sources via `files`; use `$SERVER_DIR` in `settings` (e.g. `"command": "python3", "args": ["$SERVER_DIR/server.py"]`).
3. **External server** — no `bin/`, no `files`; `settings` alone describes the launcher (`npx`, `uvx`, a remote `url`, …).

Placeholders in `settings` string values: `$BINARY` is the installed binary path, `$SERVER_DIR` the managed payload directory. Any other `$NAME` is filled from the environment at install time — on a TTY the user is prompted for missing values (an empty answer leaves the placeholder), and `--raw` writes placeholders as-is.

`platforms`, `settings`, and `message` are the only required/optional keys; unknown keys are rejected.

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

- `install` copies manifest files and records the install in `~/.local/state/ai-tools/installs.json` (including which source repository it came from).
- A destination that already exists and wasn't installed by aitools is left alone — the install fails for that destination rather than overwriting foreign files.
- Reinstalling replaces managed files and deletes files inside the skill directory that the manifest no longer lists.
- `uninstall` deletes recorded paths and the manifest's `remove` paths, keeping paths still used by another recorded install. Deletions go to `~/.Trash` when possible, and are deleted permanently when it can't take them (e.g. a different volume). Paths outside the platform tool directories are never removed, and a symlinked install aborts the operation.
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

A named skill is never installed to a platform its folder doesn't support. With `--all`, unsupported platforms are skipped.
