package models

import (
	"time"

	"github.com/google/uuid"
)

// TableMetadata represents semantic annotations for a specific table.
// Stored in engine_table_metadata table with provenance tracking.
type TableMetadata struct {
	ID                   uuid.UUID `json:"id"`
	ProjectID            uuid.UUID `json:"project_id"`
	DatasourceID         uuid.UUID `json:"datasource_id"`
	TableName            string    `json:"table_name"`
	Description          *string   `json:"description,omitempty"`
	UsageNotes           *string   `json:"usage_notes,omitempty"`
	IsEphemeral          bool      `json:"is_ephemeral"`
	PreferredAlternative *string   `json:"preferred_alternative,omitempty"`

	// Provenance: source tracking (how it was created/modified)
	Source         string  `json:"source"`                     // 'inferred', 'mcp', 'manual'
	LastEditSource *string `json:"last_edit_source,omitempty"` // How last modified (nil if never edited)

	// Provenance: actor tracking (who created/modified)
	CreatedBy *uuid.UUID `json:"created_by,omitempty"` // User who triggered creation (from JWT)
	UpdatedBy *uuid.UUID `json:"updated_by,omitempty"` // User who last updated (nil if never updated)

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}
