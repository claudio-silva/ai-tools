---
name: cursor-plugin-development
description: Use when developing or debugging Cursor plugins, slash commands, skills, subagents, and hooks. Extends the official documentation and explains known gotchas.
---

# Developing Cursor Plugins, Commands, Skills & Hooks

Cursor extensions ship as **plugins** that can bundle four surfaces:

| Feature     | Purpose                                          | `/`-menu invocable | Docs                                         |
|-------------|--------------------------------------------------|:------------------:|----------------------------------------------|
| commands    | user-invoked prompt macros (aka "slash commands")|        Yes         | n/a                                          |
| skills      | model- or user-invoked instructions              |        Yes         | [Skills](https://cursor.com/docs/skills)     |
| subagents   | prompts for specialized sub-agents               |        Yes         | [Subagents](https://cursor.com/docs/subagents) |
| hooks       | scripts around agent loop events                 |        No          | [Hooks](https://cursor.com/docs/hooks)       |

This skill captures additional details, behavior and gotchas that aren't obvious from the official documentation.

## Prerequisite: the visibility setting (the #1 gotcha)

Cursor **Settings → "Include third-party Plugins, Skills, and other configs"** must be **ON** for a local plugin's **commands** and **hooks** to load. With it **OFF**, Skills and subagents may still appear in the `/` menu, while commands silently never show up and hooks never fire — which looks like a plugin bug but is just this toggle. Turn it **ON** first when "my command/hook doesn't load".

## Plugin anatomy

```
~/.cursor/plugins/local/<name>/
├── .cursor-plugin/
│   ├── plugin.json        # manifest: name, version, description, author, keywords, logo
│   └── marketplace.json   # ONLY needed for "install local plugin from folder"
├── commands/<cmd>.md      # slash commands
├── skills/<skill>/SKILL.md
├── agents/<agent>.md      # subagents (always load, even without the setting)
└── hooks/hooks.json       # + hook scripts
```

- **Location**: local plugins live in `~/.cursor/plugins/local/<name>/`. All component dirs are auto-discovered.
- **Auto-reload**: Cursor watches the plugin dir and reloads on file edits — no window reload needed for command/skill/agent body changes. (Hooks: see the hooks section.)
- **Don't symlink** a repo into `plugins/local/` — recent Cursor rejects a symlink whose target resolves outside `plugins/local/`. Mirror with `rsync` instead, or load live via a `workspaceOpen` hook returning `pluginPaths`.
- **Logo**: a local plugin's logo won't render in the Cursor Settings UI.

## Commands (`commands/<name>.md`)

The markdown **body is the prompt** injected when the command runs. YAML frontmatter:

```yaml
---
name: my-command
description: What it does.   # also always-loaded into context (see Skills) — keep it short
argument-hint: "[<path>] [--flag] [--n N]"   # shown in the slash menu (UX)
---
```

- Surface in the `/` menu (with the setting on). **Always manual** — a command is never auto-invoked by the model.
- **The agent already receives the raw invocation** — the user's message, including the command name and every argument. So you usually do **not** need a macro to "parse" arguments: just instruct the agent to read the flags/operands off the invocation. Reach for a macro only when it splices a value into the prompt more cleanly than prose can.
- **Macros are substituted to their VALUES before the prompt reaches the model** (commands ONLY — Skills receive none of these). `$ARGUMENTS`, `$1`, `$2`, … **never appear literally**: at invocation time Cursor replaces each with the actual text — or with **nothing** when it's absent. So write the body as if the value is already spliced in, and describe the **value**, never the token:
  - ✅ *"Load the plan from `$1` (empty means no plan was given)."* → at runtime becomes *"Load the plan from /abs/foo.plan.md (empty means…)."*
  - ❌ *"`$1` is the first token unless it starts with `--`."* → gibberish once `$1` is already a value; this kind of meta-description never works.
- **`$1`, `$2`, … `$N`** — whitespace-split positionals. Use `$1` to splice **one discrete value** into a sentence. A file added by `@`-select or by dragging a file/dir/editor-tab becomes a literal **`@/absolute/path`** token (contents **not** expanded; have the agent strip the leading `@`), and it lands as the first argument — so `$1` is the natural handle for a path operand.
- **`$ARGUMENTS`** — the entire trailing text after the command name. Use it to hand the agent a **verbatim blob to act on**, e.g. *"Search the web for `$ARGUMENTS`"* or *"forward every `--…` flag from this list: `$ARGUMENTS`"*. Don't wrap it as *"the arguments are: `$ARGUMENTS`"* — that just duplicates what the agent can already see in the invocation.
- **Don't parse option-values positionally.** `--flag value` splits across positionals (`$3=--flag`, `$4=value`), so positional parsing is fragile — read flags off the invocation (or `$ARGUMENTS`), not `$2…$N`.
- **Decision rule:** default to prose ("read the invocation"); add `$1` only to splice a single value elegantly; add `$ARGUMENTS` only when the prompt must operate on the raw text inline. If a macro isn't earning its place, omit it.
- A command **cannot learn its own file path** or the plugin dir. If you need the plugin path, use a hook (see Bootstrap pattern).

## Skills (`skills/<name>/SKILL.md`)

Folder-per-skill. Frontmatter:

```yaml
---
name: my-skill
description: What it does and WHEN to use it.   # always pre-loaded into context — keep it short
disable-model-invocation: true   # optional — manual-only when true
paths: "**/*.tsx, packages/ui/**"  # optional — glob-scope (legacy alias: globs)
metadata: { team: platform }       # optional — arbitrary key/values
---
```

- **Load locations**: besides a plugin's bundled `skills/`, Cursor auto-discovers standalone skills (recursively, nested category folders allowed) from `.cursor/skills/` and `.agents/skills/` (project) and `~/.cursor/skills/` and `~/.agents/skills/` (user/global). Compat dirs `.claude/skills/` and `.codex/skills/` (plus their `~/` equivalents) are also read.
- **Keep `description` short — it costs context on every turn.** The `description` of *every active skill* — and of every command — is pre-loaded into *every* agent context (it's what the model scans to decide whether to auto-invoke). Write the shortest effective description; cut repetition and redundant trigger-term stuffing. Padding it to "raise the odds the skill gets read" backfires: it just burns context on every request. State the WHAT and WHEN once, plainly.
- **`disable-model-invocation: true`** → manual-only: appears in the `/` menu, never auto-invoked. This makes a skill behave like a slash command. **Omit it** → the model auto-invokes the skill from ambient context, matched on `description`.
- **`paths` → glob-scope the skill.** A comma-separated string or YAML list of globs; when set, the skill is surfaced **only** while the agent reads/edits matching files (keeps file-specific guidance out of context elsewhere). `globs` is accepted as a **legacy alias** — prefer `paths`. Equivalently, a skill placed in a nested `.cursor/skills/` (e.g. `apps/web/.cursor/skills/`) is auto-scoped to that subtree without setting `paths`.
- **No `$ARGUMENTS` / `$N`.** Skills receive only the user's plain message text; parse arguments agent-side.
- **Self-location / relative assets**: when a skill is invoked, Cursor injects it via a `manually_attached_skills` block that includes the SKILL.md absolute **`Path:`**. The model derives the skill's directory from that path to resolve **relative references** to sibling files (`scripts/`, `prompts/`, assets) — the self-containment mechanism. Note `pwd` is the **project root**, not the skill dir, so relative paths only work via the injected SKILL.md path, not the shell CWD.
- **No `!`shell`` injection** — Cursor does not run `` !`cmd` `` and inline its output (true for both skills and commands). Have the agent run the command via a tool instead.
- **Frontmatter fields**: `name`, `description`, `disable-model-invocation`, `paths` (legacy `globs`), `metadata` — that's the full set; no macro/templating beyond these. Folder-local hooks are **not** read from inside a skill — hooks live at the plugin level.
- **Migrating**: the built-in `/migrate-to-skills` converts dynamic rules (the "Apply Intelligently" kind) and slash commands into skills — commands become skills with `disable-model-invocation: true`. Rules with `alwaysApply: true` or explicit `globs` are left as-is.

## Subagents (`agents/<name>.md`)

One markdown file per agent under the plugin's `agents/` directory. The body is the subagent's instruction prompt; YAML frontmatter registers it:

```yaml
---
name: my-plugin-agent          # Task `subagent_type` / slash identity
description: What it does and when to delegate to it.   # keep short — used for discovery/auto-delegation
model: inherit                 # optional — pin a slug (full bracket syntax ok), or `inherit` / omit to follow the parent
readonly: false                # optional — true|false
is_background: false           # optional — true|false
---
```

- **Plugin path only here.** In a plugin, agents live at `agents/<name>.md` next to `commands/`, `skills/`, and `hooks/`. They are auto-discovered with the other plugin surfaces. (Project/user agents under `.cursor/agents/` or `~/.cursor/agents/` are a separate discovery path — out of scope for this skill.)
- **Always load**, even when "Include third-party Plugins, Skills, and other configs" is **OFF** — unlike plugin commands and hooks. They still appear in the `/` menu and as Task `subagent_type` values when the plugin is loaded.
- **`name` is the identity.** It becomes the Task tool `subagent_type` enum value and the slash-menu handle. Keep it stable; renaming breaks callers that hard-code the old type.
- **Keep `description` short.** Like skills/commands, descriptions are scanned for delegation matching — pad them and you burn context without helping discovery.
- **`model` (optional):** pin a concrete model (flat slug or full `id[param=…]` syntax), or use `inherit` / omit so the subagent follows the parent (Task spawn overrides and allow-list limits — see `working-with-subagents`).
- **`readonly` / `is_background` (optional):** booleans (`true` \| `false`).
- **Body = isolated prompt.** The subagent runs in its own context; it receives the caller's Task `prompt` plus this file's body (frontmatter stripped). Write it as a self-contained role: what the caller will pass, what to read/write, what to return, and what **not** to do (e.g. don't spawn further subagents if the host strips nesting).
- **No command macros.** Same as skills — no `$ARGUMENTS` / `$1` substitution in agent files. The caller puts operands in the Task `prompt`.
- **Edits auto-reload** with the rest of the plugin (body/frontmatter changes). A **brand-new** agent file may not appear in the Task enum until a later turn after Cursor rediscovers it — don't assume same-turn spawn after creating the file.
- **Hooks that touch subagents:** `subagentStart` / `subagentStop` (see Hooks). Matchers filter on subagent type.
- Official overview: [Subagents](https://cursor.com/docs/subagents).

## Hooks (`hooks/hooks.json`)

Scripts that observe/gate/extend the agent loop, exchanging JSON over stdin/stdout. **Use Cursor's NATIVE format** — flat, `version: 1`, **camelCase** event keys, each an array of `{ command }`:

```json
{
  "version": 1,
  "hooks": {
    "stop": [ { "command": "${CLAUDE_PLUGIN_ROOT}/hooks/on-stop.sh" } ],
    "preToolUse": [ { "command": "${CLAUDE_PLUGIN_ROOT}/hooks/guard.sh", "matcher": "Shell" } ]
  }
}
```

- **Do NOT use the Claude-Code format for a Cursor-native plugin** — the nested PascalCase shape (`{hooks:{Stop:[{matcher,hooks:[{type,command}]}]}}`) is a separate compatibility path; on a native plugin it only partially registers (typically only `stop` fires). The native flat camelCase format registers all events.
- **Per-hook options**: `command` (required), `type` (`"command"` default | `"prompt"`), `timeout` (s), `matcher` (filter), `failClosed` (block on hook failure), `loop_limit` (for `stop`/`subagentStop` follow-ups; default 5).
- **Exit codes**: `0` = ok (use stdout JSON); `2` = block (= `permission:"deny"`); other = fail-open unless `failClosed:true`.
- **Reload**: Cursor watches `hooks.json` and reloads on save; if a hook still doesn't register, restart Cursor. A newly-added event may only fire on the next turn; `sessionStart` only fires for a brand-new chat.
- **Debug**: Settings → **Hooks** tab + the **Hooks** output channel.

### Environment & working directory (critical)

Plugin hook scripts receive these env vars:

| Variable | Meaning | Reliable? |
|---|---|---|
| `CURSOR_PROJECT_DIR` | workspace root | **Yes — use this for the project** |
| `${CLAUDE_PLUGIN_ROOT}` / `${CURSOR_PLUGIN_ROOT}` | plugin install dir | **Yes — use for plugin files** (both are set; usable in the `command` string and in the script env) |
| `CLAUDE_PROJECT_DIR` | alias of project dir | Yes |
| `CURSOR_VERSION`, `CURSOR_USER_EMAIL`, `CURSOR_TRANSCRIPT_PATH` | session metadata | situational |

- **`pwd` is UNRELIABLE** — a hook's working directory varies between the plugin dir and the project dir across events. **Never derive paths from `pwd`.** Use `CURSOR_PROJECT_DIR` (project) and `${CLAUDE_PLUGIN_ROOT}` (plugin).
- **Never write into the plugin dir** from a hook — Cursor watches it and will reload (churn). Write to the project (`CURSOR_PROJECT_DIR`) or a temp/state dir.

### Hook-event map

Names below are the exact camelCase keys; the `hook_event_name` Cursor reports equals the key (no mapping). Full per-event input/output schemas: [references/hook-events.md](references/hook-events.md).

| Event | Fires when | Notes / output |
|---|---|---|
| `workspaceOpen` | workspace opens + on folder change — **outside any session** (`conversation_id` absent) | can return `pluginPaths` to load plugins live |
| `sessionStart` | a **new chat** (composer conversation) is created — **NOT** on window reload/restart | fire-and-forget; `additional_context` → conversation context (verified); `env` → **later hooks only, NOT the agent's Shell tool** (verified) |
| `sessionEnd` | a chat ends/closes | fire-and-forget; `reason` |
| `beforeSubmitPrompt` | user sends a message | can block (`continue:false`) |
| `preToolUse` | before any tool (filter via `matcher`: `Shell`/`Read`/`Write`/`Task`/`MCP:<t>`) | `permission` allow/deny, `updated_input` |
| `postToolUse` | after a tool succeeds | `additional_context`; MCP: `updated_mcp_tool_output` |
| `postToolUseFailure` | tool fails/times out/denied | observational |
| `beforeShellExecution` | before a shell command (matcher = command text) | `permission`; set `failClosed` for security |
| `afterShellExecution` | after a shell command | observational (`output`, `duration`) |
| `beforeReadFile` | before the agent reads a file | `permission` deny to block |
| `afterFileEdit` | after the agent edits a file | observational (`file_path`, `edits`) |
| `subagentStart` | before a Task subagent (matcher = subagent type) | `permission` |
| `subagentStop` | a Task subagent finishes | `followup_message` (loop) |
| `preCompact` | before context compaction | observational |
| `afterAgentResponse` | after an assistant message | observational (`text`) |
| `afterAgentThought` | after a thinking block | observational (`text`) |
| `stop` | the agent loop ends | `followup_message` (auto-continue; `loop_limit`) |
| `beforeTabFileRead` / `afterTabFileEdit` | Tab (inline completion) file access/edit | Tab-only surface |

**Key behavioral facts:**
- **Reload/restart fires no agent hook** — the session resumes; only `workspaceOpen` fires on reopen/folder-change.
- `sessionStart` is the only per-chat startup hook, and it's **per new conversation**, not per window.
- Hooks defined at `.cursor/hooks.json` (project) and `~/.cursor/hooks.json` (user) use the same native schema; plugin hooks live in the plugin's `hooks/hooks.json`. All matching sources run; precedence Enterprise → Team → Project → User.

### Bootstrap pattern: capturing the plugin path

Commands and skills can't reliably resolve their own install dir, so the robust way to record `$<PLUGIN>_DIR` for a project is a **hook** that self-locates and writes into the project:

- A `workspaceOpen` (no session needed) or `sessionStart` hook reads `${CLAUDE_PLUGIN_ROOT}` (plugin) + `CURSOR_PROJECT_DIR` (project) and **writes the plugin path into a project state file** the commands then read. Prefer `workspaceOpen` for a once-per-open bootstrap.
- **Do not expect to expose the path as a shell `$VAR` via `sessionStart` `env`.** Verified: that `env` reaches only *subsequent hooks*, never the agent's Shell tool (a `Shell` command sees the var **unset**). To put the value in front of the model, use `sessionStart` `additional_context` (it does reach the conversation — verified), but that costs context on **every** new chat regardless of whether the plugin is used; persisting to a state file (read on demand) is leaner.

## Choosing the right surface

| Need | Use |
|---|---|
| `$ARGUMENTS`/`$1` macros, `argument-hint`, always-manual | **Command** |
| Auto-invoke from context, or self-contained relative assets | **Skill** (omit `disable-model-invocation`) |
| Slash entry that's manual-only like a command but folder-structured | **Skill** with `disable-model-invocation: true` |
| Specialized delegated worker with its own prompt / Task `subagent_type` | **Subagent** (`agents/<name>.md`) |
| Observe / gate / extend the agent loop, or capture plugin/project paths | **Hook** (native format) |

## Local dev checklist

1. Enable "Include third-party Plugins, Skills, and other configs".
2. Put the plugin under `~/.cursor/plugins/local/<name>/` (or rsync-mirror a repo there).
3. Command/skill/agent edits auto-reload; for hooks, save and (if needed) restart; `sessionStart` needs a new chat.
4. Verify: commands/skills in the `/` menu; subagents in the Task picker; hooks in Settings → Hooks.
