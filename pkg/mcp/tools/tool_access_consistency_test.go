package tools

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

func TestToolAccessConsistency_AgentListingMatchesCalling(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		services.ToolGroupAgentTools: {Enabled: true},
	})

	installedApps := newMockInstalledAppService(models.AppIDAIAgents)
	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		nil,
		nil,
		installedApps,
		"http://localhost",
		zap.NewNop(),
	)

	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: installedApps,
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent:" + uuid.New().String()
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := NewToolFilter(filterDeps)(ctx, createTestTools())
	assertContainsTool(t, filteredTools, "list_approved_queries")

	queryDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: installedApps,
		},
	}

	_, tenantCtx, cleanup, err := AcquireToolAccess(ctx, queryDeps, "list_approved_queries")
	requireNoToolAccessError(t, err)
	if cleanup != nil {
		defer cleanup()
	}
	if tenantCtx == nil {
		t.Fatal("expected tenant context for allowed agent tool")
	}
}

func TestToolAccessConsistency_UserListingMatchesCalling(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		services.ToolGroupTools: {AddRequestTools: true},
	})

	installedApps := newMockInstalledAppService(models.AppIDAIDataLiaison)
	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		nil,
		nil,
		installedApps,
		"http://localhost",
		zap.NewNop(),
	)

	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: installedApps,
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	claims := &auth.Claims{
		ProjectID: projectID.String(),
		Roles:     []string{models.RoleUser},
	}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := NewToolFilter(filterDeps)(ctx, createTestTools())
	assertContainsTool(t, filteredTools, "list_approved_queries")

	queryDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: installedApps,
		},
	}

	_, tenantCtx, cleanup, err := AcquireToolAccess(ctx, queryDeps, "list_approved_queries")
	requireNoToolAccessError(t, err)
	if cleanup != nil {
		defer cleanup()
	}
	if tenantCtx == nil {
		t.Fatal("expected tenant context for allowed user tool")
	}
}

func TestToolAccessConsistency_AppOwnershipFilteringDeniesMissingApp(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		services.ToolGroupTools: {AddRequestTools: true},
	})

	installedApps := newMockInstalledAppService()
	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		nil,
		nil,
		installedApps,
		"http://localhost",
		zap.NewNop(),
	)

	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: installedApps,
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	claims := &auth.Claims{
		ProjectID: projectID.String(),
		Roles:     []string{models.RoleUser},
	}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := NewToolFilter(filterDeps)(ctx, createTestTools())
	if containsTool(filteredTools, "list_approved_queries") {
		t.Fatal("list_approved_queries should be hidden when AI Data Liaison is not installed")
	}

	queryDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: installedApps,
		},
	}

	_, _, cleanup, err := AcquireToolAccess(ctx, queryDeps, "list_approved_queries")
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil {
		t.Fatal("expected missing app installation to block list_approved_queries")
	}
	if accessErr, ok := err.(*ToolAccessError); !ok || accessErr.Code != "app_not_installed" {
		t.Fatalf("expected app_not_installed tool access error, got %v", err)
	}
}

func assertContainsTool(t *testing.T, tools []mcp.Tool, name string) {
	t.Helper()
	if !containsTool(tools, name) {
		t.Fatalf("expected %s to be present in filtered tool list: %v", name, toolNames(tools))
	}
}

func requireNoToolAccessError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected tool access to succeed, got %v", err)
	}
}
