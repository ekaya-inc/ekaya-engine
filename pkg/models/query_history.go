package models

import (
	"time"

	"github.com/google/uuid"
)

// QueryHistoryEntry represents a single entry in the MCP query history.
// Only successful queries are recorded.
type QueryHistoryEntry struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	UserID    string    `json:"user_id"`

	// The query itself
	NaturalLanguage string `json:"natural_language"`
	SQL             string `json:"sql"`

	// Execution details
	ExecutedAt          time.Time `json:"executed_at"`
	ExecutionDurationMs *int      `json:"execution_duration_ms,omitempty"`
	RowCount            *int      `json:"row_count,omitempty"`

	// Learning signals
	UserFeedback    *string `json:"user_feedback,omitempty"`
	FeedbackComment *string `json:"feedback_comment,omitempty"`

	// Query classification
	QueryType        *string  `json:"query_type,omitempty"`
	TablesUsed       []string `json:"tables_used,omitempty"`
	AggregationsUsed []string `json:"aggregations_used,omitempty"`
	TimeFilters      any      `json:"time_filters,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// QueryHistoryFilters contains filters for querying the query history table.
type QueryHistoryFilters struct {
	UserID     string
	TablesUsed []string
	Since      *time.Time
	Limit      int
}
