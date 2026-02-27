package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// --- Mocks (only methods actually called by the service) ---

type mockQuestionRepo struct {
	getByIDFunc          func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error)
	getNextPendingFunc   func(ctx context.Context, projectID uuid.UUID) (*models.OntologyQuestion, error)
	getPendingCountsFunc func(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error)
	submitAnswerFunc     func(ctx context.Context, id uuid.UUID, answer string, answeredBy *uuid.UUID) error
	createFunc           func(ctx context.Context, question *models.OntologyQuestion) error
	updateStatusFunc     func(ctx context.Context, id uuid.UUID, status models.QuestionStatus) error
	createBatchFunc      func(ctx context.Context, questions []*models.OntologyQuestion) error
	listPendingFunc      func(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error)
}

func (m *mockQuestionRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockQuestionRepo) GetNextPending(ctx context.Context, projectID uuid.UUID) (*models.OntologyQuestion, error) {
	if m.getNextPendingFunc != nil {
		return m.getNextPendingFunc(ctx, projectID)
	}
	return nil, nil
}

func (m *mockQuestionRepo) GetPendingCounts(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error) {
	if m.getPendingCountsFunc != nil {
		return m.getPendingCountsFunc(ctx, projectID)
	}
	return &repositories.QuestionCounts{}, nil
}

func (m *mockQuestionRepo) SubmitAnswer(ctx context.Context, id uuid.UUID, answer string, answeredBy *uuid.UUID) error {
	if m.submitAnswerFunc != nil {
		return m.submitAnswerFunc(ctx, id, answer, answeredBy)
	}
	return nil
}

func (m *mockQuestionRepo) Create(ctx context.Context, question *models.OntologyQuestion) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, question)
	}
	return nil
}

func (m *mockQuestionRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status models.QuestionStatus) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, id, status)
	}
	return nil
}

func (m *mockQuestionRepo) CreateBatch(ctx context.Context, questions []*models.OntologyQuestion) error {
	if m.createBatchFunc != nil {
		return m.createBatchFunc(ctx, questions)
	}
	return nil
}

func (m *mockQuestionRepo) ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error) {
	if m.listPendingFunc != nil {
		return m.listPendingFunc(ctx, projectID)
	}
	return nil, nil
}

func (m *mockQuestionRepo) UpdateStatusWithReason(ctx context.Context, id uuid.UUID, status models.QuestionStatus, reason string) error {
	return nil
}

func (m *mockQuestionRepo) ListByOntologyID(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyQuestion, error) {
	return nil, nil
}

func (m *mockQuestionRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockQuestionRepo) List(ctx context.Context, projectID uuid.UUID, filters repositories.QuestionListFilters) (*repositories.QuestionListResult, error) {
	return nil, nil
}

type mockOntologyRepo struct {
	getActiveFunc           func(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error)
	updateColumnDetailsFunc func(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error
}

func (m *mockOntologyRepo) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	if m.getActiveFunc != nil {
		return m.getActiveFunc(ctx, projectID)
	}
	return nil, nil
}

func (m *mockOntologyRepo) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	if m.updateColumnDetailsFunc != nil {
		return m.updateColumnDetailsFunc(ctx, projectID, tableName, columns)
	}
	return nil
}

func (m *mockOntologyRepo) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}

func (m *mockOntologyRepo) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return nil
}

func (m *mockOntologyRepo) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockOntologyRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

type mockKnowledgeRepo struct {
	createFunc func(ctx context.Context, fact *models.KnowledgeFact) error
}

func (m *mockKnowledgeRepo) Create(ctx context.Context, fact *models.KnowledgeFact) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, fact)
	}
	return nil
}

func (m *mockKnowledgeRepo) Update(ctx context.Context, fact *models.KnowledgeFact) error {
	return nil
}

func (m *mockKnowledgeRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	return nil, nil
}

func (m *mockKnowledgeRepo) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	return nil, nil
}

func (m *mockKnowledgeRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockKnowledgeRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockKnowledgeRepo) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}

type mockBuilder struct {
	processAnswerFunc func(ctx context.Context, projectID uuid.UUID, question *models.OntologyQuestion, answer string) (*AnswerProcessingResult, error)
}

func (m *mockBuilder) ProcessAnswer(ctx context.Context, projectID uuid.UUID, question *models.OntologyQuestion, answer string) (*AnswerProcessingResult, error) {
	if m.processAnswerFunc != nil {
		return m.processAnswerFunc(ctx, projectID, question, answer)
	}
	return &AnswerProcessingResult{}, nil
}

// --- Helper to create a service with mocks ---

func newTestQuestionService(
	questionRepo *mockQuestionRepo,
	ontologyRepo *mockOntologyRepo,
	knowledgeRepo *mockKnowledgeRepo,
	builder *mockBuilder,
) *ontologyQuestionService {
	return &ontologyQuestionService{
		questionRepo:  questionRepo,
		ontologyRepo:  ontologyRepo,
		knowledgeRepo: knowledgeRepo,
		builder:       builder,
		logger:        zap.NewNop(),
	}
}

// --- Tests for applyColumnUpdates (transformation logic) ---

func TestApplyColumnUpdates_EmptyUpdates(t *testing.T) {
	svc := newTestQuestionService(&mockQuestionRepo{}, &mockOntologyRepo{}, &mockKnowledgeRepo{}, &mockBuilder{})
	err := svc.applyColumnUpdates(context.Background(), uuid.New(), nil)
	assert.NoError(t, err)
}

func TestApplyColumnUpdates_NoActiveOntology(t *testing.T) {
	ontologyRepo := &mockOntologyRepo{
		getActiveFunc: func(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
			return nil, nil
		},
	}
	svc := newTestQuestionService(&mockQuestionRepo{}, ontologyRepo, &mockKnowledgeRepo{}, &mockBuilder{})

	err := svc.applyColumnUpdates(context.Background(), uuid.New(), []ColumnUpdate{
		{TableName: "users", ColumnName: "email"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active ontology")
}

func TestApplyColumnUpdates_GetActiveError(t *testing.T) {
	ontologyRepo := &mockOntologyRepo{
		getActiveFunc: func(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
			return nil, fmt.Errorf("db connection lost")
		},
	}
	svc := newTestQuestionService(&mockQuestionRepo{}, ontologyRepo, &mockKnowledgeRepo{}, &mockBuilder{})

	err := svc.applyColumnUpdates(context.Background(), uuid.New(), []ColumnUpdate{
		{TableName: "users", ColumnName: "email"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get active ontology")
}

func TestApplyColumnUpdates_UpdateExistingColumn(t *testing.T) {
	projectID := uuid.New()
	desc := "User email address"
	semType := "email"
	role := "identifier"

	var capturedColumns []models.ColumnDetail
	ontologyRepo := &mockOntologyRepo{
		getActiveFunc: func(ctx context.Context, pid uuid.UUID) (*models.TieredOntology, error) {
			return &models.TieredOntology{
				ProjectID: projectID,
				ColumnDetails: map[string][]models.ColumnDetail{
					"users": {
						{Name: "email", Description: "old desc", Synonyms: []string{"mail"}},
						{Name: "name", Description: "user name"},
					},
				},
			}, nil
		},
		updateColumnDetailsFunc: func(ctx context.Context, pid uuid.UUID, tableName string, columns []models.ColumnDetail) error {
			capturedColumns = columns
			return nil
		},
	}
	svc := newTestQuestionService(&mockQuestionRepo{}, ontologyRepo, &mockKnowledgeRepo{}, &mockBuilder{})

	err := svc.applyColumnUpdates(context.Background(), projectID, []ColumnUpdate{
		{
			TableName:    "users",
			ColumnName:   "email",
			Description:  &desc,
			SemanticType: &semType,
			Role:         &role,
			Synonyms:     []string{"mail", "e-mail"}, // "mail" already exists, should be deduped
		},
	})
	require.NoError(t, err)
	require.Len(t, capturedColumns, 2, "should still have 2 columns")

	// Find the updated email column
	var emailCol models.ColumnDetail
	for _, c := range capturedColumns {
		if c.Name == "email" {
			emailCol = c
			break
		}
	}
	assert.Equal(t, "User email address", emailCol.Description)
	assert.Equal(t, "email", emailCol.SemanticType)
	assert.Equal(t, "identifier", emailCol.Role)
	assert.ElementsMatch(t, []string{"mail", "e-mail"}, emailCol.Synonyms, "synonyms should be deduplicated")
}

func TestApplyColumnUpdates_CreateNewColumn(t *testing.T) {
	projectID := uuid.New()
	desc := "User age"

	var capturedColumns []models.ColumnDetail
	ontologyRepo := &mockOntologyRepo{
		getActiveFunc: func(ctx context.Context, pid uuid.UUID) (*models.TieredOntology, error) {
			return &models.TieredOntology{
				ProjectID:     projectID,
				ColumnDetails: map[string][]models.ColumnDetail{},
			}, nil
		},
		updateColumnDetailsFunc: func(ctx context.Context, pid uuid.UUID, tableName string, columns []models.ColumnDetail) error {
			capturedColumns = columns
			return nil
		},
	}
	svc := newTestQuestionService(&mockQuestionRepo{}, ontologyRepo, &mockKnowledgeRepo{}, &mockBuilder{})

	err := svc.applyColumnUpdates(context.Background(), projectID, []ColumnUpdate{
		{TableName: "users", ColumnName: "age", Description: &desc, Synonyms: []string{"years"}},
	})
	require.NoError(t, err)
	require.Len(t, capturedColumns, 1)
	assert.Equal(t, "age", capturedColumns[0].Name)
	assert.Equal(t, "User age", capturedColumns[0].Description)
	assert.Equal(t, []string{"years"}, capturedColumns[0].Synonyms)
}

func TestApplyColumnUpdates_MultipleTablesGrouped(t *testing.T) {
	projectID := uuid.New()
	desc1 := "desc1"
	desc2 := "desc2"

	updatedTables := map[string]bool{}
	ontologyRepo := &mockOntologyRepo{
		getActiveFunc: func(ctx context.Context, pid uuid.UUID) (*models.TieredOntology, error) {
			return &models.TieredOntology{
				ProjectID:     projectID,
				ColumnDetails: map[string][]models.ColumnDetail{},
			}, nil
		},
		updateColumnDetailsFunc: func(ctx context.Context, pid uuid.UUID, tableName string, columns []models.ColumnDetail) error {
			updatedTables[tableName] = true
			return nil
		},
	}
	svc := newTestQuestionService(&mockQuestionRepo{}, ontologyRepo, &mockKnowledgeRepo{}, &mockBuilder{})

	err := svc.applyColumnUpdates(context.Background(), projectID, []ColumnUpdate{
		{TableName: "users", ColumnName: "email", Description: &desc1},
		{TableName: "orders", ColumnName: "total", Description: &desc2},
	})
	require.NoError(t, err)
	assert.True(t, updatedTables["users"], "users table should be updated")
	assert.True(t, updatedTables["orders"], "orders table should be updated")
}

func TestApplyColumnUpdates_PartialUpdate(t *testing.T) {
	// Only set description, leave other fields unchanged
	projectID := uuid.New()
	desc := "Updated description"

	var capturedColumns []models.ColumnDetail
	ontologyRepo := &mockOntologyRepo{
		getActiveFunc: func(ctx context.Context, pid uuid.UUID) (*models.TieredOntology, error) {
			return &models.TieredOntology{
				ProjectID: projectID,
				ColumnDetails: map[string][]models.ColumnDetail{
					"users": {{Name: "email", SemanticType: "email", Role: "identifier"}},
				},
			}, nil
		},
		updateColumnDetailsFunc: func(ctx context.Context, pid uuid.UUID, tableName string, columns []models.ColumnDetail) error {
			capturedColumns = columns
			return nil
		},
	}
	svc := newTestQuestionService(&mockQuestionRepo{}, ontologyRepo, &mockKnowledgeRepo{}, &mockBuilder{})

	err := svc.applyColumnUpdates(context.Background(), projectID, []ColumnUpdate{
		{TableName: "users", ColumnName: "email", Description: &desc},
	})
	require.NoError(t, err)
	require.Len(t, capturedColumns, 1)
	assert.Equal(t, "Updated description", capturedColumns[0].Description)
	assert.Equal(t, "email", capturedColumns[0].SemanticType, "should be unchanged")
	assert.Equal(t, "identifier", capturedColumns[0].Role, "should be unchanged")
}

func TestApplyColumnUpdates_RepoUpdateError(t *testing.T) {
	projectID := uuid.New()
	desc := "desc"

	ontologyRepo := &mockOntologyRepo{
		getActiveFunc: func(ctx context.Context, pid uuid.UUID) (*models.TieredOntology, error) {
			return &models.TieredOntology{
				ProjectID:     projectID,
				ColumnDetails: map[string][]models.ColumnDetail{},
			}, nil
		},
		updateColumnDetailsFunc: func(ctx context.Context, pid uuid.UUID, tableName string, columns []models.ColumnDetail) error {
			return fmt.Errorf("write failed")
		},
	}
	svc := newTestQuestionService(&mockQuestionRepo{}, ontologyRepo, &mockKnowledgeRepo{}, &mockBuilder{})

	err := svc.applyColumnUpdates(context.Background(), projectID, []ColumnUpdate{
		{TableName: "users", ColumnName: "email", Description: &desc},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update columns for users")
}

// --- Tests for AnswerQuestion validation ---

func TestAnswerQuestion_QuestionNotFound(t *testing.T) {
	questionRepo := &mockQuestionRepo{
		getByIDFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
			return nil, nil
		},
	}
	svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, &mockBuilder{})

	_, err := svc.AnswerQuestion(context.Background(), uuid.New(), "yes", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "question not found")
}

func TestAnswerQuestion_QuestionGetError(t *testing.T) {
	questionRepo := &mockQuestionRepo{
		getByIDFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, &mockBuilder{})

	_, err := svc.AnswerQuestion(context.Background(), uuid.New(), "yes", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestAnswerQuestion_WrongStatus(t *testing.T) {
	for _, status := range []models.QuestionStatus{
		models.QuestionStatusAnswered,
		models.QuestionStatusDeleted,
		models.QuestionStatusEscalated,
		models.QuestionStatusDismissed,
	} {
		t.Run(string(status), func(t *testing.T) {
			questionRepo := &mockQuestionRepo{
				getByIDFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
					return &models.OntologyQuestion{
						ID:        id,
						ProjectID: uuid.New(),
						Status:    status,
					}, nil
				},
			}
			svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, &mockBuilder{})

			_, err := svc.AnswerQuestion(context.Background(), uuid.New(), "yes", "")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "not in a pending state")
		})
	}
}

func TestAnswerQuestion_AcceptsPendingAndSkipped(t *testing.T) {
	for _, status := range []models.QuestionStatus{
		models.QuestionStatusPending,
		models.QuestionStatusSkipped,
	} {
		t.Run(string(status), func(t *testing.T) {
			projectID := uuid.New()
			questionID := uuid.New()

			questionRepo := &mockQuestionRepo{
				getByIDFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
					return &models.OntologyQuestion{
						ID:        questionID,
						ProjectID: projectID,
						Status:    status,
					}, nil
				},
			}
			builder := &mockBuilder{
				processAnswerFunc: func(ctx context.Context, pid uuid.UUID, q *models.OntologyQuestion, answer string) (*AnswerProcessingResult, error) {
					return &AnswerProcessingResult{
						ActionsSummary: "done",
					}, nil
				},
			}
			svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, builder)

			result, err := svc.AnswerQuestion(context.Background(), questionID, "yes", "")
			require.NoError(t, err)
			assert.Equal(t, questionID, result.QuestionID)
			assert.Equal(t, "done", result.ActionsSummary)
			assert.True(t, result.AllComplete, "no next question means all complete")
		})
	}
}

func TestAnswerQuestion_FollowUpCreated(t *testing.T) {
	projectID := uuid.New()
	questionID := uuid.New()
	ontologyID := uuid.New()
	followUpText := "Can you clarify?"

	var createdFollowUp *models.OntologyQuestion
	questionRepo := &mockQuestionRepo{
		getByIDFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
			return &models.OntologyQuestion{
				ID:         questionID,
				ProjectID:  projectID,
				OntologyID: ontologyID,
				Status:     models.QuestionStatusPending,
				Priority:   2,
				Affects:    &models.QuestionAffects{Tables: []string{"users"}},
			}, nil
		},
		createFunc: func(ctx context.Context, q *models.OntologyQuestion) error {
			createdFollowUp = q
			return nil
		},
	}
	builder := &mockBuilder{
		processAnswerFunc: func(ctx context.Context, pid uuid.UUID, q *models.OntologyQuestion, answer string) (*AnswerProcessingResult, error) {
			return &AnswerProcessingResult{
				FollowUp: &followUpText,
			}, nil
		},
	}
	svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, builder)

	result, err := svc.AnswerQuestion(context.Background(), questionID, "yes", "")
	require.NoError(t, err)

	require.NotNil(t, createdFollowUp, "follow-up question should be created")
	assert.Equal(t, followUpText, createdFollowUp.Text)
	assert.Equal(t, projectID, createdFollowUp.ProjectID)
	assert.Equal(t, ontologyID, createdFollowUp.OntologyID)
	assert.Equal(t, 2, createdFollowUp.Priority, "should inherit parent priority")
	assert.False(t, createdFollowUp.IsRequired, "follow-ups are optional")
	assert.Equal(t, "follow_up", createdFollowUp.Category)
	assert.Equal(t, models.QuestionStatusPending, createdFollowUp.Status)
	assert.Equal(t, &questionID, createdFollowUp.ParentQuestionID)
	assert.NotNil(t, result.FollowUp)
}

func TestAnswerQuestion_NoFollowUpWhenNil(t *testing.T) {
	projectID := uuid.New()
	questionID := uuid.New()

	createCalled := false
	questionRepo := &mockQuestionRepo{
		getByIDFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
			return &models.OntologyQuestion{
				ID:        questionID,
				ProjectID: projectID,
				Status:    models.QuestionStatusPending,
			}, nil
		},
		createFunc: func(ctx context.Context, q *models.OntologyQuestion) error {
			createCalled = true
			return nil
		},
	}
	builder := &mockBuilder{
		processAnswerFunc: func(ctx context.Context, pid uuid.UUID, q *models.OntologyQuestion, answer string) (*AnswerProcessingResult, error) {
			return &AnswerProcessingResult{FollowUp: nil}, nil
		},
	}
	svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, builder)

	_, err := svc.AnswerQuestion(context.Background(), questionID, "yes", "")
	require.NoError(t, err)
	assert.False(t, createCalled, "should not create follow-up when FollowUp is nil")
}

func TestAnswerQuestion_NoFollowUpWhenEmpty(t *testing.T) {
	projectID := uuid.New()
	questionID := uuid.New()
	empty := ""

	createCalled := false
	questionRepo := &mockQuestionRepo{
		getByIDFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
			return &models.OntologyQuestion{
				ID:        questionID,
				ProjectID: projectID,
				Status:    models.QuestionStatusPending,
			}, nil
		},
		createFunc: func(ctx context.Context, q *models.OntologyQuestion) error {
			createCalled = true
			return nil
		},
	}
	builder := &mockBuilder{
		processAnswerFunc: func(ctx context.Context, pid uuid.UUID, q *models.OntologyQuestion, answer string) (*AnswerProcessingResult, error) {
			return &AnswerProcessingResult{FollowUp: &empty}, nil
		},
	}
	svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, builder)

	_, err := svc.AnswerQuestion(context.Background(), questionID, "yes", "")
	require.NoError(t, err)
	assert.False(t, createCalled, "should not create follow-up when FollowUp is empty string")
}

func TestAnswerQuestion_UserIDParsing(t *testing.T) {
	projectID := uuid.New()
	questionID := uuid.New()
	userUUID := uuid.New()

	var capturedAnsweredBy *uuid.UUID
	questionRepo := &mockQuestionRepo{
		getByIDFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
			return &models.OntologyQuestion{
				ID:        questionID,
				ProjectID: projectID,
				Status:    models.QuestionStatusPending,
			}, nil
		},
		submitAnswerFunc: func(ctx context.Context, id uuid.UUID, answer string, answeredBy *uuid.UUID) error {
			capturedAnsweredBy = answeredBy
			return nil
		},
	}
	builder := &mockBuilder{
		processAnswerFunc: func(ctx context.Context, pid uuid.UUID, q *models.OntologyQuestion, answer string) (*AnswerProcessingResult, error) {
			return &AnswerProcessingResult{}, nil
		},
	}
	svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, builder)

	t.Run("valid UUID", func(t *testing.T) {
		capturedAnsweredBy = nil
		_, err := svc.AnswerQuestion(context.Background(), questionID, "yes", userUUID.String())
		require.NoError(t, err)
		require.NotNil(t, capturedAnsweredBy)
		assert.Equal(t, userUUID, *capturedAnsweredBy)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		capturedAnsweredBy = nil
		_, err := svc.AnswerQuestion(context.Background(), questionID, "yes", "not-a-uuid")
		require.NoError(t, err)
		assert.Nil(t, capturedAnsweredBy, "invalid UUID should result in nil answeredBy")
	})

	t.Run("empty string", func(t *testing.T) {
		capturedAnsweredBy = nil
		_, err := svc.AnswerQuestion(context.Background(), questionID, "yes", "")
		require.NoError(t, err)
		assert.Nil(t, capturedAnsweredBy, "empty userID should result in nil answeredBy")
	})
}

// --- Tests for GetPendingCount ---

func TestGetPendingCount_SumsRequiredAndOptional(t *testing.T) {
	questionRepo := &mockQuestionRepo{
		getPendingCountsFunc: func(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error) {
			return &repositories.QuestionCounts{Required: 3, Optional: 7}, nil
		},
	}
	svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, &mockBuilder{})

	count, err := svc.GetPendingCount(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, 10, count)
}

func TestGetPendingCount_NilCounts(t *testing.T) {
	questionRepo := &mockQuestionRepo{
		getPendingCountsFunc: func(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error) {
			return nil, nil
		},
	}
	svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, &mockBuilder{})

	count, err := svc.GetPendingCount(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestGetPendingCount_Error(t *testing.T) {
	questionRepo := &mockQuestionRepo{
		getPendingCountsFunc: func(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	svc := newTestQuestionService(questionRepo, &mockOntologyRepo{}, &mockKnowledgeRepo{}, &mockBuilder{})

	_, err := svc.GetPendingCount(context.Background(), uuid.New())
	assert.Error(t, err)
}
