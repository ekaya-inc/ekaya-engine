package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBusinessGlossaryTerm_JSONMarshaling(t *testing.T) {
	// Create a term with all fields populated
	createdBy := uuid.New()
	updatedBy := uuid.New()
	term := BusinessGlossaryTerm{
		ID:          uuid.New(),
		ProjectID:   uuid.New(),
		Term:        "Active Users",
		Definition:  "Users who have engaged with the platform within the last 30 days",
		DefiningSQL: "SELECT COUNT(DISTINCT user_id) AS active_users FROM users WHERE deleted_at IS NULL AND updated_at >= CURRENT_DATE - INTERVAL '30 days'",
		BaseTable:   "users",
		OutputColumns: []OutputColumn{
			{Name: "active_users", Type: "integer", Description: "Count of active users"},
		},
		Aliases:   []string{"MAU", "Monthly Active Users"},
		Source:    GlossarySourceInferred,
		CreatedBy: &createdBy,
		UpdatedBy: &updatedBy,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Test JSON marshaling
	jsonBytes, err := json.Marshal(term)
	if err != nil {
		t.Fatalf("Failed to marshal term to JSON: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled BusinessGlossaryTerm
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal term from JSON: %v", err)
	}

	// Verify key fields
	if unmarshaled.Term != term.Term {
		t.Errorf("Term mismatch: got %s, want %s", unmarshaled.Term, term.Term)
	}
	if unmarshaled.DefiningSQL != term.DefiningSQL {
		t.Errorf("DefiningSQL mismatch")
	}
	if len(unmarshaled.OutputColumns) != len(term.OutputColumns) {
		t.Errorf("OutputColumns length mismatch: got %d, want %d", len(unmarshaled.OutputColumns), len(term.OutputColumns))
	}
	if len(unmarshaled.Aliases) != len(term.Aliases) {
		t.Errorf("Aliases length mismatch: got %d, want %d", len(unmarshaled.Aliases), len(term.Aliases))
	}
}

func TestGlossarySourceConstants(t *testing.T) {
	tests := []struct {
		constant string
		expected string
	}{
		{GlossarySourceInferred, "inferred"},
		{GlossarySourceManual, "manual"},
		{GlossarySourceClient, "client"},
	}

	for _, tt := range tests {
		if tt.constant != tt.expected {
			t.Errorf("Constant mismatch: got %s, want %s", tt.constant, tt.expected)
		}
	}
}

func TestBusinessGlossaryTerm_MinimalFields(t *testing.T) {
	// Test with only required fields (simulating inferred term)
	term := BusinessGlossaryTerm{
		ID:          uuid.New(),
		ProjectID:   uuid.New(),
		Term:        "Revenue",
		Definition:  "Total sales amount",
		DefiningSQL: "SELECT SUM(amount) AS revenue FROM transactions",
		Source:      GlossarySourceInferred,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Should marshal without error even with nil/empty optional fields
	jsonBytes, err := json.Marshal(term)
	if err != nil {
		t.Fatalf("Failed to marshal minimal term to JSON: %v", err)
	}

	// Unmarshal and verify
	var unmarshaled BusinessGlossaryTerm
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal minimal term from JSON: %v", err)
	}

	if unmarshaled.BaseTable != "" {
		t.Errorf("Expected empty BaseTable, got %s", unmarshaled.BaseTable)
	}
	if unmarshaled.OutputColumns != nil && len(unmarshaled.OutputColumns) > 0 {
		t.Errorf("Expected nil/empty OutputColumns, got %v", unmarshaled.OutputColumns)
	}
	if unmarshaled.Aliases != nil && len(unmarshaled.Aliases) > 0 {
		t.Errorf("Expected nil/empty Aliases, got %v", unmarshaled.Aliases)
	}
	if unmarshaled.CreatedBy != nil {
		t.Errorf("Expected nil CreatedBy, got %v", unmarshaled.CreatedBy)
	}
	if unmarshaled.UpdatedBy != nil {
		t.Errorf("Expected nil UpdatedBy, got %v", unmarshaled.UpdatedBy)
	}
}
