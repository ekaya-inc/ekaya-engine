# TASK: Prove WASM Runtime in Ekaya Engine

**Status:** DONE
**Created:** 2026-03-03
**Parent:** PLAN-app-pii-radar.md (Task 1)
**Branch:** wt-ekaya-engine-wasm

## Context

Ekaya Engine is building a WASM application platform where sandboxed applications (starting with PII Radar — a sensitive data scanner) run inside the engine with controlled access to datasources, MCP tools, LLM capabilities, and storage. Apps are WASM modules that call host-provided functions; the host manages lifecycle, state, and security.

This is the first task: prove the WASM runtime works before building anything on top of it. The code written here is foundational — it will evolve into the production host, not be thrown away.

**Key design constraints** (from PLAN-app-pii-radar.md):
- Apps drive host development — only build what's needed, no speculative infrastructure.
- Data access will go through MCP tools exposed as host functions (future tasks).
- Host will manage app state as a JSON blob with CAS versioning (future tasks).
- Runtime must support host functions that can wrap arbitrary Go service calls.

**Reference material:**
- `plans/PLAN-app-pii-radar.md` — Feature-level scope and design decisions.
- `plans/BRAINSTORM-wasm-application-platform.md` — Unvetted prior research on Extism/wazero, IronClaw security patterns, host function design. Useful input for runtime evaluation but nothing here is approved.
- `plans/BRAINSTORM-ekaya-engine-applications.md` — Application ideas catalog. Not relevant to this task.

## Objective

Validate that a WASM module can be loaded into ekaya-engine, invoked, and can call a host function that returns a response. This is a pure round-trip proof — no PII logic, no real tools, just proving the runtime integration works.

## Scope

By the end of this task:
1. A runtime has been selected and justified (with findings documented in a new `DESIGN-wasm-application-platform.md` — vetted decisions only, not a copy of the brainstorm).
2. ekaya-engine can load a `.wasm` file from disk.
3. ekaya-engine can invoke an exported function in the WASM module.
4. The WASM module can call a host-provided function and receive a response.
5. There is a test that proves the round-trip works.
6. `make check` passes (existing code must not break).

## Steps

- [x] **Research and validate WASM runtime choice.** Evaluate Extism/wazero (proposed in brainstorm) and any credible alternatives. Criteria: pure Go (no CGO), active maintenance, host function support, resource limiting capability, production use. The BRAINSTORM file has prior research on Extism, wazero, and IronClaw — review it as input but verify claims independently. Document findings and recommendation in a new `plans/DESIGN-wasm-application-platform.md`. This DESIGN file contains only vetted decisions and open questions — not speculative content from the brainstorm.

- [x] **Add runtime dependency.** Add the selected WASM runtime library to `go.mod`.

- [x] **Create a minimal WASM guest module.** Write a trivial WASM module (use TinyGo, Rust, or hand-written WAT — whatever is simplest for a proof). The module should: export a function (e.g., `run`), call an imported host function (e.g., `host_echo(input) → output`), and return the host function's response. Place the guest module source and compiled `.wasm` binary in a location that makes sense for test fixtures (e.g., `pkg/wasm/testdata/`). Check in the compiled `.wasm` binary so tests don't require a WASM toolchain to run.

- [x] **Implement the host side.** In `pkg/wasm/` (or similar), write Go code that: loads the `.wasm` file, registers a host function (`host_echo`), invokes the module's exported function, and returns the result. This code is foundational — structure it for future extension (additional host functions, module lifecycle management), but don't build those capabilities yet.

- [x] **Write a test.** Test that: module loads successfully, exported function can be called, host function is invoked by the module, the response round-trips correctly. Use standard `go test` (no Docker/testcontainers needed for this task).

- [x] **Run `make check`.** Ensure linting, existing tests, and build all pass with the new code.

- [x] **Document any surprises or constraints** discovered during implementation (add to DESIGN file or as notes in this TASK file).

## Out of Scope

- PII detection logic
- MCP tool integration
- State storage
- Scheduling / lifecycle
- UI
- App packaging / distribution
- Security model (capabilities, resource limits) — note relevant findings during runtime research but do not implement yet

## Success Criteria

A passing test that demonstrates: Go host → loads WASM → calls exported function → WASM calls host function → host returns data → WASM returns final result → Go host receives it. `make check` passes.
