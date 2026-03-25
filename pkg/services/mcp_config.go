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

// ToolGroupTools is the identifier for the per-app tools config.
const ToolGroupTools = "tools"

// ToolGroupAgentTools is the identifier for the agent tools group.
const ToolGroupAgentTools = "agent_tools"

// validToolGroups defines the known tool group identifiers for validation.
// UI metadata (names, descriptions, warnings) is defined in the frontend.
var validToolGroups = map[string]bool{
	ToolGroupTools:      true,
	ToolGroupAgentTools: true,
}

// EnabledToolInfo represents a tool that is currently enabled for API responses.
type EnabledToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	AppID       string `json:"appId"`
}

// MCPConfigResponse is the API response format for MCP configuration.
// Returns only configuration state; UI strings are defined in the frontend.
type MCPConfigResponse struct {
	ServerURL      string                             `json:"serverUrl"`
	ToolGroups     map[string]*models.ToolGroupConfig `json:"toolGroups"`
	UserTools      []EnabledToolInfo                  `json:"userTools"`      // Tools for business users (role: user)
	DeveloperTools []EnabledToolInfo                  `json:"developerTools"` // Tools for admin/data/developer roles
	AgentTools     []EnabledToolInfo                  `json:"agentTools"`     // Tools for AI agents (API key auth)
	AppNames       map[string]string                  `json:"appNames"`       // App ID to display name mapping
}

// UpdateMCPConfigRequest is the API request format for updating MCP configuration.
// Uses optional pointers to distinguish "not sent" from "sent as false".
type UpdateMCPConfigRequest struct {
	// New per-app toggles
	AddDirectDatabaseAccess     *bool `json:"addDirectDatabaseAccess,omitempty"`
	AddOntologyMaintenanceTools *bool `json:"addOntologyMaintenanceTools,omitempty"`
	AddOntologySuggestions      *bool `json:"addOntologySuggestions,omitempty"`
	AddApprovalTools            *bool `json:"addApprovalTools,omitempty"`
	AddRequestTools             *bool `json:"addRequestTools,omitempty"`
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

	// GetToolGroupsState returns the tool groups configuration state for use with GetEnabledTools.
	// This is the single source of truth for determining which tools should be enabled.
	GetToolGroupsState(ctx context.Context, projectID uuid.UUID) (map[string]*models.ToolGroupConfig, error)
}

type mcpConfigService struct {
	configRepo          repositories.MCPConfigRepository
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
	_ QueryService,
	projectService ProjectService,
	installedAppService InstalledAppService,
	baseURL string,
	logger *zap.Logger,
) MCPConfigService {
	return &mcpConfigService{
		configRepo:          configRepo,
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

	toolGroupsUpdate := make(map[string]*models.ToolGroupConfig)

	toolsNeedsUpdate := req.AddDirectDatabaseAccess != nil || req.AddOntologyMaintenanceTools != nil ||
		req.AddOntologySuggestions != nil || req.AddApprovalTools != nil || req.AddRequestTools != nil
	if toolsNeedsUpdate {
		toolsCfg := config.ToolGroups[ToolGroupTools]
		if toolsCfg == nil {
			toolsCfg = &models.ToolGroupConfig{
				AddDirectDatabaseAccess:     true,
				AddOntologyMaintenanceTools: true,
				AddOntologySuggestions:      true,
				AddApprovalTools:            true,
				AddRequestTools:             true,
			}
		}
		newToolsCfg := &models.ToolGroupConfig{
			AddDirectDatabaseAccess:     toolsCfg.AddDirectDatabaseAccess,
			AddOntologyMaintenanceTools: toolsCfg.AddOntologyMaintenanceTools,
			AddOntologySuggestions:      toolsCfg.AddOntologySuggestions,
			AddApprovalTools:            toolsCfg.AddApprovalTools,
			AddRequestTools:             toolsCfg.AddRequestTools,
		}
		if req.AddDirectDatabaseAccess != nil {
			newToolsCfg.AddDirectDatabaseAccess = *req.AddDirectDatabaseAccess
		}
		if req.AddOntologyMaintenanceTools != nil {
			newToolsCfg.AddOntologyMaintenanceTools = *req.AddOntologyMaintenanceTools
		}
		if req.AddOntologySuggestions != nil {
			newToolsCfg.AddOntologySuggestions = *req.AddOntologySuggestions
		}
		if req.AddApprovalTools != nil {
			newToolsCfg.AddApprovalTools = *req.AddApprovalTools
		}
		if req.AddRequestTools != nil {
			newToolsCfg.AddRequestTools = *req.AddRequestTools
		}
		toolGroupsUpdate[ToolGroupTools] = newToolsCfg
	}

	result := s.stateValidator.Apply(
		MCPStateTransition{
			Current: config.ToolGroups,
			Update:  toolGroupsUpdate,
		},
		MCPStateContext{},
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

	// If no config exists, use defaults (all tool groups enabled)
	if config == nil {
		config = models.DefaultMCPConfig(projectID)
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

	// If no config exists, use defaults
	if config == nil {
		config = models.DefaultMCPConfig(projectID)
	}

	state := s.normalizeToolGroups(config.ToolGroups)
	if groupConfig, ok := state[toolGroup]; ok {
		return groupConfig, nil
	}

	return nil, nil
}

// GetToolGroupsState returns the tool groups configuration state.
// This is used by the MCP tool filter to determine which tools should be enabled.
func (s *mcpConfigService) GetToolGroupsState(ctx context.Context, projectID uuid.UUID) (map[string]*models.ToolGroupConfig, error) {
	config, err := s.configRepo.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP config: %w", err)
	}

	// If no config exists, use defaults
	if config == nil {
		config = models.DefaultMCPConfig(projectID)
	}

	return s.normalizeToolGroups(config.ToolGroups), nil
}

func (s *mcpConfigService) normalizeToolGroups(state map[string]*models.ToolGroupConfig) map[string]*models.ToolGroupConfig {
	result := s.stateValidator.Apply(
		MCPStateTransition{
			Current: state,
			Update:  map[string]*models.ToolGroupConfig{},
		},
		MCPStateContext{},
	)
	return result.State
}

// buildResponse creates the API response format from the model.
// Computes per-role tool lists (UserTools, DeveloperTools, AgentTools) based on configuration.
// Filters out tools that require apps not installed (e.g., AI Data Liaison tools).
func (s *mcpConfigService) buildResponse(ctx context.Context, projectID uuid.UUID, config *models.MCPConfig) *MCPConfigResponse {
	state := s.normalizeToolGroups(config.ToolGroups)

	// Ensure all valid tool groups are in the response
	toolGroups := make(map[string]*models.ToolGroupConfig)
	for groupName := range validToolGroups {
		if gc, ok := state[groupName]; ok {
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

	// Check which apps are installed for filtering
	installedApps := map[string]bool{
		models.AppIDMCPServer: true, // always available
	}
	if s.installedAppService != nil {
		for _, appID := range []string{models.AppIDOntologyForge, models.AppIDAIDataLiaison} {
			installed, err := s.installedAppService.IsInstalled(ctx, projectID, appID)
			if err != nil {
				s.logger.Warn("failed to check app installation",
					zap.String("project_id", projectID.String()),
					zap.String("app_id", appID),
					zap.Error(err),
				)
			} else if installed {
				installedApps[appID] = true
			}
		}
	}

	// Compute per-role tool lists based on configuration
	userToolSpecs := ComputeUserTools(state)
	developerToolSpecs := ComputeDeveloperTools(state)
	agentToolSpecs := ComputeAgentTools(state)

	// Convert ToolSpec to EnabledToolInfo with app installation filtering
	userTools := filterAndConvertToolSpecs(userToolSpecs, "user", installedApps)
	developerTools := filterAndConvertToolSpecs(developerToolSpecs, "developer", installedApps)
	agentTools := filterAndConvertToolSpecs(agentToolSpecs, "agent", installedApps)

	return &MCPConfigResponse{
		ServerURL:      serverURL,
		ToolGroups:     toolGroups,
		UserTools:      userTools,
		DeveloperTools: developerTools,
		AgentTools:     agentTools,
		AppNames:       AppDisplayNames,
	}
}

// filterAndConvertToolSpecs converts ToolSpec slice to EnabledToolInfo slice,
// filtering out tools from apps that are not installed.
func filterAndConvertToolSpecs(tools []ToolSpec, role string, installedApps map[string]bool) []EnabledToolInfo {
	result := make([]EnabledToolInfo, 0, len(tools))
	for _, tool := range tools {
		appID := GetToolAppID(tool.Name, role)
		if !installedApps[appID] {
			continue
		}
		result = append(result, EnabledToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			AppID:       appID,
		})
	}
	return result
}

// Ensure mcpConfigService implements MCPConfigService at compile time.
var _ MCPConfigService = (*mcpConfigService)(nil)
