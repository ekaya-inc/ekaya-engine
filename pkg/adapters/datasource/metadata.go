package datasource

// TableMetadata represents a discovered database table.
type TableMetadata struct {
	SchemaName string
	TableName  string
	RowCount   int64
}

// ColumnMetadata represents a discovered database column.
type ColumnMetadata struct {
	ColumnName      string
	DataType        string
	IsNullable      bool
	IsPrimaryKey    bool
	IsUnique        bool
	OrdinalPosition int
	DefaultValue    *string
}

// ForeignKeyMetadata represents a discovered foreign key constraint.
type ForeignKeyMetadata struct {
	ConstraintName string
	SourceSchema   string
	SourceTable    string
	SourceColumn   string
	TargetSchema   string
	TargetTable    string
	TargetColumn   string
}

// ColumnStats contains statistics for a column.
type ColumnStats struct {
	ColumnName    string
	RowCount      int64
	NonNullCount  int64
	DistinctCount int64
	MinLength     *int64 // For text columns: minimum string length (nil for non-text)
	MaxLength     *int64 // For text columns: maximum string length (nil for non-text)
}

// ValueOverlapResult contains results from value overlap analysis.
type ValueOverlapResult struct {
	SourceDistinct int64
	TargetDistinct int64
	MatchedCount   int64
	MatchRate      float64
}

// JoinAnalysis contains results from join analysis.
type JoinAnalysis struct {
	JoinCount      int64
	SourceMatched  int64
	TargetMatched  int64
	OrphanCount    int64
	MaxSourceValue *int64 // Maximum value in source column (for semantic validation)
}

// EnumValueDistribution contains distribution statistics for a single enum value.
// Used for inferring state machine semantics (initial, terminal, error states).
type EnumValueDistribution struct {
	Value string `json:"value"` // The enum value (as string)

	// Occurrence statistics
	Count      int64   `json:"count"`       // Number of records with this value
	Percentage float64 `json:"percentage"`  // Percentage of total records (0.0-100.0)
	NullCount  int64   `json:"null_count"`  // Rows with NULL for this value (not applicable for non-NULL values)
	TotalRows  int64   `json:"total_rows"`  // Total rows in table (for reference)

	// Terminal state detection (correlation with completion timestamps)
	// These are populated when a completion timestamp column is detected in the same table
	HasCompletionAt       int64   `json:"has_completion_at,omitempty"`       // Records with this value AND non-NULL completion timestamp
	CompletionRate        float64 `json:"completion_rate,omitempty"`         // Percentage with completion timestamp (0.0-100.0)
	IsLikelyInitialState  bool    `json:"is_likely_initial_state,omitempty"` // ~0% completion, highest count among low-completion states
	IsLikelyTerminalState bool    `json:"is_likely_terminal_state,omitempty"` // ~100% completion rate
	IsLikelyErrorState    bool    `json:"is_likely_error_state,omitempty"`   // Low count relative to others
}

// EnumDistributionResult contains complete distribution analysis for an enum column.
type EnumDistributionResult struct {
	ColumnName              string                  `json:"column_name"`
	TotalRows               int64                   `json:"total_rows"`
	DistinctCount           int64                   `json:"distinct_count"`
	NullCount               int64                   `json:"null_count"`
	Distributions           []EnumValueDistribution `json:"distributions"`            // Sorted by count descending
	CompletionTimestampCol  string                  `json:"completion_timestamp_col,omitempty"` // Name of timestamp column used for terminal state detection
	HasStateSemantics       bool                    `json:"has_state_semantics"`      // True if initial/terminal states were identified
}
