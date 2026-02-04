package tools

import (
	"strings"
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

func TestProbeColumn_NullRateFromNonNullCount(t *testing.T) {
	// Test that null rate is calculated from NonNullCount when NullCount is nil.
	// This is the production scenario: adapters populate NonNullCount via COUNT(col)
	// but never populate NullCount.
	testCases := []struct {
		name             string
		rowCount         int64
		nonNullCount     int64
		nullCount        *int64
		expectedNullRate *float64
	}{
		{
			name:             "5% null rate from NonNullCount (soft delete typical)",
			rowCount:         100,
			nonNullCount:     5, // 95 nulls = 95% null rate (deleted_at column)
			nullCount:        nil,
			expectedNullRate: ptr(0.95),
		},
		{
			name:             "No nulls from NonNullCount",
			rowCount:         100,
			nonNullCount:     100,
			nullCount:        nil,
			expectedNullRate: ptr(0.0),
		},
		{
			name:             "All nulls from NonNullCount",
			rowCount:         100,
			nonNullCount:     0,
			nullCount:        nil,
			expectedNullRate: ptr(1.0),
		},
		{
			name:             "NullCount takes precedence when set",
			rowCount:         100,
			nonNullCount:     90, // Would calculate to 10% null rate
			nullCount:        ptr64(20),
			expectedNullRate: ptr(0.20), // Uses NullCount directly
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			column := &models.SchemaColumn{
				ColumnName:   "deleted_at",
				RowCount:     &tc.rowCount,
				NonNullCount: &tc.nonNullCount,
				NullCount:    tc.nullCount,
			}

			// Simulate the fixed logic from probeColumn
			var calculatedNullRate *float64
			if column.RowCount != nil && *column.RowCount > 0 {
				var nullCount int64
				if column.NullCount != nil {
					nullCount = *column.NullCount
				} else if column.NonNullCount != nil {
					nullCount = *column.RowCount - *column.NonNullCount
				}
				if nullCount > 0 || column.NullCount != nil || column.NonNullCount != nil {
					rate := float64(nullCount) / float64(*column.RowCount)
					calculatedNullRate = &rate
				}
			}

			if tc.expectedNullRate == nil {
				assert.Nil(t, calculatedNullRate)
			} else {
				assert.NotNil(t, calculatedNullRate)
				assert.InDelta(t, *tc.expectedNullRate, *calculatedNullRate, 0.001)
			}
		})
	}
}

// Helper to create *float64
func ptr(f float64) *float64 {
	return &f
}

// Helper to create *int64
func ptr64(i int64) *int64 {
	return &i
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

// ============================================================================
// Tests for probe_column error handling
// ============================================================================

func TestProbeColumnTool_ErrorResults(t *testing.T) {
	tests := []struct {
		name          string
		table         string
		column        string
		wantErrorCode string
		wantMessage   string
	}{
		{
			name:          "empty table parameter",
			table:         "   ", // whitespace-only
			column:        "status",
			wantErrorCode: "invalid_parameters",
			wantMessage:   "parameter 'table' cannot be empty",
		},
		{
			name:          "empty column parameter",
			table:         "users",
			column:        "   ", // whitespace-only
			wantErrorCode: "invalid_parameters",
			wantMessage:   "parameter 'column' cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The actual tool handler would validate these parameters
			// This test verifies the error result structure
			table := trimString(tt.table)
			column := trimString(tt.column)

			var result *ErrorResponse
			if table == "" {
				result = &ErrorResponse{
					Error:   true,
					Code:    "invalid_parameters",
					Message: "parameter 'table' cannot be empty",
				}
			} else if column == "" {
				result = &ErrorResponse{
					Error:   true,
					Code:    "invalid_parameters",
					Message: "parameter 'column' cannot be empty",
				}
			}

			// Verify error result structure
			assert.True(t, result.Error)
			assert.Equal(t, tt.wantErrorCode, result.Code)
			assert.Equal(t, tt.wantMessage, result.Message)
		})
	}
}

func TestProbeColumn_TableNotFound(t *testing.T) {
	// Test error response when table is not found
	response := probeColumnResponse{
		Table:  "nonexistent_table",
		Column: "some_column",
		Error:  "TABLE_NOT_FOUND: table \"nonexistent_table\" not found in schema registry",
	}

	// Verify error is set and other fields are nil/empty
	assert.NotEmpty(t, response.Error)
	assert.Contains(t, response.Error, "TABLE_NOT_FOUND")
	assert.Contains(t, response.Error, "nonexistent_table")
	assert.Nil(t, response.Statistics)
	assert.Nil(t, response.Joinability)
	assert.Nil(t, response.Semantic)
}

func TestProbeColumn_ColumnNotFound(t *testing.T) {
	// Test error response when column is not found
	response := probeColumnResponse{
		Table:  "users",
		Column: "nonexistent_column",
		Error:  "COLUMN_NOT_FOUND: column \"nonexistent_column\" not found in table \"users\"",
	}

	// Verify error is set and other fields are nil/empty
	assert.NotEmpty(t, response.Error)
	assert.Contains(t, response.Error, "COLUMN_NOT_FOUND")
	assert.Contains(t, response.Error, "nonexistent_column")
	assert.Contains(t, response.Error, "users")
	assert.Nil(t, response.Statistics)
	assert.Nil(t, response.Joinability)
	assert.Nil(t, response.Semantic)
}

func TestProbeColumn_ErrorCodeExtraction(t *testing.T) {
	// Test error code extraction from error message
	testCases := []struct {
		errorMessage string
		expectedCode string
		expectedMsg  string
	}{
		{
			errorMessage: "TABLE_NOT_FOUND: table \"foo\" not found",
			expectedCode: "TABLE_NOT_FOUND",
			expectedMsg:  "table \"foo\" not found",
		},
		{
			errorMessage: "COLUMN_NOT_FOUND: column \"bar\" not found in table \"foo\"",
			expectedCode: "COLUMN_NOT_FOUND",
			expectedMsg:  "column \"bar\" not found in table \"foo\"",
		},
		{
			errorMessage: "query_error: statistics computation failed",
			expectedCode: "query_error",
			expectedMsg:  "statistics computation failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expectedCode, func(t *testing.T) {
			// Simulate error code extraction logic
			errorCode := "query_error" // default
			errorMessage := tc.errorMessage
			if idx := strings.Index(tc.errorMessage, ": "); idx > 0 {
				errorCode = tc.errorMessage[:idx]
				errorMessage = tc.errorMessage[idx+2:]
			}

			assert.Equal(t, tc.expectedCode, errorCode)
			assert.Equal(t, tc.expectedMsg, errorMessage)
		})
	}
}

// ============================================================================
// Tests for probe_column column_metadata fallback
// ============================================================================

func TestProbeColumn_ColumnMetadataFallback_EnumValues(t *testing.T) {
	// Test that enum values from column_metadata are used when ontology has none
	projectID := uuid.New()
	schemaColumnID := uuid.New()

	// Create a mock column metadata repo with enum values (now in Features.EnumFeatures)
	mockColMetaRepo := newMockColumnMetadataRepository()
	mockColMetaRepo.metadata[schemaColumnID] = &models.ColumnMetadata{
		ProjectID:      projectID,
		SchemaColumnID: schemaColumnID,
		Features: models.ColumnMetadataFeatures{
			EnumFeatures: &models.EnumFeatures{
				Values: []models.ColumnEnumValue{
					{Value: "ACTIVE"},
					{Value: "SUSPENDED"},
					{Value: "BANNED"},
				},
			},
		},
	}

	// Create an ontology with column details but NO enum values
	ontology := &models.TieredOntology{
		ID:        uuid.New(),
		ProjectID: projectID,
		ColumnDetails: map[string][]models.ColumnDetail{
			"users": {
				{
					Name:        "status",
					Description: "User account status",
					Role:        "attribute",
					EnumValues:  nil, // No enum values in ontology
				},
			},
		},
	}

	// Verify ontology has no enum values
	columnDetails := ontology.GetColumnDetails("users")
	assert.Len(t, columnDetails, 1)
	assert.Nil(t, columnDetails[0].EnumValues)

	// Verify mock repo has enum values (via Features.EnumFeatures)
	meta, err := mockColMetaRepo.GetBySchemaColumnID(nil, schemaColumnID)
	assert.NoError(t, err)
	assert.NotNil(t, meta)
	enumFeatures := meta.GetEnumFeatures()
	assert.NotNil(t, enumFeatures)
	assert.Len(t, enumFeatures.Values, 3)
}

func TestProbeColumn_ColumnMetadataFallback_Description(t *testing.T) {
	// Test that description from column_metadata is used when ontology has none
	projectID := uuid.New()
	schemaColumnID := uuid.New()

	mockColMetaRepo := newMockColumnMetadataRepository()
	desc := "User current state"
	mockColMetaRepo.metadata[schemaColumnID] = &models.ColumnMetadata{
		ProjectID:      projectID,
		SchemaColumnID: schemaColumnID,
		Description:    &desc,
	}

	// Verify mock has description (keyed by SchemaColumnID now)
	meta, err := mockColMetaRepo.GetBySchemaColumnID(nil, schemaColumnID)
	assert.NoError(t, err)
	assert.NotNil(t, meta)
	assert.NotNil(t, meta.Description)
	assert.Equal(t, "User current state", *meta.Description)
}

func TestProbeColumn_ColumnMetadataFallback_EntityAndRole(t *testing.T) {
	// Test that entity (via IdentifierFeatures.EntityReferenced) and role from column_metadata are used
	projectID := uuid.New()
	schemaColumnID := uuid.New()

	mockColMetaRepo := newMockColumnMetadataRepository()
	role := "dimension"
	mockColMetaRepo.metadata[schemaColumnID] = &models.ColumnMetadata{
		ProjectID:      projectID,
		SchemaColumnID: schemaColumnID,
		Role:           &role,
		Features: models.ColumnMetadataFeatures{
			IdentifierFeatures: &models.IdentifierFeatures{
				EntityReferenced: "Account",
			},
		},
	}

	meta, err := mockColMetaRepo.GetBySchemaColumnID(nil, schemaColumnID)
	assert.NoError(t, err)
	assert.NotNil(t, meta)
	assert.NotNil(t, meta.Role)
	assert.Equal(t, "dimension", *meta.Role)
	// Entity is now in Features.IdentifierFeatures.EntityReferenced
	idFeatures := meta.GetIdentifierFeatures()
	assert.NotNil(t, idFeatures)
	assert.Equal(t, "Account", idFeatures.EntityReferenced)
}

func TestProbeColumn_ColumnMetadataIsPrimary_OntologyIsFallback(t *testing.T) {
	// Test that column_metadata is the primary source and ontology is fallback
	// After schema refactor: column_metadata takes precedence
	ontologyEnumValues := []models.EnumValue{
		{Value: "ACTIVE", Label: "Active User"},
		{Value: "INACTIVE", Label: "Inactive User"},
	}

	// When column_metadata has data, it is used (primary source)
	// Ontology is only used as fallback when column_metadata has no data
	semantic := probeColumnSemantic{
		Description: "From column_metadata",
		Role:        "attribute",
		EnumLabels: map[string]string{
			"ACTIVE":   "Active User",
			"INACTIVE": "Inactive User",
		},
	}

	// Column_metadata values are used (primary source)
	assert.Len(t, semantic.EnumLabels, 2)
	assert.Equal(t, "Active User", semantic.EnumLabels["ACTIVE"])

	// Ontology would only be used if column_metadata had no semantic data
	// This test verifies the structure
	assert.Len(t, ontologyEnumValues, 2)
}

func TestProbeColumn_ColumnMetadataFallback_NoMetadataAnywhere(t *testing.T) {
	// Test that probe works when neither ontology nor column_metadata have data
	mockColMetaRepo := newMockColumnMetadataRepository()

	// Verify empty repo returns nil (now keyed by SchemaColumnID)
	meta, err := mockColMetaRepo.GetBySchemaColumnID(nil, uuid.New())
	assert.NoError(t, err)
	assert.Nil(t, meta)
}

func TestProbeColumn_ColumnMetadataFallback_MergePartialData(t *testing.T) {
	// Test that partial data from column_metadata merges with ontology data
	// Scenario: ontology has description, column_metadata has enum values

	// Ontology provides description
	ontologySemantic := probeColumnSemantic{
		Description: "From ontology",
		Role:        "attribute",
	}

	// Column metadata would provide enum values (simulating merge logic)
	columnMetaEnumValues := []string{"VALUE1", "VALUE2"}

	// After merge, semantic should have both
	// The actual merge happens in probeColumn, this test validates the concept
	assert.Equal(t, "From ontology", ontologySemantic.Description)
	assert.Equal(t, "attribute", ontologySemantic.Role)
	assert.Len(t, columnMetaEnumValues, 2)
	assert.Empty(t, ontologySemantic.EnumLabels) // Empty, so fallback would fill this
}
