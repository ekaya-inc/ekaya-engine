package models

import (
	"time"

	"github.com/google/uuid"
)

// OntologyEntity represents a discovered domain entity (user, account, order, etc.)
// that appears in one or more tables/columns across the schema.
// Stored in engine_ontology_entities table.
type OntologyEntity struct {
	ID             uuid.UUID `json:"id"`
	ProjectID      uuid.UUID `json:"project_id"`
	OntologyID     uuid.UUID `json:"ontology_id"`
	Name           string    `json:"name"`           // e.g., "user", "account", "order"
	Description    string    `json:"description"`    // LLM explanation of the entity
	PrimarySchema  string    `json:"primary_schema"` // Schema where entity is primarily defined
	PrimaryTable   string    `json:"primary_table"`  // Table where entity is primarily defined
	PrimaryColumn  string    `json:"primary_column"` // Column where entity is primarily defined
	IsDeleted      bool      `json:"is_deleted"`     // Soft delete flag
	DeletionReason *string   `json:"deletion_reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// OntologyEntityOccurrence represents a single occurrence of an entity in a specific
// schema.table.column location, optionally with a semantic role (visitor, host, owner).
// Stored in engine_ontology_entity_occurrences table.
type OntologyEntityOccurrence struct {
	ID         uuid.UUID `json:"id"`
	EntityID   uuid.UUID `json:"entity_id"`
	SchemaName string    `json:"schema_name"`
	TableName  string    `json:"table_name"`
	ColumnName string    `json:"column_name"`
	Role       *string   `json:"role,omitempty"` // e.g., "visitor", "host", "owner", null for generic
	Confidence float64   `json:"confidence"`     // 0.0 to 1.0, default 1.0
	CreatedAt  time.Time `json:"created_at"`
}

// OntologyEntityAlias represents an alternative name for an entity.
// Used for query matching (e.g., "customer" as alias for "user").
type OntologyEntityAlias struct {
	ID        uuid.UUID `json:"id"`
	EntityID  uuid.UUID `json:"entity_id"`
	Alias     string    `json:"alias"`
	Source    *string   `json:"source,omitempty"` // 'discovery', 'user', 'query'
	CreatedAt time.Time `json:"created_at"`
}
