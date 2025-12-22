package models

import (
	"time"

	"github.com/google/uuid"
)

// LLMConversation represents a single LLM API call with verbatim input/output.
type LLMConversation struct {
	ID             uuid.UUID      `json:"id"`
	ProjectID      uuid.UUID      `json:"project_id"`
	Context        map[string]any `json:"context,omitempty"` // Caller-specific context (workflow_id, task_name, etc.)
	ConversationID *uuid.UUID     `json:"conversation_id,omitempty"`
	Iteration      int            `json:"iteration"`

	// Model info
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`

	// Request (VERBATIM)
	RequestMessages []any    `json:"request_messages"`
	RequestTools    []any    `json:"request_tools,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`

	// Response (VERBATIM)
	ResponseContent   string `json:"response_content,omitempty"`
	ResponseToolCalls []any  `json:"response_tool_calls,omitempty"`

	// Metrics
	PromptTokens     *int `json:"prompt_tokens,omitempty"`
	CompletionTokens *int `json:"completion_tokens,omitempty"`
	TotalTokens      *int `json:"total_tokens,omitempty"`
	DurationMs       int  `json:"duration_ms"`

	// Status
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// Status values for LLM conversations.
const (
	LLMConversationStatusPending = "pending" // Request sent, awaiting response
	LLMConversationStatusSuccess = "success"
	LLMConversationStatusError   = "error"
	LLMConversationStatusTimeout = "timeout"
)
