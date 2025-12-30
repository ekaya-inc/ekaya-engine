package models

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Detection Methods
// ============================================================================

// DetectionMethod represents how a relationship candidate was detected.
type DetectionMethod string

const (
	DetectionMethodValueMatch    DetectionMethod = "value_match"
	DetectionMethodNameInference DetectionMethod = "name_inference"
	DetectionMethodLLM           DetectionMethod = "llm"
	DetectionMethodHybrid        DetectionMethod = "hybrid"
)

// ValidDetectionMethods contains all valid detection method values.
var ValidDetectionMethods = []DetectionMethod{
	DetectionMethodValueMatch,
	DetectionMethodNameInference,
	DetectionMethodLLM,
	DetectionMethodHybrid,
}

// IsValidDetectionMethod checks if the given method is valid.
func IsValidDetectionMethod(m DetectionMethod) bool {
	for _, v := range ValidDetectionMethods {
		if v == m {
			return true
		}
	}
	return false
}

// ============================================================================
// Relationship Candidate Status (for new workflow system)
// ============================================================================

// RelationshipCandidateStatus represents the review state of a relationship candidate.
type RelationshipCandidateStatus string

const (
	RelCandidateStatusPending  RelationshipCandidateStatus = "pending"
	RelCandidateStatusAccepted RelationshipCandidateStatus = "accepted"
	RelCandidateStatusRejected RelationshipCandidateStatus = "rejected"
)

// ValidRelationshipCandidateStatuses contains all valid status values.
var ValidRelationshipCandidateStatuses = []RelationshipCandidateStatus{
	RelCandidateStatusPending,
	RelCandidateStatusAccepted,
	RelCandidateStatusRejected,
}

// IsValidRelationshipCandidateStatus checks if the given status is valid.
func IsValidRelationshipCandidateStatus(s RelationshipCandidateStatus) bool {
	for _, v := range ValidRelationshipCandidateStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// ============================================================================
// User Decision
// ============================================================================

// UserDecision represents an explicit user decision on a candidate.
type UserDecision string

const (
	UserDecisionAccepted UserDecision = "accepted"
	UserDecisionRejected UserDecision = "rejected"
)

// ValidUserDecisions contains all valid user decision values.
var ValidUserDecisions = []UserDecision{
	UserDecisionAccepted,
	UserDecisionRejected,
}

// IsValidUserDecision checks if the given decision is valid.
func IsValidUserDecision(d UserDecision) bool {
	for _, v := range ValidUserDecisions {
		if v == d {
			return true
		}
	}
	return false
}

// ============================================================================
// Relationship Candidate Model
// ============================================================================

// RelationshipCandidate represents a detected relationship candidate
// during the relationship discovery workflow.
type RelationshipCandidate struct {
	ID           uuid.UUID `json:"id"`
	WorkflowID   uuid.UUID `json:"workflow_id"`
	DatasourceID uuid.UUID `json:"datasource_id"`

	// Source and target (IDs are stored in DB)
	SourceColumnID uuid.UUID `json:"source_column_id"`
	TargetColumnID uuid.UUID `json:"target_column_id"`

	// Source and target names (populated by join queries, not stored in DB)
	SourceTable  string `json:"source_table,omitempty"`
	SourceColumn string `json:"source_column,omitempty"`
	TargetTable  string `json:"target_table,omitempty"`
	TargetColumn string `json:"target_column,omitempty"`

	// Detection results
	DetectionMethod DetectionMethod `json:"detection_method"`
	Confidence      float64         `json:"confidence"` // 0.0-1.0
	LLMReasoning    *string         `json:"llm_reasoning,omitempty"`

	// Metrics from detection (sample-based)
	ValueMatchRate *float64 `json:"value_match_rate,omitempty"` // NULL if not value-matched
	NameSimilarity *float64 `json:"name_similarity,omitempty"`  // Column name similarity score

	// Metrics from test join (actual SQL join)
	Cardinality    *string  `json:"cardinality,omitempty"`      // "1:1", "1:N", "N:1", "N:M"
	JoinMatchRate  *float64 `json:"join_match_rate,omitempty"`  // Actual match rate from join
	OrphanRate     *float64 `json:"orphan_rate,omitempty"`      // % of source rows with no match
	TargetCoverage *float64 `json:"target_coverage,omitempty"`  // % of target rows that are referenced
	SourceRowCount *int64   `json:"source_row_count,omitempty"` // Total source rows
	TargetRowCount *int64   `json:"target_row_count,omitempty"` // Total target rows
	MatchedRows    *int64   `json:"matched_rows,omitempty"`     // Source rows with matches
	OrphanRows     *int64   `json:"orphan_rows,omitempty"`      // Source rows without matches

	// Review state
	Status       RelationshipCandidateStatus `json:"status"`                  // pending, accepted, rejected
	IsRequired   bool                        `json:"is_required"`             // Must user make a call before save?
	UserDecision *UserDecision               `json:"user_decision,omitempty"` // accepted, rejected (after user action)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsAutoAccepted returns true if the candidate was auto-accepted based on high confidence.
func (c *RelationshipCandidate) IsAutoAccepted() bool {
	return c.Status == RelCandidateStatusAccepted && !c.IsRequired && c.UserDecision == nil
}

// IsAutoRejected returns true if the candidate was auto-rejected based on low confidence.
func (c *RelationshipCandidate) IsAutoRejected() bool {
	return c.Status == RelCandidateStatusRejected && !c.IsRequired && c.UserDecision == nil
}

// IsUserAccepted returns true if the user explicitly accepted the candidate.
func (c *RelationshipCandidate) IsUserAccepted() bool {
	return c.UserDecision != nil && *c.UserDecision == UserDecisionAccepted
}

// IsUserRejected returns true if the user explicitly rejected the candidate.
func (c *RelationshipCandidate) IsUserRejected() bool {
	return c.UserDecision != nil && *c.UserDecision == UserDecisionRejected
}

// NeedsReview returns true if the candidate requires user review before saving.
func (c *RelationshipCandidate) NeedsReview() bool {
	return c.IsRequired && c.Status == RelCandidateStatusPending
}
