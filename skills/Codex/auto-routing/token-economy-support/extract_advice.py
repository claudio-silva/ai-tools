#!/usr/bin/env python3
"""Per-thread usage for the advisory consult comparison (inherit vs brief)."""

import glob
import json
import os
from datetime import datetime, timezone

SESSIONS = os.path.expanduser("~/.codex/sessions/2026/09/10")

with open("/tmp/te-measure/adv_start.txt") as fh:
    T0 = datetime.strptime(fh.read().strip(), "%Y-%m-%dT%H:%M:%SZ").replace(
        tzinfo=timezone.utc
    )

RATES = {"luna": (5, 0.5, 30), "sol": (100, 10, 500), "astra": (250, 25, 1250)}


def credits(row, model):
    inp, cach, out = RATES[model]
    return (
        (row["input"] - row["cached"]) * inp / 1e6
        + row["cached"] * cach / 1e6
        + row["output"] * out / 1e6
    )


rows = []
for path in sorted(glob.glob(f"{SESSIONS}/*.jsonl"), key=os.path.getmtime):
    if datetime.fromtimestamp(os.path.getmtime(path), timezone.utc) < T0:
        continue
    raw = open(path).read()
    model = effort = None
    total = None
    calls = []
    for line in raw.splitlines():
        try:
            rec = json.loads(line)
        except json.JSONDecodeError:
            continue
        t, p = rec.get("type"), rec.get("payload", {})
        if t == "turn_context":
            model, effort = p.get("model") or model, p.get("effort") or effort
        elif t == "token_usage_record":
            total = p.get("thread_token_usage")
            calls.append(p.get("usage", {}))
    if not total or model != "gpt-5.6-sol":
        continue
    rows.append(
        {
            # The inherited arm opens with the whole forked context; the briefed
            # arm opens at the bare per-spawn baseline.
            "arm": "inherit" if calls[0].get("input_tokens", 0) > 50_000 else "brief",
            "calls": len(calls),
            "first_cached": calls[0].get("cached_input_tokens", 0) if calls else 0,
            "first_input": calls[0].get("input_tokens", 0) if calls else 0,
            "peak": max((c.get("input_tokens", 0) for c in calls), default=0),
            "input": total["input_tokens"],
            "cached": total["cached_input_tokens"],
            "output": total["output_tokens"],
            "reasoning": total.get("reasoning_output_tokens", 0),
        }
    )

for i, r in enumerate(rows):
    r["trial"] = 1 if i < 2 else 2

hdr = (f"{'trial':<7}{'arm':<10}{'calls':>7}{'1st ctx':>9}{'1st cache':>11}"
       f"{'peak':>9}{'cum input':>11}{'uncached':>10}{'output':>8}{'reason':>8}")
print(hdr)
print("-" * len(hdr))
for r in rows:
    fc = f"{100 * r['first_cached'] / r['first_input']:.0f}%" if r["first_input"] else "-"
    print(f"{r['trial']:<7}{r['arm']:<10}{r['calls']:>7}{r['first_input']:>9,}{fc:>11}"
          f"{r['peak']:>9,}{r['input']:>11,}{r['input'] - r['cached']:>10,}"
          f"{r['output']:>8,}{r['reasoning']:>8,}")

print("\nSame token profile priced as the advisor's model (credits):")
print(f"{'trial':<7}{'arm':<10}{'luna':>10}{'sol':>10}{'astra':>10}")
print("-" * 47)
for r in rows:
    print(f"{r['trial']:<7}{r['arm']:<10}"
          + "".join(f"{credits(r, m):>10.2f}" for m in ("luna", "sol", "astra")))

print("\nBrief vs inherit, per trial:")
for t in (1, 2):
    pair = {r["arm"]: r for r in rows if r["trial"] == t}
    if len(pair) == 2:
        for m in ("sol", "astra"):
            i, b = credits(pair["inherit"], m), credits(pair["brief"], m)
            print(f"  trial {t} on {m:<6} inherit={i:6.2f}  brief={b:6.2f}  "
                  f"brief is {b / i:.2f}x inherit")
