package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
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
		name         string
		sampleValues []string
		wantPattern  string
		wantMinRate  float64
		wantAbsent   []string // patterns that should NOT be detected
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

// ============================================================================
// Phase 2: Column Classification Tests
// ============================================================================

func TestRunPhase2ColumnClassification_Success(t *testing.T) {
	projectID := uuid.New()

	// Create mock LLM client that returns valid JSON responses
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		// Return different responses based on what type of classification is being done
		if containsStr(prompt, "Timestamp Column Classification") {
			return &llm.GenerateResponseResult{
				Content: `{"purpose": "audit_created", "confidence": 0.9, "is_soft_delete": false, "is_audit_field": true, "description": "Records when the row was created."}`,
			}, nil
		}
		if containsStr(prompt, "Boolean Column Classification") {
			return &llm.GenerateResponseResult{
				Content: `{"true_meaning": "Active", "false_meaning": "Inactive", "boolean_type": "status_indicator", "confidence": 0.85, "description": "Indicates whether the record is active."}`,
			}, nil
		}
		if containsStr(prompt, "UUID Column Classification") {
			return &llm.GenerateResponseResult{
				Content: `{"identifier_type": "primary_key", "entity_referenced": "", "needs_fk_resolution": false, "confidence": 0.95, "description": "Primary identifier for the record."}`,
			}, nil
		}
		if containsStr(prompt, "JSON Column Classification") {
			return &llm.GenerateResponseResult{
				Content: `{"json_type": "settings", "confidence": 0.8, "description": "User configuration settings."}`,
			}, nil
		}
		if containsStr(prompt, "Text Column Classification") {
			return &llm.GenerateResponseResult{
				Content: `{"text_type": "email", "confidence": 0.9, "description": "User email address."}`,
			}, nil
		}
		// Default response
		return &llm.GenerateResponseResult{
			Content: `{"text_type": "description", "confidence": 0.5, "description": "Generic text field."}`,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient = mockClient

	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), zap.NewNop())

	// Build profiles to classify
	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:           uuid.New(),
			ColumnName:         "created_at",
			TableName:          "users",
			DataType:           "timestamp",
			ClassificationPath: models.ClassificationPathTimestamp,
		},
		{
			ColumnID:           uuid.New(),
			ColumnName:         "is_active",
			TableName:          "users",
			DataType:           "boolean",
			ClassificationPath: models.ClassificationPathBoolean,
		},
		{
			ColumnID:           uuid.New(),
			ColumnName:         "id",
			TableName:          "users",
			DataType:           "uuid",
			IsPrimaryKey:       true,
			ClassificationPath: models.ClassificationPathUUID,
		},
	}

	svc := &columnFeatureExtractionService{
		llmFactory:  mockFactory,
		workerPool:  workerPool,
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	// Track progress
	var progressCalls []int
	progressCallback := func(completed, total int, message string) {
		progressCalls = append(progressCalls, completed)
	}

	result, err := svc.runPhase2ColumnClassification(context.Background(), projectID, profiles, progressCallback)
	if err != nil {
		t.Fatalf("runPhase2ColumnClassification() error = %v", err)
	}

	// Verify results
	if len(result.Features) != 3 {
		t.Errorf("len(Features) = %d, want 3", len(result.Features))
	}

	// Verify progress was reported
	if len(progressCalls) == 0 {
		t.Error("No progress callbacks received")
	}

	// Verify LLM was called for each column
	if mockClient.GenerateResponseCalls != 3 {
		t.Errorf("GenerateResponseCalls = %d, want 3", mockClient.GenerateResponseCalls)
	}

	// Verify classification results
	featuresByColumn := make(map[string]*models.ColumnFeatures)
	for _, f := range result.Features {
		for _, p := range profiles {
			if p.ColumnID == f.ColumnID {
				featuresByColumn[p.ColumnName] = f
				break
			}
		}
	}

	// Check timestamp classification
	if f, ok := featuresByColumn["created_at"]; ok {
		if f.ClassificationPath != models.ClassificationPathTimestamp {
			t.Errorf("created_at ClassificationPath = %v, want timestamp", f.ClassificationPath)
		}
		if f.TimestampFeatures == nil {
			t.Error("created_at TimestampFeatures is nil")
		} else if !f.TimestampFeatures.IsAuditField {
			t.Error("created_at should be marked as audit field")
		}
	} else {
		t.Error("created_at not found in results")
	}

	// Check boolean classification
	if f, ok := featuresByColumn["is_active"]; ok {
		if f.ClassificationPath != models.ClassificationPathBoolean {
			t.Errorf("is_active ClassificationPath = %v, want boolean", f.ClassificationPath)
		}
		if f.BooleanFeatures == nil {
			t.Error("is_active BooleanFeatures is nil")
		} else if f.BooleanFeatures.BooleanType != "status_indicator" {
			t.Errorf("is_active BooleanType = %v, want status_indicator", f.BooleanFeatures.BooleanType)
		}
	} else {
		t.Error("is_active not found in results")
	}

	// Check UUID classification
	if f, ok := featuresByColumn["id"]; ok {
		if f.ClassificationPath != models.ClassificationPathUUID {
			t.Errorf("id ClassificationPath = %v, want uuid", f.ClassificationPath)
		}
		if f.Role != models.RolePrimaryKey {
			t.Errorf("id Role = %v, want primary_key", f.Role)
		}
	} else {
		t.Error("id not found in results")
	}
}

func TestRunPhase2ColumnClassification_EmptyProfiles(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	result, err := svc.runPhase2ColumnClassification(context.Background(), uuid.New(), []*models.ColumnDataProfile{}, nil)
	if err != nil {
		t.Fatalf("runPhase2ColumnClassification() error = %v", err)
	}

	if len(result.Features) != 0 {
		t.Errorf("len(Features) = %d, want 0", len(result.Features))
	}
	if len(result.Phase3EnumQueue) != 0 {
		t.Errorf("len(Phase3EnumQueue) = %d, want 0", len(result.Phase3EnumQueue))
	}
}

func TestRunPhase2ColumnClassification_RequiresLLMSupport(t *testing.T) {
	// Service without LLM support
	svc := &columnFeatureExtractionService{
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:           uuid.New(),
			ColumnName:         "test",
			TableName:          "test_table",
			ClassificationPath: models.ClassificationPathText,
		},
	}

	_, err := svc.runPhase2ColumnClassification(context.Background(), uuid.New(), profiles, nil)
	if err == nil {
		t.Error("Expected error when LLM support is not configured")
	}
	if !containsStr(err.Error(), "LLM support") {
		t.Errorf("Error should mention LLM support, got: %v", err)
	}
}

func TestRunPhase2ColumnClassification_EnqueuesFollowUpWork(t *testing.T) {
	projectID := uuid.New()

	// Create mock LLM client that returns responses that trigger follow-up work
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		if containsStr(prompt, "Enum/Categorical Column Classification") {
			// Return response that needs detailed enum analysis
			return &llm.GenerateResponseResult{
				Content: `{"is_state_machine": true, "state_description": "Order status workflow", "needs_detailed_analysis": true, "confidence": 0.8, "description": "Order processing state."}`,
			}, nil
		}
		if containsStr(prompt, "UUID Column Classification") {
			// Return response that needs FK resolution
			return &llm.GenerateResponseResult{
				Content: `{"identifier_type": "foreign_key", "entity_referenced": "user", "needs_fk_resolution": true, "confidence": 0.7, "description": "References the user."}`,
			}, nil
		}
		if containsStr(prompt, "Numeric Column Classification") {
			// Return response that may be monetary (needs cross-column check)
			return &llm.GenerateResponseResult{
				Content: `{"numeric_type": "monetary", "may_be_monetary": true, "confidence": 0.75, "description": "Transaction amount."}`,
			}, nil
		}
		return &llm.GenerateResponseResult{
			Content: `{"text_type": "description", "confidence": 0.5, "description": "Generic field."}`,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient = mockClient

	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), zap.NewNop())

	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:           uuid.New(),
			ColumnName:         "status",
			TableName:          "orders",
			DataType:           "varchar(20)",
			DistinctCount:      5,
			ClassificationPath: models.ClassificationPathEnum,
		},
		{
			ColumnID:           uuid.New(),
			ColumnName:         "user_id",
			TableName:          "orders",
			DataType:           "uuid",
			ClassificationPath: models.ClassificationPathUUID,
		},
		{
			ColumnID:           uuid.New(),
			ColumnName:         "amount",
			TableName:          "orders",
			DataType:           "numeric",
			ClassificationPath: models.ClassificationPathNumeric,
		},
	}

	svc := &columnFeatureExtractionService{
		llmFactory:  mockFactory,
		workerPool:  workerPool,
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	result, err := svc.runPhase2ColumnClassification(context.Background(), projectID, profiles, nil)
	if err != nil {
		t.Fatalf("runPhase2ColumnClassification() error = %v", err)
	}

	// Verify Phase 3 queue (enum analysis)
	if len(result.Phase3EnumQueue) != 1 {
		t.Errorf("len(Phase3EnumQueue) = %d, want 1", len(result.Phase3EnumQueue))
	}

	// Verify Phase 4 queue (FK resolution)
	if len(result.Phase4FKQueue) != 1 {
		t.Errorf("len(Phase4FKQueue) = %d, want 1", len(result.Phase4FKQueue))
	}

	// Verify Phase 5 queue (cross-column analysis for monetary)
	if len(result.Phase5CrossColumnQueue) != 1 {
		t.Errorf("len(Phase5CrossColumnQueue) = %d, want 1", len(result.Phase5CrossColumnQueue))
	}
	if len(result.Phase5CrossColumnQueue) > 0 && result.Phase5CrossColumnQueue[0] != "orders" {
		t.Errorf("Phase5CrossColumnQueue[0] = %v, want orders", result.Phase5CrossColumnQueue[0])
	}
}

func TestRunPhase2ColumnClassification_ContinuesOnFailure(t *testing.T) {
	projectID := uuid.New()

	callCount := 0
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		callCount++
		// Fail on the second call
		if callCount == 2 {
			return nil, context.DeadlineExceeded
		}
		return &llm.GenerateResponseResult{
			Content: `{"true_meaning": "Yes", "false_meaning": "No", "boolean_type": "state", "confidence": 0.8, "description": "Test."}`,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient = mockClient

	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), zap.NewNop())

	profiles := []*models.ColumnDataProfile{
		{ColumnID: uuid.New(), ColumnName: "col1", TableName: "t1", ClassificationPath: models.ClassificationPathBoolean},
		{ColumnID: uuid.New(), ColumnName: "col2", TableName: "t1", ClassificationPath: models.ClassificationPathBoolean},
		{ColumnID: uuid.New(), ColumnName: "col3", TableName: "t1", ClassificationPath: models.ClassificationPathBoolean},
	}

	svc := &columnFeatureExtractionService{
		llmFactory:  mockFactory,
		workerPool:  workerPool,
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	result, err := svc.runPhase2ColumnClassification(context.Background(), projectID, profiles, nil)
	if err != nil {
		t.Fatalf("runPhase2ColumnClassification() error = %v (should continue on individual failures)", err)
	}

	// Should have 2 successful results (1 failure)
	if len(result.Features) != 2 {
		t.Errorf("len(Features) = %d, want 2 (1 failure)", len(result.Features))
	}
}

func TestGetClassifier_CachesClassifiers(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	// Get classifier twice
	c1 := svc.getClassifier(models.ClassificationPathTimestamp)
	c2 := svc.getClassifier(models.ClassificationPathTimestamp)

	// Should be the same instance (cached)
	if c1 != c2 {
		t.Error("Classifiers should be cached and reused")
	}

	// Different path should get different classifier
	c3 := svc.getClassifier(models.ClassificationPathBoolean)
	if c1 == c3 {
		t.Error("Different paths should get different classifiers")
	}
}

func TestGetClassifier_AllPaths(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	paths := []models.ClassificationPath{
		models.ClassificationPathTimestamp,
		models.ClassificationPathBoolean,
		models.ClassificationPathEnum,
		models.ClassificationPathUUID,
		models.ClassificationPathExternalID,
		models.ClassificationPathNumeric,
		models.ClassificationPathText,
		models.ClassificationPathJSON,
		models.ClassificationPathUnknown,
	}

	for _, path := range paths {
		t.Run(string(path), func(t *testing.T) {
			c := svc.getClassifier(path)
			if c == nil {
				t.Errorf("getClassifier(%v) returned nil", path)
			}
		})
	}
}

func TestUnknownClassifier_NoLLMCall(t *testing.T) {
	projectID := uuid.New()
	profile := &models.ColumnDataProfile{
		ColumnID:           uuid.New(),
		ColumnName:         "binary_data",
		TableName:          "files",
		DataType:           "bytea",
		ClassificationPath: models.ClassificationPathUnknown,
	}

	classifier := &unknownClassifier{logger: zap.NewNop()}

	// Should not need LLM factory
	features, err := classifier.Classify(context.Background(), projectID, profile, nil, nil)
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	if features.ClassificationPath != models.ClassificationPathUnknown {
		t.Errorf("ClassificationPath = %v, want unknown", features.ClassificationPath)
	}
	if features.Confidence != 0.5 {
		t.Errorf("Confidence = %v, want 0.5", features.Confidence)
	}
}

// containsStr is a local helper since strings.Contains is the right function to use
// This is just to avoid name collision with datasource_test.go's contains function
func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

// ============================================================================
// Phase 3: Enum Value Analysis Tests
// ============================================================================

func TestRunPhase3EnumAnalysis_Success(t *testing.T) {
	projectID := uuid.New()
	enumColumnID := uuid.New()

	// Create mock LLM client that returns a valid enum analysis response
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{
				"is_state_machine": true,
				"state_description": "Order processing workflow",
				"values": [
					{"value": "0", "label": "pending", "category": "initial"},
					{"value": "1", "label": "processing", "category": "in_progress"},
					{"value": "2", "label": "completed", "category": "terminal_success"},
					{"value": "3", "label": "failed", "category": "terminal_error"}
				],
				"confidence": 0.9,
				"description": "Tracks order status from creation through fulfillment."
			}`,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient = mockClient

	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), zap.NewNop())

	// Create profiles for the enum column
	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:           enumColumnID,
			ColumnName:         "status",
			TableName:          "orders",
			DataType:           "integer",
			DistinctCount:      4,
			SampleValues:       []string{"0", "1", "2", "3"},
			ClassificationPath: models.ClassificationPathEnum,
		},
	}

	// Create initial features (as would come from Phase 2)
	features := []*models.ColumnFeatures{
		{
			ColumnID:           enumColumnID,
			ClassificationPath: models.ClassificationPathEnum,
			Purpose:            models.PurposeEnum,
			NeedsEnumAnalysis:  true,
			EnumFeatures: &models.EnumFeatures{
				IsStateMachine: true, // Phase 2 initial detection
			},
		},
	}

	svc := &columnFeatureExtractionService{
		llmFactory:  mockFactory,
		workerPool:  workerPool,
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	// Track progress
	var progressCalls []int
	progressCallback := func(completed, total int, message string) {
		progressCalls = append(progressCalls, completed)
	}

	err := svc.runPhase3EnumAnalysis(
		context.Background(),
		projectID,
		[]uuid.UUID{enumColumnID},
		profiles,
		features,
		progressCallback,
	)
	if err != nil {
		t.Fatalf("runPhase3EnumAnalysis() error = %v", err)
	}

	// Verify LLM was called
	if mockClient.GenerateResponseCalls != 1 {
		t.Errorf("GenerateResponseCalls = %d, want 1", mockClient.GenerateResponseCalls)
	}

	// Verify progress was reported
	if len(progressCalls) == 0 {
		t.Error("No progress callbacks received")
	}

	// Verify features were updated
	f := features[0]
	if f.NeedsEnumAnalysis {
		t.Error("NeedsEnumAnalysis should be false after analysis")
	}
	if f.EnumFeatures == nil {
		t.Fatal("EnumFeatures should not be nil")
	}
	if !f.EnumFeatures.IsStateMachine {
		t.Error("IsStateMachine should be true")
	}
	if f.EnumFeatures.StateDescription != "Order processing workflow" {
		t.Errorf("StateDescription = %v, want 'Order processing workflow'", f.EnumFeatures.StateDescription)
	}
	if len(f.EnumFeatures.Values) != 4 {
		t.Errorf("len(Values) = %d, want 4", len(f.EnumFeatures.Values))
	}

	// Verify individual values
	valueLabels := make(map[string]string)
	valueCategories := make(map[string]string)
	for _, v := range f.EnumFeatures.Values {
		valueLabels[v.Value] = v.Label
		valueCategories[v.Value] = v.Category
	}

	expectedLabels := map[string]string{
		"0": "pending",
		"1": "processing",
		"2": "completed",
		"3": "failed",
	}
	for val, wantLabel := range expectedLabels {
		if gotLabel := valueLabels[val]; gotLabel != wantLabel {
			t.Errorf("Value %s label = %v, want %v", val, gotLabel, wantLabel)
		}
	}

	expectedCategories := map[string]string{
		"0": "initial",
		"1": "in_progress",
		"2": "terminal_success",
		"3": "terminal_error",
	}
	for val, wantCat := range expectedCategories {
		if gotCat := valueCategories[val]; gotCat != wantCat {
			t.Errorf("Value %s category = %v, want %v", val, gotCat, wantCat)
		}
	}

	// Verify description was updated
	if f.Description != "Tracks order status from creation through fulfillment." {
		t.Errorf("Description = %v, want 'Tracks order status from creation through fulfillment.'", f.Description)
	}
}

func TestRunPhase3EnumAnalysis_EmptyQueue(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	// Empty enum queue should skip phase without error
	err := svc.runPhase3EnumAnalysis(
		context.Background(),
		uuid.New(),
		[]uuid.UUID{}, // empty queue
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Errorf("runPhase3EnumAnalysis() with empty queue should not error, got: %v", err)
	}
}

func TestRunPhase3EnumAnalysis_ContinuesOnFailure(t *testing.T) {
	projectID := uuid.New()
	col1ID := uuid.New()
	col2ID := uuid.New()
	col3ID := uuid.New()

	callCount := 0
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		callCount++
		// Fail on second call
		if callCount == 2 {
			return nil, context.DeadlineExceeded
		}
		return &llm.GenerateResponseResult{
			Content: `{"is_state_machine": false, "state_description": "", "values": [{"value": "A", "label": "Type A"}], "confidence": 0.8, "description": "Category type."}`,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient = mockClient

	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), zap.NewNop())

	profiles := []*models.ColumnDataProfile{
		{ColumnID: col1ID, ColumnName: "type1", TableName: "t1", ClassificationPath: models.ClassificationPathEnum, SampleValues: []string{"A", "B"}},
		{ColumnID: col2ID, ColumnName: "type2", TableName: "t1", ClassificationPath: models.ClassificationPathEnum, SampleValues: []string{"X", "Y"}},
		{ColumnID: col3ID, ColumnName: "type3", TableName: "t1", ClassificationPath: models.ClassificationPathEnum, SampleValues: []string{"1", "2"}},
	}

	features := []*models.ColumnFeatures{
		{ColumnID: col1ID, ClassificationPath: models.ClassificationPathEnum, NeedsEnumAnalysis: true, EnumFeatures: &models.EnumFeatures{}},
		{ColumnID: col2ID, ClassificationPath: models.ClassificationPathEnum, NeedsEnumAnalysis: true, EnumFeatures: &models.EnumFeatures{}},
		{ColumnID: col3ID, ClassificationPath: models.ClassificationPathEnum, NeedsEnumAnalysis: true, EnumFeatures: &models.EnumFeatures{}},
	}

	svc := &columnFeatureExtractionService{
		llmFactory:  mockFactory,
		workerPool:  workerPool,
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	err := svc.runPhase3EnumAnalysis(
		context.Background(),
		projectID,
		[]uuid.UUID{col1ID, col2ID, col3ID},
		profiles,
		features,
		nil,
	)

	// Should not return error - continues on individual failures
	if err != nil {
		t.Errorf("runPhase3EnumAnalysis() should continue on individual failures, got error: %v", err)
	}

	// All 3 LLM calls should have been attempted
	if callCount != 3 {
		t.Errorf("Expected 3 LLM calls, got %d", callCount)
	}

	// Count successful analyses
	successCount := 0
	for _, f := range features {
		if !f.NeedsEnumAnalysis && len(f.EnumFeatures.Values) > 0 {
			successCount++
		}
	}
	if successCount != 2 {
		t.Errorf("Expected 2 successful analyses, got %d", successCount)
	}
}

func TestRunPhase3EnumAnalysis_NonStateMachine(t *testing.T) {
	projectID := uuid.New()
	enumColumnID := uuid.New()

	// Create mock LLM client that returns a non-state-machine enum
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{
				"is_state_machine": false,
				"state_description": "",
				"values": [
					{"value": "red", "label": "Red Color"},
					{"value": "green", "label": "Green Color"},
					{"value": "blue", "label": "Blue Color"}
				],
				"confidence": 0.95,
				"description": "Color category for the product."
			}`,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient = mockClient

	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), zap.NewNop())

	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:           enumColumnID,
			ColumnName:         "color",
			TableName:          "products",
			DataType:           "varchar(20)",
			DistinctCount:      3,
			SampleValues:       []string{"red", "green", "blue"},
			ClassificationPath: models.ClassificationPathEnum,
		},
	}

	features := []*models.ColumnFeatures{
		{
			ColumnID:           enumColumnID,
			ClassificationPath: models.ClassificationPathEnum,
			Purpose:            models.PurposeEnum,
			NeedsEnumAnalysis:  true,
			EnumFeatures:       &models.EnumFeatures{},
		},
	}

	svc := &columnFeatureExtractionService{
		llmFactory:  mockFactory,
		workerPool:  workerPool,
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	err := svc.runPhase3EnumAnalysis(
		context.Background(),
		projectID,
		[]uuid.UUID{enumColumnID},
		profiles,
		features,
		nil,
	)
	if err != nil {
		t.Fatalf("runPhase3EnumAnalysis() error = %v", err)
	}

	f := features[0]
	if f.EnumFeatures.IsStateMachine {
		t.Error("IsStateMachine should be false for category enum")
	}
	if len(f.EnumFeatures.Values) != 3 {
		t.Errorf("len(Values) = %d, want 3", len(f.EnumFeatures.Values))
	}
	// Non-state-machine enums should not have categories
	for _, v := range f.EnumFeatures.Values {
		if v.Category != "" {
			t.Errorf("Non-state-machine value %s should not have category, got %s", v.Value, v.Category)
		}
	}
}

func TestRunPhase3EnumAnalysis_MissingProfile(t *testing.T) {
	projectID := uuid.New()
	missingColumnID := uuid.New()

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient = llm.NewMockLLMClient()

	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), zap.NewNop())

	// Empty profiles - column ID in queue won't be found
	profiles := []*models.ColumnDataProfile{}
	features := []*models.ColumnFeatures{}

	svc := &columnFeatureExtractionService{
		llmFactory:  mockFactory,
		workerPool:  workerPool,
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	// Should not error, just skip the column
	err := svc.runPhase3EnumAnalysis(
		context.Background(),
		projectID,
		[]uuid.UUID{missingColumnID},
		profiles,
		features,
		nil,
	)
	if err != nil {
		t.Errorf("runPhase3EnumAnalysis() should skip missing profiles, got error: %v", err)
	}
}

func TestBuildEnumAnalysisPrompt(t *testing.T) {
	svc := &columnFeatureExtractionService{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnID:      uuid.New(),
		ColumnName:    "order_status",
		TableName:     "orders",
		DataType:      "integer",
		DistinctCount: 5,
		SampleValues:  []string{"0", "1", "2", "3", "4"},
	}

	prompt := svc.buildEnumAnalysisPrompt(profile)

	// Verify prompt contains expected sections
	expectedContents := []string{
		"# Enum Value Analysis",
		"**Table:** orders",
		"**Column:** order_status",
		"**Data type:** integer",
		"**Distinct values:** 5",
		"**Values found in data:**",
		"- `0`",
		"- `1`",
		"## Task",
		"state machine",
		"## Response Format",
	}

	for _, expected := range expectedContents {
		if !containsStr(prompt, expected) {
			t.Errorf("Prompt should contain %q", expected)
		}
	}
}

func TestMergeEnumAnalysis_UpdatesFeatures(t *testing.T) {
	svc := &columnFeatureExtractionService{logger: zap.NewNop()}

	columnID := uuid.New()
	features := []*models.ColumnFeatures{
		{
			ColumnID:          columnID,
			NeedsEnumAnalysis: true,
			Confidence:        0.5,
			EnumFeatures:      &models.EnumFeatures{},
		},
	}

	result := &EnumAnalysisResult{
		ColumnID:         columnID,
		IsStateMachine:   true,
		StateDescription: "Test workflow",
		Values: []models.ColumnEnumValue{
			{Value: "A", Label: "Alpha", Category: "initial"},
			{Value: "B", Label: "Beta", Category: "terminal"},
		},
		Description: "Test description",
		Confidence:  0.9,
	}

	svc.mergeEnumAnalysis(features, result)

	f := features[0]
	if f.NeedsEnumAnalysis {
		t.Error("NeedsEnumAnalysis should be false after merge")
	}
	if !f.EnumFeatures.IsStateMachine {
		t.Error("IsStateMachine should be true")
	}
	if f.EnumFeatures.StateDescription != "Test workflow" {
		t.Errorf("StateDescription = %v, want 'Test workflow'", f.EnumFeatures.StateDescription)
	}
	if len(f.EnumFeatures.Values) != 2 {
		t.Errorf("len(Values) = %d, want 2", len(f.EnumFeatures.Values))
	}
	if f.Description != "Test description" {
		t.Errorf("Description = %v, want 'Test description'", f.Description)
	}
	if f.Confidence != 0.9 {
		t.Errorf("Confidence = %v, want 0.9", f.Confidence)
	}
}

func TestMergeEnumAnalysis_CreatesEnumFeaturesIfNil(t *testing.T) {
	svc := &columnFeatureExtractionService{logger: zap.NewNop()}

	columnID := uuid.New()
	features := []*models.ColumnFeatures{
		{
			ColumnID:          columnID,
			NeedsEnumAnalysis: true,
			EnumFeatures:      nil, // nil initially
		},
	}

	result := &EnumAnalysisResult{
		ColumnID:       columnID,
		IsStateMachine: false,
		Values:         []models.ColumnEnumValue{{Value: "X", Label: "X Label"}},
		Confidence:     0.8,
	}

	svc.mergeEnumAnalysis(features, result)

	if features[0].EnumFeatures == nil {
		t.Error("EnumFeatures should be created if nil")
	}
	if len(features[0].EnumFeatures.Values) != 1 {
		t.Errorf("len(Values) = %d, want 1", len(features[0].EnumFeatures.Values))
	}
}

func TestMergeEnumAnalysis_DoesNotLowerConfidence(t *testing.T) {
	svc := &columnFeatureExtractionService{logger: zap.NewNop()}

	columnID := uuid.New()
	features := []*models.ColumnFeatures{
		{
			ColumnID:          columnID,
			NeedsEnumAnalysis: true,
			Confidence:        0.95, // Higher initial confidence
			EnumFeatures:      &models.EnumFeatures{},
		},
	}

	result := &EnumAnalysisResult{
		ColumnID:   columnID,
		Confidence: 0.7, // Lower confidence from enum analysis
	}

	svc.mergeEnumAnalysis(features, result)

	// Confidence should NOT be lowered
	if features[0].Confidence != 0.95 {
		t.Errorf("Confidence = %v, should remain 0.95", features[0].Confidence)
	}
}
