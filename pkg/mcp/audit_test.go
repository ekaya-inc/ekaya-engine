package mcp

import (
	"context"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
)

func TestAuditLoggerBuildEvent_UsesAgentNameForNamedAgentRequests(t *testing.T) {
	projectID := "550e8400-e29b-41d4-a716-446655440000"
	agentID := "8e0a4bd1-0bf6-4c45-9f72-44f256ab6af1"

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{
		ProjectID: projectID,
		Email:     "sales-bot",
	})

	claims, ok := auth.GetClaims(ctx)
	require.True(t, ok)
	claims.Subject = "agent:" + agentID

	logger := NewAuditLogger(nil, zap.NewNop())
	event := logger.buildEvent(ctx, &mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Name:      "list_approved_queries",
			Arguments: map[string]any{},
		},
	})

	require.NotNil(t, event.UserEmail)
	assert.Equal(t, "agent:"+agentID, event.UserID)
	assert.Equal(t, "sales-bot", *event.UserEmail)
}
