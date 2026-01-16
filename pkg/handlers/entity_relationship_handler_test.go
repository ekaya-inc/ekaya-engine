package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockDeterministicRelationshipService is a mock for testing entity relationship handler.
type mockDeterministicRelationshipService struct {
	relationships []*models.EntityRelationship
	fkResult      *services.FKDiscoveryResult
	pkMatchResult *services.PKMatchDiscoveryResult
	err           error
}

func (m *mockDeterministicRelationshipService) DiscoverFKRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, _ services.RelationshipProgressCallback) (*services.FKDiscoveryResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.fkResult, nil
}

func (m *mockDeterministicRelationshipService) DiscoverPKMatchRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, _ services.RelationshipProgressCallback) (*services.PKMatchDiscoveryResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pkMatchResult, nil
}

func (m *mockDeterministicRelationshipService) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.relationships, nil
}

// TestEntityRelationshipHandler_List_DetectionMethodMapping tests that detection_method
// is correctly mapped to relationship_type in the API response.
func TestEntityRelationshipHandler_List_DetectionMethodMapping(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()

	testCases := []struct {
		name             string
		detectionMethod  string
		expectedRelType  string
		expectedStatus   string
		isValidated      bool
		isApprovedNotNil bool
	}{
		{
			name:             "foreign_key maps to fk",
			detectionMethod:  models.DetectionMethodForeignKey,
			expectedRelType:  "fk",
			expectedStatus:   "confirmed",
			isValidated:      true,
			isApprovedNotNil: true,
		},
		{
			name:             "manual maps to manual",
			detectionMethod:  models.DetectionMethodManual,
			expectedRelType:  "manual",
			expectedStatus:   "confirmed",
			isValidated:      true,
			isApprovedNotNil: true,
		},
		{
			name:             "pk_match maps to inferred",
			detectionMethod:  models.DetectionMethodPKMatch,
			expectedRelType:  "inferred",
			expectedStatus:   "pending",
			isValidated:      false,
			isApprovedNotNil: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := "pending"
			if tc.expectedStatus == "confirmed" {
				status = "confirmed"
			}

			mockService := &mockDeterministicRelationshipService{
				relationships: []*models.EntityRelationship{
					{
						ID:                uuid.New(),
						OntologyID:        ontologyID,
						SourceEntityID:    uuid.New(),
						TargetEntityID:    uuid.New(),
						SourceColumnTable: "users",
						SourceColumnName:  "id",
						TargetColumnTable: "orders",
						TargetColumnName:  "user_id",
						DetectionMethod:   tc.detectionMethod,
						Status:            status,
						Confidence:        1.0,
					},
				},
			}

			handler := NewEntityRelationshipHandler(mockService, zap.NewNop())

			// Create request with project ID in path
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

			var listResponse EntityRelationshipListResponse
			if err := json.Unmarshal(dataBytes, &listResponse); err != nil {
				t.Fatalf("failed to unmarshal list response: %v", err)
			}

			if len(listResponse.Relationships) != 1 {
				t.Fatalf("expected 1 relationship, got %d", len(listResponse.Relationships))
			}

			rel := listResponse.Relationships[0]

			// Verify relationship_type mapping
			if rel.RelationshipType != tc.expectedRelType {
				t.Errorf("expected RelationshipType=%q, got %q", tc.expectedRelType, rel.RelationshipType)
			}

			// Verify status
			if rel.Status != tc.expectedStatus {
				t.Errorf("expected Status=%q, got %q", tc.expectedStatus, rel.Status)
			}

			// Verify is_validated
			if rel.IsValidated != tc.isValidated {
				t.Errorf("expected IsValidated=%v, got %v", tc.isValidated, rel.IsValidated)
			}

			// Verify is_approved
			if tc.isApprovedNotNil {
				if rel.IsApproved == nil {
					t.Error("expected IsApproved to be non-nil")
				} else if !*rel.IsApproved {
					t.Error("expected IsApproved=true for confirmed status")
				}
			} else {
				if rel.IsApproved != nil {
					t.Errorf("expected IsApproved=nil, got %v", *rel.IsApproved)
				}
			}
		})
	}
}

// TestEntityRelationshipHandler_List_DescriptionMapping tests that relationship descriptions
// are correctly included in the API response.
func TestEntityRelationshipHandler_List_DescriptionMapping(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()

	description1 := "Links a user to their account"
	description2 := "Links a user to the channels they own"

	testCases := []struct {
		name               string
		description        *string
		expectedInResponse string
	}{
		{
			name:               "description present",
			description:        &description1,
			expectedInResponse: description1,
		},
		{
			name:               "description null",
			description:        nil,
			expectedInResponse: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockService := &mockDeterministicRelationshipService{
				relationships: []*models.EntityRelationship{
					{
						ID:                uuid.New(),
						OntologyID:        ontologyID,
						SourceEntityID:    uuid.New(),
						TargetEntityID:    uuid.New(),
						SourceColumnTable: "users",
						SourceColumnName:  "id",
						TargetColumnTable: "orders",
						TargetColumnName:  "user_id",
						DetectionMethod:   models.DetectionMethodForeignKey,
						Status:            "confirmed",
						Confidence:        1.0,
						Description:       tc.description,
					},
				},
			}

			handler := NewEntityRelationshipHandler(mockService, zap.NewNop())

			// Create request with project ID in path
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

			var listResponse EntityRelationshipListResponse
			if err := json.Unmarshal(dataBytes, &listResponse); err != nil {
				t.Fatalf("failed to unmarshal list response: %v", err)
			}

			if len(listResponse.Relationships) != 1 {
				t.Fatalf("expected 1 relationship, got %d", len(listResponse.Relationships))
			}

			rel := listResponse.Relationships[0]

			// Verify description mapping
			if rel.Description != tc.expectedInResponse {
				t.Errorf("expected Description=%q, got %q", tc.expectedInResponse, rel.Description)
			}
		})
	}

	// Test with multiple relationships having different descriptions
	t.Run("multiple relationships with descriptions", func(t *testing.T) {
		mockService := &mockDeterministicRelationshipService{
			relationships: []*models.EntityRelationship{
				{
					ID:                uuid.New(),
					OntologyID:        ontologyID,
					SourceEntityID:    uuid.New(),
					TargetEntityID:    uuid.New(),
					SourceColumnTable: "users",
					SourceColumnName:  "account_id",
					TargetColumnTable: "accounts",
					TargetColumnName:  "id",
					DetectionMethod:   models.DetectionMethodForeignKey,
					Status:            "confirmed",
					Confidence:        1.0,
					Description:       &description1,
				},
				{
					ID:                uuid.New(),
					OntologyID:        ontologyID,
					SourceEntityID:    uuid.New(),
					TargetEntityID:    uuid.New(),
					SourceColumnTable: "users",
					SourceColumnName:  "id",
					TargetColumnTable: "channels",
					TargetColumnName:  "owner_id",
					DetectionMethod:   models.DetectionMethodPKMatch,
					Status:            "pending",
					Confidence:        0.85,
					Description:       &description2,
				},
			},
		}

		handler := NewEntityRelationshipHandler(mockService, zap.NewNop())

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

		var listResponse EntityRelationshipListResponse
		if err := json.Unmarshal(dataBytes, &listResponse); err != nil {
			t.Fatalf("failed to unmarshal list response: %v", err)
		}

		if len(listResponse.Relationships) != 2 {
			t.Fatalf("expected 2 relationships, got %d", len(listResponse.Relationships))
		}

		// Verify both descriptions are present
		if listResponse.Relationships[0].Description != description1 {
			t.Errorf("expected first relationship Description=%q, got %q", description1, listResponse.Relationships[0].Description)
		}
		if listResponse.Relationships[1].Description != description2 {
			t.Errorf("expected second relationship Description=%q, got %q", description2, listResponse.Relationships[1].Description)
		}
	})
}
