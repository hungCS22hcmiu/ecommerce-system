# ai-service

Python FastAPI sidecar that converts text to 384-dimensional embeddings using `all-MiniLM-L6-v2` (sentence-transformers). Used by `product-service` for semantic product search via pgvector.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health/live` | Process up check |
| `GET` | `/health/ready` | Model loaded check (503 until ready) |
| `POST` | `/embed` | `{"text": str}` → `{"embedding": float[384]}` |
| `POST` | `/embed/batch` | `{"texts": str[]}` (≤64) → `{"embeddings": float[][]}` |

## Running locally (inside Docker)

```bash
docker compose build ai-service
docker compose up -d ai-service
docker compose logs -f ai-service   # wait for "Application startup complete"
```

The service is internal-only — no published port. Other containers reach it at `http://ai-service:9000`.

## Running tests

```bash
docker exec ecommerce-ai-service sh -c "cd /app && pytest tests/ -v"
```

## Backfill product embeddings

Populates `products.embedding` for all ACTIVE products with `embedding IS NULL`:

```bash
docker exec ecommerce-ai-service python scripts/embed_products.py
```

Force re-embed all ACTIVE rows:

```bash
docker exec ecommerce-ai-service python scripts/embed_products.py --force
```

## Nightly refresh (cron)

To keep embeddings current after bulk imports, schedule a nightly backfill:

```bash
# Example cron entry (host machine, requires docker):
0 2 * * * docker exec ecommerce-ai-service python scripts/embed_products.py
```

Write-through on individual product create/update is handled by `product-service` (Week 24).
