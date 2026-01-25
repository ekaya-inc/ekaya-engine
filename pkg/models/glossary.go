package models

import (
	"time"

	"github.com/google/uuid"
)

// Legacy source values for glossary terms (deprecated - use ProvenanceXxx constants)
const (
	GlossarySourceInferred = "inferred" // Deprecated: use ProvenanceInference
	GlossarySourceManual   = "manual"   // Deprecated: use ProvenanceManual
	GlossarySourceMCP      = "mcp"      // Deprecated: use ProvenanceMCP
)

// Enrichment status values for glossary terms
const (
	GlossaryEnrichmentPending = "pending" // Term discovered, awaiting SQL enrichment
	GlossaryEnrichmentSuccess = "success" // SQL enrichment completed successfully
	GlossaryEnrichmentFailed  = "failed"  // SQL enrichment failed (see EnrichmentError)
)

// BusinessGlossaryTerm represents a business term with its SQL definition.
// Stored in engine_business_glossary table.
type BusinessGlossaryTerm struct {
	ID               uuid.UUID      `json:"id"`
	ProjectID        uuid.UUID      `json:"project_id"`
	OntologyID       *uuid.UUID     `json:"ontology_id,omitempty"` // Links to ontology for CASCADE delete
	Term             string         `json:"term"`
	Definition       string         `json:"definition"`
	DefiningSQL      string         `json:"defining_sql"`
	BaseTable        string         `json:"base_table,omitempty"`
	OutputColumns    []OutputColumn `json:"output_columns,omitempty"`
	Aliases          []string       `json:"aliases,omitempty"`
	EnrichmentStatus string         `json:"enrichment_status,omitempty"` // "pending", "success", "failed"
	EnrichmentError  string         `json:"enrichment_error,omitempty"`  // Error message if enrichment failed
	NeedsReview      bool           `json:"needs_review,omitempty"`      // Flagged for domain expert review
	ReviewReason     string         `json:"review_reason,omitempty"`     // Why the term needs review (e.g., "Formula may not match term semantics")

	// Provenance: source tracking (how it was created/modified)
	Source         string  `json:"source"`                     // 'inference', 'mcp', 'manual'
	LastEditSource *string `json:"last_edit_source,omitempty"` // How last modified (nil if never edited)

	// Provenance: actor tracking (who created/modified)
	CreatedBy *uuid.UUID `json:"created_by,omitempty"` // User who triggered creation (from JWT)
	UpdatedBy *uuid.UUID `json:"updated_by,omitempty"` // User who last updated (nil if never updated)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
