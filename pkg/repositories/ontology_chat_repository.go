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

// OntologyChatRepository provides data access for ontology chat messages.
type OntologyChatRepository interface {
	SaveMessage(ctx context.Context, message *models.ChatMessage) error
	GetHistory(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.ChatMessage, error)
	GetHistoryCount(ctx context.Context, projectID uuid.UUID) (int, error)
	ClearHistory(ctx context.Context, projectID uuid.UUID) error
}

type ontologyChatRepository struct{}

// NewOntologyChatRepository creates a new OntologyChatRepository.
func NewOntologyChatRepository() OntologyChatRepository {
	return &ontologyChatRepository{}
}

var _ OntologyChatRepository = (*ontologyChatRepository)(nil)

func (r *ontologyChatRepository) SaveMessage(ctx context.Context, message *models.ChatMessage) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	message.CreatedAt = time.Now()
	if message.ID == uuid.Nil {
		message.ID = uuid.New()
	}

	toolCallsJSON, err := json.Marshal(message.ToolCalls)
	if err != nil {
		return fmt.Errorf("failed to marshal tool_calls: %w", err)
	}
	if message.ToolCalls == nil {
		toolCallsJSON = nil
	}

	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if message.Metadata == nil {
		metadataJSON = []byte("{}")
	}

	query := `
		INSERT INTO engine_ontology_chat_messages (
			id, project_id, ontology_id, role, content, tool_calls, tool_call_id, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = scope.Conn.Exec(ctx, query,
		message.ID, message.ProjectID, message.OntologyID, message.Role, message.Content,
		toolCallsJSON, message.ToolCallID, metadataJSON, message.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	return nil
}

func (r *ontologyChatRepository) GetHistory(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.ChatMessage, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	if limit <= 0 {
		limit = 50 // Default limit
	}

	// Get messages in chronological order, but limit to most recent
	query := `
		SELECT id, project_id, ontology_id, role, content, tool_calls, tool_call_id, metadata, created_at
		FROM engine_ontology_chat_messages
		WHERE project_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := scope.Conn.Query(ctx, query, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat history: %w", err)
	}
	defer rows.Close()

	messages := make([]*models.ChatMessage, 0)
	for rows.Next() {
		m, err := scanChatMessageRows(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func (r *ontologyChatRepository) GetHistoryCount(ctx context.Context, projectID uuid.UUID) (int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT COUNT(*)
		FROM engine_ontology_chat_messages
		WHERE project_id = $1`

	var count int
	err := scope.Conn.QueryRow(ctx, query, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count chat messages: %w", err)
	}

	return count, nil
}

func (r *ontologyChatRepository) ClearHistory(ctx context.Context, projectID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_chat_messages WHERE project_id = $1`

	_, err := scope.Conn.Exec(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("failed to clear chat history: %w", err)
	}

	return nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanChatMessageRows(rows pgx.Rows) (*models.ChatMessage, error) {
	var m models.ChatMessage
	var toolCallsJSON, metadataJSON []byte
	var toolCallID *string

	err := rows.Scan(
		&m.ID, &m.ProjectID, &m.OntologyID, &m.Role, &m.Content,
		&toolCallsJSON, &toolCallID, &metadataJSON, &m.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan message: %w", err)
	}

	if toolCallID != nil {
		m.ToolCallID = *toolCallID
	}

	if len(toolCallsJSON) > 0 {
		if err := json.Unmarshal(toolCallsJSON, &m.ToolCalls); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool_calls: %w", err)
		}
	}

	if len(metadataJSON) > 0 {
		m.Metadata = make(map[string]any)
		if err := json.Unmarshal(metadataJSON, &m.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &m, nil
}
