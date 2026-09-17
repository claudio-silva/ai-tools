# Token Economy — evidence record

State as of 2026-09-10 on `codex-cli 0.154.0`, desktop app running `multi_agent` **v2**. This record is not loaded by the skill.

> **Revision 2 (2026-09-12).** The skill was redesigned after a review session; the redesign is drafted, not installed, and not yet measured. Sections up to *Superseded* describe the **installed** v1 skill and its measurements, which remain valid as evidence. The section **Revision 2 — redesign, reference data and open work** at the end records the new palette, the capability and cost tables gathered from public sources, the reasoning behind each change, and what a later thread must do to install and test it. Read that section first when resuming.

## What is installed

| Part | Location |
| --- | --- |
| Skill (2 files) | `~/.codex/skills/token-economy/` — `SKILL.md`, `agents/openai.yaml` |
| Hard dependency | `who-am-i` skill and `~/.codex/scripts/codex-who-am-i` |
| Worker roles (5 files) | `~/.codex/agents/te_{retrieve,trace,build,plan,debug}.toml` |
| Advisory roles (2 files) | `~/.codex/agents/te_advise.toml`, `te_advise_deep.toml` |

The skill is explicit-invocation only (`allow_implicit_invocation: false`). Roles are registered globally, discovered at process start, and are a separate install from the skill: a missing role does **not** raise an error, it yields a generic child with none of its instructions. Every role therefore opens its reply with `WORKER: <role>` as a fail-closed binding check.

## Verified by direct experiment

| Fact | Method |
| --- | --- |
| A role's TOML `model` / `model_reasoning_effort` pin **survives a full-history fork** | Spawned a role pinned `gpt-5.6-terra`/`low` from a `gpt-5.6-luna` parent passing no `model`, `reasoning_effort` or `fork_turns`; child `turn_context` recorded the pin. Reproduced under v1 and v2. |
| The runtime's "no overrides" warning applies to **spawn arguments only** | Same experiment; spawn args are ignored on a full fork, pins are not. |
| A child can spawn a grandchild on default config | Nested spawn returned `DEPTH_OK`; no `[agents]` section present in `config.toml`. |
| All seven roles bind, with their pins in effect | Seven spawns returned their `WORKER:` line; thread records showed `luna/low`, `luna/max`, `terra/max`, `sol/medium`, `sol/high`, `astra/low`, `astra/high`. |
| **Forked threads start cold on the first call**, whatever the model | First-call cache: 0% on a cross-model fork (luna → terra), 8% and 0% on same-model forks, 0% and 14% on later consult forks. The 8–14% is a partial hit on the shared system-prompt head. |
| Reasoning tokens bill at the **output** rate | `total_tokens = input_tokens + output_tokens` in every record, with `reasoning_output_tokens ≤ output_tokens`. |
| `task_name` accepts only `[a-z0-9_]` | Hyphens, brackets, uppercase and spaces each rejected before launch. |
| `followup_task` works on a **finished** worker | Two probes followed up on a thread that had already returned its result and been awaited; the second turn ran in the same thread, at the same pin, retaining its earlier reading. |
| **An agent cannot determine its own model or effort unaided** | The full model-visible prompt (`codex debug prompt-input`, 22.5 KB, 6 messages) states neither. Asked directly, `luna/low`, `sol/medium` and `astra/low` each answered `UNKNOWN`. The `who-am-i` utility supplies it from runtime evidence instead, which is why it is a hard dependency. |
| A `UserPromptSubmit` hook exists but **cannot route** | Present in the `HookEventName` enum of `codex-cli 0.154.0` alongside `PreToolUse`, `PermissionRequest`, `PostToolUse`, `Pre/PostCompact`, `Session/SubagentStart`, `Session/SubagentStop`, `Stop`, `Interrupt`. Its output struct carries only `hookEventName` and `additionalContext`; `updatedInput` exists for `PreToolUse` alone, and no hook output can set a model. |
| Catalog presence is not entitlement | `gpt-5.5` is in the local catalog and returns 404 on use. |
| Live efforts | Sol / Terra / Astra `low…ultra`; Luna `low…max` (no `ultra`). |
| Concurrency | Runtime injects 4 slots "including you", so 3 workers by default; overflow is a hard error, not a queue. Tunable via `[agents] max_concurrent_threads_per_session`. |

## Measured: full-history fork vs targeted brief

Two trials, same pinned role, same model and effort, same task, same parent context (~87k tokens: 12 relevant service modules, 12 irrelevant UI modules, one README). Only `fork_turns` differed. Order was reversed between trials to expose cache-warming effects.

| Trial | Arm | Calls | Peak context | Cumulative input | Cached | Output | Correct |
| --- | --- | --: | --: | --: | --: | --: | --- |
| 1 | fork (`fork_turns` omitted) | 1 | 86,682 | 86,682 | 8% | 80 | yes |
| 1 | brief (`fork_turns="none"`) | 3 | 28,465 | 74,743 | 74% | 378 | yes |
| 2 | brief (`fork_turns="none"`) | 4 | 28,223 | 102,390 | 64% | 530 | yes |
| 2 | fork (`fork_turns` omitted) | 1 | 88,562 | 88,562 | 0% | 84 | yes |

The brief cost **0.33× and 0.52×** the fork — the fork was 1.9–3.0× more expensive — for identical correct answers. The ratio is model-independent because both arms ran the same pin. Both arms found the three planted deviations.

Mechanism: **forked threads start cold.** An earlier probe appeared to show a 98% first-call cache hit on a small context; re-reading that thread's records in order showed its *first* call was 23,593 input with 0 cached, and the 98% figures were its second and third calls — ordinary intra-thread caching. The same correction applies to the brief arm's high cache share here.

**This result is scoped to mechanical delegation with known paths.** The brief named twelve exact file paths, so the worker read precisely what was needed. The consult measurements below show the advantage shrinking, and in one pair reversing, once the worker has to decide what to read.

Two by-products: every spawn pays a floor of ~18–20k input tokens before task content (measured in a bare session; higher in a real project), and the brief arm's worker held a 28k working context against the fork's 87k.

`thread_token_usage.input_tokens` is a **sum over API calls**, not a context size, so a worker making many tool calls re-sends its growing context each time. Peak single-call input approximates context size.

Reproduction: `token-economy-support/` — `gen.py` builds the corpus deterministically, `build_prompt.py` assembles either arm ordering, `extract.py` reads per-thread usage from the session rollouts. Whole measurement cost roughly 2.5 credits.

## Measured: advisory consult, inherited history vs curated brief

A judgment task on the same corpus plus a reasoning trajectory (a duplicate-charge investigation with four hypotheses already ruled out). The answer requires combining `billing.ts` (`retryAttempts: 7`, `timeoutMs: 2000`, `backoffFactor: 2.0`) with `payments.ts` (`idempotencyWindowMs: 4000`) — a retry can outlive the deduplication window. Neither file states it alone. Same pinned advisor per experiment; only `fork_turns` and the message differed, with order reversed between trials.

| Advisor | Trial | Arm | Calls | 1st-call ctx | 1st cache | Uncached | Output | Cost at that model | Found billing values |
| --- | --- | --- | --: | --: | --: | --: | --: | --: | --- |
| luna max | 1 | inherit | 1 | 87,169 | 0% | 87,169 | 2,609 | 0.51 | **no** |
| luna max | 1 | brief | 7 | 18,654 | 54% | 28,986 | 3,320 | 0.32 | yes |
| luna max | 2 | brief | 8 | 18,654 | 0% | 58,495 | 3,675 | 0.52 | yes |
| luna max | 2 | inherit | 1 | 89,049 | 11% | 79,065 | 767 | 0.42 | **no** |
| sol max | 1 | inherit | 1 | 92,631 | 0% | 92,631 | 773 | 9.65 | yes |
| sol max | 1 | brief | 4 | 20,535 | 62% | 23,510 | 3,122 | 4.83 | yes |
| sol max | 2 | brief | 6 | 20,535 | 0% | 42,132 | 3,074 | 7.15 | yes |
| sol max | 2 | inherit | 1 | 94,511 | 14% | 81,711 | 713 | 8.66 | yes |

Brief/inherit cost ratios: **0.61, 1.20** at luna max and **0.50, 0.83** at sol max. The brief was cheaper in three of four pairs. The variance sits entirely in the brief arm, whose cost depends on how much the advisor chooses to read; the inherited arm is one predictable uncached pass.

Two findings drive the guidance:

- **A stronger advisor reads less.** Four and six calls at sol max against seven and eight at luna max, so the brief's advantage widens rather than eroding as the advisor gets stronger.
- **Inheriting substitutes salience for search, at weak tiers only.** At luna max the inherited arm twice failed to cite billing's retry configuration while holding it in context, once writing "the exact retry implementation/timing is not included here". At sol max the inherited arm cited all of it in both trials. The effect is a property of the tier, not of inherited context, so the skill records it on the delegation path (where cheap workers can be forked) rather than on the consult path.

Both arms identified the mechanism in all eight runs; the difference was in citing the decisive values. Cost of this pair of experiments: roughly 3 credits at luna max and 35 at sol max, plus about 7.4 credits per trial for the terra/max parent — which exceeded the sol/max advisor arm in one trial, and is a reminder that max effort on an orchestrating agent is waste.

Reproduction: `plant.py` injects the cross-file interaction, `build_advice.py` assembles either arm ordering, `extract_advice.py` prices both arms at luna, sol and astra rates.

## Measured: reusing a worker vs spawning a fresh one

Two probes, both `te_retrieve` (luna/low) over the same 12-file corpus. Each ran a first task, then delivered a **second** task two ways: as a `followup_task` to the worker that had just done the first task, and as a self-contained brief to a newly spawned worker of the same role. The two probes differ only in whether the second task could be answered from what the first worker had already read.

| Probe | Second task | Route | Calls | Input | Cached | Output | Cost of the second task |
| --- | --- | --- | --: | --: | --: | --: | --: |
| A | needs material not yet read (`timeoutMs`, `backoffFactor`) | followup | 4 | 95,911 | 90.8% | 1,026 | 0.119 |
| A | same | fresh spawn | 4 | 97,716 | 84.9% | 1,009 | 0.146 |
| B | derivative of what was read (rank and sum the top three) | followup | 1 | 29,611 | 96.0% | 152 | 0.025 |
| B | same | fresh spawn | 8 | 220,791 | 82.7% | 1,345 | 0.323 |

Reuse is **0.81x** a fresh spawn when the follow-up needs new material, and **0.08x** when it does not. Costs are in luna credits; the ratio is model-independent, because the cached rate is one tenth of the input rate for every model.

The mechanism matters more than the ratio. In probe A the reused worker re-read all twelve files rather than answering from its own context, doing the same 4 calls and nearly the same input volume as the fresh worker — so the entire saving came from a higher cache share, not from avoided work. Reuse does **not** reliably make a worker trust what it is already holding. It pays when the question is genuinely derivative, where probe B's reused worker answered in a single 96%-cached call against eight calls and 221k input for the worker that had to read first.

Two incidental observations. A fresh spawn's first call was 85% and 83% cached here, far above the cold start seen on forks, because sibling threads in the same session share the prompt head and the corpus had just been read. And probe B's fresh worker spent 8 calls and 221k input on a task the reused worker closed in one — asking a cheap worker to both gather and rank is markedly less efficient than splitting it.

Reproduction: `gen.py` and `plant.py` build the corpus; `extract_reuse.py` groups `token_usage_record` entries by `turn_id` and takes each turn's last record as its total.

## Measured: the procedure end to end

One request, given to a `luna/medium` parent with no routing hints beyond `Use $token-economy`: find which of four service configs declare `retryAttempts` below 3, then write a runnable test asserting all declare at least 3.

The parent split it into two parts and routed them in sequence without prompting — retrieval to `te_retrieve` (`luna/low`, 59,576 total tokens) and the test to `te_build` (`terra/max`, 146,888), against its own 230,713. It then ran `node --test`, reported exit code 1 with `tests 5 / pass 2 / fail 3`, and named the three configs at fault. The artifact exists at the stated path and an independent re-run reproduces those counts.

An earlier attempt at the same request exposed a real gap. That run was under `codex exec`'s default read-only sandbox, so `te_build` could not write the file. The parent nonetheless presented the file's contents as the delivered outcome and never surfaced that the write had been blocked; the file did not exist. The environment was the harness's fault, but the omission was the skill's, so step 5 now makes two checks mandatory: confirm an artifact exists at its stated path, and require a test's actual output rather than its existence.

## Measured: lateral delegation is waste, and delegation overhead is charged to the parent

An earlier version of this document claimed that a clean worker context makes delegation cheaper *even at the same model*, by roughly 3x. **That was an estimate, it was never measured, and the measurement refutes it.**

Two trials, order reversed. A `luna/low` parent carrying unrelated filler context had to inventory the same twelve service files on disk. In one arm it did the work itself; in the other it briefed `te_retrieve`, which is also pinned `luna/low`, making the spawn purely lateral.

| Trial | Arm | Threads | Parent input | Worker input | Total, luna credits |
| --- | --- | --: | --: | --: | --: |
| 1 | in-thread | 1 | 110,686 | — | 0.317 |
| 1 | lateral | 2 | 165,846 | 39,602 | 0.494 |
| 2 | lateral | 2 | 166,168 | 82,846 | 0.398 |
| 2 | in-thread | 1 | 110,780 | — | 0.320 |

Lateral delegation cost **1.56x and 1.24x** doing the work in-thread. The mechanism is the important part: delegating raised the **parent's own** input from 110,780 to 166,168, a factor of 1.5, because spawning, waiting and reading a result add turns to the parent's thread and every turn re-sends the parent's whole context. The worker's tokens then land on top of that.

Repricing the same measured token counts for a `sol/medium` parent delegating to a `luna/low` worker gives 6.13 credits against 6.37 in-thread — **0.96x, essentially break-even**. So for a task of this size the rate differential barely covers the parent's extra turns. Delegation has to clear that overhead before the cheaper rate is worth anything.

## Measured: `who-am-i` resolves the self-knowledge problem

A `terra/medium` parent ran `$CODEX_HOME/scripts/codex-who-am-i` and reported `gpt-5.6-terra` / `medium`; a `te_retrieve` child spawned with `fork_turns="none"` ran the same utility and reported `gpt-5.6-luna` / `low`. Both were classified `effective-runtime-value` with **high** confidence, and both matched the pins. Outside a Codex session the same utility falls back to `configured-default` at low confidence, correctly labelled, because it has no `CODEX_THREAD_ID` to correlate.

JSON is its default output; there is no `--json` flag. The skill therefore has a hard dependency on that utility, and treats anything other than `effective-runtime-value` at high confidence as "position unknown".

## Measured: routing behaviour at three positions

Same skill, same corpus, no routing hints beyond `Use $token-economy`.

| Main agent | Request | Outcome |
| --- | --- | --- |
| `luna/low` | inventory twelve small files | kept it: reported `POSITION=gpt-5.6-luna/low`, `DECISION=self`, reasoning that the row matched its own execution level |
| `sol/medium` | inventory twelve small files | kept it — correct, since this is the 0.96x break-even case above |
| `sol/medium` | write and run a test over six configs | kept it |
| `sol/medium` | read 24 modules and write a judged paragraph on each | delegated to `te_trace` |

The first three are the intended behaviour: no rate differential in the first, and too little material in the others. The fourth shows delegation still fires when the material genuinely has to be read and judged. That contrast also corrected the wording of step 1: the original gate spoke of work that would take "many tool calls", but a capable agent collapses many calls into one script, which is cheaper than delegating. The gate now turns on the volume of material that must enter a context, and step 1 says to try to shrink the work with a script before delegating what survives.

## Choosing the main model: modelled, not measured

This left `SKILL.md` because the agent cannot act on it — the user picks the model. It is retained here as a decision aid, and it is arithmetic on published rates, not an experiment.

The main agent's context is re-sent every turn, so its rate is a fixed cost of the whole conversation, paid on the turns that needed premium reasoning and equally on those that did not. Astra is 2.5x Sol and 5x Terra on every axis, so a strong centre costs 2.5x the whole main thread, not 2.5x the hard turns.

Worked example: a 40-turn chat averaging 60k context at a 90% cache share with 2k output per turn costs about 2.1 credits per turn at Sol and 5.4 at Astra — roughly 86 against 214 credits. Aggressive downward delegation shrinks the strong centre's context growth, so call the realistic gap 60 to 130 credits. A briefed Astra consult runs about 10 credits and one inheriting a large history about 25, so a cheap centre stays ahead while it escalates fewer than roughly six to eight times across those 40 turns: about one turn in seven.

Two asymmetries favour the strong centre, and neither is a cost argument. A strong centre only decides whether a task is mechanical enough to push down, which is a judgment about the task and is checkable; a cheap centre must decide whether a problem is beyond it, which is a judgment about itself and is not — hence the record-keyed triggers in step 6. And the failures differ in kind: an over-strong centre wastes credits visibly and recoverably, while an under-strong centre that misses a trigger ships a weak decision silently, where the rework can exceed everything the cheaper rate saved.

## Not verified

- Any five-hour or weekly subscription saving. Token credit rates are published per token; they do not map to a completed task or an allowance percentage, and no such claim is made in the skill.
- **The "one turn in seven" crossover** above. Arithmetic on the published rates under a stated example, not an experiment: the rate ratios are published, the chat shape is an assumption, and a different shape moves the figure.
- **Whether a Codex hook can be made to run at all on this install.** The `UserPromptSubmit` event and its `additionalContext` output are in the 0.154.0 binary, and its input payload includes a `model` field, which would supply the one fact an agent cannot otherwise learn about itself. But across five attempts in a scratch `CODEX_HOME` — `hooks.json` in Claude-Code shape, `[[hooks.UserPromptSubmit]]` in `config.toml` with `command` as a string, `features.hooks = true`, and a `[hooks.state.<key>] enabled` entry — the config parsed and the handler never executed. `HookStateToml { enabled, trusted_hash }` suggests a trust gate that is granted through the app rather than the file. The skill therefore depends on no hook.
- Whether a worker's cache survives an idle gap before a follow-up. Both reuse probes followed up within the same minute. A cold follow-up on a large thread is the failure case that would invert the reuse rule, and it is untested.
- Whether either main-model arrangement performs better in practice. Only their relative token cost is modelled; no chat has been run both ways.
- Whether the role palette's boundaries are optimal. Only that they bind and run as pinned.
- Whether the two `astra` rows give *good* advice. Only that they bind and run as `astra/low` and `astra/high`; no route has been run against a real decision.
- The trajectory-as-evidence case. Both fork-vs-brief experiments used artifacts as the evidence, which is the case that favours briefs by construction.
- Skill-picker discovery of the rewritten skill in a fresh desktop chat, which needs an app restart to pick up the new roles.
- Whether a TOML `sandbox_mode` pin is enforced. All seven roles deliberately omit it and inherit the session policy instead.

## Superseded

The first version of this document reported a 12-scenario routing evaluation and a Luna retrieval probe against a 1,763-word single-agent `SKILL.md` with a `references/calibration.md` pricing appendix. That evaluation ran against a draft rather than the shipped wording, exercised two of seven routes, and measured no cost. The pricing appendix has been removed: it recorded bootstrap assumptions and a rate table that the routing instructions do not need.

Three later constructs have also been dropped from the skill, each for a stated reason:

- **The worker/consult split into two tables.** It encoded a relative distinction — cheaper or dearer than me — that an agent could not evaluate before `who-am-i` supplied its position. Replaced by one absolute table of seven rows keyed on task type, with a single comparison against the agent's own position in step 3.
- **The claim that lateral delegation pays.** Measured at 1.24x to 1.56x the cost of in-thread work; the section above records the refutation.
- **The `te_plan` coordinator mode.** Never run end to end, and incompatible with the sequential one-worker-at-a-time procedure. The provision has also been removed from `te_plan.toml`, which now matches the other roles in refusing to spawn.
- **Parallel dispatch, concurrency slots, disjoint write paths and worktrees.** Removed as unnecessary complexity for a procedure that runs parts in sequence. *Reinstated in Revision 2 as staged dispatch; see below.*

---

# Revision 2 — redesign, reference data and open work

Date 2026-09-12. Produced in a review session (Cursor, Claude) over the installed v1 skill and this record; no Codex probes were run in that session, so everything in this part is **design plus public data**, not measurement. Numbers here are for the record and for designing tests; the skill itself now carries **no numbers**, by decision (see *Design decisions*).

## Where the drafts are

| Artifact | Path | Status |
| --- | --- | --- |
| Rewritten skill | `/Users/claudiosilva/Projects2/Skills/Codex/auto-routing/SKILL.md` (skill name `auto-routing`) | draft; install with `bin/install` |
| Nine role TOMLs | `te-agents/te_*.toml` in the same project | draft; `bin/install` copies them to `~/.codex/agents/` and removes `te_debug.toml` and `te_advise_deep.toml`; app restart needed for role discovery |
| Skill metadata | `agents/openai.yaml` | `allow_implicit_invocation: false`; invocation is `$auto-routing` |
| Installed v1 | `~/.codex/skills/token-economy/SKILL.md`, `~/.codex/agents/te_{retrieve,trace,build,plan,debug,advise,advise_deep}.toml` | still the live version until `bin/install` runs |

## The user's goal, restated

Subscription billing (5-hour and weekly Codex allowances), not API credits. Allowance depletes too fast. Wants Astra used more often where it matters, without switching models by hand per task, and without the main thread carrying premium rates for work that does not need them. Private, single-developer use on macOS; Python available. The Realizer skill family (`rn-plan`, `rn-task-planner`, `rn-review`, `rn-run` installed as Codex skills; Lite, Pro, Architect and DevUnit specs in `~/My-GitHub-Projects/Realizer-dev`, not yet Codex-compatible) will run on top of this skill, so it must route well for decomposition, implementation and independent review without those skills naming models.

## Reference data gathered (public sources, 2026-09-11/12)

Confidence is marked per table. "Official" means OpenAI help center / developer docs; "aggregator" means Artificial Analysis; "secondary" means blog posts, Reddit threads or AI-generated summaries supplied by the user.

### Allowance consumption per model — official

Published Codex 5-hour ranges (help center, *Managing usage with GPT-6 Astra in Work and Codex*, Business Standard seat; Plus/Pro keep the same relative shape):

| Model | 5-hour range (messages) | Relative weight (Luna = 1) |
| --- | --- | --- |
| GPT-5.6 Luna | 250–2,000 | 1 |
| GPT-5.6 Terra | 25–200 | 10 |
| GPT-5.6 Sol | 10–100 | 20 |
| GPT-6 Astra | 5–45 | ~45–50 |

**Conclusion:** the subscription meter is model-weighted in the same proportions as API input pricing, so routing down genuinely preserves allowance. Whether **cached input** is discounted against the allowance is **not documented** anywhere official; the API discounts it 10× for every model. The reuse rule's saving (probe A above was entirely cache share) depends on this. **First test to run:** same short task in fresh threads at Luna low and Astra low, allowance percentage before/after; then the same with a warm cache.

### API prices per 1M tokens — official (developers.openai.com/api/docs/pricing, standard short context)

| Model | Input | Cached input | Output | Input ratio | Output ratio |
| --- | --- | --- | --- | --- | --- |
| GPT-5.6 Luna | $0.20 | $0.02 | $1.20 | 1 | 1 |
| GPT-5.6 Terra | $2.00 | $0.20 | $12.00 | 10 | 10 |
| GPT-5.6 Sol | $4.00 | $0.40 | $20.00 | 20 | 16.7 |
| GPT-6 Astra | $10.00 | $1.00 | $50.00 | 50 | 41.7 |

Long-context tiers are higher (Astra: $20 / $2 / $75). A user-supplied summary claimed "1:2:5:10 on inputs"; that is wrong for Luna at $0.20 (it holds only for Terra:Sol:Astra = 2:4:10). Reasoning tokens bill at the output rate (verified above in v1).

### Intelligence Index by reasoning effort — secondary (user-supplied table attributed to Artificial Analysis v4.3)

| Effort | Luna | Terra | Sol | Astra |
| --- | --- | --- | --- | --- |
| low | 41 | 45 | 51 | 54 |
| medium | 44 | 47 | 54 | 56 |
| high | 46 | 49 | 56 | 58 |
| xhigh | 49 | 52 | 57 | 60 |
| max | 51 | 55 | 59 | 62 |

Efforts available live (verified in v1): Sol/Terra/Astra `low…ultra`, Luna `low…max`. Ultra is excluded by the user (too expensive; reported to launch many subagents; documentation inconsistent on whether it is an effort value).

**Dominance reading, which drove the palette:**

- Luna max (51) ≈ Sol low (51) ≈ Terra xhigh (52) — Luna max at ~1/20 of Sol's rate.
- Terra max (55) ≈ Sol medium (54) ≈ Astra low (54).
- Sol high (56) ≈ Astra medium (56). Sol max (59) ≈ Astra high (58).
- Only Astra xhigh/max (60–62) is unreached by anything cheaper.
- Every Astra effort below xhigh is matched by a Sol effort at 40% of the rate; Terra is matched by Luna max at every effort except Terra max.

The index is one aggregate number and does not measure coding reliability, tool use or the profile capabilities below; it was used to check that tiers are monotone and distinct, not to settle quality.

### Cost per Intelligence Index task — aggregator (Artificial Analysis articles, partial)

| Model / effort | Cost per index task | Source |
| --- | --- | --- |
| Luna max | ≈ $0.21 (user summary: $0.18–0.21) | *gpt-5-6-has-landed*; model page reports $319.93 total for the index run |
| Terra max | $0.55–1.40 (user summary only) | secondary |
| Sol max | ≈ $1.04 (user summary: $1.04–2.82) | *gpt-5-6-has-landed* |
| Astra low | ≈ $0.82 | *gpt-5-6-intelligence-vs-cost-across-sol-terra-luna* |
| Astra max | up to ≈ $2.57 (user summary only) | secondary |

Reading: Astra low costs about four Luna-max tasks and ~80% of a Sol-max task; Astra max per task is in the same band as Sol max despite 2.5× the rate, which is consistent with Astra emitting fewer reasoning tokens. These are short-context benchmark tasks; on agentic work with a large re-sent context, input dominates and the **rate ratio** reasserts itself.

### Capability profile claims — secondary (user-supplied compilation; sources: openai.com/index/gpt-6-astra, developers.openai.com blog, Artificial Analysis, several blogs)

Astra is claimed to differ from Sol in kind, not only in score:

1. Spatial reasoning and native 3D/CAD work (Blender geometry, Cycles, Blender→Unreal pipelines, schematic→KiCad PCB layout).
2. Computer use as an operating mode (screen-to-action loops, OSWorld ~47% faster than prior models, mid-task steering without losing the mission).
3. Long-horizon autonomous coding with **token parsimony** (claimed one-third of Sol's tokens on long SWE tasks; fewer, more precise turns), plus SRE/security diagnostics.
4. Hallucination suppression on dense documents (legal, financial), ~half Sol's rate.

Independent check (Perplexity over official and aggregator sources): **no controlled evidence** was found for the token-parsimony claim (benchmarks report success rates, not token totals), none for low effort degrading computer-use accuracy, and no official effort taxonomy. Practitioner heuristics only: high as coding default, xhigh for hard debugging, max for the hardest architectural or long-running work.

**Consequence for the skill:** two overrides instead of one. The *consequence* override (existing) routes decisions to Astra for its ceiling; the new *capability* override routes execution to Astra for its profile, at moderate effort because the bottleneck is what the model knows, not how long it thinks. The parsimony claim, if it holds, would make Astra the *cheap* choice for long unsplittable implementation; it is recorded as a conditional in this record and **not** in the skill until measured.

### Coding: Luna max vs Sol vs Terra — secondary plus OpenAI positioning

- OpenAI's "Sol can now delegate to Luna" guidance (community.openai.com thread 1390765; runtimewire summary) positions Luna as a *pure subagent* for bounded, fully specified work with all context in the initial prompt, and Sol as the model that decides which files change and preserves interfaces.
- Reported Luna-max failure modes (practitioner reports, not quantified): locally correct edits that miss a repository-wide invariant; tests that mirror the prompt rather than discover edge cases; stopping after a narrow test passes; overthinking mechanical tasks at max; cost inversion when a Luna failure forces a Sol redo.
- User-supplied AI summaries rank Sol medium as the best coding balance (above Terra max), and Luna max above all Terra efforts below xhigh.

**Consequence:** the Luna/Sol boundary for implementation is the **brief-shape test** (files by absolute path + invariants + acceptance command all stated → `te_build` at Luna max; otherwise `te_engineer` at Sol medium). Economics: Luna at ~1/20 of Sol's rate makes a failed Luna attempt plus a Sol redo cost only marginally more than Sol alone, *provided the failure is detectable* — hence the acceptance-command requirement and the mechanical diff-against-brief check. Terra dropped: no niche between Luna max and Sol low/medium.

### Codex instruction-discovery facts — official (developers.openai.com/codex/guides/agents-md, config schema)

Recorded because a persistence design was explored and rejected; a later attempt should start from these:

- The instruction chain is built **once per session**, not per turn. Files created mid-session load on the next thread.
- Per directory: `AGENTS.override.md`, else `AGENTS.md`, else `project_doc_fallback_filenames` in order; **at most one file per directory**. A fallback name is ignored wherever `AGENTS.md` exists. Global scope: `~/.codex/AGENTS.override.md` else `~/.codex/AGENTS.md`; the docs describe the global override as a temporary switch ("remove the override to restore").
- Project docs are concatenated root→cwd under `project_doc_max_bytes` (default 32 KiB); project-level `.codex/config.toml` exists for trusted projects.
- Subagents receive the same project docs and config as the parent, so anything placed in the instruction chain reaches every worker.
- A `$skill` invocation injects the skill body as turn context (history), not as developer instruction; it is not guaranteed to survive compaction verbatim. `AGENTS.md`-class instructions are reapplied after compaction.
- Hooks: `UserPromptSubmit` (with `additionalContext`), `PreToolUse` (only event with `updatedInput`), `Pre/PostCompact` exist in the 0.154 binary; no handler could be made to run (five attempts, see *Not verified* above); `HookStateToml { enabled, trusted_hash }` suggests a trust grant through the app.

## Design decisions in Revision 2 and why

1. **All empirical numbers removed from the skill.** They pin the skill to one Codex version and one pricing table, invite the model to do arithmetic, and go stale silently. Each number was replaced by the mechanism it supported (context re-sent every turn; forked threads start cold; reuse pays only for derivative questions; the dearest rows carry a fixed floor). Numbers live here.
2. **Palette by dominance, not by taste.** Four tiers routed by task type (Luna low, Luna max, Sol medium, Sol high) plus three Astra roles reached only by override or by an executor label. Terra dropped. Sol kept as the single mid tier because everything below Astra xhigh is matched by a Sol effort at 40% of Astra's rate.
3. **Nine roles, seven pins.** The user prefers more roles with per-role calibration over fewer tiers that force judgment into the model. Roles are about ownership and instructions; pins are about cost; a pin can be retuned without touching routing.
4. **Two overrides.** Consequence → `te_advise` (Astra max, recommendation only, single-shot on a distilled brief, which is the only shape where max is affordable). Capability → `te_drive` (Astra low, bounded GUI/browser scenarios; low because the bottleneck is perception/action and GUI loops are input-dominated so effort barely moves the bill) or `te_operate` (Astra high, free-form automation, spatial, dense documents, production diagnostics, *Authored* deliverables).
5. **Brief-shape test** decides Luna vs Sol for implementation (see coding section). Never re-brief a Luna builder twice on the same problem; escalate to `te_engineer`.
6. **Staged parallel dispatch** reinstated: independent parts of a stage spawn together and are awaited together, amortizing the parent's turns (the dominant overhead measured in v1). Serial within dependency chains. Governing skills that require serial lanes (`rn-run`) win.
7. **Reuse rule narrowed**: derivative questions only; open follow-ups with "answer from what you already read"; every role ends with a `READ:` line so derivativeness is judged from the record; cold after a long gap or a compaction; persistent lanes demanded by a governing skill override the rule.
8. **`fork_turns="none"` kept as default**, with one exception: a `te_advise` consult whose evidence is the parent's reasoning trajectory. v1 data: brief 0.33–0.52× of fork for mechanical work; 3 of 4 consult pairs cheaper (one reversed at 1.20×); cheap tiers answer from salience when forked.
9. **Context-persistence reason to delegate** added to step 1 and as the one exception to "same tier → do it yourself": material that would otherwise stay in the main thread's context for the rest of the chat (a plan, its satellites, the files it touches) is worth moving to a worker even at the parent's tier, because v1 measured lateral delegation on a one-off task in a thread that then ended, and never measured the compounding cost of carrying the material through a long session.
10. **Reviews made explicit**: acceptance check (parent) vs mechanical checks (`te_retrieve`) vs judgment review (`te_review`, Sol medium, read-only, fresh thread, never the implementer's thread; Astra low as the independent reviewer when the implementer was Sol and independence matters). Not automatic in ordinary flows; always in Realizer flows.
11. **Nesting allowed**, down-only (to `te_retrieve` or the worker's own tier); sidesteps for unbound role (respawn once, then explicit model/effort generic spawn labelled `<role>_fallback`), concurrency overflow (hold for next batch; nested worker does it in-thread), spawning unavailable, position unknown.
12. **`who-am-i` referenced as a contract** (four fields; accept only `effective-runtime-value`/high), not as an embedded command, so that skill can change its script without breaking this one.
13. **Governing-skill precedence section** added: Realizer's *Specified/Entangled/Authored* map to `te_build`/`te_engineer`/`te_operate`; per-task review lane = `te_review`; final transversal review = `te_plan` fresh; `te_plan` reads the code itself and may only push inventories down; generic lanes get a brief line saying this procedure does not apply to them.
14. **Main-thread recommendation** stated once (mid model at medium effort; max on an orchestrating thread is waste — v1 observed a Terra-max parent outspending its Sol-max advisor). Luna as main thread judged unsuitable for long sessions: the remaining main-thread work under this skill is judgment, which is Luna's measured weak point, and a long context raises the forgetting risk.

## Persistence across compaction — explored and parked

Concern: the skill body is history and may not survive compaction; a main thread that forgets it silently does everything itself. Options examined and the user's verdict:

- `~/.codex/AGENTS.md` routing card (always on): rejected — must not apply to every thread, and it reaches subagents.
- `.bootstrap.md` via `project_doc_fallback_filenames`: does not work where a project has `AGENTS.md` (fallback consulted only when absent), loads only from the next session, and reaches subagents.
- `~/.codex/AGENTS.override.md` written/deleted by a toggle skill (Codex's documented "temporary global override"): mechanically sound and global, but the user does not like the shape; reaches subagents unless roles are told to ignore it.
- `PostCompact` / `UserPromptSubmit` hook re-injecting the skill, keyed on a per-thread marker: the robust design; blocked until hooks can be made to run (trust gate).
- `PreToolUse` hook on `spawn_agent` using `updatedInput` to validate briefs and force `fork_turns="none"`: the only option that enforces rather than instructs; same blocker.

Decision for now: the skill is invoked with `$auto-routing` and carries the full text; re-invoke after a compaction; watch the `task_name` labels for drift. The main thread's small context under this procedure (reading pushed to workers) is itself the best mitigation, since it delays compaction.

## Tests a later thread should run (cheapest first)

1. **Allowance weighting and cache discount**: identical short task at Luna low vs Astra low in fresh threads; then a warm-cache repeat. Decides whether tiering and reuse save anything under the subscription.
2. **`te_build` reliability**: three build parts with full briefs at Luna max, each checked by acceptance command plus `te_retrieve` diff-against-brief; count silent invariant misses. If unreliable, the step up is Sol medium, not Terra.
3. **Parsimony**: one long, unsplittable implementation at Sol medium (`te_engineer`) vs Astra high (`te_operate`), allowance consumed. Decides whether "long-horizon → Astra" becomes a routing row.
4. **Decomposition**: one `rn-task-planner` run as `te_plan` (Sol high) vs in the main thread; compare allowance and the size of the main thread's context afterwards. Validates decision 9.
5. **Staged dispatch**: three independent retrieval parts spawned together vs sequentially; parent input tokens. Validates decision 6.
6. **Compaction survival**: force a compaction after `$auto-routing`, then ask the thread to summarize its current instructions; observe whether it still routes.
7. **Role binding after install**: nine spawns, one per role, each returning its `WORKER:` line and `READ:` line; check `thread_token_usage` records show the intended pins.

## Open questions carried forward

- Whether an agent TOML accepts `project_doc_max_bytes = 0` (would let retrieval roles skip project docs entirely).
- Whether `who-am-i` can expose a parent-thread id (would give workers a mechanical "I am a worker" test).
- Whether `wait_agent` accepts several ids in one call on this build (affects how much staged dispatch saves).
- Whether spawn-time `model`/`reasoning_effort` are honoured on a non-forked spawn (needed by the `<role>_fallback` sidestep; v1 only established that they are ignored on a full fork).
- How the advanced Realizer skills should express executor requirements without naming models: proposal is three kinds of statement — what settles the task (*Specified/Entangled/Authored*), independence (different thread / different model from the implementer), and relative strength (at least as strong; strongest available) — which this skill resolves to roles.
