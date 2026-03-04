# TASK: Host-Managed State Storage for WASM Apps

**Status:** COMPLETE
**Created:** 2026-03-04
**Parent:** PLAN-app-pii-radar.md (Task 2)
**Branch:** wt-ekaya-engine-wasm

## Context

Ekaya Engine is building a WASM application platform where sandboxed apps run inside the engine. Task 1 proved the WASM runtime works — a Go host loads a WASM module via Extism, invokes an exported function, and the module can call host functions and receive responses (`pkg/wasm/runtime.go`).

WASM modules are stateless between runs. PII Radar (and future apps) need mutable state that persists across invocations — for example, high-watermarks tracking which rows have been scanned, or policies like "ignore PII in users.agent_data." The host manages this state and exposes it to the WASM module through host functions.

**Key design decision** (from PLAN-app-pii-radar.md):
> The WASM module gets a read/write interface to a JSON blob. The host persists it with atomic CAS versioning in a JSONB field. The module is stateless between runs.

**What exists today:**
- `pkg/wasm/runtime.go` — `Runtime.LoadAndRun()` loads WASM, registers host functions (`HostFunc` type: `func(ctx context.Context, input []byte) ([]byte, error)`), calls exports.
- `pkg/wasm/testdata/echo_guest.wasm` — Rust guest module that calls `host_echo` and returns the result.
- Extism Go SDK v1.7.1 + wazero runtime.

**Reference material:**
- `plans/PLAN-app-pii-radar.md` — Feature-level scope and design decisions.
- `plans/DESIGN-wasm-application-platform.md` — Vetted runtime decision (Extism + wazero).
- `plans/BRAINSTORM-wasm-application-platform.md` — Unvetted ideas. The IronClaw section describes a "fresh instance per execution" pattern with host-managed state, which is relevant.

## Objective

Add two host functions (`state_get` and `state_set`) that let a WASM module read and write a JSON blob. The host manages persistence with optimistic concurrency (CAS versioning) so concurrent or retried executions don't silently overwrite state.

## Scope

By the end of this task:
1. A `StateStore` interface exists in `pkg/wasm/` that the host uses to persist app state.
2. An in-memory implementation of `StateStore` exists (sufficient for testing; database-backed implementation is a future task).
3. Two host functions are available to WASM modules:
   - `state_get()` → returns the current JSON blob and version number.
   - `state_set(json_blob, expected_version)` → writes the blob if the version matches; returns error on version mismatch (CAS).
4. A guest module (new or extended) demonstrates reading and writing state.
5. Tests prove: initial empty state, write-read round-trip, CAS conflict detection (version mismatch fails), and sequential updates with incrementing versions.
6. `make check` passes.

## Steps

- [x] **Define the `StateStore` interface.** In `pkg/wasm/`, create an interface with `Get(ctx, appID) → (data []byte, version int64, err error)` and `Set(ctx, appID, data []byte, expectedVersion int64) → (newVersion int64, err error)`. The `Set` method returns an error if `expectedVersion` doesn't match the current version (CAS semantics).

- [x] **Implement `MemoryStateStore`.** An in-memory implementation using a map, suitable for tests. Thread-safe (mutex). A future task will add a PostgreSQL-backed implementation using JSONB.

- [x] **Register `state_get` and `state_set` host functions.** Extend `Runtime` (or create a builder/options pattern) so that state host functions are registered alongside any other host functions. The host functions should:
  - `state_get`: Return JSON like `{"data": <blob>, "version": <int>}`. If no state exists, return `{"data": null, "version": 0}`.
  - `state_set`: Accept JSON like `{"data": <blob>, "version": <expected_version>}`. Return `{"version": <new_version>}` on success or an error on version mismatch.

- [x] **Create or extend a guest module** that exercises state. The guest should: call `state_get` (expect empty/initial), call `state_set` with some data, call `state_get` again (expect the data back), and return a success/fail indicator. This can be a new Rust guest or an extension of the existing echo guest. Place source in `pkg/wasm/testdata/guest/` and check in the compiled `.wasm`.

- [x] **Write tests.** Test cases:
  - Read initial state (empty/null, version 0).
  - Write state, read it back (data matches, version increments).
  - CAS conflict: write with wrong version → error.
  - Sequential writes: version increments correctly (0 → 1 → 2).
  - Multiple app IDs: state is isolated per app.

- [x] **Run `make check`.** Ensure linting, existing tests, and build all pass.

- [x] **Document any design changes** in DESIGN-wasm-application-platform.md if the state interface deviates from expectations.

## Design Notes

- The `appID` parameter in `StateStore` scopes state per application. The host injects the correct `appID` when constructing host functions for a given app invocation — the WASM module doesn't need to know or provide its own ID.
- The in-memory store is intentionally simple. Database-backed persistence (JSONB column, likely on `engine_installed_apps` or a new table) will come when we integrate with the engine's existing app lifecycle.
- CAS versioning prevents a common class of bugs: if an app execution is retried or runs concurrently, stale writes fail loudly instead of silently overwriting newer state.

## Out of Scope

- Database-backed persistence (PostgreSQL JSONB) — future task
- MCP tool access from WASM
- PII detection logic
- Scheduling / lifecycle
- UI
- Security model

## Success Criteria

Tests prove: WASM module can `state_get` (initially empty), `state_set` with data, `state_get` the data back, and a `state_set` with wrong version fails with a CAS error. `make check` passes.
