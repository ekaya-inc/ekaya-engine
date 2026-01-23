package models

import (
	"time"

	"github.com/google/uuid"
)

// Source values for glossary terms
const (
	GlossarySourceInferred = "inferred" // LLM discovered during ontology extraction
	GlossarySourceManual   = "manual"   // Human added or edited via UI
	GlossarySourceMCP      = "mcp"      // MCP client added dynamically
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
	Source           string         `json:"source"`
	EnrichmentStatus string         `json:"enrichment_status,omitempty"` // "pending", "success", "failed"
	EnrichmentError  string         `json:"enrichment_error,omitempty"`  // Error message if enrichment failed
	CreatedBy        *uuid.UUID     `json:"created_by,omitempty"`
	UpdatedBy        *uuid.UUID     `json:"updated_by,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}
