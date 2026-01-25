package models

import (
	"time"

	"github.com/google/uuid"
)

// Provenance source constants for ontology elements.
// Precedence: Manual (highest) > MCP (Claude) > Inference (Engine, lowest)
const (
	ProvenanceManual    = "manual"    // Direct manual edit via UI - highest precedence
	ProvenanceMCP       = "mcp"       // Claude via MCP tools - wins over inference
	ProvenanceInference = "inference" // Engine auto-detected or LLM-generated - lowest precedence
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
	Domain         string    `json:"domain"`         // Business domain (e.g., "billing", "hospitality")
	PrimarySchema  string    `json:"primary_schema"` // Schema where entity is primarily defined
	PrimaryTable   string    `json:"primary_table"`  // Table where entity is primarily defined
	PrimaryColumn  string    `json:"primary_column"` // Column where entity is primarily defined
	IsDeleted      bool      `json:"is_deleted"`     // Soft delete flag
	DeletionReason *string   `json:"deletion_reason,omitempty"`
	Confidence     float64   `json:"confidence"` // 0.0-1.0: higher for FK-derived, lower for LLM-inferred
	IsStale        bool      `json:"is_stale"`   // True when schema changed and needs re-evaluation

	// Provenance: source tracking (how it was created/modified)
	Source         string  `json:"source"`                     // 'inference', 'mcp', 'manual'
	LastEditSource *string `json:"last_edit_source,omitempty"` // How last modified (nil if never edited)

	// Provenance: actor tracking (who created/modified)
	CreatedBy *uuid.UUID `json:"created_by,omitempty"` // User who triggered creation (from JWT)
	UpdatedBy *uuid.UUID `json:"updated_by,omitempty"` // User who last updated (nil if never updated)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OntologyEntityOccurrence represents a computed occurrence of an entity in a specific
// schema.table.column location, optionally with a semantic association.
// No longer stored in database - derived from relationships at runtime.
type OntologyEntityOccurrence struct {
	ID          uuid.UUID `json:"id"`
	EntityID    uuid.UUID `json:"entity_id"`
	SchemaName  string    `json:"schema_name"`
	TableName   string    `json:"table_name"`
	ColumnName  string    `json:"column_name"`
	Association *string   `json:"association,omitempty"` // e.g., "placed_by", "contains", "as host"
	Confidence  float64   `json:"confidence"`            // 0.0 to 1.0, default 1.0
	CreatedAt   time.Time `json:"created_at"`
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

// OntologyEntityKeyColumn represents an important business column for an entity.
// These are columns that business users typically query on (not id/timestamps).
// Stored in engine_ontology_entity_key_columns table.
type OntologyEntityKeyColumn struct {
	ID         uuid.UUID `json:"id"`
	EntityID   uuid.UUID `json:"entity_id"`
	ColumnName string    `json:"column_name"`
	Synonyms   []string  `json:"synonyms,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
