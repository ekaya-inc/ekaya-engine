package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// OntologyQuestionService provides operations for ontology question management.
// Questions are stored in the engine_ontology_questions table, decoupled from workflow lifecycle.
type OntologyQuestionService interface {
	// GetNextQuestion returns the next pending question for a project.
	GetNextQuestion(ctx context.Context, projectID uuid.UUID, includeSkipped bool) (*models.OntologyQuestion, error)

	// GetPendingQuestions returns all pending questions for a project.
	GetPendingQuestions(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error)

	// GetPendingCount returns the count of pending questions.
	GetPendingCount(ctx context.Context, projectID uuid.UUID) (int, error)

	// GetPendingCounts returns separate counts of required and optional pending questions.
	GetPendingCounts(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error)

	// AnswerQuestion processes an answer and applies any resulting actions.
	AnswerQuestion(ctx context.Context, questionID uuid.UUID, answer string, userID string) (*models.AnswerResult, error)

	// SkipQuestion marks a question as skipped.
	SkipQuestion(ctx context.Context, questionID uuid.UUID) error

	// DeleteQuestion soft-deletes a question.
	DeleteQuestion(ctx context.Context, questionID uuid.UUID) error

	// CreateQuestions creates a batch of questions for a project/workflow.
	CreateQuestions(ctx context.Context, questions []*models.OntologyQuestion) error
}

type ontologyQuestionService struct {
	questionRepo       repositories.OntologyQuestionRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	schemaRepo         repositories.SchemaRepository
	knowledgeRepo      repositories.KnowledgeRepository
	builder            OntologyBuilderService
	logger             *zap.Logger
}

// NewOntologyQuestionService creates a new ontology question service.
func NewOntologyQuestionService(
	questionRepo repositories.OntologyQuestionRepository,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	schemaRepo repositories.SchemaRepository,
	knowledgeRepo repositories.KnowledgeRepository,
	builder OntologyBuilderService,
	logger *zap.Logger,
) OntologyQuestionService {
	return &ontologyQuestionService{
		questionRepo:       questionRepo,
		columnMetadataRepo: columnMetadataRepo,
		schemaRepo:         schemaRepo,
		knowledgeRepo:      knowledgeRepo,
		builder:            builder,
		logger:             logger.Named("ontology-question"),
	}
}

var _ OntologyQuestionService = (*ontologyQuestionService)(nil)

func (s *ontologyQuestionService) GetNextQuestion(ctx context.Context, projectID uuid.UUID, includeSkipped bool) (*models.OntologyQuestion, error) {
	// Questions are stored in the dedicated table, no need to check workflow state
	question, err := s.questionRepo.GetNextPending(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get next pending question",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}
	return question, nil
}

func (s *ontologyQuestionService) GetPendingQuestions(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error) {
	questions, err := s.questionRepo.ListPending(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to list pending questions",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}
	return questions, nil
}

func (s *ontologyQuestionService) GetPendingCount(ctx context.Context, projectID uuid.UUID) (int, error) {
	counts, err := s.GetPendingCounts(ctx, projectID)
	if err != nil {
		return 0, err
	}
	if counts == nil {
		return 0, nil
	}
	return counts.Required + counts.Optional, nil
}

func (s *ontologyQuestionService) GetPendingCounts(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error) {
	counts, err := s.questionRepo.GetPendingCounts(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get pending question counts",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}
	return counts, nil
}

func (s *ontologyQuestionService) AnswerQuestion(ctx context.Context, questionID uuid.UUID, answer string, userID string) (*models.AnswerResult, error) {
	// Get the question from the dedicated table
	question, err := s.questionRepo.GetByID(ctx, questionID)
	if err != nil {
		s.logger.Error("Failed to get question",
			zap.String("question_id", questionID.String()),
			zap.Error(err))
		return nil, err
	}

	if question == nil {
		return nil, fmt.Errorf("question not found")
	}

	if question.Status != models.QuestionStatusPending && question.Status != models.QuestionStatusSkipped {
		return nil, fmt.Errorf("question is not in a pending state")
	}

	// Process the answer using LLM to extract ontology updates and knowledge facts
	processingResult, err := s.builder.ProcessAnswer(ctx, question.ProjectID, question, answer)
	if err != nil {
		s.logger.Error("Failed to process answer",
			zap.String("question_id", questionID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("process answer: %w", err)
	}

	// Apply entity updates to the ontology
	if err := s.applyEntityUpdates(ctx, question.ProjectID, processingResult.EntityUpdates); err != nil {
		s.logger.Error("Failed to apply entity updates",
			zap.String("question_id", questionID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("apply entity updates: %w", err)
	}

	// Apply column updates to the ontology
	if err := s.applyColumnUpdates(ctx, question.ProjectID, processingResult.ColumnUpdates); err != nil {
		s.logger.Error("Failed to apply column updates",
			zap.String("question_id", questionID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("apply column updates: %w", err)
	}

	// Store knowledge facts
	for _, fact := range processingResult.KnowledgeFacts {
		if err := s.knowledgeRepo.Create(ctx, fact); err != nil {
			s.logger.Error("Failed to store knowledge fact",
				zap.String("fact_type", fact.FactType),
				zap.Error(err))
			return nil, fmt.Errorf("store knowledge fact %s: %w", fact.FactType, err)
		}
	}

	// Create follow-up question if needed
	if processingResult.FollowUp != nil && *processingResult.FollowUp != "" {
		followUp := &models.OntologyQuestion{
			ProjectID:        question.ProjectID,
			Text:             *processingResult.FollowUp,
			Priority:         question.Priority,
			IsRequired:       false, // Follow-ups are optional
			Category:         "follow_up",
			Status:           models.QuestionStatusPending,
			Affects:          question.Affects, // Inherit affects from parent
			ParentQuestionID: &questionID,
		}
		if err := s.questionRepo.Create(ctx, followUp); err != nil {
			s.logger.Error("Failed to create follow-up question",
				zap.String("question_id", questionID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("create follow-up question: %w", err)
		}
	}

	// Mark question as answered
	var answeredByUUID *uuid.UUID
	if userID != "" {
		if uid, err := uuid.Parse(userID); err == nil {
			answeredByUUID = &uid
		}
	}
	if err := s.questionRepo.SubmitAnswer(ctx, questionID, answer, answeredByUUID); err != nil {
		s.logger.Error("Failed to submit answer",
			zap.String("question_id", questionID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("submit answer: %w", err)
	}

	s.logger.Info("Question answered and processed",
		zap.String("question_id", questionID.String()),
		zap.String("user_id", userID),
		zap.Int("entity_updates", len(processingResult.EntityUpdates)),
		zap.Int("knowledge_facts", len(processingResult.KnowledgeFacts)))

	// Get next question
	nextQuestion, err := s.GetNextQuestion(ctx, question.ProjectID, false)
	if err != nil {
		s.logger.Error("Failed to get next question after answer",
			zap.String("project_id", question.ProjectID.String()),
			zap.Error(err))
		return nil, err
	}

	return &models.AnswerResult{
		QuestionID:     questionID,
		FollowUp:       processingResult.FollowUp,
		ActionsSummary: processingResult.ActionsSummary,
		Thinking:       processingResult.Thinking,
		NextQuestion:   nextQuestion,
		AllComplete:    nextQuestion == nil,
	}, nil
}

// applyEntityUpdates applies entity updates from answer processing to the ontology.
// Note: EntitySummaries have been removed for v1.0 entity simplification.
// This method is now a no-op. Entity updates are logged but not persisted.
func (s *ontologyQuestionService) applyEntityUpdates(ctx context.Context, projectID uuid.UUID, updates []EntityUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	// Log updates for debugging, but don't persist (entity functionality removed)
	s.logger.Debug("Skipping entity updates - entity functionality removed for v1.0",
		zap.String("project_id", projectID.String()),
		zap.Int("update_count", len(updates)))

	return nil
}

// applyColumnUpdates applies column updates from answer processing to ColumnMetadata.
func (s *ontologyQuestionService) applyColumnUpdates(ctx context.Context, projectID uuid.UUID, updates []ColumnUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	for _, update := range updates {
		// Resolve schema table, then column
		table, err := s.schemaRepo.FindTableByName(ctx, projectID, uuid.Nil, update.TableName)
		if err != nil || table == nil {
			s.logger.Warn("Failed to find schema table for update, skipping",
				zap.String("table", update.TableName),
				zap.Error(err))
			continue
		}
		schemaCol, err := s.schemaRepo.GetColumnByName(ctx, table.ID, update.ColumnName)
		if err != nil || schemaCol == nil {
			s.logger.Warn("Failed to find schema column for update, skipping",
				zap.String("table", update.TableName),
				zap.String("column", update.ColumnName),
				zap.Error(err))
			continue
		}

		// Get existing metadata or create new
		existing, err := s.columnMetadataRepo.GetBySchemaColumnID(ctx, schemaCol.ID)
		if err != nil {
			s.logger.Warn("Failed to get column metadata, creating new",
				zap.String("table", update.TableName),
				zap.String("column", update.ColumnName),
				zap.Error(err))
		}

		meta := existing
		if meta == nil {
			meta = &models.ColumnMetadata{
				ProjectID:      projectID,
				SchemaColumnID: schemaCol.ID,
				Source:         models.ProvenanceMCP,
			}
		}

		// Apply updates
		if update.Description != nil {
			meta.Description = update.Description
		}
		if update.SemanticType != nil {
			meta.SemanticType = update.SemanticType
		}
		if update.Role != nil {
			meta.Role = update.Role
		}
		if len(update.Synonyms) > 0 {
			// Deduplicate synonyms
			existingSynonyms := make(map[string]bool)
			for _, syn := range meta.Features.Synonyms {
				existingSynonyms[syn] = true
			}
			for _, syn := range update.Synonyms {
				if !existingSynonyms[syn] {
					meta.Features.Synonyms = append(meta.Features.Synonyms, syn)
				}
			}
		}

		if err := s.columnMetadataRepo.Upsert(ctx, meta); err != nil {
			return fmt.Errorf("upsert column metadata for %s.%s: %w", update.TableName, update.ColumnName, err)
		}

		s.logger.Info("Applied column update",
			zap.String("project_id", projectID.String()),
			zap.String("table", update.TableName),
			zap.String("column", update.ColumnName))
	}

	return nil
}

func (s *ontologyQuestionService) SkipQuestion(ctx context.Context, questionID uuid.UUID) error {
	if err := s.questionRepo.UpdateStatus(ctx, questionID, models.QuestionStatusSkipped); err != nil {
		s.logger.Error("Failed to skip question",
			zap.String("question_id", questionID.String()),
			zap.Error(err))
		return err
	}

	s.logger.Info("Question skipped",
		zap.String("question_id", questionID.String()))

	return nil
}

func (s *ontologyQuestionService) DeleteQuestion(ctx context.Context, questionID uuid.UUID) error {
	if err := s.questionRepo.UpdateStatus(ctx, questionID, models.QuestionStatusDeleted); err != nil {
		s.logger.Error("Failed to delete question",
			zap.String("question_id", questionID.String()),
			zap.Error(err))
		return err
	}

	s.logger.Info("Question deleted",
		zap.String("question_id", questionID.String()))

	return nil
}

func (s *ontologyQuestionService) CreateQuestions(ctx context.Context, questions []*models.OntologyQuestion) error {
	if len(questions) == 0 {
		return nil
	}
	return s.questionRepo.CreateBatch(ctx, questions)
}
