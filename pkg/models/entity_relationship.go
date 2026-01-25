package models

import (
	"time"

	"github.com/google/uuid"
)

// Detection methods for entity relationships.
const (
	DetectionMethodForeignKey = "foreign_key" // Discovered from database FK constraint
	DetectionMethodPKMatch    = "pk_match"    // Inferred from PK type/cardinality matching
	DetectionMethodManual     = "manual"      // Created by user through chat
)

// Relationship status values.
const (
	RelationshipStatusConfirmed = "confirmed" // Auto-accepted (high confidence)
	RelationshipStatusPending   = "pending"   // Needs user review
	RelationshipStatusRejected  = "rejected"  // User declined
)

// EntityRelationship represents a relationship between two entities,
// discovered from FK constraints or inferred from PK matching.
// Stored in engine_entity_relationships table.
type EntityRelationship struct {
	ID                 uuid.UUID  `json:"id"`
	OntologyID         uuid.UUID  `json:"ontology_id"`
	SourceEntityID     uuid.UUID  `json:"source_entity_id"`
	TargetEntityID     uuid.UUID  `json:"target_entity_id"`
	SourceColumnSchema string     `json:"source_column_schema"`
	SourceColumnTable  string     `json:"source_column_table"`
	SourceColumnName   string     `json:"source_column_name"`
	TargetColumnSchema string     `json:"target_column_schema"`
	TargetColumnTable  string     `json:"target_column_table"`
	TargetColumnName   string     `json:"target_column_name"`
	DetectionMethod    string     `json:"detection_method"`      // "foreign_key", "pk_match", or "manual"
	Confidence         float64    `json:"confidence"`            // 1.0 for FK, 0.7-0.95 for pk_match
	Status             string     `json:"status"`                // "confirmed", "pending", "rejected"
	Cardinality        string     `json:"cardinality"`           // "1:1", "1:N", "N:1", "N:M", "unknown"
	Description        *string    `json:"description,omitempty"` // Optional description of the relationship
	Association        *string    `json:"association,omitempty"` // Semantic association for this direction (e.g., "placed_by", "contains")
	IsStale            bool       `json:"is_stale"`              // True when schema changed and needs re-evaluation
	CreatedBy          string     `json:"created_by"`            // Provenance: 'manual', 'mcp', 'inference'
	UpdatedBy          *string    `json:"updated_by,omitempty"`  // Who last updated (nil if never updated)
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          *time.Time `json:"updated_at,omitempty"`
}
