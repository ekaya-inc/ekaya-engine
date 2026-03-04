# TASK: MCP Tool Access from WASM

**Status:** COMPLETE
**Created:** 2026-03-04
**Parent:** PLAN-app-pii-radar.md (Task 3)
**Branch:** wt-ekaya-engine-wasm

## Context

Ekaya Engine is building a WASM application platform where sandboxed apps run inside the engine. PII Radar (the first app) will need to call MCP tools to access datasource schema and column data. This task proves that a WASM module can invoke MCP tool handlers through the host.

**Key design decision** (from PLAN-app-pii-radar.md):
> All data access goes through MCP tools (existing or new). Tools must be generic and usable by other MCP clients, not app-specific. PII detection logic lives in the WASM module, not in tools.

**What exists today:**

WASM runtime (`pkg/wasm/`):
- `runtime.go` — `Runtime.LoadAndRun()` loads WASM via Extism, registers `HostFunc` list (each is `func(ctx context.Context, input []byte) ([]byte, error)`), calls exports.
- `state.go` — `StateStore` interface, `MemoryStateStore`, `StateHostFuncs()` returns `[]HostFunc` for `state_get`/`state_set`.
- Guest modules in `pkg/wasm/testdata/` (Rust + extism-pdk, compiled `.wasm` checked in).

MCP tool system (`pkg/mcp/`):
- Tools are registered on `*server.MCPServer` via `s.AddTool(tool, handler)`.
- Handler signature: `func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)`.
- `mcp.CallToolRequest` has `Params.Name` (string) and `Params.Arguments` (map[string]any).
- `mcp.CallToolResult` has `Content` (text/JSON) and `IsError` (bool).
- Auth context flows via `AcquireToolAccess(ctx, deps, toolName)` which checks roles, acquires tenant-scoped DB connection, injects provenance. Returns `(projectID, tenantCtx, cleanup, err)`.
- MCP library: `github.com/mark3labs/mcp-go v0.43.2`.
- There is **no direct Go programmatic tool invocation API** — tools are invoked via HTTP/JSON-RPC or by calling the handler function directly.
- Simple example: `pkg/mcp/tools/health.go:42-77` — `health` tool returns system status as JSON.
- Tool access control: `pkg/mcp/tools/access.go` — role-based filtering (agent/admin/data/user).
- Error pattern: actionable errors return as `*mcp.CallToolResult` with `IsError=true`; system errors return as Go `error`.

**Reference material:**
- `plans/PLAN-app-pii-radar.md` — Feature-level scope and design decisions.
- `plans/DESIGN-wasm-application-platform.md` — Vetted runtime decision.

## Objective

Add a `tool_invoke` host function that lets a WASM module call any MCP tool handler by name with JSON arguments, and receive the tool's result. The host bridges between the WASM `HostFunc` interface and the MCP `ToolHandlerFunc` signature.

## Scope

By the end of this task:
1. A `ToolInvoker` interface exists that the host uses to call MCP tool handlers programmatically.
2. A `tool_invoke` host function is available to WASM modules — it accepts a tool name and JSON arguments, calls the tool handler, and returns the result.
3. A test proves the round-trip: WASM module calls `tool_invoke` → host invokes a tool handler → result returns to the module.
4. Auth context propagation is addressed (at minimum, documented how it will work; ideally, the host can inject auth context into the tool call).
5. `make check` passes.

## Steps

- [x] **Define the `ToolInvoker` interface.** In `pkg/wasm/`, create an interface that abstracts MCP tool invocation:
  ```
  type ToolInvoker interface {
      InvokeTool(ctx context.Context, toolName string, arguments map[string]any) (result []byte, isError bool, err error)
  }
  ```
  This decouples the WASM host from the MCP server internals. The implementation will call the actual MCP tool handler function.

- [x] **Implement a test/mock `ToolInvoker`.** For unit tests, create a simple implementation that maps tool names to handler functions. This avoids needing a full MCP server in tests.

- [x] **Implement the `tool_invoke` host function.** Create a `ToolInvokeHostFunc(invoker ToolInvoker) HostFunc` function (similar pattern to `StateHostFuncs`). The host function should:
  - Accept JSON input: `{"tool": "<name>", "arguments": {<key>: <value>, ...}}`
  - Call `invoker.InvokeTool(ctx, toolName, arguments)`
  - Return JSON output: `{"result": <tool_output>, "is_error": <bool>}` on success, or `{"error": "<message>"}` on system failure.

- [x] **Create or extend a guest module** that calls `tool_invoke`. The guest should call a mock tool (e.g., an "echo_tool" registered in the test) and return the result. Place source in `pkg/wasm/testdata/guest/` and check in the compiled `.wasm`.

- [x] **Write tests.** Test cases:
  - Successful tool invocation: guest calls `tool_invoke("echo_tool", {"msg": "hello"})` → receives tool result.
  - Tool returns error result: guest calls a tool that returns `IsError=true` → guest receives `is_error: true` in response.
  - Unknown tool: guest calls `tool_invoke("nonexistent", {})` → receives error.
  - Multiple host functions together: guest uses `tool_invoke`, `state_get`, and `state_set` in the same invocation (proves host functions compose correctly).

- [x] **Document auth context strategy.** When WASM apps invoke tools in production, they need auth context (project ID, roles) for `AcquireToolAccess`. Document in DESIGN-wasm-application-platform.md how the host will inject this — likely by setting auth claims on the context passed to `ToolInvoker.InvokeTool()`. Implementation of actual auth injection is out of scope for this task.

- [x] **Run `make check`.** Ensure linting, existing tests, and build all pass.

## Design Notes

- The `ToolInvoker` interface intentionally mirrors the MCP handler signature but returns raw bytes instead of `*mcp.CallToolResult`. This keeps the WASM package decoupled from the MCP library — the implementation bridges between the two.
- In production, the `ToolInvoker` implementation will wrap the MCP server's internal tool dispatch, injecting auth context and auditing the call. That integration is a future task when we wire up the WASM host to the engine's dependency injection.
- The WASM module receives tool results as JSON text (matching what MCP tool handlers return via `mcp.NewToolResultText`). The module parses the JSON in its own logic.

## Out of Scope

- Production `ToolInvoker` implementation wired to the real MCP server
- Auth context injection into tool calls (document strategy only)
- PII detection logic
- Data access tools (Task 4)
- Scheduling / lifecycle
- UI

## Success Criteria

Tests prove: WASM module calls `tool_invoke` host function → host calls a mock tool handler → tool result returns to the module as JSON. Host functions compose (tool_invoke + state_get/state_set work together). `make check` passes.
