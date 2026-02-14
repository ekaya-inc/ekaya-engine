package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// selectedAwareSchemaRepo tracks whether selectedOnly=true was passed.
type selectedAwareSchemaRepo struct {
	repositories.SchemaRepository

	selectedTables []*models.SchemaTable
	allTables      []*models.SchemaTable
	allColumns     []*models.SchemaColumn

	// Track calls to verify selectedOnly parameter
	listTablesSelectedOnlyValues []bool
}

func (m *selectedAwareSchemaRepo) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	m.listTablesSelectedOnlyValues = append(m.listTablesSelectedOnlyValues, selectedOnly)
	if selectedOnly {
		return m.selectedTables, nil
	}
	return m.allTables, nil
}

func (m *selectedAwareSchemaRepo) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return m.allColumns, nil
}

// selectedAwareMetadataRepo returns metadata for FK source identification.
type selectedAwareMetadataRepo struct {
	repositories.ColumnMetadataRepository

	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata
}

func (m *selectedAwareMetadataRepo) GetBySchemaColumnIDs(ctx context.Context, schemaColumnIDs []uuid.UUID) ([]*models.ColumnMetadata, error) {
	var result []*models.ColumnMetadata
	for _, id := range schemaColumnIDs {
		if meta, ok := m.metadataByColumnID[id]; ok {
			result = append(result, meta)
		}
	}
	return result, nil
}

func TestIdentifyFKSources_OnlyUsesSelectedTables(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	selectedTableID := uuid.New()
	nonSelectedTableID := uuid.New()

	selectedColID := uuid.New()
	nonSelectedColID := uuid.New()

	fkRole := "foreign_key"

	schemaRepo := &selectedAwareSchemaRepo{
		selectedTables: []*models.SchemaTable{
			{ID: selectedTableID, TableName: "users", SchemaName: "public", IsSelected: true},
		},
		allTables: []*models.SchemaTable{
			{ID: selectedTableID, TableName: "users", SchemaName: "public", IsSelected: true},
			{ID: nonSelectedTableID, TableName: "accounts", SchemaName: "public", IsSelected: false},
		},
		allColumns: []*models.SchemaColumn{
			{ID: selectedColID, SchemaTableID: selectedTableID, ColumnName: "account_id", DataType: "text"},
			{ID: nonSelectedColID, SchemaTableID: nonSelectedTableID, ColumnName: "user_id", DataType: "text"},
		},
	}

	metadataRepo := &selectedAwareMetadataRepo{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			selectedColID:    {SchemaColumnID: selectedColID, Role: &fkRole},
			nonSelectedColID: {SchemaColumnID: nonSelectedColID, Role: &fkRole},
		},
	}

	collector := &relationshipCandidateCollector{
		schemaRepo:         schemaRepo,
		columnMetadataRepo: metadataRepo,
		logger:             zap.NewNop(),
	}

	sources, _, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	// Verify selectedOnly=true was passed
	require.NotEmpty(t, schemaRepo.listTablesSelectedOnlyValues, "ListTablesByDatasource should have been called")
	assert.True(t, schemaRepo.listTablesSelectedOnlyValues[0],
		"identifyFKSources must pass selectedOnly=true to ListTablesByDatasource")

	// Only the column from the selected table should be a source
	var sourceTableNames []string
	for _, s := range sources {
		sourceTableNames = append(sourceTableNames, s.TableName)
	}
	assert.Contains(t, sourceTableNames, "users")
	assert.NotContains(t, sourceTableNames, "accounts",
		"columns from non-selected tables must not appear as FK sources")
}

func TestIdentifyFKTargets_OnlyUsesSelectedTables(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	selectedTableID := uuid.New()
	nonSelectedTableID := uuid.New()

	schemaRepo := &selectedAwareSchemaRepo{
		selectedTables: []*models.SchemaTable{
			{ID: selectedTableID, TableName: "users", SchemaName: "public", IsSelected: true},
		},
		allTables: []*models.SchemaTable{
			{ID: selectedTableID, TableName: "users", SchemaName: "public", IsSelected: true},
			{ID: nonSelectedTableID, TableName: "accounts", SchemaName: "public", IsSelected: false},
		},
		allColumns: []*models.SchemaColumn{
			{ID: uuid.New(), SchemaTableID: selectedTableID, ColumnName: "user_id", DataType: "text", IsPrimaryKey: true},
			{ID: uuid.New(), SchemaTableID: nonSelectedTableID, ColumnName: "account_id", DataType: "text", IsPrimaryKey: true},
		},
	}

	collector := &relationshipCandidateCollector{
		schemaRepo: schemaRepo,
		logger:     zap.NewNop(),
	}

	targets, err := collector.identifyFKTargets(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	// Verify selectedOnly=true was passed
	require.NotEmpty(t, schemaRepo.listTablesSelectedOnlyValues, "ListTablesByDatasource should have been called")
	assert.True(t, schemaRepo.listTablesSelectedOnlyValues[0],
		"identifyFKTargets must pass selectedOnly=true to ListTablesByDatasource")

	// Only the PK from the selected table should be a target
	var targetTableNames []string
	for _, tgt := range targets {
		targetTableNames = append(targetTableNames, tgt.TableName)
	}
	assert.Contains(t, targetTableNames, "users")
	assert.NotContains(t, targetTableNames, "accounts",
		"columns from non-selected tables must not appear as FK targets")
}
