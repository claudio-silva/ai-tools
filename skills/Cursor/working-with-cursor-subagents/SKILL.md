---
name: working-with-cursor-subagents
description: How Cursor resolves subagent models and Task spawn args. Use when developing subagents (.cursor/agents, plugin agents, ~/.cursor/agents) or when writing prompts/skills/commands that call subagents via the Task tool.
---

# Working with Cursor subagents

Load this skill when authoring a subagent definition or when writing a parent prompt that spawns one via **Task**. It records verified behavior for **model selection** and spawn gotchas — not the full plugin packaging guide (see `cursor-plugin-development` for plugin `agents/` layout).

## Where subagents live

| Location | Scope |
|---|---|
| `<workspace>/.cursor/agents/<name>.md` | Project (workspace root only; not recursive) |
| `~/.cursor/agents/<name>.md` | User / global |
| `<plugin>/agents/<name>.md` | Bundled with a Cursor plugin |

Each top-level `.md` file is one agent. Frontmatter:

```yaml
---
name: my-agent                 # Task `subagent_type` (required)
description: When to use it.   # required — keep short
model: inherit                 # optional — see Model selection
readonly: false                # optional — true|false
is_background: false           # optional — true|false
---
```

| Field | Required | Notes |
|---|---|---|
| `name` | yes | Task `subagent_type` / slash identity |
| `description` | yes | Delegation matching — keep short |
| `model` | no | Pin a model (full bracket syntax allowed) or `inherit`; omit to follow parent when Task omits `model` |
| `readonly` | no | `true` \| `false` |
| `is_background` | no | `true` \| `false` |

The markdown **body** (frontmatter stripped) is the subagent's instruction prompt.

## Discovery lag

A **newly created** agent file is often **missing** from the Task tool's `subagent_type` enum on the **same turn** it was written. Spawn fails with an invalid-enum error. Retry on a **later turn** after Cursor rediscovers agents. Editing an already-registered agent's body typically hot-reloads; **adding** a new name needs rediscovery.

## Model selection: Task arg vs frontmatter

**Two different surfaces, different syntax:**

| Surface | What you can pass |
|---|---|
| Task tool `model` argument | Only a **small allow-list of flat slugs** (non-empty). The validation error lists the current set. Empty string is rejected. **No** square-bracket parameter syntax. |
| Agent frontmatter `model` | **Full model selection syntax**, including `inherit` and bracketed forms like `claude-sonnet-4-6[thinking=true,effort=high,context=1m]`. |

So: if you need thinking/effort/context/fast knobs (or any model the Task enum cannot express), put it on the agent's **frontmatter** `model`, spawn with **no** Task `model` arg, and let the pin apply. Task `model` cannot carry that syntax.

### Resolution (verified)

Empirically verified (pin = agent YAML `model: <slug-or-bracket-form>`; unpinned = no `model` field in frontmatter):

| Agent frontmatter | Task spawn `model` | What actually runs |
|---|---|---|
| pinned | **omitted** | **the pin** — not the parent chat model |
| pinned | `"inherit"` | **the pin** — not the parent chat model |
| pinned | explicit allow-listed slug | **the spawn slug** (override wins) |
| unpinned (no `model` field) | **omitted** | **parent** chat model |
| unpinned | `"inherit"` | **parent** chat model |
| unpinned | explicit allow-listed slug | **the spawn slug** |
| any | `""` (empty string) | **rejected** — Task validation error; subagent never starts |

### Implications for authors

1. **Omit Task `model` when you want inheritance or a frontmatter pin.** Do not pass `model: ""`.
2. **`"inherit"` as a Task arg ≠ "always use the parent".** With an unpinned agent it matches the parent. With a **pinned** agent it still resolved to the **pin** in testing — same as omitting the arg.
3. **Frontmatter pin beats parent.** If the agent YAML pins a model, omitting Task `model` (or passing `"inherit"`) does **not** cascade the user's chat model.
4. **Explicit Task slug always wins** over a frontmatter pin when you need a one-off override — but only with an allow-listed flat slug.
5. Task help text that says "if omitted, uses the parent model" is only accurate for **unpinned** agents.

### Product pattern: inherit the user's model

- Agent frontmatter: **`model: inherit`** (or omit `model`)
- Parent spawn: **no `model` argument** on Task
- Never bake a concrete model into product-runtime agent frontmatter unless you intentionally want that pin

### Product pattern: force a fixed / parameterized model

- Agent frontmatter: `model: <flat-or-bracket-form>` (use brackets here when you need parameters)
- Parent spawn: omit Task `model`
- Only pass Task `model: <allow-listed-flat-slug>` when you deliberately override the pin

## Frontmatter model slug syntax (not exhaustive)

`inherit` — follow the parent (when Task omits `model` / does not override).

Bare id equals the noted defaults; brackets set parameters explicitly:

```
composer-2.5                          # equals fast
composer-2.5[fast=bool]

claude-sonnet-4-6[]                   # equals thinking, medium, 200k
claude-sonnet-4-6[thinking=bool,context=200k|1m,effort=low|medium|high|max]

claude-opus-4-8[]                     # equals thinking, high, 300k, non-fast
claude-opus-4-8[thinking=bool,context=300k|1m,effort=low|medium|high|xhigh|max,fast=bool]

claude-fable-5[]                      # equals thinking, high, 300k
claude-fable-5[thinking=bool,context=300k|1m,effort=low|medium|high|xhigh|max]

gpt-5.5[]                             # equals medium, 272k, non-fast
gpt-5.5[context=272k|1m,reasoning=none|low|medium|high|extra-high,fast=bool]
```

This list is illustrative, not complete — other families exist. Bracket forms belong on **frontmatter** `model` only; Task `model` accepts only its flat allow-list.

## How a subagent learns its model

Two different facts; only one is injected automatically.

### Effective (running) model — yes, Cursor injects it

Cursor puts the **model the subagent is actually executing as** into the subagent's own developer / system / communication instructions (e.g. "powered by Gemini 3.6 Flash", "You are Composer…", "You are Cursor Grok 4.5…"). The subagent can read that from its session context with **no tools**. Display names vary by family; they are not always the Task allow-list slug.

Instruct the subagent (in its agent body or in the Task `prompt`) to report that identity when you need it — e.g. for model-aware behavior or diagnostics.

### Task `model` argument — not visible unless you pass it in the prompt

The value the **parent** passed to Task's `model` parameter (omitted, `"inherit"`, or a slug) is **not** exposed as separate metadata to the subagent. Neither is the agent's YAML frontmatter `model` field. Verified: when Task was called with `model: "gpt-5.6-luna-medium"` or `model: "inherit"`, the child could see only its running identity, and reported `TASK_MODEL_SPECIFIED: NOT_VISIBLE` / `FRONTMATTER_MODEL: NOT_VISIBLE`.

If the child must know what was **requested** on the Task call (especially when that can differ from what it runs — e.g. Task `"inherit"` while frontmatter pins another model), the **parent must include that value in the Task `prompt`** (or bake the expectation into the agent body). Do not assume the subagent can recover the Task arg from system context.

| Fact | Visible to subagent? |
|---|---|
| Effective / running model | **Yes** — developer/system/communication identity |
| Task `model` arg as specified | **No** — put it in the Task `prompt` if needed |
| Agent frontmatter `model` | **No** — same; prompt or agent body must say if it matters |

## Resume

Do not pass `model` when `resume` is set — resume keeps the subagent's existing model.

## Checklist for parent prompts that spawn subagents

- [ ] `subagent_type` matches the agent's frontmatter `name` exactly
- [ ] New agents: expect possible same-turn discovery miss; retry next turn
- [ ] Omit Task `model` unless intentionally overriding with an allow-listed flat slug; never pass `""`
- [ ] Parameterized models (brackets) go on frontmatter `model`, not on Task
- [ ] If the product needs the user's model, ensure the agent is unpinned / `model: inherit` and spawn without Task `model`
- [ ] If the child must know the Task `model` arg (not just what it is running as), include that value in the Task `prompt`
- [ ] Put all task operands in the Task `prompt` (no `$1` / `$ARGUMENTS` macros in agent files)
