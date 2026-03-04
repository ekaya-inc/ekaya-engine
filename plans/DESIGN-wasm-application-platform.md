# DESIGN: WASM Application Platform

**Status:** IN PROGRESS
**Created:** 2026-03-03

## Runtime Decision: Extism Go SDK + wazero

**Choice:** [Extism Go SDK](https://github.com/extism/go-sdk) v1.7.1, which wraps [wazero](https://github.com/tetratelabs/wazero).

**Criteria evaluation:**

| Criterion | Result |
|-----------|--------|
| Pure Go (no CGO) | Yes — wazero is zero-dependency, pure Go |
| Active maintenance | Yes — Extism v1.7.1 (Mar 2025), wazero v1.11.0 (Dec 2024), 6K+ stars |
| Host function support | Yes — `NewHostFunctionWithStack` API with memory read/write helpers |
| Resource limiting | Partial — memory limits (MaxPages), wall-clock timeout (context + `WithCloseOnContextDone`). No instruction-level fuel metering. |
| Production use | Yes — wazero used by Arcjet (p50=10ms, p99=30ms in prod), 5,100+ dependents |

**Why Extism over raw wazero:** Extism provides plugin memory I/O helpers (`ReadString`, `WriteBytes`), host function binding with simpler API, HTTP allowlisting, multi-language PDK support (Rust, Go, JS), and compilation caching. These are capabilities we need for the production platform. Using Extism from the start avoids a rewrite.

**Why not CGO runtimes (wasmtime-go, wasmer-go):** Require CGO, complicate builds and cross-compilation. The only benefit is fuel metering (instruction-level CPU limiting), which wasmtime supports but wazero does not.

**Alternatives considered:**

| Runtime | Verdict |
|---------|---------|
| wazero direct (no Extism) | Viable but lower-level. Would need to build our own plugin ABI, memory helpers, and host function binding. Extism provides all of this. |
| wasmtime-go | Requires CGO. Has fuel metering. Rejected for build complexity. |
| wasmer-go | Requires CGO. Rejected. |

**There is no other pure-Go WASM runtime besides wazero.** This was confirmed across multiple sources.

## Resource Limiting Strategy

**What works today:**
- Memory: `Manifest.Memory.MaxPages` (each page = 64KB)
- Wall-clock time: Go `context.WithTimeout` + `wazero.NewRuntimeConfig().WithCloseOnContextDone(true)`

**Gap:** No instruction-level fuel metering. Mitigation is wall-clock timeout, which is sufficient for I/O-bound data applications. If CPU-bound workloads become a requirement, reassess CGO trade-off.

## Guest Module Language Strategy

**First-party apps:** Any compiled language (Rust, TinyGo, etc.) using the Extism PDK. WASM binary is pre-compiled and distributed — no toolchain needed on the engine.

**Future user-generated apps:** JavaScript via QuickJS-in-WASM (no compilation step required).

## Auth Context for Tool Invocation

When WASM apps invoke MCP tools via `tool_invoke`, the host must provide auth context so that `AcquireToolAccess` can authorize the call. The WASM module itself has no auth credentials — the host is responsible for injecting identity.

**Strategy:** The host injects auth claims into the `context.Context` passed to `ToolInvoker.InvokeTool()`. The production `ToolInvoker` implementation receives a pre-configured context containing:
- Project ID (which tenant the app belongs to)
- Role claims (what tools the app is allowed to call)
- Provenance markers (identifying the call as originating from a WASM app)

This matches how MCP tool handlers already receive auth context — via `auth.GetClaims(ctx)` in `AcquireToolAccess`. The WASM host simply sets up the context the same way the HTTP/JSON-RPC transport does.

**App-level permissions:** Each WASM app will have a declared set of tools it requires (part of the app manifest, future work). The host validates that the app is authorized to call the requested tool before forwarding to the `ToolInvoker`. This provides defense-in-depth beyond the role-based checks in `AcquireToolAccess`.

**Current state:** The `ToolInvoker` interface accepts a `context.Context`, so auth injection requires no interface changes. The mock `MapToolInvoker` used in tests bypasses auth entirely, which is correct for unit tests.

## Data Access Strategy for WASM Apps

WASM apps access customer data through generic MCP tools, not app-specific code. This keeps the tool surface reusable across all MCP clients.

**Schema discovery:** The `get_schema` tool returns structured JSON with all selected tables and their columns (name, data type, primary key, nullability) plus relationships. The response is structured JSON usable by both LLM clients and programmatic callers.

**Incremental data reading:** The `query` tool executes read-only SQL SELECT statements (hard cap: 1000 rows per call). Apps construct their own queries for incremental scanning using a high-watermark pattern:
```
SELECT col1, col2, ... FROM schema.table
WHERE pk_column > $high_watermark
ORDER BY pk_column
LIMIT 1000
```

The app discovers the primary key column from `get_schema` response (`is_primary_key: true`), then tracks the high-watermark in state via `state_get`/`state_set`.

**Security model:** The `query` tool validates that only SELECT/WITH statements are accepted. The WASM sandbox provides defense-in-depth — even if an app constructs malicious SQL, the query tool rejects non-SELECT statements. Tool access is further controlled by the auth context injected by the host (see Auth Context section above).

**Limitations:**
- Tables without a usable primary key require alternative scanning strategies (e.g., OFFSET/LIMIT or CTID). This can be refined in future iterations.
- Maximum 1000 rows per query call. Apps must batch their scanning across multiple `tool_invoke` calls.
- The `query` tool operates on the customer's datasource, not the engine's internal database.

## Open Questions

1. **Fuel metering** — Is wall-clock timeout sufficient for production, or will we need instruction-level metering? (Impacts CGO decision.)
2. **WASI requirements** — Current PoC uses WASI Preview 1. Will apps need filesystem or network access beyond host functions?
3. **Concurrent execution** — Extism v1.7.0+ supports concurrent plugin calls. What concurrency model for scheduled apps?
