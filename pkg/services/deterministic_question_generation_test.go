package services

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestDeterministicQuestionGenerator_HighNullRate(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()
	workflowID := uuid.New()

	gen := NewDeterministicQuestionGenerator(projectID, ontologyID, &workflowID)

	tests := []struct {
		name           string
		tables         []*models.SchemaTable
		wantQuestions  int
		wantCategory   string
		wantColumnName string
	}{
		{
			name: "generates question for 90% NULL rate column",
			tables: []*models.SchemaTable{
				{
					TableName:  "users",
					IsSelected: true,
					RowCount:   ptrTo(int64(1000)),
					Columns: []models.SchemaColumn{
						{
							ColumnName: "phone_number",
							IsSelected: true,
							NullCount:  ptrTo(int64(900)),
						},
					},
				},
			},
			wantQuestions:  1,
			wantCategory:   models.QuestionCategoryDataQuality,
			wantColumnName: "phone_number",
		},
		{
			name: "skips column with 50% NULL rate",
			tables: []*models.SchemaTable{
				{
					TableName:  "users",
					IsSelected: true,
					RowCount:   ptrTo(int64(1000)),
					Columns: []models.SchemaColumn{
						{
							ColumnName: "phone_number",
							IsSelected: true,
							NullCount:  ptrTo(int64(500)),
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips known optional column deleted_at",
			tables: []*models.SchemaTable{
				{
					TableName:  "users",
					IsSelected: true,
					RowCount:   ptrTo(int64(1000)),
					Columns: []models.SchemaColumn{
						{
							ColumnName: "deleted_at",
							IsSelected: true,
							NullCount:  ptrTo(int64(950)),
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips known optional column with _notes suffix",
			tables: []*models.SchemaTable{
				{
					TableName:  "orders",
					IsSelected: true,
					RowCount:   ptrTo(int64(1000)),
					Columns: []models.SchemaColumn{
						{
							ColumnName: "internal_notes",
							IsSelected: true,
							NullCount:  ptrTo(int64(900)),
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips known optional column description",
			tables: []*models.SchemaTable{
				{
					TableName:  "products",
					IsSelected: true,
					RowCount:   ptrTo(int64(1000)),
					Columns: []models.SchemaColumn{
						{
							ColumnName: "description",
							IsSelected: true,
							NullCount:  ptrTo(int64(850)),
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips unselected tables",
			tables: []*models.SchemaTable{
				{
					TableName:  "users",
					IsSelected: false,
					RowCount:   ptrTo(int64(1000)),
					Columns: []models.SchemaColumn{
						{
							ColumnName: "phone_number",
							IsSelected: true,
							NullCount:  ptrTo(int64(900)),
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips unselected columns",
			tables: []*models.SchemaTable{
				{
					TableName:  "users",
					IsSelected: true,
					RowCount:   ptrTo(int64(1000)),
					Columns: []models.SchemaColumn{
						{
							ColumnName: "phone_number",
							IsSelected: false,
							NullCount:  ptrTo(int64(900)),
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips columns without null count",
			tables: []*models.SchemaTable{
				{
					TableName:  "users",
					IsSelected: true,
					RowCount:   ptrTo(int64(1000)),
					Columns: []models.SchemaColumn{
						{
							ColumnName: "phone_number",
							IsSelected: true,
							NullCount:  nil,
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips tables without row count",
			tables: []*models.SchemaTable{
				{
					TableName:  "users",
					IsSelected: true,
					RowCount:   nil,
					Columns: []models.SchemaColumn{
						{
							ColumnName: "phone_number",
							IsSelected: true,
							NullCount:  ptrTo(int64(900)),
						},
					},
				},
			},
			wantQuestions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			questions := gen.GenerateFromSchema(tt.tables)

			assert.Len(t, questions, tt.wantQuestions)

			if tt.wantQuestions > 0 {
				q := questions[0]
				assert.Equal(t, tt.wantCategory, q.Category)
				assert.Contains(t, q.Affects.Columns[0], tt.wantColumnName)
				assert.Equal(t, 3, q.Priority) // nice-to-have
				assert.False(t, q.IsRequired)
				assert.Equal(t, models.QuestionStatusPending, q.Status)
				assert.Equal(t, projectID, q.ProjectID)
				assert.Equal(t, ontologyID, q.OntologyID)
				assert.Equal(t, &workflowID, q.WorkflowID)
			}
		})
	}
}

func TestDeterministicQuestionGenerator_CrypticEnumValues(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()

	gen := NewDeterministicQuestionGenerator(projectID, ontologyID, nil)

	tests := []struct {
		name           string
		tables         []*models.SchemaTable
		wantQuestions  int
		wantCategory   string
		wantColumnName string
	}{
		{
			name: "generates question for single letter enum values",
			tables: []*models.SchemaTable{
				{
					TableName:  "orders",
					IsSelected: true,
					Columns: []models.SchemaColumn{
						{
							ColumnName:    "status",
							IsSelected:    true,
							DistinctCount: ptrTo(int64(4)),
							SampleValues:  []string{"A", "P", "C", "X"},
						},
					},
				},
			},
			wantQuestions:  1,
			wantCategory:   models.QuestionCategoryEnumeration,
			wantColumnName: "status",
		},
		{
			name: "generates question for numeric codes",
			tables: []*models.SchemaTable{
				{
					TableName:  "orders",
					IsSelected: true,
					Columns: []models.SchemaColumn{
						{
							ColumnName:    "type",
							IsSelected:    true,
							DistinctCount: ptrTo(int64(5)),
							SampleValues:  []string{"1", "2", "3", "4", "5"},
						},
					},
				},
			},
			wantQuestions:  1,
			wantCategory:   models.QuestionCategoryEnumeration,
			wantColumnName: "type",
		},
		{
			name: "generates question for mixed alphanumeric codes",
			tables: []*models.SchemaTable{
				{
					TableName:  "products",
					IsSelected: true,
					Columns: []models.SchemaColumn{
						{
							ColumnName:    "category_code",
							IsSelected:    true,
							DistinctCount: ptrTo(int64(6)),
							SampleValues:  []string{"A1", "B2", "C3", "D4", "E5", "F6"},
						},
					},
				},
			},
			wantQuestions:  1,
			wantCategory:   models.QuestionCategoryEnumeration,
			wantColumnName: "category_code",
		},
		{
			name: "skips boolean-like values (true/false)",
			tables: []*models.SchemaTable{
				{
					TableName:  "users",
					IsSelected: true,
					Columns: []models.SchemaColumn{
						{
							ColumnName:    "is_active",
							IsSelected:    true,
							DistinctCount: ptrTo(int64(2)),
							SampleValues:  []string{"true", "false"},
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips boolean-like values (yes/no)",
			tables: []*models.SchemaTable{
				{
					TableName:  "users",
					IsSelected: true,
					Columns: []models.SchemaColumn{
						{
							ColumnName:    "subscribed",
							IsSelected:    true,
							DistinctCount: ptrTo(int64(2)),
							SampleValues:  []string{"yes", "no"},
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips boolean-like values (1/0)",
			tables: []*models.SchemaTable{
				{
					TableName:  "users",
					IsSelected: true,
					Columns: []models.SchemaColumn{
						{
							ColumnName:    "active",
							IsSelected:    true,
							DistinctCount: ptrTo(int64(2)),
							SampleValues:  []string{"1", "0"},
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips readable enum values",
			tables: []*models.SchemaTable{
				{
					TableName:  "orders",
					IsSelected: true,
					Columns: []models.SchemaColumn{
						{
							ColumnName:    "status",
							IsSelected:    true,
							DistinctCount: ptrTo(int64(4)),
							SampleValues:  []string{"pending", "approved", "shipped", "delivered"},
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips columns with no sample values",
			tables: []*models.SchemaTable{
				{
					TableName:  "orders",
					IsSelected: true,
					Columns: []models.SchemaColumn{
						{
							ColumnName:   "status",
							IsSelected:   true,
							SampleValues: nil,
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips high cardinality columns (>20 distinct)",
			tables: []*models.SchemaTable{
				{
					TableName:  "orders",
					IsSelected: true,
					Columns: []models.SchemaColumn{
						{
							ColumnName:    "code",
							IsSelected:    true,
							DistinctCount: ptrTo(int64(100)),
							SampleValues:  []string{"A", "B", "C"},
						},
					},
				},
			},
			wantQuestions: 0,
		},
		{
			name: "skips when only minority of values are cryptic",
			tables: []*models.SchemaTable{
				{
					TableName:  "orders",
					IsSelected: true,
					Columns: []models.SchemaColumn{
						{
							ColumnName:    "status",
							IsSelected:    true,
							DistinctCount: ptrTo(int64(5)),
							SampleValues:  []string{"pending", "approved", "shipped", "delivered", "X"},
						},
					},
				},
			},
			wantQuestions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			questions := gen.GenerateFromSchema(tt.tables)

			assert.Len(t, questions, tt.wantQuestions)

			if tt.wantQuestions > 0 {
				q := questions[0]
				assert.Equal(t, tt.wantCategory, q.Category)
				assert.Contains(t, q.Affects.Columns[0], tt.wantColumnName)
				assert.Equal(t, 1, q.Priority) // critical
				assert.True(t, q.IsRequired)
				assert.Equal(t, models.QuestionStatusPending, q.Status)
				assert.Equal(t, models.PatternEnumColumn, q.DetectedPattern)
			}
		})
	}
}

func TestDeterministicQuestionGenerator_CombinedQuestions(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()

	gen := NewDeterministicQuestionGenerator(projectID, ontologyID, nil)

	// Table with both high NULL rate and cryptic enum values
	tables := []*models.SchemaTable{
		{
			TableName:  "orders",
			IsSelected: true,
			RowCount:   ptrTo(int64(1000)),
			Columns: []models.SchemaColumn{
				{
					ColumnName: "phone",
					IsSelected: true,
					NullCount:  ptrTo(int64(950)),
				},
				{
					ColumnName:    "status",
					IsSelected:    true,
					DistinctCount: ptrTo(int64(4)),
					SampleValues:  []string{"A", "P", "C", "X"},
				},
			},
		},
	}

	questions := gen.GenerateFromSchema(tables)

	require.Len(t, questions, 2)

	// Find the data quality question
	var dataQualityQ, enumQ *models.OntologyQuestion
	for _, q := range questions {
		if q.Category == models.QuestionCategoryDataQuality {
			dataQualityQ = q
		}
		if q.Category == models.QuestionCategoryEnumeration {
			enumQ = q
		}
	}

	require.NotNil(t, dataQualityQ)
	assert.Contains(t, dataQualityQ.Text, "phone")
	assert.Contains(t, dataQualityQ.Text, "95%")
	assert.Equal(t, 3, dataQualityQ.Priority)

	require.NotNil(t, enumQ)
	assert.Contains(t, enumQ.Text, "status")
	assert.Equal(t, 1, enumQ.Priority)
	assert.True(t, enumQ.IsRequired)
}

func TestIsKnownOptionalColumn(t *testing.T) {
	tests := []struct {
		columnName string
		want       bool
	}{
		// Exact matches
		{"deleted_at", true},
		{"DELETED_AT", true}, // case insensitive
		{"archived_at", true},
		{"description", true},
		{"notes", true},
		{"middle_name", true},
		{"avatar_url", true},
		{"metadata", true},

		// Suffix patterns
		{"internal_notes", true},
		{"order_description", true},
		{"image_url", true},
		{"created_at", true},
		{"updated_on", true},

		// Prefix patterns
		{"old_email", true},
		{"legacy_id", true},
		{"alt_phone", true},
		{"custom_field", true},

		// Non-optional columns
		{"email", false},
		{"name", false},
		{"user_id", false},
		{"amount", false},
		{"created_by", false},
	}

	for _, tt := range tests {
		t.Run(tt.columnName, func(t *testing.T) {
			got := isKnownOptionalColumn(tt.columnName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsCrypticValue(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		// Cryptic values
		{"A", true},
		{"X", true},
		{"1", true},
		{"99", true},
		{"A1", true},
		{"2B", true},
		{"NY", true},
		{"CA", true},

		// Readable values
		{"pending", false},
		{"approved", false},
		{"active", false},
		{"PENDING", false},   // Readable word
		{"USA", true},        // 3-letter abbreviation - cryptic without context
		{"ABCD", false},      // 4-letter abbreviation might be readable
		{"completed", false},
		{"", false},
		{"   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got := isCrypticValue(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsBooleanLike(t *testing.T) {
	tests := []struct {
		values []string
		want   bool
	}{
		{[]string{"true", "false"}, true},
		{[]string{"false", "true"}, true},
		{[]string{"TRUE", "FALSE"}, true},
		{[]string{"yes", "no"}, true},
		{[]string{"Y", "N"}, true},
		{[]string{"1", "0"}, true},
		{[]string{"on", "off"}, true},
		{[]string{"active", "inactive"}, true},
		{[]string{"enabled", "disabled"}, true},

		{[]string{"A", "B", "C"}, false},
		{[]string{"pending"}, false},
		{[]string{"yes", "no", "maybe"}, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := isBooleanLike(tt.values)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatValuesList(t *testing.T) {
	tests := []struct {
		values []string
		want   string
	}{
		{[]string{}, ""},
		{[]string{"A"}, "'A'"},
		{[]string{"A", "B"}, "'A', 'B'"},
		{[]string{"A", "B", "C", "D", "E"}, "'A', 'B', 'C', 'D', 'E'"},
		{[]string{"A", "B", "C", "D", "E", "F", "G"}, "'A', 'B', 'C', 'D', 'E' (and 2 more)"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := formatValuesList(tt.values)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ptrTo is a helper function to create a pointer to a value.
func ptrTo[T any](v T) *T {
	return &v
}
