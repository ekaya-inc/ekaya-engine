//go:build integration

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// parseAPIResponse extracts the Data field from an ApiResponse wrapper.
// The handler wraps all responses in ApiResponse{Success: bool, Data: any}.
func parseAPIResponse[T any](t *testing.T, body []byte) T {
	t.Helper()
	var wrapper struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		t.Fatalf("failed to parse ApiResponse wrapper: %v", err)
	}
	if !wrapper.Success {
		t.Fatalf("expected success=true, got false. Body: %s", string(body))
	}
	var result T
	if err := json.Unmarshal(wrapper.Data, &result); err != nil {
		t.Fatalf("failed to parse Data field: %v", err)
	}
	return result
}

// entityTestContext holds all dependencies for entity integration tests.
type entityTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	handler      *EntityHandler
	service      services.EntityService
	entityRepo   repositories.OntologyEntityRepository
	ontologyRepo repositories.OntologyRepository
	projectID    uuid.UUID
	ontologyID   uuid.UUID
}

// setupEntityTest creates a test context with real database and services.
func setupEntityTest(t *testing.T) *entityTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)

	// Create repositories
	entityRepo := repositories.NewOntologyEntityRepository()
	relationshipRepo := repositories.NewEntityRelationshipRepository()
	ontologyRepo := repositories.NewOntologyRepository()

	// Create service
	service := services.NewEntityService(entityRepo, relationshipRepo, ontologyRepo, zap.NewNop())

	// Create handler
	handler := NewEntityHandler(service, zap.NewNop())

	// Use a fixed project ID for consistent testing
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	return &entityTestContext{
		t:            t,
		engineDB:     engineDB,
		handler:      handler,
		service:      service,
		entityRepo:   entityRepo,
		ontologyRepo: ontologyRepo,
		projectID:    projectID,
	}
}

// makeEntityRequest creates an HTTP request with proper context and executes the handler.
// Returns the response recorder. This handles scope lifecycle properly.
func (tc *entityTestContext) doRequest(method, path string, body any, handler http.HandlerFunc, pathValues map[string]string) *httptest.ResponseRecorder {
	tc.t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			tc.t.Fatalf("Failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewReader(bodyBytes)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")

	// Set path values
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}

	// Set up tenant scope
	ctx := req.Context()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close() // Close immediately after handler returns

	ctx = database.SetTenantScope(ctx, scope)

	// Set up auth claims
	claims := &auth.Claims{ProjectID: tc.projectID.String()}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)

	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler(rec, req)

	return rec
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *entityTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Entity Integration Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// createTestOntology creates an active ontology for testing and stores its ID.
func (tc *entityTestContext) createTestOntology() {
	tc.t.Helper()

	tc.ensureTestProject()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	defer scope.Close()
	ctx = database.SetTenantScope(ctx, scope)

	// Create ontology
	tc.ontologyID = uuid.New()
	ontology := &models.TieredOntology{
		ID:              tc.ontologyID,
		ProjectID:       tc.projectID,
		Version:         1,
		IsActive:        true,
		EntitySummaries: make(map[string]*models.EntitySummary),
		ColumnDetails:   make(map[string][]models.ColumnDetail),
		Metadata:        make(map[string]any),
	}

	if err := tc.ontologyRepo.Create(ctx, ontology); err != nil {
		tc.t.Fatalf("Failed to create ontology: %v", err)
	}
}

// cleanup removes test data.
func (tc *entityTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Delete in order respecting foreign keys
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entity_aliases WHERE entity_id IN (SELECT id FROM engine_ontology_entities WHERE project_id = $1)", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entity_occurrences WHERE entity_id IN (SELECT id FROM engine_ontology_entities WHERE project_id = $1)", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_entities WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", tc.projectID)
}

func TestEntityIntegration_ListEmpty(t *testing.T) {
	tc := setupEntityTest(t)
	tc.cleanup()
	tc.createTestOntology()

	rec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/entities", nil,
		tc.handler.List, map[string]string{"pid": tc.projectID.String()})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := parseAPIResponse[EntityListResponse](t, rec.Body.Bytes())

	if resp.Total != 0 {
		t.Errorf("expected 0 entities, got %d", resp.Total)
	}

	if len(resp.Entities) != 0 {
		t.Errorf("expected empty entities list, got %d", len(resp.Entities))
	}
}

func TestEntityIntegration_CreateAndList(t *testing.T) {
	tc := setupEntityTest(t)
	tc.cleanup()
	tc.createTestOntology()

	// Create entity via repository (simulating what the discovery workflow does)
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)

	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "user",
		Description:   "Represents a user in the system",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
	}

	if err := tc.entityRepo.Create(ctx, entity); err != nil {
		scope.Close()
		t.Fatalf("Failed to create entity: %v", err)
	}

	// Add an occurrence
	occurrence := &models.OntologyEntityOccurrence{
		EntityID:   entity.ID,
		SchemaName: "public",
		TableName:  "users",
		ColumnName: "id",
		Confidence: 1.0,
	}
	if err := tc.entityRepo.CreateOccurrence(ctx, occurrence); err != nil {
		scope.Close()
		t.Fatalf("Failed to create occurrence: %v", err)
	}

	// Add an alias
	source := "discovery"
	alias := &models.OntologyEntityAlias{
		EntityID: entity.ID,
		Alias:    "customer",
		Source:   &source,
	}
	if err := tc.entityRepo.CreateAlias(ctx, alias); err != nil {
		scope.Close()
		t.Fatalf("Failed to create alias: %v", err)
	}
	scope.Close() // Close before making HTTP request

	// Now fetch via HTTP handler
	rec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/entities", nil,
		tc.handler.List, map[string]string{"pid": tc.projectID.String()})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := parseAPIResponse[EntityListResponse](t, rec.Body.Bytes())

	if resp.Total != 1 {
		t.Fatalf("expected 1 entity, got %d", resp.Total)
	}

	e := resp.Entities[0]
	if e.Name != "user" {
		t.Errorf("expected name 'user', got %q", e.Name)
	}
	if e.Description != "Represents a user in the system" {
		t.Errorf("expected description, got %q", e.Description)
	}
	if e.PrimaryTable != "users" {
		t.Errorf("expected primary_table 'users', got %q", e.PrimaryTable)
	}
	if e.OccurrenceCount != 1 {
		t.Errorf("expected 1 occurrence, got %d", e.OccurrenceCount)
	}
	if len(e.Occurrences) != 1 {
		t.Errorf("expected 1 occurrence in list, got %d", len(e.Occurrences))
	}
	if len(e.Aliases) != 1 {
		t.Errorf("expected 1 alias, got %d", len(e.Aliases))
	}
	if e.Aliases[0].Alias != "customer" {
		t.Errorf("expected alias 'customer', got %q", e.Aliases[0].Alias)
	}
	if e.IsDeleted {
		t.Error("expected is_deleted to be false")
	}
}

func TestEntityIntegration_GetByID(t *testing.T) {
	tc := setupEntityTest(t)
	tc.cleanup()
	tc.createTestOntology()

	// Create entity
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)

	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "order",
		Description:   "A customer order",
		PrimarySchema: "public",
		PrimaryTable:  "orders",
		PrimaryColumn: "id",
	}
	if err := tc.entityRepo.Create(ctx, entity); err != nil {
		scope.Close()
		t.Fatalf("Failed to create entity: %v", err)
	}
	scope.Close()

	// Fetch single entity via HTTP
	rec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/entities/"+entity.ID.String(), nil,
		tc.handler.Get, map[string]string{"pid": tc.projectID.String(), "eid": entity.ID.String()})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := parseAPIResponse[EntityDetailResponse](t, rec.Body.Bytes())

	if resp.ID != entity.ID.String() {
		t.Errorf("expected ID %s, got %s", entity.ID.String(), resp.ID)
	}
	if resp.Name != "order" {
		t.Errorf("expected name 'order', got %q", resp.Name)
	}
}

func TestEntityIntegration_SoftDeleteAndRestore(t *testing.T) {
	tc := setupEntityTest(t)
	tc.cleanup()
	tc.createTestOntology()

	// Create entity
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)

	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "product",
		Description:   "A product",
		PrimarySchema: "public",
		PrimaryTable:  "products",
		PrimaryColumn: "id",
	}
	if err := tc.entityRepo.Create(ctx, entity); err != nil {
		scope.Close()
		t.Fatalf("Failed to create entity: %v", err)
	}
	scope.Close()

	pathValues := map[string]string{"pid": tc.projectID.String(), "eid": entity.ID.String()}

	// Delete entity via HTTP
	deleteRec := tc.doRequest(http.MethodDelete, "/api/projects/"+tc.projectID.String()+"/entities/"+entity.ID.String(),
		DeleteEntityRequest{Reason: "not needed"}, tc.handler.Delete, pathValues)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for delete, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	// Verify entity is not in list (soft deleted)
	listRec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/entities", nil,
		tc.handler.List, map[string]string{"pid": tc.projectID.String()})

	listResp := parseAPIResponse[EntityListResponse](t, listRec.Body.Bytes())
	if listResp.Total != 0 {
		t.Errorf("expected 0 entities after delete, got %d", listResp.Total)
	}

	// But can still get by ID (for restore preview)
	getRec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/entities/"+entity.ID.String(), nil,
		tc.handler.Get, pathValues)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for get deleted, got %d", getRec.Code)
	}

	getResp := parseAPIResponse[EntityDetailResponse](t, getRec.Body.Bytes())
	if !getResp.IsDeleted {
		t.Error("expected is_deleted to be true")
	}
	if getResp.DeletionReason == nil || *getResp.DeletionReason != "not needed" {
		t.Errorf("expected deletion_reason 'not needed', got %v", getResp.DeletionReason)
	}

	// Restore entity
	restoreRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/entities/"+entity.ID.String()+"/restore", nil,
		tc.handler.Restore, pathValues)

	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for restore, got %d: %s", restoreRec.Code, restoreRec.Body.String())
	}

	restoreResp := parseAPIResponse[EntityDetailResponse](t, restoreRec.Body.Bytes())
	if restoreResp.IsDeleted {
		t.Error("expected is_deleted to be false after restore")
	}

	// Verify entity is back in list
	listRec2 := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/entities", nil,
		tc.handler.List, map[string]string{"pid": tc.projectID.String()})

	listResp2 := parseAPIResponse[EntityListResponse](t, listRec2.Body.Bytes())
	if listResp2.Total != 1 {
		t.Errorf("expected 1 entity after restore, got %d", listResp2.Total)
	}
}

func TestEntityIntegration_AddAndRemoveAlias(t *testing.T) {
	tc := setupEntityTest(t)
	tc.cleanup()
	tc.createTestOntology()

	// Create entity
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)

	entity := &models.OntologyEntity{
		ProjectID:     tc.projectID,
		OntologyID:    tc.ontologyID,
		Name:          "account",
		Description:   "A user account",
		PrimarySchema: "public",
		PrimaryTable:  "accounts",
		PrimaryColumn: "id",
	}
	if err := tc.entityRepo.Create(ctx, entity); err != nil {
		scope.Close()
		t.Fatalf("Failed to create entity: %v", err)
	}
	scope.Close()

	pathValues := map[string]string{"pid": tc.projectID.String(), "eid": entity.ID.String()}

	// Add alias via HTTP
	addRec := tc.doRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/entities/"+entity.ID.String()+"/aliases",
		AddAliasRequest{Alias: "profile", Source: "user"}, tc.handler.AddAlias, pathValues)

	if addRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 for add alias, got %d: %s", addRec.Code, addRec.Body.String())
	}

	aliasResp := parseAPIResponse[EntityAliasResponse](t, addRec.Body.Bytes())
	if aliasResp.Alias != "profile" {
		t.Errorf("expected alias 'profile', got %q", aliasResp.Alias)
	}
	if aliasResp.Source == nil || *aliasResp.Source != "user" {
		t.Errorf("expected source 'user', got %v", aliasResp.Source)
	}

	// Remove alias via HTTP
	removePathValues := map[string]string{"pid": tc.projectID.String(), "eid": entity.ID.String(), "aid": aliasResp.ID}
	removeRec := tc.doRequest(http.MethodDelete, "/api/projects/"+tc.projectID.String()+"/entities/"+entity.ID.String()+"/aliases/"+aliasResp.ID,
		nil, tc.handler.RemoveAlias, removePathValues)

	if removeRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for remove alias, got %d: %s", removeRec.Code, removeRec.Body.String())
	}

	// Verify alias is gone
	getRec := tc.doRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/entities/"+entity.ID.String(), nil,
		tc.handler.Get, pathValues)

	getResp := parseAPIResponse[EntityDetailResponse](t, getRec.Body.Bytes())
	if len(getResp.Aliases) != 0 {
		t.Errorf("expected 0 aliases after remove, got %d", len(getResp.Aliases))
	}
}
