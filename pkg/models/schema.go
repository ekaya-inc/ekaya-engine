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
	ID              uuid.UUID      `json:"id"`
	ProjectID       uuid.UUID      `json:"project_id"`
	SchemaTableID   uuid.UUID      `json:"schema_table_id"`
	ColumnName      string         `json:"column_name"`
	DataType        string         `json:"data_type"`
	IsNullable      bool           `json:"is_nullable"`
	IsPrimaryKey    bool           `json:"is_primary_key"`
	IsSelected      bool           `json:"is_selected"`
	OrdinalPosition int            `json:"ordinal_position"`
	DistinctCount   *int64         `json:"distinct_count,omitempty"`
	NullCount       *int64         `json:"null_count,omitempty"`
	BusinessName    *string        `json:"business_name,omitempty"`
	Description     *string        `json:"description,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
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
)

// ValidRelationshipTypes contains all valid relationship type values.
var ValidRelationshipTypes = []string{
	RelationshipTypeFK,
	RelationshipTypeInferred,
	RelationshipTypeManual,
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

// ============================================================================
// Service Layer Types
// ============================================================================

// RefreshResult contains statistics from a schema refresh operation.
type RefreshResult struct {
	TablesUpserted       int   `json:"tables_upserted"`
	TablesDeleted        int64 `json:"tables_deleted"`
	ColumnsUpserted      int   `json:"columns_upserted"`
	ColumnsDeleted       int64 `json:"columns_deleted"`
	RelationshipsCreated int   `json:"relationships_created"`
	RelationshipsDeleted int64 `json:"relationships_deleted"`
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
type DatasourceColumn struct {
	ID              uuid.UUID
	ColumnName      string
	DataType        string
	IsNullable      bool
	IsPrimaryKey    bool
	IsSelected      bool
	OrdinalPosition int
	BusinessName    string
	Description     string
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
