package models

import (
	"time"

	"github.com/google/uuid"
)

// AuditPageFilters contains common pagination and time-range filters.
type AuditPageFilters struct {
	Since  *time.Time
	Until  *time.Time
	Limit  int
	Offset int
}

// QueryExecutionFilters contains filters for the query executions tab.
type QueryExecutionFilters struct {
	AuditPageFilters
	UserID      string
	Success     *bool
	IsModifying *bool
	Source      string
	QueryID     *uuid.UUID
}

// OntologyChangeFilters contains filters for the ontology changes tab.
type OntologyChangeFilters struct {
	AuditPageFilters
	UserID     string
	EntityType string
	Action     string
	Source     string
}

// SchemaChangeFilters contains filters for the schema changes tab.
type SchemaChangeFilters struct {
	AuditPageFilters
	ChangeType string
	Status     string
	TableName  string
}

// QueryApprovalFilters contains filters for the query approvals tab.
type QueryApprovalFilters struct {
	AuditPageFilters
	Status      string
	SuggestedBy string
	ReviewedBy  string
}

// AuditSummary contains aggregate counts for the audit dashboard header.
type AuditSummary struct {
	TotalQueryExecutions  int `json:"total_query_executions"`
	FailedQueryCount      int `json:"failed_query_count"`
	DestructiveQueryCount int `json:"destructive_query_count"`
	OntologyChangesCount  int `json:"ontology_changes_count"`
	PendingSchemaChanges  int `json:"pending_schema_changes"`
	PendingQueryApprovals int `json:"pending_query_approvals"`
}

// QueryExecutionRow represents a row from the query executions audit view.
type QueryExecutionRow struct {
	ID              uuid.UUID `json:"id"`
	ProjectID       uuid.UUID `json:"project_id"`
	QueryID         uuid.UUID `json:"query_id"`
	SQL             string    `json:"sql"`
	ExecutedAt      time.Time `json:"executed_at"`
	RowCount        int       `json:"row_count"`
	ExecutionTimeMs int       `json:"execution_time_ms"`
	UserID          *string   `json:"user_id,omitempty"`
	Source          string    `json:"source"`
	IsModifying     bool      `json:"is_modifying"`
	Success         bool      `json:"success"`
	ErrorMessage    *string   `json:"error_message,omitempty"`
	QueryName       *string   `json:"query_name,omitempty"` // Joined from engine_queries
}
