#!/usr/bin/env python3
"""Aggregate Phase 5 (frontend) results into docs/testing/test_result.md.

Parses Vitest JSON reporter output + Playwright JSON reporter and writes a
"Phase 5 — Frontend UX & Reliability" section.
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import sys
from pathlib import Path


def parse_kv(flags):
    out = {}
    for f in flags or []:
        if "=" in f:
            k, v = f.split("=", 1)
            out[k.strip()] = v.strip()
    return out


def load_json(path: Path):
    if not path.exists():
        return None
    try:
        return json.loads(path.read_text())
    except Exception as exc:
        print(f"WARN: failed to parse {path}: {exc}", file=sys.stderr)
        return None


def vitest_summary(summary):
    """Returns (numTotal, numPassed, numFailed) for a vitest JSON report."""
    if not summary:
        return None
    # Vitest 1.x+ shape: { numTotalTests, numPassedTests, numFailedTests, testResults: [...] }
    total = summary.get("numTotalTests")
    passed = summary.get("numPassedTests")
    failed = summary.get("numFailedTests")
    if total is None and "testResults" in summary:
        # Fall back to per-file aggregation
        total = 0
        passed = 0
        failed = 0
        for tr in summary.get("testResults", []) or []:
            ar = tr.get("assertionResults", []) or []
            total += len(ar)
            for a in ar:
                s = a.get("status")
                if s == "passed":
                    passed += 1
                elif s == "failed":
                    failed += 1
    return total, passed, failed


def playwright_summary(summary):
    """Returns (passed, failed, skipped) from Playwright json reporter."""
    if not summary:
        return None
    stats = summary.get("stats", {}) or {}
    return (
        stats.get("expected", 0),
        stats.get("unexpected", 0),
        stats.get("skipped", 0),
    )


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results-dir", required=True)
    ap.add_argument("--log-dir", required=True)
    ap.add_argument("--output", required=True)
    ap.add_argument("--status", action="append", default=[])
    args = ap.parse_args()

    status = parse_kv(args.status)
    rd = Path(args.results_dir)
    log_dir = Path(args.log_dir)

    rows = []

    # Vitest
    vit = load_json(rd / "vitest.json")
    vs = vitest_summary(vit)
    if vs:
        total, passed, failed = vs
        rows.append({
            "id": "§8.A-C-Vitest",
            "target": "Vitest unit: toast surfacing + optimistic cart + axios 401 queue",
            "observed": f"total={total}, passed={passed}, failed={failed}",
            "status": status.get("vitest", "MISSING"),
            "evidence": str(log_dir / "phase5_vitest.log"),
        })
    else:
        rows.append({
            "id": "§8.A-C-Vitest",
            "target": "Vitest unit: toast surfacing + optimistic cart + axios 401 queue",
            "observed": "n/a (no JSON report)",
            "status": status.get("vitest", "MISSING"),
            "evidence": str(log_dir / "phase5_vitest.log"),
        })

    # Playwright responsive
    resp = load_json(rd / "playwright_responsive.json")
    ps = playwright_summary(resp)
    if ps:
        rows.append({
            "id": "§8.D-Resp",
            "target": "Responsive 320px — no horizontal overflow on Home / Products / Cart",
            "observed": f"passed={ps[0]}, failed={ps[1]}, skipped={ps[2]}",
            "status": status.get("responsive", "MISSING"),
            "evidence": str(log_dir / "phase5_responsive.log"),
        })
    elif status.get("responsive") == "SKIPPED":
        rows.append({
            "id": "§8.D-Resp",
            "target": "Responsive 320px — no horizontal overflow on Home / Products / Cart",
            "observed": "skipped (frontend not reachable or SKIP_PLAYWRIGHT=1)",
            "status": "SKIPPED",
            "evidence": str(log_dir / "phase5_responsive.log"),
        })

    # Playwright a11y
    a11y = load_json(rd / "playwright_a11y.json")
    ps = playwright_summary(a11y)
    if ps:
        rows.append({
            "id": "§8.D-A11y",
            "target": "axe-core scan — zero serious / critical violations on Home / Products",
            "observed": f"passed={ps[0]}, failed={ps[1]}, skipped={ps[2]}",
            "status": status.get("a11y", "MISSING"),
            "evidence": str(log_dir / "phase5_a11y.log"),
        })
    elif status.get("a11y") == "SKIPPED":
        rows.append({
            "id": "§8.D-A11y",
            "target": "axe-core scan — zero serious / critical violations on Home / Products",
            "observed": "skipped (frontend not reachable or SKIP_PLAYWRIGHT=1)",
            "status": "SKIPPED",
            "evidence": str(log_dir / "phase5_a11y.log"),
        })

    ts = dt.datetime.now().strftime("%Y-%m-%d %H:%M")
    section = [f"## Phase 5 — Frontend UX & Reliability  (run {ts})\n",
               "| Target ID | Target | Observed | Status | Evidence |",
               "|---|---|---|---|---|"]
    for r in rows:
        section.append(f"| {r['id']} | {r['target']} | {r['observed']} | **{r['status']}** | `{r['evidence']}` |")
    section.append("")
    new_block = "\n".join(section)

    out_path = Path(args.output)
    existing = out_path.read_text() if out_path.exists() else ""
    if "## Phase 5 — Frontend UX" in existing:
        before, _, rest = existing.partition("## Phase 5 — Frontend UX")
        nxt = rest.find("\n## ")
        after = rest[nxt:] if nxt >= 0 else ""
        out_path.write_text(before + new_block + after)
    else:
        header = "# System Test Results\n\n" if not existing.strip() else ""
        out_path.write_text(header + existing.rstrip() + ("\n\n" if existing.strip() else "") + new_block)

    print(f"wrote {out_path} ({len(rows)} rows)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
