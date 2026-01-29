package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ============================================================================
// Pattern Detection Tests
// ============================================================================

func TestDetectPatternsInSamples(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name          string
		sampleValues  []string
		wantPattern   string
		wantMinRate   float64
		wantAbsent    []string // patterns that should NOT be detected
	}{
		{
			name: "UUID pattern",
			sampleValues: []string{
				"550e8400-e29b-41d4-a716-446655440000",
				"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
				"f47ac10b-58cc-4372-a567-0e02b2c3d479",
			},
			wantPattern: models.PatternUUID,
			wantMinRate: 1.0,
		},
		{
			name: "Stripe IDs",
			sampleValues: []string{
				"cus_ABC123def",
				"pi_123456789",
				"ch_abcdefghij",
				"sub_testsubscription",
			},
			wantPattern: models.PatternStripeID,
			wantMinRate: 1.0,
		},
		{
			name: "Twilio SIDs",
			sampleValues: []string{
				"AC12345678901234567890123456789012",
				"SM12345678901234567890123456789012",
			},
			wantPattern: models.PatternTwilioSID,
			wantMinRate: 1.0,
		},
		{
			name: "ISO 4217 currency codes",
			sampleValues: []string{
				"USD",
				"EUR",
				"GBP",
				"JPY",
			},
			wantPattern: models.PatternISO4217,
			wantMinRate: 1.0,
		},
		{
			name: "Unix seconds timestamps",
			sampleValues: []string{
				"1704067200", // 2024-01-01
				"1735689600", // 2025-01-01
				"1609459200", // 2021-01-01
			},
			wantPattern: models.PatternUnixSeconds,
			wantMinRate: 1.0,
		},
		{
			name: "Unix milliseconds timestamps",
			sampleValues: []string{
				"1704067200000",
				"1735689600000",
				"1609459200000",
			},
			wantPattern: models.PatternUnixMillis,
			wantMinRate: 1.0,
		},
		{
			name: "Unix nanoseconds timestamps",
			sampleValues: []string{
				"1704067200000000000",
				"1735689600000000000",
			},
			wantPattern: models.PatternUnixNanos,
			wantMinRate: 1.0,
		},
		{
			name: "Email addresses",
			sampleValues: []string{
				"user@example.com",
				"test.user@company.org",
				"admin@sub.domain.io",
			},
			wantPattern: models.PatternEmail,
			wantMinRate: 1.0,
		},
		{
			name: "URLs",
			sampleValues: []string{
				"https://example.com",
				"http://test.org/path",
				"https://api.service.io/v1/endpoint",
			},
			wantPattern: models.PatternURL,
			wantMinRate: 1.0,
		},
		{
			name: "Mixed - mostly UUIDs",
			sampleValues: []string{
				"550e8400-e29b-41d4-a716-446655440000",
				"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
				"not-a-uuid",
			},
			wantPattern: models.PatternUUID,
			wantMinRate: 0.65, // 2/3
		},
		{
			name:         "Empty samples",
			sampleValues: []string{},
			wantPattern:  "", // no pattern expected
			wantMinRate:  0,
		},
		{
			name: "Random strings - no pattern match",
			sampleValues: []string{
				"hello",
				"world",
				"testing",
			},
			wantPattern: "",
			wantAbsent:  []string{models.PatternUUID, models.PatternStripeID, models.PatternEmail},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patterns := svc.detectPatternsInSamples(tt.sampleValues)

			if tt.wantPattern == "" {
				// Expect no pattern or specific patterns absent
				for _, absent := range tt.wantAbsent {
					for _, p := range patterns {
						if p.PatternName == absent {
							t.Errorf("Pattern %s should not be detected", absent)
						}
					}
				}
				return
			}

			// Find the expected pattern
			found := false
			for _, p := range patterns {
				if p.PatternName == tt.wantPattern {
					found = true
					if p.MatchRate < tt.wantMinRate {
						t.Errorf("Pattern %s match rate = %v, want >= %v", tt.wantPattern, p.MatchRate, tt.wantMinRate)
					}
					break
				}
			}

			if !found {
				t.Errorf("Expected pattern %s not found in detected patterns", tt.wantPattern)
			}
		})
	}
}

// ============================================================================
// Classification Path Routing Tests
// ============================================================================

func TestRouteToClassificationPath(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name     string
		profile  *models.ColumnDataProfile
		wantPath models.ClassificationPath
	}{
		// Timestamp type routing
		{
			name: "timestamp type",
			profile: &models.ColumnDataProfile{
				DataType: "timestamp",
			},
			wantPath: models.ClassificationPathTimestamp,
		},
		{
			name: "timestamptz type",
			profile: &models.ColumnDataProfile{
				DataType: "timestamptz",
			},
			wantPath: models.ClassificationPathTimestamp,
		},
		{
			name: "timestamp with time zone",
			profile: &models.ColumnDataProfile{
				DataType: "timestamp with time zone",
			},
			wantPath: models.ClassificationPathTimestamp,
		},
		{
			name: "date type",
			profile: &models.ColumnDataProfile{
				DataType: "date",
			},
			wantPath: models.ClassificationPathTimestamp,
		},
		{
			name: "datetime type",
			profile: &models.ColumnDataProfile{
				DataType: "datetime",
			},
			wantPath: models.ClassificationPathTimestamp,
		},

		// Boolean type routing
		{
			name: "boolean type",
			profile: &models.ColumnDataProfile{
				DataType: "boolean",
			},
			wantPath: models.ClassificationPathBoolean,
		},
		{
			name: "bool type",
			profile: &models.ColumnDataProfile{
				DataType: "bool",
			},
			wantPath: models.ClassificationPathBoolean,
		},

		// UUID type routing
		{
			name: "uuid type",
			profile: &models.ColumnDataProfile{
				DataType: "uuid",
			},
			wantPath: models.ClassificationPathUUID,
		},

		// JSON type routing
		{
			name: "json type",
			profile: &models.ColumnDataProfile{
				DataType: "json",
			},
			wantPath: models.ClassificationPathJSON,
		},
		{
			name: "jsonb type",
			profile: &models.ColumnDataProfile{
				DataType: "jsonb",
			},
			wantPath: models.ClassificationPathJSON,
		},

		// Integer type routing
		{
			name: "integer - high cardinality numeric",
			profile: &models.ColumnDataProfile{
				DataType:      "integer",
				RowCount:      1000,
				DistinctCount: 900,
				Cardinality:   0.9,
				SampleValues:  []string{"123", "456", "789", "101112"},
			},
			wantPath: models.ClassificationPathNumeric,
		},
		{
			name: "bigint - high cardinality",
			profile: &models.ColumnDataProfile{
				DataType:      "bigint",
				RowCount:      1000,
				DistinctCount: 500,
				Cardinality:   0.5,
				SampleValues:  []string{"1000000", "2000000", "3000000"},
			},
			wantPath: models.ClassificationPathNumeric,
		},
		{
			name: "integer - boolean values (0,1)",
			profile: &models.ColumnDataProfile{
				DataType:      "integer",
				RowCount:      1000,
				DistinctCount: 2,
				Cardinality:   0.002,
				SampleValues:  []string{"0", "1"},
			},
			wantPath: models.ClassificationPathBoolean,
		},
		{
			name: "bigint - unix seconds timestamp",
			profile: &models.ColumnDataProfile{
				DataType:      "bigint",
				RowCount:      1000,
				DistinctCount: 800,
				Cardinality:   0.8,
				SampleValues:  []string{"1704067200", "1735689600", "1609459200"},
				DetectedPatterns: []models.DetectedPattern{
					{PatternName: models.PatternUnixSeconds, MatchRate: 1.0, MatchedValues: []string{"1704067200", "1735689600", "1609459200"}},
				},
			},
			wantPath: models.ClassificationPathTimestamp,
		},
		{
			name: "integer - low cardinality enum",
			profile: &models.ColumnDataProfile{
				DataType:      "integer",
				RowCount:      10000,
				DistinctCount: 5,
				Cardinality:   0.0005, // Less than 1%
				SampleValues:  []string{"1", "2", "3", "4", "5"},
			},
			wantPath: models.ClassificationPathEnum,
		},

		// Text type routing
		{
			name: "text - high cardinality",
			profile: &models.ColumnDataProfile{
				DataType:      "text",
				RowCount:      1000,
				DistinctCount: 900,
				Cardinality:   0.9,
				SampleValues:  []string{"hello world", "foo bar", "baz qux"},
			},
			wantPath: models.ClassificationPathText,
		},
		{
			name: "varchar - UUID pattern in data",
			profile: &models.ColumnDataProfile{
				DataType:      "varchar(36)",
				RowCount:      1000,
				DistinctCount: 900,
				Cardinality:   0.9,
				SampleValues: []string{
					"550e8400-e29b-41d4-a716-446655440000",
					"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
				},
				DetectedPatterns: []models.DetectedPattern{
					{PatternName: models.PatternUUID, MatchRate: 1.0},
				},
			},
			wantPath: models.ClassificationPathUUID,
		},
		{
			name: "text - Stripe ID pattern",
			profile: &models.ColumnDataProfile{
				DataType:      "text",
				RowCount:      1000,
				DistinctCount: 900,
				Cardinality:   0.9,
				SampleValues:  []string{"cus_ABC123", "cus_DEF456"},
				DetectedPatterns: []models.DetectedPattern{
					{PatternName: models.PatternStripeID, MatchRate: 1.0},
				},
			},
			wantPath: models.ClassificationPathExternalID,
		},
		{
			name: "varchar - low cardinality enum",
			profile: &models.ColumnDataProfile{
				DataType:      "varchar(20)",
				RowCount:      10000,
				DistinctCount: 10,
				Cardinality:   0.001, // Less than 1%
				SampleValues:  []string{"pending", "approved", "rejected"},
			},
			wantPath: models.ClassificationPathEnum,
		},

		// Unknown type
		{
			name: "unknown type",
			profile: &models.ColumnDataProfile{
				DataType: "bytea",
			},
			wantPath: models.ClassificationPathUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath := svc.routeToClassificationPath(tt.profile)
			if gotPath != tt.wantPath {
				t.Errorf("routeToClassificationPath() = %v, want %v", gotPath, tt.wantPath)
			}
		})
	}
}

// ============================================================================
// Unix Timestamp Validation Tests
// ============================================================================

func TestValidateUnixTimestamps(t *testing.T) {
	tests := []struct {
		name        string
		values      []string
		patternName string
		want        bool
	}{
		{
			name:        "valid unix seconds",
			values:      []string{"1704067200", "1735689600", "1609459200"},
			patternName: models.PatternUnixSeconds,
			want:        true,
		},
		{
			name:        "valid unix milliseconds",
			values:      []string{"1704067200000", "1735689600000"},
			patternName: models.PatternUnixMillis,
			want:        true,
		},
		{
			name:        "valid unix nanoseconds",
			values:      []string{"1704067200000000000"},
			patternName: models.PatternUnixNanos,
			want:        true,
		},
		{
			name:        "timestamps before 1970 - invalid",
			values:      []string{"-1000000000"},
			patternName: models.PatternUnixSeconds,
			want:        false,
		},
		{
			name:        "timestamps after 2100 - invalid",
			values:      []string{"5000000000"},
			patternName: models.PatternUnixSeconds,
			want:        false, // Year 2128
		},
		{
			name:        "empty values",
			values:      []string{},
			patternName: models.PatternUnixSeconds,
			want:        false,
		},
		{
			name:        "non-numeric values",
			values:      []string{"abc", "def"},
			patternName: models.PatternUnixSeconds,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateUnixTimestamps(tt.values, tt.patternName)
			if got != tt.want {
				t.Errorf("validateUnixTimestamps() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// External ID Pattern Detection Tests
// ============================================================================

func TestMatchesExternalIDPattern(t *testing.T) {
	tests := []struct {
		name    string
		profile *models.ColumnDataProfile
		want    bool
	}{
		{
			name: "Stripe ID with high match rate",
			profile: &models.ColumnDataProfile{
				DetectedPatterns: []models.DetectedPattern{
					{PatternName: models.PatternStripeID, MatchRate: 0.95},
				},
			},
			want: true,
		},
		{
			name: "Twilio SID with high match rate",
			profile: &models.ColumnDataProfile{
				DetectedPatterns: []models.DetectedPattern{
					{PatternName: models.PatternTwilioSID, MatchRate: 0.85},
				},
			},
			want: true,
		},
		{
			name: "AWS SES with exact threshold",
			profile: &models.ColumnDataProfile{
				DetectedPatterns: []models.DetectedPattern{
					{PatternName: models.PatternAWSSES, MatchRate: 0.80},
				},
			},
			want: true,
		},
		{
			name: "Stripe ID below threshold",
			profile: &models.ColumnDataProfile{
				DetectedPatterns: []models.DetectedPattern{
					{PatternName: models.PatternStripeID, MatchRate: 0.70},
				},
			},
			want: false,
		},
		{
			name: "UUID pattern (not external ID)",
			profile: &models.ColumnDataProfile{
				DetectedPatterns: []models.DetectedPattern{
					{PatternName: models.PatternUUID, MatchRate: 1.0},
				},
			},
			want: false,
		},
		{
			name: "No patterns detected",
			profile: &models.ColumnDataProfile{
				DetectedPatterns: nil,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesExternalIDPattern(tt.profile)
			if got != tt.want {
				t.Errorf("matchesExternalIDPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Build Column Profile Tests
// ============================================================================

func TestBuildColumnProfile(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	tableID := uuid.New()
	columnID := uuid.New()
	projectID := uuid.New()

	distinctCount := int64(100)
	nullCount := int64(10)
	minLength := int64(5)
	maxLength := int64(50)
	rowCount := int64(1000)

	tableNameByID := map[uuid.UUID]string{
		tableID: "users",
	}
	tableRowCountByID := map[uuid.UUID]int64{
		tableID: rowCount,
	}

	col := &models.SchemaColumn{
		ID:            columnID,
		ProjectID:     projectID,
		SchemaTableID: tableID,
		ColumnName:    "email",
		DataType:      "varchar(255)",
		IsNullable:    true,
		IsPrimaryKey:  false,
		IsUnique:      true,
		DistinctCount: &distinctCount,
		NullCount:     &nullCount,
		MinLength:     &minLength,
		MaxLength:     &maxLength,
		SampleValues:  []string{"user@example.com", "test@test.org"},
	}

	profile := svc.buildColumnProfile(col, tableNameByID, tableRowCountByID)

	// Verify basic fields
	if profile.ColumnID != columnID {
		t.Errorf("ColumnID = %v, want %v", profile.ColumnID, columnID)
	}
	if profile.ColumnName != "email" {
		t.Errorf("ColumnName = %v, want email", profile.ColumnName)
	}
	if profile.TableName != "users" {
		t.Errorf("TableName = %v, want users", profile.TableName)
	}
	if profile.DataType != "varchar(255)" {
		t.Errorf("DataType = %v, want varchar(255)", profile.DataType)
	}
	if !profile.IsNullable {
		t.Error("IsNullable should be true")
	}
	if profile.IsPrimaryKey {
		t.Error("IsPrimaryKey should be false")
	}
	if !profile.IsUnique {
		t.Error("IsUnique should be true")
	}

	// Verify computed fields
	if profile.RowCount != rowCount {
		t.Errorf("RowCount = %v, want %v", profile.RowCount, rowCount)
	}
	if profile.DistinctCount != distinctCount {
		t.Errorf("DistinctCount = %v, want %v", profile.DistinctCount, distinctCount)
	}
	if profile.NullCount != nullCount {
		t.Errorf("NullCount = %v, want %v", profile.NullCount, nullCount)
	}

	// Verify null rate calculation (10/1000 = 0.01)
	expectedNullRate := 0.01
	if profile.NullRate != expectedNullRate {
		t.Errorf("NullRate = %v, want %v", profile.NullRate, expectedNullRate)
	}

	// Verify cardinality calculation (100/1000 = 0.1)
	expectedCardinality := 0.1
	if profile.Cardinality != expectedCardinality {
		t.Errorf("Cardinality = %v, want %v", profile.Cardinality, expectedCardinality)
	}

	// Verify text stats
	if profile.MinLength == nil || *profile.MinLength != minLength {
		t.Errorf("MinLength = %v, want %v", profile.MinLength, minLength)
	}
	if profile.MaxLength == nil || *profile.MaxLength != maxLength {
		t.Errorf("MaxLength = %v, want %v", profile.MaxLength, maxLength)
	}

	// Verify sample values
	if len(profile.SampleValues) != 2 {
		t.Errorf("SampleValues length = %d, want 2", len(profile.SampleValues))
	}
}

// ============================================================================
// Type Detection Helper Tests
// ============================================================================

func TestTypeDetectionHelpers(t *testing.T) {
	t.Run("isUUIDTypeForClassification", func(t *testing.T) {
		tests := []struct {
			dataType string
			want     bool
		}{
			{"uuid", true},
			{"UUID", true},
			{"text", false},
			{"varchar(36)", false},
		}
		for _, tt := range tests {
			if got := isUUIDTypeForClassification(tt.dataType); got != tt.want {
				t.Errorf("isUUIDTypeForClassification(%q) = %v, want %v", tt.dataType, got, tt.want)
			}
		}
	})

	t.Run("isJSONTypeForClassification", func(t *testing.T) {
		tests := []struct {
			dataType string
			want     bool
		}{
			{"json", true},
			{"jsonb", true},
			{"JSON", true},
			{"JSONB", true},
			{"text", false},
		}
		for _, tt := range tests {
			if got := isJSONTypeForClassification(tt.dataType); got != tt.want {
				t.Errorf("isJSONTypeForClassification(%q) = %v, want %v", tt.dataType, got, tt.want)
			}
		}
	})
}

// ============================================================================
// Integration Test with Mock Repository
// ============================================================================

type mockSchemaRepoForFeatureExtraction struct {
	tables  []*models.SchemaTable
	columns []*models.SchemaColumn
}

func (m *mockSchemaRepoForFeatureExtraction) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockSchemaRepoForFeatureExtraction) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return m.columns, nil
}

// Stub implementations for interface
func (m *mockSchemaRepoForFeatureExtraction) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []repositories.TableKey) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID, selectedOnly bool) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string, selectedOnly bool) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64, sampleValues []string) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

func TestRunPhase1DataCollection(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()
	rowCount := int64(1000)

	distinctCount := int64(100)
	nullCount := int64(50)

	mockRepo := &mockSchemaRepoForFeatureExtraction{
		tables: []*models.SchemaTable{
			{
				ID:           tableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				TableName:    "users",
				RowCount:     &rowCount,
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "id",
				DataType:      "uuid",
				IsPrimaryKey:  true,
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "email",
				DataType:      "varchar(255)",
				DistinctCount: &distinctCount,
				NullCount:     &nullCount,
				SampleValues:  []string{"user@example.com", "test@test.org"},
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "created_at",
				DataType:      "timestamp",
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "is_active",
				DataType:      "boolean",
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "settings",
				DataType:      "jsonb",
			},
		},
	}

	svc := &columnFeatureExtractionService{
		schemaRepo: mockRepo,
		logger:     zap.NewNop(),
	}

	// Track progress calls
	var progressCalls []string
	progressCallback := func(current, total int, message string) {
		progressCalls = append(progressCalls, message)
	}

	result, err := svc.runPhase1DataCollection(context.Background(), projectID, datasourceID, progressCallback)
	if err != nil {
		t.Fatalf("runPhase1DataCollection() error = %v", err)
	}

	// Verify result
	if result.TotalColumns != 5 {
		t.Errorf("TotalColumns = %d, want 5", result.TotalColumns)
	}
	if len(result.Profiles) != 5 {
		t.Errorf("len(Profiles) = %d, want 5", len(result.Profiles))
	}
	if len(result.Phase2Queue) != 5 {
		t.Errorf("len(Phase2Queue) = %d, want 5", len(result.Phase2Queue))
	}

	// Verify classification paths
	pathByColumn := make(map[string]models.ClassificationPath)
	for _, p := range result.Profiles {
		pathByColumn[p.ColumnName] = p.ClassificationPath
	}

	expectedPaths := map[string]models.ClassificationPath{
		"id":         models.ClassificationPathUUID,
		"email":      models.ClassificationPathText,
		"created_at": models.ClassificationPathTimestamp,
		"is_active":  models.ClassificationPathBoolean,
		"settings":   models.ClassificationPathJSON,
	}

	for col, wantPath := range expectedPaths {
		if gotPath, ok := pathByColumn[col]; !ok {
			t.Errorf("Column %q not found in profiles", col)
		} else if gotPath != wantPath {
			t.Errorf("Column %q path = %v, want %v", col, gotPath, wantPath)
		}
	}

	// Verify progress was reported
	if len(progressCalls) == 0 {
		t.Error("No progress callbacks received")
	}
}

func TestExtractColumnFeatures_EmptySchema(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockRepo := &mockSchemaRepoForFeatureExtraction{
		tables:  []*models.SchemaTable{},
		columns: []*models.SchemaColumn{},
	}

	svc := NewColumnFeatureExtractionService(mockRepo, zap.NewNop())

	count, err := svc.ExtractColumnFeatures(context.Background(), projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("ExtractColumnFeatures() error = %v", err)
	}
	if count != 0 {
		t.Errorf("ExtractColumnFeatures() count = %d, want 0", count)
	}
}
