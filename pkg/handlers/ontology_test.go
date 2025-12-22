package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ============================================================================
// Mock Workflow Service for Handler Tests
// ============================================================================

type mockOntologyWorkflowService struct {
	startExtractionFunc  func(ctx context.Context, projectID uuid.UUID, config *models.WorkflowConfig) (*models.OntologyWorkflow, error)
	lastConfig           *models.WorkflowConfig // Captures the config passed to StartExtraction
	schemaEntityCount    int                    // Return value for GetSchemaEntityCount
	schemaEntityCountErr error                  // Error return for GetSchemaEntityCount
	workflow             *models.OntologyWorkflow
	ontology             *models.TieredOntology
}

func (m *mockOntologyWorkflowService) StartExtraction(ctx context.Context, projectID uuid.UUID, config *models.WorkflowConfig) (*models.OntologyWorkflow, error) {
	m.lastConfig = config // Capture for verification
	if m.startExtractionFunc != nil {
		return m.startExtractionFunc(ctx, projectID, config)
	}
	return &models.OntologyWorkflow{
		ID:        uuid.New(),
		ProjectID: projectID,
		State:     models.WorkflowStatePending,
		Config:    config,
	}, nil
}

func (m *mockOntologyWorkflowService) GetStatus(ctx context.Context, projectID uuid.UUID) (*models.OntologyWorkflow, error) {
	return m.workflow, nil
}

func (m *mockOntologyWorkflowService) GetOntology(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	return m.ontology, nil
}

func (m *mockOntologyWorkflowService) Cancel(ctx context.Context, workflowID uuid.UUID) error {
	return nil
}

func (m *mockOntologyWorkflowService) UpdateProgress(ctx context.Context, workflowID uuid.UUID, progress *models.WorkflowProgress) error {
	return nil
}

func (m *mockOntologyWorkflowService) MarkComplete(ctx context.Context, workflowID uuid.UUID) error {
	return nil
}

func (m *mockOntologyWorkflowService) MarkFailed(ctx context.Context, workflowID uuid.UUID, errMsg string) error {
	return nil
}

func (m *mockOntologyWorkflowService) GetByID(ctx context.Context, workflowID uuid.UUID) (*models.OntologyWorkflow, error) {
	return nil, nil
}

func (m *mockOntologyWorkflowService) DeleteOntology(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockOntologyWorkflowService) Shutdown(ctx context.Context) error {
	return nil
}

func (m *mockOntologyWorkflowService) GetSchemaEntityCount(ctx context.Context, projectID uuid.UUID) (int, error) {
	return m.schemaEntityCount, m.schemaEntityCountErr
}

// ============================================================================
// Test Helpers
// ============================================================================

func newTestOntologyHandler() (*OntologyHandler, *mockOntologyWorkflowService, *mockProjectService) {
	logger := zap.NewNop()
	mockWorkflowService := &mockOntologyWorkflowService{}
	mockProjService := &mockProjectService{defaultDatasourceID: uuid.New()}
	handler := NewOntologyHandler(mockWorkflowService, mockProjService, logger)
	return handler, mockWorkflowService, mockProjService
}

// createRequestWithProjectID creates an HTTP request with project ID in the path.
// Uses Go 1.22+ path patterns: /api/projects/{pid}/ontology/extract
func createRequestWithProjectID(method, path string, body []byte, projectID uuid.UUID) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	// Set the path value as if parsed by Go 1.22+ ServeMux
	req.SetPathValue("pid", projectID.String())
	return req
}

// ============================================================================
// StartExtraction Tests - Datasource from Project Configuration
// ============================================================================

// TestStartExtraction_WithProjectDatasource verifies that extraction uses
// the datasource configured at the project level.
func TestStartExtraction_WithProjectDatasource(t *testing.T) {
	handler, mockWorkflowService, mockProjService := newTestOntologyHandler()
	projectID := uuid.New()
	datasourceID := uuid.New()
	mockProjService.defaultDatasourceID = datasourceID
	description := "E-commerce platform for selling widgets and gadgets"

	reqBody := StartExtractionRequest{
		ProjectDescription: description,
	}
	body, _ := json.Marshal(reqBody)

	req := createRequestWithProjectID("POST", "/api/projects/"+projectID.String()+"/ontology/extract", body, projectID)
	rr := httptest.NewRecorder()

	handler.StartExtraction(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify project_description was passed to the service
	if mockWorkflowService.lastConfig == nil {
		t.Fatal("expected config to be passed to service, got nil")
	}

	if mockWorkflowService.lastConfig.ProjectDescription != description {
		t.Errorf("expected project_description %q, got %q",
			description, mockWorkflowService.lastConfig.ProjectDescription)
	}

	// Verify datasource_id was fetched from project config
	if mockWorkflowService.lastConfig.DatasourceID != datasourceID {
		t.Errorf("expected datasource_id %s from project config, got %s",
			datasourceID, mockWorkflowService.lastConfig.DatasourceID)
	}
}

// TestStartExtraction_NoDatasourceConfigured verifies that the handler
// returns a 400 error when no datasource is configured for the project.
func TestStartExtraction_NoDatasourceConfigured(t *testing.T) {
	handler, mockWorkflowService, mockProjService := newTestOntologyHandler()
	mockProjService.defaultDatasourceID = uuid.Nil // No datasource configured
	projectID := uuid.New()
	description := "Healthcare data management system for patient records"

	reqBody := StartExtractionRequest{
		ProjectDescription: description,
	}
	body, _ := json.Marshal(reqBody)

	req := createRequestWithProjectID("POST", "/api/projects/"+projectID.String()+"/ontology/extract", body, projectID)
	rr := httptest.NewRecorder()

	handler.StartExtraction(rr, req)

	// Should fail with 400 Bad Request
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 when no datasource configured, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify the error response contains the right error code
	var errResp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errResp["error"] != "no_datasource_configured" {
		t.Errorf("expected error code 'no_datasource_configured', got %q", errResp["error"])
	}

	// Service should NOT have been called
	if mockWorkflowService.lastConfig != nil {
		t.Error("service should not have been called when no datasource is configured")
	}
}

// TestStartExtraction_EmptyBody_Success verifies the handler accepts empty body
// since datasource_id comes from project config now.
func TestStartExtraction_EmptyBody_Success(t *testing.T) {
	handler, mockWorkflowService, mockProjService := newTestOntologyHandler()
	projectID := uuid.New()
	datasourceID := uuid.New()
	mockProjService.defaultDatasourceID = datasourceID

	req := createRequestWithProjectID("POST", "/api/projects/"+projectID.String()+"/ontology/extract", nil, projectID)
	rr := httptest.NewRecorder()

	handler.StartExtraction(rr, req)

	// Should succeed - datasource comes from project config
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify config was passed to service with datasource from project
	if mockWorkflowService.lastConfig == nil {
		t.Fatal("expected config to be passed to service")
	}
	if mockWorkflowService.lastConfig.DatasourceID != datasourceID {
		t.Errorf("expected datasource_id %s, got %s", datasourceID, mockWorkflowService.lastConfig.DatasourceID)
	}
}

// ============================================================================
// GetWorkflowStatus Tests - Entity Count Behavior
// ============================================================================

// TestGetWorkflowStatus_NoWorkflow_HasOntologyData verifies that when no workflow
// exists but ontology data exists, the response includes entity counts from schema.
func TestGetWorkflowStatus_NoWorkflow_HasOntologyData(t *testing.T) {
	handler, mockWorkflowService, _ := newTestOntologyHandler()
	projectID := uuid.New()

	// No workflow, but ontology with 38 entity summaries exists
	mockWorkflowService.workflow = nil
	mockWorkflowService.ontology = &models.TieredOntology{
		ID:        uuid.New(),
		ProjectID: projectID,
		EntitySummaries: map[string]*models.EntitySummary{
			"users":    {TableName: "users"},
			"orders":   {TableName: "orders"},
			"products": {TableName: "products"},
		},
	}
	mockWorkflowService.schemaEntityCount = 100 // Total from schema

	req := createRequestWithProjectID("GET", "/api/projects/"+projectID.String()+"/ontology/workflow", nil, projectID)
	rr := httptest.NewRecorder()

	handler.GetWorkflowStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response ApiResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map, got %T", response.Data)
	}

	// Verify ontology_ready and has_result are true
	if data["ontology_ready"] != true {
		t.Errorf("expected ontology_ready=true, got %v", data["ontology_ready"])
	}
	if data["has_result"] != true {
		t.Errorf("expected has_result=true, got %v", data["has_result"])
	}

	// Verify entity counts: current=3 (ontology summaries), total=100 (from schema)
	if data["current_entity"].(float64) != 3 {
		t.Errorf("expected current_entity=3, got %v", data["current_entity"])
	}
	if data["total_entities"].(float64) != 100 {
		t.Errorf("expected total_entities=100, got %v", data["total_entities"])
	}
}

// TestGetWorkflowStatus_NoWorkflow_NoOntology verifies that when neither workflow
// nor ontology exists, the response indicates idle state.
func TestGetWorkflowStatus_NoWorkflow_NoOntology(t *testing.T) {
	handler, mockWorkflowService, _ := newTestOntologyHandler()
	projectID := uuid.New()

	// No workflow, no ontology
	mockWorkflowService.workflow = nil
	mockWorkflowService.ontology = nil
	mockWorkflowService.schemaEntityCount = 50

	req := createRequestWithProjectID("GET", "/api/projects/"+projectID.String()+"/ontology/workflow", nil, projectID)
	rr := httptest.NewRecorder()

	handler.GetWorkflowStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response ApiResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map, got %T", response.Data)
	}

	// Verify ontology_ready and has_result are false (or omitted due to omitempty)
	if val, ok := data["ontology_ready"]; ok && val != false {
		t.Errorf("expected ontology_ready=false or omitted, got %v", val)
	}
	if data["has_result"] != false {
		t.Errorf("expected has_result=false, got %v", data["has_result"])
	}

	// Verify can_start_new is true (user can start extraction)
	if data["can_start_new"] != true {
		t.Errorf("expected can_start_new=true, got %v", data["can_start_new"])
	}
}

// TestGetWorkflowStatus_EntityCountFallback verifies that when schema entity count
// fails, the handler falls back to using current entity count.
func TestGetWorkflowStatus_EntityCountFallback(t *testing.T) {
	handler, mockWorkflowService, _ := newTestOntologyHandler()
	projectID := uuid.New()

	// No workflow, but ontology exists
	mockWorkflowService.workflow = nil
	mockWorkflowService.ontology = &models.TieredOntology{
		ID:        uuid.New(),
		ProjectID: projectID,
		EntitySummaries: map[string]*models.EntitySummary{
			"users":  {TableName: "users"},
			"orders": {TableName: "orders"},
		},
	}
	// Schema count fails (e.g., no datasource configured)
	mockWorkflowService.schemaEntityCount = 0
	mockWorkflowService.schemaEntityCountErr = fmt.Errorf("no datasource configured")

	req := createRequestWithProjectID("GET", "/api/projects/"+projectID.String()+"/ontology/workflow", nil, projectID)
	rr := httptest.NewRecorder()

	handler.GetWorkflowStatus(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var response ApiResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map, got %T", response.Data)
	}

	// When schema count fails, total should fall back to current (2)
	if data["current_entity"].(float64) != 2 {
		t.Errorf("expected current_entity=2, got %v", data["current_entity"])
	}
	if data["total_entities"].(float64) != 2 {
		t.Errorf("expected total_entities=2 (fallback), got %v", data["total_entities"])
	}
}
