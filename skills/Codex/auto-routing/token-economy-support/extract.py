#!/usr/bin/env python3
"""Extract per-thread token usage for the fork-vs-brief trials.

Note: `thread_token_usage.input_tokens` is the SUM over every API call in the
thread, so a worker making many tool calls re-sends its growing context each
time. Peak single-call input approximates the context size.
"""

import glob
import json
import os
from datetime import datetime, timezone

SESSIONS = os.path.expanduser("~/.codex/sessions/2026/09/10")

with open("/tmp/te-measure/t1_start.txt") as fh:
    T1 = datetime.strptime(fh.read().strip(), "%Y-%m-%dT%H:%M:%SZ").replace(
        tzinfo=timezone.utc
    )

# Published token credit rates per million (uncached input, cached input, output).
RATES = {"luna": (5, 0.5, 30), "terra": (50, 5, 300), "sol": (100, 10, 500)}


def credits(row, model):
    inp, cach, out = RATES[model]
    return (
        (row["input"] - row["cached"]) * inp / 1e6
        + row["cached"] * cach / 1e6
        + row["output"] * out / 1e6
    )


rows = []
for path in sorted(glob.glob(f"{SESSIONS}/*.jsonl"), key=os.path.getmtime):
    mtime = datetime.fromtimestamp(os.path.getmtime(path), timezone.utc)
    if mtime < T1:
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
            model = p.get("model") or model
            effort = p.get("effort") or effort
        elif t == "token_usage_record":
            total = p.get("thread_token_usage")
            calls.append(p.get("usage", {}))
    if not total:
        continue

    if effort == "medium":
        kind = "PARENT"
    elif "===== FILE:" in raw:
        kind = "fork arm"
    else:
        kind = "brief arm"

    rows.append(
        {
            "kind": kind,
            "mtime": mtime,
            "effort": effort,
            "calls": len(calls),
            "peak": max((c.get("input_tokens", 0) for c in calls), default=0),
            "input": total["input_tokens"],
            "cached": total["cached_input_tokens"],
            "output": total["output_tokens"],
            "reasoning": total.get("reasoning_output_tokens", 0),
        }
    )

rows.sort(key=lambda r: r["mtime"])
trial = 0
for i, r in enumerate(rows):
    if r["kind"] == "PARENT":
        trial += 1
    r["trial"] = trial if r["kind"] == "PARENT" else trial or 1

# Parents are written last, so re-derive trials by grouping into pairs of children.
children = [r for r in rows if r["kind"] != "PARENT"]
for i, r in enumerate(children):
    r["trial"] = 1 if i < 2 else 2

hdr = (f"{'trial':<7}{'arm':<12}{'calls':>7}{'peak ctx':>10}{'cum input':>11}"
       f"{'cached':>10}{'%cache':>8}{'uncached':>10}{'output':>8}{'reason':>8}")
print(hdr)
print("-" * len(hdr))
for r in children:
    pct = 100 * r["cached"] / r["input"] if r["input"] else 0
    print(f"{r['trial']:<7}{r['kind']:<12}{r['calls']:>7}{r['peak']:>10,}"
          f"{r['input']:>11,}{r['cached']:>10,}{pct:>7.1f}%"
          f"{r['input'] - r['cached']:>10,}{r['output']:>8,}{r['reasoning']:>8,}")

print("\nParent threads (orchestration overhead, luna/medium):")
for r in [x for x in rows if x["kind"] == "PARENT"]:
    pct = 100 * r["cached"] / r["input"] if r["input"] else 0
    print(f"  calls={r['calls']:>2} cum_input={r['input']:>9,} cached={pct:>5.1f}% "
          f"output={r['output']:>5,}")

print("\nWorker cost per arm, if the SAME work ran on each model (credits):")
print(f"{'trial':<7}{'arm':<12}{'luna':>11}{'terra':>11}{'sol':>11}")
print("-" * 52)
for r in children:
    print(f"{r['trial']:<7}{r['kind']:<12}"
          + "".join(f"{credits(r, m):>11.4f}" for m in ("luna", "terra", "sol")))

print("\nRatio brief/fork per trial (same model cancels out):")
for t in (1, 2):
    pair = {r["kind"]: r for r in children if r["trial"] == t}
    if len(pair) == 2:
        for m in ("luna", "terra", "sol"):
            f, b = credits(pair["fork arm"], m), credits(pair["brief arm"], m)
            print(f"  trial {t} {m:<6} fork={f:.4f} brief={b:.4f} -> brief is "
                  f"{b / f:.2f}x fork")
