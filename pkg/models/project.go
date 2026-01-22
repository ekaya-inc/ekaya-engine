// Package models contains domain types for ekaya-engine.
package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Project represents a project in the system.
type Project struct {
	ID           uuid.UUID              `json:"id"`
	Name         string                 `json:"name"`
	Parameters   map[string]interface{} `json:"parameters"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	Status       string                 `json:"status"`
	IndustryType string                 `json:"industry_type"`
}

// IndustryType constants for template selection.
const (
	IndustryGeneral     = "general"
	IndustryVideoStream = "video_streaming"
	IndustryMarketplace = "marketplace"
	IndustryCreatorEcon = "creator_economy"
)

// EnumDefinition defines the known values and their meanings for an enum column.
// This allows projects to provide explicit enum definitions that get merged
// during column enrichment, providing accurate descriptions for integer/string
// enum values that the LLM cannot infer from data alone.
type EnumDefinition struct {
	// Table is the table name containing the enum column.
	// Use "*" to apply to any table with this column name.
	Table string `json:"table"`

	// Column is the column name containing enum values.
	Column string `json:"column"`

	// Values maps raw enum values (as strings) to their descriptions.
	// Example: {"1": "STARTED - Transaction started", "2": "ENDED - Transaction ended"}
	Values map[string]string `json:"values"`
}

// ProjectConfig contains project-level configuration that can be customized
// per-project. This includes enum definitions and other semantic annotations.
type ProjectConfig struct {
	// EnumDefinitions provides explicit enum value → description mappings
	// for columns where the LLM cannot infer meanings from data alone.
	EnumDefinitions []EnumDefinition `json:"enum_definitions,omitempty"`
}

// JSONBMap is a map type that handles PostgreSQL JSONB serialization.
type JSONBMap map[string]interface{}

// Value implements driver.Valuer for database serialization.
func (j JSONBMap) Value() (driver.Value, error) {
	if j == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner for database deserialization.
func (j *JSONBMap) Scan(value interface{}) error {
	if value == nil {
		*j = make(map[string]interface{})
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into JSONBMap", value)
	}

	return json.Unmarshal(bytes, j)
}
