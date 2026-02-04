package models

import (
	"time"

	"github.com/google/uuid"
)

// SchemaTable represents a discovered database table from a datasource.
type SchemaTable struct {
	ID           uuid.UUID      `json:"id"`
	ProjectID    uuid.UUID      `json:"project_id"`
	DatasourceID uuid.UUID      `json:"datasource_id"`
	SchemaName   string         `json:"schema_name"`
	TableName    string         `json:"table_name"`
	IsSelected   bool           `json:"is_selected"`
	RowCount     *int64         `json:"row_count,omitempty"`
	BusinessName *string        `json:"business_name,omitempty"`
	Description  *string        `json:"description,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Columns      []SchemaColumn `json:"columns,omitempty"` // populated on demand
}

// SchemaColumn represents a table column with statistics.
type SchemaColumn struct {
	ID              uuid.UUID `json:"id"`
	ProjectID       uuid.UUID `json:"project_id"`
	SchemaTableID   uuid.UUID `json:"schema_table_id"`
	ColumnName      string    `json:"column_name"`
	DataType        string    `json:"data_type"`
	IsNullable      bool      `json:"is_nullable"`
	IsPrimaryKey    bool      `json:"is_primary_key"`
	IsUnique        bool      `json:"is_unique"`
	IsSelected      bool      `json:"is_selected"`
	OrdinalPosition int       `json:"ordinal_position"`
	DefaultValue    *string   `json:"default_value,omitempty"`
	DistinctCount   *int64    `json:"distinct_count,omitempty"`
	NullCount       *int64    `json:"null_count,omitempty"`
	MinLength       *int64    `json:"min_length,omitempty"` // For text columns: min string length
	MaxLength       *int64    `json:"max_length,omitempty"` // For text columns: max string length
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	// Discovery-related fields (populated by discovery process)
	RowCount          *int64     `json:"row_count,omitempty"`          // Denormalized table row count
	NonNullCount      *int64     `json:"non_null_count,omitempty"`     // Non-null values in column
	IsJoinable        *bool      `json:"is_joinable,omitempty"`        // Can be used as join key
	JoinabilityReason *string    `json:"joinability_reason,omitempty"` // Why column is/isn't joinable
	StatsUpdatedAt    *time.Time `json:"stats_updated_at,omitempty"`   // When stats were computed
}

// SchemaRelationship represents a relationship between two columns.
type SchemaRelationship struct {
	ID                uuid.UUID          `json:"id"`
	ProjectID         uuid.UUID          `json:"project_id"`
	SourceTableID     uuid.UUID          `json:"source_table_id"`
	SourceColumnID    uuid.UUID          `json:"source_column_id"`
	TargetTableID     uuid.UUID          `json:"target_table_id"`
	TargetColumnID    uuid.UUID          `json:"target_column_id"`
	RelationshipType  string             `json:"relationship_type"`
	Cardinality       string             `json:"cardinality"`
	Confidence        float64            `json:"confidence"`
	InferenceMethod   *string            `json:"inference_method,omitempty"`
	IsValidated       bool               `json:"is_validated"`
	ValidationResults *ValidationResults `json:"validation_results,omitempty"`
	IsApproved        *bool              `json:"is_approved,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	// Discovery metrics (populated during relationship discovery)
	MatchRate       *float64 `json:"match_rate,omitempty"`       // Value overlap 0.0-1.0
	SourceDistinct  *int64   `json:"source_distinct,omitempty"`  // Distinct values in source
	TargetDistinct  *int64   `json:"target_distinct,omitempty"`  // Distinct values in target
	MatchedCount    *int64   `json:"matched_count,omitempty"`    // Count of matched values
	RejectionReason *string  `json:"rejection_reason,omitempty"` // Why candidate was rejected
}

// ValidationResults stores metrics from relationship validation analysis.
type ValidationResults struct {
	// Row counts
	SourceRowCount int64 `json:"source_row_count"`
	TargetRowCount int64 `json:"target_row_count"`
	JoinedRowCount int64 `json:"joined_row_count"`

	// Orphan analysis
	OrphanCount int64   `json:"orphan_count"`
	OrphanRate  float64 `json:"orphan_rate"`

	// Match analysis
	MatchRate    float64 `json:"match_rate"`
	MatchQuality string  `json:"match_quality"` // "perfect", "strong", "moderate"

	// Source column profile
	SourceDistinct int64   `json:"source_distinct"`
	SourceNullRate float64 `json:"source_null_rate"`

	// Target column profile
	TargetDistinct int64   `json:"target_distinct"`
	TargetIsUnique bool    `json:"target_is_unique"`
	TargetCoverage float64 `json:"target_coverage"`

	// Naming pattern
	NamingPatternMatched bool `json:"naming_pattern_matched"`
}

// Relationship types
const (
	RelationshipTypeFK       = "fk"
	RelationshipTypeInferred = "inferred"
	RelationshipTypeManual   = "manual"
	RelationshipTypeReview   = "review" // Pending LLM review for orphan tables
)

// ValidRelationshipTypes contains all valid relationship type values.
var ValidRelationshipTypes = []string{
	RelationshipTypeFK,
	RelationshipTypeInferred,
	RelationshipTypeManual,
	RelationshipTypeReview,
}

// IsValidRelationshipType checks if the given type is valid.
func IsValidRelationshipType(t string) bool {
	for _, v := range ValidRelationshipTypes {
		if v == t {
			return true
		}
	}
	return false
}

// Cardinality types
const (
	Cardinality1To1    = "1:1"
	Cardinality1ToN    = "1:N"
	CardinalityNTo1    = "N:1"
	CardinalityNToM    = "N:M"
	CardinalityUnknown = "unknown"
)

// ValidCardinalities contains all valid cardinality values.
var ValidCardinalities = []string{
	Cardinality1To1,
	Cardinality1ToN,
	CardinalityNTo1,
	CardinalityNToM,
	CardinalityUnknown,
}

// IsValidCardinality checks if the given cardinality is valid.
func IsValidCardinality(c string) bool {
	for _, v := range ValidCardinalities {
		if v == c {
			return true
		}
	}
	return false
}

// Match quality tiers
const (
	MatchQualityPerfect  = "perfect"
	MatchQualityStrong   = "strong"
	MatchQualityModerate = "moderate"
)

// Inference methods for discovered relationships.
// Some methods are reserved for future inference algorithms.
const (
	InferenceMethodNamingPattern  = "naming_pattern"  // Reserved: column name pattern matching (e.g., user_id -> users.id)
	InferenceMethodValueOverlap   = "value_overlap"   // Active: statistical value overlap analysis
	InferenceMethodTypeMatch      = "type_match"      // Reserved: type-compatible column matching
	InferenceMethodForeignKey     = "foreign_key"     // Active: imported from database FK constraints
	InferenceMethodColumnFeatures = "column_features" // Active: FK derived from ColumnFeatureExtraction Phase 4
	InferenceMethodPKMatch        = "pk_match"        // Active: FK inferred from PK-match discovery
)

// Rejection reasons for relationship candidates
const (
	RejectionLowMatchRate      = "low_match_rate"
	RejectionHighOrphanRate    = "high_orphan_rate"
	RejectionJoinFailed        = "join_failed"
	RejectionTypeMismatch      = "type_mismatch"
	RejectionAlreadyExists     = "already_exists"
	RejectionWrongDirection    = "wrong_direction"     // Source has more distinct values than target (reversed FK)
	RejectionOrphanIntegrity   = "orphan_integrity"    // More than 5% orphan values (FK integrity violation)
	RejectionReverseOrphanHigh = "reverse_orphan_high" // >50% of target values don't exist in source (coincidental overlap)
)

// Joinability classification reasons
const (
	JoinabilityPK             = "pk"
	JoinabilityFKTarget       = "fk_target"
	JoinabilityUniqueValues   = "unique_values"
	JoinabilityTypeExcluded   = "type_excluded"
	JoinabilityLowCardinality = "low_cardinality"
	JoinabilityNoStats        = "no_stats"
	JoinabilityCardinalityOK  = "cardinality_ok"
)

// ============================================================================
// Service Layer Types
// ============================================================================

// RefreshResult contains statistics from a schema refresh operation.
type RefreshResult struct {
	TablesUpserted       int      `json:"tables_upserted"`
	TablesDeleted        int64    `json:"tables_deleted"`
	ColumnsUpserted      int      `json:"columns_upserted"`
	ColumnsDeleted       int64    `json:"columns_deleted"`
	RelationshipsCreated int      `json:"relationships_created"`
	RelationshipsDeleted int64    `json:"relationships_deleted"`
	NewTableNames        []string `json:"new_table_names,omitempty"`
	RemovedTableNames    []string `json:"removed_table_names,omitempty"`
	// Detailed column changes for change detection (PLAN-03)
	NewColumns      []RefreshColumnChange       `json:"new_columns,omitempty"`
	RemovedColumns  []RefreshColumnChange       `json:"removed_columns,omitempty"`
	ModifiedColumns []RefreshColumnModification `json:"modified_columns,omitempty"`
}

// RefreshColumnChange represents a column that was added or removed during refresh.
type RefreshColumnChange struct {
	TableName  string `json:"table_name"`
	ColumnName string `json:"column_name"`
	DataType   string `json:"data_type"`
}

// RefreshColumnModification represents a column whose type changed during refresh.
type RefreshColumnModification struct {
	TableName  string `json:"table_name"`
	ColumnName string `json:"column_name"`
	OldType    string `json:"old_type"`
	NewType    string `json:"new_type"`
}

// DatasourceSchema represents the complete schema for a customer's datasource.
type DatasourceSchema struct {
	ProjectID     uuid.UUID
	DatasourceID  uuid.UUID
	Tables        []*DatasourceTable
	Relationships []*DatasourceRelationship
}

// DatasourceTable represents a table in the customer's datasource.
type DatasourceTable struct {
	ID           uuid.UUID
	SchemaName   string
	TableName    string
	BusinessName string
	Description  string
	RowCount     int64
	IsSelected   bool
	Columns      []*DatasourceColumn
}

// DatasourceColumn represents a column in the customer's datasource.
// Note: business_name and description are now in engine_ontology_column_metadata, not engine_schema_columns.
type DatasourceColumn struct {
	ID              uuid.UUID
	ColumnName      string
	DataType        string
	IsNullable      bool
	IsPrimaryKey    bool
	IsUnique        bool
	IsSelected      bool
	OrdinalPosition int
	DefaultValue    *string
	DistinctCount   *int64
	NullCount       *int64
}

// DatasourceRelationship represents a relationship in the customer's datasource.
type DatasourceRelationship struct {
	ID               uuid.UUID
	SourceTableID    uuid.UUID
	SourceTableName  string
	SourceColumnID   uuid.UUID
	SourceColumnName string
	TargetTableID    uuid.UUID
	TargetTableName  string
	TargetColumnID   uuid.UUID
	TargetColumnName string
	RelationshipType string
	Cardinality      string
	Confidence       float64
	IsApproved       *bool
}

// AddRelationshipRequest contains input for creating a manual relationship.
type AddRelationshipRequest struct {
	SourceTableName  string `json:"source_table"`
	SourceColumnName string `json:"source_column"`
	TargetTableName  string `json:"target_table"`
	TargetColumnName string `json:"target_column"`
}

// ============================================================================
// Relationship Discovery Types
// ============================================================================

// RelationshipDetail provides enriched relationship data with resolved names and types.
type RelationshipDetail struct {
	ID               uuid.UUID `json:"id"`
	SourceTableName  string    `json:"source_table_name"`
	SourceColumnName string    `json:"source_column_name"`
	SourceColumnType string    `json:"source_column_type"`
	TargetTableName  string    `json:"target_table_name"`
	TargetColumnName string    `json:"target_column_name"`
	TargetColumnType string    `json:"target_column_type"`
	RelationshipType string    `json:"relationship_type"`
	Cardinality      string    `json:"cardinality"`
	Confidence       float64   `json:"confidence"`
	InferenceMethod  *string   `json:"inference_method,omitempty"`
	IsValidated      bool      `json:"is_validated"`
	IsApproved       *bool     `json:"is_approved,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RelationshipsResponse contains the full response for GET /relationships endpoint.
type RelationshipsResponse struct {
	Relationships []*RelationshipDetail `json:"relationships"`
	TotalCount    int                   `json:"total_count"`
	EmptyTables   []string              `json:"empty_tables,omitempty"`  // Tables with 0 rows
	OrphanTables  []string              `json:"orphan_tables,omitempty"` // Tables with data but no relationships
}

// DiscoveryResults contains statistics from a relationship discovery operation.
type DiscoveryResults struct {
	RelationshipsCreated       int      `json:"relationships_created"`
	ReviewCandidatesCreated    int      `json:"review_candidates_created"` // Review candidates for orphan tables
	TablesAnalyzed             int      `json:"tables_analyzed"`
	ColumnsAnalyzed            int      `json:"columns_analyzed"`
	TablesWithoutRelationships int      `json:"tables_without_relationships"`
	EmptyTables                int      `json:"empty_tables"`
	EmptyTableNames            []string `json:"empty_table_names,omitempty"`
	OrphanTableNames           []string `json:"orphan_table_names,omitempty"`
}

// DiscoveryMetrics captures metrics during relationship discovery for storage.
type DiscoveryMetrics struct {
	MatchRate      float64
	SourceDistinct int64
	TargetDistinct int64
	MatchedCount   int64
}
