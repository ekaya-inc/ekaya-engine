package models

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Classification Paths
// ============================================================================

// ClassificationPath identifies the analysis path used for a column.
type ClassificationPath string

const (
	ClassificationPathTimestamp  ClassificationPath = "timestamp"
	ClassificationPathBoolean    ClassificationPath = "boolean"
	ClassificationPathEnum       ClassificationPath = "enum"
	ClassificationPathUUID       ClassificationPath = "uuid"
	ClassificationPathExternalID ClassificationPath = "external_id"
	ClassificationPathNumeric    ClassificationPath = "numeric"
	ClassificationPathText       ClassificationPath = "text"
	ClassificationPathJSON       ClassificationPath = "json"
	ClassificationPathUnknown    ClassificationPath = "unknown"
)

// ValidClassificationPaths contains all valid classification path values.
var ValidClassificationPaths = []ClassificationPath{
	ClassificationPathTimestamp,
	ClassificationPathBoolean,
	ClassificationPathEnum,
	ClassificationPathUUID,
	ClassificationPathExternalID,
	ClassificationPathNumeric,
	ClassificationPathText,
	ClassificationPathJSON,
	ClassificationPathUnknown,
}

// IsValidClassificationPath checks if the given path is valid.
func IsValidClassificationPath(p ClassificationPath) bool {
	return slices.Contains(ValidClassificationPaths, p)
}

// ============================================================================
// Column Data Profile (Phase 1 Output)
// ============================================================================

// ColumnDataProfile holds raw data collected for a column during Phase 1.
// This includes statistics, sample values, detected patterns, and the assigned
// classification path. Phase 1 is deterministic (no LLM calls).
type ColumnDataProfile struct {
	ColumnID   uuid.UUID `json:"column_id"`
	ColumnName string    `json:"column_name"`
	TableID    uuid.UUID `json:"table_id"`
	TableName  string    `json:"table_name"`
	DataType   string    `json:"data_type"`

	// From DDL (schema metadata)
	IsPrimaryKey bool `json:"is_primary_key"`
	IsUnique     bool `json:"is_unique"`
	IsNullable   bool `json:"is_nullable"`

	// Statistics
	RowCount      int64   `json:"row_count"`
	DistinctCount int64   `json:"distinct_count"`
	NullCount     int64   `json:"null_count"`
	NullRate      float64 `json:"null_rate"`      // null_count / row_count (0.0 - 1.0)
	Cardinality   float64 `json:"cardinality"`    // distinct_count / row_count (0.0 - 1.0)

	// For numeric columns
	MinValue *float64 `json:"min_value,omitempty"`
	MaxValue *float64 `json:"max_value,omitempty"`
	AvgValue *float64 `json:"avg_value,omitempty"`

	// For text columns
	MinLength *int64 `json:"min_length,omitempty"`
	MaxLength *int64 `json:"max_length,omitempty"`

	// Sample values (up to 50 distinct values)
	SampleValues []string `json:"sample_values,omitempty"`

	// Pattern detection results (from sample analysis)
	DetectedPatterns []DetectedPattern `json:"detected_patterns,omitempty"`

	// Routing decision (determined in Phase 1, processed in Phase 2)
	ClassificationPath ClassificationPath `json:"classification_path"`
}

// HasOnlyBooleanValues returns true if the sample values contain only boolean-like values.
// This includes {0, 1}, {true, false}, {yes, no}, {Y, N}, {T, F} (case-insensitive).
func (p *ColumnDataProfile) HasOnlyBooleanValues() bool {
	if len(p.SampleValues) == 0 {
		return false
	}
	if p.DistinctCount > 2 {
		return false
	}
	booleanSets := []map[string]bool{
		{"0": true, "1": true},
		{"true": true, "false": true},
		{"yes": true, "no": true},
		{"y": true, "n": true},
		{"t": true, "f": true},
	}
	for _, boolSet := range booleanSets {
		allMatch := true
		for _, val := range p.SampleValues {
			if !boolSet[normalizeForBooleanCheck(val)] {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

// normalizeForBooleanCheck lowercases and trims a value for boolean comparison.
func normalizeForBooleanCheck(val string) string {
	// Simple lowercase - Go's strings package is not imported here,
	// but in the implementation we'll use strings.ToLower and strings.TrimSpace
	result := ""
	for _, c := range val {
		if c >= 'A' && c <= 'Z' {
			result += string(c + 32)
		} else if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			result += string(c)
		}
	}
	return result
}

// MatchesPattern returns true if a pattern with the given name was detected
// in the sample values with a match rate above the threshold (default 0.95).
func (p *ColumnDataProfile) MatchesPattern(patternName string) bool {
	return p.MatchesPatternWithThreshold(patternName, 0.95)
}

// MatchesPatternWithThreshold returns true if a pattern with the given name was detected
// in the sample values with a match rate at or above the specified threshold.
func (p *ColumnDataProfile) MatchesPatternWithThreshold(patternName string, threshold float64) bool {
	for _, dp := range p.DetectedPatterns {
		if dp.PatternName == patternName && dp.MatchRate >= threshold {
			return true
		}
	}
	return false
}

// ============================================================================
// Detected Pattern
// ============================================================================

// DetectedPattern represents a regex pattern match result on sample values.
// Patterns are matched against column data (not names) to make data-driven decisions.
type DetectedPattern struct {
	// PatternName identifies the pattern (e.g., "uuid", "stripe_id", "iso4217_currency")
	PatternName string `json:"pattern_name"`

	// MatchRate is the percentage of samples matching (0.0 - 1.0)
	MatchRate float64 `json:"match_rate"`

	// MatchedValues contains examples of values that matched the pattern
	MatchedValues []string `json:"matched_values,omitempty"`
}

// Pattern names for external service IDs and other recognized patterns.
const (
	PatternUUID            = "uuid"
	PatternStripeID        = "stripe_id"
	PatternAWSSES          = "aws_ses"
	PatternTwilioSID       = "twilio_sid"
	PatternISO4217         = "iso4217"
	PatternUnixSeconds     = "unix_seconds"
	PatternUnixMillis      = "unix_millis"
	PatternUnixMicros      = "unix_micros"
	PatternUnixNanos       = "unix_nanos"
	PatternEmail           = "email"
	PatternURL             = "url"
	PatternGenericExtID    = "generic_external_id"
)

// ============================================================================
// Column Features (Final Classification Results)
// ============================================================================

// ColumnFeatures holds the final classification results for a column.
// This is populated after LLM-based classification in Phase 2 and subsequent phases.
type ColumnFeatures struct {
	ColumnID uuid.UUID `json:"column_id"`

	// Classification path taken
	ClassificationPath ClassificationPath `json:"classification_path"`

	// Common fields
	Purpose      string  `json:"purpose"`       // "identifier", "timestamp", "flag", "measure", "enum", "text", "json"
	SemanticType string  `json:"semantic_type"` // More specific: "soft_delete_timestamp", "currency_cents", etc.
	Role         string  `json:"role"`          // "primary_key", "foreign_key", "attribute", "measure"
	Description  string  `json:"description"`   // LLM-generated business description
	Confidence   float64 `json:"confidence"`    // Classification confidence (0.0 - 1.0)

	// Path-specific results (populated based on classification)
	TimestampFeatures  *TimestampFeatures  `json:"timestamp_features,omitempty"`
	BooleanFeatures    *BooleanFeatures    `json:"boolean_features,omitempty"`
	EnumFeatures       *EnumFeatures       `json:"enum_features,omitempty"`
	IdentifierFeatures *IdentifierFeatures `json:"identifier_features,omitempty"`
	MonetaryFeatures   *MonetaryFeatures   `json:"monetary_features,omitempty"`

	// Flags for follow-up phases (set during Phase 2)
	NeedsEnumAnalysis     bool `json:"needs_enum_analysis"`       // Enqueue to Phase 3
	NeedsFKResolution     bool `json:"needs_fk_resolution"`       // Enqueue to Phase 4
	NeedsCrossColumnCheck bool `json:"needs_cross_column_check"`  // Enqueue to Phase 5

	// Analysis metadata
	AnalyzedAt   time.Time `json:"analyzed_at"`
	LLMModelUsed string    `json:"llm_model_used,omitempty"`
}

// Purpose constants for column classification.
const (
	PurposeIdentifier = "identifier"
	PurposeTimestamp  = "timestamp"
	PurposeFlag       = "flag"
	PurposeMeasure    = "measure"
	PurposeEnum       = "enum"
	PurposeText       = "text"
	PurposeJSON       = "json"
)

// Role constants for column classification.
const (
	RolePrimaryKey = "primary_key"
	RoleForeignKey = "foreign_key"
	RoleAttribute  = "attribute"
	RoleMeasure    = "measure"
)

// ============================================================================
// Timestamp Features
// ============================================================================

// TimestampFeatures holds classification results for timestamp columns.
type TimestampFeatures struct {
	// TimestampPurpose describes the semantic purpose of the timestamp.
	// Values: "audit_created", "audit_updated", "soft_delete", "event_time",
	//         "scheduled_time", "expiration", "cursor"
	TimestampPurpose string `json:"timestamp_purpose"`

	// TimestampScale indicates the precision for bigint timestamps.
	// Values: "seconds", "milliseconds", "microseconds", "nanoseconds", or empty for native timestamps.
	TimestampScale string `json:"timestamp_scale,omitempty"`

	// IsSoftDelete indicates if this timestamp is used for soft deletes (high null rate).
	IsSoftDelete bool `json:"is_soft_delete"`

	// IsAuditField indicates if this is an audit timestamp (created_at, updated_at).
	IsAuditField bool `json:"is_audit_field"`
}

// Timestamp purpose constants.
const (
	TimestampPurposeAuditCreated = "audit_created"
	TimestampPurposeAuditUpdated = "audit_updated"
	TimestampPurposeSoftDelete   = "soft_delete"
	TimestampPurposeEventTime    = "event_time"
	TimestampPurposeScheduled    = "scheduled_time"
	TimestampPurposeExpiration   = "expiration"
	TimestampPurposeCursor       = "cursor"
)

// Timestamp scale constants.
const (
	TimestampScaleSeconds      = "seconds"
	TimestampScaleMilliseconds = "milliseconds"
	TimestampScaleMicroseconds = "microseconds"
	TimestampScaleNanoseconds  = "nanoseconds"
)

// ============================================================================
// Boolean Features
// ============================================================================

// BooleanFeatures holds classification results for boolean columns.
type BooleanFeatures struct {
	// TrueMeaning describes what a true value represents.
	TrueMeaning string `json:"true_meaning"`

	// FalseMeaning describes what a false value represents.
	FalseMeaning string `json:"false_meaning"`

	// BooleanType categorizes the boolean's purpose.
	// Values: "feature_flag", "status_indicator", "permission", "preference", "state"
	BooleanType string `json:"boolean_type"`

	// TruePercentage is the percentage of true values (0.0 - 100.0).
	TruePercentage float64 `json:"true_percentage"`

	// FalsePercentage is the percentage of false values (0.0 - 100.0).
	FalsePercentage float64 `json:"false_percentage"`
}

// Boolean type constants.
const (
	BooleanTypeFeatureFlag     = "feature_flag"
	BooleanTypeStatusIndicator = "status_indicator"
	BooleanTypePermission      = "permission"
	BooleanTypePreference      = "preference"
	BooleanTypeState           = "state"
)

// ============================================================================
// Enum Features
// ============================================================================

// EnumFeatures holds classification results for enum/state columns.
type EnumFeatures struct {
	// IsStateMachine indicates if the values represent a state machine workflow.
	IsStateMachine bool `json:"is_state_machine"`

	// Values contains the labeled enum values with their meanings.
	Values []ColumnEnumValue `json:"values"`

	// StateDescription describes the state machine workflow (if applicable).
	StateDescription string `json:"state_description,omitempty"`
}

// ColumnEnumValue represents a single enum value with its label and category
// for the column feature extraction pipeline. This is separate from the EnumValue
// type in ontology.go which is used for storing enum metadata in the ontology.
type ColumnEnumValue struct {
	// Value is the raw value stored in the database.
	Value string `json:"value"`

	// Label is the human-readable label for this value.
	Label string `json:"label"`

	// Category classifies the value within a state machine.
	// Values: "initial", "in_progress", "terminal", "terminal_success", "terminal_error"
	Category string `json:"category,omitempty"`

	// Count is the number of rows with this value.
	Count int64 `json:"count"`

	// Percentage is the percentage of rows with this value (0.0 - 100.0).
	Percentage float64 `json:"percentage"`
}

// Enum value category constants (for state machines).
const (
	EnumCategoryInitial         = "initial"
	EnumCategoryInProgress      = "in_progress"
	EnumCategoryTerminal        = "terminal"
	EnumCategoryTerminalSuccess = "terminal_success"
	EnumCategoryTerminalError   = "terminal_error"
)

// ============================================================================
// Identifier Features
// ============================================================================

// IdentifierFeatures holds classification results for identifier columns (UUID, external IDs).
type IdentifierFeatures struct {
	// IdentifierType categorizes the identifier.
	// Values: "internal_uuid", "external_uuid", "primary_key", "foreign_key", "external_service_id"
	IdentifierType string `json:"identifier_type"`

	// ExternalService identifies the external service (e.g., "stripe", "twilio", "aws_ses").
	// Only populated for external service IDs.
	ExternalService string `json:"external_service,omitempty"`

	// FKTargetTable is the likely target table for foreign key references.
	// Only populated after FK resolution analysis.
	FKTargetTable string `json:"fk_target_table,omitempty"`

	// FKTargetColumn is the likely target column for foreign key references.
	// Only populated after FK resolution analysis.
	FKTargetColumn string `json:"fk_target_column,omitempty"`

	// FKConfidence is the confidence in the FK target (0.0 - 1.0).
	FKConfidence float64 `json:"fk_confidence,omitempty"`

	// EntityReferenced describes what entity this identifier refers to.
	EntityReferenced string `json:"entity_referenced,omitempty"`
}

// Identifier type constants.
const (
	IdentifierTypeInternalUUID     = "internal_uuid"
	IdentifierTypeExternalUUID     = "external_uuid"
	IdentifierTypePrimaryKey       = "primary_key"
	IdentifierTypeForeignKey       = "foreign_key"
	IdentifierTypeExternalService  = "external_service_id"
)

// External service constants.
const (
	ExternalServiceStripe = "stripe"
	ExternalServiceTwilio = "twilio"
	ExternalServiceAWSSES = "aws_ses"
)

// ============================================================================
// Monetary Features
// ============================================================================

// MonetaryFeatures holds classification results for monetary/currency columns.
type MonetaryFeatures struct {
	// IsMonetary indicates if this column represents a monetary amount.
	IsMonetary bool `json:"is_monetary"`

	// CurrencyUnit describes the unit of the amount.
	// Values: "cents", "dollars", "basis_points", or the actual currency code (e.g., "USD").
	CurrencyUnit string `json:"currency_unit"`

	// PairedCurrencyColumn is the name of the column containing the currency code.
	// Empty if the currency is implicit or not found.
	PairedCurrencyColumn string `json:"paired_currency_column,omitempty"`

	// AmountDescription describes what this amount represents.
	AmountDescription string `json:"amount_description,omitempty"`
}

// Currency unit constants.
const (
	CurrencyUnitCents       = "cents"
	CurrencyUnitDollars     = "dollars"
	CurrencyUnitBasisPoints = "basis_points"
)

// ============================================================================
// Feature Extraction Progress
// ============================================================================

// FeatureExtractionProgress tracks progress across all phases of column feature extraction.
// The UI uses this to display phase-specific progress bars with known counts.
type FeatureExtractionProgress struct {
	// CurrentPhase identifies which phase is currently executing.
	// Values: "phase1", "phase2", "phase3", "phase4", "phase5", "phase6"
	CurrentPhase string `json:"current_phase"`

	// PhaseDescription is a human-readable description of the current phase.
	PhaseDescription string `json:"phase_description"`

	// Phase-specific counts (known at phase start for deterministic enumeration)
	TotalItems     int `json:"total_items"`
	CompletedItems int `json:"completed_items"`

	// Summary of work discovered during Phase 1
	TotalColumns          int `json:"total_columns"`
	EnumCandidates        int `json:"enum_candidates"`
	FKCandidates          int `json:"fk_candidates"`
	CrossColumnCandidates int `json:"cross_column_candidates"`

	// Detailed phase progress (for multi-phase UI display)
	Phases []PhaseProgress `json:"phases,omitempty"`
}

// PhaseProgress tracks the progress of an individual phase.
type PhaseProgress struct {
	// PhaseID uniquely identifies this phase.
	PhaseID string `json:"phase_id"`

	// PhaseName is the display name for this phase.
	PhaseName string `json:"phase_name"`

	// Status indicates the phase execution state.
	// Values: "pending", "in_progress", "complete", "failed"
	Status string `json:"status"`

	// TotalItems is the number of items to process in this phase (known at phase start).
	TotalItems int `json:"total_items,omitempty"`

	// CompletedItems is the number of items completed so far.
	CompletedItems int `json:"completed_items,omitempty"`

	// CurrentItem describes what's currently being processed.
	CurrentItem string `json:"current_item,omitempty"`
}

// Phase status constants.
const (
	PhaseStatusPending    = "pending"
	PhaseStatusInProgress = "in_progress"
	PhaseStatusComplete   = "complete"
	PhaseStatusFailed     = "failed"
)

// Phase ID constants.
const (
	PhaseIDDataCollection      = "phase1"
	PhaseIDColumnClassification = "phase2"
	PhaseIDEnumAnalysis        = "phase3"
	PhaseIDFKResolution        = "phase4"
	PhaseIDCrossColumnAnalysis = "phase5"
	PhaseIDStoreResults        = "phase6"
)

// Percentage returns the completion percentage for the current phase (0-100).
func (p *FeatureExtractionProgress) Percentage() int {
	if p == nil || p.TotalItems == 0 {
		return 0
	}
	return int(float64(p.CompletedItems) / float64(p.TotalItems) * 100)
}

// NewFeatureExtractionProgress creates a new progress tracker with default phases.
func NewFeatureExtractionProgress() *FeatureExtractionProgress {
	return &FeatureExtractionProgress{
		CurrentPhase:     PhaseIDDataCollection,
		PhaseDescription: "Initializing...",
		Phases: []PhaseProgress{
			{PhaseID: PhaseIDDataCollection, PhaseName: "Collecting column metadata", Status: PhaseStatusPending},
			{PhaseID: PhaseIDColumnClassification, PhaseName: "Classifying columns", Status: PhaseStatusPending},
			{PhaseID: PhaseIDEnumAnalysis, PhaseName: "Analyzing enum values", Status: PhaseStatusPending},
			{PhaseID: PhaseIDFKResolution, PhaseName: "Resolving FK candidates", Status: PhaseStatusPending},
			{PhaseID: PhaseIDCrossColumnAnalysis, PhaseName: "Cross-column analysis", Status: PhaseStatusPending},
			{PhaseID: PhaseIDStoreResults, PhaseName: "Saving results", Status: PhaseStatusPending},
		},
	}
}

// SetPhaseStatus updates the status of a specific phase.
func (p *FeatureExtractionProgress) SetPhaseStatus(phaseID string, status string) {
	for i := range p.Phases {
		if p.Phases[i].PhaseID == phaseID {
			p.Phases[i].Status = status
			return
		}
	}
}

// SetPhaseProgress updates the progress of a specific phase.
func (p *FeatureExtractionProgress) SetPhaseProgress(phaseID string, completed, total int, currentItem string) {
	for i := range p.Phases {
		if p.Phases[i].PhaseID == phaseID {
			p.Phases[i].CompletedItems = completed
			p.Phases[i].TotalItems = total
			p.Phases[i].CurrentItem = currentItem
			return
		}
	}
}
