---
name: auto-routing
description: "Cost-aware delegation for Codex: an operating procedure that routes each piece of work to the cheapest pinned worker role that can do it, keeps judgment with the main agent, and defers to any skill that governs the workflow. Requires the who-am-i skill and the te_* agent roles."
---

# Auto Routing

An operating procedure for spending tokens well. It stays active for the rest of the chat and is the explicit authorization Codex requires before spawning sub-agents. Acknowledge activation in one line, then work normally.

Run step 0 once, then steps 1 to 7 in order for each piece of work. Minimize the cost of *completing* the work — briefing, worker context and reasoning, output, retries, rework, and your own extra turns — not the unit price of a model. Do not forecast token counts, do not do pricing arithmetic, and do not narrate routing decisions to the user; the `task_name` labels in the UI already expose them.

Two mechanisms drive every rule below. **Every turn re-sends your whole context**, so anything that adds turns to your thread (spawning, waiting, reading a result) or bulk to your context (reading material yourself) is charged again on every later turn. **A worker's bill is its rate times the tokens it moves**, and on reading-heavy work the tokens are mostly input, which effort barely changes; effort matters on short, reasoning-heavy questions and is wasted on retrieval. The evidence behind the rules lives in the record `token-economy-validation.md`; the rules here carry the mechanism, not the measurements.

## Step 0 — Establish your own position

**Entry:** activation, before any routing decision.

Never infer your own model or effort: the prompt does not state them and a model asked directly cannot tell. Invoke the `who-am-i` skill and take four fields from it: `model`, `reasoning_effort`, `classification`, `confidence`. Accept the answer as your position only when the classification is `effective-runtime-value` and the confidence is `high`; a `configured-default` is the config file's value, not proof of what is running, so call your position unknown rather than guess.

Map your position to a tier:

| Your model and effort | Your tier |
| --- | --- |
| luna at low or medium | 1 |
| luna at high, xhigh or max | 2 |
| terra at any effort | between 2 and 3: tiers 1–2 are cheaper, tier 3 is dearer |
| sol at any effort | 3 |
| astra at any effort | above every tier |

Re-run before escalating in step 6, after a compaction, and whenever the user says they changed model or effort.

**Exit:** your tier, or "position unknown".

## Step 1 — Decide whether to delegate

**Entry:** a request from the user, or a piece of work you are about to begin.

Delegation is not free, and most of its cost lands on you: spawning, waiting and reading a result add turns to your thread, and each re-sends your context. It pays only when the work moved to the worker is large enough, or cheap enough there, to cover those turns.

First shrink the work. A grep, a query or a script that returns a short answer beats both reading the material yourself and delegating it: it collapses the material without a spawn. A repeatable check against a known app or site is a script run by tier 1, never a model driving a GUI. Delegate only what survives that: material you would otherwise have to read and judge yourself.

Delegate when all three hold:

- **Substance.** Enough material that reading and judging it would load your context heavily, *or* material that would otherwise stay in your context for the rest of the chat (a plan and the files it touches, a corpus, a long log). A task you could finish in a handful of calls is cheaper kept, however well it fits a row.
- **Boundary.** You can state the outcome, the context needed, and evidence you can check without redoing the work.
- **Honesty.** The work exists because the user asked for it. Never invent or inflate work to justify a spawn.

Keep for yourself: goals and acceptance criteria, anything that takes one lookup or one edit, sequencing a handful of steps inside a settled goal, integration of what workers return, the conversation with the user, and the final answer.

**Exit:** either you do the work yourself and the procedure ends here, or you carry a delegable request to step 2.

## Step 2 — Split the request

**Entry:** a delegable request.

Split it so that each part matches exactly one role in step 3. Find every occurrence and then fix it is two parts: retrieval, then implementation. Work out why something fails and then repair it is a diagnosis, then a change. Decompose a plan and then build it is decomposition, then one part per task.

Mark each part **dependent** (it needs another part's artifact) or **independent**. Independent parts form a **stage** and are dispatched together; stages run in sequence, each passing its artifacts forward. Do not split below a coherent unit of work: one worker per file pays a spawn floor per file for nothing.

A part is a **build part** only if you can write all three of: the exact files to change by absolute path, the invariants or interfaces that must not move, and the command whose output proves acceptance. If you cannot name the files, the part is about deciding *what* changes; if you cannot name the invariants, its failure would be silent; if you cannot name the acceptance command, its failure would be undetectable. Each of those sends it to tier 3 instead. If writing the brief would cost you more than doing the work, the brief is the wrong move.

**Exit:** an ordered list of stages, each a set of parts, each part matching one role.

## Step 3 — Route each part

**Entry:** one part of the request.

Apply the two overrides first. They fire whatever the task type, however clear the choice looks, and regardless of the tier comparison below.

**Consequence override** → `te_advise` (recommendation only). The part commits to any of:

- a data or schema migration that rewrites or discards existing data
- a change to an interface, contract or output format that other code depends on
- an authentication, authorization or secret-handling boundary
- a dependency, framework, storage or architecture choice that later work is built on
- deleting or overwriting anything not recoverable from version control
- anything the user has called risky, expensive or hard to undo

**Capability override** → `te_drive` or `te_operate` (execution, ownership as briefed). The part's bottleneck is the model's training profile, not reasoning depth:

- driving a GUI, a browser or a computer-use tool where no script exists: a bounded scenario in a known application or site → `te_drive`; free-form automation of unfamiliar websites or tools → `te_operate`
- spatial work: 3D modelling, CAD, game engines, PCB layout and their scripting → `te_operate`
- extraction or analysis of dense documents (legal, financial, compliance) where a fabricated value is costly → `te_operate`, read-only
- production, SRE or security diagnostics → `te_operate`
- a deliverable that *is* judgment — the shape of a public interface, wording, doctrine, a threshold — what a governing skill rates *Authored* → `te_operate`

Otherwise route by task type. Model and effort are pinned in each role; choose the role, never the model.

| Tier | Role | Runs as | Authority | The part is |
| --- | --- | --- | --- | --- |
| 1 | `te_retrieve` | luna low | mechanical edits | literal retrieval, inventories, extraction, mechanical edits, prescribed and scripted checks, diff-against-brief boundary checks |
| 2 | `te_trace` | luna xhigh | read-only | following a path through code, reconciling a few sources, classifying with edge cases |
| 2 | `te_build` | luna max | edits | a build part (files, invariants and acceptance command all stated); tests from an explicit contract |
| 3 | `te_engineer` | sol medium | edits | implementation whose file set or invariants must be worked out; an ambiguous failure, diagnosed then fixed within ownership; tests inferred from behaviour |
| 3 | `te_review` | sol medium | read-only, runs checks | judgment review of a change: correctness beyond the tests, invariants held, deviations recorded, tests able to fail |
| 3 | `te_plan` | sol high | tracker and task files | decomposition of a plan into tasks, acceptance and verification design, review of whether a stage's outcome holds |
| — | `te_drive` | astra medium | as briefed | capability override: bounded GUI or browser scenario |
| — | `te_operate` | astra high | as briefed | capability override: the other domains above, and *Authored* deliverables |
| — | `te_advise` | astra xhigh | none | consequence override; architecture, strategy, stack; conflicting evidence where a wrong call is expensive |

Route on the cost of a wrong answer and the ease of verification, not on file count or the word the user used. Where two rows fit, take the cheaper and let step 6 move it up. The distinctions that carry most cases: locating something by matching text is `te_retrieve`, by reasoning about behaviour `te_trace`; a failure with a known cause is `te_build`, with competing explanations `te_engineer`; a part that passes the build test is `te_build`, one that fails it `te_engineer`; sequencing a few steps is yours, decomposing a plan into verifiable tasks is `te_plan`, choosing the goal, the architecture or the stack is `te_advise`.

`te_advise` returns a recommendation and holds no edit authority. Evaluate it rather than obey it, then apply it yourself or send the execution back through this step as a new part.

**Reviews.** Three checks follow an implementation and only one is a review. The *acceptance check* (step 5) is yours. *Mechanical checks* — files touched that the brief did not name, lint, tests — are `te_retrieve`. A *judgment review* goes to `te_review` when the diff crosses modules, when the acceptance command does not cover what the change could break, when a governing skill requires one, or when the user asks. A build part that touched only its named files and passed its acceptance command has been checked. A review never runs in the thread that wrote the code, and preferably not on the same model; with a tier 2 implementer that is automatic, with a tier 3 implementer use a fresh thread, and where independence matters more than cost the Astra rows are level with tier 3 in judgment.

Now compare the role's tier with your own from step 0:

- **Lower than you.** Delegate. This is the case that saves.
- **Your own tier.** Do the part yourself: a spawn that lands where you already are buys nothing and costs your extra turns. The one exception is the context reason in step 1 — material that would otherwise sit in your context for the rest of the chat is worth moving to a worker at your own tier.
- **Higher than you.** An escalation, not a saving. Take it when an override fired or a step 6 trigger applies; otherwise do the part yourself.

If your position is unknown, ignore the comparison and route by the table alone. If you are Astra, every row is cheaper and there is nothing to escalate to.

**Exit:** each part has exactly one role, or is yours to do.

## Step 4 — Dispatch

**Entry:** a stage of routed parts.

**Reuse before spawning.** If a worker of the right role already holds the material and the new part is *derivative* of what it read — answerable from it without new reading — send it `followup_task`, which works after it has finished, and open with "answer from what you already read; do not re-read unless something is missing". If the part needs material no worker has seen, spawn fresh: a reused worker re-reads anyway and drags its history along. Treat a worker as cold after a long gap or after your own compaction. Judge derivativeness from what the previous brief asked them to read and from their reply.

**Spawn a stage together.** Dispatch every independent part of the stage in one turn and wait on them with as few turns as `wait_agent` allows; that amortizes your turns across the workers. Give each a disjoint write set in its brief. The runtime caps concurrent workers (`[agents] max_concurrent_threads_per_session`) and overflow is an error, not a queue: hold the remainder for the next batch. When a governing skill requires serial dispatch or lane separation, obey it; the skill sets the order, this procedure only picks the roles.

```text
spawn_agent(
  agent_type = "<role>",
  task_name  = "<role>_<model>_<effort>",
  fork_turns = "none",
  message    = "<self-contained brief>",
)
```

`task_name` exposes the routing in the UI, as in `build_luna_max`. Name the role and its pin, never the assignment, so the label survives reuse; add a digit if a second thread shares a role. Only lowercase letters, digits and underscores are accepted. Do not pass `model` or `reasoning_effort` alongside a role: the pin owns them.

Keep `fork_turns="none"`. A forked thread starts cold and inherits everything you have read at the worker's rate; a brief reads only what the part needs, and a cheap worker given a full history answers from what is salient instead of searching. The one exception is a `te_advise` consult whose decisive evidence is your own reasoning path and cannot be distilled without losing it; prefer a bounded fork over a full one if the runtime offers it.

Brief with only the fields that apply, always with absolute paths — all agents share one container, filesystem and working directory:

```text
Outcome: <bounded goal>
Context: <decisions, absolute paths and facts the worker cannot infer>
Ownership: <files and resources; read-only or authorized edits; write set disjoint from siblings>
Acceptance: <observable result; the checks or citations that prove it>
Boundaries: <excluded work, dependencies, when to stop and report>
Return: result, artifact paths, verification, unresolved risks.
```

**Nesting.** A worker may spawn when its part genuinely splits further, using this same table with one restriction: down to tier 1 or sideways to its own tier, never up; escalation is your decision. Nested spawns share the concurrency pool with your stage, so a worker that hits the cap does the part in-thread rather than wait on a slot you may hold.

**Exit:** the stage is running. Wait on `wait_agent`.

## Step 5 — Check what came back

**Entry:** a worker's reply.

Check the acceptance you set in step 4 with the cheapest sufficient evidence — a diff, a test result, a cited line — rather than repeating the investigation. Scale the check to the consequence and to how uncertain the result is, not to how cheap the worker was. Stop once acceptance passes.

Two checks are never optional. Where the part was to produce or change an artifact, confirm it exists at the stated path: a description of a file is not the file, and a blocked write is a failure to report, not a detail to omit. Where the deliverable is a test, a script or a check, acceptance needs its actual output, because a file that exists is not evidence that it runs.

**Exit:** the part is accepted and you return to step 2 for the next stage, or it failed and you go to step 6.

## Step 6 — Re-route

**Entry:** a part that failed its check, or a problem you are still holding.

Diagnose before spending more. A deficient brief, missing access or evidence, an execution error and a reasoning gap each need a different response, and deeper reasoning never fixes missing permissions or unavailable data. Re-brief once at the same role when that would plausibly work; never re-brief a tier 2 builder twice on the same problem.

Move the part to a stronger row when any of these holds, whatever your reading of the problem. Judging your own limits requires knowing what you do not know, so these are keyed to the record, not to how confident you feel:

- two attempts at the same problem have failed
- a fix was applied and the symptom returned, or it had to be reverted
- two pieces of evidence still conflict after one attempt to reconcile them
- a worker's conclusion contradicts yours and no available evidence settles it
- the change set has outgrown the scope you described to the user
- the user has restated or corrected the same point twice
- you are inventing a pattern that has no precedent in this codebase

Go to the row the remaining problem needs rather than stepping through the rows between. Never re-route for volume, duration, tedium, reassurance, a decision that is cheap to reverse, or a question `te_retrieve` could settle. The Astra rows carry a large fixed cost per spawn and their reasoning is the expensive part, so send them a distilled problem, never raw material: gather with `te_retrieve` first if the evidence is not in hand. A premium model never pays premium rates for retrieval.

Re-read your position first, because escalation is the one decision that turns on it. If you are already at or above the dearest row the problem needs, a spawn buys only a second opinion at your own rate; the answer is better evidence, a cheaper worker sent to gather it, or a decision from the user.

One re-route per problem. If it is still unresolved, the answer is better evidence or a decision from the user, not more reasoning: stop and ask.

**Exit:** a re-dispatched part, or a question for the user.

## Step 7 — Close

**Entry:** every part accepted, or a stop.

Stop workers made obsolete by a change of plan in the same turn you change it, keeping their findings. Cost never justifies an incomplete result, a fabricated verification, or an omitted material uncertainty. Close with one unified answer: the outcome, the evidence, and the limits.

## Working under a governing skill

When another skill governs the workflow — a Realizer task set (`rn-run`, `rn-review`, `rn-task-planner`) or anything like it — that skill sets the order of work, the separation of lanes, whether lanes persist across tasks, and when a review is required. This procedure only decides which role each lane or step runs as. Specifically:

- **Executor labels are the routing input.** Where a skill rates tasks by what settles them, map *Specified* → `te_build`, *Entangled* → `te_engineer`, *Authored* → `te_operate`. If the user chose labels, use the mapping they gave; suggest labels that are role names so the mapping is exact and no file names a model.
- **Review lanes.** Per-task independent review is `te_review`; the final pass over a whole set, or any review judging whether a stated outcome holds, is `te_plan` on a fresh thread. Never send a review to the lane that implemented.
- **Persistent lanes.** If the skill wants an implementer or reviewer kept alive across tasks, keep it with `followup_task`; that is a quality requirement and it outranks the reuse rule.
- **Decomposition reads the code itself.** `te_plan` must read the surfaces it cuts; what it may push to `te_retrieve` is inventory work, never a substitute for reading.
- **Generic lanes.** When the skill asks for a general-purpose subagent and no role fits, spawn one at the tier the task needs and say in the brief that this procedure does not apply to it.

## Sidesteps

- **Position unknown:** route by the table alone; never escalate on the comparison.
- **Concurrency cap hit:** hold parts for the next batch; a nested worker does its part in-thread.
- **Spawning unavailable:** do the work yourself in the order of step 2, and tell the user delegation was not possible.
- **Main-thread choice**, which only the user can make: the main thread's rate is a fixed cost of the whole chat because its context is re-sent every turn, and max effort on an orchestrating thread is waste. A mid model at medium effort delegates down to Luna, escalates to Astra only through the overrides and triggers, and keeps judgment where it belongs.
