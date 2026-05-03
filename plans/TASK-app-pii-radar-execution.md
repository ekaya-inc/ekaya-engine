# TASK: Periodic and On-Demand Execution for WASM Apps

**Status:** PENDING
**Created:** 2026-03-05
**Parent:** PLAN-app-pii-radar.md (Task 6)
**Branch:** wt-ekaya-engine-wasm

## Context

Ekaya Engine is building a WASM application platform. PII Radar is the first app — it scans customer datasource columns for PII. Tasks 1-5 built the infrastructure (WASM runtime, state, tool invocation) and the PII detection logic. The WASM module works end-to-end in tests with mock tools, but there is **no way to actually run it** — no service, no scheduling, no API endpoint.

This task bridges the gap between "WASM module works in tests" and "WASM module runs in the engine."

**What exists today:**

WASM host (`pkg/wasm/`):
- `runtime.go` — `Runtime.LoadAndRun(ctx, wasmBytes, exportName, input, hostFuncs) ([]byte, error)`. Loads WASM via Extism, registers host functions, calls an export, returns output. Single-shot — no persistence between invocations.
- `state.go` — `StateStore` interface with `Get(ctx, appID) → (data, version, err)` and `Set(ctx, appID, data, expectedVersion) → (newVersion, err)`. Only `MemoryStateStore` exists (in-memory, for tests). **Production needs a database-backed implementation.**
- `tools.go` — `ToolInvoker` interface with `InvokeTool(ctx, toolName, arguments) → (result, isError, err)`. Only `MapToolInvoker` exists (map of handlers, for tests). **Production needs an implementation that calls real MCP tools.**

PII Radar guest module (`pkg/wasm/guests/pii_radar_guest/`):
- Rust WASM module compiled to `pkg/wasm/testdata/pii_radar_guest.wasm` (~300KB).
- Exports: `run` — takes JSON input `{"now": "2026-03-05T12:00:00Z"}`, calls `tool_invoke("get_schema")` and `tool_invoke("query")`, stores findings/watermarks via `state_get`/`state_set`, returns a JSON report.

Engine scheduling patterns (existing):
- `pkg/services/retention_service.go` — Background periodic task using `time.NewTicker(interval)` + goroutine. Runs across all projects. Started in `main.go` with a cancellable context.
- `pkg/services/workqueue/` — In-memory task queue with goroutine pool, concurrency strategies, retry with exponential backoff, task state tracking (`pending → running → completed/failed`), and UI update callbacks via `SetOnUpdate(func([]TaskSnapshot))`. Used for DAG orchestration.
- Shutdown pattern: cancellable contexts (`retentionCancel()`) and explicit `Shutdown(ctx)` methods in the graceful shutdown sequence in `main.go` (lines 650-683).

MCP tools:
- `pkg/mcp/tools/schema.go` — `get_schema` tool returns structured JSON with tables, columns, types, PK info.
- `pkg/mcp/tools/query.go` — `query` tool executes read-only SELECT SQL (max 1000 rows).
- Tool access is controlled by `AcquireToolAccess(ctx)` which reads auth claims from context.

**Design decisions (from PLAN and DESIGN files):**
- Apps can run periodically (cron-like: hourly, daily) and on-demand (startup, schema refresh, manual trigger).
- The host injects auth claims into `context.Context` for tool invocation. The WASM module has no auth credentials.
- The WASM module is stateless between runs. All state goes through `state_get`/`state_set`.
- MCP tools are generic — the same `get_schema` and `query` tools used by LLM clients are used by WASM apps.

**Reference material:**
- `plans/PLAN-app-pii-radar.md` — Feature-level scope and design decisions.
- `plans/DESIGN-wasm-application-platform.md` — Runtime, auth, data access strategy.
- `pkg/wasm/pii_radar_test.go` — Existing Go tests showing how to wire up `MapToolInvoker`, `MemoryStateStore`, and `Runtime.LoadAndRun` together.

## Objective

Build the service layer that can execute PII Radar (and future WASM apps) both periodically and on-demand. This is the first task that wires the WASM runtime into the engine's service layer and makes it callable from the outside (API endpoint for manual trigger, scheduler for periodic runs).

## Scope

By the end of this task:

1. A `WasmAppService` (or similar) exists that orchestrates running a WASM app: loads the WASM binary, wires up host functions (state + tools), calls `run`, and returns the report.
2. A production `ToolInvoker` implementation exists that routes `tool_invoke` calls from WASM to real MCP tool handlers.
3. A database-backed `StateStore` implementation exists that persists WASM app state in the engine's PostgreSQL database with CAS versioning.
4. A periodic scheduler runs PII Radar on a configurable interval (e.g., daily) for each project that has it enabled.
5. An API endpoint allows triggering PII Radar on-demand (manual run).
6. The scheduler follows the engine's existing patterns: cancellable context, graceful shutdown, wired in `main.go`.
7. Tests prove the service layer works (unit tests with mocks, integration test if feasible).
8. `make check` passes.

## Steps

### Step 1: Database-backed StateStore

The `MemoryStateStore` is only for tests. Production needs state persisted in PostgreSQL.

- [ ] **Create a database migration** adding a table for WASM app state. Suggested schema:
  ```sql
  CREATE TABLE wasm_app_state (
      project_id UUID NOT NULL,
      app_id TEXT NOT NULL,
      data JSONB NOT NULL DEFAULT '{}',
      version BIGINT NOT NULL DEFAULT 0,
      updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      PRIMARY KEY (project_id, app_id)
  );
  ```
  The `version` column implements CAS: `UPDATE ... SET data = $1, version = version + 1 WHERE project_id = $2 AND app_id = $3 AND version = $4`. If no row updated, return `ErrVersionMismatch`.

- [ ] **Create a repository** (`pkg/repositories/wasm_app_state_repository.go`) implementing the `StateStore` interface using the tenant-scoped DB. Follow existing repository patterns (e.g., `pkg/repositories/` — use `database.GetTenantScope(ctx)` for tenant isolation). The repository should:
  - `Get(ctx, appID)` → SELECT data, version. If no row exists, return `nil, 0, nil` (empty state, version 0).
  - `Set(ctx, appID, data, expectedVersion)` → If version is 0, INSERT (first write). Otherwise UPDATE with CAS check. Return new version or `ErrVersionMismatch`.

- [ ] **Test the repository** with an integration test (testcontainers).

### Step 2: Production ToolInvoker

The `MapToolInvoker` is for tests. Production needs an invoker that calls real MCP tool handlers.

- [ ] **Create a production `ToolInvoker`** (`pkg/wasm/mcp_tool_invoker.go` or `pkg/services/wasm_tool_invoker.go`) that:
  - Accepts a reference to the MCP server's tool registry (or the individual tool handler functions).
  - Given a `toolName` and `arguments`, looks up the tool and calls its handler.
  - The `context.Context` passed in already has auth claims injected by the `WasmAppService` (see Step 3). The invoker doesn't need to handle auth itself — it just passes the context through.
  - Returns the tool's JSON result, isError flag, and any error.

- [ ] **Evaluate the simplest integration path.** The MCP tools are registered on the MCP server (`mcpServer.MCP()`). Explore how tool handlers are invoked internally — there may be an internal dispatch method that takes a tool name and arguments. If so, the production invoker can delegate to that. If not, consider a thin adapter.

- [ ] **Test with a real MCP tool** in an integration test (e.g., call `get_schema` against a test database container).

### Step 3: WasmAppService

This is the core orchestration service. It ties together: WASM runtime + StateStore + ToolInvoker + WASM binary.

- [ ] **Create `WasmAppService`** (`pkg/services/wasm_app_service.go`) with a method like:
  ```go
  func (s *WasmAppService) RunApp(ctx context.Context, projectID uuid.UUID, appID string) (*AppRunResult, error)
  ```
  This method:
  1. Loads the WASM binary for the app. For now, PII Radar's `.wasm` is embedded or loaded from a known path. Future: app registry.
  2. Creates host functions: `StateHostFuncs(dbStateStore, appID)` + `ToolInvokeHostFunc(prodToolInvoker)`.
  3. Injects auth context into `ctx` — project ID, app-level role claims — so that tool invocations are authorized.
  4. Constructs input JSON (e.g., `{"now": "2026-03-05T12:00:00Z"}`).
  5. Calls `Runtime.LoadAndRun(ctx, wasmBytes, "run", input, hostFuncs)`.
  6. Parses and returns the report.

- [ ] **Handle errors gracefully.** If the WASM module panics, returns an error report, or times out (context deadline), the service should log the failure and not crash the engine.

- [ ] **Add a context timeout.** Each app run should have a maximum execution time (e.g., 5 minutes). Use `context.WithTimeout`.

- [ ] **Unit test** with `MemoryStateStore` and `MapToolInvoker` (same pattern as existing `pii_radar_test.go`, but going through the service layer).

### Step 4: API endpoint for on-demand execution

- [ ] **Create a handler** (`pkg/handlers/wasm_app_handler.go`) with a route like:
  ```
  POST /api/projects/{projectId}/apps/{appId}/run
  ```
  This is a protected route (authMiddleware + tenantMiddleware). It calls `WasmAppService.RunApp()` and returns the report JSON.

- [ ] **Register the route** in `main.go` alongside other handlers.

- [ ] **Consider async execution.** If the WASM app takes a long time, the API should probably return immediately with a "started" response and report results asynchronously (via WebSocket or polling). For now, synchronous is fine — PII Radar scans 1000 rows per invocation, which should be fast. Add a TODO for async if needed.

### Step 5: Periodic scheduler

- [ ] **Create a scheduler** that periodically runs WASM apps. Follow the retention service pattern:
  - A `RunScheduler(ctx context.Context, interval time.Duration)` method that spawns a goroutine with `time.NewTicker`.
  - On each tick, iterate all projects (or projects with the app enabled) and call `WasmAppService.RunApp()` for each.
  - Respect context cancellation for shutdown.

- [ ] **Wire into `main.go`.** Same pattern as retention service:
  ```go
  wasmSchedulerCtx, wasmSchedulerCancel := context.WithCancel(ctx)
  wasmAppService.RunScheduler(wasmSchedulerCtx, 1*time.Hour)
  // ... in shutdown:
  wasmSchedulerCancel()
  ```

- [ ] **Make the interval configurable.** For PII Radar, daily is fine for production. Use a shorter interval for development/testing.

- [ ] **Handle per-project errors.** If one project's scan fails, log the error and continue to the next project. Don't let one failure block all projects.

### Step 6: Test and validate

- [ ] **Unit tests** for `WasmAppService` using mocks (MemoryStateStore, MapToolInvoker).
- [ ] **Integration test** (if feasible): spin up a test database, register real MCP tools, run PII Radar through the service layer, verify findings are persisted in the database.
- [ ] **Run `make check`.** Ensure linting, existing tests, and build all pass.

## Design Notes

- **WASM binary distribution:** For now, the PII Radar `.wasm` binary can be embedded in the Go binary using `//go:embed` or loaded from a file path. Future work will add an app registry/manifest. Don't over-engineer this.
- **App registry:** This task doesn't build a full app registry. It hardcodes PII Radar as the only app. Future tasks can add a registry when more apps exist.
- **Project iteration for scheduler:** The scheduler needs to iterate projects. Use existing `ProjectService.List()` or similar. Only run for projects that have a connected datasource (no point scanning if there's nothing to scan).
- **Concurrency:** Run one project at a time (serial) in the scheduler for simplicity. Parallel execution across projects is a future optimization.
- **Idempotency:** PII Radar is naturally idempotent — it uses high-watermarks to avoid re-scanning rows. Running it multiple times is safe.
- **Logging:** Use structured logging (`zap.Logger`) consistent with the rest of the engine. Log app name, project ID, duration, finding count per run.
- **Storing the report:** The WASM module persists its findings in state (via `state_set`). The report returned from `run` is a summary. Consider whether to also store the report separately (e.g., for scan history in the UI). For now, returning it from the API and logging it is sufficient. Scan history can be added in a UI task.

## Out of Scope

- Full app registry / manifest system (future)
- Admin notification on findings (Task 7)
- Policy enforcement (Task 8)
- UI integration (Tasks 9-10)
- Async execution / WebSocket progress (future optimization)
- Parallel multi-project execution (future optimization)
- App enable/disable per project (hardcode PII Radar as always-on for now)

## Success Criteria

PII Radar can be triggered via an API endpoint and runs on a periodic schedule. It executes against real customer datasources using real MCP tools, persists findings in the database via a production StateStore, and returns a structured report. The scheduler runs across all projects, handles errors gracefully, and shuts down cleanly. `make check` passes.
