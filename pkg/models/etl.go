package models

import (
	"time"

	"github.com/google/uuid"
)

// ETL load status constants.
const (
	ETLStatusPending   = "pending"
	ETLStatusRunning   = "running"
	ETLStatusCompleted = "completed"
	ETLStatusFailed    = "failed"
)

// ETL conflict resolution strategies.
const (
	ETLConflictAppend  = "append"
	ETLConflictReplace = "replace"
	ETLConflictError   = "error"
)

// InferredColumn represents a column inferred from data sampling.
type InferredColumn struct {
	Name         string   `json:"name"`
	SQLType      string   `json:"sql_type"`
	Nullable     bool     `json:"nullable"`
	SampleValues []string `json:"sample_values,omitempty"`
}

// MatchResult represents the result of ontology matching.
type MatchResult struct {
	MatchedTable   string          `json:"matched_table,omitempty"`
	ColumnMappings []ColumnMapping `json:"column_mappings,omitempty"`
	Confidence     float64         `json:"confidence"`
	IsNewTable     bool            `json:"is_new_table"`
}

// ColumnMapping maps an inferred column to an existing ontology column.
type ColumnMapping struct {
	InferredName  string  `json:"inferred_name"`
	MappedName    string  `json:"mapped_name"`
	InferredType  string  `json:"inferred_type"`
	MappedType    string  `json:"mapped_type"`
	Confidence    float64 `json:"confidence"`
	SemanticMatch bool    `json:"semantic_match"`
}

// LoadResult holds the outcome of a data load operation.
type LoadResult struct {
	TableName     string   `json:"table_name"`
	RowsAttempted int      `json:"rows_attempted"`
	RowsLoaded    int      `json:"rows_loaded"`
	RowsSkipped   int      `json:"rows_skipped"`
	Errors        []string `json:"errors,omitempty"`
}

// LoadStatus tracks a single ETL load operation.
type LoadStatus struct {
	ID            uuid.UUID  `json:"id"`
	ProjectID     uuid.UUID  `json:"project_id"`
	AppID         string     `json:"app_id"`
	FileName      string     `json:"file_name"`
	TableName     string     `json:"table_name"`
	RowsAttempted int        `json:"rows_attempted"`
	RowsLoaded    int        `json:"rows_loaded"`
	RowsSkipped   int        `json:"rows_skipped"`
	Errors        []string   `json:"errors,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Status        string     `json:"status"`
}

// ETLSettings holds per-applet configuration stored in engine_installed_apps.settings.
type ETLSettings struct {
	WatchDirectory   string `json:"watch_directory,omitempty"`
	AutoCreateTables bool   `json:"auto_create_tables"`
	BatchSize        int    `json:"batch_size"`
	SampleRows       int    `json:"sample_rows"`
	OnConflict       string `json:"on_conflict"`
	UseOntology      bool   `json:"use_ontology"`
}

// DefaultETLSettings returns settings with sensible defaults.
func DefaultETLSettings() ETLSettings {
	return ETLSettings{
		AutoCreateTables: true,
		BatchSize:        500,
		SampleRows:       100,
		OnConflict:       ETLConflictAppend,
		UseOntology:      true,
	}
}

// ParseResult holds parsed data from a file (CSV or XLSX).
type ParseResult struct {
	Headers   []string   `json:"headers"`
	Rows      [][]string `json:"rows"`
	Delimiter rune       `json:"delimiter,omitempty"`
}

// SheetData holds parsed data from a single XLSX sheet.
type SheetData struct {
	SheetName string     `json:"sheet_name"`
	Headers   []string   `json:"headers"`
	Rows      [][]string `json:"rows"`
}

// PreviewResult holds the preview data returned before confirming a load.
type PreviewResult struct {
	FileName       string           `json:"file_name"`
	InferredSchema []InferredColumn `json:"inferred_schema"`
	OntologyMatch  *MatchResult     `json:"ontology_match,omitempty"`
	SampleRows     [][]string       `json:"sample_rows"`
	TotalRows      int              `json:"total_rows"`
}
