#!/usr/bin/env python3
"""Lateral delegation vs doing the work in-thread, at the SAME model and effort.

Both arms use a luna/low parent carrying ~90k tokens of unrelated context (UI
modules), and both must inventory the same 12 service files that live on disk.

  in_thread: the parent reads the twelve files itself.
  lateral:   the parent briefs te_retrieve, which is also pinned luna/low.

The question is whether a spawn that lands on the model already running the
chat is waste. Cost is summed over every thread each arm creates.
"""

import glob
import json
import os
import subprocess
import sys
from datetime import datetime, timezone

ROOT = "/tmp/te-measure/corpus"
SERVICES = sorted(glob.glob(f"{ROOT}/services/*.ts"))
SESSIONS = os.path.expanduser("~/.codex/sessions/2026/09/10")
RATES = {"luna": (5, 0.5, 30), "terra": (50, 5, 300), "sol": (100, 10, 500)}


def filler() -> str:
    parts = ["# Unrelated working material already in this session\n"]
    for p in sorted(glob.glob(f"{ROOT}/ui/*.tsx")):
        parts.append(f"\n===== FILE: {p} =====\n{open(p).read()}")
    return "".join(parts)


TASK = (
    "Task: report the retryAttempts value declared in each of these twelve "
    "service files, as service=value.\n" + "\n".join(SERVICES)
)

ARMS = {
    "in_thread": filler() + "\n\n" + TASK + (
        "\n\nDo this work YOURSELF in this thread. Do not spawn any sub-agent."
    ),
    "lateral": filler() + "\n\n" + TASK + (
        "\n\nExplicit authorization to delegate. Do NOT read the files yourself. "
        'spawn_agent with agent_type="te_retrieve", task_name="inv_luna_low", '
        'fork_turns="none", and a self-contained brief that lists the twelve '
        "absolute paths above and asks for service=value plus file citations. "
        "Then wait_agent and report what it returned."
    ),
}


def threads_since(t0):
    out = []
    for p in sorted(glob.glob(f"{SESSIONS}/*.jsonl"), key=os.path.getmtime):
        if datetime.fromtimestamp(os.path.getmtime(p), timezone.utc) < t0:
            continue
        model = effort = None
        first_in = peak_in = 0
        agg = None
        for line in open(p, errors="replace"):
            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue
            t, pl = rec.get("type"), rec.get("payload", {})
            if t == "turn_context":
                model = pl.get("model") or model
                effort = pl.get("effort") or effort
            elif t == "token_usage_record":
                last = pl.get("last_token_usage", {})
                if last.get("input_tokens"):
                    if not first_in:
                        first_in = last["input_tokens"]
                    peak_in = max(peak_in, last["input_tokens"])
                agg = pl.get("thread_token_usage", agg)
        if model and agg:
            out.append((model, effort, first_in, peak_in, agg))
    return out


def credits(u, model_key):
    up, ca, op = RATES[model_key]
    inp, cached, out = u["input_tokens"], u["cached_input_tokens"], u["output_tokens"]
    return (inp - cached) * up / 1e6 + cached * ca / 1e6 + out * op / 1e6


def run(arm, order):
    t0 = datetime.now(timezone.utc).replace(microsecond=0)
    subprocess.run(
        ["codex", "exec", "--enable", "multi_agent_v2", "-m", "gpt-5.6-luna",
         "-c", "model_reasoning_effort=low", "-s", "workspace-write",
         "--skip-git-repo-check", ARMS[arm]],
        capture_output=True, text=True, timeout=900,
    )
    ths = threads_since(t0)
    tot_luna = sum(credits(u, "luna") for *_, u in ths)
    tot_sol = sum(credits(u, "sol") for *_, u in ths)
    peak = max((pk for *_, pk, _ in ths), default=0)
    print(f"{order:<3}{arm:<11}{len(ths):>3} thread(s)"
          f"  parent_peak_ctx={peak:>7,}"
          f"  luna={tot_luna:>7.3f}  same-task-at-sol={tot_sol:>7.2f}")
    for m, e, fi, pk, u in ths:
        print(f"      {m}/{e:<8} first_call={fi:>7,} peak={pk:>7,} "
              f"in={u['input_tokens']:>8,} cached={u['cached_input_tokens']:>8,} "
              f"out={u['output_tokens']:>6,}")
    return tot_luna


if __name__ == "__main__":
    seq = [("T1", "in_thread"), ("T1", "lateral"),
           ("T2", "lateral"), ("T2", "in_thread")]
    res = {}
    for order, arm in seq:
        res.setdefault(arm, []).append(run(arm, order))
        sys.stdout.flush()
    a, b = res["in_thread"], res["lateral"]
    print(f"\nin_thread: {a}\nlateral:   {b}")
    print(f"lateral / in_thread ratio per trial: "
          f"{[round(y / x, 2) for x, y in zip(a, b)]}")
