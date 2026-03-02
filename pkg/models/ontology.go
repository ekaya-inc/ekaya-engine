package models

import (
	"encoding/json"

	"github.com/ekaya-inc/ekaya-engine/pkg/jsonutil"
)

// ============================================================================
// Ontology - Domain Layer
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
