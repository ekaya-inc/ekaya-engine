package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type healthResult struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// RegisterHealthTool adds a health check tool to the MCP server.
// The tool returns the server status and version.
func RegisterHealthTool(s *server.MCPServer, version string) {
	tool := mcp.NewTool(
		"health",
		mcp.WithDescription("Returns server health status and version"),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := json.Marshal(healthResult{Status: "ok", Version: version})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal health result: %w", err)
		}
		return mcp.NewToolResultText(string(result)), nil
	})
}
