# lite-chrome-automation

An agent skill that controls a real Chrome instance over the Chrome DevTools Protocol (CDP): launch dedicated browser instances, evaluate JavaScript in the page's main world, navigate, wait for content, inject cookies, and take screenshots.

## Why this skill exists

Agents are surprisingly capable at driving a browser *without* a framework — the Chrome DevTools Protocol exposes everything a page is doing, and a single WebSocket connection is enough to evaluate JS, navigate, and screenshot. What's missing is the **glue**: Chrome's debugging port, tab discovery, and a few well-known footguns (Chrome ≥136 ignores the debug port on the default profile; page context vs. isolated worlds; shell quoting for long JS probes).

This skill packages that glue into a ~200-line, zero-dependency Node script plus concise instructions, so an agent can go from "I need to inspect that live page" to evaluating JS in a real Chrome session in one command — even on session-gated sites, and even when several unaware agents need browsers at the same time.

Use it when the work is **probing rather than driving** — DOM/geometry inspection, live mutation, page-internal state — or when heavier browser-automation tooling is not available. For verified end-to-end user flows (semantic addressing, per-action re-verification, evidence capture), a semantic automation layer such as `cua-browser-automation` remains the better fit; the two complement each other.

## Features

- **Zero dependencies** — Node ≥22 built-in `fetch`/`WebSocket`, no npm install, no daemon, no MCP server.
- **Concurrency-safe** — every launch gets a unique name, an auto-allocated port, and its own profile, tracked in a shared registry (`$TMPDIR/lite-chrome/instances.json`). Unaware agents never collide; `status`/`close` manage the fleet.
- **Main-world evaluation** — real `Runtime.evaluate`: see the page's own objects (patched `fetch`, React state, `performance` entries), not a sandboxed view.
- **Session-gated pages** — either let the user log in once in the launched window, or inject session cookies via `Network.setCookie` for fully automated suites.
- **Extension development** — `--load-extension` preloads an unpacked extension, so agents can develop and debug browser extensions against real sites.
- **Screenshots** — viewport or full-page PNG capture via `Page.captureScreenshot`.
- **`@file` evaluation** — run multi-line JS probes from a file, avoiding shell quoting bugs.

## Requirements

- Node.js ≥ 22 (uses built-in `fetch` and `WebSocket`)
- macOS Chrome for the `launch` command (browsers located under `/Applications`); on other platforms start Chrome yourself with `--remote-debugging-port=<N> --user-data-dir=<dir>` and attach via `--port` (or `CDP_PORT`)

## Quick start

```bash
# Launch a dedicated instance (unique name, free port, own temp profile)
node cdp.mjs launch "https://example.com" --name my-probe

# Session-gated page? Either ask the user to log in in that window,
# or inject a session cookie:
node cdp.mjs cookie "sessionid=abc123" --domain .example.com --name my-probe
node cdp.mjs nav "https://example.com/app" --name my-probe

# List tabs, evaluate JS in the page's main world
node cdp.mjs tabs --name my-probe
node cdp.mjs eval "document.title" --name my-probe
node cdp.mjs eval @probe.js --name my-probe        # multi-line JS from file

# Wait for content, screenshot, clean up
node cdp.mjs wait "Dashboard" --name my-probe
node cdp.mjs shot page.png --full --name my-probe
node cdp.mjs close my-probe --purge
```

Every command that touches a browser takes `--name <label>` **or** `--port <N>` (or `--all` where noted).

## Commands

| Command | Description |
| --- | --- |
| `launch [url]` | Launch a dedicated Chrome instance. Options: `--name`, `--port`, `--profile dir`, `--extension dir`, `--app <path>`. Prints `{name, port, pid, profile, wsUrl}`. |
| `status` | List registered instances with liveness and current URL. |
| `tabs` | List open tabs. Options: `--name`/`--port`/`--all`. |
| `eval <expr \| @file>` | Evaluate JS in the page's MAIN world; JSON output. Options: `--tab N` (default: first non-chrome tab). |
| `nav <url>` | Navigate the selected tab. |
| `wait <text>` | Poll `document.body.innerText` for text. Options: `--timeout s` (default 15). |
| `cookie <name=value>...` | Set cookies via `Network.setCookie`. Options: `--domain` (required), `--path` (default `/`), `--secure`, `--httpOnly`, `--sameSite Lax\|Strict\|None`. |
| `shot <file.png>` | Screenshot to PNG; `--full` captures the full page (`captureBeyondViewport`). |
| `close` | Close instances by `--name`/`--port`/`--all`; `--purge` also deletes auto-created profiles. |

## How concurrent use stays safe

Unrelated agents launching browsers at the same time can't collide:

- names are auto-suffixed (`probe`, `probe-1`, `probe-2`, …) or random (`chrome-XXXXXX`),
- ports are OS-allocated (`listen(0)`), never hardcoded,
- each instance gets its own profile dir (`/tmp/lite-chrome-<name>`),
- a shared registry at `$TMPDIR/lite-chrome/instances.json` (atomic writes) lets `status` and `close` find everything that's running.

`--profile <dir>` opts into a persistent, reusable profile instead — logins and cookies then survive across launches (useful for session-gated development loops).

## Session cookies without manual login

For automated access to session-gated pages, inject cookies before navigating:

```bash
node cdp.mjs launch "https://example.com" --name suite
node cdp.mjs cookie "sessionid=abc123" \
  --domain .example.com --secure --httpOnly --name suite
node cdp.mjs nav "https://example.com/app" --name suite
```

`cookie` writes to the browser's shared cookie store, so it works from `about:blank` — no need to visit the domain first. Required per cookie: `name=value` plus `--domain` (leading `.` covers subdomains). Equivalent to `document.cookie`, except `--httpOnly`/`--secure`/`--sameSite`/`--expires` are supported.

## Why a dedicated profile?

Since Chrome 136, `--remote-debugging-port` is ignored on the **default** user-data directory (a security fix against cookie-theft malware). This skill therefore always launches a separate instance with its own profile — you can never attach to the user's everyday browser, and its cookies/sessions are not present. That trade-off is a feature: the automated browser is disposable and can't leak the user's real sessions.

## Installation

`cdp.mjs` is standalone — copy it next to `SKILL.md` and place the directory where your agent discovers skills:

| Agent | Install location |
| --- | --- |
| **Codex** | `~/.codex/skills/lite-chrome-automation/` |
| **Cursor** | `~/.cursor/skills/lite-chrome-automation/` |
| **Claude Code** | `~/.claude/skills/lite-chrome-automation/` |
| **Devin** | `~/.config/devin/skills/lite-chrome-automation/` |

Or keep it anywhere and point agents at `SKILL.md` — the script has no configuration.

## Limitations

- **No semantic layer** — interaction is plain DOM JS (`el.click()`, `dispatchEvent`), not "click the button labeled Save". Simple for agents, but less robust than accessibility-driven tools for complex flows.
- **Untrusted events only** — `Input.dispatch*` isn't wired in, so no trusted keyboard/mouse input or native file pickers.
- **`launch` is macOS-only** — other platforms: start Chrome with `--remote-debugging-port=<N> --user-data-dir=<dir>` (the debug port is ignored on the default profile) and attach via `--port`. Manually-started browsers aren't in the registry, so `--name`, `status`, and `close` don't apply to them.
- **Isolated worlds unreachable** — `eval` runs in the page's MAIN world; extension content scripts can't be inspected directly (probe the DOM they modify).
- **Full control** — the debug port gives complete control of that instance; the dedicated-profile design is what makes that safe.

## Files

- `SKILL.md` — agent-facing instructions (discovery, usage, caveats)
- `cdp.mjs` — the CLI (zero dependencies, Node ≥22)
- `README.md` — this file
