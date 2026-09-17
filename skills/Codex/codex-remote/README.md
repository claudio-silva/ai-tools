# codex-remote

Delegate bounded tasks to Codex running on remote SSH servers — synchronously or asynchronously — through a visible local relay subagent, with durable job IDs, status polling, result collection and cancellation.

## Architecture

Three components with deliberately separated concerns:

- **Operator** (`SKILL.md`) — the main agent. Prepares the task, chooses worker model/effort and permissions, evaluates results against acceptance. Never runs the transport itself.
- **Relay** (`agents/codex_remote_relay.toml`) — a `gpt-5.6-luna` / low local subagent that owns exactly one remote job: probe → dispatch → observe → collect. Prompts and results transfer **by file**, never through the relay's model context — large documents never inflate the relay's bill.
- **Helper + worker** (`scripts/remote.py`, `scripts/worker.py`) — a stateful Python CLI pair. The local helper does probe/start/status/result/logs/cancel over SSH; the remote worker runs the job detached (systemd user service when the user manager and linger are available, otherwise a SIGHUP-immune detached process).

## Features

- **Durable job IDs** — generated before the host is contacted; a lost launch response is reconciled by re-querying the same ID, never by blindly retrying
- **Model aliases** — `luna`/`terra`/`sol`/`astra` plus explicit model IDs; availability probed per host, never silently substituted
- **Conservative authority** — `read-only` default; `workspace-write` for authorized changes; `--allow-full-access` only under explicit task authority. An SSH root login stays root regardless of sandbox
- **Noninteractive-safe** — remote Codex runs with approvals `never`; approval-gated actions fail instead of hanging
- **Private state** — jobs live under `~/.local/state/codex-remote/jobs/<id>/` with private permissions; bounded reads for long logs/results

## Requirements

- **Local**: Codex with custom subagents; the relay TOML installed in `~/.codex/agents/`; Python 3
- **Remote**: Linux, Python 3, authenticated Codex on the login PATH, working SSH auth (`BatchMode`, no agent forwarding)

## Install

```sh
cp agents/codex_remote_relay.toml ~/.codex/agents/
cp -R . ~/.codex/skills/codex-remote/
```

Restart Codex so it discovers the relay role.

## Contents

| Path | Purpose |
| --- | --- |
| `SKILL.md` | Operator procedure: prepare, delegate through the relay, observe/accept |
| `agents/codex_remote_relay.toml` | Relay subagent definition (model pin + full operating instructions) |
| `scripts/remote.py` | Local helper CLI — `probe`, `start`, `status`, `result`, `logs`, `cancel` |
| `scripts/worker.py` | Remote-side job runner |
| `scripts/test_worker.py`, `scripts/test_transport.py` | Tests |
| `references/operations.md` | Persistence, permissions, containers, failure modes, limits |
