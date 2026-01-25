package services

import (
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// TestFilterEntityCandidates_LegacyMode tests filtering with legacy pattern matching enabled.
// In legacy mode, columns with patterns like *_status, is_*, has_*, *_at are excluded.
func TestFilterEntityCandidates_LegacyMode(t *testing.T) {
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
		// Should be excluded: Name pattern (*_status) - only in legacy mode
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

	// Test with legacy pattern matching ENABLED
	candidates, excluded := FilterEntityCandidates(columns, tableByID, statsMap, true, logger)

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

// TestFilterEntityCandidates_DataBasedMode tests filtering with data-based detection
// (UseLegacyPatternMatching=false). In this mode, name patterns like *_status are NOT
// used to exclude columns - only type and data (distinct count, ratio) are considered.
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
		// In legacy mode, this would be excluded by name pattern (*_status)
		// In data-based mode, it should be included if it has high enough stats
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

	// Test with data-based mode (UseLegacyPatternMatching=false)
	candidates, excluded := FilterEntityCandidates(columns, tableByID, statsMap, false, logger)

	// In data-based mode:
	// - "id" is a candidate (PK)
	// - "account_status" is now a candidate (not excluded by name pattern, has good stats)
	// - "is_active" is excluded (boolean type)
	// - "created_at" is excluded (timestamp type)

	// Verify account_status is now a candidate (not excluded by name)
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

// TestFilterEntityCandidates is the original test, kept for backward compatibility.
// It tests the legacy mode (useLegacyPatternMatching=true).
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

	candidates, excluded := FilterEntityCandidates(columns, tableByID, statsMap, true, logger)

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

// TestIsExcludedName tests the legacy pattern-matching exclusion function.
// This function is only used when UseLegacyPatternMatching=true.
// When UseLegacyPatternMatching=false, name-based exclusions are not applied
// and only type-based/data-based filters are used.
func TestIsExcludedName(t *testing.T) {
	tests := []struct {
		columnName string
		expected   bool
	}{
		{"created_at", true},
		{"updated_at", true},
		{"deleted_at", true},
		{"birth_date", true},
		{"is_active", true},
		{"is_deleted", true},
		{"has_password", true},
		{"has_avatar", true},
		{"account_status", true},
		{"user_type", true},
		{"active_flag", true},
		{"user_id", false},
		{"account_id", false},
		{"email", false},
		{"name", false},
		{"id", false},
	}

	for _, tt := range tests {
		t.Run(tt.columnName, func(t *testing.T) {
			result := isExcludedName(tt.columnName)
			if result != tt.expected {
				t.Errorf("isExcludedName(%s) = %v, want %v", tt.columnName, result, tt.expected)
			}
		})
	}
}

func TestIsEntityReferenceName(t *testing.T) {
	tests := []struct {
		columnName string
		expected   bool
	}{
		{"id", true},
		{"user_id", true},
		{"account_id", true},
		{"USER_ID", true},
		{"account_uuid", true},
		{"session_key", true},
		{"email", false},
		{"name", false},
		{"status", false},
		{"created_at", false},
	}

	for _, tt := range tests {
		t.Run(tt.columnName, func(t *testing.T) {
			result := isEntityReferenceName(tt.columnName)
			if result != tt.expected {
				t.Errorf("isEntityReferenceName(%s) = %v, want %v", tt.columnName, result, tt.expected)
			}
		})
	}
}
