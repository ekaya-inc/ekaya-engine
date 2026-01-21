package models

import (
	"time"

	"github.com/google/uuid"
)

// ColumnMetadata represents semantic annotations for a specific column.
// Stored in engine_ontology_column_metadata table with provenance tracking.
type ColumnMetadata struct {
	ID          uuid.UUID  `json:"id"`
	ProjectID   uuid.UUID  `json:"project_id"`
	TableName   string     `json:"table_name"`
	ColumnName  string     `json:"column_name"`
	Description *string    `json:"description,omitempty"`
	Entity      *string    `json:"entity,omitempty"`      // Entity this column belongs to (e.g., 'User', 'Account')
	Role        *string    `json:"role,omitempty"`        // Semantic role: 'dimension', 'measure', 'identifier', 'attribute'
	EnumValues  []string   `json:"enum_values,omitempty"` // Array of enum values with descriptions
	CreatedBy   string     `json:"created_by"`            // Provenance: 'admin', 'mcp', 'inference'
	UpdatedBy   *string    `json:"updated_by,omitempty"`  // Who last updated (nil if never updated)
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}
