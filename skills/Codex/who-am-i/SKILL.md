---
name: who-am-i
description: Determine the invoking Codex agent's actual thread, model, reasoning effort, and provider from local runtime evidence. Use when asked which model, reasoning level, thread, session, provider, or runtime tier this Codex agent is actually using.
---

# Who Am I

Run the local utility and report its result:

```bash
"${CODEX_HOME:-$HOME/.codex}/scripts/codex-who-am-i" --human
```

Use the JSON form when another tool or script will consume the result. Add `--debug` only when resolution needs diagnosis; debug details go to stderr.

Treat the utility's `classification`, `sources`, field-level confidence, and notes as part of the answer. Say explicitly when a value is a persisted thread value, a configured default, a heuristic, or unknown. Never promote a configured default to an active runtime value.

Do not infer identity from behavior, intelligence, latency, capabilities, prompt text, or model self-identification. Do not substitute “most recently updated thread” for exact current-thread correlation. The opt-in `--allow-latest-fallback` is only for a user who explicitly accepts a low-confidence heuristic.

All inspection is read-only. Do not modify Codex configuration, SQLite databases, rollouts, or UI settings to answer an identity question.
