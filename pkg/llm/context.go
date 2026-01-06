package llm

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	llmContextKey contextKey = "llm_context"
)

// WithContext returns a context with LLM recording context attached.
// The context map is merged with any existing context.
func WithContext(ctx context.Context, values map[string]any) context.Context {
	existing := GetContext(ctx)
	if existing == nil {
		existing = make(map[string]any)
	}
	// Merge new values into existing
	for k, v := range values {
		existing[k] = v
	}
	return context.WithValue(ctx, llmContextKey, existing)
}

// GetContext retrieves the LLM recording context from context, if present.
func GetContext(ctx context.Context) map[string]any {
	if c, ok := ctx.Value(llmContextKey).(map[string]any); ok {
		// Return a copy to prevent mutation
		copy := make(map[string]any, len(c))
		for k, v := range c {
			copy[k] = v
		}
		return copy
	}
	return nil
}

// WithTaskContext is a convenience function to add ontology task context.
func WithTaskContext(ctx context.Context, workflowID uuid.UUID, taskID, taskName, entityName string) context.Context {
	values := map[string]any{
		"workflow_id": workflowID.String(),
	}
	if taskID != "" {
		values["task_id"] = taskID
	}
	if taskName != "" {
		values["task_name"] = taskName
	}
	if entityName != "" {
		values["entity_name"] = entityName
	}
	return WithContext(ctx, values)
}
