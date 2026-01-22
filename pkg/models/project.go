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
