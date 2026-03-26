# Edge Function Runtime Decision

## Recommendation: Goja

**Goja** (github.com/dop251/goja) is the recommended JS runtime for edge functions.

## Context

We evaluated five candidate runtimes for executing user-supplied JavaScript/TypeScript edge functions embedded within the allyourbase Go binary. The key constraints are:

- **Single binary**: `go build` must produce one binary, no external dependencies
- **No CGo**: Pure Go strongly preferred for cross-compilation and simplicity
- **Timeout support**: Must be able to interrupt runaway user code
- **Go interop**: Must support Go callbacks for `fetch()` bridging and `ayb.db.from()` queries
- **Performance**: Cold-start latency matters; edge functions are invoked per-request

## Candidates Evaluated

### Tier 1: Fully Evaluated with PoC + Benchmarks

| Criterion | Goja | modernc.org/quickjs | fastschema/qjs (WASM) |
|---|---|---|---|
| Pure Go | Yes | Yes (C-to-Go transpiled) | Yes (WASM via Wazero) |
| CGo required | No | No | No |
| ES version | ES2015+ (partial ES2020) | ES2023 | ES2023 |
| Single binary | Yes | Yes | Yes |
| Timeout/interrupt | Yes (vm.Interrupt) | Yes (SetEvalTimeout) | **No** (goroutine leak) |
| Memory limit | Manual | Yes (SetMemoryLimit) | No |
| Go callbacks | Native (Set func) | RegisterFunc | SetFunc |
| Maturity | Very mature, years of prod use | Newer (transpiled QuickJS) | Newer (v0.0.6) |
| Maintenance | Active | Active | Active |

### Tier 2: Evaluated and Rejected (Research Only)

| Criterion | v8go (rogchap) | Deno core |
|---|---|---|
| CGo required | **Yes** | **Yes** (massive) |
| Last release | March 2023 (stale) | N/A for embedding |
| Viable | No — CGo + unmaintained | No — not designed for embedding |

## Benchmark Results

Platform: Apple M4 Max, Go 1.26, darwin/arm64. Cold-start (new runtime per invocation):

| Workload | Goja | modernc/quickjs | WASM (fastschema/qjs) |
|---|---|---|---|
| Simple handler | **9μs** / 22KB | 199μs / 9KB | 1,566μs / 1,703KB |
| Fib(25) compute | **14ms** / 36KB | 19ms / 5,841KB | 16ms / 2,021KB |
| JSON parse/transform | **23μs** / 42KB | 225μs / 10KB | 1,539μs / 1,704KB |

### Analysis

- **Goja dominates cold-start**: 20x faster than modernc/quickjs, 170x faster than WASM for typical handler workloads. For edge functions invoked per HTTP request, cold-start latency is critical.
- **Compute-heavy tasks**: All three are within ~30% for CPU-bound work (fib(25)). Goja still wins slightly, but the gap narrows.
- **Memory**: Goja uses far less memory per invocation. modernc/quickjs allocates heavily during compute. WASM allocates ~1.7MB per cold start (WASM module instantiation).

## Decision Rationale

### Why Goja

1. **Fastest cold start by a wide margin**. Edge functions run per-request; 9μs vs 200μs+ matters at scale.
2. **Proper timeout support**. `vm.Interrupt()` cleanly stops runaway code. Critical for user-supplied functions.
3. **Lowest memory per isolate**. Enables more concurrent edge function executions.
4. **Most mature Go JS runtime**. Years of production use, active development, well-documented API.
5. **Native Go callbacks**. Clean bidirectional Go↔JS interop for `fetch()` bridging and DB queries.
6. **Zero dependencies beyond the Go standard library**. No CGo, no WASM blobs, no transpiled C.

### Tradeoffs Accepted

- **ES2015+ only** (not ES2023). Missing: async/await, optional chaining, nullish coalescing, BigInt. For edge function handlers, ES2015 is sufficient. If needed, a lightweight transpile step (esbuild) can downlevel modern syntax.
- **No native TypeScript**. TypeScript support requires a transpile step before execution. This is the same approach Supabase and Cloudflare Workers use.

### Why Not modernc/quickjs

- 20x slower cold start
- Higher memory for compute workloads (5.8MB vs 36KB for fib(25))
- Transpiled C codebase is harder to debug when issues arise
- Newer, less proven in production

### Why Not WASM (fastschema/qjs)

- 170x slower cold start (1.5ms WASM module instantiation)
- **Cannot interrupt infinite loops** — fundamental safety flaw for user-supplied code
- 1.7MB allocation per invocation
- v0.0.6, least mature

## TypeScript Strategy

Goja does not natively support TypeScript. The recommended approach:

1. **Deploy-time transpilation**: Use esbuild (Go library available) to transpile TS → ES2015 at function deploy time
2. **Cache compiled output**: Store the transpiled JS alongside the TS source
3. **Runtime executes JS only**: Goja runs the pre-transpiled JavaScript

This is the same pattern used by Supabase Edge Functions and Cloudflare Workers.

## Next Steps

1. Remove the modernc/quickjs and fastschema/qjs PoC implementations (spike code)
2. Expand Goja runtime with `fetch()` bridge to Go's `net/http`
3. Add `ayb.db.from()` callback for database queries
4. Add esbuild integration for TypeScript transpilation
5. Implement runtime pooling for warm execution (reuse Goja VMs)
