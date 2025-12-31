package models

import (
	"time"

	"github.com/google/uuid"
)

// Detection methods for entity relationships.
const (
	DetectionMethodForeignKey = "foreign_key" // Discovered from database FK constraint
	DetectionMethodPKMatch    = "pk_match"    // Inferred from PK type/cardinality matching
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
	ID                 uuid.UUID `json:"id"`
	OntologyID         uuid.UUID `json:"ontology_id"`
	SourceEntityID     uuid.UUID `json:"source_entity_id"`
	TargetEntityID     uuid.UUID `json:"target_entity_id"`
	SourceColumnSchema string    `json:"source_column_schema"`
	SourceColumnTable  string    `json:"source_column_table"`
	SourceColumnName   string    `json:"source_column_name"`
	TargetColumnSchema string    `json:"target_column_schema"`
	TargetColumnTable  string    `json:"target_column_table"`
	TargetColumnName   string    `json:"target_column_name"`
	DetectionMethod    string    `json:"detection_method"` // "foreign_key" or "pk_match"
	Confidence         float64   `json:"confidence"`       // 1.0 for FK, 0.7-0.95 for pk_match
	Status             string    `json:"status"`           // "confirmed", "pending", "rejected"
	CreatedAt          time.Time `json:"created_at"`
}
