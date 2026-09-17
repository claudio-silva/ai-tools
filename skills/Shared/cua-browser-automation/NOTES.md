# cua-browser-automation — internals & verified findings

Everything below was verified against cua-driver 0.28.2 (0.20.0 where noted)
by driving isolated Chromium instances end-to-end. Read this before modifying
`browse`. Host-specific observations name the host (mostly Devin, where the
verification ran) — the CLI guidance itself is host-agnostic.

## Transport: CLI is authoritative, MCP bridge is detection-only

- `cua-driver <tool> '<json-args>'` via exec returns complete JSON.
- The Devin `mcp_call_tool` bridge renders only the MCP text channel:
  `list_windows` → "Found N window(s)" with no records; bind → "bound … with N
  tab(s)" with no `tab_id`s. The typed lane is undrivable through it. Fine for
  `mcp_list_servers`, `check_permissions`, `get_accessibility_tree` (window ids
  DO render in its text output).
- Sessions are transport-scoped: a label created via MCP is refused on the CLI
  (`session is not available to this transport`) and vice versa. `browse`
  therefore labels every context `browse-<ctx>` and never mixes.
- Since 0.28, prepared instances are also **transport-owned**: binding a
  CLI-prepared browser from the MCP bridge refuses `browser_consent_required`
  (it treats a foreign session's prepared profile like an existing profile).
  Another reason the typed lane is CLI-only in Devin.
- The MCP bridge drops `structuredContent` — verified by inspecting the
  declared `outputSchema` (full `windows[]` records exist server-side; only
  the `content[].text` string reaches the model). Devin-side limitation, not
  fixable by driver updates.
- `cua-driver describe <tool>` prints the authoritative input schema — use it
  before adding a wrapper for a new tool.
- One-shot CLI calls run on **disposable transports** (implicit sessions);
  `browse` always passes its per-ctx label. An instance prepared on a
  disposable session is reaped when that call's process exits — always pass
  `session`.
- **Daemon lifecycle**: `browse`'s `cli()` self-heals — on a dead-socket
  response it spawns `cua-driver serve --no-permissions-gate` and retries
  once. No MCP server, LaunchAgent, or manual serve needed. (`autostart` is
  still Windows-only on macOS in 0.28.)
- **TCC attribution**: driver ops launched from an agent's shell attribute
  consent prompts to the host app (Devin, `com.exafunction.windsurf`), not
  cua-driver — observed: `kill_app`-adjacent probing triggered an App
  Management (`kTCCServiceSystemPolicyAppBundles`) prompt naming Devin; the
  op was denied and harmless. Pasteboard access similarly prompted for Devin
  once; after the grant, `clipboard_write`/`clipboard_read` work silently.

## Why one driver session per context

`end_session` terminates **every** driver-owned instance prepared under that
session label (verified: two `browser_prepare`d pids under `probe` both died on
one `end_session`). Per-context teardown therefore requires per-context labels —
`browse` names them `browse-<ctx>` so `close <ctx>` ends exactly one browser.

## Multi-agent concurrency model

- `new-context` allocates `<label>-<uuid6>` names — unique across every agent,
  IDE, or harness on the machine, and doubles as the cua-driver existence test.
- All contexts share one state file `${TMPDIR}/cua-browse/state.json`.
  `save_state` merges in-memory contexts into the current on-disk dict and
  writes atomically (`tmp` + `os.replace`), so concurrent `browse` processes
  only ever overwrite their own ctx keys. Deletions propagate via `DELETED`.
- `close --all` closes every recorded context — including other agents'.
  Agents should close their own names positionally (`close a b`).
- Focus is irrelevant: input is `dom_event`-delivered in background, so a user
  switching windows mid-run does not abort or corrupt operations.

`kill_app` is NOT a close path: in standard mode it refuses driver-spawned
browsers (`foreign_process_termination_denied` — they are launched via
LaunchServices, not the runtime).

For `attach`ed contexts (`owned:false`) `browse` never calls `end_session` —
whether it would terminate a user-attached browser is unverified and the
downside is catastrophic. It just drops state.

## Launch sequence (what `browse open` hides)

1. `list_apps {"kind":"desktop"}` → pid of a RUNNING supported browser
   (chrome/chromium/edge/brave in the bundle fields). `browser_prepare` requires
   this pid even for isolated launches — it names the product to clone.
2. `browser_prepare {"pid":P,"allow_launch":true,"profile":{"mode":"isolated_new"}}`
   → `prepared_pid` (a second process, same bundle id; user's profile is never
   touched — `side_effects` in the response documents what happened).
   `isolated_named`+`name` gives a persistent scratch profile (logins survive
   across runs).
3. `list_windows {"pid":prepared,"on_screen_only":true}` → the isolated process
   maps ~10 windows, all off-screen helpers except one `about:blank`. Take that
   `window_id`.
4. `get_browser_state {"pid":P,"window_id":W,"session":L}` (bind mode) →
   `target_id`, `tabs[]` (`tab_id`,`url`,`title`,`active`),
   `binding_quality:"exact"`, `mutation_allowed:true`.

## Snapshot schema (semantic_v2)

```
{ "snapshot": {"id":"p8","complete":true,"scope":"viewport","selected_nodes":N,"total_nodes":M},
  "refs":     [{"ref":"p8:0","role":"link","name":"Learn more","value":null,
                "actions":["click","pointer"],"states":{"focusable":true},
                "visibility":"in_viewport","frame":"main"}],
  "content_refs": [...],   # populated when "query" is passed
  "page": {"url":..., "title":...},   # title lags (native title); url is current
  "outline": "…", "oopif": {"frames":0,"status":"attached"} }
```

- Refs invalidate on navigation (`refs_invalidated:true` in navigate's
  response) AND on the next snapshot of the same tab (new `snapshot.id`
  prefix). `browse` re-snapshots inside `resolve()` so callers never hold
  stale refs.
- `dom_refs_v1` = raw DOM dump incl. invisible `<link>` head nodes — forensics
  only, never act on it.
- Snapshots are budget-capped (`selected_nodes`/`total_nodes`); on truncation
  `snapshot.continuation` carries a token — `browse snap --more TOKEN` fetches
  the next chunk, `--scope p<i>:<j>` reads one subtree (scope refs must come
  from the CURRENT snapshot — older ones hit `browser_ref_stale`).
- `query:"text"` = read-only semantic match over role/name/visible text;
  matches land in `content_refs`, NOT `refs`.
- Editable inputs carry `actions:["type","pointer"]`, `states.editable`, and
  current content in `value`. Unlabeled inputs have `name:null` — `browse type`
  falls back to the single editable on the page.

## Click semantics (verified end-to-end on CNET, example.com, HN)

- `browser_click` default route `trusted` (CDP Input) **refuses** on any
  non-frontmost window: `{effect:"refused",route:"trusted_input",
  escalation:{target:"page",reason:"route_unavailable"}}`. It refuses rather
  than foreground your browser — by design.
- Retry same ref with `input_route:"dom_event"` → `{route:"dom",
  effect:"unverifiable",delivery:{mode:"background"},escalation:{reason:
  "effect_unconfirmed"}}`. `unverifiable` is normal, not failure — the next
  snapshot is the verdict. Every test click landed this way.
- `browser_pointer` (hover/right_click/double_click/scroll/drag) has the same
  trusted/dom_event shape; `session` is **required** on it (schema-required,
  unlike other tools). Refs must declare `pointer`/`scroll` capability.
  Coordinate x/y works for scroll origins, but coordinate DRAG is refused in
  background — `dom_event` requires a ref, and trusted won't foreground.
  **Foregrounding does NOT unlock trusted input**: `bring_to_front
  {"pid","window_id"}` verified `focused:true, frontmost_ordinary:true` and
  `browser_click` still refused `route_unavailable`. Background posture is a
  policy on prepared instances, not a focus state — plan for dom_event always.
  Consequently no text-range selection: static text isn't pointer-capable
  (doesn't appear in refs at all), coordinate drag is refused, and double-click
  word-select can't target non-refs. Inside editables, `type --replace`
  selects-all only. (Possibly different for an attached user browser — the
  posture policy may not apply there; unverified, Route A only.)
- `browser_type`: `ref` required; `mode:"insert_text"` (default — the whole
  string in ONE bulk Input.insertText, not per-char) or `"keystrokes"`
  (per-char Input.dispatchKeyEvent — sends REAL key events: verified `\r`
  submits forms, HN search → hn.algolia.com). `replace:true` selects-then-types
  (empty text clears). The driver focuses the ref itself — no prior click
  needed (verified). `browse press` maps enter/tab/esc/backspace/delete/space
  to keystroke chars on the resolved editable.
- `browser_dialog`: `inspect` → `dialog_id`; accept/dismiss require that exact
  id; `prompt_text` for prompt dialogs. Page-owned JS dialogs only — never
  permission prompts, extension UI, native pickers.

## page tool (legacy)

- Read-only actions (`get_text`, `query_dom`) work by default. Mutating ones
  (`execute_javascript`, `click_element`, `insert_text`, `type_keystrokes`)
  refuse unless daemon runs unrestricted with
  `CUA_DRIVER_ENABLE_LEGACY_PAGE_MUTATIONS=1` set **before daemon start**.
- `query_dom` takes `css_selector` (not `selector` — wrong arg name silently
  dumps the AX tree instead). Output is TEXT (`- [13] AXLink (Learn more)`),
  no JSON, no geometry — existence/name checks only. `browse` bridges
  `css=SEL` → accessible name → semantic ref for clicks.
- `page` args: `pid` + `window_id` (`window_id` is required even though the
  schema doesn't mark it), `target_url_contains` to pick a tab.
- There is NO viewport-coordinate lookup for CSS matches in standard mode →
  CSS is find-only. All interaction goes through semantic refs.

## Windows, tabs, frames

- Re-binding (bind mode on same pid+window_id) **re-mints tab ids** — old ids
  stay usable within the session, but equality checks fail. `browse`
  re-maps `active_tab` by URL after each `tabs` refresh.
- There is no "new tab" call. Page-opened tabs (`target=_blank`) appear as
  extra `tabs[]` entries on re-bind — check, don't assume. For parallel work,
  use a second context (second isolated instance), not a second tab.
- `set_window_frame` requires `pid`,`window_id`,`x`,`y`,`width`,`height` —
  preserve current x/y from `list_windows` bounds. Sets the OUTER frame;
  viewport = frame minus browser chrome (verified 800×600 frame → ~780×~550
  viewport). Route is `accessibility`, effect `confirmed` via value readback.
- `include_screenshot:true` returns `screenshot_png_b64` — exact tab viewport,
  no foregrounding. Verified real PNGs (1720×1274, 1200×1231).

## Lifecycle failure mode

Driver-owned instances CAN die between calls (observed once, mid-run).
Symptom: `authorization_host_failed: browser process <pid> is no longer
available`. `browse` detects dead pids via `os.kill(pid,0)`; `open` re-prepares
(clean profile → re-do the flow), other commands fail fast.

## Route A (attach to the user's real browser) — verified refusal ladder

Tested against a live stock Chrome window. Each rung was verified in order:

1. `browser_prepare {"pid":P}` alone → `browser_requires_setup` (stock browser
   has no owned CDP endpoint; detect-only call is side-effect-free).
2. `+ strategy:{kind:"existing_profile"}, window_id` without grant →
   `browser_consent_required` ("standard mode requires --grant
   existing-profile or an embedding authorization host").
3. `--grant existing-profile` is a **daemon-launch flag**, not a per-call flag:
   `cua-driver stop` then `cua-driver serve --grant existing-profile` (the
   daemon does NOT auto-relaunch from CLI calls — start it explicitly or the
   next call errors "not running"). Grant applies daemon-wide until restart.
4. With the grant, prepare → `browser_wrong_target_refused`: *"Google Chrome's
   setup accessibility proof was truncated"* — the driver's bounded setup
   (opens Chrome's own remote-debugging page in the user's window, toggles the
   per-instance checkbox, proves the loopback endpoint) fails its AX proof in
   this build, window frontmost or not. Retried after `bring_to_front` —
   same refusal.

**Verdict: existing-profile attach is not usable in standard mode** (0.20.0:
setup AX proof truncated; re-checked 0.28.2: prepared-instance ownership now
spans transports, so a CLI-prepared browser can't even be bound from MCP —
`browser_consent_required`). `browse attach` only works against a browser
that already has an owned CDP endpoint (e.g. launched with
`--remote-debugging-port`). Whether trusted input / text selection behaves
differently on an attached browser is therefore UNVERIFIED — the posture
evidence so far (prepared instances) says dom_event-only regardless of focus.

## DevTools endpoint (verified on 0.28.2)

Prepared Chromium launches with `--remote-debugging-port=0` (ephemeral,
loopback-only) and the endpoint is fully reachable by other local processes:
`cdp_pages()` discovers it via `lsof` on the prepared pid + `GET /json`;
`browse cdp` prints it for external tooling.

**`browse eval` is a transparent two-path command**: it tries the gated
`page execute_javascript` first, and on refusal runs `Runtime.evaluate`
(`returnByValue`, `awaitPromise`) over the DevTools WS using a built-in
stdlib WebSocket client (`ws_rpc` — handshake, masked client frames,
event-skipping). Verified: the gated tool refused and the fallback returned
`performance` entries, DOM queries, and awaited promises. Agents never need
to know which path ran. `eval @FILE` reads the JS from disk — multi-line
probes without shell-quoting pain.

**Full-page screenshots**: `get_browser_state`'s `include_screenshot` is
viewport-only; `shot --full` instead calls `Page.captureScreenshot` with
`captureBeyondViewport:true` over the same WS plumbing (verified: 3369px
capture of a 3000px body vs 1231px viewport).

**Extensions — verified dead end on driver-owned instances** (0.28.2): the
prepared Chromium launches with `--disable-extensions` baked into the
driver's template, and `browser_prepare` has no launch-args hook (a guessed
`launch_args` field is silently ignored — `additionalProperties:true` means
unknown fields don't error, they just do nothing). `Extensions.loadUnpacked`
over the browser-level WS (`/json/version` → `webSocketDebuggerUrl`) returns
a real extension id, but the extension stays inert: no service-worker target,
content scripts never inject (verified with a marker extension mutating
`document.title` — no effect after reload). The one context where it would
work — an `attach`ed browser with an owned endpoint — is itself a dead end
here, so `browse` has no extension command at all: use
lite-chrome-automation's `--extension` launch flag.

## Clipboard (verified on macOS, 0.28.2)
- `clipboard_write {text}` writes the REAL system pasteboard — it clobbers
  whatever the user had copied. `browse copy` verifies via `clipboard_read`.
- `clipboard_read` is types-only by default (privacy) — pass
  `include_text:true` or `text` is always `null` even when text is present
  (verified: write landed, `pbpaste` showed it, read returned `text:null`
  until `include_text`).
- This is the sanctioned page-content-copy path (per the official
  `BROWSER.md`): read the value from a fresh snapshot, `clipboard_write`,
  `clipboard_read` to prove it. No text selection needed.

## Downloads

- `browser_download {session,target_id,tab_id,ref,destination_root}` —
  `destination_root` must be a REAL directory (symlinks refused; `browse`
  realpaths first). From the CLI it refuses `browser_consent_required`
  ("requires approval through the MCP host's destructive-tool confirmation
  flow") — only an MCP host can satisfy the gate, and Devin's bridge may or
  may not prompt. Practical path: `click` the link, then find the file in the
  isolated instance's download dir.

## Recording (verified on macOS)

- `start_recording` **requires `output_dir`** and refuses if it doesn't exist
  (`protected_resource_scope_invalid`) — `browse` makedirs it first.
- `record_video:true` captures the main display to `<dir>/recording.mp4` via
  native ScreenCaptureKit on macOS 15+ (no ffmpeg); Windows/Linux need ffmpeg
  (`install_ffmpeg`).
- Per-action artifacts land in `<dir>/turn-NNNNN/` — `before.png`,
  `after.png`, `screenshot.png`, `action.json`, `evidence.json` — automatic
  per-step evidence even without `--video`.
- Recorded tabs get an agent-cursor overlay (`set_agent_cursor_enabled`,
  `set_agent_cursor_theme`); one session per tab gives distinct cursor colors
  in multi-context recordings.

## Not yet exercised (thin wrappers — verify before relying on)

`dialog` accept/dismiss, `files` (now prefers `upload`-action refs), `scroll`/
`drag`-by-ref/`hover`/`right-click`, `press tab` focus-move (Enter verified —
submits; Tab maps to `\t` keystroke but focus-move not confirmed), daemon
self-heal (code path simple; not destruction-tested). Route A attach: see the
refusal ladder above — effectively unavailable in this build.
