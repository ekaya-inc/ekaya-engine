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

func (m *mockQuestionRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockQuestionRepo) List(ctx context.Context, projectID uuid.UUID, filters repositories.QuestionListFilters) (*repositories.QuestionListResult, error) {
	return nil, nil
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

// mockSchemaRepoForQuestion implements only methods called by applyColumnUpdates.
type mockSchemaRepoForQuestion struct {
	repositories.SchemaRepository
	findTableByNameFunc func(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error)
	getColumnByNameFunc func(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error)
}

func (m *mockSchemaRepoForQuestion) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	if m.findTableByNameFunc != nil {
		return m.findTableByNameFunc(ctx, projectID, datasourceID, tableName)
	}
	return nil, nil
}

func (m *mockSchemaRepoForQuestion) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	if m.getColumnByNameFunc != nil {
		return m.getColumnByNameFunc(ctx, tableID, columnName)
	}
	return nil, nil
}

// mockColumnMetadataRepoForQuestion implements only methods called by applyColumnUpdates.
type mockColumnMetadataRepoForQuestion struct {
	repositories.ColumnMetadataRepository
	getBySchemaColumnIDFunc func(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error)
	upsertFunc              func(ctx context.Context, meta *models.ColumnMetadata) error
}

func (m *mockColumnMetadataRepoForQuestion) GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
	if m.getBySchemaColumnIDFunc != nil {
		return m.getBySchemaColumnIDFunc(ctx, schemaColumnID)
	}
	return nil, nil
}

func (m *mockColumnMetadataRepoForQuestion) Upsert(ctx context.Context, meta *models.ColumnMetadata) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, meta)
	}
	return nil
}

// --- Helper to create a service with mocks ---

func newTestQuestionService(
	questionRepo *mockQuestionRepo,
	knowledgeRepo *mockKnowledgeRepo,
	builder *mockBuilder,
) *ontologyQuestionService {
	return &ontologyQuestionService{
		questionRepo:  questionRepo,
		knowledgeRepo: knowledgeRepo,
		builder:       builder,
		logger:        zap.NewNop(),
	}
}

func newTestQuestionServiceWithRepos(
	questionRepo *mockQuestionRepo,
	knowledgeRepo *mockKnowledgeRepo,
	builder *mockBuilder,
	schemaRepo *mockSchemaRepoForQuestion,
	colMetaRepo *mockColumnMetadataRepoForQuestion,
) *ontologyQuestionService {
	return &ontologyQuestionService{
		questionRepo:       questionRepo,
		knowledgeRepo:      knowledgeRepo,
		builder:            builder,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: colMetaRepo,
		logger:             zap.NewNop(),
	}
}

// --- Tests for applyColumnUpdates (transformation logic) ---

func TestApplyColumnUpdates_EmptyUpdates(t *testing.T) {
	svc := newTestQuestionService(&mockQuestionRepo{}, &mockKnowledgeRepo{}, &mockBuilder{})
	err := svc.applyColumnUpdates(context.Background(), uuid.New(), nil)
	assert.NoError(t, err)
}

func TestApplyColumnUpdates_SkipsWhenTableNotFound(t *testing.T) {
	schemaRepo := &mockSchemaRepoForQuestion{
		findTableByNameFunc: func(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
			return nil, nil // table not found
		},
	}
	colMetaRepo := &mockColumnMetadataRepoForQuestion{}
	svc := newTestQuestionServiceWithRepos(&mockQuestionRepo{}, &mockKnowledgeRepo{}, &mockBuilder{}, schemaRepo, colMetaRepo)

	desc := "some description"
	err := svc.applyColumnUpdates(context.Background(), uuid.New(), []ColumnUpdate{
		{TableName: "nonexistent", ColumnName: "col1", Description: &desc},
	})
	assert.NoError(t, err, "should skip missing tables without error")
}

func TestApplyColumnUpdates_SkipsWhenSchemaRepoError(t *testing.T) {
	schemaRepo := &mockSchemaRepoForQuestion{
		findTableByNameFunc: func(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	colMetaRepo := &mockColumnMetadataRepoForQuestion{}
	svc := newTestQuestionServiceWithRepos(&mockQuestionRepo{}, &mockKnowledgeRepo{}, &mockBuilder{}, schemaRepo, colMetaRepo)

	desc := "some description"
	err := svc.applyColumnUpdates(context.Background(), uuid.New(), []ColumnUpdate{
		{TableName: "users", ColumnName: "col1", Description: &desc},
	})
	assert.NoError(t, err, "should skip on schema repo error without failing")
}

func TestApplyColumnUpdates_UpdateExistingColumn(t *testing.T) {
	projectID := uuid.New()
	tableID := uuid.New()
	colID := uuid.New()

	schemaRepo := &mockSchemaRepoForQuestion{
		findTableByNameFunc: func(ctx context.Context, pid, dsID uuid.UUID, tableName string) (*models.SchemaTable, error) {
			return &models.SchemaTable{ID: tableID, ProjectID: projectID, TableName: tableName}, nil
		},
		getColumnByNameFunc: func(ctx context.Context, tID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
			return &models.SchemaColumn{ID: colID, ProjectID: projectID, SchemaTableID: tableID, ColumnName: columnName}, nil
		},
	}

	existingDesc := "old description"
	existingMeta := &models.ColumnMetadata{
		ID:             uuid.New(),
		ProjectID:      projectID,
		SchemaColumnID: colID,
		Description:    &existingDesc,
		Source:         models.ProvenanceMCP,
	}

	var upserted *models.ColumnMetadata
	colMetaRepo := &mockColumnMetadataRepoForQuestion{
		getBySchemaColumnIDFunc: func(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
			return existingMeta, nil
		},
		upsertFunc: func(ctx context.Context, meta *models.ColumnMetadata) error {
			upserted = meta
			return nil
		},
	}

	svc := newTestQuestionServiceWithRepos(&mockQuestionRepo{}, &mockKnowledgeRepo{}, &mockBuilder{}, schemaRepo, colMetaRepo)

	newDesc := "updated description"
	err := svc.applyColumnUpdates(context.Background(), projectID, []ColumnUpdate{
		{TableName: "users", ColumnName: "email", Description: &newDesc},
	})
	require.NoError(t, err)
	require.NotNil(t, upserted)
	assert.Equal(t, "updated description", *upserted.Description)
	assert.Equal(t, existingMeta.ID, upserted.ID, "should update existing metadata, not create new")
}

func TestApplyColumnUpdates_CreateNewColumn(t *testing.T) {
	projectID := uuid.New()
	tableID := uuid.New()
	colID := uuid.New()

	schemaRepo := &mockSchemaRepoForQuestion{
		findTableByNameFunc: func(ctx context.Context, pid, dsID uuid.UUID, tableName string) (*models.SchemaTable, error) {
			return &models.SchemaTable{ID: tableID, ProjectID: projectID, TableName: tableName}, nil
		},
		getColumnByNameFunc: func(ctx context.Context, tID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
			return &models.SchemaColumn{ID: colID, ProjectID: projectID, SchemaTableID: tableID, ColumnName: columnName}, nil
		},
	}

	var upserted *models.ColumnMetadata
	colMetaRepo := &mockColumnMetadataRepoForQuestion{
		getBySchemaColumnIDFunc: func(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
			return nil, nil // no existing metadata
		},
		upsertFunc: func(ctx context.Context, meta *models.ColumnMetadata) error {
			upserted = meta
			return nil
		},
	}

	svc := newTestQuestionServiceWithRepos(&mockQuestionRepo{}, &mockKnowledgeRepo{}, &mockBuilder{}, schemaRepo, colMetaRepo)

	desc := "new column description"
	err := svc.applyColumnUpdates(context.Background(), projectID, []ColumnUpdate{
		{TableName: "users", ColumnName: "email", Description: &desc},
	})
	require.NoError(t, err)
	require.NotNil(t, upserted)
	assert.Equal(t, "new column description", *upserted.Description)
	assert.Equal(t, colID, upserted.SchemaColumnID)
	assert.Equal(t, projectID, upserted.ProjectID)
	assert.Equal(t, models.ProvenanceMCP, upserted.Source)
}

func TestApplyColumnUpdates_MultipleTablesGrouped(t *testing.T) {
	projectID := uuid.New()
	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	emailColID := uuid.New()
	amountColID := uuid.New()

	schemaRepo := &mockSchemaRepoForQuestion{
		findTableByNameFunc: func(ctx context.Context, pid, dsID uuid.UUID, tableName string) (*models.SchemaTable, error) {
			switch tableName {
			case "users":
				return &models.SchemaTable{ID: usersTableID, ProjectID: projectID, TableName: "users"}, nil
			case "orders":
				return &models.SchemaTable{ID: ordersTableID, ProjectID: projectID, TableName: "orders"}, nil
			}
			return nil, nil
		},
		getColumnByNameFunc: func(ctx context.Context, tID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
			switch {
			case tID == usersTableID && columnName == "email":
				return &models.SchemaColumn{ID: emailColID, SchemaTableID: usersTableID, ColumnName: "email"}, nil
			case tID == ordersTableID && columnName == "amount":
				return &models.SchemaColumn{ID: amountColID, SchemaTableID: ordersTableID, ColumnName: "amount"}, nil
			}
			return nil, nil
		},
	}

	var upsertedIDs []uuid.UUID
	colMetaRepo := &mockColumnMetadataRepoForQuestion{
		getBySchemaColumnIDFunc: func(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
			return nil, nil
		},
		upsertFunc: func(ctx context.Context, meta *models.ColumnMetadata) error {
			upsertedIDs = append(upsertedIDs, meta.SchemaColumnID)
			return nil
		},
	}

	svc := newTestQuestionServiceWithRepos(&mockQuestionRepo{}, &mockKnowledgeRepo{}, &mockBuilder{}, schemaRepo, colMetaRepo)

	emailDesc := "user email"
	amountDesc := "order amount"
	err := svc.applyColumnUpdates(context.Background(), projectID, []ColumnUpdate{
		{TableName: "users", ColumnName: "email", Description: &emailDesc},
		{TableName: "orders", ColumnName: "amount", Description: &amountDesc},
	})
	require.NoError(t, err)
	assert.Len(t, upsertedIDs, 2)
	assert.Contains(t, upsertedIDs, emailColID)
	assert.Contains(t, upsertedIDs, amountColID)
}

func TestApplyColumnUpdates_PartialUpdate(t *testing.T) {
	// Only updates the fields that are non-nil
	projectID := uuid.New()
	tableID := uuid.New()
	colID := uuid.New()

	schemaRepo := &mockSchemaRepoForQuestion{
		findTableByNameFunc: func(ctx context.Context, pid, dsID uuid.UUID, tableName string) (*models.SchemaTable, error) {
			return &models.SchemaTable{ID: tableID, ProjectID: projectID, TableName: tableName}, nil
		},
		getColumnByNameFunc: func(ctx context.Context, tID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
			return &models.SchemaColumn{ID: colID, ProjectID: projectID, SchemaTableID: tableID, ColumnName: columnName}, nil
		},
	}

	existingDesc := "original description"
	existingRole := "attribute"
	existingMeta := &models.ColumnMetadata{
		ID:             uuid.New(),
		ProjectID:      projectID,
		SchemaColumnID: colID,
		Description:    &existingDesc,
		Role:           &existingRole,
		Source:         models.ProvenanceMCP,
	}

	var upserted *models.ColumnMetadata
	colMetaRepo := &mockColumnMetadataRepoForQuestion{
		getBySchemaColumnIDFunc: func(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
			return existingMeta, nil
		},
		upsertFunc: func(ctx context.Context, meta *models.ColumnMetadata) error {
			upserted = meta
			return nil
		},
	}

	svc := newTestQuestionServiceWithRepos(&mockQuestionRepo{}, &mockKnowledgeRepo{}, &mockBuilder{}, schemaRepo, colMetaRepo)

	// Only update SemanticType, leave Description and Role untouched
	semType := "email_address"
	err := svc.applyColumnUpdates(context.Background(), projectID, []ColumnUpdate{
		{TableName: "users", ColumnName: "email", SemanticType: &semType},
	})
	require.NoError(t, err)
	require.NotNil(t, upserted)
	assert.Equal(t, "email_address", *upserted.SemanticType)
	assert.Equal(t, "original description", *upserted.Description, "should preserve existing description")
	assert.Equal(t, "attribute", *upserted.Role, "should preserve existing role")
}

func TestApplyColumnUpdates_RepoUpdateError(t *testing.T) {
	projectID := uuid.New()
	tableID := uuid.New()
	colID := uuid.New()

	schemaRepo := &mockSchemaRepoForQuestion{
		findTableByNameFunc: func(ctx context.Context, pid, dsID uuid.UUID, tableName string) (*models.SchemaTable, error) {
			return &models.SchemaTable{ID: tableID, ProjectID: projectID, TableName: tableName}, nil
		},
		getColumnByNameFunc: func(ctx context.Context, tID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
			return &models.SchemaColumn{ID: colID, ProjectID: projectID, SchemaTableID: tableID, ColumnName: columnName}, nil
		},
	}

	colMetaRepo := &mockColumnMetadataRepoForQuestion{
		getBySchemaColumnIDFunc: func(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
			return nil, nil
		},
		upsertFunc: func(ctx context.Context, meta *models.ColumnMetadata) error {
			return fmt.Errorf("upsert failed")
		},
	}

	svc := newTestQuestionServiceWithRepos(&mockQuestionRepo{}, &mockKnowledgeRepo{}, &mockBuilder{}, schemaRepo, colMetaRepo)

	desc := "some description"
	err := svc.applyColumnUpdates(context.Background(), projectID, []ColumnUpdate{
		{TableName: "users", ColumnName: "email", Description: &desc},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upsert column metadata")
}

// --- Tests for AnswerQuestion validation ---

func TestAnswerQuestion_QuestionNotFound(t *testing.T) {
	questionRepo := &mockQuestionRepo{
		getByIDFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
			return nil, nil
		},
	}
	svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, &mockBuilder{})

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
	svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, &mockBuilder{})

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
			svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, &mockBuilder{})

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
			svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, builder)

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
	followUpText := "Can you clarify?"

	var createdFollowUp *models.OntologyQuestion
	questionRepo := &mockQuestionRepo{
		getByIDFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyQuestion, error) {
			return &models.OntologyQuestion{
				ID:        questionID,
				ProjectID: projectID,
				Status:    models.QuestionStatusPending,
				Priority:  2,
				Affects:   &models.QuestionAffects{Tables: []string{"users"}},
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
	svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, builder)

	result, err := svc.AnswerQuestion(context.Background(), questionID, "yes", "")
	require.NoError(t, err)

	require.NotNil(t, createdFollowUp, "follow-up question should be created")
	assert.Equal(t, followUpText, createdFollowUp.Text)
	assert.Equal(t, projectID, createdFollowUp.ProjectID)
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
	svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, builder)

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
	svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, builder)

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
	svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, builder)

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
	svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, &mockBuilder{})

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
	svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, &mockBuilder{})

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
	svc := newTestQuestionService(questionRepo, &mockKnowledgeRepo{}, &mockBuilder{})

	_, err := svc.GetPendingCount(context.Background(), uuid.New())
	assert.Error(t, err)
}
