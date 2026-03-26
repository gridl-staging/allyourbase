<!-- audited 2026-03-20 -->

# Performance

This page is the canonical benchmark reference for Allyourbase.

The measurements below are baseline numbers captured from local benchmark runs and should be treated as best-case guidance, not production guarantees.

## Methodology

These API benchmarks come from `internal/api/benchmark_test.go` and run in-process with Go `httptest` against a local PostgreSQL instance.

- The benchmark harness uses `setupBenchServer()` to reset schema state, seed rows, and create the server.
- Requests are executed by `benchRequest()` with `httptest.NewRequest`, `httptest.NewRecorder`, and `srv.Router().ServeHTTP`.
- No real network sockets, TLS, load balancer, or cross-host latency are involved.
- Benchmark functions covered: `BenchmarkHealthCheck`, `BenchmarkReadSingle`, `BenchmarkListDefault`, `BenchmarkListFiltered`, `BenchmarkListLargeTable`, `BenchmarkCreate`, `BenchmarkUpdate`, `BenchmarkDelete`, `BenchmarkBatchCreate`.

Command used from the repository root:

```bash
go test ./internal/api -tags=integration -bench=. -benchmem -benchtime=3s
```

## Hardware and Software Environment

Measured on 2026-02-17:

- CPU: Intel i7-9750H (6C/12T @ 2.60GHz)
- OS: macOS
- Database: PostgreSQL 15.14 (local)

## Benchmark Tables

### Binary Size

| Build | Size |
| --- | --- |
| Default (`go build`) | 36 MB |
| Stripped (`-ldflags="-s -w"`) | 25 MB |

Includes the embedded admin dashboard, migration tools, and MCP server.

### Startup Time

| Mode | Time |
| --- | --- |
| External PostgreSQL | ~310 ms |

Measured from process start until the health endpoint returns HTTP 200.

### Memory Usage (RSS)

| State | RSS |
| --- | --- |
| After startup (idle) | 20.5 MB |
| After 600 requests (health + schema) | 20.5 MB |

No memory growth was observed in this run.

### API Benchmarks (httptest + local PostgreSQL)

| Benchmark | ops/3s | ns/op | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| HealthCheck | 704,338 | 4,392 | 7,792 | 42 |
| ReadSingle (100 rows) | 73,311 | 47,786 | 10,198 | 96 |
| ListDefault (100 rows, 20/page) | 30,848 | 122,002 | 36,333 | 608 |
| ListFiltered (1K rows, filter+50/page) | 16,765 | 213,918 | 77,380 | 1,419 |
| ListLargeTable (10K rows, 100/page) | 8,576 | 397,538 | 143,761 | 2,698 |
| Create | 7,776 | 533,087 | 14,096 | 163 |
| Update | 8,252 | 444,788 | 13,713 | 150 |
| Delete | 10,000 | 404,463 | 10,239 | 89 |
| BatchCreate (50 ops) | 382 | 15,687,150 | 272,462 | 5,665 |

## Interpretation and Caveats

Throughput summary from this baseline:

- Health check: ~228K req/s (baseline, no DB)
- Single record read: ~21K req/s
- List (default): ~8.2K req/s
- Create: ~1.9K req/s (includes DB write + fsync)
- Batch create (50 items): ~64 req/s, roughly ~3.2K items/s

These numbers are useful for relative comparisons and regression detection. They are not SLAs and do not represent p99 latency, internet-facing throughput, or full production topology behavior.

For production planning, include network hops, TLS, proxies, connection pooling, dataset growth, query shape, and deployment-specific limits.
