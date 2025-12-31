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
	JoinCount     int64
	SourceMatched int64
	TargetMatched int64
	OrphanCount   int64
}
