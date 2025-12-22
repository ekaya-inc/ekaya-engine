package llm

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestWithWorkflowID_AddsIDToContext(t *testing.T) {
	ctx := context.Background()
	workflowID := uuid.New()

	newCtx := WithWorkflowID(ctx, workflowID)

	// Verify original context is not modified
	if GetWorkflowID(ctx) != nil {
		t.Error("original context should not have workflow ID")
	}

	// Verify new context has the workflow ID
	gotID := GetWorkflowID(newCtx)
	if gotID == nil {
		t.Fatal("expected workflow ID in context")
	}
	if *gotID != workflowID {
		t.Errorf("expected workflow ID %s, got %s", workflowID, *gotID)
	}
}

func TestGetWorkflowID_ReturnsNilForEmptyContext(t *testing.T) {
	ctx := context.Background()

	gotID := GetWorkflowID(ctx)

	if gotID != nil {
		t.Errorf("expected nil for empty context, got %s", *gotID)
	}
}

func TestGetWorkflowID_ReturnsNilForWrongType(t *testing.T) {
	// Put an invalid workflow_id in context
	ctx := WithContext(context.Background(), map[string]any{
		"workflow_id": "not-a-valid-uuid",
	})

	gotID := GetWorkflowID(ctx)

	if gotID != nil {
		t.Error("expected nil when workflow_id is not a valid UUID")
	}
}

func TestWithWorkflowID_CanBeOverwritten(t *testing.T) {
	ctx := context.Background()
	firstID := uuid.New()
	secondID := uuid.New()

	ctx = WithWorkflowID(ctx, firstID)
	ctx = WithWorkflowID(ctx, secondID)

	gotID := GetWorkflowID(ctx)
	if gotID == nil {
		t.Fatal("expected workflow ID in context")
	}
	if *gotID != secondID {
		t.Errorf("expected second workflow ID %s, got %s", secondID, *gotID)
	}
}

func TestWithContext_MergesValues(t *testing.T) {
	ctx := context.Background()
	workflowID := uuid.New()

	// Add workflow ID
	ctx = WithWorkflowID(ctx, workflowID)

	// Add task name (should merge, not replace)
	ctx = WithContext(ctx, map[string]any{
		"task_name": "Analyze users",
	})

	// Both values should exist
	c := GetContext(ctx)
	if c == nil {
		t.Fatal("expected context to exist")
	}
	if c["workflow_id"] != workflowID.String() {
		t.Errorf("expected workflow_id %s, got %v", workflowID, c["workflow_id"])
	}
	if c["task_name"] != "Analyze users" {
		t.Errorf("expected task_name 'Analyze users', got %v", c["task_name"])
	}
}

func TestWithTaskContext_AddsAllFields(t *testing.T) {
	ctx := context.Background()
	workflowID := uuid.New()

	ctx = WithTaskContext(ctx, workflowID, "task-123", "Analyze users", "users")

	c := GetContext(ctx)
	if c == nil {
		t.Fatal("expected context to exist")
	}
	if c["workflow_id"] != workflowID.String() {
		t.Errorf("expected workflow_id %s, got %v", workflowID, c["workflow_id"])
	}
	if c["task_id"] != "task-123" {
		t.Errorf("expected task_id 'task-123', got %v", c["task_id"])
	}
	if c["task_name"] != "Analyze users" {
		t.Errorf("expected task_name 'Analyze users', got %v", c["task_name"])
	}
	if c["entity_name"] != "users" {
		t.Errorf("expected entity_name 'users', got %v", c["entity_name"])
	}
}

func TestGetContext_ReturnsNilForEmptyContext(t *testing.T) {
	ctx := context.Background()

	c := GetContext(ctx)

	if c != nil {
		t.Errorf("expected nil for empty context, got %v", c)
	}
}

func TestGetContext_ReturnsCopy(t *testing.T) {
	ctx := WithContext(context.Background(), map[string]any{
		"key": "value",
	})

	c := GetContext(ctx)
	c["key"] = "modified"

	// Original should not be modified
	c2 := GetContext(ctx)
	if c2["key"] != "value" {
		t.Errorf("expected original value 'value', got %v", c2["key"])
	}
}
