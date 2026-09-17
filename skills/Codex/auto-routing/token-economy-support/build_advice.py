#!/usr/bin/env python3
"""Assemble the advisory-consult comparison: inherited history vs curated brief.

Both arms spawn the same pinned advisor role; only `fork_turns` and the message
differ. The parent's context carries the corpus plus a reasoning trajectory, so
the inherited arm has the advantage the curated arm cannot reproduce.
"""

import glob
import sys

ROOT = "/tmp/te-measure/corpus"
SERVICES = sorted(glob.glob(f"{ROOT}/services/*.ts"))

TRAJECTORY = """
===== SESSION SO FAR =====

Reported problem: a small number of customers are charged twice for the same
order. It is intermittent, it correlates with periods of elevated latency on the
payments dependency, and it has never been reproduced on demand.

What we have already established this session:

- The order service submits exactly one charge request per order. We read the
  submission path and confirmed there is no duplicate submission, so we ruled
  out a client-side or order-service double submit.
- The duplicate charges carry the same order id but different charge ids, so
  they are not a reporting or ledger display artefact.
- We suspected a stuck database transaction retry, but the duplicates appear in
  the payment provider's own records too, so the duplication happens upstream of
  our persistence layer. That hypothesis is contradicted by the evidence.
- We checked that no service was redeployed or reconfigured in the affected
  window, so this is not a configuration rollout.
- Everything we know is consistent with the same charge being submitted more
  than once to the provider from inside our platform, during latency spikes,
  without the order service submitting twice.

Where we are stuck: we cannot identify the mechanism that turns one submitted
charge into two provider-side charges under latency, and we do not want to
guess, because the fix differs completely depending on the mechanism.
"""

QUESTION = (
    "Given the session material, state the single most likely mechanism for the "
    "duplicate charges, name the specific configuration values that make it "
    "possible, and state the evidence that would confirm or refute it. Advise "
    "only; make no changes."
)

BRIEF = (
    "Advisory consult. You have no prior context; everything you need is stated "
    "here.\\n"
    "Symptom: a small number of customers are charged twice for the same order. "
    "Intermittent, correlated with elevated latency on the payments dependency, "
    "never reproduced on demand. Duplicates share an order id but have distinct "
    "charge ids, and appear in the payment provider's own records.\\n"
    "Already ruled out: duplicate submission by the order service (submission "
    "path read and confirmed single-submit); ledger or reporting artefact; "
    "database transaction retry (duplicates exist provider-side, upstream of our "
    "persistence); configuration rollout (no deploy in the window).\\n"
    "Working conclusion: one submitted charge becomes two provider-side charges "
    "from inside the platform under latency.\\n"
    f"Platform source is at {ROOT}. Service modules are in {ROOT}/services and "
    f"conventions are in {ROOT}/README.md. Read what you need.\\n"
    "Question: state the single most likely mechanism, name the specific "
    "configuration values that make it possible, and state the evidence that "
    "would confirm or refute it. Advise only; make no changes."
)


def corpus_dump() -> str:
    parts = ["# Working material for the current session\n"]
    for path in SERVICES + sorted(glob.glob(f"{ROOT}/ui/*.tsx")):
        parts.append(f"\n===== FILE: {path} =====\n{open(path).read()}")
    parts.append(f"\n===== FILE: {ROOT}/README.md =====\n{open(f'{ROOT}/README.md').read()}")
    return "".join(parts)


def instructions(first: str) -> str:
    inherit = f"""spawn_agent with exactly these arguments:
   agent_type = "tp_advise"
   task_name  = "advice_inherit_sol_max"
   message    = "{QUESTION}"
   Do NOT pass fork_turns. Do NOT pass model. Do NOT pass reasoning_effort."""

    brief = f"""spawn_agent with exactly these arguments:
   agent_type = "tp_advise"
   task_name  = "advice_brief_sol_max"
   fork_turns = "none"
   message    = "{BRIEF}"
   Do NOT pass model. Do NOT pass reasoning_effort."""

    blocks = [inherit, brief] if first == "inherit" else [brief, inherit]

    return f"""

===== INSTRUCTIONS =====

This is an explicit, authorized delegation request. First run `echo warm` so
your context is committed, then do exactly the following and nothing else. Do
not answer the question yourself.

1. Call {blocks[0]}

2. Wait for that agent to finish using wait_agent.

3. Call {blocks[1]}

4. Wait for that agent to finish using wait_agent.

5. Output both replies verbatim, each under a heading naming its task_name.
   Add no analysis of your own.
"""


if __name__ == "__main__":
    first = sys.argv[1]
    assert first in ("inherit", "brief")
    sys.stdout.write(corpus_dump() + TRAJECTORY + instructions(first))
