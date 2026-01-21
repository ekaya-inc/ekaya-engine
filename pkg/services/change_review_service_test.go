package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestPrecedenceLevel(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected int
	}{
		{
			name:     "admin has highest precedence",
			source:   models.ProvenanceAdmin,
			expected: 3,
		},
		{
			name:     "mcp has medium precedence",
			source:   models.ProvenanceMCP,
			expected: 2,
		},
		{
			name:     "inference has lowest precedence",
			source:   models.ProvenanceInference,
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
			got := precedenceLevel(tt.source)
			if got != tt.expected {
				t.Errorf("precedenceLevel(%q) = %d, want %d", tt.source, got, tt.expected)
			}
		})
	}
}

func TestCanModify(t *testing.T) {
	service := &changeReviewService{}

	tests := []struct {
		name             string
		elementCreatedBy string
		elementUpdatedBy *string
		modifierSource   string
		expected         bool
	}{
		{
			name:             "admin can modify admin-created element",
			elementCreatedBy: models.ProvenanceAdmin,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceAdmin,
			expected:         true,
		},
		{
			name:             "mcp cannot modify admin-created element",
			elementCreatedBy: models.ProvenanceAdmin,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceMCP,
			expected:         false,
		},
		{
			name:             "inference cannot modify admin-created element",
			elementCreatedBy: models.ProvenanceAdmin,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceInference,
			expected:         false,
		},
		{
			name:             "admin can modify mcp-created element",
			elementCreatedBy: models.ProvenanceMCP,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceAdmin,
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
			modifierSource:   models.ProvenanceInference,
			expected:         false,
		},
		{
			name:             "admin can modify inference-created element",
			elementCreatedBy: models.ProvenanceInference,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceAdmin,
			expected:         true,
		},
		{
			name:             "mcp can modify inference-created element",
			elementCreatedBy: models.ProvenanceInference,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceMCP,
			expected:         true,
		},
		{
			name:             "inference can modify inference-created element",
			elementCreatedBy: models.ProvenanceInference,
			elementUpdatedBy: nil,
			modifierSource:   models.ProvenanceInference,
			expected:         true,
		},
		{
			name:             "mcp cannot modify element last updated by admin",
			elementCreatedBy: models.ProvenanceInference,
			elementUpdatedBy: strPtrForChangeReview(models.ProvenanceAdmin),
			modifierSource:   models.ProvenanceMCP,
			expected:         false,
		},
		{
			name:             "admin can modify element last updated by mcp",
			elementCreatedBy: models.ProvenanceInference,
			elementUpdatedBy: strPtrForChangeReview(models.ProvenanceMCP),
			modifierSource:   models.ProvenanceAdmin,
			expected:         true,
		},
		{
			name:             "inference cannot modify element last updated by mcp",
			elementCreatedBy: models.ProvenanceInference,
			elementUpdatedBy: strPtrForChangeReview(models.ProvenanceMCP),
			modifierSource:   models.ProvenanceInference,
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
	tests := []struct {
		name      string
		createdBy string
		updatedBy *string
		expected  string
	}{
		{
			name:      "returns updated_by when present",
			createdBy: "inference",
			updatedBy: strPtrForChangeReview("mcp"),
			expected:  "mcp",
		},
		{
			name:      "returns created_by when updated_by is nil",
			createdBy: "inference",
			updatedBy: nil,
			expected:  "inference",
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
			got := getEffectiveSource(tt.createdBy, tt.updatedBy)
			if got != tt.expected {
				t.Errorf("getEffectiveSource(%q, %v) = %q, want %q",
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
