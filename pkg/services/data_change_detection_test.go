package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ============================================================================
// Mocks for detectPotentialFKs tests
// ============================================================================

// mockSchemaRepoForDCD implements only the methods needed by detectPotentialFKs.
type mockSchemaRepoForDCD struct {
	repositories.SchemaRepository
	columnsByTable map[uuid.UUID][]*models.SchemaColumn
}

func (m *mockSchemaRepoForDCD) ListColumnsByTable(_ context.Context, _ uuid.UUID, tableID uuid.UUID, _ bool) ([]*models.SchemaColumn, error) {
	return m.columnsByTable[tableID], nil
}

// mockDiscovererForDCD implements only CheckValueOverlap.
type mockDiscovererForDCD struct {
	datasource.SchemaDiscoverer
	overlapResult *datasource.ValueOverlapResult
}

func (m *mockDiscovererForDCD) CheckValueOverlap(_ context.Context, _, _, _, _, _, _ string, _ int) (*datasource.ValueOverlapResult, error) {
	return m.overlapResult, nil
}

// ============================================================================
// detectPotentialFKs tests
// ============================================================================

func TestDetectPotentialFKs_PrefersMetadataOverNameHeuristic(t *testing.T) {
	// Setup: a column named "creator_id" that has ColumnMetadata pointing to "accounts" table
	// (not "creator" which the name heuristic would derive). This verifies metadata takes priority.
	projectID := uuid.New()
	ordersTableID := uuid.New()
	accountsTableID := uuid.New()
	accountsPKColID := uuid.New()
	creatorColID := uuid.New()

	ordersTable := &models.SchemaTable{
		ID:         ordersTableID,
		ProjectID:  projectID,
		SchemaName: "public",
		TableName:  "orders",
	}
	accountsTable := &models.SchemaTable{
		ID:         accountsTableID,
		ProjectID:  projectID,
		SchemaName: "public",
		TableName:  "accounts",
	}

	creatorCol := &models.SchemaColumn{
		ID:            creatorColID,
		ProjectID:     projectID,
		SchemaTableID: ordersTableID,
		ColumnName:    "creator_id",
		DataType:      "uuid",
	}

	accountsPKCol := &models.SchemaColumn{
		ID:            accountsPKColID,
		ProjectID:     projectID,
		SchemaTableID: accountsTableID,
		ColumnName:    "account_id",
		IsPrimaryKey:  true,
	}

	purpose := models.PurposeIdentifier
	meta := &models.ColumnMetadata{
		ID:             uuid.New(),
		ProjectID:      projectID,
		SchemaColumnID: creatorColID,
		Purpose:        &purpose,
		Features: models.ColumnMetadataFeatures{
			IdentifierFeatures: &models.IdentifierFeatures{
				FKTargetTable:  "accounts",
				FKTargetColumn: "account_id",
				FKConfidence:   0.95,
			},
		},
	}

	schemaRepo := &mockSchemaRepoForDCD{
		columnsByTable: map[uuid.UUID][]*models.SchemaColumn{
			accountsTableID: {accountsPKCol},
		},
	}

	disc := &mockDiscovererForDCD{
		overlapResult: &datasource.ValueOverlapResult{
			SourceDistinct: 100,
			TargetDistinct: 50,
			MatchedCount:   95,
			MatchRate:      0.95,
		},
	}

	svc := &dataChangeDetectionService{
		schemaRepo: schemaRepo,
		config:     DefaultDataChangeDetectionConfig(),
		logger:     zap.NewNop(),
	}

	allTables := []*models.SchemaTable{ordersTable, accountsTable}
	metadataByColumnID := map[uuid.UUID]*models.ColumnMetadata{
		creatorColID: meta,
	}

	changes, err := svc.detectPotentialFKs(
		context.Background(),
		disc,
		ordersTable,
		[]*models.SchemaColumn{creatorCol},
		allTables,
		metadataByColumnID,
	)

	require.NoError(t, err)
	require.Len(t, changes, 1, "should detect one FK pattern")

	change := changes[0]
	assert.Equal(t, "accounts", change.NewValue["target_table"])
	assert.Equal(t, "account_id", change.NewValue["target_column"],
		"should use FKTargetColumn from metadata, not scan for PK")
}

func TestDetectPotentialFKs_FallsBackToNameHeuristicWhenNoMetadata(t *testing.T) {
	// Setup: a column named "user_id" with NO metadata → should fall back to _id suffix stripping
	projectID := uuid.New()
	ordersTableID := uuid.New()
	usersTableID := uuid.New()
	usersPKColID := uuid.New()
	userIDColID := uuid.New()

	ordersTable := &models.SchemaTable{
		ID:         ordersTableID,
		ProjectID:  projectID,
		SchemaName: "public",
		TableName:  "orders",
	}
	usersTable := &models.SchemaTable{
		ID:         usersTableID,
		ProjectID:  projectID,
		SchemaName: "public",
		TableName:  "user",
	}

	userIDCol := &models.SchemaColumn{
		ID:            userIDColID,
		ProjectID:     projectID,
		SchemaTableID: ordersTableID,
		ColumnName:    "user_id",
		DataType:      "uuid",
	}

	usersPKCol := &models.SchemaColumn{
		ID:            usersPKColID,
		ProjectID:     projectID,
		SchemaTableID: usersTableID,
		ColumnName:    "id",
		IsPrimaryKey:  true,
	}

	schemaRepo := &mockSchemaRepoForDCD{
		columnsByTable: map[uuid.UUID][]*models.SchemaColumn{
			usersTableID: {usersPKCol},
		},
	}

	disc := &mockDiscovererForDCD{
		overlapResult: &datasource.ValueOverlapResult{
			SourceDistinct: 100,
			TargetDistinct: 50,
			MatchedCount:   95,
			MatchRate:      0.95,
		},
	}

	svc := &dataChangeDetectionService{
		schemaRepo: schemaRepo,
		config:     DefaultDataChangeDetectionConfig(),
		logger:     zap.NewNop(),
	}

	allTables := []*models.SchemaTable{ordersTable, usersTable}
	// No metadata → heuristic path
	metadataByColumnID := map[uuid.UUID]*models.ColumnMetadata{}

	changes, err := svc.detectPotentialFKs(
		context.Background(),
		disc,
		ordersTable,
		[]*models.SchemaColumn{userIDCol},
		allTables,
		metadataByColumnID,
	)

	require.NoError(t, err)
	require.Len(t, changes, 1, "should detect one FK pattern via name heuristic")

	change := changes[0]
	assert.Equal(t, "user", change.NewValue["target_table"])
	assert.Equal(t, "id", change.NewValue["target_column"])
}

func TestDetectPotentialFKs_SchemaQualifiedFKTargetTable(t *testing.T) {
	// Setup: metadata has schema-qualified FKTargetTable like "public.accounts"
	projectID := uuid.New()
	ordersTableID := uuid.New()
	accountsTableID := uuid.New()
	accountsPKColID := uuid.New()
	acctColID := uuid.New()

	ordersTable := &models.SchemaTable{
		ID:         ordersTableID,
		ProjectID:  projectID,
		SchemaName: "public",
		TableName:  "orders",
	}
	accountsTable := &models.SchemaTable{
		ID:         accountsTableID,
		ProjectID:  projectID,
		SchemaName: "public",
		TableName:  "accounts",
	}

	acctCol := &models.SchemaColumn{
		ID:            acctColID,
		ProjectID:     projectID,
		SchemaTableID: ordersTableID,
		ColumnName:    "acct_ref",
		DataType:      "uuid",
	}

	accountsPKCol := &models.SchemaColumn{
		ID:            accountsPKColID,
		ProjectID:     projectID,
		SchemaTableID: accountsTableID,
		ColumnName:    "id",
		IsPrimaryKey:  true,
	}

	purpose := models.PurposeIdentifier
	meta := &models.ColumnMetadata{
		ID:             uuid.New(),
		ProjectID:      projectID,
		SchemaColumnID: acctColID,
		Purpose:        &purpose,
		Features: models.ColumnMetadataFeatures{
			IdentifierFeatures: &models.IdentifierFeatures{
				FKTargetTable: "public.accounts", // schema-qualified
				FKConfidence:  0.9,
			},
		},
	}

	schemaRepo := &mockSchemaRepoForDCD{
		columnsByTable: map[uuid.UUID][]*models.SchemaColumn{
			accountsTableID: {accountsPKCol},
		},
	}

	disc := &mockDiscovererForDCD{
		overlapResult: &datasource.ValueOverlapResult{
			SourceDistinct: 50,
			TargetDistinct: 50,
			MatchedCount:   48,
			MatchRate:      0.96,
		},
	}

	svc := &dataChangeDetectionService{
		schemaRepo: schemaRepo,
		config:     DefaultDataChangeDetectionConfig(),
		logger:     zap.NewNop(),
	}

	allTables := []*models.SchemaTable{ordersTable, accountsTable}
	metadataByColumnID := map[uuid.UUID]*models.ColumnMetadata{
		acctColID: meta,
	}

	changes, err := svc.detectPotentialFKs(
		context.Background(),
		disc,
		ordersTable,
		[]*models.SchemaColumn{acctCol},
		allTables,
		metadataByColumnID,
	)

	require.NoError(t, err)
	require.Len(t, changes, 1, "should resolve schema-qualified FKTargetTable")
	assert.Equal(t, "accounts", changes[0].NewValue["target_table"])
}

// ============================================================================
// Unit Tests for Helper Functions
// ============================================================================

func TestIsStringType(t *testing.T) {
	tests := []struct {
		dataType string
		expected bool
	}{
		{"text", true},
		{"varchar", true},
		{"varchar(255)", true},
		{"character varying", true},
		{"character varying(100)", true},
		{"char", true},
		{"char(10)", true},
		{"nvarchar", true},
		{"nvarchar(max)", true},
		{"nchar", true},
		{"ntext", true},
		{"string", true},
		{"integer", false},
		{"bigint", false},
		{"numeric", false},
		{"timestamp", false},
		{"boolean", false},
		{"uuid", false},
		{"json", false},
		{"jsonb", false},
	}

	for _, tc := range tests {
		t.Run(tc.dataType, func(t *testing.T) {
			result := isStringType(tc.dataType)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDefaultDataChangeDetectionConfig(t *testing.T) {
	cfg := DefaultDataChangeDetectionConfig()

	assert.Equal(t, 100, cfg.MaxDistinctValuesForEnum, "MaxDistinctValuesForEnum default")
	assert.Equal(t, 100, cfg.MaxEnumValueLength, "MaxEnumValueLength default")
	assert.Equal(t, 0.9, cfg.MinMatchRateForFK, "MinMatchRateForFK default")
}
