package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// ============================================================================
// Mock Implementations for RelationshipWorkflowService Tests
// ============================================================================

// rwsMockWorkflowRepository is a mock for OntologyWorkflowRepository.
type rwsMockWorkflowRepository struct {
	workflow             *models.OntologyWorkflow
	createErr            error
	getByIDErr           error
	getLatestErr         error
	updateStateErr       error
	updateProgressErr    error
	updateTaskQueueErr   error
	deleteErr            error
	claimOwnershipResult bool
	claimOwnershipErr    error
	updateHeartbeatErr   error
	releaseOwnershipErr  error

	// Capture for verification
	createdWorkflow  *models.OntologyWorkflow
	updatedState     models.WorkflowState
	updatedProgress  *models.WorkflowProgress
	updatedTaskQueue []models.WorkflowTask
}

func (m *rwsMockWorkflowRepository) Create(ctx context.Context, workflow *models.OntologyWorkflow) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.createdWorkflow = workflow
	return nil
}

func (m *rwsMockWorkflowRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyWorkflow, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	return m.workflow, nil
}

func (m *rwsMockWorkflowRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) (*models.OntologyWorkflow, error) {
	return m.workflow, nil
}

func (m *rwsMockWorkflowRepository) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyWorkflow, error) {
	if m.getLatestErr != nil {
		return nil, m.getLatestErr
	}
	return m.workflow, nil
}

func (m *rwsMockWorkflowRepository) GetLatestByDatasourceAndPhase(ctx context.Context, datasourceID uuid.UUID, phase models.WorkflowPhaseType) (*models.OntologyWorkflow, error) {
	if m.getLatestErr != nil {
		return nil, m.getLatestErr
	}
	return m.workflow, nil
}

func (m *rwsMockWorkflowRepository) Update(ctx context.Context, workflow *models.OntologyWorkflow) error {
	return nil
}

func (m *rwsMockWorkflowRepository) UpdateState(ctx context.Context, id uuid.UUID, state models.WorkflowState, errorMsg string) error {
	if m.updateStateErr != nil {
		return m.updateStateErr
	}
	m.updatedState = state
	return nil
}

func (m *rwsMockWorkflowRepository) UpdateProgress(ctx context.Context, id uuid.UUID, progress *models.WorkflowProgress) error {
	if m.updateProgressErr != nil {
		return m.updateProgressErr
	}
	m.updatedProgress = progress
	return nil
}

func (m *rwsMockWorkflowRepository) UpdateTaskQueue(ctx context.Context, id uuid.UUID, tasks []models.WorkflowTask) error {
	if m.updateTaskQueueErr != nil {
		return m.updateTaskQueueErr
	}
	m.updatedTaskQueue = tasks
	return nil
}

func (m *rwsMockWorkflowRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return nil
}

func (m *rwsMockWorkflowRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *rwsMockWorkflowRepository) ClaimOwnership(ctx context.Context, workflowID, ownerID uuid.UUID) (bool, error) {
	if m.claimOwnershipErr != nil {
		return false, m.claimOwnershipErr
	}
	return m.claimOwnershipResult, nil
}

func (m *rwsMockWorkflowRepository) UpdateHeartbeat(ctx context.Context, workflowID, ownerID uuid.UUID) error {
	if m.updateHeartbeatErr != nil {
		return m.updateHeartbeatErr
	}
	return nil
}

func (m *rwsMockWorkflowRepository) ReleaseOwnership(ctx context.Context, workflowID uuid.UUID) error {
	if m.releaseOwnershipErr != nil {
		return m.releaseOwnershipErr
	}
	return nil
}

// rwsMockCandidateRepository is a mock for RelationshipCandidateRepository.
type rwsMockCandidateRepository struct {
	candidates          []*models.RelationshipCandidate
	createErr           error
	getByIDErr          error
	getByWorkflowErr    error
	updateErr           error
	deleteErr           error
	countRequiredResult int
	countRequiredErr    error

	// Capture for verification
	createdCandidates []*models.RelationshipCandidate
}

func (m *rwsMockCandidateRepository) Create(ctx context.Context, candidate *models.RelationshipCandidate) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.createdCandidates = append(m.createdCandidates, candidate)
	return nil
}

func (m *rwsMockCandidateRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.RelationshipCandidate, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	for _, c := range m.candidates {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, errors.New("candidate not found")
}

func (m *rwsMockCandidateRepository) GetByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*models.RelationshipCandidate, error) {
	if m.getByWorkflowErr != nil {
		return nil, m.getByWorkflowErr
	}
	return m.candidates, nil
}

func (m *rwsMockCandidateRepository) GetByWorkflowWithNames(ctx context.Context, workflowID uuid.UUID) ([]*models.RelationshipCandidate, error) {
	if m.getByWorkflowErr != nil {
		return nil, m.getByWorkflowErr
	}
	return m.candidates, nil
}

func (m *rwsMockCandidateRepository) GetByWorkflowAndStatus(ctx context.Context, workflowID uuid.UUID, status models.RelationshipCandidateStatus) ([]*models.RelationshipCandidate, error) {
	var result []*models.RelationshipCandidate
	for _, c := range m.candidates {
		if c.Status == status {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *rwsMockCandidateRepository) GetRequiredPending(ctx context.Context, workflowID uuid.UUID) ([]*models.RelationshipCandidate, error) {
	var result []*models.RelationshipCandidate
	for _, c := range m.candidates {
		if c.Status == models.RelCandidateStatusPending && c.IsRequired {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *rwsMockCandidateRepository) Update(ctx context.Context, candidate *models.RelationshipCandidate) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	return nil
}

func (m *rwsMockCandidateRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.RelationshipCandidateStatus, userDecision *models.UserDecision) error {
	return nil
}

func (m *rwsMockCandidateRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return nil
}

func (m *rwsMockCandidateRepository) DeleteByWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return nil
}

func (m *rwsMockCandidateRepository) CountByWorkflowAndStatus(ctx context.Context, workflowID uuid.UUID, status models.RelationshipCandidateStatus) (int, error) {
	count := 0
	for _, c := range m.candidates {
		if c.Status == status {
			count++
		}
	}
	return count, nil
}

func (m *rwsMockCandidateRepository) CountRequiredPending(ctx context.Context, workflowID uuid.UUID) (int, error) {
	if m.countRequiredErr != nil {
		return 0, m.countRequiredErr
	}
	return m.countRequiredResult, nil
}

// rwsMockSchemaRepository is a mock for SchemaRepository used in workflow tests.
type rwsMockSchemaRepository struct {
	mockSchemaRepository // Embed existing mock from schema_test.go
}

// rwsMockStateRepository is a mock for WorkflowStateRepository.
type rwsMockStateRepository struct {
	states         []*models.WorkflowEntityState
	createBatchErr error
	listErr        error

	// Capture for verification
	createdStates []*models.WorkflowEntityState
}

func (m *rwsMockStateRepository) Create(ctx context.Context, state *models.WorkflowEntityState) error {
	m.createdStates = append(m.createdStates, state)
	return nil
}

func (m *rwsMockStateRepository) CreateBatch(ctx context.Context, states []*models.WorkflowEntityState) error {
	if m.createBatchErr != nil {
		return m.createBatchErr
	}
	m.createdStates = append(m.createdStates, states...)
	return nil
}

func (m *rwsMockStateRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.WorkflowEntityState, error) {
	for _, s := range m.states {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, errors.New("state not found")
}

func (m *rwsMockStateRepository) GetByEntity(ctx context.Context, workflowID uuid.UUID, entityType models.WorkflowEntityType, entityKey string) (*models.WorkflowEntityState, error) {
	for _, s := range m.states {
		if s.WorkflowID == workflowID && s.EntityType == entityType && s.EntityKey == entityKey {
			return s, nil
		}
	}
	return nil, nil
}

func (m *rwsMockStateRepository) Update(ctx context.Context, state *models.WorkflowEntityState) error {
	return nil
}

func (m *rwsMockStateRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.WorkflowEntityStatus, lastError *string) error {
	return nil
}

func (m *rwsMockStateRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *rwsMockStateRepository) ListByWorkflow(ctx context.Context, workflowID uuid.UUID) ([]*models.WorkflowEntityState, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.states, nil
}

func (m *rwsMockStateRepository) ListByStatus(ctx context.Context, workflowID uuid.UUID, status models.WorkflowEntityStatus) ([]*models.WorkflowEntityState, error) {
	var result []*models.WorkflowEntityState
	for _, s := range m.states {
		if s.Status == status {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *rwsMockStateRepository) DeleteByWorkflow(ctx context.Context, workflowID uuid.UUID) error {
	return nil
}

func (m *rwsMockStateRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *rwsMockStateRepository) UpdateStateData(ctx context.Context, id uuid.UUID, stateData *models.WorkflowStateData) error {
	return nil
}

func (m *rwsMockStateRepository) IncrementRetryCount(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *rwsMockStateRepository) AddQuestionsToEntity(ctx context.Context, id uuid.UUID, questions []models.WorkflowQuestion) error {
	return nil
}

func (m *rwsMockStateRepository) UpdateQuestionInEntity(ctx context.Context, id uuid.UUID, questionID string, status string, answer string) error {
	return nil
}

func (m *rwsMockStateRepository) RecordAnswerInEntity(ctx context.Context, id uuid.UUID, answer models.WorkflowAnswer) error {
	return nil
}

func (m *rwsMockStateRepository) GetNextPendingQuestion(ctx context.Context, workflowID uuid.UUID) (*models.WorkflowQuestion, uuid.UUID, error) {
	return nil, uuid.Nil, nil
}

func (m *rwsMockStateRepository) GetPendingQuestions(ctx context.Context, projectID uuid.UUID, limit int) ([]models.WorkflowQuestion, error) {
	return nil, nil
}

func (m *rwsMockStateRepository) GetPendingQuestionsCount(ctx context.Context, workflowID uuid.UUID) (int, int, error) {
	return 0, 0, nil
}

func (m *rwsMockStateRepository) GetPendingQuestionsCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *rwsMockStateRepository) FindQuestionByID(ctx context.Context, questionID string) (*models.WorkflowQuestion, *models.WorkflowEntityState, *models.OntologyWorkflow, error) {
	return nil, nil, nil, nil
}

// rwsMockOntologyRepository is a mock for OntologyRepository.
type rwsMockOntologyRepository struct {
	activeOntology      *models.TieredOntology
	getActiveErr        error
	createErr           error
	createdOntology     *models.TieredOntology
	getNextVersionValue int
	getNextVersionErr   error
}

func (m *rwsMockOntologyRepository) Create(ctx context.Context, ontology *models.TieredOntology) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.createdOntology = ontology
	return nil
}

func (m *rwsMockOntologyRepository) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	if m.getActiveErr != nil {
		return nil, m.getActiveErr
	}
	return m.activeOntology, nil
}

func (m *rwsMockOntologyRepository) GetByVersion(ctx context.Context, projectID uuid.UUID, version int) (*models.TieredOntology, error) {
	return nil, nil
}

func (m *rwsMockOntologyRepository) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return nil
}

func (m *rwsMockOntologyRepository) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	return nil
}

func (m *rwsMockOntologyRepository) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return nil
}

func (m *rwsMockOntologyRepository) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	return nil
}

func (m *rwsMockOntologyRepository) UpdateMetadata(ctx context.Context, projectID uuid.UUID, metadata map[string]any) error {
	return nil
}

func (m *rwsMockOntologyRepository) SetActive(ctx context.Context, projectID uuid.UUID, version int) error {
	return nil
}

func (m *rwsMockOntologyRepository) DeactivateAll(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *rwsMockOntologyRepository) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	if m.getNextVersionErr != nil {
		return 0, m.getNextVersionErr
	}
	return m.getNextVersionValue, nil
}

func (m *rwsMockOntologyRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *rwsMockOntologyRepository) WriteCleanOntology(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

// rwsMockDatasourceService is a mock for DatasourceService.
type rwsMockDatasourceService struct {
	datasource *models.Datasource
	err        error
}

func (m *rwsMockDatasourceService) Get(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.datasource != nil {
		return m.datasource, nil
	}
	// Return a default datasource if none is set (for tests that don't need it)
	return &models.Datasource{
		ID:             datasourceID,
		DatasourceType: "postgres",
		Config:         map[string]any{},
	}, nil
}

func (m *rwsMockDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *rwsMockDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (m *rwsMockDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error) {
	return nil, nil
}

func (m *rwsMockDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType string, config map[string]any) error {
	return nil
}

func (m *rwsMockDatasourceService) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *rwsMockDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any) error {
	return nil
}

// rwsMockAdapterFactory is a mock for DatasourceAdapterFactory.
type rwsMockAdapterFactory struct {
	discoverer    datasource.SchemaDiscoverer
	discovererErr error
}

func (m *rwsMockAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	if m.discovererErr != nil {
		return nil, m.discovererErr
	}
	if m.discoverer != nil {
		return m.discoverer, nil
	}
	// Return a default mock discoverer if none is set (for tests that don't need it)
	return &rwsMockSchemaDiscoverer{
		columnStats: make(map[string][]datasource.ColumnStats),
	}, nil
}

func (m *rwsMockAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, nil
}

func (m *rwsMockAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

func (m *rwsMockAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, nil
}

// rwsMockLLMFactory is a mock for LLMClientFactory.
type rwsMockLLMFactory struct {
	client llm.LLMClient
	err    error
}

func (m *rwsMockLLMFactory) CreateForProject(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.client, nil
}

func (m *rwsMockLLMFactory) CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	return m.client, m.err
}

func (m *rwsMockLLMFactory) CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*llm.StreamingClient, error) {
	return nil, m.err
}

// rwsMockDiscoveryService is a mock for RelationshipDiscoveryService.
type rwsMockDiscoveryService struct{}

func (m *rwsMockDiscoveryService) DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DiscoveryResults, error) {
	return &models.DiscoveryResults{}, nil
}

// rwsMockQueue is a mock for workqueue.Queue that tracks enqueued tasks.
type rwsMockQueue struct {
	tasks     []workqueue.Task
	waitErr   error
	cancelled bool
}

func (m *rwsMockQueue) Enqueue(task workqueue.Task) {
	m.tasks = append(m.tasks, task)
}

func (m *rwsMockQueue) Wait(ctx context.Context) error {
	if m.waitErr != nil {
		return m.waitErr
	}
	return nil
}

func (m *rwsMockQueue) Cancel() {
	m.cancelled = true
}

// rwsMockEntityRepository is a mock for SchemaEntityRepository.
type rwsMockEntityRepository struct {
	entities []*models.SchemaEntity
	getErr   error
}

func (m *rwsMockEntityRepository) Create(ctx context.Context, entity *models.SchemaEntity) error {
	return nil
}
func (m *rwsMockEntityRepository) GetByID(ctx context.Context, entityID uuid.UUID) (*models.SchemaEntity, error) {
	return nil, nil
}
func (m *rwsMockEntityRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.SchemaEntity, error) {
	return m.entities, m.getErr
}
func (m *rwsMockEntityRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.SchemaEntity, error) {
	return m.entities, m.getErr
}
func (m *rwsMockEntityRepository) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.SchemaEntity, error) {
	return nil, nil
}
func (m *rwsMockEntityRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *rwsMockEntityRepository) Update(ctx context.Context, entity *models.SchemaEntity) error {
	return nil
}
func (m *rwsMockEntityRepository) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}
func (m *rwsMockEntityRepository) Restore(ctx context.Context, entityID uuid.UUID) error { return nil }
func (m *rwsMockEntityRepository) CreateOccurrence(ctx context.Context, occ *models.SchemaEntityOccurrence) error {
	return nil
}
func (m *rwsMockEntityRepository) GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.SchemaEntityOccurrence, error) {
	return nil, nil
}
func (m *rwsMockEntityRepository) GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.SchemaEntityOccurrence, error) {
	return nil, nil
}
func (m *rwsMockEntityRepository) GetAllOccurrencesByProject(ctx context.Context, projectID uuid.UUID) ([]*models.SchemaEntityOccurrence, error) {
	return nil, nil
}
func (m *rwsMockEntityRepository) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}
func (m *rwsMockEntityRepository) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}
func (m *rwsMockEntityRepository) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}

// ============================================================================
// Helper Functions
// ============================================================================

func rwsGetTenantCtx() TenantContextFunc {
	return func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}
}

// newTestRelationshipWorkflowService creates a service with mocks for testing.
// entityRepo is optional - if nil, a default mock with one entity will be used.
func newTestRelationshipWorkflowService(
	workflowRepo *rwsMockWorkflowRepository,
	candidateRepo *rwsMockCandidateRepository,
	schemaRepo *rwsMockSchemaRepository,
	stateRepo *rwsMockStateRepository,
	ontologyRepo *rwsMockOntologyRepository,
	dsSvc *rwsMockDatasourceService,
	adapterFactory *rwsMockAdapterFactory,
	llmFactory *rwsMockLLMFactory,
) *relationshipWorkflowService {
	// Default entity repo with one entity (prerequisite check passes)
	entityRepo := &rwsMockEntityRepository{
		entities: []*models.SchemaEntity{{ID: uuid.New(), Name: "test_entity"}},
	}
	return &relationshipWorkflowService{
		workflowRepo:     workflowRepo,
		candidateRepo:    candidateRepo,
		schemaRepo:       schemaRepo,
		stateRepo:        stateRepo,
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		dsSvc:            dsSvc,
		adapterFactory:   adapterFactory,
		llmFactory:       llmFactory,
		discoveryService: &rwsMockDiscoveryService{},
		getTenantCtx:     rwsGetTenantCtx(),
		logger:           zap.NewNop(),
		serverInstanceID: uuid.New(),
	}
}

// ============================================================================
// Tests for enqueueColumnScans
// ============================================================================

func TestRelationshipWorkflow_EnqueueColumnScans_EmptyColumns(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables:  []*models.SchemaTable{},
			columns: []*models.SchemaColumn{},
		},
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	queue := workqueue.New(zap.NewNop())

	err := svc.enqueueColumnScans(context.Background(), projectID, workflowID, datasourceID, queue)
	if err != nil {
		t.Fatalf("enqueueColumnScans() error = %v, want nil", err)
	}

	// With no columns, no tasks should be enqueued
	tasks := queue.GetTasks()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestRelationshipWorkflow_EnqueueColumnScans_WithColumns(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables: []*models.SchemaTable{
				{ID: usersTableID, TableName: "users", SchemaName: "public", DatasourceID: datasourceID},
				{ID: ordersTableID, TableName: "orders", SchemaName: "public", DatasourceID: datasourceID},
			},
			columns: []*models.SchemaColumn{
				{ID: uuid.New(), ColumnName: "id", SchemaTableID: usersTableID},
				{ID: uuid.New(), ColumnName: "email", SchemaTableID: usersTableID},
				{ID: uuid.New(), ColumnName: "id", SchemaTableID: ordersTableID},
				{ID: uuid.New(), ColumnName: "user_id", SchemaTableID: ordersTableID},
			},
		},
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	queue := workqueue.New(zap.NewNop())

	err := svc.enqueueColumnScans(context.Background(), projectID, workflowID, datasourceID, queue)
	if err != nil {
		t.Fatalf("enqueueColumnScans() error = %v, want nil", err)
	}

	// Should have 4 tasks for 4 columns
	tasks := queue.GetTasks()
	if len(tasks) != 4 {
		t.Errorf("expected 4 tasks, got %d", len(tasks))
	}
}

func TestRelationshipWorkflow_EnqueueColumnScans_SkipsOrphanedColumns(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()
	orphanedTableID := uuid.New() // This table doesn't exist

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables: []*models.SchemaTable{
				// Only users table exists
				{ID: usersTableID, TableName: "users", SchemaName: "public", DatasourceID: datasourceID},
			},
			columns: []*models.SchemaColumn{
				{ID: uuid.New(), ColumnName: "id", SchemaTableID: usersTableID},
				// This column references a table that doesn't exist
				{ID: uuid.New(), ColumnName: "orphan_col", SchemaTableID: orphanedTableID},
			},
		},
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	queue := workqueue.New(zap.NewNop())

	err := svc.enqueueColumnScans(context.Background(), projectID, workflowID, datasourceID, queue)
	if err != nil {
		t.Fatalf("enqueueColumnScans() error = %v, want nil", err)
	}

	// Should have only 1 task (orphaned column skipped)
	tasks := queue.GetTasks()
	if len(tasks) != 1 {
		t.Errorf("expected 1 task (orphaned column skipped), got %d", len(tasks))
	}
}

func TestRelationshipWorkflow_EnqueueColumnScans_ListTablesError(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			listTablesErr: errors.New("database connection failed"),
		},
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	queue := workqueue.New(zap.NewNop())

	err := svc.enqueueColumnScans(context.Background(), projectID, workflowID, datasourceID, queue)
	if err == nil {
		t.Fatal("enqueueColumnScans() expected error, got nil")
	}
	if err.Error() != "list tables: database connection failed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRelationshipWorkflow_EnqueueColumnScans_ListColumnsError(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables:         []*models.SchemaTable{},
			listColumnsErr: errors.New("failed to list columns"),
		},
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	queue := workqueue.New(zap.NewNop())

	err := svc.enqueueColumnScans(context.Background(), projectID, workflowID, datasourceID, queue)
	if err == nil {
		t.Fatal("enqueueColumnScans() expected error, got nil")
	}
	if err.Error() != "list columns: failed to list columns" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================================
// Tests for enqueueTestJoins
// ============================================================================

func TestRelationshipWorkflow_EnqueueTestJoins_EmptyCandidates(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()

	candidateRepo := &rwsMockCandidateRepository{
		candidates: []*models.RelationshipCandidate{},
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		candidateRepo,
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	queue := workqueue.New(zap.NewNop())

	err := svc.enqueueTestJoins(context.Background(), projectID, workflowID, datasourceID, queue)
	if err != nil {
		t.Fatalf("enqueueTestJoins() error = %v, want nil", err)
	}

	// With no candidates, no tasks should be enqueued
	tasks := queue.GetTasks()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestRelationshipWorkflow_EnqueueTestJoins_WithCandidates(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()

	candidateRepo := &rwsMockCandidateRepository{
		candidates: []*models.RelationshipCandidate{
			{ID: uuid.New(), WorkflowID: workflowID, Status: models.RelCandidateStatusPending},
			{ID: uuid.New(), WorkflowID: workflowID, Status: models.RelCandidateStatusPending},
			{ID: uuid.New(), WorkflowID: workflowID, Status: models.RelCandidateStatusPending},
		},
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		candidateRepo,
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	queue := workqueue.New(zap.NewNop())

	err := svc.enqueueTestJoins(context.Background(), projectID, workflowID, datasourceID, queue)
	if err != nil {
		t.Fatalf("enqueueTestJoins() error = %v, want nil", err)
	}

	// Should have 3 tasks for 3 candidates
	tasks := queue.GetTasks()
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestRelationshipWorkflow_EnqueueTestJoins_GetCandidatesError(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()

	candidateRepo := &rwsMockCandidateRepository{
		getByWorkflowErr: errors.New("database error"),
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		candidateRepo,
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	queue := workqueue.New(zap.NewNop())

	err := svc.enqueueTestJoins(context.Background(), projectID, workflowID, datasourceID, queue)
	if err == nil {
		t.Fatal("enqueueTestJoins() expected error, got nil")
	}
	if err.Error() != "get candidates: database error" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================================
// Tests for updateProgress
// ============================================================================

func TestRelationshipWorkflow_UpdateProgress_Success(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	err := svc.updateProgress(context.Background(), projectID, workflowID, "Testing phase...", 50)
	if err != nil {
		t.Fatalf("updateProgress() error = %v, want nil", err)
	}

	// Verify the progress was set correctly
	if workflowRepo.updatedProgress == nil {
		t.Fatal("progress was not updated")
	}

	progress := workflowRepo.updatedProgress
	if progress.CurrentPhase != string(models.WorkflowPhaseRelationships) {
		t.Errorf("CurrentPhase = %v, want %v", progress.CurrentPhase, models.WorkflowPhaseRelationships)
	}
	if progress.Message != "Testing phase..." {
		t.Errorf("Message = %v, want 'Testing phase...'", progress.Message)
	}
	if progress.Current != 50 {
		t.Errorf("Current = %v, want 50", progress.Current)
	}
	if progress.Total != 100 {
		t.Errorf("Total = %v, want 100", progress.Total)
	}
}

func TestRelationshipWorkflow_UpdateProgress_Error(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{
		updateProgressErr: errors.New("database write failed"),
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	err := svc.updateProgress(context.Background(), projectID, workflowID, "Testing...", 50)
	if err == nil {
		t.Fatal("updateProgress() expected error, got nil")
	}
}

// ============================================================================
// Tests for markWorkflowFailed
// ============================================================================

func TestRelationshipWorkflow_MarkWorkflowFailed_Success(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	svc.markWorkflowFailed(projectID, workflowID, "test error message")

	// Verify state was updated to failed
	if workflowRepo.updatedState != models.WorkflowStateFailed {
		t.Errorf("updatedState = %v, want %v", workflowRepo.updatedState, models.WorkflowStateFailed)
	}
}

// ============================================================================
// Tests for finalizeWorkflow
// ============================================================================

func TestRelationshipWorkflow_FinalizeWorkflow_Success(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{}
	candidateRepo := &rwsMockCandidateRepository{
		countRequiredResult: 0,
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		candidateRepo,
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	svc.finalizeWorkflow(projectID, workflowID)

	// Verify state was updated to completed
	if workflowRepo.updatedState != models.WorkflowStateCompleted {
		t.Errorf("updatedState = %v, want %v", workflowRepo.updatedState, models.WorkflowStateCompleted)
	}

	// Verify progress was updated
	if workflowRepo.updatedProgress == nil {
		t.Fatal("progress was not updated")
	}
	if workflowRepo.updatedProgress.Current != 100 {
		t.Errorf("progress.Current = %v, want 100", workflowRepo.updatedProgress.Current)
	}
}

func TestRelationshipWorkflow_FinalizeWorkflow_WithPendingCandidates(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{}
	candidateRepo := &rwsMockCandidateRepository{
		countRequiredResult: 3,
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		candidateRepo,
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	svc.finalizeWorkflow(projectID, workflowID)

	// Verify progress message includes pending count
	if workflowRepo.updatedProgress == nil {
		t.Fatal("progress was not updated")
	}
	expectedMsg := "Relationship detection complete (3 relationships need review)"
	if workflowRepo.updatedProgress.Message != expectedMsg {
		t.Errorf("progress.Message = %v, want %v", workflowRepo.updatedProgress.Message, expectedMsg)
	}
}

// ============================================================================
// Tests for initializeWorkflowEntities
// ============================================================================

func TestRelationshipWorkflow_InitializeWorkflowEntities_Success(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables: []*models.SchemaTable{
				{ID: usersTableID, TableName: "users", SchemaName: "public", DatasourceID: datasourceID},
			},
			columns: []*models.SchemaColumn{
				{ID: uuid.New(), ColumnName: "id", SchemaTableID: usersTableID},
				{ID: uuid.New(), ColumnName: "email", SchemaTableID: usersTableID},
			},
		},
	}

	stateRepo := &rwsMockStateRepository{}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		schemaRepo,
		stateRepo,
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	err := svc.initializeWorkflowEntities(context.Background(), projectID, workflowID, ontologyID, datasourceID)
	if err != nil {
		t.Fatalf("initializeWorkflowEntities() error = %v, want nil", err)
	}

	// Verify 2 column entities were created
	if len(stateRepo.createdStates) != 2 {
		t.Errorf("created %d states, want 2", len(stateRepo.createdStates))
	}

	// Verify entity types and keys
	for _, state := range stateRepo.createdStates {
		if state.EntityType != models.WorkflowEntityTypeColumn {
			t.Errorf("EntityType = %v, want column", state.EntityType)
		}
		if state.Status != models.WorkflowEntityStatusPending {
			t.Errorf("Status = %v, want pending", state.Status)
		}
		if state.WorkflowID != workflowID {
			t.Errorf("WorkflowID = %v, want %v", state.WorkflowID, workflowID)
		}
	}
}

func TestRelationshipWorkflow_InitializeWorkflowEntities_SkipsOrphanedColumns(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()
	orphanedTableID := uuid.New()

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables: []*models.SchemaTable{
				{ID: usersTableID, TableName: "users", SchemaName: "public", DatasourceID: datasourceID},
			},
			columns: []*models.SchemaColumn{
				{ID: uuid.New(), ColumnName: "id", SchemaTableID: usersTableID},
				{ID: uuid.New(), ColumnName: "orphan_col", SchemaTableID: orphanedTableID}, // No table exists
			},
		},
	}

	stateRepo := &rwsMockStateRepository{}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		schemaRepo,
		stateRepo,
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	err := svc.initializeWorkflowEntities(context.Background(), projectID, workflowID, ontologyID, datasourceID)
	if err != nil {
		t.Fatalf("initializeWorkflowEntities() error = %v, want nil", err)
	}

	// Should only have 1 entity (orphaned column skipped)
	if len(stateRepo.createdStates) != 1 {
		t.Errorf("created %d states, want 1 (orphaned column skipped)", len(stateRepo.createdStates))
	}
}

// ============================================================================
// Tests for GetStatus
// ============================================================================

func TestRelationshipWorkflow_GetStatus_Success(t *testing.T) {
	datasourceID := uuid.New()
	workflowID := uuid.New()

	expectedWorkflow := &models.OntologyWorkflow{
		ID:    workflowID,
		State: models.WorkflowStateRunning,
		Phase: models.WorkflowPhaseRelationships,
	}

	workflowRepo := &rwsMockWorkflowRepository{
		workflow: expectedWorkflow,
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	workflow, err := svc.GetStatus(context.Background(), datasourceID)
	if err != nil {
		t.Fatalf("GetStatus() error = %v, want nil", err)
	}

	if workflow.ID != workflowID {
		t.Errorf("workflow.ID = %v, want %v", workflow.ID, workflowID)
	}
}

func TestRelationshipWorkflow_GetStatus_Error(t *testing.T) {
	datasourceID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{
		getLatestErr: errors.New("database error"),
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	_, err := svc.GetStatus(context.Background(), datasourceID)
	if err == nil {
		t.Fatal("GetStatus() expected error, got nil")
	}
}

// ============================================================================
// Tests for Cancel
// ============================================================================

func TestRelationshipWorkflow_Cancel_Success(t *testing.T) {
	workflowID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{}
	candidateRepo := &rwsMockCandidateRepository{}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		candidateRepo,
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	err := svc.Cancel(context.Background(), workflowID)
	if err != nil {
		t.Fatalf("Cancel() error = %v, want nil", err)
	}
}

func TestRelationshipWorkflow_Cancel_DeleteCandidatesError(t *testing.T) {
	workflowID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{}
	candidateRepo := &rwsMockCandidateRepository{
		deleteErr: errors.New("failed to delete candidates"),
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		candidateRepo,
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	err := svc.Cancel(context.Background(), workflowID)
	if err == nil {
		t.Fatal("Cancel() expected error, got nil")
	}
}

// ============================================================================
// Tests for SaveRelationships
// ============================================================================

func TestRelationshipWorkflow_SaveRelationships_RequiredPendingError(t *testing.T) {
	workflowID := uuid.New()
	projectID := uuid.New()
	datasourceID := uuid.New()

	candidateRepo := &rwsMockCandidateRepository{
		countRequiredResult: 5,
	}
	workflowRepo := &rwsMockWorkflowRepository{
		workflow: &models.OntologyWorkflow{
			ID:           workflowID,
			ProjectID:    projectID,
			DatasourceID: &datasourceID,
		},
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		candidateRepo,
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	savedCount, err := svc.SaveRelationships(context.Background(), workflowID)
	if err == nil {
		t.Fatal("SaveRelationships() expected error, got nil")
	}
	if savedCount != 0 {
		t.Errorf("savedCount = %d, want 0", savedCount)
	}
	expectedErr := "cannot save: 5 relationships require user review"
	if err.Error() != expectedErr {
		t.Errorf("error = %v, want %v", err.Error(), expectedErr)
	}
}

func TestRelationshipWorkflow_SaveRelationships_NoDatasourceID(t *testing.T) {
	workflowID := uuid.New()
	projectID := uuid.New()

	// Workflow without datasource ID
	workflowRepo := &rwsMockWorkflowRepository{
		workflow: &models.OntologyWorkflow{
			ID:           workflowID,
			ProjectID:    projectID,
			DatasourceID: nil, // No datasource ID
		},
	}
	candidateRepo := &rwsMockCandidateRepository{
		countRequiredResult: 0,
		candidates:          []*models.RelationshipCandidate{},
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		candidateRepo,
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	savedCount, err := svc.SaveRelationships(context.Background(), workflowID)
	if err == nil {
		t.Fatal("SaveRelationships() expected error, got nil")
	}
	if savedCount != 0 {
		t.Errorf("savedCount = %d, want 0", savedCount)
	}
	if err.Error() != "workflow has no datasource ID" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRelationshipWorkflow_SaveRelationships_Success(t *testing.T) {
	workflowID := uuid.New()
	projectID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	usersIDColID := uuid.New()
	ordersUserIDColID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{
		workflow: &models.OntologyWorkflow{
			ID:           workflowID,
			ProjectID:    projectID,
			DatasourceID: &datasourceID,
		},
	}

	// Accepted candidates
	candidateRepo := &rwsMockCandidateRepository{
		countRequiredResult: 0,
		candidates: []*models.RelationshipCandidate{
			{
				ID:              uuid.New(),
				WorkflowID:      workflowID,
				SourceColumnID:  ordersUserIDColID,
				TargetColumnID:  usersIDColID,
				DetectionMethod: models.DetectionMethodValueMatch,
				Confidence:      0.95,
				Status:          models.RelCandidateStatusAccepted,
			},
		},
	}

	// Schema with columns and tables
	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables: []*models.SchemaTable{
				{ID: usersTableID, TableName: "users", SchemaName: "public", DatasourceID: datasourceID},
				{ID: ordersTableID, TableName: "orders", SchemaName: "public", DatasourceID: datasourceID},
			},
			columns: []*models.SchemaColumn{
				{ID: usersIDColID, ColumnName: "id", SchemaTableID: usersTableID, ProjectID: projectID},
				{ID: ordersUserIDColID, ColumnName: "user_id", SchemaTableID: ordersTableID, ProjectID: projectID},
			},
		},
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		candidateRepo,
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	savedCount, err := svc.SaveRelationships(context.Background(), workflowID)
	if err != nil {
		t.Fatalf("SaveRelationships() error = %v, want nil", err)
	}
	if savedCount != 1 {
		t.Errorf("savedCount = %d, want 1", savedCount)
	}

	// Verify relationship was created
	if len(schemaRepo.upsertedRelationships) != 1 {
		t.Errorf("expected 1 relationship created, got %d", len(schemaRepo.upsertedRelationships))
	}
}

func TestRelationshipWorkflow_SaveRelationships_ColumnNotFound(t *testing.T) {
	workflowID := uuid.New()
	projectID := uuid.New()
	datasourceID := uuid.New()
	missingColID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{
		workflow: &models.OntologyWorkflow{
			ID:           workflowID,
			ProjectID:    projectID,
			DatasourceID: &datasourceID,
		},
	}

	// Candidate referencing missing columns
	candidateRepo := &rwsMockCandidateRepository{
		countRequiredResult: 0,
		candidates: []*models.RelationshipCandidate{
			{
				ID:              uuid.New(),
				WorkflowID:      workflowID,
				SourceColumnID:  missingColID, // Not in schema
				TargetColumnID:  uuid.New(),
				DetectionMethod: models.DetectionMethodValueMatch,
				Status:          models.RelCandidateStatusAccepted,
			},
		},
	}

	// Schema with no columns (columns are empty)
	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables:  []*models.SchemaTable{},
			columns: []*models.SchemaColumn{}, // Empty - column not found
		},
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		candidateRepo,
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	// With fail-fast behavior, should return error when column not found
	savedCount, err := svc.SaveRelationships(context.Background(), workflowID)
	if err == nil {
		t.Fatal("SaveRelationships() expected error for missing column, got nil")
	}
	if savedCount != 0 {
		t.Errorf("savedCount = %d, want 0", savedCount)
	}

	// No relationships should be created since column not found
	if len(schemaRepo.upsertedRelationships) != 0 {
		t.Errorf("expected 0 relationships created (column not found), got %d", len(schemaRepo.upsertedRelationships))
	}
}

func TestRelationshipWorkflow_SaveRelationships_EmptyAccepted(t *testing.T) {
	workflowID := uuid.New()
	projectID := uuid.New()
	datasourceID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{
		workflow: &models.OntologyWorkflow{
			ID:           workflowID,
			ProjectID:    projectID,
			DatasourceID: &datasourceID,
		},
	}

	// No accepted candidates
	candidateRepo := &rwsMockCandidateRepository{
		countRequiredResult: 0,
		candidates:          []*models.RelationshipCandidate{},
	}

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables:  []*models.SchemaTable{},
			columns: []*models.SchemaColumn{},
		},
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		candidateRepo,
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	savedCount, err := svc.SaveRelationships(context.Background(), workflowID)
	if err != nil {
		t.Fatalf("SaveRelationships() error = %v, want nil", err)
	}
	if savedCount != 0 {
		t.Errorf("savedCount = %d, want 0", savedCount)
	}
}

// ============================================================================
// Tests for Heartbeat Start/Stop
// ============================================================================

func TestRelationshipWorkflow_Heartbeat_StartStop(t *testing.T) {
	workflowID := uuid.New()
	projectID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	// Start heartbeat
	svc.startHeartbeat(workflowID, projectID)

	// Verify it's stored
	if _, ok := svc.heartbeatStop.Load(workflowID); !ok {
		t.Error("heartbeat info not stored")
	}

	// Stop heartbeat
	svc.stopHeartbeat(workflowID)

	// Give goroutine time to exit
	time.Sleep(10 * time.Millisecond)

	// Verify it's removed
	if _, ok := svc.heartbeatStop.Load(workflowID); ok {
		t.Error("heartbeat info should be removed after stop")
	}
}

// ============================================================================
// Tests for TaskQueueWriter
// ============================================================================

func TestRelationshipWorkflow_TaskQueueWriter_StartStop(t *testing.T) {
	workflowID := uuid.New()

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	// Start writer
	writer := svc.startTaskQueueWriter(workflowID)

	// Verify it's stored
	if _, ok := svc.taskQueueWriters.Load(workflowID); !ok {
		t.Error("task queue writer not stored")
	}

	// Writer should have a valid channel
	if writer.updates == nil {
		t.Error("updates channel is nil")
	}

	// Stop writer
	svc.stopTaskQueueWriter(workflowID)

	// Wait for the writer to finish
	select {
	case <-writer.done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("writer did not stop in time")
	}

	// Verify it's removed
	if _, ok := svc.taskQueueWriters.Load(workflowID); ok {
		t.Error("task queue writer should be removed after stop")
	}
}

// ============================================================================
// Tests for StartDetection
// ============================================================================

func TestRelationshipWorkflow_StartDetection_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{
		workflow:             nil, // No existing workflow
		claimOwnershipResult: true,
	}

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables:  []*models.SchemaTable{},
			columns: []*models.SchemaColumn{},
		},
	}

	ontologyRepo := &rwsMockOntologyRepository{
		activeOntology:      nil, // No existing ontology
		getNextVersionValue: 1,
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		ontologyRepo,
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	workflow, err := svc.StartDetection(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("StartDetection() error = %v, want nil", err)
	}

	// Verify workflow was created
	if workflow == nil {
		t.Fatal("expected workflow to be created")
	}

	// Verify ontology was created (since none existed)
	if ontologyRepo.createdOntology == nil {
		t.Error("expected ontology to be created")
	}

	// Verify workflow references the created ontology
	if workflow.OntologyID != ontologyRepo.createdOntology.ID {
		t.Errorf("workflow.OntologyID = %v, want %v", workflow.OntologyID, ontologyRepo.createdOntology.ID)
	}
}

func TestRelationshipWorkflow_StartDetection_ExistingActiveWorkflow(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Existing running workflow
	existingWorkflow := &models.OntologyWorkflow{
		ID:           uuid.New(),
		ProjectID:    projectID,
		DatasourceID: &datasourceID,
		State:        models.WorkflowStateRunning,
		Phase:        models.WorkflowPhaseRelationships,
	}

	workflowRepo := &rwsMockWorkflowRepository{
		workflow: existingWorkflow,
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	_, err := svc.StartDetection(context.Background(), projectID, datasourceID)
	if err == nil {
		t.Fatal("StartDetection() expected error, got nil")
	}

	expectedErr := "relationship detection already in progress for this datasource"
	if err.Error() != expectedErr {
		t.Errorf("error = %v, want %v", err.Error(), expectedErr)
	}
}

func TestRelationshipWorkflow_StartDetection_ClaimOwnershipFails(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	workflowRepo := &rwsMockWorkflowRepository{
		workflow:             nil,   // No existing workflow
		claimOwnershipResult: false, // Ownership claimed by another server
	}

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables:  []*models.SchemaTable{},
			columns: []*models.SchemaColumn{},
		},
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	// We'll get an error from ontology creation before we reach claim ownership
	// This test is limited by the need to create an ontology
	_, err := svc.StartDetection(context.Background(), projectID, datasourceID)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ============================================================================
// Tests for Ontology Reuse Behavior
// ============================================================================

func TestRelationshipWorkflow_StartDetection_ReusesExistingActiveOntology(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	existingOntologyID := uuid.New()

	// Existing active ontology for this project
	existingOntology := &models.TieredOntology{
		ID:        existingOntologyID,
		ProjectID: projectID,
		Version:   1,
		IsActive:  true,
	}

	ontologyRepo := &rwsMockOntologyRepository{
		activeOntology: existingOntology,
	}

	workflowRepo := &rwsMockWorkflowRepository{
		workflow:             nil, // No existing workflow
		claimOwnershipResult: true,
	}

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables:  []*models.SchemaTable{},
			columns: []*models.SchemaColumn{},
		},
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		ontologyRepo,
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	workflow, err := svc.StartDetection(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("StartDetection() error = %v, want nil", err)
	}

	// Should reuse the existing ontology, not create a new one
	if ontologyRepo.createdOntology != nil {
		t.Error("should NOT have created a new ontology when active one exists")
	}

	// Workflow should reference the existing ontology
	if workflow.OntologyID != existingOntologyID {
		t.Errorf("workflow.OntologyID = %v, want %v (existing)", workflow.OntologyID, existingOntologyID)
	}
}

func TestRelationshipWorkflow_StartDetection_CreatesActiveOntologyWhenNoneExists(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	// No existing active ontology
	ontologyRepo := &rwsMockOntologyRepository{
		activeOntology:      nil, // No active ontology
		getNextVersionValue: 1,
	}

	workflowRepo := &rwsMockWorkflowRepository{
		workflow:             nil, // No existing workflow
		claimOwnershipResult: true,
	}

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables:  []*models.SchemaTable{},
			columns: []*models.SchemaColumn{},
		},
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		ontologyRepo,
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	workflow, err := svc.StartDetection(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("StartDetection() error = %v, want nil", err)
	}

	// Should have created a new ontology
	if ontologyRepo.createdOntology == nil {
		t.Fatal("should have created a new ontology when none exists")
	}

	// New ontology should be active
	if !ontologyRepo.createdOntology.IsActive {
		t.Error("created ontology should have IsActive = true")
	}

	// Workflow should reference the new ontology
	if workflow.OntologyID != ontologyRepo.createdOntology.ID {
		t.Errorf("workflow.OntologyID = %v, want %v (created)", workflow.OntologyID, ontologyRepo.createdOntology.ID)
	}
}

func TestRelationshipWorkflow_StartDetection_SecondRunReusesOntology(t *testing.T) {
	// This test verifies that running relationship detection twice on the same project
	// reuses the same ontology (no duplicate key violation)
	projectID := uuid.New()
	datasourceID := uuid.New()
	existingOntologyID := uuid.New()

	// Simulate second run: ontology already exists from first run
	existingOntology := &models.TieredOntology{
		ID:        existingOntologyID,
		ProjectID: projectID,
		Version:   1,
		IsActive:  true,
	}

	ontologyRepo := &rwsMockOntologyRepository{
		activeOntology: existingOntology,
	}

	// First workflow completed (terminal state)
	completedWorkflow := &models.OntologyWorkflow{
		ID:           uuid.New(),
		ProjectID:    projectID,
		DatasourceID: &datasourceID,
		State:        models.WorkflowStateCompleted, // Terminal state
		Phase:        models.WorkflowPhaseRelationships,
	}

	workflowRepo := &rwsMockWorkflowRepository{
		workflow:             completedWorkflow, // Previous workflow completed
		claimOwnershipResult: true,
	}

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables:  []*models.SchemaTable{},
			columns: []*models.SchemaColumn{},
		},
	}

	svc := newTestRelationshipWorkflowService(
		workflowRepo,
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		ontologyRepo,
		&rwsMockDatasourceService{},
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	workflow, err := svc.StartDetection(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("StartDetection() on second run error = %v, want nil", err)
	}

	// Should NOT create a new ontology
	if ontologyRepo.createdOntology != nil {
		t.Error("second run should NOT create a new ontology")
	}

	// New workflow should reference the existing ontology
	if workflow.OntologyID != existingOntologyID {
		t.Errorf("workflow.OntologyID = %v, want %v (existing)", workflow.OntologyID, existingOntologyID)
	}
}

// ============================================================================
// Tests for Column Statistics Collection
// ============================================================================

// rwsMockSchemaDiscoverer is a mock for SchemaDiscoverer interface.
type rwsMockSchemaDiscoverer struct {
	columnStats map[string][]datasource.ColumnStats // key: schema.table
	statsErr    error
}

func (m *rwsMockSchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	return nil, nil
}

func (m *rwsMockSchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	return nil, nil
}

func (m *rwsMockSchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	return nil, nil
}

func (m *rwsMockSchemaDiscoverer) SupportsForeignKeys() bool {
	return true
}

func (m *rwsMockSchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	key := schemaName + "." + tableName
	if stats, ok := m.columnStats[key]; ok {
		return stats, nil
	}
	// Return default stats for each column
	result := make([]datasource.ColumnStats, len(columnNames))
	for i, name := range columnNames {
		result[i] = datasource.ColumnStats{
			ColumnName:    name,
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 50,
		}
	}
	return result, nil
}

func (m *rwsMockSchemaDiscoverer) CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleLimit int) (*datasource.ValueOverlapResult, error) {
	return nil, nil
}

func (m *rwsMockSchemaDiscoverer) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	return nil, nil
}

func (m *rwsMockSchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	return nil, nil
}

func (m *rwsMockSchemaDiscoverer) Close() error {
	return nil
}

func TestRelationshipWorkflow_CollectColumnStatistics_Success(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	mockDiscoverer := &rwsMockSchemaDiscoverer{
		columnStats: map[string][]datasource.ColumnStats{
			"public.users": {
				{ColumnName: "id", RowCount: 100, NonNullCount: 100, DistinctCount: 100},
				{ColumnName: "email", RowCount: 100, NonNullCount: 100, DistinctCount: 100},
				{ColumnName: "status", RowCount: 100, NonNullCount: 100, DistinctCount: 4},
			},
			"public.orders": {
				{ColumnName: "id", RowCount: 500, NonNullCount: 500, DistinctCount: 500},
				{ColumnName: "user_id", RowCount: 500, NonNullCount: 500, DistinctCount: 95},
			},
		},
	}

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables: []*models.SchemaTable{
				{
					ID:         usersTableID,
					TableName:  "users",
					SchemaName: "public",
				},
				{
					ID:         ordersTableID,
					TableName:  "orders",
					SchemaName: "public",
				},
			},
			columns: []*models.SchemaColumn{
				{SchemaTableID: usersTableID, ColumnName: "id"},
				{SchemaTableID: usersTableID, ColumnName: "email"},
				{SchemaTableID: usersTableID, ColumnName: "status"},
				{SchemaTableID: ordersTableID, ColumnName: "id"},
				{SchemaTableID: ordersTableID, ColumnName: "user_id"},
			},
		},
	}

	adapterFactory := &rwsMockAdapterFactory{
		discoverer: mockDiscoverer,
	}

	dsSvc := &rwsMockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		dsSvc,
		adapterFactory,
		&rwsMockLLMFactory{},
	)

	// Call collectColumnStatistics
	_, err := svc.collectColumnStatistics(
		context.Background(),
		projectID,
		workflowID,
		datasourceID,
	)

	if err != nil {
		t.Fatalf("collectColumnStatistics() error = %v, want nil", err)
	}
}

func TestRelationshipWorkflow_CollectColumnStatistics_GetDatasourceError(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()

	dsSvc := &rwsMockDatasourceService{
		err: errors.New("datasource not found"),
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		dsSvc,
		&rwsMockAdapterFactory{},
		&rwsMockLLMFactory{},
	)

	_, err := svc.collectColumnStatistics(
		context.Background(),
		projectID,
		workflowID,
		datasourceID,
	)

	if err == nil {
		t.Fatal("collectColumnStatistics() error = nil, want error")
	}
}

func TestRelationshipWorkflow_CollectColumnStatistics_CreateAdapterError(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()

	dsSvc := &rwsMockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	adapterFactory := &rwsMockAdapterFactory{
		discovererErr: errors.New("failed to create adapter"),
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		&rwsMockSchemaRepository{},
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		dsSvc,
		adapterFactory,
		&rwsMockLLMFactory{},
	)

	_, err := svc.collectColumnStatistics(
		context.Background(),
		projectID,
		workflowID,
		datasourceID,
	)

	if err == nil {
		t.Fatal("collectColumnStatistics() error = nil, want error")
	}
}

func TestRelationshipWorkflow_CollectColumnStatistics_AnalyzeStatsError(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()

	mockDiscoverer := &rwsMockSchemaDiscoverer{
		statsErr: errors.New("failed to analyze stats"),
	}

	schemaRepo := &rwsMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables: []*models.SchemaTable{
				{
					ID:         usersTableID,
					TableName:  "users",
					SchemaName: "public",
				},
			},
			columns: []*models.SchemaColumn{
				{SchemaTableID: usersTableID, ColumnName: "id"},
			},
		},
	}

	adapterFactory := &rwsMockAdapterFactory{
		discoverer: mockDiscoverer,
	}

	dsSvc := &rwsMockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	svc := newTestRelationshipWorkflowService(
		&rwsMockWorkflowRepository{},
		&rwsMockCandidateRepository{},
		schemaRepo,
		&rwsMockStateRepository{},
		&rwsMockOntologyRepository{},
		dsSvc,
		adapterFactory,
		&rwsMockLLMFactory{},
	)

	_, err := svc.collectColumnStatistics(
		context.Background(),
		projectID,
		workflowID,
		datasourceID,
	)

	if err == nil {
		t.Fatal("collectColumnStatistics() error = nil, want error")
	}
}
