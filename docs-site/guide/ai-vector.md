# AI and Vector Search
<!-- audited 2026-03-21 -->

This guide covers AYB's shipped vector-search surface: embedding wiring, vector index admin APIs, nearest-neighbor queries, semantic queries, and hybrid search.

It is based on:

- `internal/api/vector_query.go`, `internal/api/semantic_query.go`, `internal/api/hybrid_search.go`, `internal/api/handler_list.go`
- `internal/server/vector_admin_handler.go`, `internal/server/routes_admin.go`
- `internal/cli/start_services_wiring_support.go::wireAIEmbedding`
- `internal/config/config_types.go::AIConfig`
- Tests: `vector_query_test.go`, `semantic_query_test.go`, `hybrid_search_test.go`, `vector_admin_handler_test.go`, `embedding_test.go`, `config_ai_test.go`

AI assistant endpoints under `/api/admin/ai/assistant*` are documented separately. This page focuses on vector search.

## Prerequisites

- A table with at least one `vector` column (`vector(N)` is recommended so dimensions are enforced).
- `pgvector` available in the connected database.
- An embedding-capable AI provider wired via `[ai]` config.

If you are running AYB in managed PostgreSQL mode, verify that the managed PostgreSQL build you are using includes `pgvector`. If it does not, point AYB at an external PostgreSQL instance with `pgvector` installed.

## AI embedding config

`AIConfig` keys used by vector search:

- `default_provider`
- `default_model`
- `embedding_provider` (optional, falls back to `default_provider`)
- `embedding_model` (optional)
- `embedding_dimensions` (map key format: `provider:model`, case-insensitive)
- `timeout`
- `max_retries`
- `breaker.failure_threshold`
- `breaker.open_seconds`
- `breaker.half_open_probe_limit`
- `providers.<name>.api_key`
- `providers.<name>.base_url`
- `providers.<name>.default_model`

Provider/model resolution in `wireAIEmbedding`:

1. Provider: `embedding_provider` -> `default_provider`
2. Model: `embedding_model` -> `providers.<provider>.default_model` -> `default_model`

Configured dimensions:

- If `embedding_dimensions[provider:model]` exists, AYB stores that expected dimension.
- On semantic queries, AYB fails fast if that configured dimension does not match the target vector column dimension.

Example:

```toml
[ai]
default_provider = "openai"
default_model = "gpt-4o-mini"
embedding_provider = "openai"
embedding_model = "text-embedding-3-small"
timeout = 30
max_retries = 2

[ai.embedding_dimensions]
"openai:text-embedding-3-small" = 1536

[ai.providers.openai]
api_key = "${OPENAI_API_KEY}"
default_model = "gpt-4o-mini"

[ai.providers.ollama]
base_url = "http://localhost:11434"
default_model = "nomic-embed-text"
```

## Admin vector indexes

All endpoints require admin auth (`Authorization: Bearer <admin-token>`).

- `POST /api/admin/vector/indexes`
- `GET /api/admin/vector/indexes`

### Create index

Request JSON:

- `table` (required)
- `column` (required, must be a vector column)
- `method` (required: `hnsw` or `ivfflat`)
- `metric` (required: `cosine`, `l2`, `inner_product`)
- `schema` (optional, defaults to `public` unless schema cache resolves table schema)
- `index_name` (optional, auto-generated as `idx_<table>_<column>_<method>`)
- `lists` (optional, used for `ivfflat`)

Example:

```bash
curl -X POST http://localhost:8090/api/admin/vector/indexes \
  -H "Authorization: Bearer $AYB_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "table": "documents",
    "column": "embedding",
    "method": "ivfflat",
    "metric": "cosine",
    "lists": 100
  }'
```

AYB executes `CREATE INDEX CONCURRENTLY IF NOT EXISTS ...`.

Response fields on success:

- `index_name`
- `method`
- `metric`
- `table`
- `column`

AYB returns `409` when another concurrent index build is already in progress.

### List indexes

`GET /api/admin/vector/indexes` returns:

```json
{
  "indexes": [
    {
      "name": "idx_documents_embedding_hnsw",
      "schema": "public",
      "table": "documents",
      "method": "hnsw",
      "definition": "CREATE INDEX ..."
    }
  ]
}
```

Only `hnsw` and `ivfflat` indexes are included.

## Query modes

All query modes are on collection list endpoints:

- `GET /api/collections/{table}`

Common vector params:

- `vector_column` (optional unless table has multiple vector columns)
- `distance` (optional; defaults to `cosine`; allowed: `cosine`, `l2`, `inner_product`)
- `perPage` controls result limit
- `filter` is applied before ranking

### 1) Nearest-neighbor (`nearest`)

Use a JSON array in the query string:

```bash
curl "http://localhost:8090/api/collections/documents?nearest=[0.12,0.34,0.56]&distance=cosine&perPage=10"
```

Validation enforced:

- `nearest` must be JSON array of numbers
- vector cannot be empty
- if column is `vector(N)`, query vector must have dimension `N`

Response rows include `_distance`.

### 2) Semantic query (`semantic_query`)

AYB embeds the text, then runs nearest-neighbor search.

```bash
curl "http://localhost:8090/api/collections/documents?semantic_query=find+similar+docs&distance=l2&perPage=10"
```

Error mapping includes:

- `400` when semantic search is not configured or pgvector is unavailable
- `500` when configured embedding dimensions do not match the target vector column, or when the provider returns an embedding with the wrong dimension
- `504` for embed timeout/cancel
- `429` for provider rate limit (`ProviderError` 429)
- `502` for provider auth/other upstream errors, including empty embedding results
- `503` for breaker-open conditions

### 3) Hybrid search (`search` + `semantic=true`)

Hybrid mode combines full-text and vector results using reciprocal rank fusion.

It requires both `search=<text>` and `semantic=true`. The hybrid path only activates when both are present.

It also requires at least one text column for the full-text side of the query, in addition to the target vector column.

```bash
curl "http://localhost:8090/api/collections/articles?search=postgres+indexing&semantic=true&distance=cosine&perPage=10"
```

Rules from `dispatchVectorPaths`:

- `semantic=true` cannot be combined with `nearest` or `semantic_query`
- `nearest` and `semantic_query` are mutually exclusive

Hybrid responses include fused fields:

- `_fts_rank`
- `_vector_distance`
- `_hybrid_score`

## Related guides

- [REST API](/guide/api-reference)
- [Admin Dashboard](/guide/admin-dashboard)
- [Comparison](/guide/comparison)
