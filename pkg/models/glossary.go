package models

import (
	"time"

	"github.com/google/uuid"
)

// BusinessGlossaryTerm represents a business term with its technical mapping.
// Used for reverse lookup from business term → schema/SQL pattern.
// Stored in engine_business_glossary table.
type BusinessGlossaryTerm struct {
	ID          uuid.UUID  `json:"id"`
	ProjectID   uuid.UUID  `json:"project_id"`
	Term        string     `json:"term"`
	Definition  string     `json:"definition"`
	SQLPattern  string     `json:"sql_pattern,omitempty"`
	BaseTable   string     `json:"base_table,omitempty"`
	ColumnsUsed []string   `json:"columns_used,omitempty"`
	Filters     []Filter   `json:"filters,omitempty"`
	Aggregation string     `json:"aggregation,omitempty"`
	Source      string     `json:"source"` // "user" or "suggested"
	CreatedBy   *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Filter represents a condition in a glossary term definition.
// Example: {"column": "transaction_state", "operator": "=", "values": ["completed"]}
type Filter struct {
	Column   string   `json:"column"`
	Operator string   `json:"operator"` // =, IN, >, <, >=, <=, !=, BETWEEN, LIKE, IS NULL, IS NOT NULL
	Values   []string `json:"values"`
}
