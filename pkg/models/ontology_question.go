package models

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Question Status
// ============================================================================

// QuestionStatus represents the current status of an ontology question.
type QuestionStatus string

const (
	QuestionStatusPending   QuestionStatus = "pending"
	QuestionStatusSkipped   QuestionStatus = "skipped"
	QuestionStatusAnswered  QuestionStatus = "answered"
	QuestionStatusEscalated QuestionStatus = "escalated"
	QuestionStatusDismissed QuestionStatus = "dismissed"
	QuestionStatusDeleted   QuestionStatus = "deleted"
)

// ValidQuestionStatuses contains all valid question status values.
var ValidQuestionStatuses = []QuestionStatus{
	QuestionStatusPending,
	QuestionStatusSkipped,
	QuestionStatusAnswered,
	QuestionStatusEscalated,
	QuestionStatusDismissed,
	QuestionStatusDeleted,
}

// IsValidQuestionStatus checks if the given status is valid.
func IsValidQuestionStatus(s QuestionStatus) bool {
	for _, v := range ValidQuestionStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// ============================================================================
// Question Categories
// ============================================================================

// Question category constants
const (
	QuestionCategoryBusinessRules = "business_rules"
	QuestionCategoryRelationship  = "relationship"
	QuestionCategoryTerminology   = "terminology"
	QuestionCategoryEnumeration   = "enumeration"
	QuestionCategoryTemporal      = "temporal"
	QuestionCategoryDataQuality   = "data_quality"
)

// ValidQuestionCategories contains all valid question category values.
var ValidQuestionCategories = []string{
	QuestionCategoryBusinessRules,
	QuestionCategoryRelationship,
	QuestionCategoryTerminology,
	QuestionCategoryEnumeration,
	QuestionCategoryTemporal,
	QuestionCategoryDataQuality,
}

// ============================================================================
// Detected Patterns
// ============================================================================

// Detected pattern constants (why a question was generated)
const (
	PatternCentralEntity  = "central_entity"
	PatternStatusColumn   = "status_column"
	PatternTypeColumn     = "type_column"
	PatternEnumColumn     = "enum_column"
	PatternFKRelationship = "fk_relationship"
	PatternOrphanTable    = "orphan_table"
	PatternTimeSeries     = "time_series"
)

// ============================================================================
// Question Model
// ============================================================================

// QuestionAffects tracks which schema elements a question relates to.
type QuestionAffects struct {
	Tables  []string `json:"tables,omitempty"`
	Columns []string `json:"columns,omitempty"` // Format: "table.column"
}

// OntologyQuestion represents a question generated during ontology extraction.
type OntologyQuestion struct {
	ID               uuid.UUID        `json:"id"`
	ProjectID        uuid.UUID        `json:"project_id"`
	OntologyID       uuid.UUID        `json:"ontology_id"`
	WorkflowID       *uuid.UUID       `json:"workflow_id,omitempty"`
	ParentQuestionID *uuid.UUID       `json:"parent_question_id,omitempty"` // For follow-up traceability
	ContentHash      string           `json:"content_hash,omitempty"`       // SHA256 hash of category + text for deduplication
	Text             string           `json:"text"`
	Priority         int              `json:"priority"`    // 1=highest, 5=lowest
	IsRequired       bool             `json:"is_required"` // Required = entity not complete until answered
	Category         string           `json:"category,omitempty"`
	Reasoning        string           `json:"reasoning,omitempty"`
	Affects          *QuestionAffects `json:"affects,omitempty"`
	DetectedPattern  string           `json:"detected_pattern,omitempty"`
	Status           QuestionStatus   `json:"status"`
	StatusReason     string           `json:"status_reason,omitempty"` // Reason for skip/escalate/dismiss
	Answer           string           `json:"answer,omitempty"`
	AnsweredBy       *uuid.UUID       `json:"answered_by,omitempty"`
	AnsweredAt       *time.Time       `json:"answered_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// IsPending returns true if the question has not been answered or skipped.
func (q *OntologyQuestion) IsPending() bool {
	return q.Status == QuestionStatusPending
}

// IsSkipped returns true if the question was skipped by the user.
func (q *OntologyQuestion) IsSkipped() bool {
	return q.Status == QuestionStatusSkipped
}

// IsAnswered returns true if the question was answered.
func (q *OntologyQuestion) IsAnswered() bool {
	return q.Status == QuestionStatusAnswered
}

// IsDeleted returns true if the question was soft-deleted.
func (q *OntologyQuestion) IsDeleted() bool {
	return q.Status == QuestionStatusDeleted
}

// AffectedTableNames returns the list of affected table names.
func (q *OntologyQuestion) AffectedTableNames() []string {
	if q.Affects == nil {
		return nil
	}
	return q.Affects.Tables
}

// AffectedColumnNames returns the list of affected column names.
func (q *OntologyQuestion) AffectedColumnNames() []string {
	if q.Affects == nil {
		return nil
	}
	return q.Affects.Columns
}

// ComputeContentHash creates a SHA256 hash of category + text for deduplication.
// Returns the first 16 characters of the hex-encoded hash.
func (q *OntologyQuestion) ComputeContentHash() string {
	h := sha256.New()
	h.Write([]byte(q.Category + "|" + q.Text))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ============================================================================
// Question Generation Input
// ============================================================================

// QuestionGenerationInput provides context for LLM question generation.
type QuestionGenerationInput struct {
	Tables        []TableContext `json:"tables"`
	Relationships []RelContext   `json:"relationships"`
}

// TableContext provides table information for question generation.
type TableContext struct {
	Name         string          `json:"name"`
	RowCount     int64           `json:"row_count"`
	IsSelected   bool            `json:"is_selected"`
	Columns      []ColumnContext `json:"columns"`
	RelatedCount int             `json:"related_count"`
}

// ColumnContext provides column information for question generation.
type ColumnContext struct {
	Name          string `json:"name"`
	DataType      string `json:"data_type"`
	IsPrimaryKey  bool   `json:"is_pk,omitempty"`
	IsForeignKey  bool   `json:"is_fk,omitempty"`
	DistinctCount *int64 `json:"distinct,omitempty"`
	NullCount     *int64 `json:"nulls,omitempty"`
}

// RelContext provides relationship information for question generation.
type RelContext struct {
	SourceTable  string `json:"source"`
	TargetTable  string `json:"target"`
	SourceColumn string `json:"source_col"`
	TargetColumn string `json:"target_col"`
}

// ============================================================================
// Answer Processing Result
// ============================================================================

// AnswerResult contains the result of processing a question answer.
type AnswerResult struct {
	QuestionID     uuid.UUID         `json:"question_id"`
	FollowUp       *string           `json:"follow_up,omitempty"`
	NextQuestion   *OntologyQuestion `json:"next_question,omitempty"`
	AllComplete    bool              `json:"all_complete"`
	ActionsSummary string            `json:"actions_summary,omitempty"`
	Thinking       string            `json:"thinking,omitempty"`
}
