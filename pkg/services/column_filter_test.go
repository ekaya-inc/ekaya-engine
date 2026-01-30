package services

import (
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// TestFilterEntityCandidates tests filtering based on data types, statistics, and column features.
// Name-based pattern matching has been removed; filtering uses stored ColumnFeatures.Purpose.
func TestFilterEntityCandidates_Basic(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	tableID := uuid.New()

	table := &models.SchemaTable{
		ID:         tableID,
		ProjectID:  projectID,
		SchemaName: "public",
		TableName:  "users",
	}

	tableByID := map[string]*models.SchemaTable{
		tableID.String(): table,
	}

	// Create test columns with different characteristics
	columns := []*models.SchemaColumn{
		// Should be candidate: Primary key
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "id",
			DataType:      "uuid",
			IsPrimaryKey:  true,
			IsUnique:      false,
		},
		// Should be candidate: Name pattern (*_id)
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "account_id",
			DataType:      "uuid",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Should be candidate: High distinct count and ratio
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "email",
			DataType:      "text",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Should be excluded: Boolean type
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "is_active",
			DataType:      "boolean",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Should be excluded: Timestamp type
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "created_at",
			DataType:      "timestamp",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Should be excluded: Low distinct count (5 < 20)
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "account_status",
			DataType:      "text",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Should be excluded: Low distinct count
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "role",
			DataType:      "text",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
	}

	// Create stats map
	statsMap := map[string]datasource.ColumnStats{
		"public.users.id": {
			ColumnName:    "id",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 100,
		},
		"public.users.account_id": {
			ColumnName:    "account_id",
			RowCount:      100,
			NonNullCount:  95,
			DistinctCount: 50,
		},
		"public.users.email": {
			ColumnName:    "email",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 98, // High distinct count (98%) and > 20
		},
		"public.users.is_active": {
			ColumnName:    "is_active",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 2,
		},
		"public.users.created_at": {
			ColumnName:    "created_at",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 95,
		},
		"public.users.account_status": {
			ColumnName:    "account_status",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 5,
		},
		"public.users.role": {
			ColumnName:    "role",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 3, // Low distinct count
		},
	}

	// Test filtering based on types and statistics (no legacy pattern matching)
	candidates, excluded := FilterEntityCandidates(columns, tableByID, statsMap, logger)

	// Verify candidates
	if len(candidates) != 3 {
		t.Errorf("Expected 3 candidates, got %d", len(candidates))
		for _, c := range candidates {
			t.Logf("Candidate: %s - %s", c.ColumnName, c.Reason)
		}
	}

	// Verify excluded
	if len(excluded) != 4 {
		t.Errorf("Expected 4 excluded columns, got %d", len(excluded))
		for _, e := range excluded {
			t.Logf("Excluded: %s - %s", e.ColumnName, e.Reason)
		}
	}

	// Verify specific candidates
	candidateNames := make(map[string]bool)
	for _, c := range candidates {
		candidateNames[c.ColumnName] = true
	}

	expectedCandidates := []string{"id", "account_id", "email"}
	for _, name := range expectedCandidates {
		if !candidateNames[name] {
			t.Errorf("Expected %s to be a candidate", name)
		}
	}

	// Verify specific excluded
	excludedNames := make(map[string]bool)
	for _, e := range excluded {
		excludedNames[e.ColumnName] = true
	}

	expectedExcluded := []string{"is_active", "created_at", "account_status", "role"}
	for _, name := range expectedExcluded {
		if !excludedNames[name] {
			t.Errorf("Expected %s to be excluded", name)
		}
	}
}

// TestFilterEntityCandidates_DataBasedMode tests filtering with data-based detection.
// Name patterns like *_status are NOT used to exclude columns -
// only type, data (distinct count, ratio), and stored ColumnFeatures are considered.
func TestFilterEntityCandidates_DataBasedMode(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	tableID := uuid.New()

	table := &models.SchemaTable{
		ID:         tableID,
		ProjectID:  projectID,
		SchemaName: "public",
		TableName:  "products",
	}

	tableByID := map[string]*models.SchemaTable{
		tableID.String(): table,
	}

	columns := []*models.SchemaColumn{
		// Should be candidate: Primary key
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "id",
			DataType:      "uuid",
			IsPrimaryKey:  true,
			IsUnique:      false,
		},
		// Name patterns are not used for exclusion; included if it has high enough stats
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "account_status",
			DataType:      "text",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Boolean type - still excluded (type-based exclusion applies in both modes)
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "is_active",
			DataType:      "boolean",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Timestamp type - still excluded (type-based exclusion applies in both modes)
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "created_at",
			DataType:      "timestamp",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
	}

	// Give account_status good stats so it would be a candidate if not name-filtered
	statsMap := map[string]datasource.ColumnStats{
		"public.products.id": {
			ColumnName:    "id",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 100,
		},
		"public.products.account_status": {
			ColumnName:    "account_status",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 50, // High distinct count (50%) and > 20
		},
		"public.products.is_active": {
			ColumnName:    "is_active",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 2,
		},
		"public.products.created_at": {
			ColumnName:    "created_at",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 95,
		},
	}

	// Test with data-based mode (name patterns not used)
	candidates, excluded := FilterEntityCandidates(columns, tableByID, statsMap, logger)

	// Expected results:
	// - "id" is a candidate (PK)
	// - "account_status" is a candidate (high distinct count, no features to exclude)
	// - "is_active" is excluded (boolean type)
	// - "created_at" is excluded (timestamp type)

	// Verify account_status is a candidate (high distinct count, no features to exclude)
	accountStatusCandidate := false
	for _, c := range candidates {
		if c.ColumnName == "account_status" {
			accountStatusCandidate = true
			break
		}
	}
	if !accountStatusCandidate {
		t.Errorf("Expected 'account_status' to be a candidate in data-based mode, but it was excluded")
		t.Log("Candidates:")
		for _, c := range candidates {
			t.Logf("  %s - %s", c.ColumnName, c.Reason)
		}
		t.Log("Excluded:")
		for _, e := range excluded {
			t.Logf("  %s - %s", e.ColumnName, e.Reason)
		}
	}

	// Verify type-based exclusions still apply (is_active, created_at)
	typeExcluded := make(map[string]bool)
	for _, e := range excluded {
		typeExcluded[e.ColumnName] = true
	}

	if !typeExcluded["is_active"] {
		t.Error("Expected 'is_active' to be excluded by type (boolean) even in data-based mode")
	}
	if !typeExcluded["created_at"] {
		t.Error("Expected 'created_at' to be excluded by type (timestamp) even in data-based mode")
	}
}

// TestFilterEntityCandidates tests basic filtering behavior with type and data constraints.
func TestFilterEntityCandidates(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	tableID := uuid.New()

	table := &models.SchemaTable{
		ID:         tableID,
		ProjectID:  projectID,
		SchemaName: "public",
		TableName:  "users",
	}

	tableByID := map[string]*models.SchemaTable{
		tableID.String(): table,
	}

	// Create test columns with different characteristics
	columns := []*models.SchemaColumn{
		// Should be candidate: Primary key
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "id",
			DataType:      "uuid",
			IsPrimaryKey:  true,
			IsUnique:      false,
		},
		// Should be candidate: Name pattern (*_id)
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "account_id",
			DataType:      "uuid",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Should be candidate: High distinct count and ratio
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "email",
			DataType:      "text",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Should be excluded: Boolean type
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "is_active",
			DataType:      "boolean",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Should be excluded: Timestamp type
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "created_at",
			DataType:      "timestamp",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Should be excluded: Name pattern (*_status)
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "account_status",
			DataType:      "text",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
		// Should be excluded: Low distinct count
		{
			ID:            uuid.New(),
			ProjectID:     projectID,
			SchemaTableID: tableID,
			ColumnName:    "role",
			DataType:      "text",
			IsPrimaryKey:  false,
			IsUnique:      false,
		},
	}

	// Create stats map
	statsMap := map[string]datasource.ColumnStats{
		"public.users.id": {
			ColumnName:    "id",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 100,
		},
		"public.users.account_id": {
			ColumnName:    "account_id",
			RowCount:      100,
			NonNullCount:  95,
			DistinctCount: 50,
		},
		"public.users.email": {
			ColumnName:    "email",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 98, // High distinct count (98%) and > 20
		},
		"public.users.is_active": {
			ColumnName:    "is_active",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 2,
		},
		"public.users.created_at": {
			ColumnName:    "created_at",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 95,
		},
		"public.users.account_status": {
			ColumnName:    "account_status",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 5,
		},
		"public.users.role": {
			ColumnName:    "role",
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 3, // Low distinct count
		},
	}

	candidates, excluded := FilterEntityCandidates(columns, tableByID, statsMap, logger)

	// Verify candidates
	if len(candidates) != 3 {
		t.Errorf("Expected 3 candidates, got %d", len(candidates))
		for _, c := range candidates {
			t.Logf("Candidate: %s - %s", c.ColumnName, c.Reason)
		}
	}

	// Verify excluded
	if len(excluded) != 4 {
		t.Errorf("Expected 4 excluded columns, got %d", len(excluded))
		for _, e := range excluded {
			t.Logf("Excluded: %s - %s", e.ColumnName, e.Reason)
		}
	}

	// Verify specific candidates
	candidateNames := make(map[string]bool)
	for _, c := range candidates {
		candidateNames[c.ColumnName] = true
	}

	expectedCandidates := []string{"id", "account_id", "email"}
	for _, name := range expectedCandidates {
		if !candidateNames[name] {
			t.Errorf("Expected %s to be a candidate", name)
		}
	}

	// Verify specific excluded
	excludedNames := make(map[string]bool)
	for _, e := range excluded {
		excludedNames[e.ColumnName] = true
	}

	expectedExcluded := []string{"is_active", "created_at", "account_status", "role"}
	for _, name := range expectedExcluded {
		if !excludedNames[name] {
			t.Errorf("Expected %s to be excluded", name)
		}
	}
}

func TestIsExcludedType(t *testing.T) {
	tests := []struct {
		dataType string
		expected bool
	}{
		{"boolean", true},
		{"BOOLEAN", true},
		{"timestamp", true},
		{"TIMESTAMP", true},
		{"timestamptz", true},
		{"date", true},
		{"DATE", true},
		{"uuid", false},
		{"text", false},
		{"integer", false},
		{"bigint", false},
	}

	for _, tt := range tests {
		t.Run(tt.dataType, func(t *testing.T) {
			result := isExcludedType(tt.dataType)
			if result != tt.expected {
				t.Errorf("isExcludedType(%s) = %v, want %v", tt.dataType, result, tt.expected)
			}
		})
	}
}

// NOTE: TestIsExcludedName and TestIsEntityReferenceName have been removed.
// Column filtering now uses stored ColumnFeatures.Purpose instead of name-based patterns.
// See PLAN-extracting-column-features.md for details.
