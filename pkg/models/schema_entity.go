package models

import (
	"time"

	"github.com/google/uuid"
)

// SchemaEntity represents a discovered domain entity (user, account, order, etc.)
// that appears in one or more tables/columns across the schema.
type SchemaEntity struct {
	ID            uuid.UUID `json:"id"`
	ProjectID     uuid.UUID `json:"project_id"`
	OntologyID    uuid.UUID `json:"ontology_id"`
	Name          string    `json:"name"`           // e.g., "user", "account", "order"
	Description   string    `json:"description"`    // LLM explanation of the entity
	PrimarySchema string    `json:"primary_schema"` // Schema where entity is primarily defined
	PrimaryTable  string    `json:"primary_table"`  // Table where entity is primarily defined
	PrimaryColumn string    `json:"primary_column"` // Column where entity is primarily defined
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SchemaEntityOccurrence represents a single occurrence of an entity in a specific
// schema.table.column location, optionally with a semantic role (visitor, host, owner).
type SchemaEntityOccurrence struct {
	ID         uuid.UUID `json:"id"`
	EntityID   uuid.UUID `json:"entity_id"`
	SchemaName string    `json:"schema_name"`
	TableName  string    `json:"table_name"`
	ColumnName string    `json:"column_name"`
	Role       *string   `json:"role,omitempty"` // e.g., "visitor", "host", "owner", null for generic
	Confidence float64   `json:"confidence"`     // 0.0 to 1.0, default 1.0
	CreatedAt  time.Time `json:"created_at"`
}
