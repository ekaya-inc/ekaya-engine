# PLAN: PII Radar Application

**Status:** IN PROGRESS
**Created:** 2026-03-03
**Parent:** DESIGN-wasm-application-platform.md
**Branch:** wt-ekaya-engine-wasm

## Purpose

PII Radar is the first WASM application built on the Ekaya Engine application platform. It scans customer datasource columns for personally identifiable information (PII), alerts admins, and enforces policies. It is also the driver for building out the WASM host infrastructure — only host capabilities that PII Radar needs get built.

## Design Decisions (from conversation)

These decisions were made during planning and apply to this feature and the broader WASM platform:

1. **Apps drive host development.** Host capabilities are built incrementally as apps need them. No speculative infrastructure.
2. **MCP tools for data access.** All data access goes through MCP tools (existing or new). Tools must be generic and usable by other MCP clients, not app-specific. PII detection logic lives in the WASM module, not in tools.
3. **Host-managed state.** The WASM module gets a read/write interface to a JSON blob. The host persists it with atomic CAS versioning in a JSONB field. The module is stateless between runs.
4. **Engine-level notifications.** Admin alerting is an engine capability (not app-specific). Apps call a host function to notify; the engine routes to email or other channels. This system is shared with native engine triggers.
5. **App-owned UI.** Each app gets a standard tile in the Project UI. Clicking the tile opens the app's own screen, likely rendered in an iframe.
6. **Execution modes.** Apps can run periodically (cron-like: hourly, daily) and on-demand (startup, schema refresh, manual trigger).

## Task Outline

Each task below becomes a TASK-*.md file when it's the next one to implement. Only the current task has concrete implementation details.

- [x] **Task 1: Prove WASM runtime** — Validate runtime choice, load a WASM module in ekaya-engine, invoke it, have it call a host function, get a response back. Pure round-trip proof.
  - TASK file: `TASK-app-pii-radar-prove-wasm-runtime.md` — **DONE**
  - Runtime: Extism Go SDK v1.7.1 + wazero. Guest: Rust + extism-pdk.
  - Code: `pkg/wasm/runtime.go`, `pkg/wasm/runtime_test.go`, `pkg/wasm/testdata/`

- [x] **Task 2: Host-managed state storage** — Implement the JSON blob storage interface. WASM module can read/write state. Host persists with CAS versioning.
  - TASK file: `TASK-app-pii-radar-state-storage.md` — **DONE**
  - Interface: `StateStore` with `Get`/`Set` + CAS versioning (`ErrVersionMismatch`).
  - Implementation: `MemoryStateStore` (in-memory, thread-safe). DB-backed is future.
  - Host functions: `state_get`/`state_set` via `StateHostFuncs(store, appID)`.
  - Code: `pkg/wasm/state.go`, `pkg/wasm/state_test.go`

- [x] **Task 3: MCP tool access from WASM** — WASM module can invoke an MCP tool via a host function. Proved round-trip with mock tools.
  - TASK file: `TASK-app-pii-radar-mcp-tool-access.md` — **DONE**
  - Interface: `ToolInvoker` with `InvokeTool(ctx, toolName, arguments) → (result, isError, err)`.
  - Host function: `tool_invoke` via `ToolInvokeHostFunc(invoker)`.
  - Test mock: `MapToolInvoker` (map of handler functions).
  - Auth strategy documented in DESIGN (host injects claims into context).
  - Code: `pkg/wasm/tools.go`, `pkg/wasm/tools_test.go`

- [x] **Task 4: Data access tooling** — Evaluated existing tools; `get_schema` (updated to structured JSON) + `query` tool cover PII Radar's needs. No new tools needed.
  - TASK file: `TASK-app-pii-radar-data-access-tooling.md` — **DONE**
  - Schema discovery: `get_schema` returns structured JSON with tables, columns, types, PK info.
  - Incremental reading: `query` tool with high-watermark SQL pattern (`WHERE pk > $hwm ORDER BY pk LIMIT 1000`).
  - Strategy documented in DESIGN-wasm-application-platform.md.

- [x] **Task 5: PII detection logic** — Implement PII detection in the WASM module. Regex patterns sourced from Presidio/PIISA, content scanning, high-watermark tracking.
  - TASK file: `TASK-app-pii-radar-pii-detection.md` — **DONE**
  - Library: `pii_detector` crate at `pkg/wasm/guests/pii_detector/` — pure Rust, no extism-pdk dependency.
  - Detection modes: column name matching + content regex scanning with validators (Luhn, SSN checksum).
  - Categories: `secrets`, `pii_identity`, `pii_contact`, `pii_financial`. Confidence: `high`, `medium`, `low`.
  - Guest module: `pkg/wasm/guests/pii_radar_guest/` — full scan lifecycle (load state → get schema → column scan → content scan → save state → report).
  - Tests: `pkg/wasm/pii_radar_test.go` — Go integration tests with mock MCP tools.
  - WASM binary: `pkg/wasm/testdata/pii_radar_guest.wasm` (~300KB).

- [ ] **Task 6: Periodic and on-demand execution** — Host can schedule a WASM app on a cron schedule. Host can trigger a WASM app on-demand (manual, startup, schema refresh).
  - TASK file: `TASK-app-pii-radar-execution.md`

- [ ] **Task 7: Engine notification system** — Engine-level capability to notify admins (email via ekaya-central). WASM apps call a host function; engine routes the notification.

- [ ] **Task 8: Policy model** — WASM module can read policies from state (e.g., "ignore PII in users.agent_data"). Start with alert-only enforcement. Policy CRUD may come from admin UI or MCP tools.

- [ ] **Task 9: UI integration** — App tile in Project UI sidebar. Iframe-based app screen showing scan results, PII findings, policy status, and scan history.

- [ ] **Task 10: App state display and management** — App screen shows current findings, allows admin to review/dismiss/acknowledge PII detections, configure policies.

## Notes

- Tasks will likely split or merge as we learn during implementation.
- Each task should be completable in a single session.
- The BRAINSTORM-wasm-application-platform.md file has unvetted ideas and reference material — useful context but not approved design.
- The BRAINSTORM-ekaya-engine-applications.md "Notes: PII Radar (App #1) implementation inputs" section has detection patterns and classification categories worth referencing when implementing Task 5.
