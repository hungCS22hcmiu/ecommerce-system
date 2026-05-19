#!/usr/bin/env python3
"""Aggregate Phase 4 (testing-debt) results into docs/testing/test_result.md.

Receives PASS/FAIL flags from phase4_run.sh and writes a "Phase 4 — Testing Debt"
section. Coverage % is reported informationally (no enforcement).
"""
from __future__ import annotations

import argparse
import datetime as dt
from pathlib import Path


def parse_kv(flags):
    out = {}
    for f in flags or []:
        if "=" in f:
            k, v = f.split("=", 1)
            out[k.strip()] = v.strip()
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results-dir", required=True)
    ap.add_argument("--log-dir", required=True)
    ap.add_argument("--output", required=True)
    ap.add_argument("--status", action="append", default=[])
    ap.add_argument("--coverage", action="append", default=[])
    args = ap.parse_args()

    status = parse_kv(args.status)
    coverage = parse_kv(args.coverage)

    log_dir = Path(args.log_dir)

    rows = [
        {
            "id": "§9.C-Pkg",
            "target": "user-service/pkg/{blacklist,verification,reset} — 100% on security utils",
            "observed": f"coverage={coverage.get('user_pkg', 'n/a') or 'n/a'}",
            "status": status.get("user_pkg", "MISSING"),
            "evidence": str(log_dir / "phase4_user_pkg.log"),
        },
        {
            "id": "§9.B-CB",
            "target": "cart-service CircuitBreaker — CLOSED→OPEN→HALF_OPEN→CLOSED cycle",
            "observed": f"coverage(circuit_breaker.go)={coverage.get('circuit_breaker', 'n/a') or 'n/a'}",
            "status": status.get("cb_unit", "MISSING"),
            "evidence": str(log_dir / "phase4_cart_cb.log"),
        },
        {
            "id": "§9.A-Idem",
            "target": "PaymentRepository idempotency — N=20 concurrent same-key inserts → 1 row + 19× ErrDuplicate",
            "observed": "N=20" if status.get("idempotency") == "PASS"
                        else "skipped" if status.get("idempotency") == "SKIPPED"
                        else "see log",
            "status": status.get("idempotency", "MISSING"),
            "evidence": str(log_dir / "phase4_idempotency.log"),
        },
        {
            "id": "§9.A-RepoQ",
            "target": "ProductRepository FTS + pgvector edge cases (empty / escape / dim mismatch)",
            "observed": "see Maven output" if status.get("repo_query") != "SKIPPED" else "skipped",
            "status": status.get("repo_query", "MISSING"),
            "evidence": str(log_dir / "phase4_repo_query.log"),
        },
        {
            "id": "§9.B-Embed",
            "target": "EmbeddingClient — 200 / 500 / timeout outcomes via WireMock",
            "observed": "see Maven output" if status.get("embed") != "SKIPPED" else "skipped",
            "status": status.get("embed", "MISSING"),
            "evidence": str(log_dir / "phase4_embed.log"),
        },
    ]

    ts = dt.datetime.now().strftime("%Y-%m-%d %H:%M")
    section = [f"## Phase 4 — Testing Debt  (run {ts})\n",
               "| Target ID | Target | Observed | Status | Evidence |",
               "|---|---|---|---|---|"]
    for r in rows:
        section.append(f"| {r['id']} | {r['target']} | {r['observed']} | **{r['status']}** | `{r['evidence']}` |")
    section.append("")
    new_block = "\n".join(section)

    out_path = Path(args.output)
    existing = out_path.read_text() if out_path.exists() else ""
    if "## Phase 4 — Testing Debt" in existing:
        before, _, rest = existing.partition("## Phase 4 — Testing Debt")
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
