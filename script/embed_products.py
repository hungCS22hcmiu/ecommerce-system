#!/usr/bin/env python3
"""
Batch-embed all ACTIVE products and write vectors to the products.embedding column.

Connects directly to ecommerce_products Postgres (exposed on localhost:5432).
Requires the AI service to be reachable — see usage note below.

Usage:
  # Recommended: run inside the ai-service container (no port-exposure needed)
  docker exec -it ecommerce-ai-service python scripts/embed_products.py

  # Alternative: expose port 9000 in docker-compose.yml ai-service block,
  # then restart with: docker compose up -d ai-service
  # and run from the host:
  python3 script/embed_products.py

  # Re-embed everything (ignore existing vectors):
  docker exec -it ecommerce-ai-service python scripts/embed_products.py --force

Prerequisites (host mode only):
  pip install "psycopg[binary]" requests

Env vars:
  AI_SERVICE_URL      — default: http://localhost:9000
  PRODUCTS_DB_HOST    — default: localhost
  PRODUCTS_DB_PORT    — default: 5432
  PRODUCTS_DB_NAME    — default: ecommerce_products
  PRODUCTS_DB_USER    — default: postgres
  PRODUCTS_DB_PASSWORD — default: postgres
"""
import argparse
import math
import os
import sys
import time

import psycopg
import requests

AI_URL = os.getenv("AI_SERVICE_URL", "http://localhost:9000")
BATCH_SIZE = 64

PRODUCTS_DSN = (
    f"host={os.getenv('PRODUCTS_DB_HOST', 'localhost')} "
    f"port={os.getenv('PRODUCTS_DB_PORT', '5432')} "
    f"dbname={os.getenv('PRODUCTS_DB_NAME', 'ecommerce_products')} "
    f"user={os.getenv('PRODUCTS_DB_USER', 'postgres')} "
    f"password={os.getenv('PRODUCTS_DB_PASSWORD', 'postgres')}"
)


def check_ai_service(url: str) -> bool:
    try:
        r = requests.get(f"{url}/health/ready", timeout=5)
        return r.status_code == 200
    except Exception:
        return False


def embed_batch(texts: list) -> list:
    r = requests.post(f"{AI_URL}/embed/batch", json={"texts": texts}, timeout=30)
    r.raise_for_status()
    return r.json()["embeddings"]


def main():
    parser = argparse.ArgumentParser(description="Embed products and store vectors in Postgres.")
    parser.add_argument("--force", action="store_true",
                        help="Re-embed even if embedding already exists")
    args = parser.parse_args()

    # Connectivity check
    print(f"AI service URL: {AI_URL}")
    print("Checking AI service connectivity...", end=" ", flush=True)
    if not check_ai_service(AI_URL):
        print("FAILED\n")
        print("Cannot reach the AI service.")
        print("The ai-service port is not exposed by default in docker-compose.\n")
        print("Option A — run this script inside Docker (recommended):")
        print("  docker exec -it ecommerce-ai-service python scripts/embed_products.py\n")
        print("Option B — expose the ai-service port in docker-compose.yml:")
        print("  Add under the ai-service block:  ports: [\"9000:9000\"]")
        print("  Then: docker compose up -d ai-service")
        print("  Then: python3 script/embed_products.py")
        sys.exit(1)
    print("OK")

    conn = psycopg.connect(PRODUCTS_DSN)

    # Fetch products to embed
    where_clause = "" if args.force else "AND p.embedding IS NULL"
    rows = conn.execute(
        f"""
        SELECT p.id,
               p.name || ' ' || COALESCE(p.description, '') || ' ' || COALESCE(c.name, '') AS text
        FROM products p
        LEFT JOIN categories c ON c.id = p.category_id
        WHERE p.status = 'ACTIVE'
          {where_clause}
        ORDER BY p.id
        """
    ).fetchall()

    total = len(rows)
    if total == 0:
        print("No products to embed.")
        conn.close()
        return

    mode = "force-re-embedding all" if args.force else "embedding new/unembedded"
    print(f"Found {total:,} ACTIVE products ({mode}).")
    print(f"Batch size: {BATCH_SIZE} | Total batches: {math.ceil(total / BATCH_SIZE)}\n")

    updated = 0
    t0 = time.time()

    for batch_num, start in enumerate(range(0, total, BATCH_SIZE), 1):
        batch = rows[start: start + BATCH_SIZE]
        ids   = [r[0] for r in batch]
        texts = [r[1] for r in batch]

        embeddings = embed_batch(texts)

        with conn.cursor() as cur:
            cur.executemany(
                "UPDATE products SET embedding = %s::vector WHERE id = %s",
                [(f"[{','.join(str(v) for v in emb)}]", pid) for emb, pid in zip(embeddings, ids)],
            )
        conn.commit()

        updated += len(batch)
        elapsed = time.time() - t0
        rate    = updated / elapsed if elapsed > 0 else 0
        remaining = total - updated
        eta = remaining / rate if rate > 0 else 0
        n_batches = math.ceil(total / BATCH_SIZE)
        print(
            f"Batch {batch_num:>4}/{n_batches} | {updated:>6}/{total} products "
            f"| {rate:>6.0f}/s | ETA {eta:>5.0f}s",
            end="\r",
        )

    elapsed = time.time() - t0
    print(f"\n\nDone! {updated:,} embeddings written in {elapsed:.1f}s "
          f"(avg {updated / elapsed:.0f} products/s)")

    conn.close()

    print("\nVerify with:")
    print("  curl 'localhost/api/v1/products/ai-search?q=gaming+laptop&limit=5'")


if __name__ == "__main__":
    main()
