# Operational choices

## SSH, Desktop and process lifetime

Any explicitly selected SSH destination can work; registration in Desktop is unnecessary. The helper uses OpenSSH config, known-host checks and `BatchMode=yes`, with agent forwarding disabled. It neither edits SSH config nor weakens host verification. Login-shell configuration must expose `python3` and `codex`; noisy startup files can corrupt machine-readable output.

An SSH proxy disconnect and a remote server shutdown are different events. Current Codex versions may run a managed daemon reached by `codex app-server proxy`. Inspect `codex app-server daemon version`, process ancestry and service configuration before concluding that app exit kills a task. Do not stop Desktop, SSH proxies or a shared daemon to test persistence without a scoped experiment. Daemon capabilities vary by CLI version.

The helper's detached worker is independent of Desktop and the initiating SSH connection. A new SSH connection reads status and results. `nohup` alone does not provide job identity, status or recovery. `command > agent.pid &` writes command output to that file, not its PID; shell PID recording would use `$!`, but a PID alone is not a durable job identity.

For Linux, prefer systemd user services plus explicitly configured linger for operation after full logout. The helper never enables linger or installs services globally. Transient jobs are not replayed after reboot; lost work needs reconciliation before a new dispatch. The state files distinguish a worker's boot ID and process start time to avoid interpreting a recycled PID as a running job. Atomic state and a per-job lock prevent duplicate launch on retries.

## Privileges and containers

Recommended bootstrap: a dedicated unprivileged development account, project workspaces, and user-managed execution. Use a separate administrator connection for explicit host provisioning. Keep host changes reproducible (scripts/configuration) and narrow privileged operations where practical.

Docker is useful isolation only with appropriate boundaries. Membership of the rootful Docker group or access to its socket effectively grants host-root capabilities. Giving that socket to an agent container does not remove host authority. Prefer rootless Docker/Podman when compatible, or a narrowly controlled launcher that starts nonprivileged containers with selected mounts and no daemon socket inside. Docker is not a VM security boundary.

This helper runs Codex on the SSH target, not automatically inside Docker. To execute entirely within a container, use a separately configured SSH execution target/login environment with Codex, Python, authentication and workspace inside that boundary, or implement a reviewed container backend. Do not pretend `--cwd` selects a container. Running a host agent that calls `docker exec` still leaves the agent on the host.

## Models and approvals

Aliases at creation (2026-09-09): luna = gpt-5.6-luna, terra = gpt-5.6-terra, sol = gpt-5.6-sol, astra = gpt-6-astra. CLI/account availability may differ; probe returns cached model capabilities when the host exposes them. Explicit full IDs are supported for future releases. Effort is sent through `model_reasoning_effort`; an unsupported pair fails rather than falling back.

Noninteractive runs cannot ask the local founder for fresh permission. Read-only/workspace-write runs fail approval-required operations; full access is an explicit privilege choice. Use an interactive remote task when live approvals are important. Host user config, project instructions, hooks and MCP servers apply; inspect them before trusting a new executor. Do not equate a requested sandbox string with an independently audited sandbox.

## Validation baseline (2026-09-09)

Linux CLI 0.153.4: authenticated probe passed; real Luna/low read-only async and sync jobs completed. Async completed approximately 28 seconds after dispatch while the initiating SSH connection had already exited; its result was fetched through a separate connection. This tests the helper's detach, not Desktop app exit, Mac sleep, full logout, reboot or network outage. Six Linux tests with a simulated CLI covered exact prompt transport, idempotent launch, changed-request rejection, cancellation, timeout, turn.failed with exit zero, invalid IDs and stale process identity. The systemd backend was not exercised live because the target had Linger=no; its prerequisite check intentionally refused that topology. Only Luna was called live; other model/effort support came from the remote cached catalogue.

## Sources

- [Codex noninteractive mode](https://learn.chatgpt.com/docs/non-interactive-mode): JSON events, saved authentication, output-last-message.
- [Codex remote connections](https://learn.chatgpt.com/docs/remote-connections): Desktop SSH integration and mobile topology.
- [Codex app server](https://learn.chatgpt.com/docs/app-server): client/server protocol and experimental transport.
- [systemd loginctl](https://www.freedesktop.org/software/systemd/man/latest/loginctl.html): linger and user manager lifetime.
- [Docker rootless](https://docs.docker.com/engine/security/rootless/) and [Docker post-install](https://docs.docker.com/engine/install/linux-postinstall/): rootless execution and Docker group authority.
