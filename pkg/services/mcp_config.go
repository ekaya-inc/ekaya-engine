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

// SubOptionInfo contains metadata about a sub-option within a tool group.
type SubOptionInfo struct {
	Enabled     bool   `json:"enabled"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Warning     string `json:"warning,omitempty"`
}

// ToolGroupInfo contains static metadata about a tool group for API responses.
type ToolGroupInfo struct {
	Enabled     bool                      `json:"enabled"`
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Warning     string                    `json:"warning,omitempty"`
	SubOptions  map[string]*SubOptionInfo `json:"subOptions,omitempty"`
}

// MCPConfigResponse is the API response format for MCP configuration.
type MCPConfigResponse struct {
	ServerURL  string                    `json:"serverUrl"`
	ToolGroups map[string]*ToolGroupInfo `json:"toolGroups"`
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

// subOptionMetadata contains static information about sub-options within tool groups.
type subOptionMetadata struct {
	Name        string
	Description string
	Warning     string
}

// toolGroupDef contains static metadata about a tool group.
type toolGroupDef struct {
	Name        string
	Description string
	Warning     string
	SubOptions  map[string]subOptionMetadata
}

// Tool group metadata - static information about each tool group.
var toolGroupMetadata = map[string]toolGroupDef{
	"developer": {
		Name:        "Developer Tools",
		Description: "Enable raw access to the Datasource and Schema. This is intended for developers building applications or data engineers building ETL pipelines.",
		Warning:     "This setting is NOT recommended for business end users doing analytics.",
		SubOptions: map[string]subOptionMetadata{
			"enableExecute": {
				Name:    "Enable Execute",
				Warning: "The MCP Client will have direct access to the Datasource using the supplied credentials -- this access includes potentially destructive operations. Back up the data before allowing AI to modify it.",
			},
		},
	},
	ToolGroupApprovedQueries: {
		Name:        "Pre-Approved Queries",
		Description: "Enable pre-approved SQL queries that can be executed by the MCP client. Queries must be created and enabled in the Pre-Approved Queries section.",
		SubOptions: map[string]subOptionMetadata{
			"forceMode": {
				Name:        "FORCE all access through Pre-Approved Queries",
				Description: "When enabled, MCP clients can only execute Pre-Approved Queries. This is the safest way to enable AI access to data but it is also the least flexible. Enable this option if you want restricted scope access to set UI features, reports or processes.",
				Warning:     "Enabling this will disable Developer Tools.",
			},
			"allowClientSuggestions": {
				Name:        "Allow MCP Client to suggest Queries for the Pre-Approved List",
				Description: "Allow the MCP Client to suggest new Queries to be added to the Pre-Approved list after your review.",
			},
		},
	},
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
		if _, known := toolGroupMetadata[groupName]; known {
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
func (s *mcpConfigService) buildResponse(ctx context.Context, projectID uuid.UUID, config *models.MCPConfig) *MCPConfigResponse {
	toolGroups := make(map[string]*ToolGroupInfo)

	// Check if approved_queries should actually be enabled (requires enabled queries)
	approvedQueriesActive, err := s.hasEnabledQueries(ctx, projectID)
	if err != nil {
		s.logger.Error("failed to check enabled queries, defaulting to disabled",
			zap.String("project_id", projectID.String()),
			zap.Error(err),
		)
		approvedQueriesActive = false
	}

	for groupName, metadata := range toolGroupMetadata {
		enabled := false
		var groupConfig *models.ToolGroupConfig
		if gc, ok := config.ToolGroups[groupName]; ok {
			groupConfig = gc
			enabled = groupConfig.Enabled
		}

		// Override approved_queries: only show as enabled if there are enabled queries
		if groupName == ToolGroupApprovedQueries && !approvedQueriesActive {
			enabled = false
		}

		info := &ToolGroupInfo{
			Enabled:     enabled,
			Name:        metadata.Name,
			Description: metadata.Description,
			Warning:     metadata.Warning,
		}

		// Build sub-options if any exist
		if len(metadata.SubOptions) > 0 {
			info.SubOptions = make(map[string]*SubOptionInfo)
			for subName, subMeta := range metadata.SubOptions {
				subEnabled := false
				if groupConfig != nil {
					switch subName {
					case "enableExecute":
						subEnabled = groupConfig.EnableExecute
					case "forceMode":
						subEnabled = groupConfig.ForceMode
					case "allowClientSuggestions":
						subEnabled = groupConfig.AllowClientSuggestions
					}
				}
				info.SubOptions[subName] = &SubOptionInfo{
					Enabled:     subEnabled,
					Name:        subMeta.Name,
					Description: subMeta.Description,
					Warning:     subMeta.Warning,
				}
			}
		}

		toolGroups[groupName] = info
	}

	return &MCPConfigResponse{
		ServerURL:  fmt.Sprintf("%s/mcp/%s", s.baseURL, projectID.String()),
		ToolGroups: toolGroups,
	}
}

// Ensure mcpConfigService implements MCPConfigService at compile time.
var _ MCPConfigService = (*mcpConfigService)(nil)
