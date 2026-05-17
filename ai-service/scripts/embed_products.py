#!/usr/bin/env python3
"""
Backfill product embeddings.

Usage:
  python scripts/embed_products.py              # skips rows where embedding IS NOT NULL
  python scripts/embed_products.py --force      # re-embeds all ACTIVE rows

Env vars (all have defaults for local docker-compose):
  DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
  AI_SERVICE_URL  (default: http://localhost:9000)
"""
import argparse
import json
import os
import time
import urllib.request

import psycopg

DB_DSN = (
    f"host={os.getenv('DB_HOST', 'localhost')} "
    f"port={os.getenv('DB_PORT', '5432')} "
    f"dbname={os.getenv('DB_NAME', 'ecommerce_products')} "
    f"user={os.getenv('DB_USER', 'postgres')} "
    f"password={os.getenv('DB_PASSWORD', 'postgres')}"
)
AI_URL = os.getenv("AI_SERVICE_URL", "http://localhost:9000")
BATCH_SIZE = 64


def call_embed_batch(texts: list[str]) -> list[list[float]]:
    body = json.dumps({"texts": texts}).encode()
    req = urllib.request.Request(
        f"{AI_URL}/embed/batch",
        data=body,
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read())["embeddings"]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--force", action="store_true", help="re-embed all ACTIVE rows")
    args = parser.parse_args()

    with psycopg.connect(DB_DSN) as conn:
        where = "status = 'ACTIVE'" if args.force else "status = 'ACTIVE' AND embedding IS NULL"
        rows = conn.execute(
            f"""
            SELECT p.id,
                   p.name || ' ' || COALESCE(p.description, '') || ' ' || COALESCE(c.name, '') AS text
            FROM products p
            LEFT JOIN categories c ON c.id = p.category_id
            WHERE {where}
            ORDER BY p.id
            """
        ).fetchall()

    if not rows:
        print("Nothing to embed.")
        return

    total = len(rows)
    print(f"Embedding {total} products in batches of {BATCH_SIZE}...")
    updated = 0
    start = time.time()

    with psycopg.connect(DB_DSN) as conn:
        for i in range(0, total, BATCH_SIZE):
            batch = rows[i : i + BATCH_SIZE]
            ids = [r[0] for r in batch]
            texts = [r[1] for r in batch]

            embeddings = call_embed_batch(texts)

            pairs = [
                ("[" + ",".join(f"{v:.8f}" for v in vec) + "]", pid)
                for pid, vec in zip(ids, embeddings)
            ]
            conn.executemany(
                "UPDATE products SET embedding = %s::vector WHERE id = %s",
                pairs,
            )
            conn.commit()
            updated += len(batch)

            elapsed = time.time() - start
            rate = updated / elapsed if elapsed > 0 else 0
            eta = (total - updated) / rate if rate > 0 else 0
            print(f"  {updated}/{total} done | {rate:.0f} products/s | ETA {eta:.0f}s")

    print(f"Backfill complete. {updated} rows updated.")


if __name__ == "__main__":
    main()
