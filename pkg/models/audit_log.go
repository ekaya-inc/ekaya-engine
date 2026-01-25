package models

import (
	"time"

	"github.com/google/uuid"
)

// AuditEntityType represents the type of entity being audited.
const (
	AuditEntityTypeEntity          = "entity"
	AuditEntityTypeRelationship    = "relationship"
	AuditEntityTypeGlossaryTerm    = "glossary_term"
	AuditEntityTypeProjectKnowledge = "project_knowledge"
)

// AuditAction represents the type of action being audited.
const (
	AuditActionCreate = "create"
	AuditActionUpdate = "update"
	AuditActionDelete = "delete"
)

// AuditLogEntry represents a single entry in the unified audit log.
// Stored in engine_audit_log table.
type AuditLogEntry struct {
	ID         uuid.UUID `json:"id"`
	ProjectID  uuid.UUID `json:"project_id"`
	EntityType string    `json:"entity_type"` // 'entity', 'relationship', 'glossary_term', 'project_knowledge'
	EntityID   uuid.UUID `json:"entity_id"`   // ID of the affected object
	Action     string    `json:"action"`      // 'create', 'update', 'delete'

	// Who/how
	Source string     `json:"source"`              // 'inference', 'mcp', 'manual'
	UserID *uuid.UUID `json:"user_id,omitempty"` // Who triggered the action (from JWT, may be null for system operations)

	// What changed (for updates)
	ChangedFields map[string]FieldChange `json:"changed_fields,omitempty"` // {"field": {"old": ..., "new": ...}}

	CreatedAt time.Time `json:"created_at"`
}

// FieldChange represents the old and new values for a changed field.
type FieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}
