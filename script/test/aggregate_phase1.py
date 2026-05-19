#!/usr/bin/env python3
"""Aggregate Phase 1 results into docs/testing/test_result.md.

Reads k6 summary JSON files in --results-dir and combines them with PASS/FAIL
status flags supplied by phase1_run.sh into a Markdown table that maps 1:1 to
the Phase 1 target rows in docs/testing/testing_plan.md §1.
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import sys
from pathlib import Path


def load_summary(path: Path) -> dict | None:
    if not path.exists():
        return None
    try:
        return json.loads(path.read_text())
    except Exception as exc:                              # noqa: BLE001
        print(f"WARN: failed to parse {path}: {exc}", file=sys.stderr)
        return None


def _metric(summary: dict | None, name: str) -> dict:
    if not summary:
        return {}
    m = summary.get("metrics", {}).get(name) or {}
    # k6 v1.x writes values flat; older versions used a "values" sub-dict.
    return m.get("values", m)


def metric_p95(summary: dict | None, name: str) -> str:
    m = _metric(summary, name)
    p95 = m.get("p(95)") or m.get("p95")
    return "n/a" if p95 is None else f"{p95:.0f}ms"


def metric_avg(summary: dict | None, name: str) -> str:
    m = _metric(summary, name)
    a = m.get("avg")
    return "n/a" if a is None else f"{a:.0f}ms"


def metric_count(summary: dict | None, name: str) -> str:
    m = _metric(summary, name)
    cnt = m.get("count")
    return "n/a" if cnt is None else str(int(cnt))


def check_rate(summary: dict | None, name: str) -> str:
    """Returns the success-rate of a named check: '✓ N / ✗ M'."""
    if not summary:
        return "n/a"
    rc = summary.get("root_group", {}).get("checks", {}) or {}
    # k6 returns checks as a dict keyed by check name (v1.x).
    if isinstance(rc, dict):
        entry = rc.get(name) or {}
    else:
        entry = next((e for e in rc if e.get("name") == name), {})
    if not entry:
        return "n/a"
    passes = entry.get("passes", 0)
    fails = entry.get("fails", 0)
    return f"✓ {passes} / ✗ {fails}"


def parse_status_flags(flags: list[str]) -> dict[str, str]:
    out: dict[str, str] = {}
    for flag in flags:
        if "=" not in flag:
            continue
        k, v = flag.split("=", 1)
        out[k.strip()] = v.strip()
    return out


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--results-dir", required=True)
    ap.add_argument("--log-dir", required=True)
    ap.add_argument("--output", required=True)
    ap.add_argument("--status", action="append", default=[],
                    help="repeated: --status name=PASS|FAIL|SKIPPED|MISSING")
    args = ap.parse_args()

    results_dir = Path(args.results_dir)
    log_dir = Path(args.log_dir)
    status_map = parse_status_flags(args.status)

    happy = load_summary(results_dir / "saga_happy.json")
    fail = load_summary(results_dir / "saga_fail.json")
    race = load_summary(results_dir / "race_inventory.json")

    rows = [
        {
            "id": "§2-Happy",
            "target": "Saga TTC P95 < 2.0s (happy path)",
            "observed": (
                f"P95 = {metric_p95(happy, 'saga_ttc_ms')}, "
                f"avg = {metric_avg(happy, 'saga_ttc_ms')}; "
                f"COMPLETED check {check_rate(happy, 'reached COMPLETED')}"
            ),
            "status": status_map.get("saga_happy", "MISSING"),
            "evidence": "script/k6/results/saga_happy.json",
        },
        {
            "id": "§2-Fail",
            "target": "Saga TTC P95 < 1.5s (payment failure)",
            "observed": (
                f"P95 = {metric_p95(fail, 'saga_ttc_ms')}, "
                f"avg = {metric_avg(fail, 'saga_ttc_ms')}; "
                f"FAILED check {check_rate(fail, 'payment terminal FAILED')}"
            ),
            "status": status_map.get("saga_fail", "MISSING"),
            "evidence": "script/k6/results/saga_fail.json",
        },
        {
            "id": "§2-Comp",
            "target": "Compensation TTC P95 < 2.0s",
            "observed": (
                f"P95 = {metric_p95(fail, 'compensation_ttc_ms')}, "
                f"avg = {metric_avg(fail, 'compensation_ttc_ms')}; "
                f"CANCELLED check {check_rate(fail, 'order terminal CANCELLED')}"
            ),
            "status": status_map.get("saga_fail", "MISSING"),
            "evidence": "script/k6/results/saga_fail.json",
        },
        {
            "id": "§3.B",
            "target": "Race: 1 × 201, 9 × 409, final stock = 0",
            "observed": (
                f"success={metric_count(race, 'order_success')}, "
                f"conflict={metric_count(race, 'order_conflict')}, "
                f"other={metric_count(race, 'order_other')}"
            ),
            "status": status_map.get("race_inventory", "MISSING"),
            "evidence": "script/k6/results/race_inventory.json (DB stock validated by counts)",
        },
        {
            "id": "§7.B",
            "target": "Saga idempotency — 3 replays → 1 payment row",
            "observed": (
                "skipped" if status_map.get("saga_replay") == "SKIPPED"
                else "see go test output"
            ),
            "status": status_map.get("saga_replay", "MISSING"),
            "evidence": str(log_dir / "saga_replay.log"),
        },
        {
            "id": "§7.C",
            "target": "X-Correlation-ID present in all 5 service logs",
            "observed": "see correlation_id.log for per-service ✓/✗",
            "status": status_map.get("correlation_id", "MISSING"),
            "evidence": str(log_dir / "correlation_id.log"),
        },
    ]

    ts = dt.datetime.now().strftime("%Y-%m-%d %H:%M")
    section = [f"## Phase 1 — Functional & Saga Correctness  (run {ts})\n"]
    section.append("| Target ID | Target | Observed | Status | Evidence |")
    section.append("|---|---|---|---|---|")
    for r in rows:
        section.append(
            f"| {r['id']} | {r['target']} | {r['observed']} | **{r['status']}** | `{r['evidence']}` |"
        )
    section.append("")
    new_block = "\n".join(section)

    out_path = Path(args.output)
    existing = out_path.read_text() if out_path.exists() and out_path.stat().st_size > 0 else ""
    if "## Phase 1 — Functional & Saga Correctness" in existing:
        # replace existing Phase 1 block
        before, _, rest = existing.partition("## Phase 1 — Functional & Saga Correctness")
        # everything before the next ##-heading (or end) belongs to this Phase 1 block
        nxt = rest.find("\n## ")
        after = rest[nxt:] if nxt >= 0 else ""
        out_path.write_text(before + new_block + after)
    else:
        header = "# System Test Results\n\n" if not existing.strip() else ""
        out_path.write_text(header + existing.rstrip() + ("\n\n" if existing.strip() else "") + new_block)

    print(f"wrote {out_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
