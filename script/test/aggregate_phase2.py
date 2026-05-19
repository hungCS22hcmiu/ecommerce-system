#!/usr/bin/env python3
"""Aggregate Phase 2 results into docs/testing/test_result.md.

Reads:
  - k6 summary JSON files                            (per-endpoint P95/throughput/check rates)
  - sidecar monitor CSV files                        (pg connections, kafka lag, ai memory)
  - product-service log lines with `ai.search.layer` (per-layer AI timings)
  - script/k6/results/kafka_throughput.json          (throughput script output)
  - script/k6/results/ai_cold_start.json             (cold-start script output)

Writes a "Phase 2 — Load & Throughput" section, appended to test_result.md.
"""
from __future__ import annotations

import argparse
import csv
import datetime as dt
import json
import re
import subprocess
import sys
from pathlib import Path
from collections import defaultdict


# ── k6 summary parsing (matches Phase 1 schema in k6 v1.x) ───────────────────

def _metric(summary, name):
    if not summary: return {}
    m = summary.get("metrics", {}).get(name) or {}
    return m.get("values", m)

def k6_p95(summary, name):
    m = _metric(summary, name)
    v = m.get("p(95)") or m.get("p95")
    return None if v is None else float(v)

def k6_rps(summary, name="http_reqs"):
    m = _metric(summary, name)
    v = m.get("rate")
    return None if v is None else float(v)

def k6_failed_rate(summary):
    m = _metric(summary, "http_req_failed")
    v = m.get("rate") or m.get("value")
    return None if v is None else float(v)

def k6_check_rate(summary):
    m = _metric(summary, "checks")
    v = m.get("rate") or m.get("value")
    return None if v is None else float(v)

def load_json(path: Path):
    if not path.exists(): return None
    try: return json.loads(path.read_text())
    except Exception as exc:
        print(f"WARN: failed to parse {path}: {exc}", file=sys.stderr)
        return None


# ── AI layer logs ─────────────────────────────────────────────────────────────

AI_LAYER_RE = re.compile(
    r"ai\.search\.layer.*?embed_ms=(\d+)\s+vector_ms=(\d+)\s+rerank_ms=(\d+)"
)

def fetch_product_service_logs(since: str) -> str:
    try:
        out = subprocess.run(
            ["docker", "compose", "logs", "--since", since, "product-service"],
            capture_output=True, text=True, timeout=30,
        )
        return out.stdout + "\n" + out.stderr
    except Exception as exc:
        print(f"WARN: failed to fetch product-service logs: {exc}", file=sys.stderr)
        return ""

def ai_layer_p95(log_text: str):
    embed, vector, rerank = [], [], []
    for m in AI_LAYER_RE.finditer(log_text):
        embed.append(int(m.group(1)))
        vector.append(int(m.group(2)))
        rerank.append(int(m.group(3)))
    def p95(xs):
        if not xs: return None
        xs = sorted(xs)
        idx = max(0, min(len(xs) - 1, int(0.95 * len(xs)) - 1))
        return xs[idx] if idx >= 0 else xs[-1]
    return {
        "samples": len(embed),
        "embed_p95": p95(embed),
        "vector_p95": p95(vector),
        "rerank_p95": p95(rerank),
    }


# ── Sidecar parsers ───────────────────────────────────────────────────────────

def pg_conn_max(csv_path: Path):
    if not csv_path.exists(): return {"max_per_db": {}, "max_global": None}
    per_db_max = defaultdict(int)
    global_max = 0
    by_ts = defaultdict(int)
    with csv_path.open() as f:
        reader = csv.DictReader(f)
        for row in reader:
            try:
                n = int(row["connections"])
                ts = row["timestamp"]
                db = row["datname"]
            except Exception:
                continue
            per_db_max[db] = max(per_db_max[db], n)
            by_ts[ts] += n
    if by_ts:
        global_max = max(by_ts.values())
    return {"max_per_db": dict(per_db_max), "max_global": global_max}

def kafka_lag_max(csv_path: Path):
    if not csv_path.exists(): return None
    mx = 0
    with csv_path.open() as f:
        reader = csv.DictReader(f)
        for row in reader:
            try: mx = max(mx, int(row["lag"]))
            except Exception: pass
    return mx

def ai_mem_max(csv_path: Path):
    if not csv_path.exists(): return None
    # mem_usage is like "123.4MiB"; parse to MiB.
    def to_mib(s):
        s = s.strip()
        if s.endswith("MiB"): return float(s[:-3])
        if s.endswith("GiB"): return float(s[:-3]) * 1024
        if s.endswith("KiB"): return float(s[:-3]) / 1024
        try: return float(s)
        except Exception: return 0.0
    mx = 0.0
    with csv_path.open() as f:
        reader = csv.DictReader(f)
        for row in reader:
            mx = max(mx, to_mib(row.get("mem_usage", "")))
    return mx  # in MiB

def redis_peak(log_path: Path):
    """`redis-cli --latency-history` prints '<min> <max> <avg> <count>' rows.
    Return the max observed across all rows (in ms)."""
    if not log_path.exists(): return None
    peak = None
    for line in log_path.read_text().splitlines():
        parts = line.strip().split()
        if len(parts) >= 4 and parts[1].replace('.', '', 1).isdigit():
            try:
                mx = float(parts[1])
                peak = mx if peak is None else max(peak, mx)
            except ValueError:
                pass
    return peak

def hikari_leak_count(log_path: Path):
    if not log_path.exists(): return None
    for line in reversed(log_path.read_text().splitlines()):
        if line.startswith("leak_count="):
            try: return int(line.split("=", 1)[1].strip())
            except Exception: return None
    return None


# ── Formatting helpers ────────────────────────────────────────────────────────

def status_cell(observed, target, comparison="lt"):
    """Returns PASS / FAIL / AT_RISK markdown given observed & target numbers."""
    if observed is None: return "MISSING"
    if comparison == "lt":
        # AT_RISK = within 10% of the target on the failing side
        if observed > target * 1.1: return "FAIL"
        if observed > target:       return "AT_RISK"
        return "PASS"
    elif comparison == "gte":
        if observed < target * 0.9: return "FAIL"
        if observed < target:       return "AT_RISK"
        return "PASS"
    return "MISSING"

def fmt_ms(v):  return f"{v:.0f}ms" if v is not None else "n/a"
def fmt_int(v): return f"{int(v)}"   if v is not None else "n/a"
def fmt_pct(v): return f"{v*100:.2f}%" if v is not None else "n/a"


# ── Row builders ──────────────────────────────────────────────────────────────

def perf_row(target_id, target_label, summary_path, p95_target_ms, rps_target, evidence):
    summary = load_json(summary_path)
    p95 = k6_p95(summary, "http_req_duration") if summary else None
    if p95 is None and summary:
        p95 = k6_p95(summary, "http_req_duration{expected_response:true}")
    rps = k6_rps(summary) if summary else None
    failed = k6_failed_rate(summary) if summary else None
    chk = k6_check_rate(summary) if summary else None
    status = "MISSING"
    if p95 is not None and rps is not None:
        s1 = status_cell(p95, p95_target_ms, "lt")
        s2 = status_cell(rps, rps_target, "gte")
        order = {"FAIL": 0, "AT_RISK": 1, "PASS": 2, "MISSING": 3}
        status = min([s1, s2], key=lambda x: order[x])
    return {
        "id": target_id,
        "target": target_label,
        "observed": f"P95={fmt_ms(p95)}, throughput={fmt_int(rps)}/s, http_req_failed={fmt_pct(failed)}, checks={fmt_pct(chk)}",
        "status": status,
        "evidence": evidence,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results-dir", default="script/k6/results")
    ap.add_argument("--monitors-dir", default="script/k6/results/monitors")
    ap.add_argument("--logs-since", default="20m")
    ap.add_argument("--output", default="docs/testing/test_result.md")
    args = ap.parse_args()

    rd = Path(args.results_dir)
    md = Path(args.monitors_dir)

    rows = []

    # §1 Service performance
    rows.append(perf_row("§1-User",     "POST /auth/login — P95 < 300ms @ 100 RPS",
                         rd / "auth_login.json", 300, 100, "script/k6/results/auth_login.json"))
    rows.append(perf_row("§1-Cart",     "POST /cart/items — P95 < 40ms @ 500 RPS",
                         rd / "cart_ops.json", 40, 500, "script/k6/results/cart_ops.json"))
    rows.append(perf_row("§1-Product",  "GET /products/search — P95 < 150ms @ 150 RPS",
                         rd / "product_browse.json", 150, 150, "script/k6/results/product_browse.json"))
    rows.append(perf_row("§1-Order",    "POST /orders — P95 < 400ms @ 50 RPS",
                         rd / "order_create.json", 400, 50, "script/k6/results/order_create.json"))

    # §1 Kafka throughput
    kt = load_json(rd / "kafka_throughput.json")
    if kt is not None:
        thr = kt.get("throughput_msg_per_s")
        rows.append({
            "id": "§1-Kafka",
            "target": "orders.created consumer — throughput ≥ 200 msg/s",
            "observed": f"throughput={thr} msg/s, drain={kt.get('drain_secs')}s, n={kt.get('num_messages')}",
            "status": status_cell(thr, 200, "gte"),
            "evidence": "script/k6/results/kafka_throughput.json",
        })
    else:
        rows.append({"id": "§1-Kafka", "target": "orders.created consumer ≥ 200 msg/s",
                     "observed": "n/a", "status": "MISSING",
                     "evidence": "script/k6/results/kafka_throughput.json"})

    # §3.A 50 VU composite (must pass both p95 AND error-rate budget)
    co = load_json(rd / "checkout_50vu.json")
    if co is not None:
        chain_p95 = k6_p95(co, "checkout_chain_ms") or k6_p95(co, "http_req_duration")
        failed = k6_failed_rate(co)
        s_p95   = status_cell(chain_p95, 1000, "lt")
        s_err   = status_cell(failed, 0.001, "lt") if failed is not None else "MISSING"
        order = {"FAIL": 0, "AT_RISK": 1, "PASS": 2, "MISSING": 3}
        worst = min([s_p95, s_err], key=lambda x: order[x])
        rows.append({
            "id": "§3.A",
            "target": "50 VU composite checkout — err <0.1%, P95 <1000ms",
            "observed": f"chain P95={fmt_ms(chain_p95)}, failed={fmt_pct(failed)}",
            "status": worst,
            "evidence": "script/k6/results/checkout_50vu.json",
        })
    else:
        rows.append({"id": "§3.A", "target": "50 VU composite checkout",
                     "observed": "n/a", "status": "MISSING",
                     "evidence": "script/k6/results/checkout_50vu.json"})

    # §4 AI search — total P95 from k6 + per-layer from logs
    ai = load_json(rd / "ai_search.json")
    ai_total_p95 = k6_p95(ai, "http_req_duration") if ai else None
    ai_rps = k6_rps(ai) if ai else None
    rows.append({
        "id": "§4-Total",
        "target": "/ai-search total P95 < 250ms @ 20 RPS",
        "observed": f"P95={fmt_ms(ai_total_p95)}, throughput={fmt_int(ai_rps)}/s",
        "status": status_cell(ai_total_p95, 250, "lt") if ai_total_p95 else "MISSING",
        "evidence": "script/k6/results/ai_search.json",
    })

    layers = ai_layer_p95(fetch_product_service_logs(args.logs_since))
    rows.append({
        "id": "§4-Embed", "target": "AI embed P95 < 100ms",
        "observed": f"P95={fmt_ms(layers['embed_p95'])} (n={layers['samples']})",
        "status": status_cell(layers["embed_p95"], 100, "lt"),
        "evidence": "docker compose logs product-service | grep ai.search.layer",
    })
    rows.append({
        "id": "§4-Vector", "target": "pgvector search P95 < 50ms",
        "observed": f"P95={fmt_ms(layers['vector_p95'])} (n={layers['samples']})",
        "status": status_cell(layers["vector_p95"], 50, "lt"),
        "evidence": "docker compose logs product-service | grep ai.search.layer",
    })
    rows.append({
        "id": "§4-Rerank", "target": "Re-ranking P95 < 30ms",
        "observed": f"P95={fmt_ms(layers['rerank_p95'])} (n={layers['samples']})",
        "status": status_cell(layers["rerank_p95"], 30, "lt"),
        "evidence": "docker compose logs product-service | grep ai.search.layer",
    })

    # §4 Cold start
    cs = load_json(rd / "ai_cold_start.json")
    if cs is not None:
        ms = cs.get("cold_start_ms")
        rows.append({
            "id": "§4-ColdStart", "target": "AI cold start < 15s",
            "observed": f"{ms}ms",
            "status": status_cell(ms, 15000, "lt") if isinstance(ms, (int, float)) else "MISSING",
            "evidence": "script/k6/results/ai_cold_start.json",
        })

    # §4 Memory
    mem = ai_mem_max(md / "ai_mem.csv")
    if mem is not None:
        rows.append({
            "id": "§4-Memory", "target": "AI service memory ≤ 1.5 GB (1536 MiB)",
            "observed": f"peak={mem:.0f} MiB",
            "status": status_cell(mem, 1536, "lt"),
            "evidence": "script/k6/results/monitors/ai_mem.csv",
        })

    # §5 Kafka lag, Redis, Postgres
    klag = kafka_lag_max(md / "kafka_lag.csv")
    if klag is not None:
        rows.append({
            "id": "§5-KafkaLag", "target": "Peak consumer lag < 50",
            "observed": f"peak={klag}",
            "status": status_cell(klag, 50, "lt"),
            "evidence": "script/k6/results/monitors/kafka_lag.csv",
        })

    redis_pk = redis_peak(md / "redis.log")
    if redis_pk is not None:
        rows.append({
            "id": "§5-Redis", "target": "Redis peak max latency < 1 ms (P99 acquisition)",
            "observed": f"{redis_pk} ms",
            "status": status_cell(redis_pk, 1.0, "lt"),
            "evidence": "script/k6/results/monitors/redis.log",
        })

    pg = pg_conn_max(md / "pg.csv")
    if pg["max_global"] is not None:
        per_db = ", ".join(f"{k}:{v}" for k, v in sorted(pg["max_per_db"].items())) or "n/a"
        rows.append({
            "id": "§6-PG-Conns",
            "target": "Postgres conns: per-DB ≤25 (Go)/20 (Java), global ≤ 150",
            "observed": f"global peak={pg['max_global']}; per-DB peaks={per_db}",
            "status": status_cell(pg["max_global"], 150, "lt") if pg["max_global"] else "MISSING",
            "evidence": "script/k6/results/monitors/pg.csv",
        })

    # §6 HikariCP leaks
    leaks = hikari_leak_count(md / "hikari.log")
    if leaks is not None:
        rows.append({
            "id": "§6-HikariLeak",
            "target": "Zero HikariCP leak warnings",
            "observed": f"leak_count={leaks}",
            "status": "PASS" if leaks == 0 else "FAIL",
            "evidence": "script/k6/results/monitors/hikari.log",
        })

    # ── Write markdown ────────────────────────────────────────────────────────
    ts = dt.datetime.now().strftime("%Y-%m-%d %H:%M")
    section = [f"## Phase 2 — Load & Throughput  (run {ts})\n",
               "| Target ID | Target | Observed | Status | Evidence |",
               "|---|---|---|---|---|"]
    for r in rows:
        section.append(f"| {r['id']} | {r['target']} | {r['observed']} | **{r['status']}** | `{r['evidence']}` |")
    section.append("")
    new_block = "\n".join(section)

    out_path = Path(args.output)
    existing = out_path.read_text() if out_path.exists() else ""
    if "## Phase 2 — Load & Throughput" in existing:
        before, _, rest = existing.partition("## Phase 2 — Load & Throughput")
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
