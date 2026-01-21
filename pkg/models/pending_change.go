package models

import (
	"time"

	"github.com/google/uuid"
)

// Change type constants for schema changes.
const (
	ChangeTypeNewTable       = "new_table"
	ChangeTypeDroppedTable   = "dropped_table"
	ChangeTypeNewColumn      = "new_column"
	ChangeTypeDroppedColumn  = "dropped_column"
	ChangeTypeModifiedColumn = "modified_column"
	// Data-level changes (for PLAN-04)
	ChangeTypeNewEnumValue      = "new_enum_value"
	ChangeTypeCardinalityChange = "cardinality_change"
	ChangeTypeNewFKPattern      = "new_fk_pattern"
)

// Change source constants.
const (
	ChangeSourceSchemaRefresh = "schema_refresh"
	ChangeSourceDataScan      = "data_scan"
	ChangeSourceManual        = "manual"
)

// Change status constants.
const (
	ChangeStatusPending     = "pending"
	ChangeStatusApproved    = "approved"
	ChangeStatusRejected    = "rejected"
	ChangeStatusAutoApplied = "auto_applied"
)

// Suggested action constants.
const (
	SuggestedActionCreateEntity         = "create_entity"
	SuggestedActionReviewEntity         = "review_entity"
	SuggestedActionCreateColumnMetadata = "create_column_metadata"
	SuggestedActionUpdateColumnMetadata = "update_column_metadata"
	SuggestedActionReviewColumn         = "review_column"
	// Data-level change actions (for PLAN-04)
	SuggestedActionUpdateRelationship = "update_relationship"
	SuggestedActionCreateRelationship = "create_relationship"
)

// PendingChange represents a detected schema or data change awaiting review.
type PendingChange struct {
	ID               uuid.UUID      `json:"id"`
	ProjectID        uuid.UUID      `json:"project_id"`
	ChangeType       string         `json:"change_type"`
	ChangeSource     string         `json:"change_source"`
	TableName        string         `json:"table_name,omitempty"`
	ColumnName       string         `json:"column_name,omitempty"`
	OldValue         map[string]any `json:"old_value,omitempty"`
	NewValue         map[string]any `json:"new_value,omitempty"`
	SuggestedAction  string         `json:"suggested_action,omitempty"`
	SuggestedPayload map[string]any `json:"suggested_payload,omitempty"`
	Status           string         `json:"status"`
	ReviewedBy       *string        `json:"reviewed_by,omitempty"`
	ReviewedAt       *time.Time     `json:"reviewed_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

// ColumnChange represents a column that was added or removed.
type ColumnChange struct {
	TableName  string `json:"table_name"`
	ColumnName string `json:"column_name"`
	DataType   string `json:"data_type"`
}

// ColumnModification represents a column whose type changed.
type ColumnModification struct {
	TableName  string `json:"table_name"`
	ColumnName string `json:"column_name"`
	OldType    string `json:"old_type"`
	NewType    string `json:"new_type"`
}
