package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

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
