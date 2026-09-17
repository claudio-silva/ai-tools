#!/usr/bin/env python3
"""Assemble parent prompts for the fork-vs-brief measurement.

The parent's context is loaded with the whole corpus (services + irrelevant UI
noise) so that a full-history fork carries far more than a targeted brief would.
Both arms spawn the same pinned role; only `fork_turns` differs.
"""

import glob
import sys

ROOT = "/tmp/te-measure/corpus"

FORK_MSG = (
    "Audit assignment. The platform material is already present in your "
    "context. Identify every service whose retryAttempts deviates from the "
    "documented platform default. Then report in your required format."
)

SERVICE_PATHS = sorted(glob.glob(f"{ROOT}/services/*.ts"))

BRIEF_MSG = (
    "Audit assignment. You have no prior context; everything you need is here.\\n"
    "Goal: identify every service whose retryAttempts deviates from the "
    "documented platform default.\\n"
    f"Documented default is stated in {ROOT}/README.md.\\n"
    "Service modules to inspect (absolute paths):\\n"
    + "\\n".join(SERVICE_PATHS)
    + "\\nRead those files yourself, then report in your required format."
)


def corpus_dump() -> str:
    parts = ["# Working material for the current session\n"]
    for path in SERVICE_PATHS + sorted(glob.glob(f"{ROOT}/ui/*.tsx")):
        with open(path) as fh:
            parts.append(f"\n===== FILE: {path} =====\n{fh.read()}")
    with open(f"{ROOT}/README.md") as fh:
        parts.append(f"\n===== FILE: {ROOT}/README.md =====\n{fh.read()}")
    return "".join(parts)


def instructions(first: str) -> str:
    fork_block = f"""spawn_agent with exactly these arguments:
   agent_type = "te_measure"
   task_name  = "audit_fork_luna_low"
   message    = "{FORK_MSG}"
   Do NOT pass fork_turns. Do NOT pass model. Do NOT pass reasoning_effort."""

    brief_block = f"""spawn_agent with exactly these arguments:
   agent_type = "te_measure"
   task_name  = "audit_brief_luna_low"
   fork_turns = "none"
   message    = "{BRIEF_MSG}"
   Do NOT pass model. Do NOT pass reasoning_effort."""

    blocks = [fork_block, brief_block] if first == "fork" else [brief_block, fork_block]

    return f"""

===== INSTRUCTIONS =====

This is an explicit, authorized delegation request. Do exactly the following
steps in order and nothing else. Do not perform the audit yourself.

1. Call {blocks[0]}

2. Wait for that agent to finish using wait_agent.

3. Call {blocks[1]}

4. Wait for that agent to finish using wait_agent.

5. Output both replies verbatim, each under a heading naming its task_name.
   Add no analysis of your own.
"""


if __name__ == "__main__":
    first = sys.argv[1]
    assert first in ("fork", "brief")
    sys.stdout.write(corpus_dump() + instructions(first))
