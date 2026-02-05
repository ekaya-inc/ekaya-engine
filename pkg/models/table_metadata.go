package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// TableType constants for table classification.
const (
	TableTypeTransactional = "transactional" // Event/action tables with timestamps
	TableTypeReference     = "reference"     // Static lookup tables (countries, status codes)
	TableTypeLogging       = "logging"       // Audit/history tables, append-only
	TableTypeEphemeral     = "ephemeral"     // Temporary/session data
	TableTypeJunction      = "junction"      // Many-to-many relationship tables
)

// TableMetadata represents semantic annotations for a specific table.
// Stored in engine_ontology_table_metadata table with provenance tracking.
// Links to engine_schema_tables via SchemaTableID instead of datasource_id/table_name.
type TableMetadata struct {
	ID            uuid.UUID `json:"id"`
	ProjectID     uuid.UUID `json:"project_id"`
	SchemaTableID uuid.UUID `json:"schema_table_id"` // FK to engine_schema_tables

	// Core classification (typed columns from LLM analysis)
	TableType            *string  `json:"table_type,omitempty"`            // transactional, reference, logging, ephemeral, junction
	Description          *string  `json:"description,omitempty"`           // What this table represents
	UsageNotes           *string  `json:"usage_notes,omitempty"`           // When to use/not use this table
	IsEphemeral          bool     `json:"is_ephemeral"`                    // Whether it's transient/temp data
	PreferredAlternative *string  `json:"preferred_alternative,omitempty"` // Table to use instead if ephemeral
	Confidence           *float64 `json:"confidence,omitempty"`            // Classification confidence (0.0 - 1.0)

	// Type-specific features (single JSONB for extensibility)
	Features TableMetadataFeatures `json:"features"`

	// Analysis metadata
	AnalyzedAt   *time.Time `json:"analyzed_at,omitempty"`
	LLMModelUsed *string    `json:"llm_model_used,omitempty"`

	// Provenance: source tracking (how it was created/modified)
	Source         string  `json:"source"`                     // 'inferred', 'mcp', 'manual'
	LastEditSource *string `json:"last_edit_source,omitempty"` // How last modified (nil if never edited)

	// Provenance: actor tracking (who created/modified)
	CreatedBy *uuid.UUID `json:"created_by,omitempty"` // User who triggered creation (from JWT)
	UpdatedBy *uuid.UUID `json:"updated_by,omitempty"` // User who last updated (nil if never updated)

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// TableMetadataFeatures holds type-specific features as JSONB.
// This structure supports relationship, temporal, and size features for tables.
type TableMetadataFeatures struct {
	RelationshipSummary *RelationshipSummaryFeatures `json:"relationship_summary,omitempty"`
	TemporalFeatures    *TableTemporalFeatures       `json:"temporal_features,omitempty"`
	SizeFeatures        *TableSizeFeatures           `json:"size_features,omitempty"`
}

// RelationshipSummaryFeatures captures FK relationship statistics for a table.
type RelationshipSummaryFeatures struct {
	IncomingFKCount int `json:"incoming_fk_count"` // Number of FKs pointing to this table
	OutgoingFKCount int `json:"outgoing_fk_count"` // Number of FKs from this table
}

// TableTemporalFeatures captures time-related patterns for a table.
type TableTemporalFeatures struct {
	HasSoftDelete      bool `json:"has_soft_delete"`      // Has deleted_at or similar
	HasAuditTimestamps bool `json:"has_audit_timestamps"` // Has created_at/updated_at
}

// TableSizeFeatures captures size and growth patterns for a table.
type TableSizeFeatures struct {
	IsLargeTable  bool   `json:"is_large_table"` // High row count
	GrowthPattern string `json:"growth_pattern"` // append_only, update_heavy, mixed
}

// Scan implements sql.Scanner for reading JSONB from database.
func (f *TableMetadataFeatures) Scan(value interface{}) error {
	if value == nil {
		*f = TableMetadataFeatures{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		*f = TableMetadataFeatures{}
		return nil
	}

	return json.Unmarshal(bytes, f)
}

// Value implements driver.Valuer for writing JSONB to database.
func (f TableMetadataFeatures) Value() (interface{}, error) {
	return json.Marshal(f)
}

// GetRelationshipSummary returns relationship summary features, or nil if not available.
func (m *TableMetadata) GetRelationshipSummary() *RelationshipSummaryFeatures {
	return m.Features.RelationshipSummary
}

// GetTemporalFeatures returns temporal features, or nil if not available.
func (m *TableMetadata) GetTemporalFeatures() *TableTemporalFeatures {
	return m.Features.TemporalFeatures
}

// GetSizeFeatures returns size features, or nil if not available.
func (m *TableMetadata) GetSizeFeatures() *TableSizeFeatures {
	return m.Features.SizeFeatures
}

// SetFromAnalysis populates TableMetadata fields from table analysis results.
// This is used by the extraction pipeline to convert analysis results into
// the format stored in engine_ontology_table_metadata.
func (m *TableMetadata) SetFromAnalysis(
	tableType string,
	description string,
	usageNotes string,
	isEphemeral bool,
	confidence float64,
	analyzedAt time.Time,
	llmModel string,
) {
	if tableType != "" {
		m.TableType = &tableType
	}
	if description != "" {
		m.Description = &description
	}
	if usageNotes != "" {
		m.UsageNotes = &usageNotes
	}
	m.IsEphemeral = isEphemeral
	if confidence > 0 {
		m.Confidence = &confidence
	}
	if !analyzedAt.IsZero() {
		m.AnalyzedAt = &analyzedAt
	}
	if llmModel != "" {
		m.LLMModelUsed = &llmModel
	}
}

// SetRelationshipSummary sets the relationship summary features.
func (m *TableMetadata) SetRelationshipSummary(incomingFKs, outgoingFKs int) {
	m.Features.RelationshipSummary = &RelationshipSummaryFeatures{
		IncomingFKCount: incomingFKs,
		OutgoingFKCount: outgoingFKs,
	}
}

// SetTemporalFeatures sets the temporal features.
func (m *TableMetadata) SetTemporalFeatures(hasSoftDelete, hasAuditTimestamps bool) {
	m.Features.TemporalFeatures = &TableTemporalFeatures{
		HasSoftDelete:      hasSoftDelete,
		HasAuditTimestamps: hasAuditTimestamps,
	}
}

// SetSizeFeatures sets the size features.
func (m *TableMetadata) SetSizeFeatures(isLargeTable bool, growthPattern string) {
	m.Features.SizeFeatures = &TableSizeFeatures{
		IsLargeTable:  isLargeTable,
		GrowthPattern: growthPattern,
	}
}
