package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockRelationshipWorkflowService is a mock implementation for testing
type mockRelationshipWorkflowService struct {
	getEntitiesWithOccurrencesFunc func(ctx context.Context, datasourceID uuid.UUID) ([]*services.EntityWithOccurrences, error)
	getStatusWithCountsFunc        func(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, *services.CandidateCounts, error)
}

func (m *mockRelationshipWorkflowService) StartDetection(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyWorkflow, error) {
	return nil, nil
}

func (m *mockRelationshipWorkflowService) GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, error) {
	return nil, nil
}

func (m *mockRelationshipWorkflowService) GetStatusWithCounts(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, *services.CandidateCounts, error) {
	if m.getStatusWithCountsFunc != nil {
		return m.getStatusWithCountsFunc(ctx, datasourceID)
	}
	return nil, nil, nil
}

func (m *mockRelationshipWorkflowService) GetByID(ctx context.Context, workflowID uuid.UUID) (*models.OntologyWorkflow, error) {
	return nil, nil
}

func (m *mockRelationshipWorkflowService) GetCandidatesGrouped(ctx context.Context, datasourceID uuid.UUID) (*services.CandidatesGrouped, error) {
	return nil, nil
}

func (m *mockRelationshipWorkflowService) GetEntitiesWithOccurrences(ctx context.Context, datasourceID uuid.UUID) ([]*services.EntityWithOccurrences, error) {
	if m.getEntitiesWithOccurrencesFunc != nil {
		return m.getEntitiesWithOccurrencesFunc(ctx, datasourceID)
	}
	return nil, nil
}

func (m *mockRelationshipWorkflowService) UpdateCandidateDecision(ctx context.Context, datasourceID, candidateID uuid.UUID, decision string) (*models.RelationshipCandidate, error) {
	return nil, nil
}

func (m *mockRelationshipWorkflowService) Cancel(ctx context.Context, workflowID uuid.UUID) error {
	return nil
}

func (m *mockRelationshipWorkflowService) SaveRelationships(ctx context.Context, workflowID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockRelationshipWorkflowService) UpdateProgress(ctx context.Context, workflowID uuid.UUID, progress *models.WorkflowProgress) error {
	return nil
}

func (m *mockRelationshipWorkflowService) MarkComplete(ctx context.Context, workflowID uuid.UUID) error {
	return nil
}

func (m *mockRelationshipWorkflowService) MarkFailed(ctx context.Context, workflowID uuid.UUID, errMsg string) error {
	return nil
}

func (m *mockRelationshipWorkflowService) Shutdown(ctx context.Context) error {
	return nil
}

func TestRelationshipWorkflowHandler_GetEntities_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	entityID := uuid.New()
	occurrenceID := uuid.New()

	role := "visitor"
	mockService := &mockRelationshipWorkflowService{
		getEntitiesWithOccurrencesFunc: func(ctx context.Context, dsID uuid.UUID) ([]*services.EntityWithOccurrences, error) {
			if dsID != datasourceID {
				return nil, fmt.Errorf("unexpected datasource ID")
			}
			return []*services.EntityWithOccurrences{
				{
					Entity: &models.OntologyEntity{
						ID:            entityID,
						ProjectID:     projectID,
						OntologyID:    uuid.New(),
						Name:          "user",
						Description:   "A person who uses the system",
						PrimarySchema: "public",
						PrimaryTable:  "users",
						PrimaryColumn: "id",
					},
					Occurrences: []*models.OntologyEntityOccurrence{
						{
							ID:         occurrenceID,
							EntityID:   entityID,
							SchemaName: "public",
							TableName:  "visits",
							ColumnName: "visitor_id",
							Role:       &role,
							Confidence: 1.0,
						},
					},
				},
			}, nil
		},
	}

	handler := NewRelationshipWorkflowHandler(mockService, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/relationships/entities", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.GetEntities(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response EntitiesResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(response.Entities))
	}

	entity := response.Entities[0]
	if entity.Name != "user" {
		t.Errorf("expected entity name 'user', got '%s'", entity.Name)
	}
	if entity.Description != "A person who uses the system" {
		t.Errorf("expected description 'A person who uses the system', got '%s'", entity.Description)
	}
	if entity.PrimaryTable != "users" {
		t.Errorf("expected primary table 'users', got '%s'", entity.PrimaryTable)
	}
	if len(entity.Occurrences) != 1 {
		t.Fatalf("expected 1 occurrence, got %d", len(entity.Occurrences))
	}

	occurrence := entity.Occurrences[0]
	if occurrence.TableName != "visits" {
		t.Errorf("expected table name 'visits', got '%s'", occurrence.TableName)
	}
	if occurrence.ColumnName != "visitor_id" {
		t.Errorf("expected column name 'visitor_id', got '%s'", occurrence.ColumnName)
	}
	if occurrence.Role == nil || *occurrence.Role != "visitor" {
		t.Errorf("expected role 'visitor', got %v", occurrence.Role)
	}
	if occurrence.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", occurrence.Confidence)
	}
}

func TestRelationshipWorkflowHandler_GetEntities_ServiceError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockService := &mockRelationshipWorkflowService{
		getEntitiesWithOccurrencesFunc: func(ctx context.Context, dsID uuid.UUID) ([]*services.EntityWithOccurrences, error) {
			return nil, fmt.Errorf("database error")
		},
	}

	handler := NewRelationshipWorkflowHandler(mockService, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/relationships/entities", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.GetEntities(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestRelationshipWorkflowHandler_GetStatus_WithEntityCounts(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	workflowID := uuid.New()

	mockService := &mockRelationshipWorkflowService{
		getStatusWithCountsFunc: func(ctx context.Context, dsID uuid.UUID) (*models.OntologyWorkflow, *services.CandidateCounts, error) {
			if dsID != datasourceID {
				return nil, nil, fmt.Errorf("unexpected datasource ID")
			}
			workflow := &models.OntologyWorkflow{
				ID:    workflowID,
				Phase: models.WorkflowPhaseRelationships,
				State: models.WorkflowStateCompleted,
			}
			counts := &services.CandidateCounts{
				Confirmed:       5,
				NeedsReview:     0,
				Rejected:        2,
				EntityCount:     3,
				OccurrenceCount: 8,
				IslandCount:     1,
				CanSave:         true,
			}
			return workflow, counts, nil
		},
	}

	handler := NewRelationshipWorkflowHandler(mockService, zap.NewNop())

	url := fmt.Sprintf("/api/projects/%s/datasources/%s/relationships/status", projectID, datasourceID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("pid", projectID.String())
	req.SetPathValue("dsid", datasourceID.String())
	rec := httptest.NewRecorder()

	handler.GetStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response RelationshipWorkflowStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.EntityCount != 3 {
		t.Errorf("expected entity count 3, got %d", response.EntityCount)
	}
	if response.OccurrenceCount != 8 {
		t.Errorf("expected occurrence count 8, got %d", response.OccurrenceCount)
	}
	if response.IslandCount != 1 {
		t.Errorf("expected island count 1, got %d", response.IslandCount)
	}
	if response.ConfirmedCount != 5 {
		t.Errorf("expected confirmed count 5, got %d", response.ConfirmedCount)
	}
	if !response.CanSave {
		t.Error("expected CanSave to be true")
	}
}
