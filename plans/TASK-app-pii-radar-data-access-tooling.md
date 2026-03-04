# TASK: Data Access Tooling for PII Scanning

**Status:** DONE
**Created:** 2026-03-04
**Parent:** PLAN-app-pii-radar.md (Task 4)
**Branch:** wt-ekaya-engine-wasm

## Context

Ekaya Engine is building a WASM application platform. PII Radar (the first app) needs to scan customer datasource columns for sensitive data. It will use MCP tools to access schema and data — all data access goes through generic MCP tools, not app-specific code.

**What PII Radar needs to do:**
1. Discover which tables and columns exist in the customer's datasource.
2. Read column data incrementally (using a high-watermark to avoid re-scanning rows already checked).
3. Store scan progress in state (via `state_get`/`state_set`, already implemented).

**Existing MCP tools for data access:**

| Tool | What it does | Useful for PII Radar? |
|------|-------------|----------------------|
| `get_schema` | Returns schema context with tables, columns, semantic annotations | Yes — discover tables/columns to scan. But returns enriched ontology data, not raw schema. |
| `sample` | Quick data preview: N rows from a table (default 10, max 100) | Partially — can read data, but no high-watermark/pagination. |
| `probe_column` | Column stats: distinct count, null rate, sample values | Partially — sample values could be scanned for PII, but limited data. |
| `query` | Execute read-only SQL SELECT | Yes — most flexible, can do `WHERE id > high_watermark ORDER BY id LIMIT N`. |
| `search_schema` | Full-text search across tables/columns | Not directly useful for scanning. |

**Gap analysis:**
- **Schema discovery:** `get_schema` returns ontology-enriched data. PII Radar needs a simpler list of tables and their columns (names, types). Could use `get_schema` as-is or create a lighter tool.
- **Incremental data reading:** No existing tool supports high-watermark pagination. The `query` tool is flexible enough (PII Radar can construct its own `SELECT ... WHERE id > ? LIMIT ?`), but the app would need to know the primary key / ordering column for each table. Alternatively, a new `scan_table` tool could handle pagination generically.

**What exists in the WASM runtime (`pkg/wasm/`):**
- `runtime.go` — `Runtime.LoadAndRun()` with `HostFunc` registration.
- `state.go` — `StateStore` interface, `MemoryStateStore`, `StateHostFuncs()`.
- `tools.go` — `ToolInvoker` interface, `ToolInvokeHostFunc()`, `MapToolInvoker` for tests.
- Guest modules in `pkg/wasm/testdata/` (Rust + extism-pdk).

**Reference material:**
- `plans/PLAN-app-pii-radar.md` — Feature-level scope and design decisions.
- `plans/DESIGN-wasm-application-platform.md` — Vetted runtime and auth decisions.
- `plans/BRAINSTORM-ekaya-engine-applications.md` — PII Radar implementation inputs section (lines 308-329) has detection pattern details.
- `pkg/mcp/tools/developer.go` — `sample` tool (lines 623-744) and `query` tool (lines 468-621).
- `pkg/mcp/tools/schema.go` — `get_schema` tool (lines 32-84).
- `pkg/mcp/tools/probe.go` — `probe_column` tool (lines 34-117).

## Objective

Ensure PII Radar has the MCP tools it needs to discover schema and read data incrementally. Evaluate whether existing tools suffice or if new/extended tools are needed. Implement any new tools required, keeping them generic for all MCP clients.

## Scope

By the end of this task:
1. PII Radar has a viable path to discover all tables and columns in a datasource.
2. PII Radar has a viable path to read data from a table incrementally (high-watermark / pagination).
3. Any new tools are registered in the MCP tool system and usable by any MCP client.
4. A test proves the new tools work (unit or integration test depending on whether DB access is needed).
5. `make check` passes.

## Steps

- [x] **Evaluate `get_schema` for schema discovery.**
- [x] **Evaluate `query` tool for incremental scanning.**
- [x] **Implement any new tools needed.** Updated `get_schema` to return structured JSON instead of text blob.
- [x] **Write tests for any new tools.**
- [x] **Document the data access strategy** in DESIGN-wasm-application-platform.md.
- [x] **Run `make check`.** All checks pass.

## Design Notes

- **Prefer existing tools.** If `get_schema` + `query` can serve PII Radar's needs, don't create new tools. New tools add maintenance burden and API surface.
- **SQL construction in WASM:** If PII Radar constructs SQL via the `query` tool, the SQL is still subject to the `query` tool's validation (SELECT-only, read-only). The WASM sandbox adds defense-in-depth. However, if this feels too permissive, a more constrained `scan_table` tool could limit what the app can do.
- **High-watermark strategy:** The simplest approach is to use a table's primary key as the ordering column. Tables without a usable PK might need a different strategy (e.g., scan by CTID in Postgres, or full-table-scan with OFFSET/LIMIT). This can be refined in future iterations.

## Out of Scope

- PII detection logic (Task 5 — uses the tools from this task)
- Production `ToolInvoker` wired to real MCP server
- Scheduling / lifecycle
- UI
- Notification system

## Success Criteria

A documented data access strategy for PII Radar with viable tool paths for schema discovery and incremental data reading. Any new tools have tests and pass `make check`. The strategy is recorded in DESIGN-wasm-application-platform.md.
