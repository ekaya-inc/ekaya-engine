package models

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Chat Roles
// ============================================================================

// ChatRole represents the role of a chat message sender.
type ChatRole string

const (
	ChatRoleUser      ChatRole = "user"
	ChatRoleAssistant ChatRole = "assistant"
	ChatRoleSystem    ChatRole = "system"
	ChatRoleTool      ChatRole = "tool"
)

// ValidChatRoles contains all valid chat role values.
var ValidChatRoles = []ChatRole{
	ChatRoleUser,
	ChatRoleAssistant,
	ChatRoleSystem,
	ChatRoleTool,
}

// IsValidChatRole checks if the given role is valid.
func IsValidChatRole(r ChatRole) bool {
	for _, v := range ValidChatRoles {
		if v == r {
			return true
		}
	}
	return false
}

// ============================================================================
// Tool Calls
// ============================================================================

// ToolCall represents an LLM tool call request.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction contains the function name and arguments for a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ============================================================================
// Chat Message
// ============================================================================

// ChatMessage represents a message in the ontology chat interface.
type ChatMessage struct {
	ID         uuid.UUID      `json:"id"`
	ProjectID  uuid.UUID      `json:"project_id"`
	OntologyID uuid.UUID      `json:"ontology_id"`
	Role       ChatRole       `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// IsFromUser returns true if the message is from a user.
func (m *ChatMessage) IsFromUser() bool {
	return m.Role == ChatRoleUser
}

// IsFromAssistant returns true if the message is from the assistant.
func (m *ChatMessage) IsFromAssistant() bool {
	return m.Role == ChatRoleAssistant
}

// IsToolResponse returns true if the message is a tool response.
func (m *ChatMessage) IsToolResponse() bool {
	return m.Role == ChatRoleTool
}

// HasToolCalls returns true if the message contains tool calls.
func (m *ChatMessage) HasToolCalls() bool {
	return len(m.ToolCalls) > 0
}

// ============================================================================
// Chat Events (for SSE streaming)
// ============================================================================

// ChatEventType represents the type of a streaming chat event.
type ChatEventType string

const (
	ChatEventText            ChatEventType = "text"
	ChatEventToolCall        ChatEventType = "tool_call"
	ChatEventToolResult      ChatEventType = "tool_result"
	ChatEventOntologyUpdate  ChatEventType = "ontology_update"
	ChatEventKnowledgeStored ChatEventType = "knowledge_stored"
	ChatEventDone            ChatEventType = "done"
	ChatEventError           ChatEventType = "error"
)

// ChatEvent represents a streaming event from the chat service.
type ChatEvent struct {
	Type    ChatEventType `json:"type"`
	Content string        `json:"content,omitempty"`
	Data    any           `json:"data,omitempty"`
}

// NewTextEvent creates a text streaming event.
func NewTextEvent(content string) ChatEvent {
	return ChatEvent{Type: ChatEventText, Content: content}
}

// NewToolCallEvent creates a tool call event.
func NewToolCallEvent(toolCall ToolCall) ChatEvent {
	return ChatEvent{Type: ChatEventToolCall, Data: toolCall}
}

// NewToolResultEvent creates a tool result event.
func NewToolResultEvent(toolID string, result any) ChatEvent {
	return ChatEvent{
		Type:    ChatEventToolResult,
		Content: toolID,
		Data:    result,
	}
}

// NewOntologyUpdateEvent creates an ontology update event.
func NewOntologyUpdateEvent(tableName string, update any) ChatEvent {
	return ChatEvent{
		Type:    ChatEventOntologyUpdate,
		Content: tableName,
		Data:    update,
	}
}

// NewKnowledgeStoredEvent creates a knowledge stored event.
func NewKnowledgeStoredEvent(fact *KnowledgeFact) ChatEvent {
	return ChatEvent{Type: ChatEventKnowledgeStored, Data: fact}
}

// NewDoneEvent creates a completion event.
func NewDoneEvent() ChatEvent {
	return ChatEvent{Type: ChatEventDone}
}

// NewErrorEvent creates an error event.
func NewErrorEvent(err string) ChatEvent {
	return ChatEvent{Type: ChatEventError, Content: err}
}

// ============================================================================
// Chat Initialization Response
// ============================================================================

// ChatInitResponse contains the response for chat initialization.
type ChatInitResponse struct {
	OpeningMessage       string `json:"opening_message"`
	PendingQuestionCount int    `json:"pending_question_count"`
	HasExistingHistory   bool   `json:"has_existing_history"`
}

// ============================================================================
// Knowledge Facts
// ============================================================================

// Knowledge fact types
const (
	FactTypeFiscalYear   = "fiscal_year"
	FactTypeBusinessRule = "business_rule"
	FactTypeTerminology  = "terminology"
	FactTypeConvention   = "convention"
	FactTypeEnumeration  = "enumeration"
	FactTypeRelationship = "relationship"
)

// ValidFactTypes contains all valid fact type values.
var ValidFactTypes = []string{
	FactTypeFiscalYear,
	FactTypeBusinessRule,
	FactTypeTerminology,
	FactTypeConvention,
	FactTypeEnumeration,
	FactTypeRelationship,
}

// IsValidFactType checks if the given fact type is valid.
func IsValidFactType(t string) bool {
	for _, v := range ValidFactTypes {
		if v == t {
			return true
		}
	}
	return false
}

// KnowledgeFact represents a learned business fact about the project.
type KnowledgeFact struct {
	ID         uuid.UUID  `json:"id"`
	ProjectID  uuid.UUID  `json:"project_id"`
	OntologyID *uuid.UUID `json:"ontology_id,omitempty"` // Links to ontology for CASCADE delete
	FactType   string     `json:"fact_type"`
	Key        string     `json:"key"`
	Value      string     `json:"value"`
	Context    string     `json:"context,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ============================================================================
// Chat Tool Definitions (for LLM)
// ============================================================================

// ChatTool represents a tool definition for the chat LLM.
type ChatTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Available chat tools
var ChatToolDefinitions = []ChatTool{
	{
		Name:        "query_column_values",
		Description: "Query actual values from a column in the datasource",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"table_name":  map[string]any{"type": "string", "description": "Name of the table"},
				"column_name": map[string]any{"type": "string", "description": "Name of the column"},
				"limit":       map[string]any{"type": "integer", "description": "Max values to return", "default": 20},
			},
			"required": []string{"table_name", "column_name"},
		},
	},
	{
		Name:        "query_schema_metadata",
		Description: "Get metadata about tables or columns from schema analysis",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"table_name":  map[string]any{"type": "string", "description": "Name of the table"},
				"column_name": map[string]any{"type": "string", "description": "Optional column name"},
			},
			"required": []string{"table_name"},
		},
	},
	{
		Name:        "update_entity",
		Description: "Update an entity's description, synonyms, or column information in the ontology",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"table_name":    map[string]any{"type": "string", "description": "Name of the table/entity"},
				"business_name": map[string]any{"type": "string", "description": "Updated business name"},
				"description":   map[string]any{"type": "string", "description": "Updated description"},
				"synonyms":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Synonyms to add"},
				"column_updates": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"column_name": map[string]any{"type": "string"},
							"description": map[string]any{"type": "string"},
							"synonyms":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
					"description": "Column-level updates",
				},
			},
			"required": []string{"table_name"},
		},
	},
	{
		Name:        "store_knowledge",
		Description: "Store a project-level business fact or convention",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"fact_type": map[string]any{
					"type":        "string",
					"enum":        ValidFactTypes,
					"description": "Type of fact being stored",
				},
				"key":     map[string]any{"type": "string", "description": "Unique key for the fact"},
				"value":   map[string]any{"type": "string", "description": "The fact value"},
				"context": map[string]any{"type": "string", "description": "Additional context about the fact"},
			},
			"required": []string{"fact_type", "key", "value"},
		},
	},
	{
		Name:        "update_domain",
		Description: "Update the domain summary description or sample questions",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description":      map[string]any{"type": "string", "description": "Updated domain description"},
				"sample_questions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Sample business questions"},
			},
		},
	},
}
