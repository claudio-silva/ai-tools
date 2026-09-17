# Execution behaviour

Read when persistence, permissions or deployment boundaries affect a job.

## Transport and lifetime

Linux targets require Python 3 and authenticated Codex on the login PATH. Any explicitly selected SSH destination can work; Desktop registration is unnecessary. OpenSSH configuration and known-host checks apply, BatchMode is enabled and agent forwarding is disabled. The helper does not install credentials or change SSH configuration.

Both dispatch modes start an independent remote worker. Sync additionally polls. Auto selects a systemd user service when the user manager and linger are available, otherwise a detached process. Detached workers ignore SIGHUP, close SSH streams and start a separate session. Logout cleanup, reboot and OOM can still terminate them. The helper never enables linger or replays jobs after reboot.

A Desktop SSH proxy and its app-server may have separate lifetimes. Inspect the actual deployment before inferring what Desktop exit does. The helper's jobs have their own lifecycle. Local relay loss stops local reporting; it does not cancel remote work.

Job IDs, atomic state and launch locks prevent duplicate dispatch on identical retries. Boot ID and process start time detect stale/recycled PIDs. A lost response means unknown state; reconnect using the same job ID before considering another run.

## Permissions and containers

The remote process has the selected SSH account's identity. Codex sandbox policy does not change that identity. Fresh approvals cannot be answered through this noninteractive backend; approval-required actions fail. Full access requires task authority. Use an interactive workflow when live approvals are needed.

Codex executes in the SSH target environment. A working-directory option does not select a container. Container isolation requires the runtime and tools inside that boundary. Rootful Docker socket access confers effective host-root authority. A host agent invoking docker exec retains host permissions.

Remote account/project instructions, hooks, skills and MCP configuration apply; local conversation and skills are not automatically inherited. Job prompts, metadata, logs and results are stored privately under ~/.local/state/codex-remote/jobs and may contain sensitive data. No automatic cleanup or backup is provided.

## Reporting and limits

The relay is a native local subagent; client UI determines its placement. Remote events are not injected as native local tool calls. Stopping the relay does not cancel the job; explicit cancel writes a request observed by the remote worker.

Timeout/cancellation terminates the Codex process group without undoing external effects or guaranteeing termination of intentionally detached services. Time limits are not token budgets. Successful exit is not acceptance. Reconcile failures or unknown states before repeating external effects.

Model aliases live in remote.py. Verify support on the target; cached catalogue information is advisory. Never silently replace unsupported models or efforts. Relay model selection is independent of remote worker model selection.

## References

- [Codex noninteractive mode](https://learn.chatgpt.com/docs/non-interactive-mode)
- [Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents)
- [Codex remote connections](https://learn.chatgpt.com/docs/remote-connections)
- [systemd loginctl](https://www.freedesktop.org/software/systemd/man/latest/loginctl.html)
- [Docker privileges](https://docs.docker.com/engine/install/linux-postinstall/)
