package models

import (
	"time"

	"github.com/google/uuid"
)

// ColumnMetadata represents semantic annotations for a specific column.
// Stored in engine_ontology_column_metadata table with provenance tracking.
type ColumnMetadata struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	TableName   string    `json:"table_name"`
	ColumnName  string    `json:"column_name"`
	Description *string   `json:"description,omitempty"`
	Entity      *string   `json:"entity,omitempty"`       // Entity this column belongs to (e.g., 'User', 'Account')
	Role        *string   `json:"role,omitempty"`         // Semantic role: 'dimension', 'measure', 'identifier', 'attribute'
	EnumValues  []string  `json:"enum_values,omitempty"`  // Array of enum values with descriptions
	IsSensitive *bool     `json:"is_sensitive,omitempty"` // Sensitive data override: nil=auto-detect, true=always, false=never

	// Provenance: source tracking (how it was created/modified)
	Source         string  `json:"source"`                     // 'inferred', 'mcp', 'manual'
	LastEditSource *string `json:"last_edit_source,omitempty"` // How last modified (nil if never edited)

	// Provenance: actor tracking (who created/modified)
	CreatedBy *uuid.UUID `json:"created_by,omitempty"` // User who triggered creation (from JWT)
	UpdatedBy *uuid.UUID `json:"updated_by,omitempty"` // User who last updated (nil if never updated)

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}
