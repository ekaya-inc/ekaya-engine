package models

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Workflow Entity Types
// ============================================================================

// WorkflowEntityType represents the type of entity being tracked in workflow state.
type WorkflowEntityType string

const (
	WorkflowEntityTypeGlobal WorkflowEntityType = "global"
	WorkflowEntityTypeTable  WorkflowEntityType = "table"
	WorkflowEntityTypeColumn WorkflowEntityType = "column"
)

// ValidWorkflowEntityTypes contains all valid entity type values.
var ValidWorkflowEntityTypes = []WorkflowEntityType{
	WorkflowEntityTypeGlobal,
	WorkflowEntityTypeTable,
	WorkflowEntityTypeColumn,
}

// IsValidWorkflowEntityType checks if the given type is valid.
func IsValidWorkflowEntityType(t WorkflowEntityType) bool {
	for _, v := range ValidWorkflowEntityTypes {
		if v == t {
			return true
		}
	}
	return false
}

// ============================================================================
// Workflow Entity Status
// ============================================================================

// WorkflowEntityStatus represents the extraction status of an entity.
// State machine:
//
//	pending → scanning → scanned → analyzing → complete
//	                                    ↓
//	                              needs_input → (answer) → analyzing
//
//	Any state can transition to: failed
type WorkflowEntityStatus string

const (
	WorkflowEntityStatusPending    WorkflowEntityStatus = "pending"
	WorkflowEntityStatusScanning   WorkflowEntityStatus = "scanning"
	WorkflowEntityStatusScanned    WorkflowEntityStatus = "scanned"
	WorkflowEntityStatusAnalyzing  WorkflowEntityStatus = "analyzing"
	WorkflowEntityStatusComplete   WorkflowEntityStatus = "complete"
	WorkflowEntityStatusNeedsInput WorkflowEntityStatus = "needs_input"
	WorkflowEntityStatusFailed     WorkflowEntityStatus = "failed"
)

// ValidWorkflowEntityStatuses contains all valid status values.
var ValidWorkflowEntityStatuses = []WorkflowEntityStatus{
	WorkflowEntityStatusPending,
	WorkflowEntityStatusScanning,
	WorkflowEntityStatusScanned,
	WorkflowEntityStatusAnalyzing,
	WorkflowEntityStatusComplete,
	WorkflowEntityStatusNeedsInput,
	WorkflowEntityStatusFailed,
}

// IsValidWorkflowEntityStatus checks if the given status is valid.
func IsValidWorkflowEntityStatus(s WorkflowEntityStatus) bool {
	for _, v := range ValidWorkflowEntityStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the status is a terminal state (complete or failed).
func (s WorkflowEntityStatus) IsTerminal() bool {
	return s == WorkflowEntityStatusComplete || s == WorkflowEntityStatusFailed
}

// CanTransitionTo returns true if transitioning from this status to the target is valid.
func (s WorkflowEntityStatus) CanTransitionTo(target WorkflowEntityStatus) bool {
	// Any state can transition to failed
	if target == WorkflowEntityStatusFailed {
		return true
	}

	switch s {
	case WorkflowEntityStatusPending:
		return target == WorkflowEntityStatusScanning
	case WorkflowEntityStatusScanning:
		return target == WorkflowEntityStatusScanned
	case WorkflowEntityStatusScanned:
		return target == WorkflowEntityStatusAnalyzing
	case WorkflowEntityStatusAnalyzing:
		return target == WorkflowEntityStatusComplete || target == WorkflowEntityStatusNeedsInput
	case WorkflowEntityStatusNeedsInput:
		return target == WorkflowEntityStatusAnalyzing
	case WorkflowEntityStatusComplete, WorkflowEntityStatusFailed:
		return false // Terminal states
	default:
		return false
	}
}

// ============================================================================
// Workflow Entity State Model
// ============================================================================

// WorkflowEntityState tracks the state of an entity during ontology extraction.
// This is ephemeral data that is deleted when the workflow completes.
// Note: Named WorkflowEntityState to avoid collision with WorkflowState (string type in ontology_workflow.go).
type WorkflowEntityState struct {
	ID              uuid.UUID            `json:"id"`
	ProjectID       uuid.UUID            `json:"project_id"`
	OntologyID      uuid.UUID            `json:"ontology_id"`
	WorkflowID      uuid.UUID            `json:"workflow_id"`
	EntityType      WorkflowEntityType   `json:"entity_type"`
	EntityKey       string               `json:"entity_key"`
	Status          WorkflowEntityStatus `json:"status"`
	StateData       *WorkflowStateData   `json:"state_data,omitempty"`
	DataFingerprint *string              `json:"data_fingerprint,omitempty"`
	LastError       *string              `json:"last_error,omitempty"`
	RetryCount      int                  `json:"retry_count"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// WorkflowStateData contains all extraction state for an entity.
// This JSONB field holds everything needed during extraction:
// - gathered: Data from SQL scanning phase (column stats, sample values, etc.)
// - llm_analysis: Intermediate LLM reasoning and conclusions
// - questions: Questions generated for this entity
// - answers: User answers (can cascade to update other entities)
type WorkflowStateData struct {
	Gathered    map[string]any     `json:"gathered,omitempty"`
	LLMAnalysis map[string]any     `json:"llm_analysis,omitempty"`
	Questions   []WorkflowQuestion `json:"questions,omitempty"`
	Answers     []WorkflowAnswer   `json:"answers,omitempty"`
}

// ============================================================================
// Workflow Data Types
// ============================================================================

// ColumnScanData holds statistics from SQL scanning (no LLM required).
// This data is collected in the Scanning phase before LLM analysis.
// Stored in WorkflowStateData.Gathered during extraction, not persisted to ontology.
type ColumnScanData struct {
	RowCount         int64     `json:"row_count"`
	NonNullCount     int64     `json:"non_null_count"`
	DistinctCount    int64     `json:"distinct_count"`
	NullPercent      float64   `json:"null_percent"`
	SampleValues     []string  `json:"sample_values,omitempty"`     // Up to 50 distinct values
	IsEnumCandidate  bool      `json:"is_enum_candidate"`           // distinct_count <= 50 AND < 10% of rows
	ValueFingerprint string    `json:"value_fingerprint,omitempty"` // SHA256 of sorted values for change detection
	ScannedAt        time.Time `json:"scanned_at"`
}

// ============================================================================
// Workflow Question/Answer Types
// ============================================================================

// WorkflowQuestion is the in-workflow question representation.
// This is stored in state_data.questions and is ephemeral (deleted with workflow).
type WorkflowQuestion struct {
	ID              string           `json:"id"`
	Text            string           `json:"text"`
	Priority        int              `json:"priority"`
	IsRequired      bool             `json:"is_required"`
	Category        string           `json:"category,omitempty"`
	Reasoning       string           `json:"reasoning,omitempty"`
	Affects         *QuestionAffects `json:"affects,omitempty"`
	DetectedPattern string           `json:"detected_pattern,omitempty"`
	Status          string           `json:"status"` // pending, skipped, answered
	Answer          string           `json:"answer,omitempty"`
	AnsweredBy      string           `json:"answered_by,omitempty"`
	AnsweredAt      *time.Time       `json:"answered_at,omitempty"`
	ParentID        string           `json:"parent_id,omitempty"` // For follow-ups
}

// WorkflowAnswer records a user's answer with all actions taken.
// This is stored in state_data.answers for audit trail.
type WorkflowAnswer struct {
	QuestionID     string    `json:"question_id"`
	Answer         string    `json:"answer"`
	AnsweredBy     string    `json:"answered_by"`
	AnsweredAt     time.Time `json:"answered_at"`
	EntityUpdates  []string  `json:"entity_updates,omitempty"`  // Summary of what was updated
	ColumnUpdates  []string  `json:"column_updates,omitempty"`  // Summary of what was updated
	KnowledgeFacts []string  `json:"knowledge_facts,omitempty"` // IDs of facts created
	FollowUpID     string    `json:"follow_up_id,omitempty"`    // ID of follow-up if created
}

// IsPending returns true if the question is pending.
func (q *WorkflowQuestion) IsPending() bool {
	return q.Status == string(QuestionStatusPending)
}

// IsAnswered returns true if the question is answered.
func (q *WorkflowQuestion) IsAnswered() bool {
	return q.Status == string(QuestionStatusAnswered)
}

// ============================================================================
// WorkflowEntityState Helper Methods
// ============================================================================

// IsGlobal returns true if this is a global entity.
func (ws *WorkflowEntityState) IsGlobal() bool {
	return ws.EntityType == WorkflowEntityTypeGlobal
}

// IsTable returns true if this is a table entity.
func (ws *WorkflowEntityState) IsTable() bool {
	return ws.EntityType == WorkflowEntityTypeTable
}

// IsColumn returns true if this is a column entity.
func (ws *WorkflowEntityState) IsColumn() bool {
	return ws.EntityType == WorkflowEntityTypeColumn
}

// TableName returns the table name from the entity key.
// For global: returns ""
// For table: returns the entity key (e.g., "orders")
// For column: returns the table part (e.g., "orders" from "orders.status")
func (ws *WorkflowEntityState) TableName() string {
	switch ws.EntityType {
	case WorkflowEntityTypeGlobal:
		return ""
	case WorkflowEntityTypeTable:
		return ws.EntityKey
	case WorkflowEntityTypeColumn:
		parts := strings.SplitN(ws.EntityKey, ".", 2)
		if len(parts) >= 1 {
			return parts[0]
		}
		return ""
	default:
		return ""
	}
}

// ColumnName returns the column name for column entities, empty string otherwise.
func (ws *WorkflowEntityState) ColumnName() string {
	if ws.EntityType != WorkflowEntityTypeColumn {
		return ""
	}
	parts := strings.SplitN(ws.EntityKey, ".", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// ============================================================================
// Entity Key Helpers
// ============================================================================

// GlobalEntityKey returns the entity key for the global entity.
func GlobalEntityKey() string {
	return ""
}

// TableEntityKey returns the entity key for a table entity.
// Entity keys never include database schema prefix (e.g., use "orders" not "public.orders").
func TableEntityKey(tableName string) string {
	return tableName
}

// ColumnEntityKey returns the entity key for a column entity.
// Format: "table_name.column_name" (e.g., "orders.status").
func ColumnEntityKey(tableName, columnName string) string {
	return tableName + "." + columnName
}

// ParseColumnEntityKey extracts table and column names from a column entity key.
// Returns ("", "") if the key is not a valid column key.
func ParseColumnEntityKey(entityKey string) (tableName, columnName string) {
	parts := strings.SplitN(entityKey, ".", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
