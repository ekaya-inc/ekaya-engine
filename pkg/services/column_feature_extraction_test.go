package services

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
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

	// Sample values are no longer stored on SchemaColumn; buildColumnProfile won't populate them
	if len(profile.SampleValues) != 0 {
		t.Errorf("SampleValues length = %d, want 0 (sample values are no longer stored on SchemaColumn)", len(profile.SampleValues))
	}
}

func TestBuildColumnProfile_NullRateFromNonNullCount(t *testing.T) {
	// This test verifies the fix for the NullCount bug.
	// In production, adapters populate NonNullCount but not NullCount.
	// The code must calculate NullCount = RowCount - NonNullCount.
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	tableID := uuid.New()
	rowCount := int64(100)
	nonNullCount := int64(5) // 95 nulls = 95% null rate

	tableNameByID := map[uuid.UUID]string{tableID: "users"}
	tableRowCountByID := map[uuid.UUID]int64{tableID: rowCount}

	col := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: tableID,
		ColumnName:    "deleted_at",
		DataType:      "timestamp with time zone",
		IsNullable:    true,
		NonNullCount:  &nonNullCount,
		NullCount:     nil, // Simulates production: never populated
	}

	profile := svc.buildColumnProfile(col, tableNameByID, tableRowCountByID)

	// Verify NullCount was calculated
	expectedNullCount := int64(95)
	if profile.NullCount != expectedNullCount {
		t.Errorf("NullCount = %d, want %d", profile.NullCount, expectedNullCount)
	}

	// Verify NullRate was calculated correctly
	expectedNullRate := 0.95
	if profile.NullRate != expectedNullRate {
		t.Errorf("NullRate = %f, want %f", profile.NullRate, expectedNullRate)
	}
}

func TestBuildColumnProfile_NullCountPreferred(t *testing.T) {
	// If NullCount IS populated (future-proofing), prefer it over calculation
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	tableID := uuid.New()
	rowCount := int64(100)
	nullCount := int64(80)
	nonNullCount := int64(20)

	tableNameByID := map[uuid.UUID]string{tableID: "users"}
	tableRowCountByID := map[uuid.UUID]int64{tableID: rowCount}

	col := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: tableID,
		ColumnName:    "optional_field",
		DataType:      "text",
		IsNullable:    true,
		NullCount:     &nullCount,    // Explicitly set
		NonNullCount:  &nonNullCount, // Also present, but should be ignored
	}

	profile := svc.buildColumnProfile(col, tableNameByID, tableRowCountByID)

	// Should use NullCount directly, not calculate from NonNullCount
	if profile.NullCount != nullCount {
		t.Errorf("NullCount = %d, want %d (should use NullCount directly)", profile.NullCount, nullCount)
	}

	// Verify NullRate uses the explicit NullCount
	expectedNullRate := 0.80
	if profile.NullRate != expectedNullRate {
		t.Errorf("NullRate = %f, want %f", profile.NullRate, expectedNullRate)
	}
}

func TestBuildColumnProfile_ZeroRowCount(t *testing.T) {
	// Edge case: rowCount is 0 - should not divide by zero
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	tableID := uuid.New()
	rowCount := int64(0)
	nonNullCount := int64(0)

	tableNameByID := map[uuid.UUID]string{tableID: "empty_table"}
	tableRowCountByID := map[uuid.UUID]int64{tableID: rowCount}

	col := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: tableID,
		ColumnName:    "some_column",
		DataType:      "text",
		NonNullCount:  &nonNullCount,
		NullCount:     nil,
	}

	profile := svc.buildColumnProfile(col, tableNameByID, tableRowCountByID)

	// NullCount should NOT be calculated when rowCount is 0 (the condition checks rowCount > 0)
	if profile.NullCount != 0 {
		t.Errorf("NullCount = %d, want 0 (should not calculate with rowCount=0)", profile.NullCount)
	}

	// NullRate should be 0 (no division attempted)
	if profile.NullRate != 0 {
		t.Errorf("NullRate = %f, want 0", profile.NullRate)
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

func (m *mockSchemaRepoForFeatureExtraction) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockSchemaRepoForFeatureExtraction) ListAllTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
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
func (m *mockSchemaRepoForFeatureExtraction) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) ListAllColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetTablesByNames(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string]*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetTableCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFeatureExtraction) GetSelectedTableNamesByProject(ctx context.Context, projectID uuid.UUID) ([]string, error) {
	return nil, nil
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
func (m *mockSchemaRepoForFeatureExtraction) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64) error {
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
func (m *mockSchemaRepoForFeatureExtraction) GetRelationshipsByMethod(ctx context.Context, projectID, datasourceID uuid.UUID, method string) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFeatureExtraction) DeleteInferredRelationshipsByProject(ctx context.Context, projectID uuid.UUID) (int64, error) {
	return 0, nil
}

// mockColumnMetadataRepoForFeatureExtraction is a stub ColumnMetadataRepository for tests
// that only need the service to compile (e.g., constructor tests, Phase 1 tests).
type mockColumnMetadataRepoForFeatureExtraction struct{}

func (m *mockColumnMetadataRepoForFeatureExtraction) Upsert(ctx context.Context, meta *models.ColumnMetadata) error {
	return nil
}
func (m *mockColumnMetadataRepoForFeatureExtraction) UpsertFromExtraction(ctx context.Context, meta *models.ColumnMetadata) error {
	return nil
}
func (m *mockColumnMetadataRepoForFeatureExtraction) GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
	return nil, nil
}
func (m *mockColumnMetadataRepoForFeatureExtraction) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}
func (m *mockColumnMetadataRepoForFeatureExtraction) GetBySchemaColumnIDs(ctx context.Context, schemaColumnIDs []uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}
func (m *mockColumnMetadataRepoForFeatureExtraction) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockColumnMetadataRepoForFeatureExtraction) DeleteBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) error {
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
				IsSelected:    true,
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "email",
				DataType:      "varchar(255)",
				DistinctCount: &distinctCount,
				NullCount:     &nullCount,
				IsSelected:    true,
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "created_at",
				DataType:      "timestamp",
				IsSelected:    true,
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "is_active",
				DataType:      "boolean",
				IsSelected:    true,
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "settings",
				DataType:      "jsonb",
				IsSelected:    true,
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

	mockColumnMetadataRepo := &mockColumnMetadataRepoForFeatureExtraction{}
	svc := NewColumnFeatureExtractionService(mockRepo, mockColumnMetadataRepo, zap.NewNop())

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
	if mockClient.GenerateResponseCalls.Load() != 3 {
		t.Errorf("GenerateResponseCalls = %d, want 3", mockClient.GenerateResponseCalls.Load())
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

	var callCount atomic.Int64
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		count := callCount.Add(1)
		// Fail on the second call
		if count == 2 {
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

	// Should return error when LLM calls fail (fail fast)
	if err == nil {
		t.Fatal("runPhase2ColumnClassification() should return error on LLM failures")
	}

	// Should still have 2 successful results in partial output
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

func TestTimestampClassifier_BuildPrompt_SemanticGuidance(t *testing.T) {
	classifier := &timestampClassifier{logger: zap.NewNop()}

	// Test with a deleted_at column that has low non-null rate (2% deleted)
	// This should still be classified as soft_delete based on semantics, not threshold
	profile := &models.ColumnDataProfile{
		ColumnID:   uuid.New(),
		ColumnName: "deleted_at",
		TableName:  "users",
		DataType:   "timestamp with time zone",
		NullRate:   0.98, // 98% null = 2% deleted
		RowCount:   10000,
		IsNullable: true,
	}

	prompt := classifier.buildPrompt(profile)

	// Verify prompt does NOT contain rigid threshold-based rules
	forbiddenPatterns := []string{
		"90-100% NULL",
		"0-5% NULL",
		"5-90% NULL",
		"high null rate", // from old soft_delete description
	}
	for _, forbidden := range forbiddenPatterns {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("Prompt should NOT contain rigid threshold guidance %q", forbidden)
		}
	}

	// Verify prompt contains semantic guidance
	expectedContents := []string{
		"null rate indicates frequency, not purpose",
		"Consider what NULL vs non-NULL means semantically",
		"DATA characteristics should inform your decision",
		"audit_created",
		"audit_updated",
		"soft_delete",
		"event_time",
		"scheduled_time",
		"expiration",
		"cursor",
		// Verify descriptions include semantic meaning
		"NULL = active, non-NULL = deleted", // soft_delete description
		"typically NOT NULL",                // audit field descriptions
	}

	for _, expected := range expectedContents {
		if !strings.Contains(prompt, expected) {
			t.Errorf("Prompt should contain semantic guidance %q", expected)
		}
	}
}

func TestTimestampClassifier_BuildPrompt_NanosecondPrecision(t *testing.T) {
	classifier := &timestampClassifier{logger: zap.NewNop()}

	// Test with a bigint column that has nanosecond-scale timestamps
	// The nanosecond pattern is detected from DetectedPatterns, not raw sample values
	profile := &models.ColumnDataProfile{
		ColumnID:     uuid.New(),
		ColumnName:   "event_ns",
		TableName:    "events",
		DataType:     "bigint",
		NullRate:     0.0,
		RowCount:     1000,
		SampleValues: []string{"1704067200000000000", "1704067200123456789"},
		DetectedPatterns: []models.DetectedPattern{
			{PatternName: models.PatternUnixNanos, MatchRate: 1.0},
		},
	}

	prompt := classifier.buildPrompt(profile)

	// Verify nanosecond precision guidance is included
	if !strings.Contains(prompt, "Nanosecond precision suggests cursor/pagination use") {
		t.Error("Prompt should contain nanosecond precision guidance for high-precision timestamps")
	}
}

func TestTimestampClassifier_NeedsCrossColumnCheck_NullableTimestamps(t *testing.T) {
	// Test that nullable timestamps with mixed null/non-null values are flagged
	// for cross-column validation, regardless of whether they're soft deletes
	tests := []struct {
		name       string
		nullRate   float64
		isNullable bool
		wantFlag   bool
	}{
		{"soft_delete_rare_2pct", 0.98, true, true},        // 2% deleted - flagged
		{"soft_delete_common_50pct", 0.50, true, true},     // 50% deleted - flagged
		{"completed_at_70pct", 0.30, true, true},           // 70% completed - flagged
		{"created_at_required", 0.0, false, false},         // NOT NULL - not flagged
		{"created_at_nullable_no_nulls", 0.0, true, false}, // nullable but no nulls - not flagged
		{"all_null_no_data", 1.0, true, false},             // 100% null (no data yet) - not flagged
		{"mostly_null_99pct", 0.99, true, true},            // 1% non-null - flagged
		{"mostly_non_null_1pct", 0.01, true, true},         // 99% non-null - flagged
	}

	classifier := &timestampClassifier{logger: zap.NewNop()}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &models.ColumnDataProfile{
				ColumnID:   uuid.New(),
				ColumnName: "test_timestamp",
				TableName:  "test_table",
				DataType:   "timestamp with time zone",
				IsNullable: tt.isNullable,
				NullRate:   tt.nullRate,
				RowCount:   10000,
			}

			// Mock LLM response - use a generic response since we're testing
			// the flagging logic, not the LLM classification
			mockResponse := `{
				"purpose": "event_time",
				"confidence": 0.85,
				"is_soft_delete": false,
				"is_audit_field": false,
				"description": "Records when an event occurred."
			}`

			features, err := classifier.parseResponse(profile, mockResponse, "test-model")
			if err != nil {
				t.Fatalf("parseResponse error: %v", err)
			}

			if features.NeedsCrossColumnCheck != tt.wantFlag {
				t.Errorf("NeedsCrossColumnCheck = %v, want %v (nullRate=%.2f, isNullable=%v)",
					features.NeedsCrossColumnCheck, tt.wantFlag, tt.nullRate, tt.isNullable)
			}
		})
	}
}

func TestTimestampClassifier_NeedsCrossColumnCheck_SoftDeleteNotSpecial(t *testing.T) {
	// Verify that IsSoftDelete no longer has special handling -
	// the flag depends on nullability and null rate, not the classification
	classifier := &timestampClassifier{logger: zap.NewNop()}

	// Case: Non-nullable column classified as soft_delete (edge case)
	// Should NOT be flagged because it's not nullable
	profile := &models.ColumnDataProfile{
		ColumnID:   uuid.New(),
		ColumnName: "deleted_at",
		TableName:  "users",
		DataType:   "timestamp with time zone",
		IsNullable: false, // Not nullable
		NullRate:   0.0,
		RowCount:   10000,
	}

	// LLM incorrectly classifies as soft_delete (shouldn't happen, but test the logic)
	mockResponse := `{
		"purpose": "soft_delete",
		"confidence": 0.75,
		"is_soft_delete": true,
		"is_audit_field": false,
		"description": "Records deletion time."
	}`

	features, err := classifier.parseResponse(profile, mockResponse, "test-model")
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}

	// Even though IsSoftDelete=true, non-nullable means no cross-column check needed
	if features.NeedsCrossColumnCheck {
		t.Error("NeedsCrossColumnCheck should be false for non-nullable columns, even if classified as soft_delete")
	}
}

func TestTimestampClassifier_UncertaintyGeneratesQuestion(t *testing.T) {
	// Test that when LLM returns low confidence with needs_clarification=true,
	// the features are flagged for clarification
	classifier := &timestampClassifier{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnID:   uuid.New(),
		ColumnName: "processed_at",
		TableName:  "orders",
		DataType:   "timestamp with time zone",
		IsNullable: true,
		NullRate:   0.5,
		RowCount:   10000,
	}

	// LLM is uncertain and asks a clarification question
	mockResponse := `{
		"purpose": "event_time",
		"confidence": 0.55,
		"is_soft_delete": false,
		"is_audit_field": false,
		"description": "Unclear purpose - could be event time or soft delete indicator",
		"needs_clarification": true,
		"clarification_question": "Is this column used for soft deletes or tracking when an order was processed?"
	}`

	features, err := classifier.parseResponse(profile, mockResponse, "test-model")
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}

	if !features.NeedsClarification {
		t.Error("NeedsClarification should be true when LLM returns needs_clarification=true with low confidence")
	}

	if features.ClarificationQuestion == "" {
		t.Error("ClarificationQuestion should not be empty when LLM provides a question")
	}

	expectedQuestion := "Is this column used for soft deletes or tracking when an order was processed?"
	if features.ClarificationQuestion != expectedQuestion {
		t.Errorf("ClarificationQuestion = %q, want %q", features.ClarificationQuestion, expectedQuestion)
	}

	if features.Confidence >= 0.7 {
		t.Errorf("Confidence should be less than 0.7 for uncertain classifications, got %v", features.Confidence)
	}
}

func TestTimestampClassifier_NoQuestionWhenConfident(t *testing.T) {
	// Test that even if LLM sets needs_clarification=true but confidence >= 0.7,
	// we don't flag for clarification (the condition requires all three)
	classifier := &timestampClassifier{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnID:   uuid.New(),
		ColumnName: "created_at",
		TableName:  "users",
		DataType:   "timestamp with time zone",
		IsNullable: false,
		NullRate:   0.0,
		RowCount:   10000,
	}

	// LLM says needs_clarification but has high confidence (edge case)
	mockResponse := `{
		"purpose": "audit_created",
		"confidence": 0.85,
		"is_soft_delete": false,
		"is_audit_field": true,
		"description": "Records when the record was created",
		"needs_clarification": true,
		"clarification_question": "Some question that shouldn't be used"
	}`

	features, err := classifier.parseResponse(profile, mockResponse, "test-model")
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}

	if features.NeedsClarification {
		t.Error("NeedsClarification should be false when confidence >= 0.7")
	}

	if features.ClarificationQuestion != "" {
		t.Errorf("ClarificationQuestion should be empty when not flagged for clarification, got %q", features.ClarificationQuestion)
	}
}

func TestTimestampClassifier_NoQuestionWhenEmptyQuestion(t *testing.T) {
	// Test that needs_clarification=true requires a non-empty question
	classifier := &timestampClassifier{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnID:   uuid.New(),
		ColumnName: "updated_at",
		TableName:  "users",
		DataType:   "timestamp with time zone",
		IsNullable: true,
		NullRate:   0.5,
		RowCount:   10000,
	}

	// LLM says needs_clarification but provides empty question
	mockResponse := `{
		"purpose": "audit_updated",
		"confidence": 0.55,
		"is_soft_delete": false,
		"is_audit_field": true,
		"description": "Probably an audit field",
		"needs_clarification": true,
		"clarification_question": ""
	}`

	features, err := classifier.parseResponse(profile, mockResponse, "test-model")
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}

	if features.NeedsClarification {
		t.Error("NeedsClarification should be false when clarification_question is empty")
	}
}

func TestTimestampClassifier_PromptIncludesClarificationFields(t *testing.T) {
	// Verify the prompt includes clarification field documentation
	classifier := &timestampClassifier{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnName: "test_timestamp",
		TableName:  "test_table",
		DataType:   "timestamp with time zone",
		NullRate:   0.5,
		RowCount:   1000,
	}

	prompt := classifier.buildPrompt(profile)

	// Check that clarification fields are in the JSON example
	if !containsStr(prompt, "needs_clarification") {
		t.Error("Prompt should include needs_clarification field in JSON example")
	}

	if !containsStr(prompt, "clarification_question") {
		t.Error("Prompt should include clarification_question field in JSON example")
	}

	// Check that the instruction about when to use clarification is present
	if !containsStr(prompt, "confidence < 0.7") {
		t.Error("Prompt should explain when to use clarification fields")
	}
}

// containsStr is a local helper since strings.Contains is the right function to use
// This is just to avoid name collision with datasource_test.go's contains function
func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

// ============================================================================
// Numeric Classifier Tests
// ============================================================================

func TestNumericClassifier_NonPKIdentifierGetsAttributeRole(t *testing.T) {
	// Bug: FK columns like app_id, channel_id were getting role: "primary_key"
	// because the LLM classified them as numeric_type: "identifier" and the code
	// treated any "identifier" as a primary key regardless of IsPrimaryKey flag.
	classifier := &numericClassifier{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnID:     uuid.New(),
		ColumnName:   "app_id",
		TableName:    "content_posts",
		DataType:     "integer",
		IsPrimaryKey: false, // NOT a primary key - it's a FK
	}

	// LLM returns identifier because it looks like an ID column
	mockResponse := `{
		"numeric_type": "identifier",
		"may_be_monetary": false,
		"confidence": 0.90,
		"description": "Reference to applications table."
	}`

	features, err := classifier.parseResponse(profile, mockResponse, "test-model")
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}

	// Should NOT be primary_key since IsPrimaryKey is false
	if features.Role == models.RolePrimaryKey {
		t.Errorf("Role = %q, want anything other than %q for non-PK identifier column",
			features.Role, models.RolePrimaryKey)
	}
}

func TestNumericClassifier_TruePKGetsCorrectRole(t *testing.T) {
	// Actual primary key columns should still get role: "primary_key"
	classifier := &numericClassifier{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnID:     uuid.New(),
		ColumnName:   "id",
		TableName:    "users",
		DataType:     "integer",
		IsPrimaryKey: true, // Actually a primary key
	}

	mockResponse := `{
		"numeric_type": "identifier",
		"may_be_monetary": false,
		"confidence": 0.95,
		"description": "Auto-incrementing user identifier."
	}`

	features, err := classifier.parseResponse(profile, mockResponse, "test-model")
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}

	if features.Role != models.RolePrimaryKey {
		t.Errorf("Role = %q, want %q for actual PK column", features.Role, models.RolePrimaryKey)
	}
}

func TestNumericClassifier_NonPKIdentifierFlaggedForFKResolution(t *testing.T) {
	// Non-PK identifier columns should be flagged for FK resolution in Phase 4
	// so the system can determine if they're foreign keys
	classifier := &numericClassifier{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnID:     uuid.New(),
		ColumnName:   "task_id",
		TableName:    "paid_placements",
		DataType:     "integer",
		IsPrimaryKey: false,
	}

	mockResponse := `{
		"numeric_type": "identifier",
		"may_be_monetary": false,
		"confidence": 0.90,
		"description": "Reference to marketing tasks."
	}`

	features, err := classifier.parseResponse(profile, mockResponse, "test-model")
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}

	if !features.NeedsFKResolution {
		t.Error("NeedsFKResolution should be true for non-PK identifier columns")
	}
}

func TestNumericClassifier_DimensionColumnGetsAttributeRole(t *testing.T) {
	// Integer dimension columns like week_number, day_offset should not be PKs
	classifier := &numericClassifier{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnID:     uuid.New(),
		ColumnName:   "week_number",
		TableName:    "content_posts",
		DataType:     "integer",
		IsPrimaryKey: false,
	}

	// LLM classifies as identifier (incorrectly, but it happens)
	mockResponse := `{
		"numeric_type": "identifier",
		"may_be_monetary": false,
		"confidence": 0.70,
		"description": "Week number within the year."
	}`

	features, err := classifier.parseResponse(profile, mockResponse, "test-model")
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}

	if features.Role == models.RolePrimaryKey {
		t.Errorf("Role = %q, want anything other than %q for dimension column",
			features.Role, models.RolePrimaryKey)
	}
}

func TestNumericClassifier_PromptIncludesMeasureGuidance(t *testing.T) {
	// The prompt should include guidance about aggregation prefixes and
	// business abbreviations so the LLM correctly classifies measure columns
	classifier := &numericClassifier{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnID:   uuid.New(),
		ColumnName: "total_revenue",
		TableName:  "weekly_metrics",
		DataType:   "numeric",
	}

	prompt := classifier.buildPrompt(profile)

	// The measure type description should mention aggregation prefixes as measure indicators
	// (not just the column name appearing in the prompt, but actual guidance text)
	if !strings.Contains(prompt, "avg_") || !strings.Contains(prompt, "total_") {
		t.Error("prompt should include guidance about aggregation prefixes (avg_, total_) as measure indicators")
	}
}

func TestNumericClassifier_PromptMeasureTypeIncludesRatesAndCosts(t *testing.T) {
	// The measure type description should be broad enough to cover cost metrics and rates
	classifier := &numericClassifier{logger: zap.NewNop()}

	profile := &models.ColumnDataProfile{
		ColumnID:   uuid.New(),
		ColumnName: "cpa",
		TableName:  "paid_placements",
		DataType:   "numeric",
	}

	prompt := classifier.buildPrompt(profile)

	// The measure description should include cost-related terms
	if !strings.Contains(prompt, "cost") && !strings.Contains(prompt, "rate") {
		t.Error("prompt measure type should mention cost and rate metrics")
	}
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
	if mockClient.GenerateResponseCalls.Load() != 1 {
		t.Errorf("GenerateResponseCalls = %d, want 1", mockClient.GenerateResponseCalls.Load())
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

	var callCount atomic.Int64
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		count := callCount.Add(1)
		// Fail on second call
		if count == 2 {
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

	// Should return error when LLM calls fail (fail fast)
	if err == nil {
		t.Fatal("runPhase3EnumAnalysis() should return error on LLM failures")
	}

	// All 3 LLM calls should have been attempted
	if callCount.Load() != 3 {
		t.Errorf("Expected 3 LLM calls, got %d", callCount.Load())
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

// ============================================================================
// Phase 4: FK Resolution Tests
// ============================================================================

func TestRunPhase4FKResolution_SkipsWhenEmpty(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	err := svc.runPhase4FKResolution(
		context.Background(),
		uuid.New(),
		uuid.New(),
		[]uuid.UUID{}, // Empty queue
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Errorf("runPhase4FKResolution() error = %v, want nil", err)
	}
}

func TestRunPhase4FKResolution_FallsBackToLLMOnlyWithoutDatasource(t *testing.T) {
	projectID := uuid.New()
	fkColumnID := uuid.New()

	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{"target_table": "users", "target_column": "id", "confidence": 0.8, "reasoning": "Standard FK naming convention."}`,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient = mockClient

	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), zap.NewNop())

	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:           fkColumnID,
			ColumnName:         "user_id",
			TableName:          "orders",
			DataType:           "uuid",
			SampleValues:       []string{"550e8400-e29b-41d4-a716-446655440000"},
			ClassificationPath: models.ClassificationPathUUID,
		},
	}

	features := []*models.ColumnFeatures{
		{
			ColumnID:          fkColumnID,
			Purpose:           models.PurposeIdentifier,
			NeedsFKResolution: true,
			IdentifierFeatures: &models.IdentifierFeatures{
				IdentifierType: models.IdentifierTypeForeignKey,
			},
		},
	}

	// Service WITHOUT datasource dependencies
	svc := &columnFeatureExtractionService{
		llmFactory:  mockFactory,
		workerPool:  workerPool,
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
		// datasourceService and adapterFactory are nil
	}

	err := svc.runPhase4FKResolution(
		context.Background(),
		projectID,
		uuid.New(),
		[]uuid.UUID{fkColumnID},
		profiles,
		features,
		nil,
	)
	if err != nil {
		t.Fatalf("runPhase4FKResolution() error = %v", err)
	}

	// Verify LLM was called (LLM-only fallback)
	if mockClient.GenerateResponseCalls.Load() != 1 {
		t.Errorf("GenerateResponseCalls = %d, want 1", mockClient.GenerateResponseCalls.Load())
	}

	// Verify features were updated
	f := features[0]
	if f.NeedsFKResolution {
		t.Error("NeedsFKResolution should be false after resolution")
	}
	if f.IdentifierFeatures == nil {
		t.Fatal("IdentifierFeatures should not be nil")
	}
	if f.IdentifierFeatures.FKTargetTable != "users" {
		t.Errorf("FKTargetTable = %v, want 'users'", f.IdentifierFeatures.FKTargetTable)
	}
	if f.IdentifierFeatures.FKTargetColumn != "id" {
		t.Errorf("FKTargetColumn = %v, want 'id'", f.IdentifierFeatures.FKTargetColumn)
	}
}

func TestMergeFKResolution(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	columnID := uuid.New()
	features := []*models.ColumnFeatures{
		{
			ColumnID:          columnID,
			Purpose:           models.PurposeIdentifier,
			NeedsFKResolution: true,
			IdentifierFeatures: &models.IdentifierFeatures{
				IdentifierType: models.IdentifierTypeForeignKey,
			},
		},
	}

	result := &FKResolutionResult{
		ColumnID:       columnID,
		FKTargetTable:  "customers",
		FKTargetColumn: "id",
		FKConfidence:   0.92,
		LLMModelUsed:   "claude-3-haiku",
	}

	svc.mergeFKResolution(features, result)

	// Verify update
	f := features[0]
	if f.NeedsFKResolution {
		t.Error("NeedsFKResolution should be false after merge")
	}
	if f.IdentifierFeatures.FKTargetTable != "customers" {
		t.Errorf("FKTargetTable = %v, want 'customers'", f.IdentifierFeatures.FKTargetTable)
	}
	if f.IdentifierFeatures.FKTargetColumn != "id" {
		t.Errorf("FKTargetColumn = %v, want 'id'", f.IdentifierFeatures.FKTargetColumn)
	}
	if f.IdentifierFeatures.FKConfidence != 0.92 {
		t.Errorf("FKConfidence = %v, want 0.92", f.IdentifierFeatures.FKConfidence)
	}
	if f.Role != models.RoleForeignKey {
		t.Errorf("Role = %v, want %v", f.Role, models.RoleForeignKey)
	}
}

func TestMergeFKResolution_CreatesIdentifierFeatures(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	columnID := uuid.New()
	features := []*models.ColumnFeatures{
		{
			ColumnID:           columnID,
			NeedsFKResolution:  true,
			IdentifierFeatures: nil, // No identifier features yet
		},
	}

	result := &FKResolutionResult{
		ColumnID:       columnID,
		FKTargetTable:  "products",
		FKTargetColumn: "id",
		FKConfidence:   0.85,
	}

	svc.mergeFKResolution(features, result)

	// Should create IdentifierFeatures
	if features[0].IdentifierFeatures == nil {
		t.Fatal("IdentifierFeatures should be created")
	}
	if features[0].IdentifierFeatures.FKTargetTable != "products" {
		t.Errorf("FKTargetTable = %v, want 'products'", features[0].IdentifierFeatures.FKTargetTable)
	}
}

func TestMergeFKResolution_NoTargetDoesNotSetRole(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	columnID := uuid.New()
	features := []*models.ColumnFeatures{
		{
			ColumnID:          columnID,
			Role:              models.RoleAttribute, // Original role
			NeedsFKResolution: true,
			IdentifierFeatures: &models.IdentifierFeatures{
				IdentifierType: "internal_uuid",
			},
		},
	}

	// Result with empty target (no FK found)
	result := &FKResolutionResult{
		ColumnID:       columnID,
		FKTargetTable:  "",
		FKTargetColumn: "",
		FKConfidence:   0,
	}

	svc.mergeFKResolution(features, result)

	// Role should remain unchanged (not set to foreign_key)
	if features[0].Role != models.RoleAttribute {
		t.Errorf("Role = %v, want %v (should not change when no target)", features[0].Role, models.RoleAttribute)
	}
}

func TestBuildFKResolutionPrompt(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	profile := &models.ColumnDataProfile{
		ColumnID:      uuid.New(),
		ColumnName:    "customer_id",
		TableName:     "orders",
		DataType:      "uuid",
		DistinctCount: 1000,
		SampleValues:  []string{"abc-123", "def-456"},
	}

	candidates := []phase4FKCandidate{
		{
			Table:          "customers",
			Column:         "id",
			DataType:       "uuid",
			OverlapRate:    0.98,
			MatchedCount:   980,
			TargetDistinct: 5000,
		},
		{
			Table:          "users",
			Column:         "id",
			DataType:       "uuid",
			OverlapRate:    0.65,
			MatchedCount:   650,
			TargetDistinct: 10000,
		},
	}

	prompt := svc.buildFKResolutionPrompt(profile, candidates)

	// Verify prompt contains key information
	if !strings.Contains(prompt, "customer_id") {
		t.Error("Prompt should contain column name")
	}
	if !strings.Contains(prompt, "orders") {
		t.Error("Prompt should contain table name")
	}
	if !strings.Contains(prompt, "customers") {
		t.Error("Prompt should contain candidate table 'customers'")
	}
	if !strings.Contains(prompt, "98.0%") {
		t.Error("Prompt should contain overlap rate percentage")
	}
	if !strings.Contains(prompt, "FK Target Resolution") {
		t.Error("Prompt should have proper header")
	}
}

func TestParseFKResolutionResponse(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	profile := &models.ColumnDataProfile{
		ColumnID:   uuid.New(),
		ColumnName: "user_id",
	}

	candidates := []phase4FKCandidate{
		{Table: "users", Column: "id", OverlapRate: 0.95},
		{Table: "accounts", Column: "id", OverlapRate: 0.70},
	}

	content := `{"target_table": "users", "target_column": "id", "confidence": 0.92, "reasoning": "High overlap and naming match."}`

	result, err := svc.parseFKResolutionResponse(profile, content, "test-model", candidates)
	if err != nil {
		t.Fatalf("parseFKResolutionResponse() error = %v", err)
	}

	if result.ColumnID != profile.ColumnID {
		t.Error("ColumnID should match profile")
	}
	if result.FKTargetTable != "users" {
		t.Errorf("FKTargetTable = %v, want 'users'", result.FKTargetTable)
	}
	if result.FKTargetColumn != "id" {
		t.Errorf("FKTargetColumn = %v, want 'id'", result.FKTargetColumn)
	}
	if result.FKConfidence != 0.92 {
		t.Errorf("FKConfidence = %v, want 0.92", result.FKConfidence)
	}
}

func TestParseFKResolutionResponse_FallsBackToHighestOverlap(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	profile := &models.ColumnDataProfile{
		ColumnID:   uuid.New(),
		ColumnName: "ref_id",
	}

	candidates := []phase4FKCandidate{
		{Table: "items", Column: "id", OverlapRate: 0.95}, // Best overlap
		{Table: "products", Column: "id", OverlapRate: 0.70},
	}

	// LLM response chooses something not in candidates
	content := `{"target_table": "nonexistent", "target_column": "id", "confidence": 0.5, "reasoning": "Guess."}`

	result, err := svc.parseFKResolutionResponse(profile, content, "test-model", candidates)
	if err != nil {
		t.Fatalf("parseFKResolutionResponse() error = %v", err)
	}

	// Should fall back to highest overlap candidate
	if result.FKTargetTable != "items" {
		t.Errorf("FKTargetTable = %v, want 'items' (fallback to highest overlap)", result.FKTargetTable)
	}
}

// ============================================================================
// Phase 5: Cross-Column Analysis Tests
// ============================================================================

func TestRunPhase5CrossColumnAnalysis_Success(t *testing.T) {
	projectID := uuid.New()
	amountColumnID := uuid.New()
	softDeleteColumnID := uuid.New()
	currencyColumnID := uuid.New()

	// Create mock LLM client that returns a valid cross-column analysis response
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{
				"monetary_pairings": [
					{
						"amount_column": "total_amount",
						"currency_column": "currency_code",
						"currency_unit": "cents",
						"amount_description": "Total transaction amount including taxes",
						"confidence": 0.92
					}
				],
				"soft_delete_validations": [
					{
						"column_name": "deleted_at",
						"is_soft_delete": true,
						"non_null_meaning": "Record was soft-deleted at this timestamp",
						"description": "Soft delete marker for logical deletion",
						"confidence": 0.95
					}
				]
			}`,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient = mockClient

	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), zap.NewNop())

	// Create profiles for the table columns
	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:     amountColumnID,
			ColumnName:   "total_amount",
			TableName:    "orders",
			DataType:     "numeric",
			SampleValues: []string{"1000", "2500", "500"},
		},
		{
			ColumnID:     currencyColumnID,
			ColumnName:   "currency_code",
			TableName:    "orders",
			DataType:     "varchar(3)",
			SampleValues: []string{"USD", "EUR", "GBP"},
			DetectedPatterns: []models.DetectedPattern{
				{PatternName: models.PatternISO4217, MatchRate: 1.0},
			},
		},
		{
			ColumnID:     softDeleteColumnID,
			ColumnName:   "deleted_at",
			TableName:    "orders",
			DataType:     "timestamp",
			NullRate:     0.95,
			SampleValues: []string{"2024-01-15 10:30:00"},
		},
	}

	// Create initial features (as would come from Phase 2)
	features := []*models.ColumnFeatures{
		{
			ColumnID:              amountColumnID,
			ClassificationPath:    models.ClassificationPathNumeric,
			Purpose:               models.PurposeMeasure,
			NeedsCrossColumnCheck: true,
			MonetaryFeatures: &models.MonetaryFeatures{
				IsMonetary: false, // Will be confirmed in Phase 5
			},
		},
		{
			ColumnID:           currencyColumnID,
			ClassificationPath: models.ClassificationPathText,
			Purpose:            models.PurposeText,
		},
		{
			ColumnID:              softDeleteColumnID,
			ClassificationPath:    models.ClassificationPathTimestamp,
			Purpose:               models.PurposeTimestamp,
			NeedsCrossColumnCheck: true,
			TimestampFeatures: &models.TimestampFeatures{
				IsSoftDelete: true, // Initial detection from Phase 2
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

	err := svc.runPhase5CrossColumnAnalysis(
		context.Background(),
		projectID,
		[]string{"orders"},
		profiles,
		features,
		progressCallback,
	)
	if err != nil {
		t.Fatalf("runPhase5CrossColumnAnalysis() error = %v", err)
	}

	// Verify LLM was called
	if mockClient.GenerateResponseCalls.Load() != 1 {
		t.Errorf("GenerateResponseCalls = %d, want 1", mockClient.GenerateResponseCalls.Load())
	}

	// Verify progress was reported
	if len(progressCalls) == 0 {
		t.Error("No progress callbacks received")
	}

	// Verify monetary features were updated
	amountFeature := features[0]
	if amountFeature.NeedsCrossColumnCheck {
		t.Error("NeedsCrossColumnCheck should be false after analysis")
	}
	if amountFeature.MonetaryFeatures == nil {
		t.Fatal("MonetaryFeatures should not be nil")
	}
	if !amountFeature.MonetaryFeatures.IsMonetary {
		t.Error("IsMonetary should be true")
	}
	if amountFeature.MonetaryFeatures.CurrencyUnit != "cents" {
		t.Errorf("CurrencyUnit = %v, want 'cents'", amountFeature.MonetaryFeatures.CurrencyUnit)
	}
	if amountFeature.MonetaryFeatures.PairedCurrencyColumn != "currency_code" {
		t.Errorf("PairedCurrencyColumn = %v, want 'currency_code'", amountFeature.MonetaryFeatures.PairedCurrencyColumn)
	}
	if amountFeature.SemanticType != "monetary" {
		t.Errorf("SemanticType = %v, want 'monetary'", amountFeature.SemanticType)
	}

	// Verify soft delete features were updated
	softDeleteFeature := features[2]
	if softDeleteFeature.NeedsCrossColumnCheck {
		t.Error("NeedsCrossColumnCheck should be false after analysis")
	}
	if softDeleteFeature.TimestampFeatures == nil {
		t.Fatal("TimestampFeatures should not be nil")
	}
	if !softDeleteFeature.TimestampFeatures.IsSoftDelete {
		t.Error("IsSoftDelete should be true after validation")
	}
	if softDeleteFeature.Description != "Soft delete marker for logical deletion" {
		t.Errorf("Description = %v, want 'Soft delete marker for logical deletion'", softDeleteFeature.Description)
	}
}

func TestRunPhase5CrossColumnAnalysis_EmptyQueue(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	// Empty table queue should skip phase without error
	err := svc.runPhase5CrossColumnAnalysis(
		context.Background(),
		uuid.New(),
		[]string{}, // empty queue
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Errorf("runPhase5CrossColumnAnalysis() with empty queue should not error, got: %v", err)
	}
}

func TestRunPhase5CrossColumnAnalysis_ContinuesOnFailure(t *testing.T) {
	projectID := uuid.New()

	var callCount atomic.Int64
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		count := callCount.Add(1)
		// Fail on second call
		if count == 2 {
			return nil, context.DeadlineExceeded
		}
		return &llm.GenerateResponseResult{
			Content: `{"monetary_pairings": [], "soft_delete_validations": []}`,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient = mockClient

	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), zap.NewNop())

	// Create profiles for multiple tables
	col1ID := uuid.New()
	col2ID := uuid.New()
	col3ID := uuid.New()
	profiles := []*models.ColumnDataProfile{
		{ColumnID: col1ID, ColumnName: "amount1", TableName: "table1"},
		{ColumnID: col2ID, ColumnName: "amount2", TableName: "table2"},
		{ColumnID: col3ID, ColumnName: "amount3", TableName: "table3"},
	}

	features := []*models.ColumnFeatures{
		{ColumnID: col1ID, NeedsCrossColumnCheck: true, MonetaryFeatures: &models.MonetaryFeatures{}},
		{ColumnID: col2ID, NeedsCrossColumnCheck: true, MonetaryFeatures: &models.MonetaryFeatures{}},
		{ColumnID: col3ID, NeedsCrossColumnCheck: true, MonetaryFeatures: &models.MonetaryFeatures{}},
	}

	svc := &columnFeatureExtractionService{
		llmFactory:  mockFactory,
		workerPool:  workerPool,
		logger:      zap.NewNop(),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}

	err := svc.runPhase5CrossColumnAnalysis(
		context.Background(),
		projectID,
		[]string{"table1", "table2", "table3"},
		profiles,
		features,
		nil,
	)

	// Should return error when LLM calls fail (fail fast)
	if err == nil {
		t.Fatal("runPhase5CrossColumnAnalysis() should return error on LLM failures")
	}

	// All 3 LLM calls should have been attempted
	if callCount.Load() != 3 {
		t.Errorf("Expected 3 LLM calls, got %d", callCount.Load())
	}
}

func TestBuildCrossColumnPrompt(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	amountColumnID := uuid.New()
	softDeleteColumnID := uuid.New()
	currencyColumnID := uuid.New()

	allProfiles := []*models.ColumnDataProfile{
		{ColumnID: amountColumnID, ColumnName: "total_amount", TableName: "orders", DataType: "numeric", NullRate: 0.0, DistinctCount: 500},
		{ColumnID: currencyColumnID, ColumnName: "currency_code", TableName: "orders", DataType: "varchar(3)", NullRate: 0.0, DistinctCount: 5},
		{ColumnID: softDeleteColumnID, ColumnName: "deleted_at", TableName: "orders", DataType: "timestamp", NullRate: 0.95, DistinctCount: 10},
	}

	monetaryColumns := []*models.ColumnDataProfile{
		{ColumnID: amountColumnID, ColumnName: "total_amount", DataType: "numeric", SampleValues: []string{"1000", "2500"}},
	}

	softDeleteColumns := []*models.ColumnDataProfile{
		{ColumnID: softDeleteColumnID, ColumnName: "deleted_at", DataType: "timestamp", NullRate: 0.95, SampleValues: []string{"2024-01-15"}},
	}

	currencyColumns := []*models.ColumnDataProfile{
		{ColumnID: currencyColumnID, ColumnName: "currency_code", DataType: "varchar(3)", SampleValues: []string{"USD", "EUR"}},
	}

	prompt := svc.buildCrossColumnPrompt("orders", allProfiles, monetaryColumns, softDeleteColumns, currencyColumns)

	// Verify prompt contains key sections
	if !strings.Contains(prompt, "Cross-Column Analysis") {
		t.Error("Prompt should contain 'Cross-Column Analysis' header")
	}
	if !strings.Contains(prompt, "**Table:** orders") {
		t.Error("Prompt should contain table name")
	}
	if !strings.Contains(prompt, "Monetary Column Analysis") {
		t.Error("Prompt should contain monetary analysis section")
	}
	if !strings.Contains(prompt, "Soft Delete Validation") {
		t.Error("Prompt should contain soft delete validation section")
	}
	if !strings.Contains(prompt, "total_amount") {
		t.Error("Prompt should contain monetary column name")
	}
	if !strings.Contains(prompt, "deleted_at") {
		t.Error("Prompt should contain soft delete column name")
	}
	if !strings.Contains(prompt, "currency_code") {
		t.Error("Prompt should contain currency column name")
	}
	if !strings.Contains(prompt, "95.0%") {
		t.Error("Prompt should contain null rate for soft delete column")
	}
}

func TestParseCrossColumnResponse(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	amountColumnID := uuid.New()
	softDeleteColumnID := uuid.New()

	monetaryColumns := []*models.ColumnDataProfile{
		{ColumnID: amountColumnID, ColumnName: "amount"},
	}
	softDeleteColumns := []*models.ColumnDataProfile{
		{ColumnID: softDeleteColumnID, ColumnName: "removed_at"},
	}

	content := `{
		"monetary_pairings": [
			{
				"amount_column": "amount",
				"currency_column": "currency",
				"currency_unit": "dollars",
				"amount_description": "Order total",
				"confidence": 0.88
			}
		],
		"soft_delete_validations": [
			{
				"column_name": "removed_at",
				"is_soft_delete": true,
				"non_null_meaning": "Item was removed from cart",
				"description": "Tracks when item was removed",
				"confidence": 0.9
			}
		]
	}`

	result, err := svc.parseCrossColumnResponse("orders", content, "test-model", monetaryColumns, softDeleteColumns)
	if err != nil {
		t.Fatalf("parseCrossColumnResponse() error = %v", err)
	}

	if result.TableName != "orders" {
		t.Errorf("TableName = %v, want 'orders'", result.TableName)
	}
	if result.LLMModelUsed != "test-model" {
		t.Errorf("LLMModelUsed = %v, want 'test-model'", result.LLMModelUsed)
	}

	// Verify monetary pairings
	if len(result.MonetaryPairings) != 1 {
		t.Fatalf("len(MonetaryPairings) = %d, want 1", len(result.MonetaryPairings))
	}
	mp := result.MonetaryPairings[0]
	if mp.AmountColumnID != amountColumnID {
		t.Errorf("AmountColumnID = %v, want %v", mp.AmountColumnID, amountColumnID)
	}
	if mp.AmountColumnName != "amount" {
		t.Errorf("AmountColumnName = %v, want 'amount'", mp.AmountColumnName)
	}
	if mp.CurrencyColumnName != "currency" {
		t.Errorf("CurrencyColumnName = %v, want 'currency'", mp.CurrencyColumnName)
	}
	if mp.CurrencyUnit != "dollars" {
		t.Errorf("CurrencyUnit = %v, want 'dollars'", mp.CurrencyUnit)
	}
	if mp.Confidence != 0.88 {
		t.Errorf("Confidence = %v, want 0.88", mp.Confidence)
	}

	// Verify soft delete validations
	if len(result.SoftDeleteValidations) != 1 {
		t.Fatalf("len(SoftDeleteValidations) = %d, want 1", len(result.SoftDeleteValidations))
	}
	sd := result.SoftDeleteValidations[0]
	if sd.ColumnID != softDeleteColumnID {
		t.Errorf("ColumnID = %v, want %v", sd.ColumnID, softDeleteColumnID)
	}
	if sd.ColumnName != "removed_at" {
		t.Errorf("ColumnName = %v, want 'removed_at'", sd.ColumnName)
	}
	if !sd.IsSoftDelete {
		t.Error("IsSoftDelete should be true")
	}
	if sd.NonNullMeaning != "Item was removed from cart" {
		t.Errorf("NonNullMeaning = %v, want 'Item was removed from cart'", sd.NonNullMeaning)
	}
}

func TestMergeCrossColumnAnalysis(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	amountColumnID := uuid.New()
	softDeleteColumnID := uuid.New()

	features := []*models.ColumnFeatures{
		{
			ColumnID:              amountColumnID,
			ClassificationPath:    models.ClassificationPathNumeric,
			NeedsCrossColumnCheck: true,
			MonetaryFeatures:      &models.MonetaryFeatures{IsMonetary: false},
			Confidence:            0.7,
		},
		{
			ColumnID:              softDeleteColumnID,
			ClassificationPath:    models.ClassificationPathTimestamp,
			NeedsCrossColumnCheck: true,
			TimestampFeatures:     &models.TimestampFeatures{IsSoftDelete: true},
			Confidence:            0.8,
		},
	}

	result := &CrossColumnResult{
		TableName: "orders",
		MonetaryPairings: []MonetaryPairing{
			{
				AmountColumnID:     amountColumnID,
				AmountColumnName:   "total_amount",
				CurrencyColumnName: "currency",
				CurrencyUnit:       "cents",
				AmountDescription:  "Total order amount",
				Confidence:         0.95,
			},
		},
		SoftDeleteValidations: []SoftDeleteValidation{
			{
				ColumnID:       softDeleteColumnID,
				ColumnName:     "deleted_at",
				IsSoftDelete:   true,
				NonNullMeaning: "Record deleted",
				Description:    "Soft delete timestamp",
				Confidence:     0.92,
			},
		},
	}

	svc.mergeCrossColumnAnalysis(features, result)

	// Verify monetary feature updates
	amountFeature := features[0]
	if amountFeature.NeedsCrossColumnCheck {
		t.Error("NeedsCrossColumnCheck should be false after merge")
	}
	if !amountFeature.MonetaryFeatures.IsMonetary {
		t.Error("IsMonetary should be true")
	}
	if amountFeature.MonetaryFeatures.CurrencyUnit != "cents" {
		t.Errorf("CurrencyUnit = %v, want 'cents'", amountFeature.MonetaryFeatures.CurrencyUnit)
	}
	if amountFeature.MonetaryFeatures.PairedCurrencyColumn != "currency" {
		t.Errorf("PairedCurrencyColumn = %v, want 'currency'", amountFeature.MonetaryFeatures.PairedCurrencyColumn)
	}
	if amountFeature.SemanticType != "monetary" {
		t.Errorf("SemanticType = %v, want 'monetary'", amountFeature.SemanticType)
	}
	if amountFeature.Role != models.RoleMeasure {
		t.Errorf("Role = %v, want 'measure'", amountFeature.Role)
	}
	if amountFeature.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95 (higher value from cross-column)", amountFeature.Confidence)
	}

	// Verify soft delete feature updates
	softDeleteFeature := features[1]
	if softDeleteFeature.NeedsCrossColumnCheck {
		t.Error("NeedsCrossColumnCheck should be false after merge")
	}
	if !softDeleteFeature.TimestampFeatures.IsSoftDelete {
		t.Error("IsSoftDelete should be true")
	}
	if softDeleteFeature.SemanticType != models.TimestampPurposeSoftDelete {
		t.Errorf("SemanticType = %v, want '%v'", softDeleteFeature.SemanticType, models.TimestampPurposeSoftDelete)
	}
	if softDeleteFeature.Description != "Soft delete timestamp" {
		t.Errorf("Description = %v, want 'Soft delete timestamp'", softDeleteFeature.Description)
	}
	if softDeleteFeature.Confidence != 0.92 {
		t.Errorf("Confidence = %v, want 0.92 (higher value from cross-column)", softDeleteFeature.Confidence)
	}
}

func TestMergeCrossColumnAnalysis_SoftDeleteRejected(t *testing.T) {
	svc := &columnFeatureExtractionService{
		logger: zap.NewNop(),
	}

	columnID := uuid.New()

	features := []*models.ColumnFeatures{
		{
			ColumnID:              columnID,
			ClassificationPath:    models.ClassificationPathTimestamp,
			NeedsCrossColumnCheck: true,
			TimestampFeatures:     &models.TimestampFeatures{IsSoftDelete: true},
			SemanticType:          "soft_delete",
		},
	}

	// LLM determines this is NOT a soft delete (e.g., it's an optional event timestamp)
	result := &CrossColumnResult{
		TableName: "events",
		SoftDeleteValidations: []SoftDeleteValidation{
			{
				ColumnID:       columnID,
				ColumnName:     "completed_at",
				IsSoftDelete:   false, // Rejected!
				NonNullMeaning: "Task was completed at this time",
				Description:    "Optional completion timestamp",
				Confidence:     0.85,
			},
		},
	}

	svc.mergeCrossColumnAnalysis(features, result)

	// Verify soft delete was rejected
	feature := features[0]
	if feature.NeedsCrossColumnCheck {
		t.Error("NeedsCrossColumnCheck should be false after merge")
	}
	// IsSoftDelete should be false (LLM rejected it)
	if feature.TimestampFeatures.IsSoftDelete {
		t.Error("IsSoftDelete should be false after validation rejected it")
	}
	// SemanticType should NOT be soft_delete anymore
	if feature.SemanticType == models.TimestampPurposeSoftDelete {
		t.Errorf("SemanticType should not be '%v' after rejection", models.TimestampPurposeSoftDelete)
	}
}

// ============================================================================
// Phase 6: Store Results Tests
// ============================================================================

// mockColumnMetadataRepoForStoreFeatures tracks UpsertFromExtraction calls for Phase 6 tests
type mockColumnMetadataRepoForStoreFeatures struct {
	storedMetadata map[uuid.UUID]*models.ColumnMetadata
	failForColumns map[uuid.UUID]bool
}

func (m *mockColumnMetadataRepoForStoreFeatures) Upsert(ctx context.Context, meta *models.ColumnMetadata) error {
	return nil
}
func (m *mockColumnMetadataRepoForStoreFeatures) UpsertFromExtraction(ctx context.Context, meta *models.ColumnMetadata) error {
	if m.failForColumns != nil && m.failForColumns[meta.SchemaColumnID] {
		return fmt.Errorf("simulated failure for column %s", meta.SchemaColumnID)
	}
	m.storedMetadata[meta.SchemaColumnID] = meta
	return nil
}
func (m *mockColumnMetadataRepoForStoreFeatures) GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
	return nil, nil
}
func (m *mockColumnMetadataRepoForStoreFeatures) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}
func (m *mockColumnMetadataRepoForStoreFeatures) GetBySchemaColumnIDs(ctx context.Context, schemaColumnIDs []uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}
func (m *mockColumnMetadataRepoForStoreFeatures) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockColumnMetadataRepoForStoreFeatures) DeleteBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) error {
	return nil
}

func TestStoreFeatures_Success(t *testing.T) {
	// Create mock column metadata repo that tracks upserts
	metadataRepo := &mockColumnMetadataRepoForStoreFeatures{
		storedMetadata: make(map[uuid.UUID]*models.ColumnMetadata),
	}

	svc := &columnFeatureExtractionService{
		columnMetadataRepo: metadataRepo,
		logger:             zap.NewNop(),
	}

	projectID := uuid.New()
	col1ID := uuid.New()
	col2ID := uuid.New()

	features := []*models.ColumnFeatures{
		{
			ColumnID:           col1ID,
			ClassificationPath: models.ClassificationPathTimestamp,
			Purpose:            models.PurposeTimestamp,
			Description:        "Created timestamp",
		},
		{
			ColumnID:           col2ID,
			ClassificationPath: models.ClassificationPathBoolean,
			Purpose:            models.PurposeFlag,
			Description:        "Active flag",
		},
	}

	ctx := context.Background()
	err := svc.storeFeatures(ctx, projectID, features)
	if err != nil {
		t.Fatalf("storeFeatures failed: %v", err)
	}

	// Verify both features were stored
	if len(metadataRepo.storedMetadata) != 2 {
		t.Errorf("Expected 2 stored features, got %d", len(metadataRepo.storedMetadata))
	}

	// Verify the features were stored correctly (ColumnMetadata.Description is a *string)
	if stored, ok := metadataRepo.storedMetadata[col1ID]; !ok {
		t.Error("Feature for col1 not found")
	} else if stored.Description == nil || *stored.Description != "Created timestamp" {
		t.Errorf("Expected description 'Created timestamp', got %v", stored.Description)
	}

	if stored, ok := metadataRepo.storedMetadata[col2ID]; !ok {
		t.Error("Feature for col2 not found")
	} else if stored.Description == nil || *stored.Description != "Active flag" {
		t.Errorf("Expected description 'Active flag', got %v", stored.Description)
	}
}

func TestStoreFeatures_EmptyFeatures(t *testing.T) {
	metadataRepo := &mockColumnMetadataRepoForStoreFeatures{
		storedMetadata: make(map[uuid.UUID]*models.ColumnMetadata),
	}

	svc := &columnFeatureExtractionService{
		columnMetadataRepo: metadataRepo,
		logger:             zap.NewNop(),
	}

	ctx := context.Background()
	err := svc.storeFeatures(ctx, uuid.New(), []*models.ColumnFeatures{})
	if err != nil {
		t.Fatalf("storeFeatures with empty list should not fail: %v", err)
	}

	if len(metadataRepo.storedMetadata) != 0 {
		t.Errorf("Expected 0 stored features, got %d", len(metadataRepo.storedMetadata))
	}
}

func TestStoreFeatures_ContinuesOnError(t *testing.T) {
	col1ID := uuid.New()
	col2ID := uuid.New()
	col3ID := uuid.New()

	metadataRepo := &mockColumnMetadataRepoForStoreFeatures{
		storedMetadata: make(map[uuid.UUID]*models.ColumnMetadata),
		failForColumns: map[uuid.UUID]bool{col2ID: true}, // Fail for col2
	}

	svc := &columnFeatureExtractionService{
		columnMetadataRepo: metadataRepo,
		logger:             zap.NewNop(),
	}

	features := []*models.ColumnFeatures{
		{ColumnID: col1ID, Description: "Col1"},
		{ColumnID: col2ID, Description: "Col2"}, // This will fail
		{ColumnID: col3ID, Description: "Col3"},
	}

	ctx := context.Background()
	err := svc.storeFeatures(ctx, uuid.New(), features)
	// Should not return error since some succeeded
	if err != nil {
		t.Fatalf("storeFeatures should not fail when some columns succeed: %v", err)
	}

	// Verify col1 and col3 were stored, col2 was not
	if _, ok := metadataRepo.storedMetadata[col1ID]; !ok {
		t.Error("Feature for col1 should be stored")
	}
	if _, ok := metadataRepo.storedMetadata[col2ID]; ok {
		t.Error("Feature for col2 should NOT be stored (should have failed)")
	}
	if _, ok := metadataRepo.storedMetadata[col3ID]; !ok {
		t.Error("Feature for col3 should be stored")
	}
}

func TestStoreFeatures_AllFail_ReturnsError(t *testing.T) {
	col1ID := uuid.New()
	col2ID := uuid.New()

	metadataRepo := &mockColumnMetadataRepoForStoreFeatures{
		storedMetadata: make(map[uuid.UUID]*models.ColumnMetadata),
		failForColumns: map[uuid.UUID]bool{col1ID: true, col2ID: true}, // All fail
	}

	svc := &columnFeatureExtractionService{
		columnMetadataRepo: metadataRepo,
		logger:             zap.NewNop(),
	}

	features := []*models.ColumnFeatures{
		{ColumnID: col1ID, Description: "Col1"},
		{ColumnID: col2ID, Description: "Col2"},
	}

	ctx := context.Background()
	err := svc.storeFeatures(ctx, uuid.New(), features)
	// Should return error since all failed
	if err == nil {
		t.Fatal("storeFeatures should fail when all columns fail")
	}
}

func TestStoreFeatures_SkipsNilFeatures(t *testing.T) {
	col1ID := uuid.New()

	metadataRepo := &mockColumnMetadataRepoForStoreFeatures{
		storedMetadata: make(map[uuid.UUID]*models.ColumnMetadata),
	}

	svc := &columnFeatureExtractionService{
		columnMetadataRepo: metadataRepo,
		logger:             zap.NewNop(),
	}

	features := []*models.ColumnFeatures{
		{ColumnID: col1ID, Description: "Col1"},
		nil, // Should be skipped
	}

	ctx := context.Background()
	err := svc.storeFeatures(ctx, uuid.New(), features)
	if err != nil {
		t.Fatalf("storeFeatures should not fail: %v", err)
	}

	if len(metadataRepo.storedMetadata) != 1 {
		t.Errorf("Expected 1 stored feature (nil should be skipped), got %d", len(metadataRepo.storedMetadata))
	}
}

// ============================================================================
// Tests for createQuestionsFromUncertainClassifications
// ============================================================================

// mockOntologyRepoForFeatureExtraction is a minimal mock for testing question creation
type mockOntologyRepoForFeatureExtraction struct {
	ontology *models.TieredOntology
	getErr   error
}

func (m *mockOntologyRepoForFeatureExtraction) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.ontology, nil
}

func (m *mockOntologyRepoForFeatureExtraction) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}
func (m *mockOntologyRepoForFeatureExtraction) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return nil
}
func (m *mockOntologyRepoForFeatureExtraction) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	return nil
}
func (m *mockOntologyRepoForFeatureExtraction) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockOntologyRepoForFeatureExtraction) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

// mockQuestionServiceForFeatureExtraction is a minimal mock for testing question creation
type mockQuestionServiceForFeatureExtraction struct {
	createdQuestions []*models.OntologyQuestion
	createErr        error
}

func (m *mockQuestionServiceForFeatureExtraction) GetNextQuestion(ctx context.Context, projectID uuid.UUID, includeSkipped bool) (*models.OntologyQuestion, error) {
	return nil, nil
}
func (m *mockQuestionServiceForFeatureExtraction) GetPendingQuestions(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error) {
	return nil, nil
}
func (m *mockQuestionServiceForFeatureExtraction) GetPendingCount(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockQuestionServiceForFeatureExtraction) GetPendingCounts(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error) {
	return nil, nil
}
func (m *mockQuestionServiceForFeatureExtraction) AnswerQuestion(ctx context.Context, questionID uuid.UUID, answer string, userID string) (*models.AnswerResult, error) {
	return nil, nil
}
func (m *mockQuestionServiceForFeatureExtraction) SkipQuestion(ctx context.Context, questionID uuid.UUID) error {
	return nil
}
func (m *mockQuestionServiceForFeatureExtraction) DeleteQuestion(ctx context.Context, questionID uuid.UUID) error {
	return nil
}
func (m *mockQuestionServiceForFeatureExtraction) CreateQuestions(ctx context.Context, questions []*models.OntologyQuestion) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.createdQuestions = append(m.createdQuestions, questions...)
	return nil
}

func TestCreateQuestionsFromUncertainClassifications_CreatesQuestions(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()
	columnID := uuid.New()

	ontologyRepo := &mockOntologyRepoForFeatureExtraction{
		ontology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}

	questionService := &mockQuestionServiceForFeatureExtraction{}

	svc := &columnFeatureExtractionService{
		ontologyRepo:    ontologyRepo,
		questionService: questionService,
		logger:          zap.NewNop(),
	}

	features := []*models.ColumnFeatures{
		{
			ColumnID:              columnID,
			NeedsClarification:    true,
			ClarificationQuestion: "Is this column tracking record updates or something else?",
		},
	}

	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:   columnID,
			TableName:  "users",
			ColumnName: "updated_at",
			DataType:   "timestamp with time zone",
			NullRate:   0.05,
		},
	}

	svc.createQuestionsFromUncertainClassifications(context.Background(), projectID, features, profiles)

	if len(questionService.createdQuestions) != 1 {
		t.Fatalf("Expected 1 question to be created, got %d", len(questionService.createdQuestions))
	}

	q := questionService.createdQuestions[0]
	if q.Text != "Is this column tracking record updates or something else?" {
		t.Errorf("Question text = %q, want %q", q.Text, "Is this column tracking record updates or something else?")
	}
	if q.Category != models.QuestionCategoryTerminology {
		t.Errorf("Question category = %q, want %q", q.Category, models.QuestionCategoryTerminology)
	}
	if q.Priority != 3 {
		t.Errorf("Question priority = %d, want 3", q.Priority)
	}
	if !strings.Contains(q.Reasoning, "users.updated_at") {
		t.Errorf("Question context should contain table.column, got: %q", q.Reasoning)
	}
	if !strings.Contains(q.Reasoning, "timestamp with time zone") {
		t.Errorf("Question context should contain data type, got: %q", q.Reasoning)
	}
	if !strings.Contains(q.Reasoning, "5.0%") {
		t.Errorf("Question context should contain null rate, got: %q", q.Reasoning)
	}
}

func TestCreateQuestionsFromUncertainClassifications_SkipsColumnsWithoutClarification(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()
	col1ID := uuid.New()
	col2ID := uuid.New()

	ontologyRepo := &mockOntologyRepoForFeatureExtraction{
		ontology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}

	questionService := &mockQuestionServiceForFeatureExtraction{}

	svc := &columnFeatureExtractionService{
		ontologyRepo:    ontologyRepo,
		questionService: questionService,
		logger:          zap.NewNop(),
	}

	features := []*models.ColumnFeatures{
		{
			ColumnID:           col1ID,
			NeedsClarification: false, // Not flagged for clarification
		},
		{
			ColumnID:              col2ID,
			NeedsClarification:    true,
			ClarificationQuestion: "", // Empty question
		},
	}

	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:   col1ID,
			TableName:  "users",
			ColumnName: "id",
		},
		{
			ColumnID:   col2ID,
			TableName:  "users",
			ColumnName: "created_at",
		},
	}

	svc.createQuestionsFromUncertainClassifications(context.Background(), projectID, features, profiles)

	if len(questionService.createdQuestions) != 0 {
		t.Errorf("Expected 0 questions (all should be skipped), got %d", len(questionService.createdQuestions))
	}
}

func TestCreateQuestionsFromUncertainClassifications_ContinuesOnOntologyNotFound(t *testing.T) {
	projectID := uuid.New()
	columnID := uuid.New()

	ontologyRepo := &mockOntologyRepoForFeatureExtraction{
		ontology: nil, // No active ontology
	}

	questionService := &mockQuestionServiceForFeatureExtraction{}

	svc := &columnFeatureExtractionService{
		ontologyRepo:    ontologyRepo,
		questionService: questionService,
		logger:          zap.NewNop(),
	}

	features := []*models.ColumnFeatures{
		{
			ColumnID:              columnID,
			NeedsClarification:    true,
			ClarificationQuestion: "What does this status mean?",
		},
	}

	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:   columnID,
			TableName:  "users",
			ColumnName: "status",
		},
	}

	// Should not panic
	svc.createQuestionsFromUncertainClassifications(context.Background(), projectID, features, profiles)

	if len(questionService.createdQuestions) != 0 {
		t.Errorf("Expected 0 questions when no ontology, got %d", len(questionService.createdQuestions))
	}
}

func TestCreateQuestionsFromUncertainClassifications_ContinuesOnQuestionServiceError(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()
	columnID := uuid.New()

	ontologyRepo := &mockOntologyRepoForFeatureExtraction{
		ontology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}

	questionService := &mockQuestionServiceForFeatureExtraction{
		createErr: fmt.Errorf("simulated database error"),
	}

	svc := &columnFeatureExtractionService{
		ontologyRepo:    ontologyRepo,
		questionService: questionService,
		logger:          zap.NewNop(),
	}

	features := []*models.ColumnFeatures{
		{
			ColumnID:              columnID,
			NeedsClarification:    true,
			ClarificationQuestion: "What does this status mean?",
		},
	}

	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:   columnID,
			TableName:  "users",
			ColumnName: "status",
		},
	}

	// Should not panic - error is logged but not returned
	svc.createQuestionsFromUncertainClassifications(context.Background(), projectID, features, profiles)
}

func TestCreateQuestionsFromUncertainClassifications_NilDependencies(t *testing.T) {
	projectID := uuid.New()
	columnID := uuid.New()

	svc := &columnFeatureExtractionService{
		ontologyRepo:    nil, // nil dependencies
		questionService: nil,
		logger:          zap.NewNop(),
	}

	features := []*models.ColumnFeatures{
		{
			ColumnID:              columnID,
			NeedsClarification:    true,
			ClarificationQuestion: "What does this status mean?",
		},
	}

	profiles := []*models.ColumnDataProfile{
		{
			ColumnID:   columnID,
			TableName:  "users",
			ColumnName: "status",
		},
	}

	// Should not panic
	svc.createQuestionsFromUncertainClassifications(context.Background(), projectID, features, profiles)
}
