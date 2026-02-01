package services

import "github.com/google/uuid"

// RelationshipCandidate holds all data needed for LLM to evaluate a potential FK relationship.
// This struct is populated during the deterministic collection phase and passed to the
// LLM validation phase for semantic evaluation.
type RelationshipCandidate struct {
	// Source column info (the FK column)
	SourceTable         string   `json:"source_table"`
	SourceColumn        string   `json:"source_column"`
	SourceDataType      string   `json:"source_data_type"`
	SourceIsPK          bool     `json:"source_is_pk"`
	SourceDistinctCount int64    `json:"source_distinct_count"`
	SourceNullRate      float64  `json:"source_null_rate"`
	SourceSamples       []string `json:"source_samples"` // Up to 10 sample values

	// Target column info (the referenced PK/unique column)
	TargetTable         string   `json:"target_table"`
	TargetColumn        string   `json:"target_column"`
	TargetDataType      string   `json:"target_data_type"`
	TargetIsPK          bool     `json:"target_is_pk"`
	TargetDistinctCount int64    `json:"target_distinct_count"`
	TargetNullRate      float64  `json:"target_null_rate"`
	TargetSamples       []string `json:"target_samples"` // Up to 10 sample values

	// Join analysis results (from SQL)
	JoinCount      int64 `json:"join_count"`      // Rows that matched
	OrphanCount    int64 `json:"orphan_count"`    // Source values not in target
	ReverseOrphans int64 `json:"reverse_orphans"` // Target values not in source
	SourceMatched  int64 `json:"source_matched"`  // Distinct source values that matched
	TargetMatched  int64 `json:"target_matched"`  // Distinct target values that matched

	// Context from ColumnFeatures (if available)
	SourcePurpose string `json:"source_purpose,omitempty"` // "identifier", "timestamp", "measure", etc.
	SourceRole    string `json:"source_role,omitempty"`    // "primary_key", "foreign_key", "attribute"
	TargetPurpose string `json:"target_purpose,omitempty"`
	TargetRole    string `json:"target_role,omitempty"`

	// Internal tracking (not passed to LLM)
	SourceColumnID uuid.UUID `json:"-"`
	TargetColumnID uuid.UUID `json:"-"`
}

// RelationshipValidationResult is the LLM response for a relationship candidate.
type RelationshipValidationResult struct {
	IsValidFK   bool    `json:"is_valid_fk"`
	Confidence  float64 `json:"confidence"`  // 0.0-1.0
	Cardinality string  `json:"cardinality"` // "1:1", "N:1", "1:N", "N:M"
	Reasoning   string  `json:"reasoning"`   // Why valid/invalid
	SourceRole  string  `json:"source_role"` // "owner", "creator", etc. (optional)
}

// ValidatedRelationship combines a candidate with its validation result.
type ValidatedRelationship struct {
	Candidate *RelationshipCandidate
	Result    *RelationshipValidationResult
}

// OrphanRate calculates the percentage of source values that have no match in the target.
// Returns 0.0 if there are no source values to compare.
func (c *RelationshipCandidate) OrphanRate() float64 {
	if c.SourceDistinctCount == 0 {
		return 0.0
	}
	return float64(c.OrphanCount) / float64(c.SourceDistinctCount)
}

// MatchRate calculates the percentage of source values that matched target values.
// Returns 0.0 if there are no source values to compare.
func (c *RelationshipCandidate) MatchRate() float64 {
	if c.SourceDistinctCount == 0 {
		return 0.0
	}
	return float64(c.SourceMatched) / float64(c.SourceDistinctCount)
}

// CoverageRate calculates what percentage of target values are referenced by source.
// Returns 0.0 if there are no target values to compare.
func (c *RelationshipCandidate) CoverageRate() float64 {
	if c.TargetDistinctCount == 0 {
		return 0.0
	}
	return float64(c.TargetMatched) / float64(c.TargetDistinctCount)
}
