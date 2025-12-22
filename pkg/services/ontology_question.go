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
	questionRepo  repositories.OntologyQuestionRepository
	ontologyRepo  repositories.OntologyRepository
	knowledgeRepo repositories.KnowledgeRepository
	builder       OntologyBuilderService
	logger        *zap.Logger
}

// NewOntologyQuestionService creates a new ontology question service.
func NewOntologyQuestionService(
	questionRepo repositories.OntologyQuestionRepository,
	ontologyRepo repositories.OntologyRepository,
	knowledgeRepo repositories.KnowledgeRepository,
	builder OntologyBuilderService,
	logger *zap.Logger,
) OntologyQuestionService {
	return &ontologyQuestionService{
		questionRepo:  questionRepo,
		ontologyRepo:  ontologyRepo,
		knowledgeRepo: knowledgeRepo,
		builder:       builder,
		logger:        logger.Named("ontology-question"),
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
		if err := s.knowledgeRepo.Upsert(ctx, fact); err != nil {
			s.logger.Error("Failed to store knowledge fact",
				zap.String("fact_type", fact.FactType),
				zap.String("key", fact.Key),
				zap.Error(err))
			return nil, fmt.Errorf("store knowledge fact %s/%s: %w", fact.FactType, fact.Key, err)
		}
	}

	// Create follow-up question if needed
	if processingResult.FollowUp != nil && *processingResult.FollowUp != "" {
		followUp := &models.OntologyQuestion{
			ProjectID:        question.ProjectID,
			OntologyID:       question.OntologyID,
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
func (s *ontologyQuestionService) applyEntityUpdates(ctx context.Context, projectID uuid.UUID, updates []EntityUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	// Get the active ontology to merge updates
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return fmt.Errorf("no active ontology for project %s", projectID.String())
	}

	for _, update := range updates {
		// Get existing summary or create new one
		existingSummary := ontology.EntitySummaries[update.TableName]
		if existingSummary == nil {
			existingSummary = &models.EntitySummary{
				TableName: update.TableName,
			}
		}

		// Merge updates into existing summary
		if update.BusinessName != nil {
			existingSummary.BusinessName = *update.BusinessName
		}
		if update.Description != nil {
			existingSummary.Description = *update.Description
		}
		if update.Domain != nil {
			existingSummary.Domain = *update.Domain
		}
		if len(update.Synonyms) > 0 {
			// Deduplicate synonyms
			existingSynonyms := make(map[string]bool)
			for _, syn := range existingSummary.Synonyms {
				existingSynonyms[syn] = true
			}
			for _, syn := range update.Synonyms {
				if !existingSynonyms[syn] {
					existingSummary.Synonyms = append(existingSummary.Synonyms, syn)
				}
			}
		}

		// Persist the update
		if err := s.ontologyRepo.UpdateEntitySummary(ctx, projectID, update.TableName, existingSummary); err != nil {
			return fmt.Errorf("update entity %s: %w", update.TableName, err)
		}
	}

	return nil
}

// applyColumnUpdates applies column updates from answer processing to the ontology.
func (s *ontologyQuestionService) applyColumnUpdates(ctx context.Context, projectID uuid.UUID, updates []ColumnUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	// Get the active ontology to merge updates
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return fmt.Errorf("no active ontology for project %s", projectID.String())
	}

	// Group updates by table for efficient batch updates
	updatesByTable := make(map[string][]ColumnUpdate)
	for _, update := range updates {
		updatesByTable[update.TableName] = append(updatesByTable[update.TableName], update)
	}

	for tableName, tableUpdates := range updatesByTable {
		// Get existing columns or create new slice
		columns := ontology.ColumnDetails[tableName]
		if columns == nil {
			columns = []models.ColumnDetail{}
		}

		// Apply each column update
		for _, update := range tableUpdates {
			found := false
			for i := range columns {
				if columns[i].Name == update.ColumnName {
					// Update existing column
					if update.Description != nil {
						columns[i].Description = *update.Description
					}
					if update.SemanticType != nil {
						columns[i].SemanticType = *update.SemanticType
					}
					if update.Role != nil {
						columns[i].Role = *update.Role
					}
					if len(update.Synonyms) > 0 {
						// Deduplicate synonyms
						existingSynonyms := make(map[string]bool)
						for _, syn := range columns[i].Synonyms {
							existingSynonyms[syn] = true
						}
						for _, syn := range update.Synonyms {
							if !existingSynonyms[syn] {
								columns[i].Synonyms = append(columns[i].Synonyms, syn)
							}
						}
					}
					found = true
					break
				}
			}

			if !found {
				// Create new column detail
				newColumn := models.ColumnDetail{
					Name: update.ColumnName,
				}
				if update.Description != nil {
					newColumn.Description = *update.Description
				}
				if update.SemanticType != nil {
					newColumn.SemanticType = *update.SemanticType
				}
				if update.Role != nil {
					newColumn.Role = *update.Role
				}
				if len(update.Synonyms) > 0 {
					newColumn.Synonyms = update.Synonyms
				}
				columns = append(columns, newColumn)
			}
		}

		// Persist the updates for this table
		if err := s.ontologyRepo.UpdateColumnDetails(ctx, projectID, tableName, columns); err != nil {
			return fmt.Errorf("update columns for %s: %w", tableName, err)
		}

		s.logger.Info("Applied column updates",
			zap.String("project_id", projectID.String()),
			zap.String("table_name", tableName),
			zap.Int("column_count", len(tableUpdates)))
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
