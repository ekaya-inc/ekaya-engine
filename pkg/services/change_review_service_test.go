package services

import (
	"testing"

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
