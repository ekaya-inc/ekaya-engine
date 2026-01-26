package services

import (
	"context"
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ToolGroupApprovedQueries is the identifier for the pre-approved queries tool group.
const ToolGroupApprovedQueries = "approved_queries"

// ToolGroupAgentTools is the identifier for the agent tools group.
const ToolGroupAgentTools = "agent_tools"

// ToolGroupUser is the identifier for the user tools group.
const ToolGroupUser = "user"

// validToolGroups defines the known tool group identifiers for validation.
// UI metadata (names, descriptions, warnings) is defined in the frontend.
var validToolGroups = map[string]bool{
	ToolGroupUser:            true,
	"developer":              true,
	ToolGroupApprovedQueries: true, // Legacy - kept for backward compatibility
	ToolGroupAgentTools:      true,
}

// EnabledToolInfo represents a tool that is currently enabled for API responses.
type EnabledToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPConfigResponse is the API response format for MCP configuration.
// Returns only configuration state; UI strings are defined in the frontend.
type MCPConfigResponse struct {
	ServerURL      string                             `json:"serverUrl"`
	ToolGroups     map[string]*models.ToolGroupConfig `json:"toolGroups"`
	UserTools      []EnabledToolInfo                  `json:"userTools"`      // Tools for business users (role: user)
	DeveloperTools []EnabledToolInfo                  `json:"developerTools"` // Tools for admin/data/developer roles
	AgentTools     []EnabledToolInfo                  `json:"agentTools"`     // Tools for AI agents (API key auth)
	EnabledTools   []EnabledToolInfo                  `json:"enabledTools"`   // Deprecated: kept for backward compatibility
}

// UpdateMCPConfigRequest is the API request format for updating MCP configuration.
// Uses optional pointers to distinguish "not sent" from "sent as false".
type UpdateMCPConfigRequest struct {
	// User Tools sub-option: allow MCP clients with user role to update ontology
	AllowOntologyMaintenance *bool `json:"allowOntologyMaintenance,omitempty"`
	// Developer Tools sub-options
	AddQueryTools          *bool `json:"addQueryTools,omitempty"`          // adds Query loadout
	AddOntologyMaintenance *bool `json:"addOntologyMaintenance,omitempty"` // adds Ontology Maintenance tools
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

	// GetToolGroupsState returns the tool groups configuration state for use with GetEnabledTools.
	// This is the single source of truth for determining which tools should be enabled.
	GetToolGroupsState(ctx context.Context, projectID uuid.UUID) (map[string]*models.ToolGroupConfig, error)
}

type mcpConfigService struct {
	configRepo          repositories.MCPConfigRepository
	queryService        QueryService
	projectService      ProjectService
	installedAppService InstalledAppService
	stateValidator      MCPStateValidator
	baseURL             string
	logger              *zap.Logger
}

// NewMCPConfigService creates a new MCP config service with dependencies.
// installedAppService can be nil for backwards compatibility (tools won't be filtered by app installation).
func NewMCPConfigService(
	configRepo repositories.MCPConfigRepository,
	queryService QueryService,
	projectService ProjectService,
	installedAppService InstalledAppService,
	baseURL string,
	logger *zap.Logger,
) MCPConfigService {
	return &mcpConfigService{
		configRepo:          configRepo,
		queryService:        queryService,
		projectService:      projectService,
		installedAppService: installedAppService,
		stateValidator:      NewMCPStateValidator(),
		baseURL:             baseURL,
		logger:              logger,
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

	// Convert flat request to ToolGroups map update
	// Only include groups that have options being updated
	toolGroupsUpdate := make(map[string]*models.ToolGroupConfig)

	// Apply User Tools sub-option if provided
	if req.AllowOntologyMaintenance != nil {
		userConfig := config.ToolGroups["user"]
		if userConfig == nil {
			userConfig = &models.ToolGroupConfig{AllowOntologyMaintenance: true}
		}
		toolGroupsUpdate["user"] = &models.ToolGroupConfig{
			AllowOntologyMaintenance: *req.AllowOntologyMaintenance,
			// Preserve other fields from existing config
			Enabled:     userConfig.Enabled,
			CustomTools: userConfig.CustomTools,
		}
	}

	// Apply Developer Tools sub-options if provided
	devNeedsUpdate := req.AddQueryTools != nil || req.AddOntologyMaintenance != nil
	if devNeedsUpdate {
		devConfig := config.ToolGroups["developer"]
		if devConfig == nil {
			devConfig = &models.ToolGroupConfig{AddQueryTools: true, AddOntologyMaintenance: true}
		}
		newDevConfig := &models.ToolGroupConfig{
			AddQueryTools:          devConfig.AddQueryTools,
			AddOntologyMaintenance: devConfig.AddOntologyMaintenance,
			// Preserve other fields from existing config
			Enabled:     devConfig.Enabled,
			CustomTools: devConfig.CustomTools,
		}
		if req.AddQueryTools != nil {
			newDevConfig.AddQueryTools = *req.AddQueryTools
		}
		if req.AddOntologyMaintenance != nil {
			newDevConfig.AddOntologyMaintenance = *req.AddOntologyMaintenance
		}
		toolGroupsUpdate["developer"] = newDevConfig
	}

	// Build state context for validation
	hasQueries, err := s.hasEnabledQueries(ctx, projectID)
	if err != nil {
		s.logger.Warn("failed to check enabled queries, assuming none",
			zap.String("project_id", projectID.String()),
			zap.Error(err),
		)
		hasQueries = false
	}

	// Apply state transition using the validator
	result := s.stateValidator.Apply(
		MCPStateTransition{
			Current: config.ToolGroups,
			Update:  toolGroupsUpdate,
		},
		MCPStateContext{HasEnabledQueries: hasQueries},
	)

	// If validation failed, return error without persisting
	if result.Error != nil {
		s.logger.Debug("MCP config update rejected",
			zap.String("project_id", projectID.String()),
			zap.String("error_code", result.Error.Code),
			zap.String("error_message", result.Error.Message),
		)
		return nil, result.Error
	}

	// Update config with validated state
	config.ToolGroups = result.State

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

// GetToolGroupsState returns the tool groups configuration state.
// This is used by the MCP tool filter to determine which tools should be enabled,
// ensuring consistency with the UI display via GetEnabledTools.
func (s *mcpConfigService) GetToolGroupsState(ctx context.Context, projectID uuid.UUID) (map[string]*models.ToolGroupConfig, error) {
	config, err := s.configRepo.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP config: %w", err)
	}

	// If no config exists, use defaults
	if config == nil {
		config = models.DefaultMCPConfig(projectID)
	}

	return config.ToolGroups, nil
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
// Uses the state validator to ensure the response is normalized (sub-options reset when disabled).
// Computes per-role tool lists (UserTools, DeveloperTools, AgentTools) based on configuration.
// Filters out tools that require apps not installed (e.g., AI Data Liaison tools).
func (s *mcpConfigService) buildResponse(ctx context.Context, projectID uuid.UUID, config *models.MCPConfig) *MCPConfigResponse {
	// Use the state validator to normalize the state for response
	// Apply an empty update to get normalized state with all tool groups
	result := s.stateValidator.Apply(
		MCPStateTransition{
			Current: config.ToolGroups,
			Update:  map[string]*models.ToolGroupConfig{}, // Empty update = just normalize
		},
		MCPStateContext{HasEnabledQueries: true}, // Context doesn't matter for empty update
	)

	// Ensure all valid tool groups are in the response
	toolGroups := make(map[string]*models.ToolGroupConfig)
	for groupName := range validToolGroups {
		if gc, ok := result.State[groupName]; ok {
			toolGroups[groupName] = gc
		} else {
			toolGroups[groupName] = &models.ToolGroupConfig{}
		}
	}

	serverURL, err := url.JoinPath(s.baseURL, "mcp", projectID.String())
	if err != nil {
		s.logger.Error("failed to build server URL",
			zap.String("base_url", s.baseURL),
			zap.String("project_id", projectID.String()),
			zap.Error(err),
		)
		// Fall back to simple concatenation if URL parsing fails
		serverURL = s.baseURL + "/mcp/" + projectID.String()
	}

	// Check if AI Data Liaison app is installed for filtering
	dataLiaisonInstalled := false
	if s.installedAppService != nil {
		installed, err := s.installedAppService.IsInstalled(ctx, projectID, models.AppIDAIDataLiaison)
		if err != nil {
			s.logger.Warn("failed to check AI Data Liaison app installation, assuming not installed",
				zap.String("project_id", projectID.String()),
				zap.Error(err),
			)
		} else {
			dataLiaisonInstalled = installed
		}
	}

	// Compute per-role tool lists based on configuration
	userToolSpecs := ComputeUserTools(result.State)
	developerToolSpecs := ComputeDeveloperTools(result.State)
	agentToolSpecs := ComputeAgentTools(result.State)

	// Convert ToolSpec to EnabledToolInfo with data liaison filtering
	userTools := s.filterAndConvertToolSpecs(userToolSpecs, dataLiaisonInstalled)
	developerTools := s.filterAndConvertToolSpecs(developerToolSpecs, dataLiaisonInstalled)
	agentTools := s.filterAndConvertToolSpecs(agentToolSpecs, dataLiaisonInstalled)

	// EnabledTools (deprecated) - uses the old computation for backward compatibility
	enabledTools := s.filterAndConvertToolDefs(result.EnabledTools, dataLiaisonInstalled)

	return &MCPConfigResponse{
		ServerURL:      serverURL,
		ToolGroups:     toolGroups,
		UserTools:      userTools,
		DeveloperTools: developerTools,
		AgentTools:     agentTools,
		EnabledTools:   enabledTools,
	}
}

// filterAndConvertToolSpecs converts ToolSpec slice to EnabledToolInfo slice,
// filtering out data liaison tools if the app is not installed.
func (s *mcpConfigService) filterAndConvertToolSpecs(tools []ToolSpec, dataLiaisonInstalled bool) []EnabledToolInfo {
	result := make([]EnabledToolInfo, 0, len(tools))
	for _, tool := range tools {
		// Skip data liaison tools if app is not installed
		if !dataLiaisonInstalled && DataLiaisonTools[tool.Name] {
			continue
		}
		result = append(result, EnabledToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
		})
	}
	return result
}

// filterAndConvertToolDefs converts ToolDefinition slice to EnabledToolInfo slice,
// filtering out data liaison tools if the app is not installed.
func (s *mcpConfigService) filterAndConvertToolDefs(tools []ToolDefinition, dataLiaisonInstalled bool) []EnabledToolInfo {
	result := make([]EnabledToolInfo, 0, len(tools))
	for _, tool := range tools {
		// Skip data liaison tools if app is not installed
		if !dataLiaisonInstalled && DataLiaisonTools[tool.Name] {
			continue
		}
		result = append(result, EnabledToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
		})
	}
	return result
}

// Ensure mcpConfigService implements MCPConfigService at compile time.
var _ MCPConfigService = (*mcpConfigService)(nil)
