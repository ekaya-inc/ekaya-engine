package models

import (
	"time"

	"github.com/google/uuid"
)

// ProjectKnowledge represents a project-level fact learned during refinement.
// Stored in engine_project_knowledge table.
// Project knowledge has project-lifecycle scope - persists across ontology re-extractions.
type ProjectKnowledge struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	FactType  string    `json:"fact_type"` // Type of fact (e.g., "business_rule", "convention")
	Value     string    `json:"value"`     // The actual fact content
	Context   *string   `json:"context,omitempty"`

	// Provenance: source tracking (how it was created/modified)
	Source         string  `json:"source"`                     // 'inference', 'mcp', 'manual'
	LastEditSource *string `json:"last_edit_source,omitempty"` // How last modified (nil if never edited)

	// Provenance: actor tracking (who created/modified)
	CreatedBy *uuid.UUID `json:"created_by,omitempty"` // User who triggered creation (from JWT)
	UpdatedBy *uuid.UUID `json:"updated_by,omitempty"` // User who last updated (nil if never updated)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
