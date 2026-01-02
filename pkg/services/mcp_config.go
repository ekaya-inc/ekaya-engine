package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ToolGroupApprovedQueries is the identifier for the pre-approved queries tool group.
const ToolGroupApprovedQueries = "approved_queries"

// validToolGroups defines the known tool group identifiers for validation.
// UI metadata (names, descriptions, warnings) is defined in the frontend.
var validToolGroups = map[string]bool{
	"developer":              true,
	ToolGroupApprovedQueries: true,
}

// MCPConfigResponse is the API response format for MCP configuration.
// Returns only configuration state; UI strings are defined in the frontend.
type MCPConfigResponse struct {
	ServerURL  string                             `json:"serverUrl"`
	ToolGroups map[string]*models.ToolGroupConfig `json:"toolGroups"`
}

// UpdateMCPConfigRequest is the API request format for updating MCP configuration.
type UpdateMCPConfigRequest struct {
	ToolGroups map[string]*models.ToolGroupConfig `json:"toolGroups"`
}

// MCPConfigService orchestrates MCP configuration management.
type MCPConfigService interface {
	// Get retrieves the MCP configuration for a project, creating default if not exists.
	Get(ctx context.Context, projectID uuid.UUID) (*MCPConfigResponse, error)

	// Update updates the MCP configuration for a project.
	Update(ctx context.Context, projectID uuid.UUID, req *UpdateMCPConfigRequest) (*MCPConfigResponse, error)

	// IsToolGroupEnabled checks if a specific tool group is enabled for a project.
	IsToolGroupEnabled(ctx context.Context, projectID uuid.UUID, toolGroup string) (bool, error)

	// GetToolGroupConfig returns the full configuration for a tool group.
	// Returns nil if the tool group doesn't exist or has no configuration.
	GetToolGroupConfig(ctx context.Context, projectID uuid.UUID, toolGroup string) (*models.ToolGroupConfig, error)

	// ShouldShowApprovedQueriesTools determines if approved queries tools should appear.
	// Returns true only if the approved_queries tool group is enabled AND there are enabled queries.
	ShouldShowApprovedQueriesTools(ctx context.Context, projectID uuid.UUID) (bool, error)
}

type mcpConfigService struct {
	configRepo     repositories.MCPConfigRepository
	queryService   QueryService
	projectService ProjectService
	baseURL        string
	logger         *zap.Logger
}

// NewMCPConfigService creates a new MCP config service with dependencies.
func NewMCPConfigService(
	configRepo repositories.MCPConfigRepository,
	queryService QueryService,
	projectService ProjectService,
	baseURL string,
	logger *zap.Logger,
) MCPConfigService {
	return &mcpConfigService{
		configRepo:     configRepo,
		queryService:   queryService,
		projectService: projectService,
		baseURL:        baseURL,
		logger:         logger,
	}
}

// Get retrieves the MCP configuration for a project.
func (s *mcpConfigService) Get(ctx context.Context, projectID uuid.UUID) (*MCPConfigResponse, error) {
	config, err := s.configRepo.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP config: %w", err)
	}

	// If no config exists, use defaults
	if config == nil {
		config = models.DefaultMCPConfig(projectID)
	}

	return s.buildResponse(ctx, projectID, config), nil
}

// Update updates the MCP configuration for a project.
func (s *mcpConfigService) Update(ctx context.Context, projectID uuid.UUID, req *UpdateMCPConfigRequest) (*MCPConfigResponse, error) {
	// Get existing config or create default
	config, err := s.configRepo.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP config: %w", err)
	}
	if config == nil {
		config = models.DefaultMCPConfig(projectID)
	}

	// Merge updates - only update known tool groups
	for groupName, groupConfig := range req.ToolGroups {
		if validToolGroups[groupName] {
			if config.ToolGroups == nil {
				config.ToolGroups = make(map[string]*models.ToolGroupConfig)
			}
			config.ToolGroups[groupName] = groupConfig
		}
	}

	// Persist changes
	if err := s.configRepo.Upsert(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to save MCP config: %w", err)
	}

	s.logger.Info("Updated MCP config",
		zap.String("project_id", projectID.String()),
	)

	return s.buildResponse(ctx, projectID, config), nil
}

// IsToolGroupEnabled checks if a specific tool group is enabled for a project.
func (s *mcpConfigService) IsToolGroupEnabled(ctx context.Context, projectID uuid.UUID, toolGroup string) (bool, error) {
	config, err := s.configRepo.Get(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("failed to get MCP config: %w", err)
	}

	// If no config exists, use defaults (all tool groups disabled)
	if config == nil {
		return false, nil
	}

	if groupConfig, ok := config.ToolGroups[toolGroup]; ok {
		return groupConfig.Enabled, nil
	}

	return false, nil
}

// GetToolGroupConfig returns the full configuration for a tool group.
func (s *mcpConfigService) GetToolGroupConfig(ctx context.Context, projectID uuid.UUID, toolGroup string) (*models.ToolGroupConfig, error) {
	config, err := s.configRepo.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP config: %w", err)
	}

	// If no config exists, return nil (use defaults)
	if config == nil {
		return nil, nil
	}

	if groupConfig, ok := config.ToolGroups[toolGroup]; ok {
		return groupConfig, nil
	}

	return nil, nil
}

// ShouldShowApprovedQueriesTools determines if approved queries tools should appear.
// Returns true only if the approved_queries tool group is enabled AND there are enabled queries.
func (s *mcpConfigService) ShouldShowApprovedQueriesTools(ctx context.Context, projectID uuid.UUID) (bool, error) {
	// Check if approved_queries tool group is enabled
	enabled, err := s.IsToolGroupEnabled(ctx, projectID, ToolGroupApprovedQueries)
	if err != nil {
		return false, fmt.Errorf("failed to check tool group: %w", err)
	}
	if !enabled {
		return false, nil
	}

	// Check if any enabled queries exist (uses efficient LIMIT 1 query)
	hasQueries, err := s.hasEnabledQueries(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("failed to check enabled queries: %w", err)
	}

	return hasQueries, nil
}

// hasEnabledQueries checks if the project has any enabled queries.
// Uses an efficient LIMIT 1 query instead of fetching all enabled queries.
func (s *mcpConfigService) hasEnabledQueries(ctx context.Context, projectID uuid.UUID) (bool, error) {
	// Get default datasource for the project
	dsID, err := s.projectService.GetDefaultDatasourceID(ctx, projectID)
	if err != nil {
		// No datasource means no queries can exist
		return false, nil
	}

	// Efficiently check if any enabled queries exist (uses LIMIT 1)
	return s.queryService.HasEnabledQueries(ctx, projectID, dsID)
}

// buildResponse creates the API response format from the model.
// Returns only configuration state; UI metadata is defined in the frontend.
func (s *mcpConfigService) buildResponse(ctx context.Context, projectID uuid.UUID, config *models.MCPConfig) *MCPConfigResponse {
	toolGroups := make(map[string]*models.ToolGroupConfig)

	// Check if approved_queries should actually be enabled (requires enabled queries)
	approvedQueriesActive, err := s.hasEnabledQueries(ctx, projectID)
	if err != nil {
		s.logger.Error("failed to check enabled queries, defaulting to disabled",
			zap.String("project_id", projectID.String()),
			zap.Error(err),
		)
		approvedQueriesActive = false
	}

	// Include all valid tool groups in response
	for groupName := range validToolGroups {
		groupConfig := &models.ToolGroupConfig{}
		if gc, ok := config.ToolGroups[groupName]; ok {
			// Copy the config to avoid mutating the original
			groupConfig = &models.ToolGroupConfig{
				Enabled:                gc.Enabled,
				EnableExecute:          gc.EnableExecute,
				ForceMode:              gc.ForceMode,
				AllowClientSuggestions: gc.AllowClientSuggestions,
			}
		}

		// Override approved_queries: only show as enabled if there are enabled queries
		if groupName == ToolGroupApprovedQueries && !approvedQueriesActive {
			groupConfig.Enabled = false
		}

		toolGroups[groupName] = groupConfig
	}

	return &MCPConfigResponse{
		ServerURL:  fmt.Sprintf("%s/mcp/%s", s.baseURL, projectID.String()),
		ToolGroups: toolGroups,
	}
}

// Ensure mcpConfigService implements MCPConfigService at compile time.
var _ MCPConfigService = (*mcpConfigService)(nil)
