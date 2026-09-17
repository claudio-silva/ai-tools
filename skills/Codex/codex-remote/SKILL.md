---
name: codex-remote
description: Delegate bounded tasks to Codex on SSH servers, synchronously or asynchronously, with explicit model, reasoning effort, durable job IDs, status, results and cancellation. Use for remote development, heavy builds, or authorized server administration.
---

# Remote Codex delegation

Use the bundled Python helper for independently running Linux jobs. This is delegation through SSH and Codex CLI, not a native child in the Desktop subagent tree. Jobs need no Desktop registration and do not automatically appear in its sidebar. Desktop task tools remain useful when the user explicitly wants a native app task.

## Select the target and mandate

- Resolve the user-selected SSH alias or explicit `user@host`; use `ssh -G TARGET` to inspect effective user/hostname. Concrete aliases in SSH config (including Include files) are candidates, not permission to contact every host. Never scan all registered machines or copy credentials automatically. Desktop registration alone is not an SSH address: resolve its actual connection first.
- Require working SSH authentication, Linux, Python 3, and authenticated Codex on the target's login PATH. Run `probe`; it reports UID, CLI version, model catalogue when exposed, and persistence prerequisites. A missing model catalogue means unverified availability; execution may fail. Never silently substitute a model or effort.
- State remote absolute working directory, outcome, scope, acceptance, model and effort. Select among Luna, Terra, Sol and Astra using the user's choice, or make a task-appropriate choice and disclose it. The helper accepts aliases and explicit model IDs; consult `--help`. Other model IDs/efforts are passed exactly and must be verified on the host.
- Default to `read-only`; use `workspace-write` for authorized project changes. Host administration or `danger-full-access` requires authorization covering those privileges; `--allow-full-access` records that deliberate choice. A root SSH login remains root even in read-only mode. This skill never grants sudo, Docker socket access or extra authority by itself.
- The prompt must include relevant instructions, acceptance criteria and limits: a remote CLI session does not inherit the local conversation or local skills. Use isolated worktrees or non-overlapping paths for concurrent writes. Do not copy a repository or install prerequisites unless within the requested task.

## Dispatch and collect

Run `python3 <skill-dir>/scripts/remote.py --help`. All responses are JSON. Prompts go through a UTF-8 file or stdin, never interpolated into shell code. The helper uses the remote user's saved Codex auth, disables fresh approval prompts (`never` means approval-required actions fail), and preserves normal sandbox enforcement. It does not bypass approvals or enable automatic review.

```bash
python3 <skill-dir>/scripts/remote.py probe --host devbox
python3 <skill-dir>/scripts/remote.py start --host devbox --cwd /srv/project --model luna --effort medium --prompt-file /absolute/task.txt --mode async
python3 <skill-dir>/scripts/remote.py start --host devbox --cwd /srv/project --model sol --effort high --sandbox workspace-write --prompt-file /absolute/task.txt --mode sync --wait-seconds 45
python3 <skill-dir>/scripts/remote.py status --host devbox --job JOB_ID
python3 <skill-dir>/scripts/remote.py result --host devbox --job JOB_ID
python3 <skill-dir>/scripts/remote.py logs --host devbox --job JOB_ID
python3 <skill-dir>/scripts/remote.py cancel --host devbox --job JOB_ID
```

- Both modes first detach the job remotely. Sync polls within a `--wait-seconds` window (default 45, plus SSH call latency), then returns the current state without cancelling. Continue bounded waits or other work as appropriate. Async returns after dispatch. Persist host, job ID, remote state path, mandate and verification limits in the task's normal records.
- A generated job ID is printed to stderr **before** contacting the host. A lost launch response is ambiguous: query that same ID; never retry with a new ID automatically. Explicit `--job` supports idempotent retries of exactly the same request. Existing IDs cannot start another run.
- `--max-seconds` (default 3600) is a wall-clock limit, not a token/cost budget. Cancellation requests and timeout terminate the process group created for Codex. They cannot undo external effects or guarantee termination of deliberately detached processes/services created by the task.
- Inspect status, final response and relevant evidence; exit zero alone does not prove acceptance. `failed`, `timed_out`, `cancelled`, `launch_failed` and `lost` are not completion. An unreachable server means unknown state. Do not resubmit work with external effects without reconciliation.
- Jobs live under the remote user's `~/.local/state/codex-remote/jobs/JOB_ID` with private permissions. Prompt, metadata, events and final output can contain sensitive project data. No automatic deletion or log transfer. Output reads are bounded; use offsets for longer logs/results.

## Persistence and execution boundaries

Read [operations.md](references/operations.md) for disconnects, service managers, containers, security and known limitations. Prefer `--backend auto`: it chooses a systemd user service only when the user manager and linger are available; otherwise it uses a detached process. `--backend systemd` refuses missing prerequisites. No backend auto-restarts a task after failure or reboot.

Detached mode closes SSH streams, ignores SIGHUP and starts a separate session. It survives ordinary SSH disconnects but is not immune to host logout cleanup, reboot or OOM. Do not promise 24/7 operation solely from a successful detached launch. If persistence across complete logout is required, arrange an authorized service-manager setup and test it.

Remote work does not wake or notify this local conversation by itself. Use the product's monitoring/automation mechanism only when requested; record whether that monitor itself depends on the local machine. Never claim mobile reachability or independence from the Mac without testing the actual topology.
