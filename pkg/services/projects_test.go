//go:build integration

package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/central"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

func TestProjectService_UpdateAuthServerURL_Success(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000201")

	// Ensure test project exists
	ensureTestProject(t, engineDB, projectID, "Update Auth URL Test")

	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	authURL := "http://localhost:5002"
	err := service.UpdateAuthServerURL(context.Background(), projectID, authURL)
	if err != nil {
		t.Fatalf("UpdateAuthServerURL failed: %v", err)
	}

	// Verify auth_server_url was set by using GetAuthServerURL
	retrievedURL, err := service.GetAuthServerURL(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetAuthServerURL failed: %v", err)
	}

	if retrievedURL != authURL {
		t.Errorf("expected auth_server_url %q, got %q", authURL, retrievedURL)
	}
}

func TestProjectService_UpdateAuthServerURL_InitializesParametersIfNil(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000202")

	// Create test project with empty JSON parameters (equivalent to nil map in Go)
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status, parameters)
		VALUES ($1, $2, 'active', '{}'::jsonb)
		ON CONFLICT (id) DO UPDATE SET parameters = '{}'::jsonb
	`, projectID, "Nil Parameters Test")
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	authURL := "https://auth.example.com"
	err = service.UpdateAuthServerURL(context.Background(), projectID, authURL)
	if err != nil {
		t.Fatalf("UpdateAuthServerURL failed: %v", err)
	}

	// Verify auth_server_url was set by using GetAuthServerURL
	retrievedURL, err := service.GetAuthServerURL(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetAuthServerURL failed: %v", err)
	}

	if retrievedURL != authURL {
		t.Errorf("expected auth_server_url %q, got %q", authURL, retrievedURL)
	}
}

func TestProjectService_UpdateAuthServerURL_PreservesExistingParameters(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000203")

	// Create test project with existing parameters
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status, parameters)
		VALUES ($1, $2, 'active', $3::jsonb)
		ON CONFLICT (id) DO UPDATE SET parameters = $3::jsonb
	`, projectID, "Preserve Parameters Test", `{"papi_url": "https://papi.example.com", "default_datasource": "datasource-id"}`)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	authURL := "http://localhost:5002"
	err = service.UpdateAuthServerURL(context.Background(), projectID, authURL)
	if err != nil {
		t.Fatalf("UpdateAuthServerURL failed: %v", err)
	}

	// Verify auth_server_url was set by using GetAuthServerURL
	retrievedURL, err := service.GetAuthServerURL(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetAuthServerURL failed: %v", err)
	}

	if retrievedURL != authURL {
		t.Errorf("expected auth_server_url %q, got %q", authURL, retrievedURL)
	}

	// Verify existing parameters were preserved by reading directly from DB
	scope2, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope: %v", err)
	}
	defer scope2.Close()

	var parametersJSON string
	err = scope2.Conn.QueryRow(ctx, `SELECT parameters::text FROM engine_projects WHERE id = $1`, projectID).Scan(&parametersJSON)
	if err != nil {
		t.Fatalf("Failed to query project parameters: %v", err)
	}

	// Check that parameters contains both old and new values
	if !strings.Contains(parametersJSON, "papi_url") {
		t.Error("expected papi_url to be preserved in parameters")
	}
	if !strings.Contains(parametersJSON, "default_datasource") {
		t.Error("expected default_datasource to be preserved in parameters")
	}
	if !strings.Contains(parametersJSON, "auth_server_url") {
		t.Error("expected auth_server_url to be added to parameters")
	}
}

func TestProjectService_UpdateAuthServerURL_ProjectNotFound(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	// Use a non-existent project ID
	nonExistentID := uuid.New()

	err := service.UpdateAuthServerURL(context.Background(), nonExistentID, "http://localhost:5002")
	if err == nil {
		t.Fatal("expected error when project not found")
	}
}

func TestProjectService_GetAuthServerURL_Success(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000204")

	// Create test project with auth_server_url set
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope: %v", err)
	}
	defer scope.Close()

	expectedAuthURL := "http://localhost:5002"
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status, parameters)
		VALUES ($1, $2, 'active', $3::jsonb)
		ON CONFLICT (id) DO UPDATE SET parameters = $3::jsonb
	`, projectID, "Get Auth URL Test", `{"auth_server_url": "`+expectedAuthURL+`"}`)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	authURL, err := service.GetAuthServerURL(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetAuthServerURL failed: %v", err)
	}

	if authURL != expectedAuthURL {
		t.Errorf("expected auth URL %q, got %q", expectedAuthURL, authURL)
	}
}

func TestProjectService_GetAuthServerURL_NotSet(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000205")

	// Ensure test project exists without auth_server_url
	ensureTestProject(t, engineDB, projectID, "Get Auth URL Not Set Test")

	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	authURL, err := service.GetAuthServerURL(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetAuthServerURL failed: %v", err)
	}

	if authURL != "" {
		t.Errorf("expected empty auth URL, got %q", authURL)
	}
}

func TestProjectService_GetAuthServerURL_ProjectNotFound(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	// Use a non-existent project ID
	nonExistentID := uuid.New()

	_, err := service.GetAuthServerURL(context.Background(), nonExistentID)
	if err == nil {
		t.Fatal("expected error when project not found")
	}
}

func TestProjectService_UpdateAndGetAuthServerURL_RoundTrip(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000206")

	// Ensure test project exists
	ensureTestProject(t, engineDB, projectID, "Round Trip Test")

	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	// Update auth_server_url
	authURL := "http://localhost:5002"
	if err := service.UpdateAuthServerURL(context.Background(), projectID, authURL); err != nil {
		t.Fatalf("UpdateAuthServerURL failed: %v", err)
	}

	// Get auth_server_url and verify it matches
	retrievedAuthURL, err := service.GetAuthServerURL(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetAuthServerURL failed: %v", err)
	}

	if retrievedAuthURL != authURL {
		t.Errorf("expected auth URL %q, got %q", authURL, retrievedAuthURL)
	}

	// Update to a different auth_server_url
	newAuthURL := "https://us.dev.ekaya.ai"
	if err := service.UpdateAuthServerURL(context.Background(), projectID, newAuthURL); err != nil {
		t.Fatalf("UpdateAuthServerURL failed on second update: %v", err)
	}

	// Verify the update
	retrievedAuthURL, err = service.GetAuthServerURL(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetAuthServerURL failed after second update: %v", err)
	}

	if retrievedAuthURL != newAuthURL {
		t.Errorf("expected auth URL %q after update, got %q", newAuthURL, retrievedAuthURL)
	}
}

func TestProjectService_Provision_CreatesEmptyOntology(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)

	// Use a unique project ID for this test
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000207")
	ctx := context.Background()

	// Clean up any existing data for this project ID before the test
	scope, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope: %v", err)
	}
	// Use tenant context for the ontology delete (RLS)
	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err == nil {
		_, _ = tenantScope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, projectID)
		tenantScope.Close()
	}
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_projects WHERE id = $1`, projectID)
	scope.Close()

	// Create repositories
	projectRepo := repositories.NewProjectRepository()
	userRepo := repositories.NewUserRepository()
	ontologyRepo := repositories.NewOntologyRepository()

	service := NewProjectService(engineDB.DB, projectRepo, userRepo, ontologyRepo, nil, nil, nil, nil, "", zap.NewNop())

	// Provision new project
	result, err := service.Provision(ctx, projectID, "Empty Ontology Test", nil)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	if !result.Created {
		t.Error("expected project to be created (new), got already exists")
	}

	// Verify ontology was created
	tenantScope, err = engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer tenantScope.Close()

	var ontologyExists bool
	err = tenantScope.Conn.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM engine_ontologies
			WHERE project_id = $1 AND is_active = true
		)
	`, projectID).Scan(&ontologyExists)
	if err != nil {
		t.Fatalf("Failed to check for ontology: %v", err)
	}

	if !ontologyExists {
		t.Error("expected an active ontology to be created with the project")
	}

	// Verify ontology is empty (version 1)
	var version int
	err = tenantScope.Conn.QueryRow(ctx, `
		SELECT version
		FROM engine_ontologies
		WHERE project_id = $1 AND is_active = true
	`, projectID).Scan(&version)
	if err != nil {
		t.Fatalf("Failed to query ontology details: %v", err)
	}

	if version != 1 {
		t.Errorf("expected ontology version 1, got %d", version)
	}
}

func TestProjectService_GetOntologySettings_DefaultsToLegacyPatternMatching(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000220")

	// Ensure test project exists without any ontology settings
	ensureTestProject(t, engineDB, projectID, "Default Ontology Settings Test")

	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	// Set up tenant context
	ctx := context.Background()
	scope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()
	ctx = database.SetTenantScope(ctx, scope)

	settings, err := service.GetOntologySettings(ctx, projectID)
	if err != nil {
		t.Fatalf("GetOntologySettings failed: %v", err)
	}

	// Should default to true for backward compatibility
	if !settings.UseLegacyPatternMatching {
		t.Error("expected UseLegacyPatternMatching to default to true")
	}
}

func TestProjectService_SetOntologySettings_Success(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000221")

	// Ensure test project exists
	ensureTestProject(t, engineDB, projectID, "Set Ontology Settings Test")

	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	// Set up tenant context
	ctx := context.Background()
	scope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()
	ctx = database.SetTenantScope(ctx, scope)

	// Disable legacy pattern matching
	settings := &OntologySettings{UseLegacyPatternMatching: false}
	err = service.SetOntologySettings(ctx, projectID, settings)
	if err != nil {
		t.Fatalf("SetOntologySettings failed: %v", err)
	}

	// Verify the setting was persisted
	retrievedSettings, err := service.GetOntologySettings(ctx, projectID)
	if err != nil {
		t.Fatalf("GetOntologySettings failed: %v", err)
	}

	if retrievedSettings.UseLegacyPatternMatching {
		t.Error("expected UseLegacyPatternMatching to be false after setting")
	}
}

func TestProjectService_SetOntologySettings_RoundTrip(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000222")

	// Ensure test project exists
	ensureTestProject(t, engineDB, projectID, "Ontology Settings Round Trip Test")

	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	// Set up tenant context
	ctx := context.Background()
	scope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()
	ctx = database.SetTenantScope(ctx, scope)

	// Disable legacy pattern matching
	err = service.SetOntologySettings(ctx, projectID, &OntologySettings{
		UseLegacyPatternMatching: false,
	})
	if err != nil {
		t.Fatalf("SetOntologySettings (disable) failed: %v", err)
	}

	settings, err := service.GetOntologySettings(ctx, projectID)
	if err != nil {
		t.Fatalf("GetOntologySettings failed: %v", err)
	}

	if settings.UseLegacyPatternMatching {
		t.Error("expected UseLegacyPatternMatching to be false")
	}

	// Re-enable legacy pattern matching
	err = service.SetOntologySettings(ctx, projectID, &OntologySettings{
		UseLegacyPatternMatching: true,
	})
	if err != nil {
		t.Fatalf("SetOntologySettings (enable) failed: %v", err)
	}

	settings, err = service.GetOntologySettings(ctx, projectID)
	if err != nil {
		t.Fatalf("GetOntologySettings (after enable) failed: %v", err)
	}

	if !settings.UseLegacyPatternMatching {
		t.Error("expected UseLegacyPatternMatching to be true after re-enabling")
	}
}

func TestProjectService_SetOntologySettings_PreservesOtherParameters(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000223")

	// Create test project with existing parameters
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status, parameters)
		VALUES ($1, $2, 'active', $3::jsonb)
		ON CONFLICT (id) DO UPDATE SET parameters = $3::jsonb
	`, projectID, "Preserve Other Params Test", `{"papi_url": "https://papi.example.com", "auto_approve": {"schema_changes": true}}`)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	projectRepo := repositories.NewProjectRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, nil, nil, nil, nil, nil, "", zap.NewNop())

	// Set up tenant context for the service call
	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer tenantScope.Close()
	tenantCtx := database.SetTenantScope(ctx, tenantScope)

	// Set ontology settings
	err = service.SetOntologySettings(tenantCtx, projectID, &OntologySettings{
		UseLegacyPatternMatching: false,
	})
	if err != nil {
		t.Fatalf("SetOntologySettings failed: %v", err)
	}

	// Verify other parameters were preserved by reading directly from DB
	scope2, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope: %v", err)
	}
	defer scope2.Close()

	var parametersJSON string
	err = scope2.Conn.QueryRow(ctx, `SELECT parameters::text FROM engine_projects WHERE id = $1`, projectID).Scan(&parametersJSON)
	if err != nil {
		t.Fatalf("Failed to query project parameters: %v", err)
	}

	// Check that all parameters are present
	if !strings.Contains(parametersJSON, "papi_url") {
		t.Error("expected papi_url to be preserved in parameters")
	}
	if !strings.Contains(parametersJSON, "auto_approve") {
		t.Error("expected auto_approve to be preserved in parameters")
	}
	if !strings.Contains(parametersJSON, "ontology") {
		t.Error("expected ontology settings to be added to parameters")
	}
	if !strings.Contains(parametersJSON, "use_legacy_pattern_matching") {
		t.Error("expected use_legacy_pattern_matching to be in ontology settings")
	}
}

func TestProjectService_Provision_WithMCPServerApp(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000230")
	ctx := context.Background()

	// Clean up
	cleanupProject(t, engineDB, projectID)

	projectRepo := repositories.NewProjectRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, ontologyRepo, nil, nil, nil, nil, "", zap.NewNop())

	// Provision with mcp-server application
	params := map[string]interface{}{
		"applications": []central.ApplicationInfo{
			{Name: central.AppMCPServer},
		},
	}
	result, err := service.Provision(ctx, projectID, "MCP Server App Test", params)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if !result.Created {
		t.Error("expected project to be created")
	}

	// Verify ontology was created (MCP setup ran)
	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer tenantScope.Close()

	var ontologyExists bool
	err = tenantScope.Conn.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM engine_ontologies WHERE project_id = $1 AND is_active = true)
	`, projectID).Scan(&ontologyExists)
	if err != nil {
		t.Fatalf("Failed to check ontology: %v", err)
	}
	if !ontologyExists {
		t.Error("expected ontology to be created when mcp-server app is specified")
	}

	// Verify applications in result
	if len(result.Applications) != 1 {
		t.Fatalf("expected 1 application in result, got %d", len(result.Applications))
	}
	if result.Applications[0].Name != central.AppMCPServer {
		t.Errorf("expected application name %q, got %q", central.AppMCPServer, result.Applications[0].Name)
	}
}

func TestProjectService_Provision_WithNoApps_FallsBackToMCP(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000231")
	ctx := context.Background()

	// Clean up
	cleanupProject(t, engineDB, projectID)

	projectRepo := repositories.NewProjectRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, ontologyRepo, nil, nil, nil, nil, "", zap.NewNop())

	// Provision without applications (backward compat)
	result, err := service.Provision(ctx, projectID, "No Apps Fallback Test", nil)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if !result.Created {
		t.Error("expected project to be created")
	}

	// Verify ontology was created (MCP setup ran as fallback)
	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer tenantScope.Close()

	var ontologyExists bool
	err = tenantScope.Conn.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM engine_ontologies WHERE project_id = $1 AND is_active = true)
	`, projectID).Scan(&ontologyExists)
	if err != nil {
		t.Fatalf("Failed to check ontology: %v", err)
	}
	if !ontologyExists {
		t.Error("expected ontology to be created as fallback when no applications specified")
	}
}

func TestProjectService_Provision_WithNonMCPApp_SkipsMCPSetup(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000232")
	ctx := context.Background()

	// Clean up
	cleanupProject(t, engineDB, projectID)

	projectRepo := repositories.NewProjectRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	service := NewProjectService(engineDB.DB, projectRepo, nil, ontologyRepo, nil, nil, nil, nil, "", zap.NewNop())

	// Provision with only ai-data-liaison (no mcp-server)
	params := map[string]interface{}{
		"applications": []central.ApplicationInfo{
			{Name: central.AppAIDataLiaison},
		},
	}
	result, err := service.Provision(ctx, projectID, "Non-MCP App Test", params)
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if !result.Created {
		t.Error("expected project to be created")
	}

	// Verify ontology was NOT created (MCP setup skipped)
	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer tenantScope.Close()

	var ontologyExists bool
	err = tenantScope.Conn.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM engine_ontologies WHERE project_id = $1 AND is_active = true)
	`, projectID).Scan(&ontologyExists)
	if err != nil {
		t.Fatalf("Failed to check ontology: %v", err)
	}
	if ontologyExists {
		t.Error("expected no ontology when mcp-server app is not specified")
	}
}

func cleanupProject(t *testing.T, engineDB *testhelpers.EngineDB, projectID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err == nil {
		_, _ = tenantScope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, projectID)
		tenantScope.Close()
	}

	scope, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_projects WHERE id = $1`, projectID)
}

// ensureTestProject creates a test project if it doesn't exist.
func ensureTestProject(t *testing.T, engineDB *testhelpers.EngineDB, projectID uuid.UUID, name string) {
	t.Helper()

	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, name)
	if err != nil {
		t.Fatalf("Failed to ensure test project: %v", err)
	}
}
