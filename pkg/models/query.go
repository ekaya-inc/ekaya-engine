package models

import (
	"time"

	"github.com/google/uuid"
)

// QueryParameter defines a single parameter for a parameterized query.
type QueryParameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`        // string, integer, decimal, boolean, date, timestamp, uuid, string[], integer[]
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"` // nil if no default
}

// Query represents a saved SQL query with metadata.
type Query struct {
	ID                    uuid.UUID        `json:"id"`
	ProjectID             uuid.UUID        `json:"project_id"`
	DatasourceID          uuid.UUID        `json:"datasource_id"`
	NaturalLanguagePrompt string           `json:"natural_language_prompt"`
	AdditionalContext     *string          `json:"additional_context,omitempty"`
	SQLQuery              string           `json:"sql_query"`
	Dialect               string           `json:"dialect"`
	IsEnabled             bool             `json:"is_enabled"`
	Parameters            []QueryParameter `json:"parameters,omitempty"`
	UsageCount            int              `json:"usage_count"`
	LastUsedAt            *time.Time       `json:"last_used_at,omitempty"`
	CreatedAt             time.Time        `json:"created_at"`
	UpdatedAt             time.Time        `json:"updated_at"`
}
