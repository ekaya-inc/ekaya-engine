package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/jsonutil"
)

// ============================================================================
// Tiered Ontology - Domain Layer (Tier 0)
// ============================================================================

// DomainSummary represents the top-level business context (~500 tokens).
// Provides high-level understanding without detailed column information.
type DomainSummary struct {
	Description       string              `json:"description"`
	Domains           []string            `json:"domains"`
	Conventions       *ProjectConventions `json:"conventions,omitempty"`
	RelationshipGraph []RelationshipEdge  `json:"relationship_graph,omitempty"`
	SampleQuestions   []string            `json:"sample_questions,omitempty"`
}

// RelationshipEdge represents a connection between entities in the domain graph.
type RelationshipEdge struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Label       string `json:"label,omitempty"`
	Cardinality string `json:"cardinality,omitempty"`
}

// ProjectConventions captures database-wide patterns that affect all queries.
// Only patterns appearing in >50% of tables are reported as conventions.
type ProjectConventions struct {
	SoftDelete   *SoftDeleteConvention `json:"soft_delete,omitempty"`
	Currency     *CurrencyConvention   `json:"currency,omitempty"`
	AuditColumns []AuditColumnInfo     `json:"audit_columns,omitempty"`
}

// SoftDeleteConvention describes how soft-deleted records are identified.
type SoftDeleteConvention struct {
	Enabled    bool    `json:"enabled"`
	Column     string  `json:"column"`
	ColumnType string  `json:"column_type"` // "timestamp" or "boolean"
	Filter     string  `json:"filter"`      // SQL fragment: "deleted_at IS NULL" or "is_deleted = false"
	Coverage   float64 `json:"coverage"`    // 0.0-1.0, percentage of tables with this column
}

// CurrencyConvention describes how monetary values are stored.
type CurrencyConvention struct {
	DefaultCurrency string   `json:"default_currency"` // e.g., "USD"
	Format          string   `json:"format"`           // "cents" or "dollars"
	ColumnPatterns  []string `json:"column_patterns"`  // e.g., ["*_amount", "*_price", "*_cost"]
	Transform       string   `json:"transform"`        // "divide_by_100" or "none"
}

// AuditColumnInfo describes an audit column and how widely it's used.
type AuditColumnInfo struct {
	Column   string  `json:"column"`
	Coverage float64 `json:"coverage"` // 0.0-1.0, percentage of tables with this column
}

// ============================================================================
// Tiered Ontology - Entity Layer (Tier 1)
// ============================================================================

// EntitySummary represents a per-table business summary (~75 tokens each).
type EntitySummary struct {
	TableName     string      `json:"table_name"`
	BusinessName  string      `json:"business_name"`
	Description   string      `json:"description"`
	Domain        string      `json:"domain"`
	Synonyms      []string    `json:"synonyms,omitempty"`
	KeyColumns    []KeyColumn `json:"key_columns,omitempty"`
	ColumnCount   int         `json:"column_count"`
	Relationships []string    `json:"relationships,omitempty"`
}

// KeyColumn represents an important column in an entity summary.
type KeyColumn struct {
	Name     string   `json:"name"`
	Synonyms []string `json:"synonyms,omitempty"`
}

// ============================================================================
// Tiered Ontology - Column Layer (Tier 2)
// ============================================================================

// ColumnDetail represents detailed semantic information for a column.
type ColumnDetail struct {
	Name          string      `json:"name"`
	Description   string      `json:"description,omitempty"`
	Synonyms      []string    `json:"synonyms,omitempty"`
	SemanticType  string      `json:"semantic_type,omitempty"`
	Role          string      `json:"role,omitempty"`           // dimension, measure, identifier, attribute
	FKAssociation string      `json:"fk_association,omitempty"` // payer, payee, host, visitor, etc.
	EnumValues    []EnumValue `json:"enum_values,omitempty"`
	IsPrimaryKey  bool        `json:"is_primary_key"`
	IsForeignKey  bool        `json:"is_foreign_key"`
	ForeignTable  string      `json:"foreign_table,omitempty"`
}

// EnumValue represents a known value in an enumeration column.
type EnumValue struct {
	Value       string `json:"value"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`

	// Distribution metadata (populated by enum distribution analysis)
	Count      *int64   `json:"count,omitempty"`      // Number of records with this value
	Percentage *float64 `json:"percentage,omitempty"` // Percentage of total records (0.0-100.0)

	// State semantics (populated when state column detected with completion timestamp)
	IsLikelyInitialState  *bool `json:"is_likely_initial_state,omitempty"`  // High count, low completion rate
	IsLikelyTerminalState *bool `json:"is_likely_terminal_state,omitempty"` // High completion rate (~100%)
	IsLikelyErrorState    *bool `json:"is_likely_error_state,omitempty"`    // Low count relative to others
}

// UnmarshalJSON handles LLM responses that may return enum values in different formats:
// 1. Plain string: "AUD" → {value: "AUD"}
// 2. Plain number: 42 → {value: "42"}
// 3. Object with value as string: {value: "AUD", label: "Australian Dollar"}
// 4. Object with value as number: {value: 2, label: "Type 2"}
func (e *EnumValue) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal as a plain string (e.g., "AUD")
	var plainString string
	if err := json.Unmarshal(data, &plainString); err == nil {
		e.Value = plainString
		e.Label = ""
		e.Description = ""
		return nil
	}

	// Try to unmarshal as a plain number (e.g., 42)
	var plainNumber json.Number
	if err := json.Unmarshal(data, &plainNumber); err == nil {
		e.Value = plainNumber.String()
		e.Label = ""
		e.Description = ""
		return nil
	}

	// Otherwise, unmarshal as object with flexible value type
	type flexibleEnumValue struct {
		Value       json.RawMessage `json:"value"`
		Label       string          `json:"label,omitempty"`
		Description string          `json:"description,omitempty"`
	}

	var flex flexibleEnumValue
	if err := json.Unmarshal(data, &flex); err != nil {
		return err
	}

	e.Label = flex.Label
	e.Description = flex.Description
	e.Value = jsonutil.FlexibleStringValue(flex.Value)
	return nil
}

// Column roles
const (
	ColumnRoleDimension  = "dimension"
	ColumnRoleMeasure    = "measure"
	ColumnRoleIdentifier = "identifier"
	ColumnRoleAttribute  = "attribute"
)

// Business domains for classification
const (
	DomainSales      = "sales"
	DomainFinance    = "finance"
	DomainOperations = "operations"
	DomainCustomer   = "customer"
	DomainProduct    = "product"
	DomainAnalytics  = "analytics"
	DomainHR         = "hr"
	DomainInventory  = "inventory"
	DomainMarketing  = "marketing"
	DomainUnknown    = "unknown"
)

// ValidDomains contains all valid business domain values.
var ValidDomains = []string{
	DomainSales,
	DomainFinance,
	DomainOperations,
	DomainCustomer,
	DomainProduct,
	DomainAnalytics,
	DomainHR,
	DomainInventory,
	DomainMarketing,
	DomainUnknown,
}

// ============================================================================
// Tiered Ontology - Complete Structure
// ============================================================================

// TieredOntology represents the complete hierarchical ontology structure.
type TieredOntology struct {
	ID              uuid.UUID                 `json:"id"`
	ProjectID       uuid.UUID                 `json:"project_id"`
	Version         int                       `json:"version"`
	IsActive        bool                      `json:"is_active"`
	DomainSummary   *DomainSummary            `json:"domain_summary,omitempty"`
	EntitySummaries map[string]*EntitySummary `json:"entity_summaries,omitempty"` // table_name -> summary
	ColumnDetails   map[string][]ColumnDetail `json:"column_details,omitempty"`   // table_name -> columns
	Metadata        map[string]any            `json:"metadata,omitempty"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
}

// GetEntitySummary returns the summary for a specific table, or nil if not found.
func (o *TieredOntology) GetEntitySummary(tableName string) *EntitySummary {
	if o.EntitySummaries == nil {
		return nil
	}
	return o.EntitySummaries[tableName]
}

// GetColumnDetails returns the column details for a specific table, or nil if not found.
func (o *TieredOntology) GetColumnDetails(tableName string) []ColumnDetail {
	if o.ColumnDetails == nil {
		return nil
	}
	return o.ColumnDetails[tableName]
}

// TableCount returns the number of tables in the ontology.
func (o *TieredOntology) TableCount() int {
	if o.EntitySummaries == nil {
		return 0
	}
	return len(o.EntitySummaries)
}

// ColumnCount returns the total number of columns across all tables.
func (o *TieredOntology) ColumnCount() int {
	if o.ColumnDetails == nil {
		return 0
	}
	count := 0
	for _, columns := range o.ColumnDetails {
		count += len(columns)
	}
	return count
}

// TotalEntityCount returns the total number of entities: 1 (global) + tables + columns.
func (o *TieredOntology) TotalEntityCount() int {
	return 1 + o.TableCount() + o.ColumnCount()
}

// ============================================================================
// Description Processing Types
// ============================================================================

// DomainContext contains the LLM-refined domain understanding.
// This is derived from user description + schema analysis.
type DomainContext struct {
	Summary        string            `json:"summary"`
	PrimaryDomains []string          `json:"primary_domains"`
	KeyTerminology map[string]string `json:"key_terminology"`
}

// EntityHint provides pre-seeded context for a table from user description.
type EntityHint struct {
	BusinessName string   `json:"business_name,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	Synonyms     []string `json:"synonyms,omitempty"`
}
