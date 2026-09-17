# browser-automation

Real-browser end-to-end testing through `browse` — a stateful driver over the `cua-driver` CLI that turns browser automation into single commands: allocate an isolated context, run an entire multi-step flow in **one `batch` call**, and get back verified results. No MCP server required.

## Features

- **Whole flows in one call** — `batch` executes an entire scenario (`open` → `type` → `press` → `wait-for` → `shot`) in a single invocation, the big speed/token win over step-at-a-time driving; batch lines may carry `--ctx` to drive several browsers at once
- **Managed browser lifecycle** — `new-context` is pre-flight and allocator in one: it verifies `cua-driver` is installed, mints a collision-free context name (parallel agents can't collide), and `close` reaps what you own
- **Verification built in, not bolted on** — every `open`/`click`/`type` ends with a fresh snapshot; refs are re-resolved inside each action so they can't go stale; `--wait-for` polling replaces manual sleeps
- **Isolated driver-owned Chromium** — never touches the user's profile or daily browser; input is delivered in background without stealing focus
- **Rich page access** — `snap`/`find`/`tree`/`text`/`read`/`dom` for reading; `eval` for page JS with a transparent DevTools-endpoint fallback; `cdp` exports endpoints for external tooling like `playwright-core`
- **Evidence capture** — viewport screenshots, per-action before/after folders, and video recording via macOS ScreenCaptureKit (`record --video`)
- **Parallel browsers** — independent named contexts for two-tab / multi-user tests
- **Honest limits** — the verified "cannot do" list (no text-range selection, no native browser chrome, no keyboard modifiers, no trusted-input bypass) is documented in SKILL.md rather than discovered at runtime

## Requirements

- `cua-driver` CLI on `PATH` (macOS, Windows or Linux)
- Python 3 — `browse` is stdlib-only

## Usage

A whole smoke test in two calls:

```sh
CTX=$(browse new-context smoke)
browse --ctx $CTX batch <<'EOF'
open http://127.0.0.1:8787/
type "Username" demo --replace
type "Password" secret --replace
press enter
wait-for "Dashboard"
shot /tmp/after-login.png
EOF
browse close $CTX
```

Parallel browsers for a two-user scenario:

```sh
A=$(browse new-context alice); B=$(browse new-context bob)
browse --ctx $A open http://127.0.0.1:8787/listen/ABC123
browse --ctx $B open http://127.0.0.1:8787/listen/ABC123
browse --ctx $A click "Listen"
browse --ctx $B snap
browse close $A $B
```

## Install

Copy this folder to `~/.config/devin/skills/browser-automation/` — or the skills directory of any environment where `cua-driver` is installed.

## Contents

| Path | Purpose |
| --- | --- |
| `SKILL.md` | Command reference, parallel-context patterns, verified can/cannot limits, operating rules |
| `browse` | The driver script — stdlib Python, per-context state in `$TMPDIR/cua-browse/` |
| `NOTES.md` | Internals and verified findings: transport behavior, MCP-bridge limits, session ownership |
