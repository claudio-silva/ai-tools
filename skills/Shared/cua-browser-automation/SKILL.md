---
name: cua-browser-automation
description: Drive a real browser the way a human tester would: open the app, click through a flow, read what rendered, and capture proof. Use it when the thing being verified is what a browser actually renders and how a real user interacts with it — smoke-test a freshly implemented feature end-to-end, automate a service that exposes no API, reach flows HTTP tests cannot, verify responsive layouts, and capture evidence.
---

# cua-browser-automation

Real-browser testing through the `browse` wrapper script — one shell call per
step, no manual plumbing. It launches an isolated driver-owned Chromium
(it never touches the user's profile or daily browser), keeps a snapshot → act
→ verify loop internally, and prints compact actionable output.

## First call — always

```sh
CTX=$(<skill-dir>/browse new-context mytest)
```

`new-context` is the pre-flight **and** the allocator: it fails with an install
hint if `cua-driver` is absent (never substitute shell Chrome, `webfetch`/`curl`,
or desktop AX/pixel tools — a fetched body is not a rendered page), and prints a
globally-unique context name. Pass it as `--ctx` on every later call. Context
names are generated, not chosen — parallel agents on this machine, including
agents from other IDEs/harnesses, can never collide.

The browser **never needs to be focused or foregrounded** — input is delivered
in background and the user can keep working in other windows mid-run.

## Commands

Invoke as `<skill-dir>/browse <cmd> --ctx $CTX` (or `python3 <skill-dir>/browse`)
— `browse` is the script next to this file. `--ctx` defaults to `main` when omitted —
fine for quick interactive use, but agents should always use `new-context`.

| Command | What it does |
| --- | --- |
| `new-context [LABEL]` | Verify cua-driver + allocate a unique ctx name (prints it). |
| `open URL [--profile P] [--wait-for T]` | Launch/reuse the isolated browser, navigate, print the snapshot. `--profile` = persistent scratch profile (keeps logins). `--wait-for` polls until text appears before returning. |
| `snap [--full] [--query T] [--scope REF] [--more TOKEN]` | Fresh snapshot: `ref role name actions` table of actionable elements. Truncated pages print a continuation token — `--more` fetches the next chunk; `--scope` reads one subtree only. |
| `find TEXT` | Semantic search over role/name/visible text; shows matching content + actionable refs. |
| `click TARGET [--wait-for T]` | Click by accessible-name text, `p<i>:<j>` ref, or `css=SELECTOR`. Prints `navigated -> URL` or `same page` after. |
| `type TARGET TEXT [--replace] [--keystrokes] [--wait-for T]` | Types the whole string at once (insert-at-caret). The driver focuses the field itself. `--replace` = select-all-then-type (i.e. *set* the value; empty TEXT + `--replace` clears). `--keystrokes` = per-char key events for fields that reject bulk inserts. |
| `press KEY [TARGET]` | Real key event — `enter` (submits forms), `tab` (moves focus), `escape`, `backspace`, `delete`, `space`. TARGET names the field; omit it to use the page's single editable. |
| `batch [FILE]` | One command per line from FILE or stdin — `click Sign in`, `type User john`, lines may carry `--ctx` to drive several browsers in one call. Stops on first failure (`--keep-going` to continue). The big speed/token win. |
| `tree` | Concise semantic outline of the page (AX+DOM tree text) — better than `snap` for understanding structure. |
| `files TARGET PATH...` | Assign local files to a file input (absolute paths, no symlinks). |
| `hover` / `right-click` / `double-click TARGET` | Pointer actions on an element. |
| `scroll DY [TARGET]` / `drag TARGET TO` | Scroll (dy px, optional element) / drag element→element or `x,y`. |
| `shot [PATH] [--full]` | Tab-viewport PNG to PATH (default: tmp file; path printed). `--full` captures the whole scrollable page via CDP `captureBeyondViewport`. Prints the px→css scale when ≠1 — divide measured PNG coords by it before passing `x,y` to `scroll`/`drag`. |
| `text` | Rendered page text (may include browser-UI strings — popups/menus). |
| `read TARGET` | Print one element's role/name/value/states — extract a generated code or field value (e.g. the `/listen/<code>` on the host page). |
| `copy [TARGET]` | Copy page text (or an element's value/name) to the **user's real system clipboard**, verified by read-back. It clobbers the clipboard — use for a reason, not casually. |
| `download TARGET DIR` | Save what a link downloads into DIR. Refuses from the CLI (`browser_consent_required` — needs MCP-host approval); the practical path is `click` the link and find the file in the browser's download dir. |
| `dom CSS` | CSS-selector existence check (read-only). |
| `eval JS\|@FILE` | Run JS in the page's **MAIN world** and print the value (`returnByValue`, awaits promises) — real page internals, not a sandboxed view. `@FILE` reads JS from disk for multi-line probes. Transparent fallback: tries the driver's JS tool first, and on its standard-mode refusal evaluates over the instance's own DevTools endpoint — same result either way. |
| `cdp` | Print the instance's DevTools http + browser/page WS endpoints — for external CDP tooling (e.g. playwright-core `connectOverCDP`) when `eval`'s one-shot call isn't enough. |
| `wait-for TEXT [--timeout S]` | Poll until text appears (default 15s). |
| `dialog inspect` / `accept` / `dismiss [--text T]` | JS alert/confirm/prompt/beforeunload. |
| `tabs` / `tab SEL` | List tabs; switch active by index, tab_id, or URL/title substring. |
| `resize W H` | Set outer window frame (viewport is frame minus chrome). |
| `record start [DIR] [--video]` / `stop` / `state` | Evidence capture: per-action turn folders (`before/after/screenshot.png` + `evidence.json`); `--video` adds `recording.mp4` of the display (macOS ScreenCaptureKit, no ffmpeg). |
| `url` | Current URL + title. |
| `status` | All known contexts on this machine: pid, alive, url. |
| `close CTX ...` / `close --all` | Terminate owned browser(s). `--all` closes *every* recorded context — other agents' too; close your own names. Attached browsers are never killed. |

## Parallel browsers (two-tab / multi-user tests)

Each context is a fully independent browser instance:

```sh
A=$(browse new-context alice); B=$(browse new-context bob)
browse --ctx $A open http://127.0.0.1:8787/listen/ABC123
browse --ctx $B open http://127.0.0.1:8787/listen/ABC123
browse --ctx $A click "Listen"
browse --ctx $B snap
browse close $A $B
```

Tabs opened by the page itself surface via `tabs` and are selected with `tab`.

## Example — a whole smoke test in two calls

```sh
CTX=$(browse new-context smoke)
browse --ctx $CTX batch <<'EOF'
open http://127.0.0.1:8787/
type "Username" demo
type "Password" secret
press enter                          # or: click "Sign in"
wait-for "Start"
click "Start" --wait-for "listening"
shot /tmp/after-start.png
EOF
browse close $CTX
```

Form recipe: `type FIELD "John" --replace` sets a field (select-all + type);
`press enter FIELD` submits. No manual sleeps — `--wait-for` is the wait.

## Built-in verification — what the script already checks

- `open`/`click`/`type` end with a fresh snapshot; `click` prints
  `navigated -> <url>` when the page moved, `type` prints the field's new value.
- `effect=unverifiable` is normal for background input — the printed post-state
  is the verdict, not the effect code. A `FAIL` line means it genuinely didn't work.
- `browser_navigate` waits for the navigation commit; `open` adds a paint
  settle. For async content (SPAs, websockets), use `--wait-for` — no manual sleeps.
- Refs are re-resolved against a fresh snapshot inside every action — they
  cannot go stale between calls.

## What it can and cannot do — read before planning a test

**Can** (verified end-to-end): launch isolated Chromium instances in parallel,
navigate, click/type/hover/scroll/drag by semantic name or ref, submit forms
with `press enter`, upload local files to file inputs, handle JS dialogs,
search/inspect the rendered page (`find`/`dom`/`tree`/`text`/`snap`),
extract element values (`read`), copy page/element content to the system
clipboard (`copy`), run page JavaScript (`eval` — transparent DevTools
fallback), take viewport screenshots, record video + per-action
before/after evidence, resize the window for responsive checks, run
multi-step flows via `batch`, and drive several independent browsers at
once — all in background, without stealing the user's focus. The daemon
self-starts on first call; **no MCP server is required** — host MCP bridges
may render only the text channel, so the CLI is the reliable transport.

**Cannot** (verified limits — do not attempt, do not improvise around):
- **Text-range selection** on page content (no drag-select; static text isn't
  addressable; only whole-field select via `type --replace`). To *copy* text
  without selecting it, use `copy` — clipboard write is the sanctioned path.
- **CSS-selector clicks** — `css=` resolves via accessible name; `dom` is a
  read-only existence check. Interaction is semantic only.
- **Native browser chrome** — OS permission prompts (mic/camera), native file
  pickers, extension UI. `files` bypasses `<input type=file>`; the rest waits
  for a human.
- **Attaching to the user's everyday browser** — the existing-profile grant
  path fails its setup proof in this driver build (`browser_wrong_target_
  refused`). Tests always run in isolated instances; cookies/logins from the
  user's profile are unavailable (use `--profile` to persist a scratch one).
- **Trusted input** — prepared instances are permanently background-posture;
  foregrounding them does not change it. All input is `dom_event` — trust-gated
  controls that ignore synthetic events are a real dead end: report, don't retry.
- **Keyboard modifiers** (Shift/Cmd/Ctrl chords) — keystrokes send plain
  characters only.
- **Downloads via `browser_download`** — gated to MCP-host approval; `click`
  the link instead and find the file in the instance's download dir.
- **User extensions** — driver-owned instances launch with
  `--disable-extensions` baked in, and launch args can't be injected
  (verified: `Extensions.loadUnpacked` registers but never activates — no
  content scripts, no background). For extension work use
  lite-chrome-automation's `--extension` launch flag.

## Rules that matter

- **Always close your contexts** (`browse close $CTX …`) at the end of a run.
- **Native browser chrome is out of scope** — permission prompts, native file
  pickers, extension UI. File inputs have the `files` bypass; for mic/camera
  prompts, ask the user.
- **This skill verifies, it doesn't fix.** On failure report the snapshot /
  screenshot evidence; the implementation lane owns the code change.
- Anything `browse` doesn't cover: raw `cua-driver <tool> '<json>'` works —
  schemas via `cua-driver describe <tool>`; internals and gotchas in
  `NOTES.md` beside this file. The official deep reference pack is the
  `cua-driver` skill (`BROWSER.md` for the browser lane) — consult it before
  improvising past a refusal.
