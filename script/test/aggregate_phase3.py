#!/usr/bin/env python3
"""Aggregate Phase 3 results into docs/testing/test_result.md.

Reads chaos script JSON outputs + k6 summary JSONs, appends a
"Phase 3 — Resilience & Chaos" section to test_result.md.
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import sys
from pathlib import Path


def load_json(path: Path):
    if not path.exists(): return None
    try: return json.loads(path.read_text())
    except Exception as exc:
        print(f"WARN: failed to parse {path}: {exc}", file=sys.stderr)
        return None


def _metric(summary, name):
    if not summary: return {}
    m = summary.get("metrics", {}).get(name) or {}
    return m.get("values", m)


def k6_p95(summary, name):
    m = _metric(summary, name)
    v = m.get("p(95)") or m.get("p95")
    return None if v is None else float(v)


def k6_count(summary, name):
    m = _metric(summary, name)
    v = m.get("count")
    return None if v is None else int(v)


def fmt_ms(v):  return f"{v:.0f}ms" if v is not None else "n/a"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results-dir", default="script/k6/results")
    ap.add_argument("--output", default="docs/testing/test_result.md")
    args = ap.parse_args()

    rd = Path(args.results_dir)
    rows = []

    # §7.A — CB OPEN after 5 failures.
    # Correct pass criterion: the LAST 5 of 10 responses are 503 (CB OPEN fast-fail).
    # The first 5 may be slow failures (HTTP timeout codes 000/504) while the CB counts
    # up to its 5-failure threshold; that's also acceptable behavior.
    cb = load_json(rd / "chaos_cb_cart.json")
    if cb is not None:
        n503 = cb.get("post_503_count", 0)
        other = cb.get("post_other_count", 0)
        statuses = cb.get("post_statuses", "").strip().split()
        last5 = statuses[-5:] if len(statuses) >= 5 else statuses
        last5_all_503 = len(last5) == 5 and all(s == "503" for s in last5)
        rows.append({
            "id": "§7.A-CB",
            "target": "Cart CB OPEN after 5 failures (last 5 of 10 POST = 503)",
            "observed": f"503={n503}, other={other}, statuses=`{' '.join(statuses)}`",
            "status": "PASS" if last5_all_503 else "FAIL",
            "evidence": "script/k6/results/chaos_cb_cart.json",
        })

    # §7.A — Degraded GET /cart < 20ms.
    # In k6 v1.x, summary JSON sets metric.thresholds[expr] = True when the threshold
    # was BREACHED (test failed), False when it held. We want all entries to be False.
    deg = load_json(rd / "cart_get_degraded.json")
    if deg is not None:
        p95 = k6_p95(deg, "http_req_duration")
        breached = []
        for name, t in (deg.get("metrics", {}) or {}).items():
            for k, v in (t.get("thresholds") or {}).items():
                if v is True:   # True == breached
                    breached.append(f"{name}:{k}")
        rows.append({
            "id": "§7.A-Deg",
            "target": "GET /cart P95 < 20ms while product-service down",
            "observed": (
                f"P95={fmt_ms(p95)}; thresholds breached={breached or 'none'}"
            ),
            "status": "PASS" if (p95 is not None and p95 < 20 and not breached)
                      else ("FAIL" if p95 is not None else "MISSING"),
            "evidence": "script/k6/results/cart_get_degraded.json",
        })

    # §3.C — Order race / deadlock free
    race = load_json(rd / "chaos_order_race.json")
    if race is not None:
        deadlocks = race.get("deadlock_errors", 0)
        s500 = race.get("ship_500", 0)
        log500 = race.get("http_500_in_logs", 0)
        ok = (deadlocks == 0) and (s500 == 0) and (log500 == 0)
        rows.append({
            "id": "§3.C",
            "target": "Concurrent Kafka + HTTP transition — 0 deadlocks, 0 500s",
            "observed": (
                f"rounds={race.get('rounds')} ship 200/409/500/other="
                f"{race.get('ship_200')}/{race.get('ship_409')}/{race.get('ship_500')}/{race.get('ship_other')}; "
                f"deadlocks={deadlocks}, 5xx_in_logs={log500}"
            ),
            "status": "PASS" if ok else "FAIL",
            "evidence": "script/k6/results/chaos_order_race.json",
        })

    # §7.D-API — burst
    api = load_json(rd / "rate_limit_api.json")
    if api is not None:
        c429 = k6_count(api, "status_429") or 0
        c200 = k6_count(api, "status_200") or 0
        cother = k6_count(api, "status_other") or 0
        rows.append({
            "id": "§7.D-API",
            "target": "Nginx api_limit — ≥35 of 50 burst are 429 (matches burst=5 cfg)",
            "observed": f"200={c200}, 429={c429}, other={cother}",
            "status": "PASS" if c429 >= 35 else "FAIL",
            "evidence": "script/k6/results/rate_limit_api.json",
        })

    # §7.D-Auth — sequential
    auth = load_json(rd / "rate_limit_auth.json")
    if auth is not None:
        c429 = k6_count(auth, "status_429") or 0
        c200 = k6_count(auth, "status_200") or 0
        c401 = k6_count(auth, "status_401") or 0
        cother = k6_count(auth, "status_other") or 0
        rows.append({
            "id": "§7.D-Auth",
            "target": "Nginx auth_limit — ≥1 of 9 sequential logins is 429",
            "observed": f"200={c200}, 401={c401}, 429={c429}, other={cother}",
            "status": "PASS" if c429 >= 1 else "FAIL",
            "evidence": "script/k6/results/rate_limit_auth.json",
        })

    # Mid-saga kill recovery
    kill = load_json(rd / "chaos_saga_kill.json")
    if kill is not None:
        pending = kill.get("pending_count", 0)
        dlq_delta = kill.get("dlq_delta", 0)
        dups = kill.get("duplicate_payment_count", 0)
        ok = (pending == 0) and (dlq_delta == 0) and (dups == 0)
        rows.append({
            "id": "Mid-Saga",
            "target": "Payment-service kill recovery — 0 PENDING, 0 DLQ delta, 0 dup payments",
            "observed": (
                f"pending={pending}, dlq_delta={dlq_delta}, duplicates={dups}, "
                f"payments_in_window={kill.get('payments_in_window')}"
            ),
            "status": "PASS" if ok else "FAIL",
            "evidence": "script/k6/results/chaos_saga_kill.json",
        })

    ts = dt.datetime.now().strftime("%Y-%m-%d %H:%M")
    section = [f"## Phase 3 — Resilience & Chaos  (run {ts})\n",
               "| Target ID | Target | Observed | Status | Evidence |",
               "|---|---|---|---|---|"]
    for r in rows:
        section.append(f"| {r['id']} | {r['target']} | {r['observed']} | **{r['status']}** | `{r['evidence']}` |")
    section.append("")
    new_block = "\n".join(section)

    out_path = Path(args.output)
    existing = out_path.read_text() if out_path.exists() else ""
    if "## Phase 3 — Resilience & Chaos" in existing:
        before, _, rest = existing.partition("## Phase 3 — Resilience & Chaos")
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
