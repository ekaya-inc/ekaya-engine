package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestPrecedenceLevel(t *testing.T) {
	checker := NewPrecedenceChecker()
	tests := []struct {
		name     string
		source   string
		expected int
	}{
		{
			name:     "admin has highest precedence",
			source:   models.ProvenanceManual,
			expected: 3,
		},
		{
			name:     "mcp has medium precedence",
			source:   models.ProvenanceMCP,
			expected: 2,
		},
		{
			name:     "inference has lowest precedence",
			source:   models.ProvenanceInferred,
			expected: 1,
		},
		{
			name:     "unknown source has lowest",
			source:   "unknown",
			expected: 0,
		},
		{
			name:     "empty string has lowest",
			source:   "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.GetPrecedenceLevel(tt.source)
			if got != tt.expected {
				t.Errorf("GetPrecedenceLevel(%q) = %d, want %d", tt.source, got, tt.expected)
			}
		})
	}
}

func TestCanModify(t *testing.T) {
	service := &changeReviewService{precedenceChecker: NewPrecedenceChecker()}

	tests := []struct {
		name             string
		elementCreatedBy string
		elementUpdatedBy *string
		modifierSource   string
		expected         bool
	}{
		{
			name:             "admin can modify admin-created element",
			elementCreatedBy: models.ProvenanceManual,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceManual,
			expected:         true,
		},
		{
			name:             "mcp cannot modify admin-created element",
			elementCreatedBy: models.ProvenanceManual,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceMCP,
			expected:         false,
		},
		{
			name:             "inference cannot modify admin-created element",
			elementCreatedBy: models.ProvenanceManual,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceInferred,
			expected:         false,
		},
		{
			name:             "admin can modify mcp-created element",
			elementCreatedBy: models.ProvenanceMCP,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceManual,
			expected:         true,
		},
		{
			name:             "mcp can modify mcp-created element",
			elementCreatedBy: models.ProvenanceMCP,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceMCP,
			expected:         true,
		},
		{
			name:             "inference cannot modify mcp-created element",
			elementCreatedBy: models.ProvenanceMCP,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceInferred,
			expected:         false,
		},
		{
			name:             "admin can modify inference-created element",
			elementCreatedBy: models.ProvenanceInferred,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceManual,
			expected:         true,
		},
		{
			name:             "mcp can modify inference-created element",
			elementCreatedBy: models.ProvenanceInferred,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceMCP,
			expected:         true,
		},
		{
			name:             "inference can modify inference-created element",
			elementCreatedBy: models.ProvenanceInferred,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceInferred,
			expected:         true,
		},
		{
			name:             "mcp cannot modify element last updated by admin",
			elementCreatedBy: models.ProvenanceInferred,
			elementUpdatedBy: strPtrForChangeReview(models.ProvenanceManual),
			modifierSource:   models.ProvenanceMCP,
			expected:         false,
		},
		{
			name:             "admin can modify element last updated by mcp",
			elementCreatedBy: models.ProvenanceInferred,
			elementUpdatedBy: strPtrForChangeReview(models.ProvenanceMCP),
			modifierSource:   models.ProvenanceManual,
			expected:         true,
		},
		{
			name:             "inference cannot modify element last updated by mcp",
			elementCreatedBy: models.ProvenanceInferred,
			elementUpdatedBy: strPtrForChangeReview(models.ProvenanceMCP),
			modifierSource:   models.ProvenanceInferred,
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.CanModify(tt.elementCreatedBy, tt.elementUpdatedBy, tt.modifierSource)
			if got != tt.expected {
				t.Errorf("CanModify(%q, %v, %q) = %v, want %v",
					tt.elementCreatedBy, tt.elementUpdatedBy, tt.modifierSource, got, tt.expected)
			}
		})
	}
}

func TestGetEffectiveSource(t *testing.T) {
	checker := NewPrecedenceChecker()
	tests := []struct {
		name      string
		createdBy string
		updatedBy *string
		expected  string
	}{
		{
			name:      "returns updated_by when present",
			createdBy: "inferred",
			updatedBy: strPtrForChangeReview("mcp"),
			expected:  "mcp",
		},
		{
			name:      "returns created_by when updated_by is nil",
			createdBy: "inferred",
			updatedBy: nil,
			expected:  "inferred",
		},
		{
			name:      "returns created_by when updated_by is empty",
			createdBy: "admin",
			updatedBy: strPtrForChangeReview(""),
			expected:  "admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.GetEffectiveSource(tt.createdBy, tt.updatedBy)
			if got != tt.expected {
				t.Errorf("GetEffectiveSource(%q, %v) = %q, want %q",
					tt.createdBy, tt.updatedBy, got, tt.expected)
			}
		})
	}
}

// strPtrForChangeReview is a helper function to get a pointer to a string.
// Named uniquely to avoid collision with strPtr in other test files.
func strPtrForChangeReview(s string) *string {
	return &s
}

// TestApproveAllChanges_UsesCancelSafeAsync verifies that ApproveAllChanges
// uses ProcessChangesAsync which handles context cancellation safely.
// This test ensures we don't regress to the previous behavior that caused
// crashes when the HTTP request context was canceled.
func TestApproveAllChanges_UsesCancelSafeAsync(t *testing.T) {
	projectID := uuid.New()
	changeID := uuid.New()

	// Create a mock pending change repository
	mockPendingChangeRepo := &mockPendingChangeRepoForApproveAll{
		changes: []*models.PendingChange{
			{
				ID:              changeID,
				ProjectID:       projectID,
				ChangeType:      models.ChangeTypeNewEnumValue,
				Status:          models.ChangeStatusPending,
				TableName:       "test_table",
				ColumnName:      "status",
				SuggestedAction: "", // No action needed
			},
		},
	}

	// Create a mock incremental DAG service to verify ProcessChangesAsync is called
	mockIncrementalDAG := &mockIncrementalDAGForApproveAll{}

	service := &changeReviewService{
		pendingChangeRepo: mockPendingChangeRepo,
		incrementalDAG:    mockIncrementalDAG,
		precedenceChecker: NewPrecedenceChecker(),
		logger:            zap.NewNop(),
	}

	// Use a context that we'll cancel to simulate HTTP request completion
	ctx, cancel := context.WithCancel(context.Background())

	// Call ApproveAllChanges
	result, err := service.ApproveAllChanges(ctx, projectID, "mcp")
	if err != nil {
		t.Fatalf("ApproveAllChanges should not error, got: %v", err)
	}

	// Cancel the context to simulate HTTP response sent
	cancel()

	// Verify the result
	if result.Approved != 1 {
		t.Errorf("Expected 1 approved change, got %d", result.Approved)
	}

	// Verify ProcessChangesAsync was called (not ProcessChanges with ctx)
	if !mockIncrementalDAG.processChangesAsyncCalled {
		t.Error("Expected ProcessChangesAsync to be called, but it wasn't")
	}

	// Verify it was called with the right project ID
	if mockIncrementalDAG.calledProjectID != projectID {
		t.Errorf("Expected ProcessChangesAsync to be called with project %s, got %s",
			projectID, mockIncrementalDAG.calledProjectID)
	}
}

// mockPendingChangeRepoForApproveAll is a mock for testing ApproveAllChanges
type mockPendingChangeRepoForApproveAll struct {
	changes []*models.PendingChange
}

func (m *mockPendingChangeRepoForApproveAll) List(ctx context.Context, projectID uuid.UUID, status string, limit int) ([]*models.PendingChange, error) {
	return m.changes, nil
}

func (m *mockPendingChangeRepoForApproveAll) ListByType(ctx context.Context, projectID uuid.UUID, changeType string, limit int) ([]*models.PendingChange, error) {
	return nil, nil
}

func (m *mockPendingChangeRepoForApproveAll) Create(ctx context.Context, change *models.PendingChange) error {
	return nil
}

func (m *mockPendingChangeRepoForApproveAll) CreateBatch(ctx context.Context, changes []*models.PendingChange) error {
	return nil
}

func (m *mockPendingChangeRepoForApproveAll) GetByID(ctx context.Context, id uuid.UUID) (*models.PendingChange, error) {
	for _, c := range m.changes {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}

func (m *mockPendingChangeRepoForApproveAll) UpdateStatus(ctx context.Context, id uuid.UUID, status string, reviewedBy string) error {
	return nil
}

func (m *mockPendingChangeRepoForApproveAll) CountByStatus(ctx context.Context, projectID uuid.UUID) (map[string]int, error) {
	return nil, nil
}

func (m *mockPendingChangeRepoForApproveAll) DeleteOldApproved(ctx context.Context, projectID uuid.UUID, olderThanDays int) (int, error) {
	return 0, nil
}

func (m *mockPendingChangeRepoForApproveAll) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

// mockIncrementalDAGForApproveAll is a mock for testing ApproveAllChanges
type mockIncrementalDAGForApproveAll struct {
	processChangesAsyncCalled bool
	calledProjectID           uuid.UUID
	calledChangeCount         int
}

func (m *mockIncrementalDAGForApproveAll) ProcessChange(ctx context.Context, change *models.PendingChange) error {
	return nil
}

func (m *mockIncrementalDAGForApproveAll) ProcessChanges(ctx context.Context, changes []*models.PendingChange) error {
	// This should NOT be called directly anymore
	return nil
}

func (m *mockIncrementalDAGForApproveAll) ProcessChangeAsync(ctx context.Context, change *models.PendingChange) {
}

func (m *mockIncrementalDAGForApproveAll) ProcessChangesAsync(projectID uuid.UUID, changes []*models.PendingChange) {
	m.processChangesAsyncCalled = true
	m.calledProjectID = projectID
	m.calledChangeCount = len(changes)
}

func (m *mockIncrementalDAGForApproveAll) SetChangeReviewService(svc ChangeReviewService) {
}
