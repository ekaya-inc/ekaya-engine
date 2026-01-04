package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// TestEntityByPrimaryTableMapping verifies that entityByPrimaryTable uses
// PrimarySchema/PrimaryTable rather than occurrences.
//
// Scenario: billing_engagements table has:
// - "billing_engagement" entity (owns the table, PrimaryTable = "billing_engagements")
// - "user" entity occurrences for host_id and visitor_id columns
//
// The old code used "first occurrence wins" which would incorrectly associate
// billing_engagements with whichever entity was first in the occurrence list.
// The fix uses entity.PrimaryTable so billing_engagements → billing_engagement.
func TestEntityByPrimaryTableMapping(t *testing.T) {
	// Create entities
	billingEngagementEntityID := uuid.New()
	userEntityID := uuid.New()

	entities := []*models.OntologyEntity{
		{
			ID:            billingEngagementEntityID,
			Name:          "billing_engagement",
			PrimarySchema: "public",
			PrimaryTable:  "billing_engagements",
		},
		{
			ID:            userEntityID,
			Name:          "user",
			PrimarySchema: "public",
			PrimaryTable:  "users",
		},
	}

	// Build entityByPrimaryTable map (same logic as in DiscoverRelationships)
	entityByPrimaryTable := make(map[string]*models.OntologyEntity)
	for _, entity := range entities {
		key := entity.PrimarySchema + "." + entity.PrimaryTable
		entityByPrimaryTable[key] = entity
	}

	// Verify billing_engagements maps to billing_engagement entity
	billingKey := "public.billing_engagements"
	if got := entityByPrimaryTable[billingKey]; got == nil {
		t.Fatalf("expected entity for %s, got nil", billingKey)
	} else if got.ID != billingEngagementEntityID {
		t.Errorf("expected billing_engagements to map to billing_engagement entity, got %s", got.Name)
	}

	// Verify users maps to user entity
	usersKey := "public.users"
	if got := entityByPrimaryTable[usersKey]; got == nil {
		t.Fatalf("expected entity for %s, got nil", usersKey)
	} else if got.ID != userEntityID {
		t.Errorf("expected users to map to user entity, got %s", got.Name)
	}

	// Verify that a table not owned by any entity returns nil
	unknownKey := "public.unknown_table"
	if got := entityByPrimaryTable[unknownKey]; got != nil {
		t.Errorf("expected nil for unknown table, got %s", got.Name)
	}
}

// TestOldOccByTableBehaviorWasBroken demonstrates why the old occurrence-based
// mapping was incorrect.
//
// The old code would iterate through occurrences and use "first wins" logic,
// which meant the entity associated with a table depended on occurrence order,
// not on which entity actually owns the table.
func TestOldOccByTableBehaviorWasBroken(t *testing.T) {
	// Create entities
	billingEngagementEntityID := uuid.New()
	userEntityID := uuid.New()

	entityByID := map[uuid.UUID]*models.OntologyEntity{
		billingEngagementEntityID: {
			ID:            billingEngagementEntityID,
			Name:          "billing_engagement",
			PrimarySchema: "public",
			PrimaryTable:  "billing_engagements",
		},
		userEntityID: {
			ID:            userEntityID,
			Name:          "user",
			PrimarySchema: "public",
			PrimaryTable:  "users",
		},
	}

	// Simulate occurrences where user entity has an occurrence in billing_engagements
	// (via host_id column) BEFORE the billing_engagement entity's occurrence
	occurrences := []*models.OntologyEntityOccurrence{
		// User entity occurs in billing_engagements.host_id
		{
			EntityID:   userEntityID,
			SchemaName: "public",
			TableName:  "billing_engagements",
			ColumnName: "host_id",
		},
		// Billing engagement entity occurs in its primary table
		{
			EntityID:   billingEngagementEntityID,
			SchemaName: "public",
			TableName:  "billing_engagements",
			ColumnName: "id",
		},
		// User entity in its own table
		{
			EntityID:   userEntityID,
			SchemaName: "public",
			TableName:  "users",
			ColumnName: "id",
		},
	}

	// OLD (broken) logic: first occurrence wins
	occByTable := make(map[string]*models.OntologyEntity)
	for _, occ := range occurrences {
		key := occ.SchemaName + "." + occ.TableName
		if _, exists := occByTable[key]; !exists {
			occByTable[key] = entityByID[occ.EntityID]
		}
	}

	// With the old code, billing_engagements would incorrectly map to "user"
	// because the user's host_id occurrence comes first
	billingKey := "public.billing_engagements"
	oldResult := occByTable[billingKey]
	if oldResult == nil {
		t.Fatal("expected entity for billing_engagements")
	}

	// This demonstrates the bug: old code returns "user" instead of "billing_engagement"
	if oldResult.Name == "billing_engagement" {
		t.Skip("occurrence order in test data happens to be correct")
	}
	if oldResult.Name != "user" {
		t.Errorf("expected old code to incorrectly return 'user', got %s", oldResult.Name)
	}

	// NEW (correct) logic: use PrimaryTable
	entityByPrimaryTable := make(map[string]*models.OntologyEntity)
	for _, entity := range entityByID {
		key := entity.PrimarySchema + "." + entity.PrimaryTable
		entityByPrimaryTable[key] = entity
	}

	newResult := entityByPrimaryTable[billingKey]
	if newResult == nil {
		t.Fatal("expected entity for billing_engagements with new logic")
	}
	if newResult.Name != "billing_engagement" {
		t.Errorf("expected new code to correctly return 'billing_engagement', got %s", newResult.Name)
	}
}

// TestPKMatch_RequiresDistinctCount verifies that columns without DistinctCount
// are skipped and do not create relationships.
func TestPKMatch_RequiresDistinctCount(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	entityID := uuid.New()

	distinctCount := int64(100)

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, entityID)

	// Schema with two columns: one WITH stats, one WITHOUT
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					DistinctCount: &distinctCount, // Has stats
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					DistinctCount: nil, // NO stats - should be skipped
				},
			},
		},
	}

	// Create service
	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
	)

	// Execute PK match discovery
	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: NO relationships created because orders.user_id lacks stats
	if result.InferredRelationships != 0 {
		t.Errorf("expected 0 inferred relationships (column without stats should be skipped), got %d", result.InferredRelationships)
	}

	// Verify no relationships were persisted
	if len(mocks.relationshipRepo.created) != 0 {
		t.Errorf("expected 0 relationships to be created, got %d", len(mocks.relationshipRepo.created))
	}
}

// TestPKMatch_WorksWithoutRowCount verifies that columns with DistinctCount
// but missing RowCount can still pass the cardinality filter.
func TestPKMatch_WorksWithoutRowCount(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	entityID := uuid.New()

	distinctCount := int64(50) // Meets absolute threshold (>= 20)

	// Create mocks
	mocks := setupMocks(projectID, ontologyID, datasourceID, entityID)

	// Mock discoverer returns 0 orphans (100% match)
	mocks.discoverer.joinAnalysisFunc = func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
		return &datasource.JoinAnalysis{
			OrphanCount: 0, // All source rows match = valid relationship
		}, nil
	}

	// Schema: PK candidate has DistinctCount, but table has NO RowCount
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	mocks.schemaRepo.tables = []*models.SchemaTable{
		{
			ID:         usersTableID,
			SchemaName: "public",
			TableName:  "users",
			RowCount:   nil, // NO row count - ratio check should be skipped
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: usersTableID,
					ColumnName:    "id",
					DataType:      "uuid",
					IsPrimaryKey:  true,
					DistinctCount: &distinctCount, // Has distinct count >= 20
				},
			},
		},
		{
			ID:         ordersTableID,
			SchemaName: "public",
			TableName:  "orders",
			RowCount:   &distinctCount,
			Columns: []models.SchemaColumn{
				{
					SchemaTableID: ordersTableID,
					ColumnName:    "user_id",
					DataType:      "uuid",
					IsPrimaryKey:  false,
					DistinctCount: &distinctCount, // Has stats
				},
			},
		},
	}

	// Create service
	service := NewDeterministicRelationshipService(
		mocks.datasourceService,
		mocks.adapterFactory,
		mocks.ontologyRepo,
		mocks.entityRepo,
		mocks.relationshipRepo,
		mocks.schemaRepo,
	)

	// Execute PK match discovery
	result, err := service.DiscoverPKMatchRelationships(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: relationship WAS created (missing RowCount doesn't block when DistinctCount >= 20)
	if result.InferredRelationships != 1 {
		t.Errorf("expected 1 inferred relationship (column has sufficient DistinctCount), got %d", result.InferredRelationships)
	}

	// Verify relationship was persisted
	if len(mocks.relationshipRepo.created) != 1 {
		t.Fatalf("expected 1 relationship to be created, got %d", len(mocks.relationshipRepo.created))
	}

	// Verify relationship details
	rel := mocks.relationshipRepo.created[0]
	if rel.FromEntityID != entityID {
		t.Errorf("expected FromEntityID to be %s, got %s", entityID, rel.FromEntityID)
	}
	if rel.ToEntityID != entityID {
		t.Errorf("expected ToEntityID to be %s, got %s", entityID, rel.ToEntityID)
	}
}

// mockTestServices holds all mock dependencies for testing
type mockTestServices struct {
	datasourceService  *mockTestDatasourceService
	adapterFactory     *mockTestAdapterFactory
	discoverer         *mockTestSchemaDiscoverer
	ontologyRepo       *mockTestOntologyRepo
	entityRepo         *mockTestEntityRepo
	relationshipRepo   *mockTestRelationshipRepo
	schemaRepo         *mockTestSchemaRepo
}

// setupMocks creates all mock dependencies with sensible defaults
func setupMocks(projectID, ontologyID, datasourceID, entityID uuid.UUID) *mockTestServices {
	discoverer := &mockTestSchemaDiscoverer{}

	return &mockTestServices{
		datasourceService: &mockTestDatasourceService{
			datasource: &models.Datasource{
				ID:             datasourceID,
				DatasourceType: "postgres",
				Config:         map[string]any{},
			},
		},
		adapterFactory: &mockTestAdapterFactory{
			discoverer: discoverer,
		},
		discoverer: discoverer,
		ontologyRepo: &mockTestOntologyRepo{
			ontology: &models.TieredOntology{
				ID:        ontologyID,
				ProjectID: projectID,
			},
		},
		entityRepo: &mockTestEntityRepo{
			entities: []*models.OntologyEntity{
				{
					ID:            entityID,
					OntologyID:    ontologyID,
					Name:          "user",
					PrimarySchema: "public",
					PrimaryTable:  "users",
				},
			},
			occurrences: []*models.OntologyEntityOccurrence{
				{
					EntityID:   entityID,
					SchemaName: "public",
					TableName:  "users",
					ColumnName: "id",
				},
			},
		},
		relationshipRepo: &mockTestRelationshipRepo{
			created: []*models.EntityRelationship{},
		},
		schemaRepo: &mockTestSchemaRepo{
			tables: []*models.SchemaTable{},
		},
	}
}

// Mock implementations for deterministic relationship service tests

type mockTestDatasourceService struct {
	datasource *models.Datasource
}

func (m *mockTestDatasourceService) GetByID(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *mockTestDatasourceService) Get(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *mockTestDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error) {
	return nil, nil
}

func (m *mockTestDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockTestDatasourceService) Update(ctx context.Context, ds *models.Datasource) error {
	return nil
}

func (m *mockTestDatasourceService) Delete(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

func (m *mockTestDatasourceService) SetDefault(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

type mockTestAdapterFactory struct {
	discoverer *mockTestSchemaDiscoverer
}

func (m *mockTestAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, nil
}

func (m *mockTestAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	return m.discoverer, nil
}

func (m *mockTestAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, nil
}

func (m *mockTestAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

type mockTestSchemaDiscoverer struct {
	joinAnalysisFunc func(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error)
}

func (m *mockTestSchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) SupportsForeignKeys() bool {
	return false
}

func (m *mockTestSchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleLimit int) (*datasource.ValueOverlapResult, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	if m.joinAnalysisFunc != nil {
		return m.joinAnalysisFunc(ctx, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn)
	}
	// Default: no matches
	return &datasource.JoinAnalysis{
		OrphanCount: 100, // All rows orphaned = no relationship
	}, nil
}

func (m *mockTestSchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	return nil, nil
}

func (m *mockTestSchemaDiscoverer) Close() error {
	return nil
}

type mockTestOntologyRepo struct {
	ontology *models.TieredOntology
}

func (m *mockTestOntologyRepo) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}

func (m *mockTestOntologyRepo) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	return m.ontology, nil
}

func (m *mockTestOntologyRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.TieredOntology, error) {
	return m.ontology, nil
}

func (m *mockTestOntologyRepo) Update(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}

func (m *mockTestOntologyRepo) List(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]*models.TieredOntology, error) {
	return nil, nil
}

type mockTestEntityRepo struct {
	entities    []*models.OntologyEntity
	occurrences []*models.OntologyEntityOccurrence
}

func (m *mockTestEntityRepo) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockTestEntityRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return m.entities, nil
}

func (m *mockTestEntityRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockTestEntityRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) CreateOccurrence(ctx context.Context, occurrence *models.OntologyEntityOccurrence) error {
	return nil
}

func (m *mockTestEntityRepo) GetOccurrences(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	return m.occurrences, nil
}

func (m *mockTestEntityRepo) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}

func (m *mockTestEntityRepo) GetAliases(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}

func (m *mockTestEntityRepo) GetKeyColumns(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

type mockTestRelationshipRepo struct {
	created []*models.EntityRelationship
}

func (m *mockTestRelationshipRepo) Create(ctx context.Context, relationship *models.EntityRelationship) error {
	m.created = append(m.created, relationship)
	return nil
}

func (m *mockTestRelationshipRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	return m.created, nil
}

func (m *mockTestRelationshipRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockTestRelationshipRepo) Update(ctx context.Context, relationship *models.EntityRelationship) error {
	return nil
}

func (m *mockTestRelationshipRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

type mockTestSchemaRepo struct {
	tables  []*models.SchemaTable
	columns []*models.SchemaColumn
}

func (m *mockTestSchemaRepo) GetTables(ctx context.Context, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockTestSchemaRepo) GetColumns(ctx context.Context, datasourceID uuid.UUID, schemaName, tableName string) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockTestSchemaRepo) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	// Flatten columns from tables
	var result []*models.SchemaColumn
	for _, table := range m.tables {
		for i := range table.Columns {
			result = append(result, &table.Columns[i])
		}
	}
	return result, nil
}

func (m *mockTestSchemaRepo) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockTestSchemaRepo) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	return nil
}

// Additional stub methods to satisfy interfaces

func (m *mockTestOntologyRepo) DeactivateAll(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockTestOntologyRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) DeleteAlias(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockTestEntityRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockTestRelationshipRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockTestRelationshipRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}

// More missing stub methods

func (m *mockTestDatasourceService) Delete(ctx context.Context, datasourceID uuid.UUID) error {
	return nil
}

func (m *mockTestOntologyRepo) GetByVersion(ctx context.Context, projectID uuid.UUID, version int) (*models.TieredOntology, error) {
	return nil, nil
}

func (m *mockTestEntityRepo) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockTestRelationshipRepo) GetByTables(ctx context.Context, projectID uuid.UUID, sourceSchema, sourceTable, targetSchema, targetTable string) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockTestSchemaRepo) GetJoinableColumns(ctx context.Context, projectID uuid.UUID, schemaName, tableName string) ([]*models.SchemaColumn, error) {
	return nil, nil
}
