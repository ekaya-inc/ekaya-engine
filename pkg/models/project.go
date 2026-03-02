// Package models contains domain types for ekaya-engine.
package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Project represents a project in the system.
type Project struct {
	ID            uuid.UUID              `json:"id"`
	Name          string                 `json:"name"`
	Parameters    map[string]interface{} `json:"parameters"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Status        string                 `json:"status"`
	IndustryType  string                 `json:"industry_type"`
	DomainSummary *DomainSummary         `json:"domain_summary,omitempty"`
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

// EnumFile represents the structure of an enum definitions file (.ekaya/enums.yaml).
// This allows projects to define enum values and their meanings in a YAML file format.
type EnumFile struct {
	// Enums is the list of enum definitions in the file.
	Enums []EnumDefinition `yaml:"enums" json:"enums"`
}

// ParseEnumFileContent parses enum definitions from raw content.
// The ext parameter should be the file extension (e.g., ".yaml", ".json") to determine format.
// If ext is empty or unknown, YAML is tried first, then JSON.
func ParseEnumFileContent(data []byte, ext string) ([]EnumDefinition, error) {
	var enumFile EnumFile

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &enumFile); err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &enumFile); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
	default:
		// Try YAML first (more permissive), then JSON
		if err := yaml.Unmarshal(data, &enumFile); err != nil {
			if jsonErr := json.Unmarshal(data, &enumFile); jsonErr != nil {
				return nil, fmt.Errorf("parse enum file (tried YAML and JSON): YAML: %v, JSON: %v", err, jsonErr)
			}
		}
	}

	return enumFile.Enums, nil
}
