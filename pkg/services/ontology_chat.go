package services

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// OntologyChatService provides operations for ontology chat interface.
type OntologyChatService interface {
	// Initialize initializes a chat session and returns an opening message.
	Initialize(ctx context.Context, projectID uuid.UUID) (*models.ChatInitResponse, error)

	// SendMessage sends a message and streams the response via the event channel.
	// The channel will receive events until ChatEventDone or ChatEventError.
	// NOTE: Caller owns the channel and is responsible for closing it. This service
	// writes events but does not close the channel, allowing the caller (handler)
	// to manage the channel lifecycle.
	SendMessage(ctx context.Context, projectID uuid.UUID, message string, eventChan chan<- models.ChatEvent) error

	// GetHistory retrieves chat history for a project.
	GetHistory(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.ChatMessage, error)

	// ClearHistory clears the chat history for a project.
	ClearHistory(ctx context.Context, projectID uuid.UUID) error

	// SaveMessage saves a chat message.
	SaveMessage(ctx context.Context, message *models.ChatMessage) error
}

type ontologyChatService struct {
	chatRepo           repositories.OntologyChatRepository
	ontologyRepo       repositories.OntologyRepository
	knowledgeRepo      repositories.KnowledgeRepository
	schemaRepo         repositories.SchemaRepository
	dagRepo            repositories.OntologyDAGRepository
	ontologyEntityRepo repositories.OntologyEntityRepository
	entityRelRepo      repositories.EntityRelationshipRepository
	llmFactory         llm.LLMClientFactory
	datasourceService  DatasourceService
	adapterFactory     datasource.DatasourceAdapterFactory
	logger             *zap.Logger
}

// NewOntologyChatService creates a new ontology chat service.
func NewOntologyChatService(
	chatRepo repositories.OntologyChatRepository,
	ontologyRepo repositories.OntologyRepository,
	knowledgeRepo repositories.KnowledgeRepository,
	schemaRepo repositories.SchemaRepository,
	dagRepo repositories.OntologyDAGRepository,
	ontologyEntityRepo repositories.OntologyEntityRepository,
	entityRelRepo repositories.EntityRelationshipRepository,
	llmFactory llm.LLMClientFactory,
	datasourceService DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	logger *zap.Logger,
) OntologyChatService {
	return &ontologyChatService{
		chatRepo:           chatRepo,
		ontologyRepo:       ontologyRepo,
		knowledgeRepo:      knowledgeRepo,
		schemaRepo:         schemaRepo,
		dagRepo:            dagRepo,
		ontologyEntityRepo: ontologyEntityRepo,
		entityRelRepo:      entityRelRepo,
		llmFactory:         llmFactory,
		datasourceService:  datasourceService,
		adapterFactory:     adapterFactory,
		logger:             logger.Named("ontology-chat"),
	}
}

var _ OntologyChatService = (*ontologyChatService)(nil)

func (s *ontologyChatService) Initialize(ctx context.Context, projectID uuid.UUID) (*models.ChatInitResponse, error) {
	// Check for existing history
	historyCount, err := s.chatRepo.GetHistoryCount(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get chat history count",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}

	// Note: The old workflow state pending questions system has been replaced by DAG-based workflow.
	// Pending question count is now always 0 since questions are handled differently.
	pendingCount := 0

	// Get the ontology to provide context
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get active ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}

	var openingMessage string

	if historyCount == 0 {
		// First conversation
		if ontology == nil || ontology.DomainSummary == nil {
			openingMessage = "Hello! I'm here to help you build and refine your data ontology. " +
				"It looks like we haven't extracted any ontology yet. " +
				"Would you like to start the extraction process?"
		} else {
			entityCount := ontology.TableCount()
			openingMessage = fmt.Sprintf(
				"Hello! I'm here to help you refine your data ontology. "+
					"I have analyzed %d tables in your database. "+
					"Feel free to ask me anything about your schema or tell me about specific business rules.",
				entityCount,
			)
		}
	} else {
		// Continuing conversation
		openingMessage = "Welcome back! How can I help you refine your data ontology?"
	}

	s.logger.Info("Chat initialized",
		zap.String("project_id", projectID.String()),
		zap.Int("pending_questions", pendingCount),
		zap.Bool("has_history", historyCount > 0))

	return &models.ChatInitResponse{
		OpeningMessage:       openingMessage,
		PendingQuestionCount: pendingCount,
		HasExistingHistory:   historyCount > 0,
	}, nil
}

func (s *ontologyChatService) SendMessage(ctx context.Context, projectID uuid.UUID, message string, eventChan chan<- models.ChatEvent) error {
	// Extract userID from context (JWT claims)
	userID, err := auth.RequireUserIDFromContext(ctx)
	if err != nil {
		s.logger.Error("User ID not found in context",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		eventChan <- models.NewErrorEvent("Authentication required")
		return fmt.Errorf("user ID not found in context: %w", err)
	}

	// Get DAG first to get ontologyID for all messages
	dag, err := s.dagRepo.GetLatestByProject(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get DAG",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		eventChan <- models.NewErrorEvent("No ontology extraction found. Please run extraction first.")
		return err
	}
	if dag == nil {
		s.logger.Error("No DAG found for project",
			zap.String("project_id", projectID.String()))
		eventChan <- models.NewErrorEvent("No ontology extraction found. Please run extraction first.")
		return fmt.Errorf("no DAG found for project %s", projectID)
	}

	if dag.OntologyID == nil {
		s.logger.Error("DAG has no ontology",
			zap.String("project_id", projectID.String()),
			zap.String("dag_id", dag.ID.String()))
		eventChan <- models.NewErrorEvent("DAG has no ontology")
		return fmt.Errorf("DAG %s has no ontology", dag.ID)
	}

	// Save user message with ontologyID from DAG
	userMessage := &models.ChatMessage{
		ProjectID:  projectID,
		OntologyID: *dag.OntologyID,
		Role:       models.ChatRoleUser,
		Content:    message,
	}

	if err := s.chatRepo.SaveMessage(ctx, userMessage); err != nil {
		s.logger.Error("Failed to save user message",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		eventChan <- models.NewErrorEvent("Failed to save message")
		return err
	}

	// Get datasource config (decrypted)
	ds, err := s.datasourceService.Get(ctx, projectID, dag.DatasourceID)
	if err != nil {
		s.logger.Error("Failed to get datasource",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", dag.DatasourceID.String()),
			zap.Error(err))
		eventChan <- models.NewErrorEvent("Failed to get datasource configuration")
		return err
	}

	// Create query executor with identity parameters for connection pooling
	queryExecutor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config, projectID, dag.DatasourceID, userID)
	if err != nil {
		s.logger.Error("Failed to create query executor",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		eventChan <- models.NewErrorEvent("Failed to connect to datasource")
		return err
	}
	defer queryExecutor.Close()

	// Create tool executor
	toolExecutor := llm.NewOntologyToolExecutor(&llm.OntologyToolExecutorConfig{
		ProjectID:          projectID,
		DatasourceID:       dag.DatasourceID,
		OntologyRepo:       s.ontologyRepo,
		KnowledgeRepo:      s.knowledgeRepo,
		SchemaRepo:         s.schemaRepo,
		OntologyEntityRepo: s.ontologyEntityRepo,
		EntityRelRepo:      s.entityRelRepo,
		QueryExecutor:      queryExecutor,
		Logger:             s.logger,
	})

	// Create streaming client
	streamingClient, err := s.llmFactory.CreateStreamingClient(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to create LLM client",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		eventChan <- models.NewErrorEvent("Failed to create LLM client")
		return err
	}

	// Get ontology for system prompt context
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get active ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		eventChan <- models.NewErrorEvent("Failed to get ontology")
		return err
	}

	// Build messages from chat history
	history, err := s.chatRepo.GetHistory(ctx, projectID, 20)
	if err != nil {
		s.logger.Error("Failed to get chat history",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		eventChan <- models.NewErrorEvent("Failed to get chat history")
		return err
	}

	messages := s.convertChatHistoryToLLMMessages(history)

	// Build system prompt with ontology context
	systemPrompt := s.buildChatSystemPrompt(ontology)

	// Create streaming request
	req := &llm.StreamingRequest{
		SystemPrompt: systemPrompt,
		Messages:     messages,
		Tools:        llm.GetOntologyChatTools(),
		Temperature:  0.7,
	}

	// Stream with tools and forward events
	internalChan := make(chan llm.StreamEvent, 100)
	errChan := make(chan error, 1)
	go func() {
		defer close(internalChan)
		errChan <- streamingClient.StreamWithTools(ctx, req, toolExecutor, internalChan)
	}()

	// Accumulators for building complete messages
	var textBuilder strings.Builder
	var pendingToolCalls []models.ToolCall

	// Forward stream events to chat events and save to history
	for event := range internalChan {
		chatEvent := s.convertStreamEventToChatEvent(event)
		eventChan <- chatEvent

		switch event.Type {
		case llm.StreamEventText:
			textBuilder.WriteString(event.Content)

		case llm.StreamEventToolCall:
			// Accumulate tool calls for the assistant message
			if tc, ok := event.Data.(llm.ToolCall); ok {
				pendingToolCalls = append(pendingToolCalls, models.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: models.ToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}

		case llm.StreamEventToolResult:
			// When we see the first tool result, save the assistant message with tool calls
			if len(pendingToolCalls) > 0 || textBuilder.Len() > 0 {
				assistantMsg := &models.ChatMessage{
					ProjectID:  projectID,
					OntologyID: ontology.ID,
					Role:       models.ChatRoleAssistant,
					Content:    textBuilder.String(),
					ToolCalls:  pendingToolCalls,
				}
				if err := s.chatRepo.SaveMessage(ctx, assistantMsg); err != nil {
					s.logger.Error("Failed to save assistant message with tool calls",
						zap.String("project_id", projectID.String()),
						zap.Error(err))
				}
				// Reset accumulators for next iteration
				textBuilder.Reset()
				pendingToolCalls = nil
			}

			// Save the tool result as a tool message
			toolCallID := ""
			if data, ok := event.Data.(map[string]string); ok {
				toolCallID = data["tool_call_id"]
			}
			toolMsg := &models.ChatMessage{
				ProjectID:  projectID,
				OntologyID: ontology.ID,
				Role:       models.ChatRoleTool,
				Content:    event.Content,
				ToolCallID: toolCallID,
			}
			if err := s.chatRepo.SaveMessage(ctx, toolMsg); err != nil {
				s.logger.Error("Failed to save tool result message",
					zap.String("project_id", projectID.String()),
					zap.String("tool_call_id", toolCallID),
					zap.Error(err))
			}

		case llm.StreamEventDone, llm.StreamEventError:
			// Save any remaining text as final assistant message
			if textBuilder.Len() > 0 {
				assistantMsg := &models.ChatMessage{
					ProjectID:  projectID,
					OntologyID: ontology.ID,
					Role:       models.ChatRoleAssistant,
					Content:    textBuilder.String(),
				}
				if err := s.chatRepo.SaveMessage(ctx, assistantMsg); err != nil {
					s.logger.Error("Failed to save final assistant message",
						zap.String("project_id", projectID.String()),
						zap.Error(err))
				}
			}
			break
		}

		// Stop on done or error
		if event.Type == llm.StreamEventDone || event.Type == llm.StreamEventError {
			break
		}
	}

	// Wait for streaming goroutine to complete and return any error
	return <-errChan
}

func (s *ontologyChatService) GetHistory(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.ChatMessage, error) {
	messages, err := s.chatRepo.GetHistory(ctx, projectID, limit)
	if err != nil {
		s.logger.Error("Failed to get chat history",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}
	return messages, nil
}

func (s *ontologyChatService) ClearHistory(ctx context.Context, projectID uuid.UUID) error {
	if err := s.chatRepo.ClearHistory(ctx, projectID); err != nil {
		s.logger.Error("Failed to clear chat history",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return err
	}

	s.logger.Info("Chat history cleared",
		zap.String("project_id", projectID.String()))

	return nil
}

func (s *ontologyChatService) SaveMessage(ctx context.Context, message *models.ChatMessage) error {
	if err := s.chatRepo.SaveMessage(ctx, message); err != nil {
		s.logger.Error("Failed to save message",
			zap.String("project_id", message.ProjectID.String()),
			zap.Error(err))
		return err
	}
	return nil
}

// ============================================================================
// Helper Methods
// ============================================================================

// convertChatHistoryToLLMMessages converts chat message history to LLM message format.
func (s *ontologyChatService) convertChatHistoryToLLMMessages(history []*models.ChatMessage) []llm.Message {
	messages := make([]llm.Message, 0, len(history))
	for _, msg := range history {
		llmMsg := llm.Message{
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}

		// Convert role
		switch msg.Role {
		case models.ChatRoleUser:
			llmMsg.Role = llm.RoleUser
		case models.ChatRoleAssistant:
			llmMsg.Role = llm.RoleAssistant
		case models.ChatRoleTool:
			llmMsg.Role = llm.RoleTool
		default:
			llmMsg.Role = llm.RoleUser
		}

		// Convert tool calls if present
		if len(msg.ToolCalls) > 0 {
			llmMsg.ToolCalls = make([]llm.ToolCall, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				llmMsg.ToolCalls[i] = llm.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: llm.ToolCallFunc{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		messages = append(messages, llmMsg)
	}
	return messages
}

// buildChatSystemPrompt creates a system prompt with ontology context.
func (s *ontologyChatService) buildChatSystemPrompt(ontology *models.TieredOntology) string {
	var sb strings.Builder

	sb.WriteString(`You are an AI assistant helping users understand and refine their data ontology. Your role is to:

1. Answer questions about the database schema, tables, and columns
2. Help identify and document business rules and domain knowledge
3. Clarify the meaning and relationships between data entities
4. Store important facts about the data using the available tools

Available tools:
- query_column_values: Query sample values from database columns
- query_schema_metadata: Get metadata about tables and columns
- update_entity: Update descriptions or synonyms for tables
- update_column: Update descriptions or semantic types for columns
- store_knowledge: Store business facts and domain knowledge
- create_domain_entity: Create a new domain entity (e.g., 'campaign', 'subscription')
- create_entity_relationship: Create a relationship between domain entities

Guidelines:
- Be helpful and conversational while staying focused on data understanding
- Use tools when you need to explore the data or update the ontology
- When storing knowledge, categorize it appropriately (terminology, business_rule, data_relationship, constraint, context)
- Ask clarifying questions when the user's intent is unclear

`)

	if ontology != nil && ontology.DomainSummary != nil {
		sb.WriteString("## Current Domain Context\n\n")
		if ontology.DomainSummary.Description != "" {
			if len(ontology.DomainSummary.Domains) > 0 {
				sb.WriteString(fmt.Sprintf("**Domains:** %s\n", strings.Join(ontology.DomainSummary.Domains, ", ")))
			}
			sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", ontology.DomainSummary.Description))
		}

		if len(ontology.EntitySummaries) > 0 {
			sb.WriteString("## Available Tables\n\n")
			// RESEARCH: Consider ordering by relevance (row count, relationship degree,
			// query frequency) rather than alphabetically to signal importance to the LLM.
			tableNames := make([]string, 0, len(ontology.EntitySummaries))
			for name := range ontology.EntitySummaries {
				tableNames = append(tableNames, name)
			}
			sort.Strings(tableNames)
			for _, tableName := range tableNames {
				entity := ontology.EntitySummaries[tableName]
				if entity.Description != "" {
					sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tableName, entity.Description))
				} else {
					sb.WriteString(fmt.Sprintf("- **%s**\n", tableName))
				}
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("Note: No ontology has been extracted yet. You may need to help the user start the extraction process.\n\n")
	}

	return sb.String()
}

// convertStreamEventToChatEvent converts an LLM stream event to a chat event.
func (s *ontologyChatService) convertStreamEventToChatEvent(event llm.StreamEvent) models.ChatEvent {
	switch event.Type {
	case llm.StreamEventText:
		return models.NewTextEvent(event.Content)
	case llm.StreamEventToolCall:
		// Convert llm.ToolCall to models.ToolCall
		if tc, ok := event.Data.(llm.ToolCall); ok {
			modelTC := models.ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: models.ToolCallFunction{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
			return models.NewToolCallEvent(modelTC)
		}
		return models.NewTextEvent(event.Content)
	case llm.StreamEventToolResult:
		if data, ok := event.Data.(map[string]string); ok {
			return models.NewToolResultEvent(data["tool_call_id"], event.Content)
		}
		return models.NewToolResultEvent("", event.Content)
	case llm.StreamEventDone:
		return models.NewDoneEvent()
	case llm.StreamEventError:
		return models.NewErrorEvent(event.Content)
	default:
		return models.NewTextEvent(event.Content)
	}
}
