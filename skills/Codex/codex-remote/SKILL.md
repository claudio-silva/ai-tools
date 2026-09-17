---
name: codex-remote
description: Delegate work to remote Codex workers through a visible local Luna relay. Use in the main conversation to prepare remote tasks, choose worker models and permissions, and collect results.
---

# Remote delegation — operator

This skill is for the main agent. Its scripts and operational reference live in this directory. Local execution is performed by the global `codex_remote_relay` custom subagent, whose TOML definition is installed separately. Do not load the relay's full procedure into the operator's context: pass this skill's resolved absolute directory as the helper directory in the relay assignment.

Use one native local `codex_remote_relay` relay per remote job, with **no conversation-history fork**. Its custom-agent configuration selects **gpt-5.6-luna / low**. The client displays its ordinary subagent activity; exact sidebar placement is client-dependent. Remote jobs themselves are not registered as native children. If the custom subagent, Luna/low, or native subagents are unavailable, report that limitation rather than silently changing the workflow or model. Explicit user overrides take precedence.

## Prepare the task

Resolve the authorized SSH target, absolute remote working directory, outcome, scope, permissions, remote worker model/effort, timeout and desired observation mode. Any selected SSH alias or user@host can work; Desktop registration is unnecessary. The remote environment requires Linux, Python 3 and authenticated Codex. Discovery is not permission to contact every configured host.

Worker model and effort are independent of the Luna relay. Select Luna, Terra, Sol, Astra or an explicit model ID according to the user's request or a disclosed task-appropriate choice. Default to read-only; use workspace-write for authorized project work. Host administration/full access requires authority covering that task. Use isolated workspaces or non-overlapping paths for parallel writes.

Write the complete worker prompt once to a UTF-8 file accessible to the local relay. Include relevant instructions, limits and acceptance criteria; the remote worker does not inherit this conversation or local skills. Reuse existing material through code where possible. Do not place the large prompt in the relay assignment, and do not fork the parent's history. Allocate a stable job ID and a new absolute local result-file path.

## Delegate through the custom relay agent

Create the global custom subagent `codex_remote_relay`, with no inherited conversation history (`fork_turns: "none"` where supported). Its TOML definition controls the relay model and effort; do not silently replace that agent with a generic child. When the client cannot select a named custom subagent, report the limitation rather than approximating the workflow with a generic child.

Supply this compact assignment, filling in the references and execution values:

```text
You are the local relay defined by `codex_remote_relay`; do not solve the worker task or spawn another relay.
Operation: start | reattach | cancel
Public task label: <short label>
Helper directory: <absolute path to this codex-remote skill directory>
SSH target: <alias or user@host>
Job ID: <stable unique ID, or existing ID for reattach/cancel>
Remote working directory: <absolute remote path>
Worker model / reasoning effort: <selected values>
Sandbox and authority: <selected policy and concise scope>
Maximum runtime: <seconds>
Prompt file: <absolute local path; start only>
Result file: <new absolute local path>
Observation: observe | dispatch-only
Do not read or repeat the prompt/result document; transfer through the helper.
Return compact state, host/job ID, result path and any actionable blocker.
```

Only include fields needed for the operation. Reattach/cancel do not require the original task prompt. The relay receives its procedure from the custom-agent definition and obtains the helper only from the supplied directory.

## Observe and accept

- Prefer **observe** when the user values visible progress: the relay keeps watching until a terminal state or blocker. The main agent may wait for it (synchronous usage) or continue other work while it runs (asynchronous usage).
- Use **dispatch-only** when explicitly desired: the relay returns after launching. Its UI may show Done while the remote job continues; do not present this as a live status indicator.
- Preserve host/job ID and the local relay reference in the task's normal records. Send follow-ups to the existing relay where possible; if it is unavailable, create a fresh Luna/low relay using the existing job ID. Never create a second job merely because a reply was lost.
- Stopping a local subagent is not remote cancellation. Delegate an explicit cancel operation and confirm the remote terminal state. Local app loss stops reporting; it does not automatically stop detached remote work or create a future notification.
- Evaluate the result and relevant artifacts against acceptance in the main agent. Read the saved result directly as needed, without asking Luna to reproduce it. Successful process exit is not proof of acceptance.

The relay handles probing, transport, waits, bounded diagnostics and collection. It escalates problems to the operator; it does not change scope, worker model or privileges on its own.
