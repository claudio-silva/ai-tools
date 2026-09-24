# About

This repository's tools are skills, MCP servers, and, later, plugins. `bin/aitools` installs and manages them on macOS Apple Silicon. It copies skill files and shipped MCP binaries; it does not build.

## imagen

[imagen](mcp/imagen/) is a lightweight MCP server for OpenAI **gpt-image-2.5** image generation and editing, over stdio. It is a Go program with one direct dependency and ships as a static `mcp/imagen/bin/imagen` binary. The full parameter tables are in [mcp/imagen/README.md](mcp/imagen/README.md).

Tools:

| Tool | What it does |
| --- | --- |
| `generate_image` | Create an image from a text prompt |
| `edit_image` | Edit 1–16 existing images, with an optional mask |
| `get_usage_guide` | Return the embedded prompting guide, also sent at `initialize` |

`generate_image` and `edit_image` write the image to an absolute `outputPath`. Models are `gpt-image-2.5-flare` (default) and `gpt-image-2.5-sunburst`. Quality runs from `low` to `max`. `background` can be `transparent` for png or webp. `n` from 1 to 10 writes `name-1.ext` through `name-N.ext`.

Install with `aitools install imagen`. That copies the shipped binary and writes the server into each platform MCP config. Tool calls need `OPENAI_API_KEY`; install fills `$OPENAI_API_KEY` from the environment, or prompts for it. An empty answer at the prompt leaves the placeholder. Pass `--raw` to skip the prompt and leave placeholders. Restart the MCP client after installing.

To rebuild the binary during development:

```sh
cd mcp/imagen
make build      # mcp/imagen/bin/imagen
```

## Skills

`bin/aitools` lists, installs, updates, and uninstalls the skills in this repository. Clone the repo and run `aitools setup` to symlink the script to `~/bin/aitools` or `~/.local/bin/aitools`, whichever of those directories is on `PATH`; the link is followed back to this repository:

```sh
git clone https://github.com/claudio-silva/ai-tools.git
cd ai-tools
./bin/aitools setup
aitools list
```

`aitools setup --remove` removes that link when it points at this clone.

`aitools pull` runs `git pull` in that clone and prints the commit, which is the repository version. A zip download has no commit to update.

A skill is a directory under `skills/<Platform>/<name>/` with a `SKILL.md` and a `manifest.json`. The folder name selects the platforms that skill can be installed to:

| Folder | Platforms |
| --- | --- |
| `Shared` | cursor, codex, claude-code, devin |
| `Cursor` | cursor |
| `Codex` | codex |
| `Claude` | claude-code |
| `Devin` | devin |

`Shared` is the cross-platform folder. `Claude` and `Devin` are reserved for skills that belong to only that platform; none are in the repo yet.

The installer copies the files named in the manifest. It does not symlink, and it does not copy anything the manifest omits, so READMEs, notes, and repo-only tooling stay in the repository. Deletions go through the `trash` command on `PATH` (macOS `/usr/bin/trash`, or the [trash](https://github.com/ali-rantakari/trash) CLI).

## Commands

| Command | What it does |
| --- | --- |
| `install [tool...]` | Install the named tools. Skills are copied from their manifest. MCP servers are copied from the shipped binary and written into the platform MCP config |
| `update [tool...]` | Reinstall installed tools whose repository copy is newer. No names, or `--all`, selects every outdated install |
| `uninstall [tool...]` | Remove tools this command previously installed |
| `list [tool...]` | List tools in this repository, grouped by platform. MCP servers are marked `(mcp)` |
| `installed [tool...]` | List tools installed on each platform, then by scope |
| `about` / `info` / `show <tool>` | Show one repository tool's version, metadata, and install status by scope (◉, ◎, ▲, or not installed) |
| `pull` | Run `git pull` in this repository and print its commit |
| `setup` | Symlink this script to `~/bin/aitools` or `~/.local/bin/aitools` when that directory is on `PATH` |
| `help` | Show the command and option summary |

`install`, `update`, and `uninstall` default to `--global`. `installed` defaults to both scopes. `list` and `about` / `info` / `show` show the repository only. `update` and `update --all` select every installed tool whose repository copy is newer. `install` and `uninstall` still need a tool name or `--all`. `install` and `update` take `--raw` when the tool is an MCP server.

## Options

| Option | Meaning |
| --- | --- |
| `-g`, `--global` | User-level directories. Default for `install`, `update`, and `uninstall` when neither scope flag is set. Combine with `--local` to act on both. |
| `-l`, `--local` | Project directories. Combine with `--global` to act on both. |
| `-C`, `--directory DIR` | Project used for `--local`. Defaults to the current directory. Valid only when the local scope is included. |
| `-p`, `--platform NAME` | Limit to one platform. Repeat the flag, or pass a comma-separated list: `cursor`, `codex`, `claude-code`, `devin`. |
| `-a`, `--all` | `install`: every matching tool in the repo. `update`: every outdated install. `uninstall`: every recorded install for the selected scope and platforms. |
| `-n`, `--dry-run` | Print the copy and delete actions without changing anything. |
| `--version` | With `installed`, append each tool's version. |
| `--raw` | With `install` or `update` of an MCP server, write `$NAME` placeholders instead of filling them. |
| `-h`, `--help` | Show the command and option summary. |

A named skill is never installed to a platform its folder does not support. With `--all`, platforms a skill does not support are skipped, and each matching skill is installed only to the platforms you named.

```sh
./bin/aitools install removebg-cli --platform cursor
./bin/aitools install --all --platform cursor
./bin/aitools install auto-routing
./bin/aitools install --local --directory ~/src/my-app affinity-sdk
./bin/aitools install --global --local removebg-cli
./bin/aitools uninstall auto-routing
./bin/aitools uninstall --all --local
./bin/aitools list
./bin/aitools list --platform codex
./bin/aitools about auto-routing
./bin/aitools about imagen
./bin/aitools install imagen
./bin/aitools installed --version
./bin/aitools update
./bin/aitools update --all
./bin/aitools pull
./bin/aitools update auto-routing
./bin/aitools install auto-routing --dry-run
./bin/aitools install imagen --raw
./bin/aitools uninstall imagen
```

## Where skills are installed

| Platform | Global | Local (project) |
| --- | --- | --- |
| cursor | `~/.cursor/skills` | `.cursor/skills` |
| codex | `$CODEX_HOME/skills` (`$CODEX_HOME` defaults to `~/.codex`) | `.agents/skills` |
| claude-code | `~/.claude/skills` | `.claude/skills` |
| devin | `~/.config/devin/skills` (Devin for Terminal) | `.devin/skills` |

A Shared skill is copied once into each selected platform directory.

Cursor discovers `.agents/skills` as a project skill directory on its own, so a local Codex install is also visible to Cursor. When **Include third-party Plugins, Skills, and other configs** is enabled, Cursor also discovers the other platforms' skill directories (`.codex/skills`, `.claude/skills`, and their `~/` equivalents). A Shared skill can therefore appear more than once in Cursor.

Codex loads custom agent TOML files from `$CODEX_HOME/agents` (usually `~/.codex/agents`) and does not register a project `.codex/agents` directory. Manifests that install roles use `$CODEX_HOME/agents` for both global and local installs. Restart Codex after installing or removing those files.

## Where MCP servers are installed

Each MCP is `mcp/<name>/` with a `manifest.json` and a shipped binary at `mcp/<name>/bin/<name>`. `aitools install` copies that binary to `~/bin/<name>` when `~/bin` exists and is writable, otherwise to `~/.local/bin/<name>`. Install stops if neither directory is available. The `settings` object is written into:

| Platform | Global | Local (project) |
| --- | --- | --- |
| cursor | `~/.cursor/mcp.json` | `.cursor/mcp.json` |
| codex | `$CODEX_HOME/config.toml` | `.codex/config.toml` |
| claude-code | `~/.claude.json` | `.mcp.json` |
| devin | `~/.config/devin/mcp_config.json` | `.devin/mcp_config.json` |

String values in `settings` may contain `$BINARY` and `$NAME`. `$BINARY` is always the installed executable. Other `$NAME` placeholders are filled from the environment, or by a prompt. An empty answer at the prompt leaves the placeholder. `--raw` skips the prompt and leaves placeholders in the file. Secrets are written only into the platform config.

An existing server entry that this tool did not record is left in place, and install stops for that destination. Uninstall removes the server key and a binary this tool copied to `~/bin` or `~/.local/bin` when no other recorded install still uses it.

## Install record

Installs are recorded in `~/.local/state/ai-tools/installs.json`, or `$XDG_STATE_HOME/ai-tools/installs.json` when that variable is set. `uninstall` deletes recorded paths, plus any paths the current manifest lists under `remove`. A path that another recorded install still uses is kept.

A destination skill directory that already exists and was not installed by this tool is left in place, and `install` stops with an error for that destination. Move it aside, then install again.

Reinstalling a managed skill copies the current manifest again and deletes files inside that skill directory that the manifest no longer lists. Files the manifest installs outside the skill directory are replaced.

`list` prints the skills in this repository, grouped by the platforms they can be installed to. A Shared skill appears under each platform. Directory paths are omitted.

```text
cursor
  affinity-sdk
  cursor-plugin-development
  removebg-cli

codex
  affinity-sdk
  auto-routing
  removebg-cli
```

`installed` reads the platform directories, not only the record. A folder that contains `SKILL.md` is listed. ◉ marks a skill from this repository. ◎ marks a skill that is not in this repository. ▲ replaces ◉ when a file in the repository is newer than the installed copy. An installed copy that is newer than the repository keeps ◉. Files this tool installed outside the skill directory are counted on that line. A recorded skill whose directory is missing is still listed, so `uninstall` can remove the files that were installed beside it.

Output is grouped by scope, then by tool kind, then by platform. Platforms with nothing installed are omitted, and a scope with nothing in it says so on one line.

```text
Global

  Skills
    cursor       ~/.cursor/skills
      ◉ affinity-sdk
      ▲ cursor-plugin-development
      ◎ some-other-skill
    codex        ~/.codex/skills
      ◉ auto-routing  (+ 9 files in ~/.codex/agents)

  MCP servers
    devin        ~/.config/devin/mcp_config.json
      ◉ imagen
      ◎ nano-banana

Project  ~/src/my-app
  nothing installed
```

MCP servers are read from the platform MCP configs. The same marks apply: ◉ is an MCP from this repository, ◎ is not, ▲ when the shipped binary is newer than the installed copy or the recorded command path differs.

## Versions

A skill's version is `v` plus the local modification time of its newest manifest file, to the minute: `vYYMMDDHHmm`, for example `v2608151725`. There is no counter to bump. `about` prints the repository skill's version. `installed --version` prints the installed copy's version. When the repository is newer, it shows the installed version, then the repository version:

```text
◉ auto-routing - v2608151725
▲ cursor-plugin-development - v2601010900 < v2608151725
```

An MCP server's version uses the same format, from the binary's modification time. `about` prints the shipped binary. `installed --version` prints the installed copy.

`update` reinstalls every selected skill whose repository copy is newer. It uninstalls that copy, then installs it, so files the current manifest does not list are removed. With no skill names, or with `--all`, it checks every installed tool from this repository. For an MCP server it recopies the shipped binary and rewrites the config entry.

## manifest.json

Every skill has a manifest next to `SKILL.md`:

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

`remove` is optional. Those paths are deleted on install and on uninstall when they are present. Use it for retired skill directories and obsolete files a previous version installed. A path that another recorded install still uses is kept.

Variables in `to` and `remove` (longer names are expanded first):

| Variable | Meaning |
| --- | --- |
| `$SKILL_DIR` | Destination skill directory |
| `$SKILLS_DIR` | Platform skills directory for this install |
| `$PLATFORM_HOME` | Config root for this platform and scope. Global cursor is `~/.cursor`, local cursor is `<project>/.cursor`. Global codex is `$CODEX_HOME`, local codex is `<project>/.codex`. Claude Code and Devin follow the same pattern (`~/.claude`, `~/.config/devin`, and the project equivalents). |
| `$CODEX_HOME` | Codex user config directory, in both scopes. Agent TOML files belong here. |
| `$HOME` | Home directory |

`to` and `remove` must expand to an absolute path inside the selected platform directories, `$CODEX_HOME`, or the project directory. They cannot be the home directory, a platform config root, a skills directory, or `$CODEX_HOME/agents` itself.

`auto-routing` is the manifest that installs files outside the skill directory and deletes retired ones:

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
    "$CODEX_HOME/agents/te_debug.toml",
    "$CODEX_HOME/agents/te_advise_deep.toml"
  ]
}
```
