package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ConversationRepository provides data access for LLM conversation records.
type ConversationRepository interface {
	Save(ctx context.Context, conv *models.LLMConversation) error
	Update(ctx context.Context, conv *models.LLMConversation) error
	GetByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.LLMConversation, error)
	GetByContext(ctx context.Context, projectID uuid.UUID, key, value string) ([]*models.LLMConversation, error)
	GetByConversationID(ctx context.Context, conversationID uuid.UUID) ([]*models.LLMConversation, error)
}

type conversationRepository struct{}

// NewConversationRepository creates a new ConversationRepository.
func NewConversationRepository() ConversationRepository {
	return &conversationRepository{}
}

var _ ConversationRepository = (*conversationRepository)(nil)

func (r *conversationRepository) Save(ctx context.Context, conv *models.LLMConversation) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	conv.CreatedAt = time.Now()
	if conv.ID == uuid.Nil {
		conv.ID = uuid.New()
	}

	// Marshal JSONB fields
	requestMessagesJSON, err := json.Marshal(conv.RequestMessages)
	if err != nil {
		return fmt.Errorf("failed to marshal request_messages: %w", err)
	}

	var requestToolsJSON, responseToolCallsJSON, contextJSON []byte
	if conv.RequestTools != nil {
		requestToolsJSON, err = json.Marshal(conv.RequestTools)
		if err != nil {
			return fmt.Errorf("failed to marshal request_tools: %w", err)
		}
	}
	if conv.ResponseToolCalls != nil {
		responseToolCallsJSON, err = json.Marshal(conv.ResponseToolCalls)
		if err != nil {
			return fmt.Errorf("failed to marshal response_tool_calls: %w", err)
		}
	}
	if conv.Context != nil {
		contextJSON, err = json.Marshal(conv.Context)
		if err != nil {
			return fmt.Errorf("failed to marshal context: %w", err)
		}
	}

	// Use NULL for empty error_message (success cases)
	var errorMessage *string
	if conv.ErrorMessage != "" {
		errorMessage = &conv.ErrorMessage
	}

	query := `
		INSERT INTO engine_llm_conversations (
			id, project_id, context, conversation_id, iteration,
			endpoint, model, request_messages, request_tools, temperature,
			response_content, response_tool_calls,
			prompt_tokens, completion_tokens, total_tokens, duration_ms,
			status, error_message, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`

	_, err = scope.Conn.Exec(ctx, query,
		conv.ID, conv.ProjectID, contextJSON, conv.ConversationID, conv.Iteration,
		conv.Endpoint, conv.Model, requestMessagesJSON, requestToolsJSON, conv.Temperature,
		conv.ResponseContent, responseToolCallsJSON,
		conv.PromptTokens, conv.CompletionTokens, conv.TotalTokens, conv.DurationMs,
		conv.Status, errorMessage, conv.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save llm conversation: %w", err)
	}

	return nil
}

func (r *conversationRepository) Update(ctx context.Context, conv *models.LLMConversation) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Marshal JSONB fields for response
	var responseToolCallsJSON []byte
	var err error
	if conv.ResponseToolCalls != nil {
		responseToolCallsJSON, err = json.Marshal(conv.ResponseToolCalls)
		if err != nil {
			return fmt.Errorf("failed to marshal response_tool_calls: %w", err)
		}
	}

	// Use NULL for empty error_message (success cases)
	var errorMessage *string
	if conv.ErrorMessage != "" {
		errorMessage = &conv.ErrorMessage
	}

	query := `
		UPDATE engine_llm_conversations
		SET response_content = $2,
		    response_tool_calls = $3,
		    prompt_tokens = $4,
		    completion_tokens = $5,
		    total_tokens = $6,
		    duration_ms = $7,
		    status = $8,
		    error_message = $9
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query,
		conv.ID,
		conv.ResponseContent, responseToolCallsJSON,
		conv.PromptTokens, conv.CompletionTokens, conv.TotalTokens, conv.DurationMs,
		conv.Status, errorMessage,
	)
	if err != nil {
		return fmt.Errorf("failed to update llm conversation: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("conversation not found: %s", conv.ID)
	}

	return nil
}

func (r *conversationRepository) GetByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.LLMConversation, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, context, conversation_id, iteration,
		       endpoint, model, request_messages, request_tools, temperature,
		       response_content, response_tool_calls,
		       prompt_tokens, completion_tokens, total_tokens, duration_ms,
		       status, error_message, created_at
		FROM engine_llm_conversations
		WHERE project_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := scope.Conn.Query(ctx, query, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	return scanConversationRows(rows)
}

// GetByContext queries conversations by a key-value pair in the context JSONB.
// Example: GetByContext(ctx, projectID, "workflow_id", "uuid-string")
func (r *conversationRepository) GetByContext(ctx context.Context, projectID uuid.UUID, key, value string) ([]*models.LLMConversation, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, context, conversation_id, iteration,
		       endpoint, model, request_messages, request_tools, temperature,
		       response_content, response_tool_calls,
		       prompt_tokens, completion_tokens, total_tokens, duration_ms,
		       status, error_message, created_at
		FROM engine_llm_conversations
		WHERE project_id = $1 AND context->>$2 = $3
		ORDER BY created_at ASC`

	rows, err := scope.Conn.Query(ctx, query, projectID, key, value)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	return scanConversationRows(rows)
}

func (r *conversationRepository) GetByConversationID(ctx context.Context, conversationID uuid.UUID) ([]*models.LLMConversation, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, context, conversation_id, iteration,
		       endpoint, model, request_messages, request_tools, temperature,
		       response_content, response_tool_calls,
		       prompt_tokens, completion_tokens, total_tokens, duration_ms,
		       status, error_message, created_at
		FROM engine_llm_conversations
		WHERE conversation_id = $1
		ORDER BY iteration ASC`

	rows, err := scope.Conn.Query(ctx, query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	return scanConversationRows(rows)
}

func scanConversationRows(rows pgx.Rows) ([]*models.LLMConversation, error) {
	var conversations []*models.LLMConversation

	for rows.Next() {
		conv, err := scanConversationRow(rows)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conv)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return conversations, nil
}

func scanConversationRow(row pgx.Row) (*models.LLMConversation, error) {
	var conv models.LLMConversation
	var contextJSON, requestMessagesJSON, requestToolsJSON, responseToolCallsJSON []byte
	var errorMessage *string

	err := row.Scan(
		&conv.ID, &conv.ProjectID, &contextJSON, &conv.ConversationID, &conv.Iteration,
		&conv.Endpoint, &conv.Model, &requestMessagesJSON, &requestToolsJSON, &conv.Temperature,
		&conv.ResponseContent, &responseToolCallsJSON,
		&conv.PromptTokens, &conv.CompletionTokens, &conv.TotalTokens, &conv.DurationMs,
		&conv.Status, &errorMessage, &conv.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan conversation: %w", err)
	}

	// Convert nullable error_message
	if errorMessage != nil {
		conv.ErrorMessage = *errorMessage
	}

	// Unmarshal JSONB fields
	if len(contextJSON) > 0 {
		if err := json.Unmarshal(contextJSON, &conv.Context); err != nil {
			return nil, fmt.Errorf("failed to unmarshal context: %w", err)
		}
	}
	if len(requestMessagesJSON) > 0 {
		if err := json.Unmarshal(requestMessagesJSON, &conv.RequestMessages); err != nil {
			return nil, fmt.Errorf("failed to unmarshal request_messages: %w", err)
		}
	}
	if len(requestToolsJSON) > 0 {
		if err := json.Unmarshal(requestToolsJSON, &conv.RequestTools); err != nil {
			return nil, fmt.Errorf("failed to unmarshal request_tools: %w", err)
		}
	}
	if len(responseToolCallsJSON) > 0 {
		if err := json.Unmarshal(responseToolCallsJSON, &conv.ResponseToolCalls); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response_tool_calls: %w", err)
		}
	}

	return &conv, nil
}
