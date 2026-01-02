package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ============================================================================
// Mock Repositories
// ============================================================================

type mockOntologyRepo struct {
	ontology *models.TieredOntology
	err      error
}

func (m *mockOntologyRepo) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return m.err
}

func (m *mockOntologyRepo) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ontology, nil
}

func (m *mockOntologyRepo) GetByVersion(ctx context.Context, projectID uuid.UUID, version int) (*models.TieredOntology, error) {
	return m.ontology, m.err
}

func (m *mockOntologyRepo) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return m.err
}

func (m *mockOntologyRepo) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	if m.ontology != nil && m.ontology.EntitySummaries != nil {
		m.ontology.EntitySummaries[tableName] = summary
	}
	return m.err
}

func (m *mockOntologyRepo) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return m.err
}

func (m *mockOntologyRepo) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	if m.ontology != nil && m.ontology.ColumnDetails != nil {
		m.ontology.ColumnDetails[tableName] = columns
	}
	return m.err
}

func (m *mockOntologyRepo) SetActive(ctx context.Context, projectID uuid.UUID, version int) error {
	return m.err
}

func (m *mockOntologyRepo) DeactivateAll(ctx context.Context, projectID uuid.UUID) error {
	return m.err
}

func (m *mockOntologyRepo) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, m.err
}

func (m *mockOntologyRepo) UpdateMetadata(ctx context.Context, projectID uuid.UUID, metadata map[string]any) error {
	if m.ontology != nil {
		if m.ontology.Metadata == nil {
			m.ontology.Metadata = make(map[string]any)
		}
		for k, v := range metadata {
			m.ontology.Metadata[k] = v
		}
	}
	return m.err
}

func (m *mockOntologyRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return m.err
}

func (m *mockOntologyRepo) WriteCleanOntology(ctx context.Context, projectID uuid.UUID) error {
	return m.err
}

type mockWorkflowStateRepo struct {
	questions   []models.WorkflowQuestion
	entityState *models.WorkflowEntityState
	workflow    *models.OntologyWorkflow
	states      []*models.WorkflowEntityState
	err         error
	answeredIDs map[string]string
}

func newMockWorkflowStateRepo() *mockWorkflowStateRepo {
	return &mockWorkflowStateRepo{
		answeredIDs: make(map[string]string),
	}
}

func (m *mockWorkflowStateRepo) Create(ctx context.Context, state *models.WorkflowEntityState) error {
	return m.err
}

func (m *mockWorkflowStateRepo) CreateBatch(ctx context.Context, states []*models.WorkflowEntityState) error {
	return m.err
}

func (m *mockWorkflowStateRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.WorkflowEntityState, error) {
	return m.entityState, m.err
}

func (m *mockWorkflowStateRepo) Update(ctx context.Context, state *models.WorkflowEntityState) error {
	return m.err
}

func (m *mockWorkflowStateRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return m.err
}

func (m *mockWorkflowStateRepo) ListByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*models.WorkflowEntityState, error) {
	return m.states, m.err
}

func (m *mockWorkflowStateRepo) ListByStatus(ctx context.Context, workflowID uuid.UUID, status models.WorkflowEntityStatus) ([]*models.WorkflowEntityState, error) {
	return m.states, m.err
}

func (m *mockWorkflowStateRepo) GetByEntity(ctx context.Context, workflowID uuid.UUID, entityType models.WorkflowEntityType, entityKey string) (*models.WorkflowEntityState, error) {
	return m.entityState, m.err
}

func (m *mockWorkflowStateRepo) DeleteByWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	return m.err
}

func (m *mockWorkflowStateRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return m.err
}

func (m *mockWorkflowStateRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status models.WorkflowEntityStatus, lastError *string) error {
	return m.err
}

func (m *mockWorkflowStateRepo) UpdateStateData(ctx context.Context, id uuid.UUID, stateData *models.WorkflowStateData) error {
	return m.err
}

func (m *mockWorkflowStateRepo) IncrementRetryCount(ctx context.Context, id uuid.UUID) error {
	return m.err
}

func (m *mockWorkflowStateRepo) AddQuestionsToEntity(ctx context.Context, id uuid.UUID, questions []models.WorkflowQuestion) error {
	return m.err
}

func (m *mockWorkflowStateRepo) UpdateQuestionInEntity(ctx context.Context, id uuid.UUID, questionID string, status string, answer string) error {
	if m.err != nil {
		return m.err
	}
	m.answeredIDs[questionID] = answer
	return nil
}

func (m *mockWorkflowStateRepo) RecordAnswerInEntity(ctx context.Context, id uuid.UUID, answer models.WorkflowAnswer) error {
	return m.err
}

func (m *mockWorkflowStateRepo) GetNextPendingQuestion(ctx context.Context, workflowID uuid.UUID) (*models.WorkflowQuestion, uuid.UUID, error) {
	if len(m.questions) > 0 {
		return &m.questions[0], uuid.New(), m.err
	}
	return nil, uuid.Nil, m.err
}

func (m *mockWorkflowStateRepo) GetPendingQuestions(ctx context.Context, projectID uuid.UUID, limit int) ([]models.WorkflowQuestion, error) {
	if m.err != nil {
		return nil, m.err
	}
	if limit > 0 && len(m.questions) > limit {
		return m.questions[:limit], nil
	}
	return m.questions, nil
}

func (m *mockWorkflowStateRepo) GetPendingQuestionsCount(ctx context.Context, workflowID uuid.UUID) (required int, optional int, err error) {
	return len(m.questions), 0, m.err
}

func (m *mockWorkflowStateRepo) GetPendingQuestionsCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return len(m.questions), m.err
}

func (m *mockWorkflowStateRepo) FindQuestionByID(ctx context.Context, questionID string) (*models.WorkflowQuestion, *models.WorkflowEntityState, *models.OntologyWorkflow, error) {
	if m.err != nil {
		return nil, nil, nil, m.err
	}
	for i := range m.questions {
		if m.questions[i].ID == questionID {
			return &m.questions[i], m.entityState, m.workflow, nil
		}
	}
	return nil, nil, nil, fmt.Errorf("question not found: %s", questionID)
}

// Ensure mockWorkflowStateRepo implements repositories.WorkflowStateRepository
var _ repositories.WorkflowStateRepository = (*mockWorkflowStateRepo)(nil)

type mockKnowledgeRepo struct {
	facts   []*models.KnowledgeFact
	fact    *models.KnowledgeFact
	err     error
	upserts []*models.KnowledgeFact
}

func (m *mockKnowledgeRepo) Upsert(ctx context.Context, fact *models.KnowledgeFact) error {
	if m.err != nil {
		return m.err
	}
	fact.ID = uuid.New()
	m.upserts = append(m.upserts, fact)
	return nil
}

func (m *mockKnowledgeRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	return m.facts, m.err
}

func (m *mockKnowledgeRepo) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	return m.facts, m.err
}

func (m *mockKnowledgeRepo) GetByKey(ctx context.Context, projectID uuid.UUID, factType, key string) (*models.KnowledgeFact, error) {
	return m.fact, m.err
}

func (m *mockKnowledgeRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return m.err
}

type mockSchemaRepo struct {
	tables  []*models.SchemaTable
	columns []*models.SchemaColumn
	err     error
}

func (m *mockSchemaRepo) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return m.tables, m.err
}

func (m *mockSchemaRepo) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	for _, t := range m.tables {
		if t.TableName == tableName {
			return t, nil
		}
	}
	return nil, m.err
}

func (m *mockSchemaRepo) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	return m.err
}

func (m *mockSchemaRepo) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []repositories.TableKey) (int64, error) {
	return 0, m.err
}

func (m *mockSchemaRepo) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	return m.err
}

func (m *mockSchemaRepo) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return m.err
}

func (m *mockSchemaRepo) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return m.columns, m.err
}

func (m *mockSchemaRepo) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return m.columns, m.err
}

func (m *mockSchemaRepo) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	return m.err
}

func (m *mockSchemaRepo) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	return 0, m.err
}

func (m *mockSchemaRepo) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	return m.err
}

func (m *mockSchemaRepo) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64) error {
	return m.err
}

func (m *mockSchemaRepo) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return m.err
}

func (m *mockSchemaRepo) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	return m.err
}

func (m *mockSchemaRepo) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	return m.err
}

func (m *mockSchemaRepo) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return m.err
}

func (m *mockSchemaRepo) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	return 0, m.err
}

func (m *mockSchemaRepo) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	return m.err
}

func (m *mockSchemaRepo) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return m.err
}

func (m *mockSchemaRepo) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) GetRelationshipCandidates(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.LegacyRelationshipCandidate, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	return nil, m.err
}

func (m *mockSchemaRepo) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string][]*models.SchemaColumn, error) {
	return make(map[string][]*models.SchemaColumn), m.err
}

func (m *mockSchemaRepo) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return len(m.columns), m.err
}

type mockQueryExecutor struct {
	result *datasource.QueryExecutionResult
	err    error
}

func (m *mockQueryExecutor) ExecuteQuery(ctx context.Context, sqlQuery string, limit int) (*datasource.QueryExecutionResult, error) {
	return m.result, m.err
}

func (m *mockQueryExecutor) ExecuteQueryWithParams(ctx context.Context, sqlQuery string, params []any, limit int) (*datasource.QueryExecutionResult, error) {
	return m.result, m.err
}

func (m *mockQueryExecutor) ValidateQuery(ctx context.Context, sqlQuery string) error {
	return m.err
}

func (m *mockQueryExecutor) Execute(ctx context.Context, sqlStatement string) (*datasource.ExecuteResult, error) {
	return nil, m.err
}

func (m *mockQueryExecutor) QuoteIdentifier(name string) string {
	return `"` + name + `"`
}

func (m *mockQueryExecutor) Close() error {
	return nil
}

// ============================================================================
// Tests
// ============================================================================

func TestOntologyToolExecutor_UnknownTool(t *testing.T) {
	executor := NewOntologyToolExecutor(&OntologyToolExecutorConfig{
		ProjectID: uuid.New(),
		Logger:    zap.NewNop(),
	})

	_, err := executor.ExecuteTool(context.Background(), "unknown_tool", "{}")
	if err == nil {
		t.Error("expected error for unknown tool")
	}
	if err.Error() != "unknown tool: unknown_tool" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestOntologyToolExecutor_StoreKnowledge(t *testing.T) {
	projectID := uuid.New()
	knowledgeRepo := &mockKnowledgeRepo{}

	executor := NewOntologyToolExecutor(&OntologyToolExecutorConfig{
		ProjectID:     projectID,
		KnowledgeRepo: knowledgeRepo,
		Logger:        zap.NewNop(),
	})

	args := `{"fact_type": "terminology", "key": "SKU", "value": "Stock Keeping Unit - unique product identifier"}`
	result, err := executor.ExecuteTool(context.Background(), "store_knowledge", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["success"] != true {
		t.Error("expected success to be true")
	}
	if response["fact_type"] != "terminology" {
		t.Errorf("expected fact_type 'terminology', got %v", response["fact_type"])
	}
	if response["key"] != "SKU" {
		t.Errorf("expected key 'SKU', got %v", response["key"])
	}

	// Verify the fact was stored
	if len(knowledgeRepo.upserts) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(knowledgeRepo.upserts))
	}
	if knowledgeRepo.upserts[0].Value != "Stock Keeping Unit - unique product identifier" {
		t.Errorf("unexpected value stored: %s", knowledgeRepo.upserts[0].Value)
	}
}

func TestOntologyToolExecutor_StoreKnowledge_InvalidFactType(t *testing.T) {
	executor := NewOntologyToolExecutor(&OntologyToolExecutorConfig{
		ProjectID:     uuid.New(),
		KnowledgeRepo: &mockKnowledgeRepo{},
		Logger:        zap.NewNop(),
	})

	args := `{"fact_type": "invalid_type", "key": "test", "value": "test value"}`
	_, err := executor.ExecuteTool(context.Background(), "store_knowledge", args)
	if err == nil {
		t.Error("expected error for invalid fact type")
	}
}

func TestOntologyToolExecutor_UpdateEntity(t *testing.T) {
	projectID := uuid.New()
	ontology := &models.TieredOntology{
		ID:              uuid.New(),
		ProjectID:       projectID,
		EntitySummaries: make(map[string]*models.EntitySummary),
		ColumnDetails:   make(map[string][]models.ColumnDetail),
	}
	ontologyRepo := &mockOntologyRepo{ontology: ontology}

	executor := NewOntologyToolExecutor(&OntologyToolExecutorConfig{
		ProjectID:    projectID,
		OntologyRepo: ontologyRepo,
		Logger:       zap.NewNop(),
	})

	args := `{"table_name": "orders", "business_name": "Sales Orders", "description": "Customer purchase orders", "domain": "sales"}`
	result, err := executor.ExecuteTool(context.Background(), "update_entity", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["success"] != true {
		t.Error("expected success to be true")
	}
	if response["table_name"] != "orders" {
		t.Errorf("expected table_name 'orders', got %v", response["table_name"])
	}

	// Verify entity was updated
	summary := ontology.EntitySummaries["orders"]
	if summary == nil {
		t.Fatal("expected entity summary to be created")
	}
	if summary.BusinessName != "Sales Orders" {
		t.Errorf("expected business_name 'Sales Orders', got %s", summary.BusinessName)
	}
	if summary.Domain != "sales" {
		t.Errorf("expected domain 'sales', got %s", summary.Domain)
	}
}

func TestOntologyToolExecutor_UpdateColumn(t *testing.T) {
	projectID := uuid.New()
	ontology := &models.TieredOntology{
		ID:              uuid.New(),
		ProjectID:       projectID,
		EntitySummaries: make(map[string]*models.EntitySummary),
		ColumnDetails:   make(map[string][]models.ColumnDetail),
	}
	ontologyRepo := &mockOntologyRepo{ontology: ontology}

	executor := NewOntologyToolExecutor(&OntologyToolExecutorConfig{
		ProjectID:    projectID,
		OntologyRepo: ontologyRepo,
		Logger:       zap.NewNop(),
	})

	args := `{"table_name": "orders", "column_name": "order_date", "description": "Date when order was placed", "semantic_type": "date"}`
	result, err := executor.ExecuteTool(context.Background(), "update_column", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["success"] != true {
		t.Error("expected success to be true")
	}

	// Verify column was updated
	columns := ontology.ColumnDetails["orders"]
	if len(columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(columns))
	}
	if columns[0].Name != "order_date" {
		t.Errorf("expected column name 'order_date', got %s", columns[0].Name)
	}
	if columns[0].SemanticType != "date" {
		t.Errorf("expected semantic type 'date', got %s", columns[0].SemanticType)
	}
}

func TestOntologyToolExecutor_AnswerQuestion(t *testing.T) {
	projectID := uuid.New()
	questionID := uuid.New().String()
	stateRepo := newMockWorkflowStateRepo()

	// Set up mock data
	stateRepo.questions = []models.WorkflowQuestion{
		{ID: questionID, Text: "What is SKU?", Status: "pending"},
	}
	stateRepo.entityState = &models.WorkflowEntityState{ID: uuid.New()}
	stateRepo.workflow = &models.OntologyWorkflow{ID: uuid.New()}

	executor := NewOntologyToolExecutor(&OntologyToolExecutorConfig{
		ProjectID: projectID,
		StateRepo: stateRepo,
		Logger:    zap.NewNop(),
	})

	args := `{"question_id": "` + questionID + `", "answer": "This is the answer"}`
	result, err := executor.ExecuteTool(context.Background(), "answer_question", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["success"] != true {
		t.Error("expected success to be true")
	}

	// Verify question was answered
	if answer, ok := stateRepo.answeredIDs[questionID]; !ok || answer != "This is the answer" {
		t.Errorf("question was not properly answered: %v", stateRepo.answeredIDs)
	}
}

func TestOntologyToolExecutor_GetPendingQuestions(t *testing.T) {
	projectID := uuid.New()
	stateRepo := newMockWorkflowStateRepo()
	stateRepo.questions = []models.WorkflowQuestion{
		{ID: uuid.New().String(), Text: "What is SKU?", Category: "terminology", Priority: 1, Status: "pending"},
		{ID: uuid.New().String(), Text: "What does order_status represent?", Category: "column", Priority: 2, Status: "pending"},
	}

	executor := NewOntologyToolExecutor(&OntologyToolExecutorConfig{
		ProjectID: projectID,
		StateRepo: stateRepo,
		Logger:    zap.NewNop(),
	})

	args := `{"limit": 5}`
	result, err := executor.ExecuteTool(context.Background(), "get_pending_questions", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	questions := response["questions"].([]any)
	if len(questions) != 2 {
		t.Errorf("expected 2 questions, got %d", len(questions))
	}

	count := response["count"].(float64)
	if count != 2 {
		t.Errorf("expected count 2, got %v", count)
	}
}

func TestOntologyToolExecutor_QuerySchemaMetadata(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()

	schemaRepo := &mockSchemaRepo{
		tables: []*models.SchemaTable{
			{
				ID:          tableID,
				ProjectID:   projectID,
				SchemaName:  "public",
				TableName:   "orders",
				RowCount:    ptr(int64(1000)),
				Description: ptr("Customer orders table"),
			},
		},
		columns: []*models.SchemaColumn{
			{ColumnName: "id", DataType: "integer", IsPrimaryKey: true},
			{ColumnName: "order_date", DataType: "date", IsNullable: false},
		},
	}

	executor := NewOntologyToolExecutor(&OntologyToolExecutorConfig{
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		SchemaRepo:   schemaRepo,
		Logger:       zap.NewNop(),
	})

	args := `{"table_name": "orders"}`
	result, err := executor.ExecuteTool(context.Background(), "query_schema_metadata", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	tables := response["tables"].([]any)
	if len(tables) != 1 {
		t.Errorf("expected 1 table, got %d", len(tables))
	}

	tableCount := response["table_count"].(float64)
	if tableCount != 1 {
		t.Errorf("expected table_count 1, got %v", tableCount)
	}
}

func TestOntologyToolExecutor_QueryColumnValues(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	queryExecutor := &mockQueryExecutor{
		result: &datasource.QueryExecutionResult{
			Columns:  []datasource.ColumnInfo{{Name: "status", Type: "TEXT"}},
			Rows:     []map[string]any{{"status": "pending"}, {"status": "shipped"}, {"status": "delivered"}},
			RowCount: 3,
		},
	}

	executor := NewOntologyToolExecutor(&OntologyToolExecutorConfig{
		ProjectID:     projectID,
		DatasourceID:  datasourceID,
		QueryExecutor: queryExecutor,
		Logger:        zap.NewNop(),
	})

	args := `{"table_name": "orders", "column_name": "status", "limit": 10}`
	result, err := executor.ExecuteTool(context.Background(), "query_column_values", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response map[string]any
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	values := response["values"].([]any)
	if len(values) != 3 {
		t.Errorf("expected 3 values, got %d", len(values))
	}

	if response["table"] != "orders" {
		t.Errorf("expected table 'orders', got %v", response["table"])
	}
	if response["column"] != "status" {
		t.Errorf("expected column 'status', got %v", response["column"])
	}
}

func TestOntologyToolExecutor_QueryColumnValues_NoExecutor(t *testing.T) {
	executor := NewOntologyToolExecutor(&OntologyToolExecutorConfig{
		ProjectID: uuid.New(),
		// No QueryExecutor
		Logger: zap.NewNop(),
	})

	args := `{"table_name": "orders", "column_name": "status"}`
	result, err := executor.ExecuteTool(context.Background(), "query_column_values", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return an error message in JSON, not fail
	var response map[string]any
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["error"] == nil {
		t.Error("expected error in response when no executor available")
	}
}

// Helper function
func ptr[T any](v T) *T {
	return &v
}
