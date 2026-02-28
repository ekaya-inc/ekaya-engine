package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestPrecedenceChecker_GetPrecedenceLevel(t *testing.T) {
	checker := NewPrecedenceChecker()

	tests := []struct {
		name   string
		source string
		want   int
	}{
		{"manual is 3", models.ProvenanceManual, 3},
		{"mcp is 2", models.ProvenanceMCP, 2},
		{"inferred is 1", models.ProvenanceInferred, 1},
		{"unknown is 0", "unknown", 0},
		{"empty string is 0", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.GetPrecedenceLevel(tt.source)
			if got != tt.want {
				t.Errorf("GetPrecedenceLevel(%q) = %d, want %d", tt.source, got, tt.want)
			}
		})
	}
}

func TestPrecedenceChecker_GetEffectiveSource(t *testing.T) {
	checker := NewPrecedenceChecker()

	mcp := models.ProvenanceMCP
	empty := ""

	tests := []struct {
		name      string
		createdBy string
		updatedBy *string
		want      string
	}{
		{"updatedBy set uses updatedBy", models.ProvenanceInferred, &mcp, models.ProvenanceMCP},
		{"updatedBy nil uses createdBy", models.ProvenanceManual, nil, models.ProvenanceManual},
		{"updatedBy empty uses createdBy", models.ProvenanceInferred, &empty, models.ProvenanceInferred},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.GetEffectiveSource(tt.createdBy, tt.updatedBy)
			if got != tt.want {
				t.Errorf("GetEffectiveSource(%q, %v) = %q, want %q", tt.createdBy, tt.updatedBy, got, tt.want)
			}
		})
	}
}

func TestPrecedenceChecker_CanModify(t *testing.T) {
	checker := NewPrecedenceChecker()

	tests := []struct {
		name           string
		createdBy      string
		updatedBy      *string
		modifierSource string
		want           bool
	}{
		// Manual can modify everything
		{"manual modifies manual", models.ProvenanceManual, nil, models.ProvenanceManual, true},
		{"manual modifies mcp", models.ProvenanceMCP, nil, models.ProvenanceManual, true},
		{"manual modifies inferred", models.ProvenanceInferred, nil, models.ProvenanceManual, true},

		// MCP can modify MCP and Inference, not Manual
		{"mcp modifies mcp", models.ProvenanceMCP, nil, models.ProvenanceMCP, true},
		{"mcp modifies inferred", models.ProvenanceInferred, nil, models.ProvenanceMCP, true},
		{"mcp cannot modify manual", models.ProvenanceManual, nil, models.ProvenanceMCP, false},

		// Inference can only modify Inference
		{"inferred modifies inferred", models.ProvenanceInferred, nil, models.ProvenanceInferred, true},
		{"inferred cannot modify mcp", models.ProvenanceMCP, nil, models.ProvenanceInferred, false},
		{"inferred cannot modify manual", models.ProvenanceManual, nil, models.ProvenanceInferred, false},

		// Unknown source has level 0
		{"unknown cannot modify inferred", models.ProvenanceInferred, nil, "unknown", false},
		{"unknown can modify unknown", "unknown", nil, "unknown", true},

		// updatedBy overrides createdBy for element level
		{"mcp cannot modify inferred-then-manual", models.ProvenanceInferred, strPtrForPrecedence(models.ProvenanceManual), models.ProvenanceMCP, false},
		{"manual can modify inferred-then-mcp", models.ProvenanceInferred, strPtrForPrecedence(models.ProvenanceMCP), models.ProvenanceManual, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.CanModify(tt.createdBy, tt.updatedBy, tt.modifierSource)
			if got != tt.want {
				t.Errorf("CanModify(%q, %v, %q) = %v, want %v", tt.createdBy, tt.updatedBy, tt.modifierSource, got, tt.want)
			}
		})
	}
}

func TestGlossaryPrecedenceChecker_GetPrecedenceLevel(t *testing.T) {
	checker := NewGlossaryPrecedenceChecker()

	tests := []struct {
		name   string
		source string
		want   int
	}{
		{"manual is 3", models.GlossarySourceManual, 3},
		{"mcp is 2", models.GlossarySourceMCP, 2},
		{"inferred is 1", models.GlossarySourceInferred, 1},
		{"unknown is 0", "unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.GetPrecedenceLevel(tt.source)
			if got != tt.want {
				t.Errorf("GetPrecedenceLevel(%q) = %d, want %d", tt.source, got, tt.want)
			}
		})
	}
}

func TestGlossaryPrecedenceChecker_CanModify(t *testing.T) {
	checker := NewGlossaryPrecedenceChecker()

	tests := []struct {
		name           string
		termSource     string
		modifierSource string
		want           bool
	}{
		{"manual modifies manual", models.GlossarySourceManual, models.GlossarySourceManual, true},
		{"manual modifies inferred", models.GlossarySourceInferred, models.GlossarySourceManual, true},
		{"mcp modifies inferred", models.GlossarySourceInferred, models.GlossarySourceMCP, true},
		{"mcp cannot modify manual", models.GlossarySourceManual, models.GlossarySourceMCP, false},
		{"inferred cannot modify manual", models.GlossarySourceManual, models.GlossarySourceInferred, false},
		{"inferred cannot modify mcp", models.GlossarySourceMCP, models.GlossarySourceInferred, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checker.CanModify(tt.termSource, tt.modifierSource)
			if got != tt.want {
				t.Errorf("CanModify(%q, %q) = %v, want %v", tt.termSource, tt.modifierSource, got, tt.want)
			}
		})
	}
}

func strPtrForPrecedence(s string) *string {
	return &s
}
