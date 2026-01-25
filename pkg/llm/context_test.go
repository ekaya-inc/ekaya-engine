package llm

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestWithContext_MergesValues(t *testing.T) {
	ctx := context.Background()
	workflowID := uuid.New()

	// Add workflow ID
	ctx = WithContext(ctx, map[string]any{
		"workflow_id": workflowID.String(),
	})

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

func TestWithConversationID_SetsID(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	ctx = WithConversationID(ctx, id)

	gotID, ok := GetConversationID(ctx)
	if !ok {
		t.Fatal("expected conversation ID to be present")
	}
	if gotID != id {
		t.Errorf("expected conversation ID %s, got %s", id, gotID)
	}
}

func TestGetConversationID_ReturnsFalseWhenNotSet(t *testing.T) {
	ctx := context.Background()

	_, ok := GetConversationID(ctx)
	if ok {
		t.Error("expected ok to be false when conversation ID not set")
	}
}
