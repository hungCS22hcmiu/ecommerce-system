# Phase 6: RAG Product Search — Implementation Plan

## Context

Phase 6 of `docs/planning/six-month-plan.md` (Weeks 21–24) adds AI-powered product search to the existing e-commerce platform. Today, `GET /api/v1/products/search?q=` uses Postgres FTS (GIN index on `name || description` via `to_tsvector` / `plainto_tsquery`) — strong for keyword overlap but blind to semantic intent ("comfortable shoes for long walks" vs. a description that says "all-day cushioning").

Goal: introduce **semantic search** via embeddings stored in `pgvector` and a small Python `ai-service` sidecar, with a graceful fallback to the existing keyword search when the AI path is unavailable. Surface it in the React frontend as a toggle on the existing `ProductListPage`. The implementation reuses the existing layering (controller → service → repository → cache → envelope) and adds **no new infrastructure besides one Python container and a Postgres extension swap**.

### Architecture target

```
Browser (Smart Search toggle)
  → Nginx :80
    → product-service /api/v1/products/ai-search
        → ai-service POST /embed   (sentence-transformers, all-MiniLM-L6-v2, 384-d)
        → Postgres: SELECT ... ORDER BY embedding <=> $1 LIMIT n
        → cache result in Redis ("aiSearch", 15-min TTL)
        → ApiResponse envelope
    Fallback: ai-service timeout / 5xx → existing keyword FTS path, response tagged mode: "fallback-keyword"

Embedding refresh:
  - Backfill: ai-service/scripts/embed_products.py (one-shot + nightly)
  - Write-through: product create/update → @Async embed + UPDATE products SET embedding = ?::vector
```

---

## Critical files to create / modify

### Infrastructure

- `docker-compose.yml`
  - Swap `postgres:15-alpine` → `pgvector/pgvector:pg15`
  - Add `ai-service` block: internal-only on port 9000 (no published port), `backend` network, healthcheck on `/health/ready`
- `.env.example`
  - `AI_SERVICE_URL=http://ai-service:9000`
  - `AI_SERVICE_TIMEOUT_MS=2000`
  - `EMBEDDING_MODEL=all-MiniLM-L6-v2`
  - `EMBEDDING_DIM=384`
- `script/init-databases.sql` — append under `\c ecommerce_products`:
  ```sql
  CREATE EXTENSION IF NOT EXISTS vector;
  ```
- `nginx/nginx.conf` — **no change**; ai-service stays inside the Docker network behind product-service.

### New service: `ai-service/` (Python FastAPI)

- `ai-service/Dockerfile` — `python:3.11-slim`; pre-downloads the model into the image layer so cold start is fast
- `ai-service/requirements.txt` — `fastapi`, `uvicorn[standard]`, `sentence-transformers`, `psycopg[binary]`, `pytest`
- `ai-service/main.py`
  - `POST /embed`  `{ "text": str }` → `{ "embedding": float[384] }`
  - `POST /embed/batch`  `{ "texts": str[] }` (≤ 64) → `{ "embeddings": float[][] }`
  - `GET /health/live` — process up
  - `GET /health/ready` — model loaded
- `ai-service/scripts/embed_products.py` — batch backfill: reads `ecommerce_products.products`, embeds `name + ' ' + description + ' ' + category.name`, writes vector via `UPDATE ... WHERE id = $1`. Idempotent (`WHERE embedding IS NULL`, plus `--force` flag).
- `ai-service/tests/test_embed.py`
- `ai-service/README.md`

### product-service (Java) — modify

- `src/main/resources/db/migration/V3__add_product_embeddings.sql`
  ```sql
  ALTER TABLE products ADD COLUMN embedding vector(384);
  CREATE INDEX idx_products_embedding ON products
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
  ```
- `pom.xml` — add `com.pgvector:pgvector` (Hibernate type) + `spring-boot-starter-webflux` (WebClient)
- `model/Product.java` — add `@Column(columnDefinition = "vector(384)") private float[] embedding;` mapped via the pgvector Hibernate user type
- `repository/ProductRepository.java`
  ```java
  @Query(value = """
      SELECT * FROM products
      WHERE status = 'ACTIVE' AND embedding IS NOT NULL
      ORDER BY embedding <=> CAST(:queryVec AS vector)
      LIMIT :limit
      """, nativeQuery = true)
  List<Product> findBySemanticSimilarity(@Param("queryVec") String queryVec,
                                         @Param("limit") int limit);
  ```
- `config/HttpClientConfig.java` — `WebClient` bean pointed at `${ai-service.url}` with 2 s timeout (fail-fast, no retries — fallback path handles it)
- `client/EmbeddingClient.java` — `embed(String text): float[]`; throws `AIServiceException` on timeout/non-2xx
- `service/AISearchService.java` (interface) + `AISearchServiceImpl.java`
  - validate `q.length() >= 2`
  - call `EmbeddingClient.embed(q)` → vector
  - call `repo.findBySemanticSimilarity(toPgvectorLiteral(vector), limit)`
  - map to `ProductSummaryResponse`, attach scores (`1 - distance`)
- `controller/ProductController.java` — add:
  ```
  GET /api/v1/products/ai-search?q=<query>&limit=<int>   (default 10, max 50)
  ```
- `dto/AISearchResponse.java`
  ```java
  record AISearchResponse(
      String query,
      List<ProductSummaryResponse> results,
      List<Double> scores,
      String mode   // "ai" | "fallback-keyword"
  ) {}
  ```
- `exception/AIServiceException.java` + handler in `GlobalExceptionHandler` — catch at controller-advice level and route to keyword fallback (NOT a 5xx response — the user must get results)
- `config/RedisConfig.java` — register `"aiSearch"` cache, TTL 15 min
- `service/ProductServiceImpl.java` — on `createProduct` / `updateProduct` (only when `name`, `description`, or `categoryId` changed): `@Async` job that calls EmbeddingClient + JDBC `UPDATE products SET embedding = ?::vector WHERE id = ?`. Behind `ai-service.write-through-enabled` (default `true`). Failures logged, don't fail the create.
- `application.yaml`
  ```yaml
  ai-service:
    url: ${AI_SERVICE_URL:http://localhost:9000}
    timeout-ms: ${AI_SERVICE_TIMEOUT_MS:2000}
    write-through-enabled: true
  ```

### Tests

- `product-service/src/test/.../service/AISearchServiceTest.java` — unit tests with mocked `EmbeddingClient` + repo
- `product-service/src/test/.../integration/AISearchIntegrationTest.java` — Testcontainers (`pgvector/pgvector:pg15` + WireMock for ai-service): seed 5 products with hand-crafted 384-d unit vectors and assert ranking order
- `product-service/src/test/.../integration/AISearchFallbackTest.java` — WireMock returns 500 / times out → controller still returns 200 with `mode: "fallback-keyword"`

### API spec

- `api/openapi.yaml` — add `/products/ai-search` GET; add `AISearchResponse` schema

### Frontend (`frontend/`)

- `src/types/product.ts` — add `AISearchResponse` type
- `src/features/products/productApi.ts` — add `aiSearch(q, limit)` → `ApiResponse<AISearchResponse>`
- `src/features/products/useProductAISearch.ts` — `queryKey: ['products', 'ai-search', q]`, `staleTime: 60_000`, `enabled: q.trim().length >= 2`
- `src/features/products/SearchBar.tsx` — toggle: **Keyword** | **Smart Search ✨**; persists via `?mode=ai`
- `src/pages/ProductListPage.tsx` — branch on `mode`: render existing grid or AI results; when `mode === 'ai'`, show `AISearchBadge` per result; on `mode === 'fallback-keyword'`, fire toast "Smart search unavailable — showing keyword results"
- `src/components/shared/AISearchBadge.tsx` — small "✨ Semantic match" pill

---

## Reusable patterns (don't reinvent)

- **Response envelope:** `ApiResponse<T>` (`product-service/.../dto/ApiResponse.java`) — wrap `AISearchResponse` the same way as every other endpoint.
- **Caching:** mirror the `@Cacheable("productList", key = "{'search', #query, ...}")` pattern from `ProductServiceImpl.searchProducts` (line 119) — apply identically to `aiSearchProducts`.
- **Exception handling:** add `@ExceptionHandler(AIServiceException.class)` next to existing handlers in `GlobalExceptionHandler` (already maps OptimisticLock / NotFound / InsufficientStock / AccessDenied).
- **Response mapping:** reuse the existing `ProductSummaryResponse` mapper — don't introduce a new product DTO.
- **Frontend skeleton + empty state:** reuse `ProductCardSkeleton` and `EmptyState` already in `frontend/src/components/`.
- **Frontend URL state:** reuse the `useSearchParams` pattern from `ProductListPage` — just add `mode` alongside existing `q` and `page`.

---

## Step-by-step build order (4 weeks, 18 h/week)

### Week 21 — Foundations & pgvector setup (study + DB)

1. Hand-draw the RAG flow on paper (no code): browser → product-service → ai-service → pgvector.
2. Read the pgvector README; understand `<=>` (cosine distance), `<->` (L2), `<#>` (inner product).
3. Local Python notebook: load `all-MiniLM-L6-v2`, embed 3 sample strings, eyeball cosine similarity.
4. Swap Postgres image in `docker-compose.yml` to `pgvector/pgvector:pg15`.
5. Add `CREATE EXTENSION vector` to `script/init-databases.sql` under `\c ecommerce_products`.
6. Write `V3__add_product_embeddings.sql` (column + IVFFLAT index, `lists=100`).
7. Verify migration applies on a wiped volume: `docker compose down -v && docker compose up -d postgres product-service`.
8. Confirm `\d products` shows `embedding vector(384)` and the IVFFLAT index.

**Deliverable:** `embedding` column exists. You can `INSERT` a literal `'[0.1,0.2,...]'::vector(384)` row by hand and `SELECT ... ORDER BY embedding <=> '[...]'` returns it correctly.

### Week 22 — ai-service sidecar + batch embed

1. Scaffold `ai-service/` (FastAPI + sentence-transformers + uvicorn).
2. Implement `POST /embed` (single) and `POST /embed/batch` (≤ 64 texts).
3. Health checks: `/health/live` (process up), `/health/ready` (model loaded).
4. Dockerfile pre-downloads the model in the image layer so cold start is fast (~5–10 s).
5. Add `ai-service` to `docker-compose.yml` on the `backend` network, no published port, healthcheck on `/health/ready`.
6. Write `scripts/embed_products.py` — backfill `products.embedding` for ACTIVE products where `embedding IS NULL` (batches of 32).
7. Run the backfill against seed data; spot-check 5 rows.
8. Manual SQL test:
   ```sql
   SELECT name FROM products ORDER BY embedding <=> '<query_vec>'::vector LIMIT 5;
   ```

**Deliverable:** All seeded products have embeddings. SQL-level semantic search visibly outperforms `LIKE '%shoes%'`.

### Week 23 — product-service `/ai-search` endpoint + fallback

1. Add `spring-boot-starter-webflux` + pgvector Hibernate type to `pom.xml`.
2. Map `embedding` field in `Product.java`.
3. Implement `EmbeddingClient` (WebClient, 2 s timeout, no retries — fail fast).
4. Implement `AISearchService.search(query, limit)`:
   - validate `q.length() >= 2`
   - call `EmbeddingClient.embed(q)`
   - call `repo.findBySemanticSimilarity(toPgvectorLiteral(vector), limit)`
   - map to `ProductSummaryResponse`, attach scores (`1 - distance`)
5. Controller `GET /ai-search` with `@Cacheable("aiSearch", key = "{#q, #limit}")` (TTL 15 min).
6. Fallback path: `AIServiceException` caught in advice → invoke existing keyword `searchProducts` → wrap response with `mode: "fallback-keyword"`, HTTP 200.
7. Add `"aiSearch"` cache to `RedisConfig`.
8. Add OpenAPI entries.

**Tests:**
- Unit: `AISearchServiceTest` (mock EmbeddingClient + repo).
- Integration: `AISearchIntegrationTest` (Testcontainers pgvector + WireMock): seed 5 products with hand-crafted 384-d unit vectors, assert ordering.
- Fallback: WireMock 500 / timeout → response 200, `mode: "fallback-keyword"`.

**Deliverable:** `curl 'http://localhost/api/v1/products/ai-search?q=comfortable+running+shoes'` returns ranked results. Stopping ai-service still returns results via keyword fallback.

### Week 24 — Frontend toggle + write-through refresh + polish

1. `productApi.aiSearch()` + `useProductAISearch` hook.
2. Toggle in `SearchBar` (`Keyword | ✨ Smart Search`), persisted in `?mode=ai`.
3. `ProductListPage` branches on `mode`; renders `AISearchBadge` on results when `mode === "ai"`.
4. Toast on `mode === "fallback-keyword"` (reuse existing `toast.ts`).
5. Write-through embedding refresh:
   - `ProductServiceImpl.createProduct` / `updateProduct` — when `name`/`description`/`categoryId` changed, schedule `@Async` job: EmbeddingClient + JDBC `UPDATE products SET embedding = ?::vector WHERE id = ?`.
   - Feature flag `ai-service.write-through-enabled` (default `true`).
6. Defensive nightly refresh: document a cron-style invocation of `embed_products.py --force` in `ai-service/README.md`. No orchestrator required at this scale.
7. Demo script: `script/demo-ai-search.sh` — runs 5 hand-picked natural-language queries side-by-side against keyword and AI search, prints both result sets.
8. ADR: `docs/adrs/rag-search.md` — model choice, dim choice (384 vs 1536), IVFFLAT vs HNSW, "what changes at 1 M products?".

**Deliverable:** Browser demo. User toggles Smart Search, types "shoes for long walks", gets semantically ranked results. Killing ai-service degrades gracefully with a toast.

---

## Verification (end-to-end)

After Week 24, **all** of the following must pass:

1. `docker compose down -v && docker compose up --build -d` — fresh stack boots; Flyway V3 applies; ai-service health-check goes green within 60 s.
2. `docker exec ecommerce-postgres psql -U postgres -d ecommerce_products -c "\dx"` — shows `vector` extension.
3. `docker exec ecommerce-postgres psql -U postgres -d ecommerce_products -c "SELECT count(*) FROM products WHERE embedding IS NOT NULL;"` — equals count of seeded ACTIVE products (post-backfill).
4. `curl 'http://localhost/api/v1/products/ai-search?q=comfortable+running+shoes&limit=5'` — 200, ranked results, `mode: "ai"`.
5. `docker compose stop ai-service && curl 'http://localhost/api/v1/products/ai-search?q=shoes'` — 200, `mode: "fallback-keyword"`, FTS results.
6. Create a new product via the seller flow → within ~5 s, AI search finds it (write-through proven).
7. `./mvnw test` in product-service — all green, including new AISearch suites.
8. `cd ai-service && pytest` — embed/batch tests pass.
9. Frontend `npm run build` — 0 TypeScript errors.
10. Manual browser walk: toggle on, search "long walks comfortable footwear", visually verify results are semantically better than keyword.
11. Cache check: same `q` twice within 60 s → Redis `MONITOR` shows one miss, one hit.

---

## Risks & decisions

- **Model dimensions — 384 (`all-MiniLM-L6-v2`) over 1536 (OpenAI):** free, local, fast; smaller index footprint; quality sufficient for this catalog size. Documented in the ADR.
- **IVFFLAT vs HNSW:** IVFFLAT at `lists=100` is fine for < 100 k rows; HNSW is overkill at demo scale. Discussed in the "scale to 1 M" reflection.
- **Write-through latency:** embedding takes ~50–150 ms — must be `@Async` so it never blocks the seller's create/update response. Failures logged but don't fail the create.
- **Cold start:** model load adds ~5–10 s to ai-service boot — Dockerfile pre-downloads the model layer; healthcheck guards against premature traffic.
- **Cache key correctness:** `"aiSearch"` cache is keyed on `(q, limit)` only — semantic search is anonymous/public, so that's correct. Short TTL (15 min) avoids needing surgical invalidation on product writes.
- **No nginx route for ai-service:** internal-only is the right call — keeps the ML surface off the public internet and avoids auth on the embed endpoint.

---

## Out of scope (deferred)

- Hybrid scoring (vector similarity + rating/popularity) — optional Week 24 stretch.
- Query analytics dashboard.
- Re-ranking or generation with an LLM (true RAG with generation) — pure retrieval is enough for the interview story.
- HNSW index migration.
- Per-tenant / per-locale embeddings.
