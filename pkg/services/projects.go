package services

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/central"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ErrOntologyNotFound is returned when no active ontology exists for a project.
var ErrOntologyNotFound = errors.New("no active ontology found for project")

// CentralProjectClient is the subset of central.Client used by ProjectService for deletion.
type CentralProjectClient interface {
	DeleteProject(ctx context.Context, baseURL, projectID, token, callbackUrl string) (*central.AppActionResponse, error)
}

// DeleteResult is returned by Delete when central may require a redirect.
type DeleteResult struct {
	RedirectUrl string // If set, redirect the user here (central confirmation page)
	Status      string // e.g., "pending_delete", "deleted"
}

// DeleteCallbackResult is returned by CompleteDeleteCallback after processing.
type DeleteCallbackResult struct {
	ProjectsPageURL string // Central's projects list URL for post-deletion redirect
}

// ProvisionResult contains the result of provisioning a project.
type ProvisionResult struct {
	ProjectID       uuid.UUID
	Name            string
	Applications    []central.ApplicationInfo // applications assigned by central
	PAPIURL         string
	ProjectsPageURL string // URL to ekaya-central projects list
	ProjectPageURL  string // URL to this project in ekaya-central
	Created         bool   // true if project was created, false if already existed
}

// AutoApproveSettings contains the auto-approve configuration for a project.
type AutoApproveSettings struct {
	SchemaChanges    bool `json:"schema_changes"`    // Auto-approve schema changes (new tables, columns, etc.)
	InferenceChanges bool `json:"inference_changes"` // Auto-approve inference changes (LLM-generated)
}

// OntologySettings contains ontology extraction configuration for a project.
type OntologySettings struct {
	// UseLegacyPatternMatching controls whether column name pattern matching is used
	// during FK candidate filtering. When true (default), columns are filtered based
	// on naming patterns (e.g., _id suffix, is_ prefix). When false, filtering relies
	// solely on data-based analysis (cardinality, join validation).
	UseLegacyPatternMatching bool `json:"use_legacy_pattern_matching"`
}

// ProjectService defines the interface for project operations.
type ProjectService interface {
	Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error)
	ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error)
	GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error)
	Delete(ctx context.Context, id uuid.UUID) (*DeleteResult, error)
	CompleteDeleteCallback(ctx context.Context, projectID uuid.UUID, action, status, nonce string) (*DeleteCallbackResult, error)
	GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error)
	SetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID, datasourceID uuid.UUID) error
	SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string)
	GetAuthServerURL(ctx context.Context, projectID uuid.UUID) (string, error)
	UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error

	// Auto-approve settings for living ontology changes
	GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*AutoApproveSettings, error)
	SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *AutoApproveSettings) error

	// Ontology extraction settings
	GetOntologySettings(ctx context.Context, projectID uuid.UUID) (*OntologySettings, error)
	SetOntologySettings(ctx context.Context, projectID uuid.UUID, settings *OntologySettings) error

	// SyncServerURL pushes the engine's base URL to ekaya-central so redirect URLs
	// and MCP setup links are correct after TLS/domain configuration changes.
	SyncServerURL(ctx context.Context, projectID uuid.UUID, papiURL, token string) error
}

// projectService implements ProjectService.
type projectService struct {
	db                   *database.DB
	projectRepo          repositories.ProjectRepository
	userRepo             repositories.UserRepository
	ontologyRepo         repositories.OntologyRepository
	mcpConfigRepo        repositories.MCPConfigRepository
	agentAPIKeyService   AgentAPIKeyService
	centralClient        *central.Client
	centralProjectClient CentralProjectClient
	nonceStore           NonceStore
	baseURL              string
	logger               *zap.Logger
}

// NewProjectService creates a new project service with dependencies.
func NewProjectService(
	db *database.DB,
	projectRepo repositories.ProjectRepository,
	userRepo repositories.UserRepository,
	ontologyRepo repositories.OntologyRepository,
	mcpConfigRepo repositories.MCPConfigRepository,
	agentAPIKeyService AgentAPIKeyService,
	centralClient *central.Client,
	nonceStore NonceStore,
	baseURL string,
	logger *zap.Logger,
) ProjectService {
	return &projectService{
		db:                   db,
		projectRepo:          projectRepo,
		userRepo:             userRepo,
		ontologyRepo:         ontologyRepo,
		mcpConfigRepo:        mcpConfigRepo,
		agentAPIKeyService:   agentAPIKeyService,
		centralClient:        centralClient,
		centralProjectClient: centralClient,
		nonceStore:           nonceStore,
		baseURL:              baseURL,
		logger:               logger,
	}
}

// Provision ensures a project exists with the given ID (idempotent).
// This is called during user's first access to provision the project locally.
// Uses WithoutTenant since the project may not exist yet.
func (s *projectService) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
	// Acquire connection without tenant context (project may not exist yet)
	scope, err := s.db.WithoutTenant(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer scope.Close()

	// Set tenant scope in context for repositories
	ctx = database.SetTenantScope(ctx, scope)

	// Check if project already exists
	existingProject, err := s.projectRepo.Get(ctx, projectID)
	if err == nil {
		// Project exists - return its info
		var papiURL, projectsPageURL, projectPageURL string
		var storedApps []central.ApplicationInfo
		if existingProject.Parameters != nil {
			if v, ok := existingProject.Parameters["papi_url"].(string); ok {
				papiURL = v
			}
			if v, ok := existingProject.Parameters["projects_page_url"].(string); ok {
				projectsPageURL = v
			}
			if v, ok := existingProject.Parameters["project_page_url"].(string); ok {
				projectPageURL = v
			}
			storedApps, _ = existingProject.Parameters["applications"].([]central.ApplicationInfo)
		}
		return &ProvisionResult{
			ProjectID:       existingProject.ID,
			Name:            existingProject.Name,
			Applications:    storedApps,
			PAPIURL:         papiURL,
			ProjectsPageURL: projectsPageURL,
			ProjectPageURL:  projectPageURL,
			Created:         false,
		}, nil
	}

	// Check if it's a "not found" error or a real error
	if !errors.Is(err, apperrors.ErrNotFound) {
		return nil, fmt.Errorf("failed to check project existence: %w", err)
	}

	// Project doesn't exist - create it
	s.logger.Info("Provisioning new project",
		zap.String("project_id", projectID.String()),
		zap.String("name", name))

	project := &models.Project{
		ID:         projectID,
		Name:       name,
		Parameters: params,
		Status:     "active",
	}

	if err = s.projectRepo.Create(ctx, project); err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	// Determine which applications to provision.
	// Default to MCP server if no applications specified (backward compat with old central).
	applications, _ := params["applications"].([]central.ApplicationInfo)
	hasMCPServer := len(applications) == 0
	hasAIAgents := false
	for _, app := range applications {
		if app.Name == central.AppMCPServer {
			hasMCPServer = true
		}
		if app.Name == central.AppAIAgents {
			hasAIAgents = true
		}
	}

	if hasMCPServer {
		// Create empty ontology for immediate MCP tool use.
		s.createEmptyOntology(ctx, projectID)
	}

	if hasMCPServer || hasAIAgents {
		// Create MCP config with defaults.
		s.createDefaultMCPConfig(ctx, projectID)

		// Generate Agent API Key for authentication.
		s.generateAgentAPIKey(ctx, projectID)
	}

	// Extract URLs from parameters
	var papiURL, projectsPageURL, projectPageURL string
	if params != nil {
		if v, ok := params["papi_url"].(string); ok {
			papiURL = v
		}
		if v, ok := params["projects_page_url"].(string); ok {
			projectsPageURL = v
		}
		if v, ok := params["project_page_url"].(string); ok {
			projectPageURL = v
		}
	}

	return &ProvisionResult{
		ProjectID:       project.ID,
		Name:            project.Name,
		Applications:    applications,
		PAPIURL:         papiURL,
		ProjectsPageURL: projectsPageURL,
		ProjectPageURL:  projectPageURL,
		Created:         true,
	}, nil
}

// createEmptyOntology creates an empty ontology record for the given project.
// This enables immediate use of MCP ontology tools without requiring extraction.
// Errors are logged but not propagated since this is best-effort.
func (s *projectService) createEmptyOntology(ctx context.Context, projectID uuid.UUID) {
	emptyOntology := &models.TieredOntology{
		ProjectID:     projectID,
		Version:       1,
		IsActive:      true,
		ColumnDetails: make(map[string][]models.ColumnDetail),
		Metadata:      make(map[string]any),
	}

	if err := s.ontologyRepo.Create(ctx, emptyOntology); err != nil {
		s.logger.Error("failed to create initial ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err),
		)
	}
}

// createDefaultMCPConfig creates the MCP config with default settings for a new project.
// This ensures tools are configured correctly from the start and prevents
// other code paths (like SetAgentAPIKey) from creating config with wrong defaults.
// Errors are logged but not propagated since this is best-effort.
func (s *projectService) createDefaultMCPConfig(ctx context.Context, projectID uuid.UUID) {
	if s.mcpConfigRepo == nil {
		return
	}

	defaultConfig := models.DefaultMCPConfig(projectID)
	if err := s.mcpConfigRepo.Upsert(ctx, defaultConfig); err != nil {
		s.logger.Error("failed to create default MCP config",
			zap.String("project_id", projectID.String()),
			zap.Error(err),
		)
	}
}

// generateAgentAPIKey generates an Agent API Key for MCP authentication.
// This is called during project provisioning so the key is ready when the user
// first visits the MCP Server configuration page. Users can regenerate the key there if needed.
// Errors are logged but not propagated since this is best-effort.
func (s *projectService) generateAgentAPIKey(ctx context.Context, projectID uuid.UUID) {
	if s.agentAPIKeyService == nil {
		return
	}

	if _, err := s.agentAPIKeyService.GenerateKey(ctx, projectID); err != nil {
		s.logger.Error("failed to generate agent API key",
			zap.String("project_id", projectID.String()),
			zap.Error(err),
		)
	}
}

// GetByID returns a project by its ID.
// Assumes tenant context is already set by middleware.
func (s *projectService) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return s.projectRepo.Get(ctx, id)
}

// GetByIDWithoutTenant returns a project by its ID, managing its own connection.
// Used when tenant context is not available (e.g., checking if project exists before provisioning).
func (s *projectService) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	scope, err := s.db.WithoutTenant(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer scope.Close()

	ctx = database.SetTenantScope(ctx, scope)
	return s.projectRepo.Get(ctx, id)
}

// Delete initiates project deletion. If central requires a redirect for billing cleanup,
// returns a DeleteResult with RedirectUrl. Otherwise deletes locally.
// Assumes tenant context is already set by middleware.
func (s *projectService) Delete(ctx context.Context, id uuid.UUID) (*DeleteResult, error) {
	token, papiURL, err := s.getAuthContext(ctx)
	if err != nil {
		// No central context (e.g., standalone mode) — delete locally
		s.logger.Warn("No auth context for central delete, deleting locally",
			zap.String("project_id", id.String()),
			zap.Error(err))
		if err := s.projectRepo.Delete(ctx, id); err != nil {
			return nil, fmt.Errorf("failed to delete project: %w", err)
		}
		return &DeleteResult{Status: "deleted"}, nil
	}

	// Build callback URL pointing to the Settings page
	callbackUrl := s.buildDeleteCallbackURL(id.String())

	centralResp, err := s.centralProjectClient.DeleteProject(ctx, papiURL, id.String(), token, callbackUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to notify central: %w", err)
	}

	s.logger.Info("Central delete response",
		zap.String("project_id", id.String()),
		zap.String("status", centralResp.Status),
		zap.String("redirect_url", centralResp.RedirectUrl),
		zap.String("callback_url", callbackUrl),
	)

	// If central requires a redirect (billing confirmation), return it without deleting
	if centralResp.RedirectUrl != "" {
		return &DeleteResult{
			RedirectUrl: centralResp.RedirectUrl,
			Status:      centralResp.Status,
		}, nil
	}

	// No redirect — central handled it immediately, delete locally
	if err := s.projectRepo.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("failed to delete project: %w", err)
	}

	return &DeleteResult{Status: centralResp.Status}, nil
}

// CompleteDeleteCallback processes the callback from central after a redirect flow.
// Returns the projects_page_url so the frontend can redirect there after deletion.
func (s *projectService) CompleteDeleteCallback(ctx context.Context, projectID uuid.UUID, action, status, nonce string) (*DeleteCallbackResult, error) {
	if !s.nonceStore.Validate(nonce, "delete", projectID.String(), "project") {
		return nil, fmt.Errorf("invalid or expired callback nonce")
	}

	if status == "cancelled" {
		s.logger.Info("Project delete cancelled by user",
			zap.String("project_id", projectID.String()),
		)
		return &DeleteCallbackResult{}, nil
	}

	if status != "success" {
		return nil, fmt.Errorf("unexpected callback status: %s", status)
	}

	// Read project before deletion to extract projects_page_url for the redirect
	var projectsPageURL string
	project, err := s.projectRepo.Get(ctx, projectID)
	if err == nil && project.Parameters != nil {
		if v, ok := project.Parameters["projects_page_url"].(string); ok {
			projectsPageURL = v
		}
	}

	if err := s.projectRepo.Delete(ctx, projectID); err != nil {
		return nil, fmt.Errorf("failed to delete project: %w", err)
	}

	s.logger.Info("Project deleted via callback",
		zap.String("project_id", projectID.String()),
	)
	return &DeleteCallbackResult{ProjectsPageURL: projectsPageURL}, nil
}

// buildDeleteCallbackURL constructs the engine callback URL for project deletion.
func (s *projectService) buildDeleteCallbackURL(projectID string) string {
	nonce := s.nonceStore.Generate("delete", projectID, "project")
	callbackURL := fmt.Sprintf("%s/projects/%s/settings", s.baseURL, projectID)

	params := url.Values{}
	params.Set("callback_action", "delete")
	params.Set("callback_state", nonce)

	return callbackURL + "?" + params.Encode()
}

// getAuthContext extracts the JWT token and central API URL from the request context.
func (s *projectService) getAuthContext(ctx context.Context) (token, papiURL string, err error) {
	t, ok := auth.GetToken(ctx)
	if !ok {
		return "", "", fmt.Errorf("no auth token in context")
	}

	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return "", "", fmt.Errorf("no auth claims in context")
	}

	if claims.PAPI == "" {
		return "", "", fmt.Errorf("no central API URL in token claims")
	}

	return t, claims.PAPI, nil
}

// GetDefaultDatasourceID retrieves the default datasource ID from project parameters.
// Returns uuid.Nil if no datasource is configured.
func (s *projectService) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to get project: %w", err)
	}

	if project.Parameters == nil {
		return uuid.Nil, nil
	}

	dsIDStr, ok := project.Parameters["default_datasource_id"].(string)
	if !ok || dsIDStr == "" {
		return uuid.Nil, nil
	}

	dsID, err := uuid.Parse(dsIDStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid datasource ID format: %w", err)
	}

	return dsID, nil
}

// SetDefaultDatasourceID updates the default datasource ID in project parameters.
func (s *projectService) SetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID, datasourceID uuid.UUID) error {
	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if project.Parameters == nil {
		project.Parameters = make(map[string]interface{})
	}
	project.Parameters["default_datasource_id"] = datasourceID.String()

	if err := s.projectRepo.Update(ctx, project); err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	s.logger.Info("Set default datasource for project",
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()))

	return nil
}

// GetAuthServerURL retrieves the auth server URL from project parameters.
// Returns empty string if not configured. Uses WithoutTenant since this is called
// from OAuth discovery before authentication.
func (s *projectService) GetAuthServerURL(ctx context.Context, projectID uuid.UUID) (string, error) {
	scope, err := s.db.WithoutTenant(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer scope.Close()

	ctx = database.SetTenantScope(ctx, scope)

	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get project: %w", err)
	}

	if project.Parameters == nil {
		return "", nil
	}

	authServerURL, _ := project.Parameters["auth_server_url"].(string)
	return authServerURL, nil
}

// UpdateAuthServerURL updates the auth_server_url in project parameters.
// Used to persist the auth URL when a user first accesses a project with a custom auth_url.
// Uses WithoutTenant since this is called during OAuth flow before full authentication.
func (s *projectService) UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error {
	scope, err := s.db.WithoutTenant(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer scope.Close()

	ctx = database.SetTenantScope(ctx, scope)

	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if project.Parameters == nil {
		project.Parameters = make(map[string]interface{})
	}
	project.Parameters["auth_server_url"] = authServerURL

	if err := s.projectRepo.Update(ctx, project); err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	s.logger.Info("Updated auth server URL for project",
		zap.String("project_id", projectID.String()),
		zap.String("auth_server_url", authServerURL))

	return nil
}

// GetAutoApproveSettings retrieves the auto-approve settings from project parameters.
// Returns default settings (all false) if not configured.
func (s *projectService) GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*AutoApproveSettings, error) {
	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	settings := &AutoApproveSettings{}

	if project.Parameters != nil {
		if autoApprove, ok := project.Parameters["auto_approve"].(map[string]interface{}); ok {
			if v, ok := autoApprove["schema_changes"].(bool); ok {
				settings.SchemaChanges = v
			}
			if v, ok := autoApprove["inference_changes"].(bool); ok {
				settings.InferenceChanges = v
			}
		}
	}

	return settings, nil
}

// SetAutoApproveSettings updates the auto-approve settings in project parameters.
func (s *projectService) SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *AutoApproveSettings) error {
	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if project.Parameters == nil {
		project.Parameters = make(map[string]interface{})
	}

	project.Parameters["auto_approve"] = map[string]interface{}{
		"schema_changes":    settings.SchemaChanges,
		"inference_changes": settings.InferenceChanges,
	}

	if err := s.projectRepo.Update(ctx, project); err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	s.logger.Info("Updated auto-approve settings for project",
		zap.String("project_id", projectID.String()),
		zap.Bool("schema_changes", settings.SchemaChanges),
		zap.Bool("inference_changes", settings.InferenceChanges))

	return nil
}

// GetOntologySettings returns the ontology extraction settings for a project.
// Returns default settings (UseLegacyPatternMatching=true) if not configured.
func (s *projectService) GetOntologySettings(ctx context.Context, projectID uuid.UUID) (*OntologySettings, error) {
	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	// Default: use legacy pattern matching for backward compatibility
	settings := &OntologySettings{
		UseLegacyPatternMatching: true,
	}

	if project.Parameters != nil {
		if ontology, ok := project.Parameters["ontology"].(map[string]interface{}); ok {
			if v, ok := ontology["use_legacy_pattern_matching"].(bool); ok {
				settings.UseLegacyPatternMatching = v
			}
		}
	}

	return settings, nil
}

// SetOntologySettings updates the ontology extraction settings in project parameters.
func (s *projectService) SetOntologySettings(ctx context.Context, projectID uuid.UUID, settings *OntologySettings) error {
	project, err := s.projectRepo.Get(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if project.Parameters == nil {
		project.Parameters = make(map[string]interface{})
	}

	project.Parameters["ontology"] = map[string]interface{}{
		"use_legacy_pattern_matching": settings.UseLegacyPatternMatching,
	}

	if err := s.projectRepo.Update(ctx, project); err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	s.logger.Info("Updated ontology settings for project",
		zap.String("project_id", projectID.String()),
		zap.Bool("use_legacy_pattern_matching", settings.UseLegacyPatternMatching))

	return nil
}

// ProvisionFromClaims provisions a project and user from JWT claims.
// This consolidates all provisioning logic - validates claims, creates project, and adds user.
// Idempotent - safe to call multiple times.
func (s *projectService) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error) {
	// Validate required claims
	if claims.ProjectID == "" {
		return nil, fmt.Errorf("missing project ID in claims")
	}
	if claims.Email == "" {
		return nil, fmt.Errorf("missing email in claims")
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("missing subject in claims")
	}
	if claims.PAPI == "" {
		return nil, fmt.Errorf("missing PAPI URL in claims")
	}

	// Parse project ID
	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("invalid project ID format: %w", err)
	}

	// Parse user ID from subject
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID in subject: %w", err)
	}

	// Provision project with ekaya-central (notifies central and gets project info)
	token, ok := auth.GetToken(ctx)
	if !ok {
		return nil, fmt.Errorf("missing JWT token in context")
	}

	projectInfo, err := s.centralClient.ProvisionProject(ctx, claims.PAPI, claims.ProjectID, token)
	if err != nil {
		return nil, fmt.Errorf("failed to provision project with ekaya-central: %w", err)
	}

	// Build parameters from claims and central response
	params := map[string]interface{}{
		"roles": claims.Roles,
	}
	params["papi_url"] = claims.PAPI
	if projectInfo.URLs.ProjectsPage != "" {
		params["projects_page_url"] = projectInfo.URLs.ProjectsPage
	}
	if projectInfo.URLs.ProjectPage != "" {
		params["project_page_url"] = projectInfo.URLs.ProjectPage
	}
	if projectInfo.URLs.AuthServerURL != "" {
		params["auth_server_url"] = projectInfo.URLs.AuthServerURL
	}
	if len(projectInfo.Applications) > 0 {
		params["applications"] = projectInfo.Applications
	}

	appNames := make([]string, len(projectInfo.Applications))
	for i, app := range projectInfo.Applications {
		appNames[i] = app.Name
	}
	s.logger.Info("Provisioning project from claims",
		zap.String("project_id", claims.ProjectID),
		zap.String("project_name", projectInfo.Name),
		zap.String("email", claims.Email),
		zap.String("papi", claims.PAPI),
		zap.Strings("roles", claims.Roles),
		zap.Strings("applications", appNames))

	// Ensure project exists (idempotent)
	result, err := s.Provision(ctx, projectID, projectInfo.Name, params)
	if err != nil {
		return nil, fmt.Errorf("failed to provision project: %w", err)
	}

	// Determine user role from claims
	userRole := models.RoleUser
	if len(claims.Roles) > 0 {
		userRole = claims.Roles[0]
	}

	// Validate role
	if !models.IsValidRole(userRole) {
		return nil, fmt.Errorf("%w: %s", apperrors.ErrInvalidRole, userRole)
	}

	// Ensure user exists (idempotent) - needs tenant context
	scope, err := s.db.WithTenant(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire tenant connection: %w", err)
	}
	defer scope.Close()

	ctxWithScope := database.SetTenantScope(ctx, scope)

	// Store email for audit trail display
	var email *string
	if claims.Email != "" {
		email = &claims.Email
	}

	user := &models.User{
		ProjectID: projectID,
		UserID:    userID,
		Email:     email,
		Role:      userRole,
	}

	if err := s.userRepo.Add(ctxWithScope, user); err != nil {
		return nil, fmt.Errorf("failed to add user to project: %w", err)
	}

	s.logger.Info("User ensured in project",
		zap.String("project_id", projectID.String()),
		zap.String("user_id", userID.String()),
		zap.String("role", userRole))

	return result, nil
}

// SyncFromCentralAsync fetches project info from ekaya-central in the background
// and updates the local project if the name has changed. This is fire-and-forget
// so the user doesn't wait for the sync to complete.
func (s *projectService) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {
	go func() {
		// Use background context since the request context may be cancelled
		ctx, cancel := context.WithTimeout(context.Background(), central.DefaultTimeout)
		defer cancel()

		// Fetch project info from ekaya-central
		projectInfo, err := s.centralClient.GetProject(ctx, papiURL, projectID.String(), token)
		if err != nil {
			s.logger.Error("Failed to sync project from ekaya-central",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return
		}

		// Get current project from DB
		scope, err := s.db.WithoutTenant(ctx)
		if err != nil {
			s.logger.Error("Failed to acquire connection for sync",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return
		}
		defer scope.Close()

		ctx = database.SetTenantScope(ctx, scope)

		project, err := s.projectRepo.Get(ctx, projectID)
		if err != nil {
			s.logger.Error("Failed to get project for sync",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return
		}

		// Check if name changed
		if project.Name == projectInfo.Name {
			s.logger.Debug("Project name unchanged, skipping sync",
				zap.String("project_id", projectID.String()))
			return
		}

		// Update project name
		oldName := project.Name
		project.Name = projectInfo.Name

		if err := s.projectRepo.Update(ctx, project); err != nil {
			s.logger.Error("Failed to update project name from central",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return
		}

		s.logger.Info("Synced project name from ekaya-central",
			zap.String("project_id", projectID.String()),
			zap.String("old_name", oldName),
			zap.String("new_name", projectInfo.Name))
	}()
}

// SyncServerURL pushes the engine's base URL to ekaya-central.
func (s *projectService) SyncServerURL(ctx context.Context, projectID uuid.UUID, papiURL, token string) error {
	_, err := s.centralClient.UpdateServerUrl(ctx, papiURL, projectID.String(), s.baseURL, token)
	if err != nil {
		return fmt.Errorf("failed to update server URL in ekaya-central: %w", err)
	}

	s.logger.Info("Synced server URL to ekaya-central",
		zap.String("project_id", projectID.String()),
		zap.String("server_url", s.baseURL))

	return nil
}

// Ensure projectService implements ProjectService at compile time.
var _ ProjectService = (*projectService)(nil)
