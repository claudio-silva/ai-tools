---
name: lite-chrome-automation
description: Control a real Chrome instance over the DevTools Protocol — evaluate JS in the page's main world, navigate, screenshot, set cookies, launch with unpacked extensions. Use for inspecting and debugging web pages or browser extensions, when the work is more about probing a live page than driving it, or when more powerful browser automation is not available.
allowed-tools: [exec, read]
---

# lite-chrome-automation

Drive Chrome via the DevTools Protocol (CDP) with `cdp.mjs` (next to this file): zero dependencies, Node ≥22 only. Covers launching, tab discovery, MAIN-world `Runtime.evaluate`, navigation, DOM-text waiting, cookie injection, and screenshots.

## Why a dedicated profile

Chrome ≥136 ignores `--remote-debugging-port` on the **default** user-data dir (security fix), so you can never attach to the user's everyday browser. This script launches a **separate instance** with its own profile and auto-allocated port — which also means no cookies or sessions carry over.

## Concurrency model

Every `launch` registers a **unique browser**: its own name, auto-allocated free port, and own profile dir, recorded in a shared registry at `$TMPDIR/lite-chrome/instances.json`. Unaware agents launched simultaneously never collide. Launch with `--name <label>` so you can find your own instance again later; `status` lists all live instances; `close <name>` frees it (`--purge` also deletes its auto-created profile). Close only the instances you launched.

## Workflow

1. **Launch** a dedicated instance (separate profile, CDP on an auto-allocated port):

   ```bash
   node cdp.mjs launch <url> [--name <label>] [--port N] [--profile dir] [--extension dir]
   ```

   - Default profile: fresh temp dir per instance (`/tmp/lite-chrome-<name>`); nothing persists unless `--purge` is skipped on close.
   - `--profile dir`: persistent profile — logins/sessions survive across launches.
   - `--extension dir`: loads an unpacked extension — enables developing and debugging extensions against real sites.
   - Prints `{name, port, pid, profile, ...}` — note the name for later commands.

2. **Auth for session-gated sites** — two options:
   - Ask the user to log in once in the launched window (their session lives in that instance's profile; persists if `--profile` was used).
   - Inject a session cookie yourself (automated path, no user step):

     ```bash
     node cdp.mjs cookie "sessionid=abc123" "csrftoken=xyz" --domain .example.com [--secure] [--httpOnly]
     node cdp.mjs nav https://example.com/app   # same name/--port
     ```

3. **Pick the tab**: `node cdp.mjs tabs [--name N | --port N]` — match by URL substring (first tab, else `tabs[0]`).

4. **Evaluate**: `node cdp.mjs eval "<expr>" [--name N | --port N] [--tab N]` — runs in the page's MAIN world; IIFEs/async work; object results are JSON-serialized; `eval @file.js` reads JS from a file (use for multi-line probes — avoids shell quoting).

5. **Navigate / wait / screenshot**: `node cdp.mjs nav <url>` · `node cdp.mjs wait "<text>" [--timeout s]` (polls `document.body.innerText`) · `node cdp.mjs shot out.png [--full]`.

6. **Cleanup**: `node cdp.mjs status` to see what's running; `node cdp.mjs close <name> [--purge]` when done. `close --all` kills every registered instance — only use when nothing else should be running.

## Interaction

No semantic layer — interact via plain DOM JS:

```js
document.querySelector('#login').click();
document.querySelector('[name=q]').value = 'x';
document.querySelector('[name=q]').dispatchEvent(new Event('input', {bubbles: true}));
```

React-ignored `input.value` setters need `Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, v)` + `input` event — standard CDP workaround.

## Limits (verified)

- Interactions are **untrusted DOM events** — fine for React-driven sites, but no real keyboard/mouse input (no `Input.dispatch*` wired in).
- No semantic/accessibility addressing — element lookup is CSS selectors in JS only.
- `launch` is macOS-only (`/Applications` lookup); on other platforms start Chrome yourself with `--remote-debugging-port=<N> --user-data-dir=<dir>` and attach via `--port` (or `CDP_PORT`). Manually-started browsers aren't in the registry: `--name`/`status`/`close` don't apply to them.
- Debug port grants full control over that Chrome — the dedicated-profile requirement is what keeps it safe.
- `eval` runs in the page's MAIN world; extension content scripts live in an isolated world and are not reachable — inspect them via the page DOM they modify instead.
