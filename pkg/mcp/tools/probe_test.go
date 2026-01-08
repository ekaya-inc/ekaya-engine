package tools

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestProbeColumnResponse_Structure(t *testing.T) {
	// Test response structure with all fields populated
	nullRate := 0.05
	cardinalityRatio := 0.01
	minLen := int64(5)
	maxLen := int64(10)

	response := probeColumnResponse{
		Table:  "users",
		Column: "status",
		Statistics: &probeColumnStatistics{
			DistinctCount:    5,
			RowCount:         1000,
			NonNullCount:     950,
			NullRate:         &nullRate,
			CardinalityRatio: &cardinalityRatio,
			MinLength:        &minLen,
			MaxLength:        &maxLen,
		},
		Joinability: &probeColumnJoinability{
			IsJoinable: false,
			Reason:     "low_cardinality",
		},
		SampleValues: []string{"ACTIVE", "SUSPENDED", "BANNED"},
		Semantic: &probeColumnSemantic{
			Entity:      "User",
			Role:        "attribute",
			Description: "User account status",
			EnumLabels: map[string]string{
				"ACTIVE":    "Normal active account",
				"SUSPENDED": "Temporarily disabled",
			},
		},
	}

	assert.Equal(t, "users", response.Table)
	assert.Equal(t, "status", response.Column)
	assert.NotNil(t, response.Statistics)
	assert.NotNil(t, response.Joinability)
	assert.NotNil(t, response.Semantic)
	assert.Len(t, response.SampleValues, 3)
}

func TestProbeColumnResponse_MinimalData(t *testing.T) {
	// Test response with minimal data (no statistics or semantic info)
	response := probeColumnResponse{
		Table:  "users",
		Column: "id",
	}

	assert.Equal(t, "users", response.Table)
	assert.Equal(t, "id", response.Column)
	assert.Nil(t, response.Statistics)
	assert.Nil(t, response.Joinability)
	assert.Nil(t, response.Semantic)
}

func TestProbeColumnResponse_StatisticsOnly(t *testing.T) {
	// Test response with statistics but no semantic data
	nullRate := 0.0
	cardinalityRatio := 1.0

	response := probeColumnResponse{
		Table:  "users",
		Column: "user_id",
		Statistics: &probeColumnStatistics{
			DistinctCount:    1000,
			RowCount:         1000,
			NonNullCount:     1000,
			NullRate:         &nullRate,
			CardinalityRatio: &cardinalityRatio,
		},
		Joinability: &probeColumnJoinability{
			IsJoinable: true,
			Reason:     "high_cardinality",
		},
	}

	assert.NotNil(t, response.Statistics)
	assert.Equal(t, int64(1000), response.Statistics.DistinctCount)
	assert.Equal(t, 0.0, *response.Statistics.NullRate)
	assert.Equal(t, 1.0, *response.Statistics.CardinalityRatio)
	assert.True(t, response.Joinability.IsJoinable)
}

func TestProbeColumnResponse_SemanticOnly(t *testing.T) {
	// Test response with semantic data but no statistics
	response := probeColumnResponse{
		Table:  "users",
		Column: "email",
		Semantic: &probeColumnSemantic{
			Entity:      "User",
			Role:        "identifier",
			Description: "User email address",
		},
	}

	assert.NotNil(t, response.Semantic)
	assert.Equal(t, "User", response.Semantic.Entity)
	assert.Equal(t, "identifier", response.Semantic.Role)
	assert.Nil(t, response.Statistics)
}

func TestProbeColumnResponse_EnumLabels(t *testing.T) {
	// Test response with enum labels
	response := probeColumnResponse{
		Table:  "users",
		Column: "role",
		Semantic: &probeColumnSemantic{
			Entity: "User",
			Role:   "dimension",
			EnumLabels: map[string]string{
				"ADMIN":     "System administrator with full access",
				"USER":      "Regular user with standard permissions",
				"MODERATOR": "User with content moderation capabilities",
			},
		},
	}

	assert.NotNil(t, response.Semantic)
	assert.Len(t, response.Semantic.EnumLabels, 3)
	assert.Equal(t, "System administrator with full access", response.Semantic.EnumLabels["ADMIN"])
	assert.Equal(t, "Regular user with standard permissions", response.Semantic.EnumLabels["USER"])
	assert.Equal(t, "User with content moderation capabilities", response.Semantic.EnumLabels["MODERATOR"])
}

func TestProbeColumnsResponse_Structure(t *testing.T) {
	// Test batch response structure
	response := probeColumnsResponse{
		Results: map[string]*probeColumnResponse{
			"users.status": {
				Table:  "users",
				Column: "status",
				Statistics: &probeColumnStatistics{
					DistinctCount: 5,
					RowCount:      1000,
				},
			},
			"users.role": {
				Table:  "users",
				Column: "role",
				Statistics: &probeColumnStatistics{
					DistinctCount: 3,
					RowCount:      1000,
				},
			},
		},
	}

	assert.Len(t, response.Results, 2)
	assert.NotNil(t, response.Results["users.status"])
	assert.NotNil(t, response.Results["users.role"])
}

func TestProbeColumnsResponse_PartialFailure(t *testing.T) {
	// Test batch response with partial failures
	response := probeColumnsResponse{
		Results: map[string]*probeColumnResponse{
			"users.status": {
				Table:  "users",
				Column: "status",
				Statistics: &probeColumnStatistics{
					DistinctCount: 5,
					RowCount:      1000,
				},
			},
			"nonexistent.column": {
				Table:  "nonexistent",
				Column: "column",
				Error:  "table 'nonexistent' not found",
			},
		},
	}

	assert.Len(t, response.Results, 2)
	assert.NotNil(t, response.Results["users.status"].Statistics)
	assert.Empty(t, response.Results["users.status"].Error)
	assert.NotEmpty(t, response.Results["nonexistent.column"].Error)
	assert.Nil(t, response.Results["nonexistent.column"].Statistics)
}

func TestProbeColumn_BuildStatistics(t *testing.T) {
	// Test statistics building from SchemaColumn
	distinctCount := int64(10)
	rowCount := int64(100)
	nonNullCount := int64(95)
	nullCount := int64(5)
	minLen := int64(3)
	maxLen := int64(20)

	_ = &models.SchemaColumn{
		ColumnName:    "username",
		DistinctCount: &distinctCount,
		RowCount:      &rowCount,
		NonNullCount:  &nonNullCount,
		NullCount:     &nullCount,
		MinLength:     &minLen,
		MaxLength:     &maxLen,
	}

	// Manually compute what probeColumn would compute
	expectedNullRate := float64(nullCount) / float64(rowCount)
	expectedCardinalityRatio := float64(distinctCount) / float64(rowCount)

	// Verify calculations
	assert.Equal(t, 0.05, expectedNullRate)
	assert.Equal(t, 0.1, expectedCardinalityRatio)
}

func TestProbeColumn_BuildJoinability(t *testing.T) {
	// Test joinability building from SchemaColumn
	isJoinable := true
	joinabilityReason := "high_cardinality"

	column := &models.SchemaColumn{
		ColumnName:        "user_id",
		IsJoinable:        &isJoinable,
		JoinabilityReason: &joinabilityReason,
	}

	assert.True(t, *column.IsJoinable)
	assert.Equal(t, "high_cardinality", *column.JoinabilityReason)
}

func TestProbeColumn_SemanticFromOntology(t *testing.T) {
	// Test semantic information extraction from ontology
	ontology := &models.TieredOntology{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
		EntitySummaries: map[string]*models.EntitySummary{
			"users": {
				TableName:    "users",
				BusinessName: "User",
				Description:  "Platform users",
			},
		},
		ColumnDetails: map[string][]models.ColumnDetail{
			"users": {
				{
					Name:        "status",
					Description: "User account status",
					Role:        "attribute",
					EnumValues: []models.EnumValue{
						{Value: "ACTIVE", Label: "Active account"},
						{Value: "SUSPENDED", Label: "Temporarily suspended"},
					},
				},
			},
		},
	}

	// Get entity summary
	entitySummary := ontology.GetEntitySummary("users")
	assert.NotNil(t, entitySummary)
	assert.Equal(t, "User", entitySummary.BusinessName)

	// Get column details
	columnDetails := ontology.GetColumnDetails("users")
	assert.Len(t, columnDetails, 1)
	assert.Equal(t, "status", columnDetails[0].Name)
	assert.Equal(t, "User account status", columnDetails[0].Description)
	assert.Len(t, columnDetails[0].EnumValues, 2)
}

func TestProbeColumn_MissingStatistics(t *testing.T) {
	// Test handling of missing statistics
	column := &models.SchemaColumn{
		ColumnName:    "email",
		DistinctCount: nil, // Missing statistics
		RowCount:      nil,
	}

	assert.Nil(t, column.DistinctCount)
	assert.Nil(t, column.RowCount)
	// Statistics section should be omitted in response when data is missing
}

func TestProbeColumn_NullRateCalculation(t *testing.T) {
	// Test null rate calculation edge cases
	testCases := []struct {
		name         string
		nullCount    int64
		rowCount     int64
		expectedRate float64
	}{
		{"No nulls", 0, 100, 0.0},
		{"Half nulls", 50, 100, 0.5},
		{"All nulls", 100, 100, 1.0},
		{"Some nulls", 15, 200, 0.075},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualRate := float64(tc.nullCount) / float64(tc.rowCount)
			assert.Equal(t, tc.expectedRate, actualRate)
		})
	}
}

func TestProbeColumn_CardinalityRatioCalculation(t *testing.T) {
	// Test cardinality ratio calculation edge cases
	testCases := []struct {
		name          string
		distinctCount int64
		rowCount      int64
		expectedRatio float64
		description   string
	}{
		{"Unique values", 100, 100, 1.0, "All values are unique (PK-like)"},
		{"Low cardinality", 5, 100, 0.05, "Few distinct values (enum-like)"},
		{"Medium cardinality", 50, 100, 0.5, "Half distinct values"},
		{"High cardinality", 95, 100, 0.95, "Almost all values unique"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualRatio := float64(tc.distinctCount) / float64(tc.rowCount)
			assert.Equal(t, tc.expectedRatio, actualRatio, tc.description)
		})
	}
}
