package services

import (
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// --- determineWorkFromStates Tests ---

func TestOrchestrator_DetermineWorkFromStates_PendingTableEntities(t *testing.T) {
	states := []*models.WorkflowEntityState{
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "orders", Status: models.WorkflowEntityStatusPending},
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "customers", Status: models.WorkflowEntityStatusPending},
	}

	orch := &Orchestrator{}
	items := orch.determineWorkFromStates(states)

	if len(items.ScanTasks) != 2 {
		t.Errorf("expected 2 scan tasks, got %d", len(items.ScanTasks))
	}
	if len(items.AnalyzeTasks) != 0 {
		t.Errorf("expected 0 analyze tasks, got %d", len(items.AnalyzeTasks))
	}

	// Verify entity keys are captured
	expectedKeys := map[string]bool{"orders": true, "customers": true}
	for _, key := range items.ScanTasks {
		if !expectedKeys[key] {
			t.Errorf("unexpected scan task key: %s", key)
		}
	}
}

func TestOrchestrator_DetermineWorkFromStates_ScannedTableEntities(t *testing.T) {
	states := []*models.WorkflowEntityState{
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "orders", Status: models.WorkflowEntityStatusScanned},
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "customers", Status: models.WorkflowEntityStatusScanned},
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "products", Status: models.WorkflowEntityStatusScanned},
	}

	orch := &Orchestrator{}
	items := orch.determineWorkFromStates(states)

	if len(items.ScanTasks) != 0 {
		t.Errorf("expected 0 scan tasks, got %d", len(items.ScanTasks))
	}
	if len(items.AnalyzeTasks) != 3 {
		t.Errorf("expected 3 analyze tasks, got %d", len(items.AnalyzeTasks))
	}

	// Verify entity keys are captured
	expectedKeys := map[string]bool{"orders": true, "customers": true, "products": true}
	for _, key := range items.AnalyzeTasks {
		if !expectedKeys[key] {
			t.Errorf("unexpected analyze task key: %s", key)
		}
	}
}

func TestOrchestrator_DetermineWorkFromStates_SkipsGlobalAndColumnEntities(t *testing.T) {
	states := []*models.WorkflowEntityState{
		// Global entity - should be skipped
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeGlobal, EntityKey: "", Status: models.WorkflowEntityStatusPending},
		// Column entities - should be skipped
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeColumn, EntityKey: "orders.status", Status: models.WorkflowEntityStatusPending},
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeColumn, EntityKey: "orders.total", Status: models.WorkflowEntityStatusScanned},
		// Table entity - should be included
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "orders", Status: models.WorkflowEntityStatusPending},
	}

	orch := &Orchestrator{}
	items := orch.determineWorkFromStates(states)

	// Only the table entity should generate a task
	if len(items.ScanTasks) != 1 {
		t.Errorf("expected 1 scan task (table only), got %d", len(items.ScanTasks))
	}
	if len(items.AnalyzeTasks) != 0 {
		t.Errorf("expected 0 analyze tasks, got %d", len(items.AnalyzeTasks))
	}

	if items.ScanTasks[0] != "orders" {
		t.Errorf("expected scan task 'orders', got %s", items.ScanTasks[0])
	}
}

func TestOrchestrator_DetermineWorkFromStates_MixedStates(t *testing.T) {
	states := []*models.WorkflowEntityState{
		// Should be in ScanTasks
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "orders", Status: models.WorkflowEntityStatusPending},
		// Should be in AnalyzeTasks
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "customers", Status: models.WorkflowEntityStatusScanned},
		// Already scanning - no action
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "products", Status: models.WorkflowEntityStatusScanning},
		// Already analyzing - no action
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "items", Status: models.WorkflowEntityStatusAnalyzing},
		// Waiting for input - no action
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "categories", Status: models.WorkflowEntityStatusNeedsInput},
		// Complete - no action
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "users", Status: models.WorkflowEntityStatusComplete},
		// Failed - no action
		{ID: uuid.New(), EntityType: models.WorkflowEntityTypeTable, EntityKey: "logs", Status: models.WorkflowEntityStatusFailed},
	}

	orch := &Orchestrator{}
	items := orch.determineWorkFromStates(states)

	if len(items.ScanTasks) != 1 {
		t.Errorf("expected 1 scan task, got %d", len(items.ScanTasks))
	}
	if len(items.AnalyzeTasks) != 1 {
		t.Errorf("expected 1 analyze task, got %d", len(items.AnalyzeTasks))
	}

	if items.ScanTasks[0] != "orders" {
		t.Errorf("expected scan task 'orders', got %s", items.ScanTasks[0])
	}
	if items.AnalyzeTasks[0] != "customers" {
		t.Errorf("expected analyze task 'customers', got %s", items.AnalyzeTasks[0])
	}
}

// --- isAllComplete Tests ---

func TestOrchestrator_IsAllComplete_TerminalStates(t *testing.T) {
	states := []*models.WorkflowEntityState{
		{Status: models.WorkflowEntityStatusComplete},
		{Status: models.WorkflowEntityStatusFailed},
		{Status: models.WorkflowEntityStatusComplete},
	}

	orch := &Orchestrator{}
	if !orch.isAllComplete(states) {
		t.Error("expected isAllComplete to return true for all terminal states")
	}
}

func TestOrchestrator_IsAllComplete_NonTerminalStates(t *testing.T) {
	testCases := []struct {
		name   string
		states []*models.WorkflowEntityState
	}{
		{
			name: "has pending",
			states: []*models.WorkflowEntityState{
				{Status: models.WorkflowEntityStatusComplete},
				{Status: models.WorkflowEntityStatusPending},
			},
		},
		{
			name: "has scanning",
			states: []*models.WorkflowEntityState{
				{Status: models.WorkflowEntityStatusScanning},
			},
		},
		{
			name: "has scanned",
			states: []*models.WorkflowEntityState{
				{Status: models.WorkflowEntityStatusScanned},
			},
		},
		{
			name: "has analyzing",
			states: []*models.WorkflowEntityState{
				{Status: models.WorkflowEntityStatusAnalyzing},
			},
		},
		{
			name: "has needs_input",
			states: []*models.WorkflowEntityState{
				{Status: models.WorkflowEntityStatusNeedsInput},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			orch := &Orchestrator{}
			if orch.isAllComplete(tc.states) {
				t.Error("expected isAllComplete to return false for non-terminal states")
			}
		})
	}
}

func TestOrchestrator_IsAllComplete_EmptyStates(t *testing.T) {
	orch := &Orchestrator{}
	// Empty list means all entities (none) are complete
	if !orch.isAllComplete([]*models.WorkflowEntityState{}) {
		t.Error("expected isAllComplete to return true for empty states")
	}
}

// --- allTablesComplete Tests ---

func TestOrchestrator_AllTablesComplete_AllComplete(t *testing.T) {
	states := []*models.WorkflowEntityState{
		{EntityType: models.WorkflowEntityTypeTable, Status: models.WorkflowEntityStatusComplete},
		{EntityType: models.WorkflowEntityTypeTable, Status: models.WorkflowEntityStatusComplete},
		// Column entities are ignored
		{EntityType: models.WorkflowEntityTypeColumn, Status: models.WorkflowEntityStatusScanned},
		// Global entity is ignored
		{EntityType: models.WorkflowEntityTypeGlobal, Status: models.WorkflowEntityStatusPending},
	}

	orch := &Orchestrator{}
	if !orch.allTablesComplete(states) {
		t.Error("expected allTablesComplete to return true when all tables are complete")
	}
}

func TestOrchestrator_AllTablesComplete_OneNotComplete(t *testing.T) {
	states := []*models.WorkflowEntityState{
		{EntityType: models.WorkflowEntityTypeTable, Status: models.WorkflowEntityStatusComplete},
		{EntityType: models.WorkflowEntityTypeTable, Status: models.WorkflowEntityStatusAnalyzing}, // Not complete
	}

	orch := &Orchestrator{}
	if orch.allTablesComplete(states) {
		t.Error("expected allTablesComplete to return false when one table is not complete")
	}
}

func TestOrchestrator_AllTablesComplete_NoTables(t *testing.T) {
	states := []*models.WorkflowEntityState{
		{EntityType: models.WorkflowEntityTypeGlobal, Status: models.WorkflowEntityStatusPending},
		{EntityType: models.WorkflowEntityTypeColumn, Status: models.WorkflowEntityStatusScanned},
	}

	orch := &Orchestrator{}
	// No tables means all tables (none) are complete
	if !orch.allTablesComplete(states) {
		t.Error("expected allTablesComplete to return true when there are no tables")
	}
}
