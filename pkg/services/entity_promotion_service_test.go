package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// mockOntologyEntityRepo implements the minimum interface needed for promotion testing.
type mockPromotionEntityRepo struct {
	entities     []*models.OntologyEntity
	aliasesByID  map[uuid.UUID][]*models.OntologyEntityAlias
	updatedItems []*models.OntologyEntity
}

func (m *mockPromotionEntityRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	return m.entities, nil
}

func (m *mockPromotionEntityRepo) GetPromotedByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	var promoted []*models.OntologyEntity
	for _, e := range m.entities {
		if e.IsPromoted {
			promoted = append(promoted, e)
		}
	}
	return promoted, nil
}

func (m *mockPromotionEntityRepo) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return m.aliasesByID, nil
}

func (m *mockPromotionEntityRepo) Update(ctx context.Context, entity *models.OntologyEntity) error {
	m.updatedItems = append(m.updatedItems, entity)
	return nil
}

// Implement remaining OntologyEntityRepository methods with stubs
func (m *mockPromotionEntityRepo) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}
func (m *mockPromotionEntityRepo) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockPromotionEntityRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockPromotionEntityRepo) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockPromotionEntityRepo) GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockPromotionEntityRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockPromotionEntityRepo) DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockPromotionEntityRepo) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}
func (m *mockPromotionEntityRepo) MarkInferenceEntitiesStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockPromotionEntityRepo) ClearStaleFlag(ctx context.Context, entityID uuid.UUID) error {
	return nil
}
func (m *mockPromotionEntityRepo) GetStaleEntities(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockPromotionEntityRepo) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}
func (m *mockPromotionEntityRepo) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}
func (m *mockPromotionEntityRepo) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}
func (m *mockPromotionEntityRepo) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}
func (m *mockPromotionEntityRepo) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}
func (m *mockPromotionEntityRepo) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}
func (m *mockPromotionEntityRepo) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}
func (m *mockPromotionEntityRepo) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}
func (m *mockPromotionEntityRepo) CountOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockPromotionEntityRepo) GetOccurrenceTablesByEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]string, error) {
	return nil, nil
}

func (m *mockPromotionEntityRepo) TransferAliasesToEntity(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockPromotionEntityRepo) TransferKeyColumnsToEntity(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

// mockPromotionRelationshipRepo implements EntityRelationshipRepository for testing.
type mockPromotionRelationshipRepo struct {
	relationships []*models.EntityRelationship
}

func (m *mockPromotionRelationshipRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	return m.relationships, nil
}

// Stub remaining methods
func (m *mockPromotionRelationshipRepo) Create(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}
func (m *mockPromotionRelationshipRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}
func (m *mockPromotionRelationshipRepo) GetByOntologyGroupedByTarget(ctx context.Context, ontologyID uuid.UUID) (map[uuid.UUID][]*models.EntityRelationship, error) {
	return nil, nil
}
func (m *mockPromotionRelationshipRepo) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	return nil, nil
}
func (m *mockPromotionRelationshipRepo) GetByTargetEntity(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}
func (m *mockPromotionRelationshipRepo) GetByEntityPair(ctx context.Context, ontologyID uuid.UUID, fromEntityID uuid.UUID, toEntityID uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}
func (m *mockPromotionRelationshipRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}
func (m *mockPromotionRelationshipRepo) Upsert(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}
func (m *mockPromotionRelationshipRepo) Update(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}
func (m *mockPromotionRelationshipRepo) UpdateDescription(ctx context.Context, id uuid.UUID, description string) error {
	return nil
}
func (m *mockPromotionRelationshipRepo) UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error {
	return nil
}
func (m *mockPromotionRelationshipRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockPromotionRelationshipRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockPromotionRelationshipRepo) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}
func (m *mockPromotionRelationshipRepo) MarkInferenceRelationshipsStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockPromotionRelationshipRepo) ClearStaleFlag(ctx context.Context, relationshipID uuid.UUID) error {
	return nil
}
func (m *mockPromotionRelationshipRepo) GetStaleRelationships(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockPromotionRelationshipRepo) UpdateSourceEntityID(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockPromotionRelationshipRepo) UpdateTargetEntityID(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

// mockPromotionOntologyRepo implements OntologyRepository for testing.
type mockPromotionOntologyRepo struct {
	activeOntology *models.TieredOntology
}

func (m *mockPromotionOntologyRepo) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	return m.activeOntology, nil
}

// Stub remaining methods
func (m *mockPromotionOntologyRepo) Create(ctx context.Context, o *models.TieredOntology) error {
	return nil
}
func (m *mockPromotionOntologyRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.TieredOntology, error) {
	return nil, nil
}
func (m *mockPromotionOntologyRepo) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}
func (m *mockPromotionOntologyRepo) UpdateEntitySummary(ctx context.Context, ontologyID uuid.UUID, entityName string, summary *models.EntitySummary) error {
	return nil
}
func (m *mockPromotionOntologyRepo) UpdateColumnDetails(ctx context.Context, ontologyID uuid.UUID, tableName string, details []models.ColumnDetail) error {
	return nil
}
func (m *mockPromotionOntologyRepo) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return nil
}
func (m *mockPromotionOntologyRepo) UpdateMetadata(ctx context.Context, ontologyID uuid.UUID, metadata map[string]any) error {
	return nil
}
func (m *mockPromotionOntologyRepo) UpdateMergedContext(ctx context.Context, ontologyID uuid.UUID, mergedContext string) error {
	return nil
}
func (m *mockPromotionOntologyRepo) UpdateDomainDescriptions(ctx context.Context, ontologyID uuid.UUID, descriptions map[string]string) error {
	return nil
}
func (m *mockPromotionOntologyRepo) UpdateProjectPurpose(ctx context.Context, ontologyID uuid.UUID, purpose string) error {
	return nil
}
func (m *mockPromotionOntologyRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockPromotionOntologyRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockPromotionOntologyRepo) SetActive(ctx context.Context, projectID, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockPromotionOntologyRepo) GetColumn(ctx context.Context, ontologyID uuid.UUID, tableName, columnName string) (*models.ColumnDetail, error) {
	return nil, nil
}
func (m *mockPromotionOntologyRepo) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return nil
}

// mockPromotionSchemaRepo implements SchemaRepository for testing.
type mockPromotionSchemaRepo struct{}

func (m *mockPromotionSchemaRepo) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	return nil
}
func (m *mockPromotionSchemaRepo) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []repositories.TableKey) (int64, error) {
	return 0, nil
}
func (m *mockPromotionSchemaRepo) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockPromotionSchemaRepo) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockPromotionSchemaRepo) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID, selectedOnly bool) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetColumnsWithFeaturesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string, selectedOnly bool) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockPromotionSchemaRepo) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	return nil
}
func (m *mockPromotionSchemaRepo) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	return 0, nil
}
func (m *mockPromotionSchemaRepo) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockPromotionSchemaRepo) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64, sampleValues []string) error {
	return nil
}
func (m *mockPromotionSchemaRepo) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockPromotionSchemaRepo) UpdateColumnFeatures(ctx context.Context, projectID, columnID uuid.UUID, features *models.ColumnFeatures) error {
	return nil
}
func (m *mockPromotionSchemaRepo) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	return nil
}
func (m *mockPromotionSchemaRepo) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	return nil
}
func (m *mockPromotionSchemaRepo) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}
func (m *mockPromotionSchemaRepo) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *mockPromotionSchemaRepo) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	return nil
}
func (m *mockPromotionSchemaRepo) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return nil
}
func (m *mockPromotionSchemaRepo) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockPromotionSchemaRepo) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}
func (m *mockPromotionSchemaRepo) ClearColumnFeaturesByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockPromotionSchemaRepo) GetRelationshipsByMethod(ctx context.Context, projectID, datasourceID uuid.UUID, method string) ([]*models.SchemaRelationship, error) {
	return nil, nil
}

func TestEntityPromotionService_ScoreAndPromoteEntities(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	// Create test entities
	userEntityID := uuid.New()
	sessionEntityID := uuid.New()
	manualEntityID := uuid.New()

	entities := []*models.OntologyEntity{
		{
			ID:            userEntityID,
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          "User",
			PrimaryTable:  "users",
			PrimarySchema: "public",
			Source:        "inferred",
			IsPromoted:    true, // default
			CreatedAt:     time.Now(),
		},
		{
			ID:            sessionEntityID,
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          "Session",
			PrimaryTable:  "sessions",
			PrimarySchema: "public",
			Source:        "inferred",
			IsPromoted:    true, // default
			CreatedAt:     time.Now(),
		},
		{
			ID:            manualEntityID,
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          "Manual",
			PrimaryTable:  "manual_table",
			PrimarySchema: "public",
			Source:        "manual", // manual source - should be preserved
			IsPromoted:    false,    // manually demoted
			CreatedAt:     time.Now(),
		},
	}

	// Create relationships - User has 5 inbound references (hub)
	relationships := []*models.EntityRelationship{
		{SourceColumnTable: "orders", SourceColumnSchema: "public", SourceColumnName: "user_id", TargetColumnTable: "users", TargetColumnSchema: "public"},
		{SourceColumnTable: "profiles", SourceColumnSchema: "public", SourceColumnName: "user_id", TargetColumnTable: "users", TargetColumnSchema: "public"},
		{SourceColumnTable: "posts", SourceColumnSchema: "public", SourceColumnName: "author_id", TargetColumnTable: "users", TargetColumnSchema: "public"},
		{SourceColumnTable: "comments", SourceColumnSchema: "public", SourceColumnName: "user_id", TargetColumnTable: "users", TargetColumnSchema: "public"},
		{SourceColumnTable: "likes", SourceColumnSchema: "public", SourceColumnName: "user_id", TargetColumnTable: "users", TargetColumnSchema: "public"},
		// Session has only 1 inbound reference (not a hub)
		{SourceColumnTable: "users", SourceColumnSchema: "public", SourceColumnName: "last_session_id", TargetColumnTable: "sessions", TargetColumnSchema: "public"},
	}

	entityRepo := &mockPromotionEntityRepo{
		entities:    entities,
		aliasesByID: make(map[uuid.UUID][]*models.OntologyEntityAlias),
	}

	relationshipRepo := &mockPromotionRelationshipRepo{
		relationships: relationships,
	}

	ontologyRepo := &mockPromotionOntologyRepo{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}

	svc := NewEntityPromotionService(
		entityRepo,
		relationshipRepo,
		&mockPromotionSchemaRepo{},
		ontologyRepo,
		zap.NewNop(),
	)

	promoted, demoted, err := svc.ScoreAndPromoteEntities(ctx, projectID)
	require.NoError(t, err)

	// User should be promoted (5 inbound refs = 30 points, >= 50 threshold)
	// Session should be demoted (1 inbound ref = 0 points, < 50 threshold)
	// Manual should be counted but not updated (preserved)
	assert.Equal(t, 1, promoted, "Expected 1 promoted (User)")
	assert.Equal(t, 2, demoted, "Expected 2 demoted (Session + Manual counted as demoted)")

	// Verify updates were made only for non-manual entities
	assert.Len(t, entityRepo.updatedItems, 2, "Should update 2 entities (not the manual one)")

	// Check User was promoted
	var userUpdated *models.OntologyEntity
	for _, e := range entityRepo.updatedItems {
		if e.Name == "User" {
			userUpdated = e
		}
	}
	require.NotNil(t, userUpdated, "User entity should have been updated")
	assert.True(t, userUpdated.IsPromoted, "User should be promoted")
	assert.NotNil(t, userUpdated.PromotionScore, "User should have a promotion score")
	assert.GreaterOrEqual(t, *userUpdated.PromotionScore, PromotionThreshold, "User score should be >= threshold")

	// Check Session was demoted
	var sessionUpdated *models.OntologyEntity
	for _, e := range entityRepo.updatedItems {
		if e.Name == "Session" {
			sessionUpdated = e
		}
	}
	require.NotNil(t, sessionUpdated, "Session entity should have been updated")
	assert.False(t, sessionUpdated.IsPromoted, "Session should be demoted")
	assert.NotNil(t, sessionUpdated.PromotionScore, "Session should have a promotion score")
	assert.Less(t, *sessionUpdated.PromotionScore, PromotionThreshold, "Session score should be < threshold")
}

func TestEntityPromotionService_NoActiveOntology(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockPromotionOntologyRepo{
		activeOntology: nil, // No active ontology
	}

	svc := NewEntityPromotionService(
		&mockPromotionEntityRepo{},
		&mockPromotionRelationshipRepo{},
		&mockPromotionSchemaRepo{},
		ontologyRepo,
		zap.NewNop(),
	)

	promoted, demoted, err := svc.ScoreAndPromoteEntities(ctx, projectID)
	require.NoError(t, err)
	assert.Equal(t, 0, promoted)
	assert.Equal(t, 0, demoted)
}

func TestEntityPromotionService_NoEntities(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	entityRepo := &mockPromotionEntityRepo{
		entities: []*models.OntologyEntity{}, // No entities
	}

	ontologyRepo := &mockPromotionOntologyRepo{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}

	svc := NewEntityPromotionService(
		entityRepo,
		&mockPromotionRelationshipRepo{},
		&mockPromotionSchemaRepo{},
		ontologyRepo,
		zap.NewNop(),
	)

	promoted, demoted, err := svc.ScoreAndPromoteEntities(ctx, projectID)
	require.NoError(t, err)
	assert.Equal(t, 0, promoted)
	assert.Equal(t, 0, demoted)
}
