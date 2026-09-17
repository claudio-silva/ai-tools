# Cursor hook events — per-event I/O reference

Every hook receives a JSON object on **stdin** and may return a JSON object on **stdout**. Common input fields present on most events: `hook_event_name`, `conversation_id` (absent for app-lifecycle events like `workspaceOpen`), `generation_id`, and `workspace_roots`. Exit `0` = honor stdout; exit `2` = block (= `permission:"deny"`); any other non-zero = fail-open unless the hook sets `failClosed: true`.

Source of truth: `https://cursor.com/docs/hooks`.

---

## App / session lifecycle

### `workspaceOpen`
Fires when a workspace opens and when the folder set changes — **outside any conversation** (`conversation_id` is absent).
- In: `{ workspace_roots }`
- Out: `{ "pluginPaths": ["/abs/plugin/dir", ...] }` to load plugins live, or `{}`.

### `sessionStart`
Fires once when a **new chat** is created (not on reload/restart).
- In: `{ session_id, ... }`
- Out (optional): `{ "env": { "KEY": "val" }, "additional_context": "text injected into the session" }`. Fire-and-forget.

### `sessionEnd`
Fires when a chat ends.
- In: `{ reason }` — observational.

---

## Prompt & agent loop

### `beforeSubmitPrompt`
Before the user's message is submitted.
- In: `{ prompt, attachments? }`
- Out: `{ "continue": true|false }` (`false` blocks submission).

### `afterAgentResponse`
After an assistant message is produced. In: `{ text }`. Observational.

### `afterAgentThought`
After a reasoning/thinking block. In: `{ text }`. Observational.

### `stop`
The agent loop has ended.
- In: `{ status }`
- Out: `{ "followup_message": "..." }` to auto-continue. Bounded by `loop_limit` (default 5).

### `preCompact`
Before context compaction. In: `{ usage info }`. Observational.

---

## Tools

`matcher` filters by tool: e.g. `Shell`, `Read`, `Write`, `Edit`, `Task`, `MCP:<tool>`.

### `preToolUse`
Before any tool call.
- In: `{ tool_name, tool_input }`
- Out: `{ "permission": "allow"|"deny"|"ask", "updated_input"?: {...}, "user_message"?, "agent_message"? }`. Exit 2 == deny.

### `postToolUse`
After a tool succeeds.
- In: `{ tool_name, tool_input, tool_output }`
- Out: `{ "additional_context"?: "...", "updated_mcp_tool_output"?: ... }`.

### `postToolUseFailure`
After a tool fails, times out, or is denied. In: `{ tool_name, failure_type, error }`. Observational.

---

## Shell

### `beforeShellExecution`
Before a shell command runs (matcher = command text).
- In: `{ command, cwd }`
- Out: `{ "permission": "allow"|"deny"|"ask", "user_message"?, "agent_message"? }`. For security gates set `failClosed: true` so a hook error blocks rather than allows.

### `afterShellExecution`
After a shell command. In: `{ command, output, exit_code, duration }`. Observational.

---

## Files

### `beforeReadFile`
Before the agent reads a file (also covers attachments/context reads).
- In: `{ file_path, content_size? }`
- Out: `{ "permission": "allow"|"deny", "agent_message"? }` — deny to redact/block.

### `afterFileEdit`
After the agent edits a file.
- In: `{ file_path, edits: [{ old_string, new_string }] }`. Observational.

---

## Subagents (Task tool)

### `subagentStart`
Before a Task subagent runs (matcher = subagent type).
- In: `{ subagent_type, prompt }`
- Out: `{ "permission": "allow"|"deny" }`.

### `subagentStop`
After a Task subagent finishes.
- In: `{ subagent_type, status }`
- Out: `{ "followup_message"?: "..." }` (bounded by `loop_limit`).

---

## Tab (inline completion)

### `beforeTabFileRead`
Before Tab reads a file for completion context. In: `{ file_path }`. Out: `{ permission }`.

### `afterTabFileEdit`
After Tab applies an edit. In: `{ file_path, edits }`. Observational.

---

## Environment available to every hook command

| Variable | Meaning |
|---|---|
| `CURSOR_PROJECT_DIR` / `CLAUDE_PROJECT_DIR` | workspace root (reliable) |
| `CLAUDE_PLUGIN_ROOT` / `CURSOR_PLUGIN_ROOT` | plugin install dir (reliable; usable in the `command` string) |
| `CURSOR_VERSION` | Cursor version |
| `CURSOR_USER_EMAIL` | signed-in user |
| `CURSOR_TRANSCRIPT_PATH` | path to the conversation transcript |

The hook process **cwd is unreliable** — never derive paths from `pwd`; use the env vars above.
