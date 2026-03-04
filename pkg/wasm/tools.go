package wasm

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolInvoker abstracts MCP tool invocation so the WASM host can call
// tool handlers without depending on the MCP library directly.
type ToolInvoker interface {
	// InvokeTool calls the named MCP tool with the given arguments.
	// Returns the tool's result as raw bytes (JSON text), whether the
	// tool itself reported an error (IsError), and any system-level error.
	InvokeTool(ctx context.Context, toolName string, arguments map[string]any) (result []byte, isError bool, err error)
}

// ToolInvokeHostFunc returns a single HostFunc that exposes tool_invoke
// to WASM modules. The module sends JSON with the tool name and arguments,
// and receives the tool result as JSON.
func ToolInvokeHostFunc(invoker ToolInvoker) HostFunc {
	return HostFunc{
		Name: "tool_invoke",
		Fn: func(ctx context.Context, input []byte) ([]byte, error) {
			var req toolInvokeRequest
			if err := json.Unmarshal(input, &req); err != nil {
				return json.Marshal(toolInvokeErrorResponse{
					Error: fmt.Sprintf("invalid tool_invoke input: %v", err),
				})
			}

			if req.Tool == "" {
				return json.Marshal(toolInvokeErrorResponse{
					Error: "tool name is required",
				})
			}

			result, isError, err := invoker.InvokeTool(ctx, req.Tool, req.Arguments)
			if err != nil {
				return json.Marshal(toolInvokeErrorResponse{
					Error: fmt.Sprintf("tool invocation failed: %v", err),
				})
			}

			return json.Marshal(toolInvokeResponse{
				Result:  json.RawMessage(result),
				IsError: isError,
			})
		},
	}
}

type toolInvokeRequest struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

type toolInvokeResponse struct {
	Result  json.RawMessage `json:"result"`
	IsError bool            `json:"is_error"`
}

type toolInvokeErrorResponse struct {
	Error string `json:"error"`
}

// MapToolInvoker is a simple ToolInvoker backed by a map of handler functions.
// Useful for testing without a full MCP server.
type MapToolInvoker struct {
	Handlers map[string]func(ctx context.Context, arguments map[string]any) (result []byte, isError bool, err error)
}

func (m *MapToolInvoker) InvokeTool(ctx context.Context, toolName string, arguments map[string]any) ([]byte, bool, error) {
	handler, ok := m.Handlers[toolName]
	if !ok {
		return nil, false, fmt.Errorf("unknown tool: %s", toolName)
	}
	return handler(ctx, arguments)
}
