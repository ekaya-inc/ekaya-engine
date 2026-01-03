package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockOntologyDAGService is a mock implementation for testing
type mockOntologyDAGService struct {
	startFunc     func(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyDAG, error)
	getStatusFunc func(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error)
	cancelFunc    func(ctx context.Context, dagID uuid.UUID) error
}

func (m *mockOntologyDAGService) Start(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	if m.startFunc != nil {
		return m.startFunc(ctx, projectID, datasourceID)
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

func (m *mockOntologyDAGService) Shutdown(ctx context.Context) error {
	return nil
}

// mockProjectServiceForDAG is a minimal mock for the project service
type mockProjectServiceForDAG struct{}

func (m *mockProjectServiceForDAG) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}

func TestOntologyDAGHandler_StartExtraction_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	ontologyID := uuid.New()
	now := time.Now()
	currentNode := "EntityDiscovery"

	mockService := &mockOntologyDAGService{
		startFunc: func(ctx context.Context, pID, dsID uuid.UUID) (*models.OntologyDAG, error) {
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
					{ID: uuid.New(), NodeName: "EntityDiscovery", NodeOrder: 1, Status: models.DAGNodeStatusRunning},
					{ID: uuid.New(), NodeName: "EntityEnrichment", NodeOrder: 2, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "RelationshipDiscovery", NodeOrder: 3, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "RelationshipEnrichment", NodeOrder: 4, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "OntologyFinalization", NodeOrder: 5, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "ColumnEnrichment", NodeOrder: 6, Status: models.DAGNodeStatusPending},
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
	if len(nodes) != 6 {
		t.Errorf("expected 6 nodes, got %d", len(nodes))
	}
}

func TestOntologyDAGHandler_StartExtraction_ServiceError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockService := &mockOntologyDAGService{
		startFunc: func(ctx context.Context, pID, dsID uuid.UUID) (*models.OntologyDAG, error) {
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

func TestOntologyDAGHandler_GetStatus_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	now := time.Now()
	currentNode := "EntityEnrichment"

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
					{ID: uuid.New(), NodeName: "EntityDiscovery", NodeOrder: 1, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "EntityEnrichment", NodeOrder: 2, Status: models.DAGNodeStatusRunning, Progress: &models.DAGNodeProgress{Current: 5, Total: 15, Message: "Processing table users"}},
					{ID: uuid.New(), NodeName: "RelationshipDiscovery", NodeOrder: 3, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "RelationshipEnrichment", NodeOrder: 4, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "OntologyFinalization", NodeOrder: 5, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "ColumnEnrichment", NodeOrder: 6, Status: models.DAGNodeStatusPending},
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
	if len(nodes) != 6 {
		t.Errorf("expected 6 nodes, got %d", len(nodes))
	}

	// Check that EntityDiscovery is completed and EntityEnrichment is running
	firstNode := nodes[0].(map[string]any)
	if firstNode["name"] != "EntityDiscovery" {
		t.Errorf("expected first node name 'EntityDiscovery', got '%s'", firstNode["name"])
	}
	if firstNode["status"] != "completed" {
		t.Errorf("expected first node status 'completed', got '%s'", firstNode["status"])
	}

	secondNode := nodes[1].(map[string]any)
	if secondNode["name"] != "EntityEnrichment" {
		t.Errorf("expected second node name 'EntityEnrichment', got '%s'", secondNode["name"])
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
					{ID: uuid.New(), NodeName: "EntityDiscovery", NodeOrder: 1, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "EntityEnrichment", NodeOrder: 2, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "RelationshipDiscovery", NodeOrder: 3, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "RelationshipEnrichment", NodeOrder: 4, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "OntologyFinalization", NodeOrder: 5, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "ColumnEnrichment", NodeOrder: 6, Status: models.DAGNodeStatusCompleted},
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
					{ID: uuid.New(), NodeName: "EntityDiscovery", NodeOrder: 1, Status: models.DAGNodeStatusCompleted},
					{ID: uuid.New(), NodeName: "EntityEnrichment", NodeOrder: 2, Status: models.DAGNodeStatusFailed, ErrorMessage: &errMsg},
					{ID: uuid.New(), NodeName: "RelationshipDiscovery", NodeOrder: 3, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "RelationshipEnrichment", NodeOrder: 4, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "OntologyFinalization", NodeOrder: 5, Status: models.DAGNodeStatusPending},
					{ID: uuid.New(), NodeName: "ColumnEnrichment", NodeOrder: 6, Status: models.DAGNodeStatusPending},
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

	// Check that EntityEnrichment node has error message
	nodes := dataMap["nodes"].([]any)
	secondNode := nodes[1].(map[string]any)
	if secondNode["status"] != "failed" {
		t.Errorf("expected second node status 'failed', got '%s'", secondNode["status"])
	}
	if secondNode["error"] != errMsg {
		t.Errorf("expected error message '%s', got '%s'", errMsg, secondNode["error"])
	}
}
