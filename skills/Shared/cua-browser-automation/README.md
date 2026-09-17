# cua-browser-automation

Real-browser end-to-end testing through `browse` — a stateful driver over the `cua-driver` CLI that turns browser automation into simple commands: allocate an isolated context, run an entire multi-step flow in **one `batch` call**, and get back verified results. No MCP server required.

Where lightweight CDP probes (see `lite-chrome-automation`) answer "what is this page doing", this skill answers "does the whole flow actually work" — semantic element addressing, per-action re-verification, and evidence capture, driving a real browser the way a human tester would.

## Why CLI, not the cua-driver MCP

This skill exists because, when it was written, **cua-driver's MCP server was not usable for the typed browser loop on Devin or on Cursor**. Both hosts forward only the MCP text `content` channel to the model and drop `structuredContent`. cua-driver puts the IDs the next call needs — `window_id`, `tab_id`, `element_token`, `snapshot_id` — in `structuredContent`, while the text is a summary (`Found N window(s)`, `bound … with N tab(s)`). Without those IDs the agent cannot navigate, snapshot, click, or type.

That is a host limitation, not a broken driver. The same tools return complete JSON over the CLI, and the official cua-driver skill already treats CLI as the default transport. `browse` is the portable way to make cua available to browser agents while MCP clients omit structured results. Internals of that finding live in `NOTES.md`.

Even if a host later forwards `structuredContent` correctly, this wrapper can still be worth keeping:

- whole flows in one `batch` call (fewer round-trips than a chain of MCP tools)
- collision-free context names across parallel agents and IDEs
- transparent `eval` DevTools fallback when the gated JS tool refuses
- exported CDP endpoints, full-page screenshots, parallel isolated browsers
- a host-agnostic CLI that behaves the same on Codex, Claude Code, Devin, and Cursor

## Cursor

On Cursor, **do not install this skill as the default way to verify a web app**. Cursor already has a built-in browser-automation tab (semantic snapshots, click/type/fill, screenshots) that covers typical development workflows better: no extra daemon, refs stay in the conversation, and you do not need `cua-driver`.

The cua-driver MCP plugin on Cursor hits the same structured-content drop as Devin, so it is not a substitute for this CLI wrapper or for Cursor's built-in browser.

Use this skill on Cursor only when you specifically need what the built-in tab cannot do (isolated real Chromium, parallel browsers, `batch`, video evidence, file upload, Playwright CDP attach) **and** you are willing to drive it via `browse` rather than MCP.

## Features

- **Whole flows in one call** — `batch` executes an entire scenario (`open` → `type` → `press` → `wait-for` → `shot`) in a single invocation, the big speed/token win over step-at-a-time driving; batch lines may carry `--ctx` to drive several browsers at once
- **Managed browser lifecycle** — `new-context` is pre-flight and allocator in one: it verifies `cua-driver` is installed, mints a collision-free context name (parallel agents can't collide), and `close` reaps what you own
- **Verification built in, not bolted on** — every `open`/`click`/`type` ends with a fresh snapshot; refs are re-resolved inside each action so they can't go stale; `--wait-for` polling replaces manual sleeps
- **Isolated driver-owned Chromium** — never touches the user's profile or daily browser; input is delivered in background without stealing focus
- **Rich page access** — `snap`/`find`/`tree`/`text`/`read`/`dom` for reading; `eval JS|@FILE` runs JS in the page's MAIN world (driver tool first, transparent DevTools-endpoint fallback); `cdp` exports browser/page endpoints for external tooling like `playwright-core`
- **Evidence capture** — viewport or full-page (`shot --full`) screenshots, per-action before/after folders, and video recording via macOS ScreenCaptureKit (`record --video`)
- **Parallel browsers** — independent named contexts for two-tab / multi-user tests
- **Honest limits** — the verified "cannot do" list (no text-range selection, no native browser chrome, no keyboard modifiers, no trusted-input bypass, no user extensions) is documented in SKILL.md rather than discovered at runtime

## Requirements

- `cua-driver` CLI on `PATH` (macOS, Windows or Linux)
- Python 3 — `browse` is stdlib-only
- A supported browser (Chrome/Chromium/Edge/Brave) running, so `browser_prepare` can clone the product into an isolated instance

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
shot /tmp/after-login.png --full
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

Copy this folder into the skills directory of any environment where `cua-driver` is installed:

| Agent | Install location |
| --- | --- |
| **Codex** | `~/.codex/skills/cua-browser-automation/` |
| **Cursor** | `~/.cursor/skills/cua-browser-automation/` |
| **Claude Code** | `~/.claude/skills/cua-browser-automation/` |
| **Devin** | `~/.config/devin/skills/cua-browser-automation/` |

## Contents

| Path | Purpose |
| --- | --- |
| `SKILL.md` | Command reference, parallel-context patterns, verified can/cannot limits, operating rules |
| `browse` | The driver script — stdlib Python, per-context state in `$TMPDIR/cua-browse/` |
| `NOTES.md` | Internals and verified findings: transport behavior, MCP-bridge limits, session ownership |
