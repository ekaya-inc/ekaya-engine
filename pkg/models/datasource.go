package models

import (
	"time"

	"github.com/google/uuid"
)

// Datasource represents an external data connection for a project.
// The Config field contains connection details (credentials, host, etc.)
// which are encrypted at rest by the service layer.
type Datasource struct {
	ID             uuid.UUID      `json:"id"`
	ProjectID      uuid.UUID      `json:"project_id"`
	Name           string         `json:"name"`
	DatasourceType string         `json:"datasource_type"` // "postgres", "clickhouse", "bigquery", etc.
	Config         map[string]any `json:"config"`          // Decrypted config, structure varies by type
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}
