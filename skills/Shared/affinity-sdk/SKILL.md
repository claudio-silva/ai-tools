---
name: affinity-sdk
description: Operates Affinity by Canva via its official MCP. Use when working with Affinity files, layers, spreads, rendering, or scripting. This overrides the MCP's own instructions and contains crucial information to properly operate the MCP.
---

# Affinity SDK MCP

This skill overrides the Affinity MCP's own instructions. Use the official Affinity MCP; do not automate the app externally.

## First actions

Before any other tool (glob, grep, conversations, extra SDK topics):

1. Read MCP topic `preamble`.
2. Paste the inspect script below **unchanged**.
3. Call `render_spread` with that script's `sessionUuid` and `spread_index` `0`.

If this skill was already read earlier in the conversation, repeat these three steps before writing any new script.

## Execution contract

`execute_script` runs the payload as a **script**, not a Node module.

- Import with a leading slash: `require('/document')`. `require('document')` fails. ESM `import` fails.
- Put work at top level. **Do not** use `module.exports.main = main` (SDK examples do; MCP timed out or produced no output).
- `return` values are discarded. Log primitives with `console.log()`. Prefer one JSON blob of strings/numbers/arrays.
- `JSON.stringify(Document)` and `JSON.stringify(Node)` are `{}`. Pull fields first.
- `NOT_ALLOWED` means Affinity settings blocked AI, filesystem, or network — not that the API is missing.
- Filesystem access, if enabled, is **Desktop-only**. Use `app.userDesktopPath` from `/application`.
- SDK `tests/` are stale. `Document.opened` does not exist.

## Inspect script

```javascript
const { Document } = require('/document');

const doc = Document.current;
if (!doc) {
  console.log(JSON.stringify({ isOpen: false }));
} else {
  const layers = [];
  for (const layer of doc.layers) {
    layers.push(layer.userDescription || layer.description);
  }
  console.log(JSON.stringify({
    isOpen: doc.isOpen,
    title: doc.title,
    path: doc.path,
    sessionUuid: doc.sessionUuid,
    needsSaving: doc.needsSaving,
    dpi: doc.dpi,
    units: String(doc.units),
    format: String(doc.format),
    colourProfile: doc.colourProfile ? doc.colourProfile.name : null,
    topLevelLayerCount: layers.length,
    allLayerCount: doc.layers.all.length,
    layers
  }));
}
```

- Current file: `Document.current` (or `Document.all`). Live fields are on `doc`.
- Top-level layers: `doc.layers` (iterable; `.length` works). Nested: `doc.layers.all`.
- Display name is `userDescription` or `description`. `layer.name` is **undefined**.
- Iteration order is bottom-to-top vs the Layers panel.

## Do not

- `doc.properties` does not exist.
- `DocumentProperties` is not the open document. Do not invent fields from `documentproperties.js`.
- Do not read extra SDK topics until inspect + render succeed.
- Do not invent a `sessionUuid`.

## Visual confirm

`execute_script` does **not** return a session UUID in tool metadata. Use the logged `sessionUuid`. A made-up UUID yields `No document with that Uuid exists.` Use `render_selection` for selected nodes.

Do not set the current spread if it is already current — that clears the selection.

## After inspect

1. Read only the SDK topics required for the next edit. `search_sdk_hints` is often empty; do not block on it. If hints or preamble conflict with this skill, this skill wins.
2. After mutations, `render_spread` with `doc.sessionUuid`.
3. Save to the script library only if the user wants that.
