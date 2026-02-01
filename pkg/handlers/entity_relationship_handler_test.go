package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Mock Services
// ============================================================================

// mockDeterministicRelationshipService is a mock for discovery operations.
type mockDeterministicRelationshipService struct {
	fkResult      *services.FKDiscoveryResult
	pkMatchResult *services.PKMatchDiscoveryResult
	err           error
}

func (m *mockDeterministicRelationshipService) DiscoverFKRelationships(_ context.Context, _, _ uuid.UUID, _ services.RelationshipProgressCallback) (*services.FKDiscoveryResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.fkResult, nil
}

func (m *mockDeterministicRelationshipService) DiscoverPKMatchRelationships(_ context.Context, _, _ uuid.UUID, _ services.RelationshipProgressCallback) (*services.PKMatchDiscoveryResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pkMatchResult, nil
}

func (m *mockDeterministicRelationshipService) GetByProject(_ context.Context, _ uuid.UUID) ([]*models.EntityRelationship, error) {
	// No longer used by List, but kept for interface compliance
	return nil, nil
}

// mockSchemaServiceForRelationships mocks SchemaService for relationship listing.
type mockSchemaServiceForRelationships struct {
	services.SchemaService
	response *models.RelationshipsResponse
	err      error
}

func (m *mockSchemaServiceForRelationships) GetRelationshipsResponse(_ context.Context, _, _ uuid.UUID) (*models.RelationshipsResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

// mockProjectServiceForRelationships mocks ProjectService for getting default datasource.
type mockProjectServiceForRelationships struct {
	services.ProjectService
	datasourceID uuid.UUID
	err          error
}

func (m *mockProjectServiceForRelationships) GetDefaultDatasourceID(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	if m.err != nil {
		return uuid.Nil, m.err
	}
	return m.datasourceID, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

func ptrString(s string) *string { return &s }
func ptrBool(b bool) *bool       { return &b }

// ============================================================================
// Tests for List Endpoint (reads from engine_schema_relationships)
// ============================================================================

// TestEntityRelationshipHandler_List_InferenceMethodMapping tests that inference_method
// from schema relationships is correctly mapped to relationship_type in the API response.
func TestEntityRelationshipHandler_List_InferenceMethodMapping(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	testCases := []struct {
		name            string
		inferenceMethod *string
		expectedRelType string
	}{
		{
			name:            "fk maps to fk",
			inferenceMethod: ptrString("fk"),
			expectedRelType: "fk",
		},
		{
			name:            "foreign_key maps to fk",
			inferenceMethod: ptrString("foreign_key"),
			expectedRelType: "fk",
		},
		{
			name:            "manual maps to manual",
			inferenceMethod: ptrString("manual"),
			expectedRelType: "manual",
		},
		{
			name:            "pk_match maps to inferred",
			inferenceMethod: ptrString("pk_match"),
			expectedRelType: "inferred",
		},
		{
			name:            "column_features maps to inferred",
			inferenceMethod: ptrString("column_features"),
			expectedRelType: "inferred",
		},
		{
			name:            "nil inference_method maps to inferred",
			inferenceMethod: nil,
			expectedRelType: "inferred",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			mockSchemaService := &mockSchemaServiceForRelationships{
				response: &models.RelationshipsResponse{
					Relationships: []*models.RelationshipDetail{
						{
							ID:               uuid.New(),
							SourceTableName:  "users",
							SourceColumnName: "id",
							SourceColumnType: "uuid",
							TargetTableName:  "orders",
							TargetColumnName: "user_id",
							TargetColumnType: "uuid",
							RelationshipType: "user_defined",
							Cardinality:      "N:1",
							Confidence:       1.0,
							InferenceMethod:  tc.inferenceMethod,
							IsValidated:      true,
							IsApproved:       ptrBool(true),
							CreatedAt:        now,
							UpdatedAt:        now,
						},
					},
					TotalCount: 1,
				},
			}

			mockProjectService := &mockProjectServiceForRelationships{
				datasourceID: datasourceID,
			}

			handler := NewEntityRelationshipHandler(
				&mockDeterministicRelationshipService{},
				mockSchemaService,
				mockProjectService,
				zap.NewNop(),
			)

			req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/relationships", nil)
			req.SetPathValue("pid", projectID.String())

			rec := httptest.NewRecorder()
			handler.List(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
			}

			var response ApiResponse
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if !response.Success {
				t.Fatal("expected success=true")
			}

			dataBytes, err := json.Marshal(response.Data)
			if err != nil {
				t.Fatalf("failed to marshal data: %v", err)
			}

			var listResponse SchemaRelationshipListResponse
			if err := json.Unmarshal(dataBytes, &listResponse); err != nil {
				t.Fatalf("failed to unmarshal list response: %v", err)
			}

			if len(listResponse.Relationships) != 1 {
				t.Fatalf("expected 1 relationship, got %d", len(listResponse.Relationships))
			}

			rel := listResponse.Relationships[0]
			if rel.RelationshipType != tc.expectedRelType {
				t.Errorf("expected RelationshipType=%q, got %q", tc.expectedRelType, rel.RelationshipType)
			}
		})
	}
}

// TestEntityRelationshipHandler_List_ColumnTypes tests that column types are included.
func TestEntityRelationshipHandler_List_ColumnTypes(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	now := time.Now()

	mockSchemaService := &mockSchemaServiceForRelationships{
		response: &models.RelationshipsResponse{
			Relationships: []*models.RelationshipDetail{
				{
					ID:               uuid.New(),
					SourceTableName:  "users",
					SourceColumnName: "id",
					SourceColumnType: "bigint",
					TargetTableName:  "orders",
					TargetColumnName: "user_id",
					TargetColumnType: "integer",
					RelationshipType: "user_defined",
					Cardinality:      "N:1",
					Confidence:       1.0,
					InferenceMethod:  ptrString("fk"),
					IsValidated:      true,
					IsApproved:       ptrBool(true),
					CreatedAt:        now,
					UpdatedAt:        now,
				},
			},
			TotalCount: 1,
		},
	}

	mockProjectService := &mockProjectServiceForRelationships{
		datasourceID: datasourceID,
	}

	handler := NewEntityRelationshipHandler(
		&mockDeterministicRelationshipService{},
		mockSchemaService,
		mockProjectService,
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/relationships", nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	dataBytes, err := json.Marshal(response.Data)
	if err != nil {
		t.Fatalf("failed to marshal data: %v", err)
	}

	var listResponse SchemaRelationshipListResponse
	if err := json.Unmarshal(dataBytes, &listResponse); err != nil {
		t.Fatalf("failed to unmarshal list response: %v", err)
	}

	if len(listResponse.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(listResponse.Relationships))
	}

	rel := listResponse.Relationships[0]
	if rel.SourceColumnType != "bigint" {
		t.Errorf("expected SourceColumnType=bigint, got %q", rel.SourceColumnType)
	}
	if rel.TargetColumnType != "integer" {
		t.Errorf("expected TargetColumnType=integer, got %q", rel.TargetColumnType)
	}
}

// TestEntityRelationshipHandler_List_EmptyAndOrphanTables tests that empty/orphan tables are returned.
func TestEntityRelationshipHandler_List_EmptyAndOrphanTables(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	now := time.Now()

	mockSchemaService := &mockSchemaServiceForRelationships{
		response: &models.RelationshipsResponse{
			Relationships: []*models.RelationshipDetail{
				{
					ID:               uuid.New(),
					SourceTableName:  "users",
					SourceColumnName: "id",
					SourceColumnType: "uuid",
					TargetTableName:  "orders",
					TargetColumnName: "user_id",
					TargetColumnType: "uuid",
					RelationshipType: "user_defined",
					Cardinality:      "N:1",
					Confidence:       1.0,
					InferenceMethod:  ptrString("fk"),
					IsValidated:      true,
					CreatedAt:        now,
					UpdatedAt:        now,
				},
			},
			TotalCount:   1,
			EmptyTables:  []string{"audit_logs", "temp_data"},
			OrphanTables: []string{"settings", "config"},
		},
	}

	mockProjectService := &mockProjectServiceForRelationships{
		datasourceID: datasourceID,
	}

	handler := NewEntityRelationshipHandler(
		&mockDeterministicRelationshipService{},
		mockSchemaService,
		mockProjectService,
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/relationships", nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	dataBytes, err := json.Marshal(response.Data)
	if err != nil {
		t.Fatalf("failed to marshal data: %v", err)
	}

	var listResponse SchemaRelationshipListResponse
	if err := json.Unmarshal(dataBytes, &listResponse); err != nil {
		t.Fatalf("failed to unmarshal list response: %v", err)
	}

	if len(listResponse.EmptyTables) != 2 {
		t.Errorf("expected 2 empty tables, got %d", len(listResponse.EmptyTables))
	}
	if len(listResponse.OrphanTables) != 2 {
		t.Errorf("expected 2 orphan tables, got %d", len(listResponse.OrphanTables))
	}
}

// TestEntityRelationshipHandler_List_NoDatasource tests behavior when no datasource is configured.
func TestEntityRelationshipHandler_List_NoDatasource(t *testing.T) {
	projectID := uuid.New()

	mockProjectService := &mockProjectServiceForRelationships{
		err: errors.New("no default datasource configured"),
	}

	handler := NewEntityRelationshipHandler(
		&mockDeterministicRelationshipService{},
		&mockSchemaServiceForRelationships{},
		mockProjectService,
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/relationships", nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.List(rec, req)

	// Should return empty response, not error
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !response.Success {
		t.Fatal("expected success=true")
	}

	dataBytes, err := json.Marshal(response.Data)
	if err != nil {
		t.Fatalf("failed to marshal data: %v", err)
	}

	var listResponse SchemaRelationshipListResponse
	if err := json.Unmarshal(dataBytes, &listResponse); err != nil {
		t.Fatalf("failed to unmarshal list response: %v", err)
	}

	if len(listResponse.Relationships) != 0 {
		t.Errorf("expected 0 relationships when no datasource, got %d", len(listResponse.Relationships))
	}
	if listResponse.TotalCount != 0 {
		t.Errorf("expected TotalCount=0, got %d", listResponse.TotalCount)
	}
}

// TestEntityRelationshipHandler_List_SchemaServiceError tests error handling.
func TestEntityRelationshipHandler_List_SchemaServiceError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockSchemaService := &mockSchemaServiceForRelationships{
		err: errors.New("database connection failed"),
	}

	mockProjectService := &mockProjectServiceForRelationships{
		datasourceID: datasourceID,
	}

	handler := NewEntityRelationshipHandler(
		&mockDeterministicRelationshipService{},
		mockSchemaService,
		mockProjectService,
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/relationships", nil)
	req.SetPathValue("pid", projectID.String())

	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

// ============================================================================
// Tests for mapInferenceMethodToType helper
// ============================================================================

func TestMapInferenceMethodToType(t *testing.T) {
	testCases := []struct {
		input    *string
		expected string
	}{
		{nil, "inferred"},
		{ptrString("fk"), "fk"},
		{ptrString("foreign_key"), "fk"},
		{ptrString("manual"), "manual"},
		{ptrString("pk_match"), "inferred"},
		{ptrString("column_features"), "inferred"},
		{ptrString("unknown"), "inferred"},
	}

	for _, tc := range testCases {
		name := "nil"
		if tc.input != nil {
			name = *tc.input
		}
		t.Run(name, func(t *testing.T) {
			result := mapInferenceMethodToType(tc.input)
			if result != tc.expected {
				t.Errorf("mapInferenceMethodToType(%v) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}
