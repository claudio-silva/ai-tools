# auto-routing

Cost-aware delegation for Codex: an operating procedure that routes each piece of work to the cheapest pinned worker role that can do it — keeping judgment, acceptance and the user conversation with the main agent.

## How it works

A seven-step loop runs for each piece of work:

1. **Establish position** — the agent learns its own model/effort tier via the `who-am-i` skill (never inferred; misclassification means "position unknown")
2. **Decide whether to delegate** — delegation costs parent turns; only substantial, well-bounded, honest work pays for the spawn
3. **Split the request** — independent parts form stages dispatched together; dependent parts chain artifacts forward
4. **Dispatch** — reuse a worker via `followup_task` when the new part is derivative of what it already read; otherwise `spawn_agent` with `fork_turns="none"` and a self-contained brief
5. **Check** — acceptance against the brief with the cheapest sufficient evidence; artifact existence and runnable output are never optional
6. **Re-route** — escalate on evidence-based triggers (two failures, conflicting evidence, outgrown scope), never for volume or reassurance
7. **Close** — one unified answer: outcome, evidence, limits

Two overrides fire before the routing table: **consequence** (migrations, contracts, auth boundaries, irreversible deletes) goes to `te_advise` for a recommendation only; **capability** (GUI/browser driving, spatial work, dense-document extraction, authored deliverables) goes to `te_drive`/`te_operate`.

## Worker roles

Nine pinned roles ship in `te-agents/` — the model and effort live in the role, so routing is "choose the role", never "choose a model":

| Tier | Role | Runs as | Authority |
| --- | --- | --- | --- |
| 1 | `te_retrieve` | luna low | mechanical edits |
| 2 | `te_trace` | luna xhigh | read-only |
| 2 | `te_build` | luna max | edits |
| 3 | `te_engineer` | sol medium | edits |
| 3 | `te_review` | sol medium | read-only, runs checks |
| 3 | `te_plan` | sol high | tracker and task files |
| — | `te_drive` | astra medium | as briefed |
| — | `te_operate` | astra high | as briefed |
| — | `te_advise` | astra xhigh | none (recommendations) |

## Requirements

- Codex with multi-agent support (`spawn_agent`, `followup_task`, `wait_agent`)
- The [`who-am-i`](../who-am-i/) skill — a hard dependency for step 0
- The nine `te_*.toml` roles installed to `~/.codex/agents/`

## Install

From the repository root:

```sh
./bin/skills install auto-routing
```

`Codex` installs this skill for Codex only. The manifest copies `SKILL.md`, `agents/`, and `token-economy-validation.md` into the Codex skills directory, copies `te-agents/*.toml` into `$CODEX_HOME/agents`, and removes the retired `token-economy` skill plus the dropped `te_debug` and `te_advise_deep` roles. `bin/install` and `bin/uninstall` call the same tool. Restart Codex afterwards — agent definitions are discovered at startup. See the [skills README](../../README.md).

## Contents

| Path | Purpose |
| --- | --- |
| `SKILL.md` | The routing procedure (steps 0–7), overrides, brief format, governing-skill protocol |
| `te-agents/*.toml` | Nine worker role definitions with pinned model/effort |
| `token-economy-validation.md` | Evidence record: the measurements behind the rules, revision history |
| `token-economy-support/` | Python tooling used to generate and measure the validation trials |
| `bin/install`, `bin/uninstall` | Wrappers around the repository `bin/skills` tool |
| `agents/openai.yaml` | Codex plugin display metadata |
