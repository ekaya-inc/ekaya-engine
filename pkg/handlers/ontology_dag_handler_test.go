package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockOntologyDAGService is a mock implementation for testing
type mockOntologyDAGService struct {
	startFunc     func(ctx context.Context, projectID, datasourceID uuid.UUID, projectOverview string) (*models.OntologyDAG, error)
	getStatusFunc func(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error)
	cancelFunc    func(ctx context.Context, dagID uuid.UUID) error
	deleteFunc    func(ctx context.Context, projectID uuid.UUID) error
}

func (m *mockOntologyDAGService) Start(ctx context.Context, projectID, datasourceID uuid.UUID, projectOverview string) (*models.OntologyDAG, error) {
	if m.startFunc != nil {
		return m.startFunc(ctx, projectID, datasourceID, projectOverview)
	}
	return nil, nil
}

func (m *mockOntologyDAGService) GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	if m.getStatusFunc != nil {
		return m.getStatusFunc(ctx, datasourceID)
	}
	return nil, nil
}

func (m *mockOntologyDAGService) Cancel(ctx context.Context, dagID uuid.UUID) error {
	if m.cancelFunc != nil {
		return m.cancelFunc(ctx, dagID)
	}
	return nil
}

func (m *mockOntologyDAGService) Delete(ctx context.Context, projectID uuid.UUID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, projectID)
	}
	return nil
}

func (m *mockOntologyDAGService) Shutdown(ctx context.Context) error {
	return nil
}

// mockProjectServiceForDAG is a minimal mock for the project service
type mockProjectServiceForDAG struct{}

func (m *mockProjectServiceForDAG) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (m *mockProjectServiceForDAG) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {
	// No-op for tests
}

func (m *mockProjectServiceForDAG) GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*services.AutoApproveSettings, error) {
	return nil, nil
}

func (m *mockProjectServiceForDAG) SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *services.AutoApproveSettings) error {
	return nil
}

func (m *mockProjectServiceForDAG) GetOntologySettings(ctx context.Context, projectID uuid.UUID) (*services.OntologySettings, error) {
	return &services.OntologySettings{UseLegacyPatternMatching: true}, nil
}

func (m *mockProjectServiceForDAG) SetOntologySettings(ctx context.Context, projectID uuid.UUID, settings *services.OntologySettings) error {
	return nil
}

func TestOntologyDAGHandler_StartExtraction_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	ontologyID := uuid.New()
	now := time.Now()
	currentNode := "KnowledgeSeeding"

	mockService := &mockOntologyDAGService{
		startFunc: func(ctx context.Context, pID, dsID uuid.UUID, overview string) (*models.OntologyDAG, error) {
			if pID != projectID {
				return nil, fmt.Errorf("unexpected project ID")
			}
			if dsID != datasourceID {
				return nil, fmt.Errorf("unexpected datasource ID")
			}
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				OntologyID:   &ontologyID,
				Status:       models.DAGStatusRunning,
				CurrentNode:  &currentNode,
				StartedAt:    &now,
				CreatedAt:    now,
				UpdatedAt:    now,
				Nodes: []models.DAGNode{
					{ID: uuid.New(), NodeName: "KnowledgeSeeding", NodeOrder: 1, Status: models.DAGNodeStatusRunning},
					{ID: uuid.New(), NodeName: "ColumnFeatureExtraction", NodeOrder: 2, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "FKDiscovery", NodeOrder: 3, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "TableFeatureExtraction", NodeOrder: 4, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "PKMatchDiscovery", NodeOrder: 5, Status: models.DAGNodeStatusPending},
				},
			}, nil
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/extract", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.StartExtraction(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.Success {
		t.Errorf("expected success to be true")
	}

	dataMap := response.Data.(map[string]any)
	if dataMap["dag_id"] != dagID.String() {
		t.Errorf("expected dag_id %s, got %s", dagID.String(), dataMap["dag_id"])
	}
	if dataMap["status"] != "running" {
		t.Errorf("expected status 'running', got '%s'", dataMap["status"])
	}
	if dataMap["current_node"] != currentNode {
		t.Errorf("expected current_node '%s', got '%s'", currentNode, dataMap["current_node"])
	}

	nodes := dataMap["nodes"].([]any)
	if len(nodes) != 5 {
		t.Errorf("expected 5 nodes, got %d", len(nodes))
	}
}

func TestOntologyDAGHandler_StartExtraction_ServiceError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockService := &mockOntologyDAGService{
		startFunc: func(ctx context.Context, pID, dsID uuid.UUID, overview string) (*models.OntologyDAG, error) {
			return nil, fmt.Errorf("database error")
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/extract", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.StartExtraction(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestOntologyDAGHandler_StartExtraction_WithProjectOverview(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	now := time.Now()
	currentNode := "KnowledgeSeeding"
	expectedOverview := "This is our e-commerce platform for B2B wholesale."

	var receivedOverview string
	mockService := &mockOntologyDAGService{
		startFunc: func(ctx context.Context, pID, dsID uuid.UUID, overview string) (*models.OntologyDAG, error) {
			receivedOverview = overview
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				Status:       models.DAGStatusRunning,
				CurrentNode:  &currentNode,
				StartedAt:    &now,
				CreatedAt:    now,
				UpdatedAt:    now,
				Nodes:        []models.DAGNode{},
			}, nil
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/extract", projectID, datasourceID)
	body := strings.NewReader(fmt.Sprintf(`{"project_overview": "%s"}`, expectedOverview))
	req := httptest.NewRequest(http.MethodPost, url, body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.StartExtraction(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	if receivedOverview != expectedOverview {
		t.Errorf("expected overview %q, got %q", expectedOverview, receivedOverview)
	}
}

func TestOntologyDAGHandler_StartExtraction_EmptyBody(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	now := time.Now()
	currentNode := "KnowledgeSeeding"

	var receivedOverview string
	mockService := &mockOntologyDAGService{
		startFunc: func(ctx context.Context, pID, dsID uuid.UUID, overview string) (*models.OntologyDAG, error) {
			receivedOverview = overview
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				Status:       models.DAGStatusRunning,
				CurrentNode:  &currentNode,
				StartedAt:    &now,
				CreatedAt:    now,
				UpdatedAt:    now,
				Nodes:        []models.DAGNode{},
			}, nil
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/extract", projectID, datasourceID)
	// Send request with nil body (empty POST)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.StartExtraction(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	// Verify that extraction started with empty overview (backward compatible)
	if receivedOverview != "" {
		t.Errorf("expected empty overview for nil body, got %q", receivedOverview)
	}
}

func TestOntologyDAGHandler_StartExtraction_MalformedJSON(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	now := time.Now()
	currentNode := "KnowledgeSeeding"

	var receivedOverview string
	mockService := &mockOntologyDAGService{
		startFunc: func(ctx context.Context, pID, dsID uuid.UUID, overview string) (*models.OntologyDAG, error) {
			receivedOverview = overview
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				Status:       models.DAGStatusRunning,
				CurrentNode:  &currentNode,
				StartedAt:    &now,
				CreatedAt:    now,
				UpdatedAt:    now,
				Nodes:        []models.DAGNode{},
			}, nil
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/extract", projectID, datasourceID)
	// Send malformed JSON - extraction should still proceed without overview
	body := strings.NewReader(`{"project_overview": invalid json`)
	req := httptest.NewRequest(http.MethodPost, url, body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.StartExtraction(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	// Verify that extraction started with empty overview despite malformed JSON
	if receivedOverview != "" {
		t.Errorf("expected empty overview for malformed JSON, got %q", receivedOverview)
	}
}

func TestOntologyDAGHandler_GetStatus_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	now := time.Now()
	currentNode := "ColumnFeatureExtraction"

	mockService := &mockOntologyDAGService{
		getStatusFunc: func(ctx context.Context, dsID uuid.UUID) (*models.OntologyDAG, error) {
			if dsID != datasourceID {
				return nil, fmt.Errorf("unexpected datasource ID")
			}
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				Status:       models.DAGStatusRunning,
				CurrentNode:  &currentNode,
				StartedAt:    &now,
				CreatedAt:    now,
				UpdatedAt:    now,
				Nodes: []models.DAGNode{
					{ID: uuid.New(), NodeName: "KnowledgeSeeding", NodeOrder: 1, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "ColumnFeatureExtraction", NodeOrder: 2, Status: models.DAGNodeStatusRunning, Progress: &models.DAGNodeProgress{Current: 5, Total: 15, Message: "Processing table users"}},
					{ID: uuid.New(), NodeName: "FKDiscovery", NodeOrder: 3, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "TableFeatureExtraction", NodeOrder: 4, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "PKMatchDiscovery", NodeOrder: 5, Status: models.DAGNodeStatusPending},
				},
			}, nil
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/dag", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.GetStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.Success {
		t.Errorf("expected success to be true")
	}

	dataMap := response.Data.(map[string]any)
	if dataMap["status"] != "running" {
		t.Errorf("expected status 'running', got '%s'", dataMap["status"])
	}
	if dataMap["current_node"] != currentNode {
		t.Errorf("expected current_node '%s', got '%s'", currentNode, dataMap["current_node"])
	}

	nodes := dataMap["nodes"].([]any)
	if len(nodes) != 5 {
		t.Errorf("expected 5 nodes, got %d", len(nodes))
	}

	// Check that KnowledgeSeeding is completed and ColumnFeatureExtraction is running
	firstNode := nodes[0].(map[string]any)
	if firstNode["name"] != "KnowledgeSeeding" {
		t.Errorf("expected first node name 'KnowledgeSeeding', got '%s'", firstNode["name"])
	}
	if firstNode["status"] != "completed" {
		t.Errorf("expected first node status 'completed', got '%s'", firstNode["status"])
	}

	secondNode := nodes[1].(map[string]any)
	if secondNode["name"] != "ColumnFeatureExtraction" {
		t.Errorf("expected second node name 'ColumnFeatureExtraction', got '%s'", secondNode["name"])
	}
	if secondNode["status"] != "running" {
		t.Errorf("expected second node status 'running', got '%s'", secondNode["status"])
	}

	// Check progress on running node
	progress := secondNode["progress"].(map[string]any)
	if progress["current"].(float64) != 5 {
		t.Errorf("expected progress current 5, got %v", progress["current"])
	}
	if progress["total"].(float64) != 15 {
		t.Errorf("expected progress total 15, got %v", progress["total"])
	}
	if progress["message"] != "Processing table users" {
		t.Errorf("expected progress message 'Processing table users', got '%s'", progress["message"])
	}
}

func TestOntologyDAGHandler_GetStatus_NoDAGExists(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockService := &mockOntologyDAGService{
		getStatusFunc: func(ctx context.Context, dsID uuid.UUID) (*models.OntologyDAG, error) {
			return nil, nil // No DAG exists
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/dag", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.GetStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.Success {
		t.Errorf("expected success to be true")
	}

	if response.Data != nil {
		t.Errorf("expected data to be nil when no DAG exists, got %v", response.Data)
	}
}

func TestOntologyDAGHandler_GetStatus_ServiceError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockService := &mockOntologyDAGService{
		getStatusFunc: func(ctx context.Context, dsID uuid.UUID) (*models.OntologyDAG, error) {
			return nil, fmt.Errorf("database error")
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/dag", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.GetStatus(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestOntologyDAGHandler_Cancel_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	now := time.Now()

	cancelCalled := false
	mockService := &mockOntologyDAGService{
		getStatusFunc: func(ctx context.Context, dsID uuid.UUID) (*models.OntologyDAG, error) {
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				Status:       models.DAGStatusRunning,
				CreatedAt:    now,
				UpdatedAt:    now,
			}, nil
		},
		cancelFunc: func(ctx context.Context, id uuid.UUID) error {
			if id != dagID {
				return fmt.Errorf("unexpected DAG ID")
			}
			cancelCalled = true
			return nil
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/dag/cancel", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Cancel(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	if !cancelCalled {
		t.Error("expected cancel to be called")
	}

	var response ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.Success {
		t.Errorf("expected success to be true")
	}
}

func TestOntologyDAGHandler_Cancel_NoDAGFound(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockService := &mockOntologyDAGService{
		getStatusFunc: func(ctx context.Context, dsID uuid.UUID) (*models.OntologyDAG, error) {
			return nil, nil // No DAG exists
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/dag/cancel", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Cancel(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestOntologyDAGHandler_Cancel_DAGNotRunning(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	now := time.Now()

	mockService := &mockOntologyDAGService{
		getStatusFunc: func(ctx context.Context, dsID uuid.UUID) (*models.OntologyDAG, error) {
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				Status:       models.DAGStatusCompleted, // Already completed
				CreatedAt:    now,
				UpdatedAt:    now,
			}, nil
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/dag/cancel", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Cancel(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestOntologyDAGHandler_Cancel_CancelError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	now := time.Now()

	mockService := &mockOntologyDAGService{
		getStatusFunc: func(ctx context.Context, dsID uuid.UUID) (*models.OntologyDAG, error) {
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				Status:       models.DAGStatusRunning,
				CreatedAt:    now,
				UpdatedAt:    now,
			}, nil
		},
		cancelFunc: func(ctx context.Context, id uuid.UUID) error {
			return fmt.Errorf("failed to cancel")
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/dag/cancel", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Cancel(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestOntologyDAGHandler_CompletedDAG(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	now := time.Now()
	completedAt := now.Add(5 * time.Minute)

	mockService := &mockOntologyDAGService{
		getStatusFunc: func(ctx context.Context, dsID uuid.UUID) (*models.OntologyDAG, error) {
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				Status:       models.DAGStatusCompleted,
				StartedAt:    &now,
				CompletedAt:  &completedAt,
				CreatedAt:    now,
				UpdatedAt:    completedAt,
				Nodes: []models.DAGNode{
					{ID: uuid.New(), NodeName: "KnowledgeSeeding", NodeOrder: 1, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "ColumnFeatureExtraction", NodeOrder: 2, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "FKDiscovery", NodeOrder: 3, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "TableFeatureExtraction", NodeOrder: 4, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "PKMatchDiscovery", NodeOrder: 5, Status: models.DAGNodeStatusCompleted},
				},
			}, nil
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/dag", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.GetStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	dataMap := response.Data.(map[string]any)
	if dataMap["status"] != "completed" {
		t.Errorf("expected status 'completed', got '%s'", dataMap["status"])
	}

	// All nodes should be completed
	nodes := dataMap["nodes"].([]any)
	for i, node := range nodes {
		nodeMap := node.(map[string]any)
		if nodeMap["status"] != "completed" {
			t.Errorf("expected node %d status 'completed', got '%s'", i, nodeMap["status"])
		}
	}

	// completed_at should be set
	if dataMap["completed_at"] == nil {
		t.Error("expected completed_at to be set for completed DAG")
	}
}

func TestOntologyDAGHandler_FailedDAG(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	now := time.Now()
	errMsg := "LLM request timed out"

	mockService := &mockOntologyDAGService{
		getStatusFunc: func(ctx context.Context, dsID uuid.UUID) (*models.OntologyDAG, error) {
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				Status:       models.DAGStatusFailed,
				StartedAt:    &now,
				CreatedAt:    now,
				UpdatedAt:    now,
				Nodes: []models.DAGNode{
					{ID: uuid.New(), NodeName: "KnowledgeSeeding", NodeOrder: 1, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "ColumnFeatureExtraction", NodeOrder: 2, Status: models.DAGNodeStatusFailed, ErrorMessage: &errMsg},
					{ID: uuid.New(), NodeName: "FKDiscovery", NodeOrder: 3, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "TableFeatureExtraction", NodeOrder: 4, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "PKMatchDiscovery", NodeOrder: 5, Status: models.DAGNodeStatusPending},
				},
			}, nil
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology/dag", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.GetStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	dataMap := response.Data.(map[string]any)
	if dataMap["status"] != "failed" {
		t.Errorf("expected status 'failed', got '%s'", dataMap["status"])
	}

	// Check that ColumnFeatureExtraction node has error message
	nodes := dataMap["nodes"].([]any)
	secondNode := nodes[1].(map[string]any)
	if secondNode["status"] != "failed" {
		t.Errorf("expected second node status 'failed', got '%s'", secondNode["status"])
	}
	if secondNode["error"] != errMsg {
		t.Errorf("expected error message '%s', got '%s'", errMsg, secondNode["error"])
	}
}

// ============================================================================
// Delete Handler Tests
// ============================================================================

func TestOntologyDAGHandler_Delete_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	deleteCalled := false
	mockService := &mockOntologyDAGService{
		deleteFunc: func(ctx context.Context, pID uuid.UUID) error {
			if pID != projectID {
				return fmt.Errorf("unexpected project ID")
			}
			deleteCalled = true
			return nil
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	if !deleteCalled {
		t.Error("expected delete to be called")
	}

	var response ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.Success {
		t.Errorf("expected success to be true")
	}

	dataMap := response.Data.(map[string]any)
	if dataMap["message"] != "Ontology deleted successfully" {
		t.Errorf("expected success message, got '%s'", dataMap["message"])
	}
}

func TestOntologyDAGHandler_Delete_ServiceError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockService := &mockOntologyDAGService{
		deleteFunc: func(ctx context.Context, pID uuid.UUID) error {
			return fmt.Errorf("delete failed")
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestOntologyDAGHandler_Delete_RunningDAGError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockService := &mockOntologyDAGService{
		deleteFunc: func(ctx context.Context, pID uuid.UUID) error {
			return fmt.Errorf("cannot delete ontology while extraction is running")
		},
	}

	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/ontology", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["message"] != "cannot delete ontology while extraction is running" {
		t.Errorf("expected error message about running extraction, got '%s'", response["message"])
	}
}

func TestOntologyDAGHandler_Delete_InvalidProjectID(t *testing.T) {
	datasourceID := uuid.New()

	mockService := &mockOntologyDAGService{}
	handler := NewOntologyDAGHandler(mockService, nil, zap.NewNop())

	url := fmt.Sprintf("/api/projects/invalid-uuid/datasources/%s/ontology", datasourceID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.SetPathValue("pid", "invalid-uuid")
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}
