# aitools

The tool manager for AI coding agents.

`aitools` installs, updates, and uninstalls **skills** and **MCP servers** for Cursor, Codex, Claude Code, and Devin; from any repository that follows the [prescribed layout](ABOUT.md). Think npm, but for agent tools: every conforming repo is a registry. No central index, no accounts, no permission needed; publish a repo, and anyone can install your tools with one command.

- Any `owner/repo` on GitHub, GitLab, or Bitbucket (or a local directory) is a tool source.
- Installs are recorded, so `update` and `uninstall` know exactly what they own.
- One static Go binary, no dependencies. macOS today; Linux and Windows are [planned](#platforms).

## Install

### npx (recommended)

```sh
npx aitools help
```

The npm package ships the compiled binary; no downloads at run time.

### Release binary

Download the binary for your platform, then:

```sh
./aitools setup
```

`setup` copies the binary to `~/bin` or `~/.local/bin` (whichever is on `PATH`). If you ran it from a cloned repository, it offers to remove the checkout; the installed binary is self-contained. `aitools setup --remove` undoes the install.

### From source

```sh
git clone https://github.com/claudio-silva/ai-tools.git
cd ai-tools
go build -o aitools .
./aitools setup
```

## Quick start

```sh
# pick a default source; any repo with the layout
aitools use @gh/claudio-silva/ai-toolbox

# see what it offers
aitools list

# install tools
aitools install cua-browser-automation
aitools install --all -p cursor

# or a one-off source, no default needed
aitools install owner/repo some-skill

# what's installed, and what's outdated
aitools installed --version

# refresh the cached snapshot, then update everything outdated
aitools pull
aitools update
```

## Sources

The optional first argument of most commands selects the repository for that operation; otherwise the `use` default applies. `aitools use -` clears the default.

| Form | Meaning |
| --- | --- |
| `owner/repo` | `github.com/owner/repo` |
| `@gh/owner/repo` | GitHub |
| `@bb/owner/repo` | Bitbucket |
| `@gl/group/repo` | GitLab (nested groups OK) |
| `https://…`, `git@host:…`, `ssh://…` | Full URLs |
| `./path`, `~/path`, `/path` | Local directories |

Remote sources must be public repositories (for now). A remote is downloaded once into a local cache and reused until `pull`, `use`, or an explicit first use fetches it again; nothing checks the network on its own. `aitools cache` lists snapshots; `aitools cache clear [repo]` drops them.

## Commands

| Command | What it does |
| --- | --- |
| `use [repo]` | Show or set the default source; `use -` clears it |
| `install [repo] [tool...]` | Install tools; `--all` for everything matching |
| `update [repo] [tool...]` | Reinstall tools whose repository copy is newer |
| `uninstall [repo] [tool...]` | Remove tools aitools installed |
| `list [repo] [tool...]` | List the repository's tools, by platform |
| `installed [tool...]` | List installed tools: ◉ managed, ◎ other, ▲ update available |
| `about`/`info`/`show <tool>` | Details and install status for one tool |
| `pull [repo]` | Refresh the source snapshot (`git pull` or re-download) |
| `cache` / `cache clear [repo]` | Inspect or drop cached remote snapshots |
| `setup [--remove]` | Install or remove the `aitools` binary itself |

Scope and platform options: `-g`/`--global`, `-l`/`--local`, `-C`/`--directory`, `-p`/`--platform` (repeatable, or comma-separated), `-n`/`--dry-run`, `--version`, `--raw`. Full details: `aitools help`.

## What is a tool?

Two kinds today; hooks, plugins, and other agent artifacts are planned:

- **Skills**: `skills/<Platform>/<name>/` with `SKILL.md` and `manifest.json`. Files are copied into the platform's skills directory; `{from, to}` manifest entries can place files anywhere the platform's config allows (e.g. Codex agent TOMLs in `$CODEX_HOME/agents`).
- **MCP servers**: `mcp/<name>/` with `manifest.json`. A shipped `bin/<name>` binary is copied to `~/bin`; script servers (Python, Node, …) ship a payload under a managed directory exposed as `$SERVER_DIR`; settings-only manifests can launch external commands like `npx` or `uvx`. The `settings` block is written into each platform's MCP config.

Tool authors: the layout, manifests, variables, and safety rules are specified in [ABOUT.md](ABOUT.md). A ready-made example lives at [claudio-silva/ai-toolbox](https://github.com/claudio-silva/ai-toolbox).

## Install locations

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

`$CODEX_HOME` defaults to `~/.codex`.

## Platforms

macOS arm64 and amd64. OS-specific behavior sits behind a small seam (`os_darwin.go`), so Linux is a small follow-up and Windows a larger one; both ship when there's demand. Note that WSL2 is not native Windows support: a Linux binary inside WSL manages the WSL filesystem, while Windows-hosted agents read `C:\Users\…`.

## State

`~/.local/state/ai-tools/` (or `$XDG_STATE_HOME/ai-tools/`) holds:

- `installs.json`: the install record that powers update/uninstall
- `config.json`: the `use` default source
- `repos/`: cached remote snapshots

Deletions are moved to `~/.Trash` when possible — a path the Trash can't take (e.g. on another volume) is deleted permanently.
