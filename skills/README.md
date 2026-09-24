# Skills

`bin/skills` lists, installs, updates, and uninstalls the skills in this repository. Clone the repo and symlink the script onto your `PATH`; the link is followed back to this repository:

```sh
git clone https://github.com/claudio-silva/ai-tools.git
ln -s "$(pwd)/ai-tools/bin/skills" /usr/local/bin/skills
skills list
```

`skills pull` runs `git pull` in that clone and prints the commit, which is the repository version. A zip download has no commit to update.

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
| `install [skill...]` | Copy the selected skills into the platform directories |
| `update [skill...]` | Reinstall installed skills whose repository copy is newer. No names, or `--all`, selects every outdated install |
| `uninstall [skill...]` | Remove skills this tool previously installed |
| `list [skill...]` | List skills in this repository, grouped by platform |
| `installed [skill...]` | List skills installed on each platform, then by scope |
| `about <skill>` | Show one repository skill's version and metadata |
| `pull` | Run `git pull` in this repository and print its commit |
| `help` | Show the command and option summary |

`install`, `update`, and `uninstall` default to `--global`. `installed` defaults to both scopes. `list` and `about` show the repository only. `update` and `update --all` select every installed skill whose repository copy is newer. `install` and `uninstall` still need a skill name or `--all`.

## Options

| Option | Meaning |
| --- | --- |
| `-g`, `--global` | User-level directories. Default for `install`, `update`, and `uninstall` when neither scope flag is set. Combine with `--local` to act on both. |
| `-l`, `--local` | Project directories. Combine with `--global` to act on both. |
| `-C`, `--directory DIR` | Project used for `--local`. Defaults to the current directory. Valid only when the local scope is included. |
| `-p`, `--platform NAME` | Limit to one platform. Repeat the flag, or pass a comma-separated list: `cursor`, `codex`, `claude-code`, `devin`. |
| `-a`, `--all` | `install`: every skill in the repo. `update`: every installed skill whose repository copy is newer. `uninstall`: every skill this tool has recorded for the selected scope and platforms. |
| `-n`, `--dry-run` | Print the copy and delete actions without changing anything. |
| `--version` | With `installed`, append each skill's version. |
| `-h`, `--help` | Show the command and option summary. |

A named skill is never installed to a platform its folder does not support. With `--all`, platforms a skill does not support are skipped, and each matching skill is installed only to the platforms you named.

```sh
./bin/skills install removebg-cli --platform cursor
./bin/skills install --all --platform cursor
./bin/skills install auto-routing
./bin/skills install --local --directory ~/src/my-app affinity-sdk
./bin/skills install --global --local removebg-cli
./bin/skills uninstall auto-routing
./bin/skills uninstall --all --local
./bin/skills list
./bin/skills list --platform codex
./bin/skills about auto-routing
./bin/skills installed --version
./bin/skills update
./bin/skills update --all
./bin/skills pull
./bin/skills update auto-routing
./bin/skills install auto-routing --dry-run
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

```text
cursor
  global  ~/.cursor/skills
    ◉ affinity-sdk
    ▲ cursor-plugin-development
    ◎ some-other-skill
  local   ~/src/my-app/.cursor/skills
    (none)

codex
  global  ~/.codex/skills
    ◉ auto-routing  (+ 9 files in ~/.codex/agents)
  local   ~/src/my-app/.agents/skills
    (none)
```

## Versions

A skill's version is `v` plus the local modification time of its newest manifest file, to the minute: `vYYMMDDHHmm`, for example `v2608151725`. There is no counter to bump. `about` prints the repository skill's version. `installed --version` prints the installed copy's version. When the repository is newer, it shows the installed version, then the repository version:

```text
◉ auto-routing - v2608151725
▲ cursor-plugin-development - v2601010900 < v2608151725
```

`update` reinstalls every selected skill whose repository copy is newer. It uninstalls that copy, then installs it, so files the current manifest does not list are removed. With no skill names, or with `--all`, it checks every installed skill from this repository.

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
