---
name: working-with-codex-subagents
description: How Codex discovers custom agents and resolves subagent models, effort, and spawn args. Use when developing Codex agents (~/.codex/agents, [agents] roles, plugin-bundled) or when writing prompts/skills that delegate via spawn_agent.
---

# Working with Codex subagents

Load this skill when authoring a Codex agent definition or a parent prompt that delegates via **`spawn_agent`**. It records **verified** behavior for discovery, model/effort resolution, and spawn gotchas.

Verified against **codex-cli 0.145.0** (`multi_agent` stable/on, `multi_agent_v2` stable but **off**). Findings marked *(verified)* were reproduced directly by experiment; *(binary)* came from `codex debug` / `--strict-config`; *(docs)* is documentation only and weaker.

Codex changes fast here — if you face problems related to this when developing, and Codex is a a different version, re-verify with the commands in [Verifying on your build](#verifying-on-your-build).

## Where agent definitions live

| Location | Registers? |
|---|---|
| `~/.codex/agents/<file>.toml` | **Yes** — the reliable path *(verified)* |
| `[agents.<name>]` in `config.toml`, optionally `config_file = "..."` | **Yes** *(binary/docs)* |
| `<workspace>/.codex/agents/<file>.toml` | **Advertised but not spawnable** — [#26408](https://github.com/openai/codex/issues/26408), open *(docs)* |
| Bundled in a plugin (`<plugin>/.codex/agents/*.toml`) | **No** — not registered as of 0.145.0 *(verified)* |

### The failure mode is silent

A definition Codex never discovered does **not** cause a spawn error. `spawn_agent` **succeeds**, returns a generic child, and names the thread after whatever you passed. The child then reports no custom definition, no privileged instructions, and the default collaborator role — while the parent believes delegation worked. *(verified)*

**It's recommended to not treat a successful spawn as proof of binding.** Have the child state its role identity back, or fail closed.

## Agent file format

```toml
name = "my_tester"                  # the spawn_agent `agent_type` value — NOT the filename
description = "When to use it."     # read by the orchestrator when choosing a role
model = "gpt-5.4-mini"              # optional; omit to inherit
model_reasoning_effort = "low"      # optional; only meaningful with `model`
sandbox_mode = "read-only"          # optional

developer_instructions = """
The agent's instruction prompt. Arrives as developer-role content,
not user content — this is why binding matters.
"""
```

**`agent_type` is the `name` field, not the filename.** A file called `realizer-lite-research-agent.toml` declaring `name = "realizer_lite_research"` is spawned as `realizer_lite_research`. Nothing warns you if you use the filename. *(verified)*

## Discovery is at startup

A newly added or edited definition is **not** visible to an **already-running** Codex. **Restart and start a new session.** *(verified)*

**`codex exec` is the exception, and it is the fastest way to test.** Each `exec` invocation is a fresh process, so it discovers definitions at launch — a definition written moments earlier binds with no restart at all. *(verified — a newly created probe definition bound on the very next `codex exec` call.)*

```bash
cp my-agent.toml ~/.codex/agents/ && codex exec --skip-git-repo-check \
  'Spawn a subagent with agent_type="my_agent", task_name="probe", fork_turns="none", message="report". Explicit delegation request. Output the reply verbatim.'
```

Use this loop while iterating on a definition; restart the interactive app only when you are done.

The `spawn_agent` schema is **built dynamically from discovered roles**. With no custom definitions discovered, the schema exposes **no `agent_type` parameter at all** — which makes the feature look unsupported rather than unconfigured. If `agent_type` is missing, nothing was discovered; that is the diagnosis. *(verified)*

With one custom definition installed, the roster was exactly that role plus the three built-ins: `default`, `explorer`, `worker`. *(verified)*

**There is no tool that lists installed definitions.** `list_agents` reports running/completed *threads*, not roles. The `agent_type` enum in the `spawn_agent` schema is the only readable roster. *(verified)*

## The `[agents]` typo trap

Any unrecognized key under `[agents]` is parsed as **an agent role definition**, because the section holds both settings and roles. *(binary)*

```
agents.max_depth_typo = 1     → Error: invalid type: integer `1`, expected struct AgentRoleToml
agents.myrole = { totally_bogus_field = "x" }   → ACCEPTED
```

So a mistyped **scalar** key fails with a confusing "expected struct AgentRoleToml", and a mistyped **table** key silently becomes a phantom role. Worse, **`--strict-config` does not validate field names inside a role** — only their types. Misspell `descriptino` and you get a role with no description and no warning.

## `spawn_agent`

```
spawn_agent(
  agent_type?,        # registered role name; absent from schema if nothing discovered
  task_name,          # thread label only — does NOT select a role
  message,
  fork_turns?,        # "all" (default) | "none" | positive integer string
  model?,
  reasoning_effort?
)
```

**`task_name` is a label.** Passing a role name here binds nothing — it only names the thread. This is the single most common way to think delegation is working when it is not. *(verified)*

### The correct call

```
spawn_agent(
  agent_type = "realizer_lite_research",   # the registered role — this is what binds
  task_name  = "<independent task label>", # names the thread; unrelated to the role
  message    = "<task payload>"
)
```

`agent_type` and `task_name` are **independent**. Give `task_name` a label describing the unit of work, not a copy of the role name — the two serve different purposes and conflating them hides which one is doing the binding.

### If `agent_type` is not in the schema, stop

The parameter is emitted only when at least one custom role was discovered. **Its absence means no subagent is available yet** — either none is installed, or one is installed incorrectly (wrong directory, only bundled in a plugin, or Codex not restarted since).

In that state the agent **cannot call a specific subagent at all.** Every spawn will produce a generic child no matter what you pass. Treat a missing `agent_type` as a hard stop and a registration diagnosis — not as a reason to fall back to `task_name`, which will appear to work and will not.

## Model and effort resolution

Precedence, highest first *(docs, consistent with observed behavior)*:

1. explicit `model` / `reasoning_effort` on the spawn
2. the agent definition file
3. `[agents] default_subagent_model` / `default_subagent_reasoning_effort`
4. the parent session
5. the model's own default

| Definition | Spawn arg | What runs |
|---|---|---|
| pinned | omitted | **the pin** — not the parent |
| pinned | explicit | **the spawn value** |
| unpinned | omitted | **parent** (or the `[agents]` default, if set) |
| unpinned | explicit | **the spawn value** |

Note level 3: Codex has a **global default layer** that Cursor lacks. An unpinned agent does not necessarily follow the parent — it follows `default_subagent_model` when that is configured.

### `fork_turns` silently overrides your model choice

**Full-history forks (`fork_turns` omitted or `"all"`) inherit the parent model and reasoning effort and do not accept overrides.** To make `model` / `reasoning_effort` take effect, set `fork_turns` to `"none"` or a positive integer string. *(binary — this is injected into the live prompt)*

Since `"all"` is the **default**, a spawn that passes `model` and nothing else has its model argument **silently ignored**. This is Codex's analogue of "don't pass `model` with `resume`", and it is easy to miss because nothing errors.

Codex's own injected guidance also says to set `model` / `reasoning_effort` **only** when the user, `AGENTS.md`, or skill instructions explicitly ask.

### Reasoning effort values

Per-model, from the live catalog *(binary — `codex debug models`)*:

| Model | Default | Supported efforts |
|---|---|---|
| `gpt-5.6-sol` | low | low, medium, high, xhigh, max, ultra |
| `gpt-5.6-terra` | medium | low, medium, high, xhigh, max, ultra |
| `gpt-5.6-luna` | medium | low, medium, high, xhigh, max |
| `gpt-5.5` | medium | low, medium, high, xhigh |
| `gpt-5.4` | medium | low, medium, high, xhigh |
| `gpt-5.4-mini` | medium | low, medium, high, xhigh |

**`minimal` is not supported by any model.** **`extra-high` is not the spelling — it is `xhigh`.** `max` and `ultra` are real but only on the newest families, so an effort valid for one model is invalid for another. Regenerate this table rather than trusting it.

## Concurrency and depth

`~/.codex/config.toml`:

```toml
[agents]
max_concurrent_threads_per_session = 3   # spawned threads, EXCLUDING the primary
max_depth = 1                            # root = depth 0; 1 = leaves only
```

`max_threads` is a documented **legacy alias** for `max_concurrent_threads_per_session`. Both are unset by default. *(docs)*

### The count the agent sees is +1 from the config

Codex injects the limit into the live prompt as *"There are N available concurrency slots, meaning that up to N agents can be active at once, **including you**."* The config key **excludes** the primary; the injected number **includes** it. *(binary, confirmed by varying the key)*

| Config | Injected | Usable subagent slots |
|---|---|---|
| unset | **4** | **3** |
| `= 2` | 3 | 2 |
| `= 5` | 6 | 5 |

**The default gives 3 concurrent subagents.** Do not size a fan-out at the raw config number — it is one lower than the slot count the agent is told about.

Overflow is a **hard error**, not a queue: `collab spawn failed: agent thread limit reached`. *(docs)*

`agents.max_depth` **is a recognized key** — it passes `--strict-config` *(binary)* — even though it is absent from the published `[agents]` reference. Default `1`: the root spawns leaves, leaves cannot spawn grandchildren. Children *can* spawn their own children when depth allows.

## Delegation is gated by default

The live prompt carries:

> `<multi_agent_mode>` Any earlier instruction enabling proactive multi-agent delegation no longer applies. Do not spawn sub-agents unless the user or applicable AGENTS.md/skill instructions explicitly ask for sub-agents, delegation, or parallel agent work.

So **a skill that wants delegation must ask for it explicitly.** Merely defining agents and hoping the orchestrator reaches for them will not work. *(binary)*

## The collaboration tools

| Tool | Purpose |
|---|---|
| `spawn_agent` | Create a new agent and give it a task |
| `followup_task` | Give an **existing** agent a new task, triggering a turn |
| `send_message` | Pass a message to a running agent **without** triggering a turn |
| `wait_agent` | Block on a running agent |
| `interrupt_agent` | Interrupt a running agent |
| `list_agents` | List running/completed **threads** — never installed roles |

### They are not callable from `exec`

All six must be **direct tool calls** (e.g. `to=functions.collaboration.spawn_agent`). They are deliberately absent from the `functions.exec` `tools.*` namespace, so a shell-mediated call cannot reach them. *(binary)*

### Message envelope

Replies arrive in the analysis channel as:

```
Message Type: MESSAGE | FINAL_ANSWER
Task name: <recipient>
Sender: <author>
Payload:
<payload text>
```

`task_name` is what appears as **Task name**, which is the practical reason to make it a meaningful work label — it is how you tell concurrent threads apart in the transcript.

## All agents share one filesystem and working directory

From the injected prompt *(binary)*:

> All agents have access to the same container and filesystem as you. All agents use the same current working directory. As a result, edits made by one agent are immediately visible to all other agents.

There is **no filesystem isolation between agents** — not even a per-agent cwd. Two children editing the same file will clobber each other, and a child that `cd`s somewhere has not moved itself anywhere private.

So any parallel fan-out that writes must arrange its own isolation — separate git worktrees, disjoint output paths, or a partition of the file set — and **pass absolute paths**, since a relative path means the same location for every agent.

## How a subagent learns its model

**A subagent cannot read its own TOML settings.** Only `developer_instructions` reaches it — as the prompt itself. Every other field is invisible to the running child. *(verified)*

| Fact | Visible to the subagent? |
|---|---|
| `developer_instructions` | **Yes** — it *is* the child's developer message *(verified)* |
| `name` | **No** — reported `NOT_VISIBLE`, or the generic `Codex` identity *(verified)* |
| `description` | **No** — same *(verified)* |
| `model` | **No** — `NOT_VISIBLE`; no slug appears anywhere in the prompt *(verified)* |
| `model_reasoning_effort` | **No** — `NOT_VISIBLE` *(verified)* |
| `sandbox_mode` | **No** — the child reports the **session's** sandbox, not its pin *(verified)* |
| The spawn `model` / `reasoning_effort` args | **No** — assume invisible; pass in `message` if needed |
| Concurrency slots | **Yes** — injected as prose *(binary)* |

Probe method: a definition pinning `gpt-5.4-mini` / `high` / `workspace-write` was spawned from a `gpt-5.6-terra` / `low` / `read-only` parent and asked to report each fact or answer `NOT_VISIBLE`. Its `developer_instructions` were plainly in effect — the reply followed their exact format — while it reported `NOT_VISIBLE` for name, description, model, and effort, and reported the **parent's** `read-only` sandbox rather than its own `workspace-write`.

> The sandbox result shows only what the child can **see**. Whether a TOML `sandbox_mode` is still **enforced** while being invisible was not tested — do not read this as evidence the pin is ignored.

Consequences:

- **Role identity is behavioral, not introspectable.** A child knows what it is only insofar as its `developer_instructions` say so. To verify binding, the instructions must **tell it to identify itself** — there is no metadata to query.
- **Never write instructions that reference the definition's own fields** ("use the model configured for you", "your description says…"). The child cannot see them.
- If the child must know its model — for model-aware behavior or diagnostics — **the parent must state it in `message`**. This matters most when `fork_turns="all"` silently voided the model argument, since neither side observes the discrepancy.

## Verifying on your build

```bash
codex --version
codex features list | grep -E "multi_agent|multi_agent_v2"
codex debug models | jq -r '.models[] | "\(.slug) \(.default_reasoning_level) \([.supported_reasoning_levels[].effort]|join(","))"'
codex debug prompt-input "x" | jq -r '.[].content[]?.text' | grep -i "concurrency slots"
codex --strict-config -c 'agents.some_key=1' exec --skip-git-repo-check "x"   # is some_key a real setting?
```

`codex debug prompt-input` is the highest-value one: it renders exactly what the model sees, including the concurrency count, the `fork_turns` rule, and the delegation gate.

## Checklist for parent prompts that spawn subagents

- [ ] `agent_type` matches the definition's **`name` field**, not its filename
- [ ] Definition installed in `~/.codex/agents/` — not only bundled in a plugin, not project-scoped
- [ ] Codex **restarted** since the definition was added or changed (not needed for `codex exec`)
- [ ] `agent_type` present in the `spawn_agent` schema — if absent, **no subagent is available**; stop and fix registration
- [ ] `agent_type` and `task_name` both set, and `task_name` is a work label rather than a copy of the role
- [ ] Binding **verified**, not assumed — a successful spawn proves nothing
- [ ] Bind failure **fails closed**; never silently continue on a generic child
- [ ] `task_name` used only as a label
- [ ] Passing `model` / `reasoning_effort`? Then `fork_turns` is `"none"` or an integer string, or the values are ignored
- [ ] Effort value valid **for that specific model** (`xhigh`, never `extra-high`; never `minimal`)
- [ ] Fan-out sized to **slots − 1** (default: 3), with a plan for `agent thread limit reached`
- [ ] The skill/prompt **explicitly asks** for delegation — it is gated off by default
- [ ] Collaboration tools called directly, never from inside `exec`
- [ ] Parallel writers isolated deliberately — agents share one cwd and filesystem — with **absolute** paths
- [ ] Anything the child must know is in `message` — it cannot read its own TOML fields
- [ ] Agent instructions never reference the definition's own `name` / `description` / `model` — invisible to the child
- [ ] Binding checks work by having the child **state its identity**, since there is no metadata to query
