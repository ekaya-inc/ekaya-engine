package services

import (
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// PrecedenceChecker validates if a modifier can update an ontology element based on provenance rules.
// Precedence hierarchy: Manual (3) > MCP (2) > Inference (1)
// A modifier can only update elements with equal or lower precedence.
type PrecedenceChecker interface {
	// CanModify checks if the modifier can update an element based on provenance.
	// Returns true if modification is allowed, false otherwise.
	CanModify(elementCreatedBy string, elementUpdatedBy *string, modifierSource string) bool

	// GetPrecedenceLevel returns the numeric precedence level for a source.
	GetPrecedenceLevel(source string) int

	// GetEffectiveSource returns the effective source (UpdatedBy if set, otherwise CreatedBy).
	GetEffectiveSource(createdBy string, updatedBy *string) string
}

// precedenceChecker implements PrecedenceChecker.
type precedenceChecker struct{}

// NewPrecedenceChecker creates a new PrecedenceChecker service.
func NewPrecedenceChecker() PrecedenceChecker {
	return &precedenceChecker{}
}

// CanModify checks if the modifier can update an element based on provenance hierarchy.
// Manual (3) > MCP (2) > Inference (1)
func (s *precedenceChecker) CanModify(elementCreatedBy string, elementUpdatedBy *string, modifierSource string) bool {
	modifierLevel := s.GetPrecedenceLevel(modifierSource)
	existingSource := s.GetEffectiveSource(elementCreatedBy, elementUpdatedBy)
	existingLevel := s.GetPrecedenceLevel(existingSource)

	// Modifier must have equal or higher precedence to update
	return modifierLevel >= existingLevel
}

// GetPrecedenceLevel returns the numeric precedence level for a source.
// Manual: 3, MCP: 2, Inference: 1, Unknown: 0
func (s *precedenceChecker) GetPrecedenceLevel(source string) int {
	switch source {
	case models.ProvenanceManual:
		return 3
	case models.ProvenanceMCP:
		return 2
	case models.ProvenanceInference:
		return 1
	default:
		return 0
	}
}

// GetEffectiveSource returns the effective source for an element.
// If UpdatedBy is set, use it. Otherwise, use CreatedBy.
func (s *precedenceChecker) GetEffectiveSource(createdBy string, updatedBy *string) string {
	if updatedBy != nil && *updatedBy != "" {
		return *updatedBy
	}
	return createdBy
}

// GlossaryPrecedenceChecker validates if a modifier can update a glossary term based on source rules.
// Maps glossary source values to precedence hierarchy: Manual (3) > MCP (2) > Inferred (1)
type GlossaryPrecedenceChecker interface {
	// CanModify checks if the modifier can update a glossary term based on source.
	// Returns true if modification is allowed, false otherwise.
	CanModify(termSource string, modifierSource string) bool

	// GetPrecedenceLevel returns the numeric precedence level for a glossary source.
	GetPrecedenceLevel(source string) int
}

// glossaryPrecedenceChecker implements GlossaryPrecedenceChecker.
type glossaryPrecedenceChecker struct{}

// NewGlossaryPrecedenceChecker creates a new GlossaryPrecedenceChecker service.
func NewGlossaryPrecedenceChecker() GlossaryPrecedenceChecker {
	return &glossaryPrecedenceChecker{}
}

// CanModify checks if the modifier can update a glossary term based on source hierarchy.
// Manual (3) > MCP (2) > Inferred (1)
func (s *glossaryPrecedenceChecker) CanModify(termSource string, modifierSource string) bool {
	modifierLevel := s.GetPrecedenceLevel(modifierSource)
	existingLevel := s.GetPrecedenceLevel(termSource)

	// Modifier must have equal or higher precedence to update
	return modifierLevel >= existingLevel
}

// GetPrecedenceLevel returns the numeric precedence level for a glossary source.
// Manual: 3, MCP: 2, Inferred: 1, Unknown: 0
func (s *glossaryPrecedenceChecker) GetPrecedenceLevel(source string) int {
	switch source {
	case models.GlossarySourceManual:
		return 3
	case models.GlossarySourceMCP:
		return 2
	case models.GlossarySourceInferred:
		return 1
	default:
		return 0
	}
}
