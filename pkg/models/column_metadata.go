package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ColumnMetadata represents semantic annotations for a specific column.
// Stored in engine_ontology_column_metadata table with provenance tracking.
// Links to engine_schema_columns via SchemaColumnID instead of table_name/column_name.
type ColumnMetadata struct {
	ID             uuid.UUID `json:"id"`
	ProjectID      uuid.UUID `json:"project_id"`
	SchemaColumnID uuid.UUID `json:"schema_column_id"` // FK to engine_schema_columns

	// Core classification (typed columns from LLM analysis)
	ClassificationPath *string  `json:"classification_path,omitempty"` // timestamp, boolean, enum, uuid, external_id, numeric, text, json, unknown
	Purpose            *string  `json:"purpose,omitempty"`             // identifier, timestamp, flag, measure, enum, text, json
	SemanticType       *string  `json:"semantic_type,omitempty"`       // soft_delete_timestamp, currency_cents, etc.
	Role               *string  `json:"role,omitempty"`                // primary_key, foreign_key, attribute, measure, dimension, identifier
	Description        *string  `json:"description,omitempty"`         // LLM-generated business description
	Confidence         *float64 `json:"confidence,omitempty"`          // Classification confidence (0.0 - 1.0)

	// Type-specific features (single JSONB for extensibility)
	Features ColumnMetadataFeatures `json:"features"`

	// Processing flags (for deferred analysis)
	NeedsEnumAnalysis     bool    `json:"needs_enum_analysis"`
	NeedsFKResolution     bool    `json:"needs_fk_resolution"`
	NeedsCrossColumnCheck bool    `json:"needs_cross_column_check"`
	NeedsClarification    bool    `json:"needs_clarification"`
	ClarificationQuestion *string `json:"clarification_question,omitempty"`

	// User overrides
	IsSensitive *bool `json:"is_sensitive,omitempty"` // Sensitive data override: nil=auto-detect, true=always, false=never

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

// ColumnMetadataFeatures holds type-specific features as JSONB.
// This structure supports timestamp, boolean, enum, identifier, and monetary features.
type ColumnMetadataFeatures struct {
	TimestampFeatures  *TimestampFeatures  `json:"timestamp_features,omitempty"`
	BooleanFeatures    *BooleanFeatures    `json:"boolean_features,omitempty"`
	EnumFeatures       *EnumFeatures       `json:"enum_features,omitempty"`
	IdentifierFeatures *IdentifierFeatures `json:"identifier_features,omitempty"`
	MonetaryFeatures   *MonetaryFeatures   `json:"monetary_features,omitempty"`

	// Cross-cutting features (not tied to a specific classification path)
	Synonyms []string `json:"synonyms,omitempty"` // Alternative names for this column (e.g., "revenue", "sales")
}

// Scan implements sql.Scanner for reading JSONB from database.
func (f *ColumnMetadataFeatures) Scan(value interface{}) error {
	if value == nil {
		*f = ColumnMetadataFeatures{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		*f = ColumnMetadataFeatures{}
		return nil
	}

	return json.Unmarshal(bytes, f)
}

// Value implements driver.Valuer for writing JSONB to database.
func (f ColumnMetadataFeatures) Value() (interface{}, error) {
	return json.Marshal(f)
}

// GetTimestampFeatures returns timestamp-specific features, or nil if not available.
func (m *ColumnMetadata) GetTimestampFeatures() *TimestampFeatures {
	return m.Features.TimestampFeatures
}

// GetBooleanFeatures returns boolean-specific features, or nil if not available.
func (m *ColumnMetadata) GetBooleanFeatures() *BooleanFeatures {
	return m.Features.BooleanFeatures
}

// GetEnumFeatures returns enum-specific features, or nil if not available.
func (m *ColumnMetadata) GetEnumFeatures() *EnumFeatures {
	return m.Features.EnumFeatures
}

// GetIdentifierFeatures returns identifier-specific features, or nil if not available.
func (m *ColumnMetadata) GetIdentifierFeatures() *IdentifierFeatures {
	return m.Features.IdentifierFeatures
}

// GetMonetaryFeatures returns monetary-specific features, or nil if not available.
func (m *ColumnMetadata) GetMonetaryFeatures() *MonetaryFeatures {
	return m.Features.MonetaryFeatures
}

// SetFeatures populates ColumnMetadata fields from a ColumnFeatures struct.
// This is used by the extraction pipeline to convert analysis results into
// the format stored in engine_ontology_column_metadata.
func (m *ColumnMetadata) SetFeatures(features *ColumnFeatures) {
	if features == nil {
		return
	}

	// Set core classification fields
	if features.ClassificationPath != "" {
		path := string(features.ClassificationPath)
		m.ClassificationPath = &path
	}
	if features.Purpose != "" {
		m.Purpose = &features.Purpose
	}
	if features.SemanticType != "" {
		m.SemanticType = &features.SemanticType
	}
	if features.Role != "" {
		m.Role = &features.Role
	}
	if features.Description != "" {
		m.Description = &features.Description
	}
	if features.Confidence > 0 {
		m.Confidence = &features.Confidence
	}

	// Copy type-specific features
	m.Features.TimestampFeatures = features.TimestampFeatures
	m.Features.BooleanFeatures = features.BooleanFeatures
	m.Features.EnumFeatures = features.EnumFeatures
	m.Features.IdentifierFeatures = features.IdentifierFeatures
	m.Features.MonetaryFeatures = features.MonetaryFeatures

	// Copy processing flags
	m.NeedsEnumAnalysis = features.NeedsEnumAnalysis
	m.NeedsFKResolution = features.NeedsFKResolution
	m.NeedsCrossColumnCheck = features.NeedsCrossColumnCheck
	m.NeedsClarification = features.NeedsClarification
	if features.ClarificationQuestion != "" {
		m.ClarificationQuestion = &features.ClarificationQuestion
	}

	// Copy analysis metadata
	if !features.AnalyzedAt.IsZero() {
		m.AnalyzedAt = &features.AnalyzedAt
	}
	if features.LLMModelUsed != "" {
		m.LLMModelUsed = &features.LLMModelUsed
	}
}
