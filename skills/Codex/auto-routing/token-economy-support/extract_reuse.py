#!/usr/bin/env python3
"""Per-turn usage for the reuse probe: followup on an existing worker vs a fresh spawn.

`turn_token_usage` accumulates within a turn, so the last record of each
turn_id is that turn's total. A reused worker has two turns; a fresh worker one.
"""

import glob
import json
import os
from datetime import datetime, timezone

SESSIONS = os.path.expanduser("~/.codex/sessions/2026/09/10")

with open("/tmp/te-measure/reuse_start.txt") as fh:
    T0 = datetime.strptime(fh.read().strip(), "%Y-%m-%dT%H:%M:%SZ").replace(
        tzinfo=timezone.utc
    )

RATES = {"luna": (5, 0.5, 30), "sol": (100, 10, 500), "astra": (250, 25, 1250)}


def credits(inp, cached, out, model):
    u, c, o = RATES[model]
    return (inp - cached) * u / 1e6 + cached * c / 1e6 + out * o / 1e6


threads = []
for path in sorted(glob.glob(f"{SESSIONS}/*.jsonl"), key=os.path.getmtime):
    if datetime.fromtimestamp(os.path.getmtime(path), timezone.utc) < T0:
        continue
    model = effort = None
    turns = {}
    order = []
    calls_per_turn = {}
    for line in open(path):
        try:
            rec = json.loads(line)
        except json.JSONDecodeError:
            continue
        t, p = rec.get("type"), rec.get("payload", {})
        if t == "turn_context":
            model, effort = p.get("model") or model, p.get("effort") or effort
        elif t == "token_usage_record":
            tid = p.get("turn_id")
            if tid not in turns:
                order.append(tid)
                calls_per_turn[tid] = 0
            turns[tid] = p.get("turn_token_usage", {})
            calls_per_turn[tid] += 1
    if not turns or effort != "low":
        continue
    threads.append(
        {"file": os.path.basename(path), "model": model, "effort": effort,
         "order": order, "turns": turns, "calls": calls_per_turn}
    )

hdr = f"{'thread':<10}{'turn':<7}{'calls':>7}{'input':>10}{'cached':>10}{'%cache':>8}{'uncached':>10}{'output':>8}{'luna cr':>9}"
print(hdr)
print("-" * len(hdr))
label = {}
for i, th in enumerate(threads):
    name = f"W{i + 1}"
    label[name] = th
    for j, tid in enumerate(th["order"]):
        u = th["turns"][tid]
        inp, cached, out = u["input_tokens"], u["cached_input_tokens"], u["output_tokens"]
        pct = 100 * cached / inp if inp else 0
        print(f"{name:<10}{j + 1:<7}{th['calls'][tid]:>7}{inp:>10,}{cached:>10,}"
              f"{pct:>7.1f}%{inp - cached:>10,}{out:>8,}"
              f"{credits(inp, cached, out, 'luna'):>9.3f}")

print("\nMarginal cost of the SECOND task, priced at each model's rates:")
print(f"{'route':<34}{'luna':>10}{'sol':>10}{'astra':>10}")
print("-" * 64)
reused = next((t for t in threads if len(t["order"]) > 1), None)
fresh = next((t for t in threads if len(t["order"]) == 1), None)
if reused and fresh:
    ru = reused["turns"][reused["order"][1]]
    fu = fresh["turns"][fresh["order"][0]]
    for tag, u in (("followup on existing worker", ru), ("fresh spawn, self-contained", fu)):
        row = f"{tag:<34}"
        for m in ("luna", "sol", "astra"):
            row += f"{credits(u['input_tokens'], u['cached_input_tokens'], u['output_tokens'], m):>10.3f}"
        print(row)
    for m in ("luna", "sol", "astra"):
        r = credits(ru["input_tokens"], ru["cached_input_tokens"], ru["output_tokens"], m)
        f = credits(fu["input_tokens"], fu["cached_input_tokens"], fu["output_tokens"], m)
        print(f"  {m:<6} followup is {r / f:.2f}x a fresh spawn")
