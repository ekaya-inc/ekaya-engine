package llm

import (
	"context"
	"encoding/json"
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
// Mock implementations
// ============================================================================

type mockOntologyRepo struct {
	getActiveFunc           func(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error)
	updateColumnDetailsFunc func(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error
}

func (m *mockOntologyRepo) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}
func (m *mockOntologyRepo) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	if m.getActiveFunc != nil {
		return m.getActiveFunc(ctx, projectID)
	}
	return nil, nil
}
func (m *mockOntologyRepo) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return nil
}
func (m *mockOntologyRepo) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	if m.updateColumnDetailsFunc != nil {
		return m.updateColumnDetailsFunc(ctx, projectID, tableName, columns)
	}
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
func (m *mockKnowledgeRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockKnowledgeRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockKnowledgeRepo) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}

type mockSchemaRepo struct {
	listTablesByDatasourceFunc func(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error)
	listAllColumnsByTableFunc  func(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error)
}

// Implement only the methods actually called by tool_executor.go
func (m *mockSchemaRepo) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	if m.listTablesByDatasourceFunc != nil {
		return m.listTablesByDatasourceFunc(ctx, projectID, datasourceID)
	}
	return nil, nil
}
func (m *mockSchemaRepo) ListAllColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	if m.listAllColumnsByTableFunc != nil {
		return m.listAllColumnsByTableFunc(ctx, projectID, tableID)
	}
	return nil, nil
}

// Stub the remaining SchemaRepository methods (not used by tool_executor.go)
func (m *mockSchemaRepo) ListAllTablesByDatasource(context.Context, uuid.UUID, uuid.UUID) ([]*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetTableByID(context.Context, uuid.UUID, uuid.UUID) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetTableByName(context.Context, uuid.UUID, uuid.UUID, string, string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepo) FindTableByName(context.Context, uuid.UUID, uuid.UUID, string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepo) UpsertTable(context.Context, *models.SchemaTable) error { return nil }
func (m *mockSchemaRepo) SoftDeleteRemovedTables(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ []repositories.TableKey) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepo) UpdateTableSelection(context.Context, uuid.UUID, uuid.UUID, bool) error {
	return nil
}
func (m *mockSchemaRepo) ListColumnsByTable(context.Context, uuid.UUID, uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepo) ListColumnsByDatasource(context.Context, uuid.UUID, uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetColumnsByTables(context.Context, uuid.UUID, []string) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetColumnCountByProject(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockSchemaRepo) GetColumnByID(context.Context, uuid.UUID, uuid.UUID) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetColumnByName(context.Context, uuid.UUID, string) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepo) UpsertColumn(context.Context, *models.SchemaColumn) error { return nil }
func (m *mockSchemaRepo) SoftDeleteRemovedColumns(context.Context, uuid.UUID, []string) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepo) UpdateColumnSelection(context.Context, uuid.UUID, uuid.UUID, bool) error {
	return nil
}
func (m *mockSchemaRepo) UpdateColumnStats(context.Context, uuid.UUID, *int64, *int64, *int64, *int64) error {
	return nil
}
func (m *mockSchemaRepo) ListRelationshipsByDatasource(context.Context, uuid.UUID, uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetRelationshipByID(context.Context, uuid.UUID, uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetRelationshipByColumns(context.Context, uuid.UUID, uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepo) UpsertRelationship(context.Context, *models.SchemaRelationship) error {
	return nil
}
func (m *mockSchemaRepo) UpdateRelationshipApproval(context.Context, uuid.UUID, uuid.UUID, bool) error {
	return nil
}
func (m *mockSchemaRepo) SoftDeleteRelationship(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (m *mockSchemaRepo) SoftDeleteOrphanedRelationships(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepo) GetRelationshipsByMethod(context.Context, uuid.UUID, uuid.UUID, string) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetRelationshipDetails(context.Context, uuid.UUID, uuid.UUID) ([]*models.RelationshipDetail, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetEmptyTables(context.Context, uuid.UUID, uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetOrphanTables(context.Context, uuid.UUID, uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepo) UpsertRelationshipWithMetrics(context.Context, *models.SchemaRelationship, *models.DiscoveryMetrics) error {
	return nil
}
func (m *mockSchemaRepo) GetJoinableColumns(context.Context, uuid.UUID, uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepo) UpdateColumnJoinability(context.Context, uuid.UUID, *int64, *int64, *int64, *bool, *string) error {
	return nil
}
func (m *mockSchemaRepo) GetPrimaryKeyColumns(context.Context, uuid.UUID, uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepo) GetNonPKColumnsByExactType(context.Context, uuid.UUID, uuid.UUID, string) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepo) SelectAllTablesAndColumns(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (m *mockSchemaRepo) DeleteInferredRelationshipsByProject(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

type mockQueryExecutor struct {
	queryFunc           func(ctx context.Context, sqlQuery string, limit int) (*datasource.QueryExecutionResult, error)
	quoteIdentifierFunc func(name string) string
}

func (m *mockQueryExecutor) Query(ctx context.Context, sqlQuery string, limit int) (*datasource.QueryExecutionResult, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sqlQuery, limit)
	}
	return &datasource.QueryExecutionResult{}, nil
}
func (m *mockQueryExecutor) QueryWithParams(context.Context, string, []any, int) (*datasource.QueryExecutionResult, error) {
	return nil, nil
}
func (m *mockQueryExecutor) Execute(context.Context, string) (*datasource.ExecuteResult, error) {
	return nil, nil
}
func (m *mockQueryExecutor) ExecuteWithParams(context.Context, string, []any) (*datasource.ExecuteResult, error) {
	return nil, nil
}
func (m *mockQueryExecutor) ValidateQuery(context.Context, string) error { return nil }
func (m *mockQueryExecutor) ExplainQuery(context.Context, string) (*datasource.ExplainResult, error) {
	return nil, nil
}
func (m *mockQueryExecutor) QuoteIdentifier(name string) string {
	if m.quoteIdentifierFunc != nil {
		return m.quoteIdentifierFunc(name)
	}
	return `"` + name + `"`
}
func (m *mockQueryExecutor) Close() error { return nil }

// ============================================================================
// Helper to create executor with defaults
// ============================================================================

func newTestExecutor(opts ...func(*OntologyToolExecutorConfig)) *OntologyToolExecutor {
	cfg := &OntologyToolExecutorConfig{
		ProjectID:     uuid.New(),
		OntologyID:    uuid.New(),
		DatasourceID:  uuid.New(),
		OntologyRepo:  &mockOntologyRepo{},
		KnowledgeRepo: &mockKnowledgeRepo{},
		SchemaRepo:    &mockSchemaRepo{},
		QueryExecutor: &mockQueryExecutor{},
		Logger:        zap.NewNop(),
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return NewOntologyToolExecutor(cfg)
}

// ============================================================================
// ExecuteTool dispatch tests
// ============================================================================

func TestOntologyToolExecutor_DispatchKnownTools(t *testing.T) {
	knownTools := []string{
		"query_column_values",
		"query_schema_metadata",
		"store_knowledge",
		"update_column",
	}

	for _, tool := range knownTools {
		t.Run(tool, func(t *testing.T) {
			executor := newTestExecutor()
			// Pass minimal valid args — we're just testing dispatch routing, not validation
			_, err := executor.ExecuteTool(context.Background(), tool, `{}`)
			// Some tools will return validation errors for empty args, but NOT "unknown tool"
			if err != nil {
				assert.NotContains(t, err.Error(), "unknown tool")
			}
		})
	}
}

func TestOntologyToolExecutor_UnknownToolReturnsError(t *testing.T) {
	executor := newTestExecutor()

	_, err := executor.ExecuteTool(context.Background(), "nonexistent_tool", `{}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool: nonexistent_tool")
}

// ============================================================================
// queryColumnValues tests
// ============================================================================

func TestOntologyToolExecutor_QueryColumnValues_MissingTableName(t *testing.T) {
	executor := newTestExecutor()

	_, err := executor.ExecuteTool(context.Background(), "query_column_values",
		`{"column_name": "email"}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "table_name and column_name are required")
}

func TestOntologyToolExecutor_QueryColumnValues_MissingColumnName(t *testing.T) {
	executor := newTestExecutor()

	_, err := executor.ExecuteTool(context.Background(), "query_column_values",
		`{"table_name": "users"}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "table_name and column_name are required")
}

func TestOntologyToolExecutor_QueryColumnValues_LimitDefaults(t *testing.T) {
	var capturedLimit int
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.QueryExecutor = &mockQueryExecutor{
			queryFunc: func(ctx context.Context, sqlQuery string, limit int) (*datasource.QueryExecutionResult, error) {
				capturedLimit = limit
				return &datasource.QueryExecutionResult{Rows: []map[string]any{}}, nil
			},
		}
	})

	_, err := executor.ExecuteTool(context.Background(), "query_column_values",
		`{"table_name": "users", "column_name": "email"}`)

	require.NoError(t, err)
	assert.Equal(t, 10, capturedLimit)
}

func TestOntologyToolExecutor_QueryColumnValues_LimitCappedAt100(t *testing.T) {
	var capturedLimit int
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.QueryExecutor = &mockQueryExecutor{
			queryFunc: func(ctx context.Context, sqlQuery string, limit int) (*datasource.QueryExecutionResult, error) {
				capturedLimit = limit
				return &datasource.QueryExecutionResult{Rows: []map[string]any{}}, nil
			},
		}
	})

	_, err := executor.ExecuteTool(context.Background(), "query_column_values",
		`{"table_name": "users", "column_name": "email", "limit": 500}`)

	require.NoError(t, err)
	assert.Equal(t, 100, capturedLimit)
}

func TestOntologyToolExecutor_QueryColumnValues_NilQueryExecutor(t *testing.T) {
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.QueryExecutor = nil
	})

	result, err := executor.ExecuteTool(context.Background(), "query_column_values",
		`{"table_name": "users", "column_name": "email"}`)

	require.NoError(t, err) // Returns JSON error, not Go error
	assert.Contains(t, result, "No query executor available")
}

func TestOntologyToolExecutor_QueryColumnValues_QueryError(t *testing.T) {
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.QueryExecutor = &mockQueryExecutor{
			queryFunc: func(ctx context.Context, sqlQuery string, limit int) (*datasource.QueryExecutionResult, error) {
				return nil, assert.AnError
			},
		}
	})

	result, err := executor.ExecuteTool(context.Background(), "query_column_values",
		`{"table_name": "users", "column_name": "email"}`)

	require.NoError(t, err) // Returns JSON error, not Go error
	assert.Contains(t, result, "Query failed")
}

// ============================================================================
// querySchemaMetadata tests
// ============================================================================

func TestOntologyToolExecutor_QuerySchemaMetadata_ValidTableFilter(t *testing.T) {
	tableID := uuid.New()
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.SchemaRepo = &mockSchemaRepo{
			listTablesByDatasourceFunc: func(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
				return []*models.SchemaTable{
					{ID: tableID, TableName: "users"},
					{ID: uuid.New(), TableName: "orders"},
				}, nil
			},
			listAllColumnsByTableFunc: func(ctx context.Context, projectID, tID uuid.UUID) ([]*models.SchemaColumn, error) {
				if tID == tableID {
					return []*models.SchemaColumn{
						{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
						{ColumnName: "name", DataType: "text"},
					}, nil
				}
				return nil, nil
			},
		}
	})

	result, err := executor.ExecuteTool(context.Background(), "query_schema_metadata",
		`{"table_name": "users"}`)

	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &resp))
	assert.Equal(t, float64(1), resp["table_count"])
	tables := resp["tables"].([]any)
	require.Len(t, tables, 1)
	table := tables[0].(map[string]any)
	assert.Equal(t, "users", table["name"])
}

func TestOntologyToolExecutor_QuerySchemaMetadata_EmptyTableNameReturnsAll(t *testing.T) {
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.SchemaRepo = &mockSchemaRepo{
			listTablesByDatasourceFunc: func(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
				return []*models.SchemaTable{
					{ID: uuid.New(), TableName: "users"},
					{ID: uuid.New(), TableName: "orders"},
				}, nil
			},
			listAllColumnsByTableFunc: func(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
				return []*models.SchemaColumn{}, nil
			},
		}
	})

	result, err := executor.ExecuteTool(context.Background(), "query_schema_metadata",
		`{"table_name": ""}`)

	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &resp))
	assert.Equal(t, float64(2), resp["table_count"])
}

func TestOntologyToolExecutor_QuerySchemaMetadata_RepoErrorPropagates(t *testing.T) {
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.SchemaRepo = &mockSchemaRepo{
			listTablesByDatasourceFunc: func(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
				return nil, assert.AnError
			},
		}
	})

	_, err := executor.ExecuteTool(context.Background(), "query_schema_metadata", `{}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list tables")
}

// ============================================================================
// storeKnowledge tests
// ============================================================================

func TestOntologyToolExecutor_StoreKnowledge_MissingFactType(t *testing.T) {
	executor := newTestExecutor()

	_, err := executor.ExecuteTool(context.Background(), "store_knowledge",
		`{"value": "some fact"}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fact_type and value are required")
}

func TestOntologyToolExecutor_StoreKnowledge_MissingValue(t *testing.T) {
	executor := newTestExecutor()

	_, err := executor.ExecuteTool(context.Background(), "store_knowledge",
		`{"fact_type": "terminology"}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fact_type and value are required")
}

func TestOntologyToolExecutor_StoreKnowledge_InvalidFactType(t *testing.T) {
	executor := newTestExecutor()

	_, err := executor.ExecuteTool(context.Background(), "store_knowledge",
		`{"fact_type": "bogus_type", "value": "test"}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid fact_type: bogus_type")
}

func TestOntologyToolExecutor_StoreKnowledge_ValidFactTypes(t *testing.T) {
	validTypes := []string{"terminology", "business_rule", "data_relationship", "constraint", "context"}

	for _, ft := range validTypes {
		t.Run(ft, func(t *testing.T) {
			executor := newTestExecutor()
			args, _ := json.Marshal(map[string]string{"fact_type": ft, "value": "test value"})

			result, err := executor.ExecuteTool(context.Background(), "store_knowledge", string(args))

			require.NoError(t, err)
			var resp map[string]any
			require.NoError(t, json.Unmarshal([]byte(result), &resp))
			assert.Equal(t, true, resp["success"])
			assert.Equal(t, ft, resp["fact_type"])
		})
	}
}

// ============================================================================
// updateColumn tests
// ============================================================================

func TestOntologyToolExecutor_UpdateColumn_MissingTableName(t *testing.T) {
	executor := newTestExecutor()

	_, err := executor.ExecuteTool(context.Background(), "update_column",
		`{"column_name": "email"}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "table_name and column_name are required")
}

func TestOntologyToolExecutor_UpdateColumn_MissingColumnName(t *testing.T) {
	executor := newTestExecutor()

	_, err := executor.ExecuteTool(context.Background(), "update_column",
		`{"table_name": "users"}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "table_name and column_name are required")
}

func TestOntologyToolExecutor_UpdateColumn_InvalidSemanticType(t *testing.T) {
	executor := newTestExecutor()

	_, err := executor.ExecuteTool(context.Background(), "update_column",
		`{"table_name": "users", "column_name": "email", "semantic_type": "bogus"}`)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid semantic_type: bogus")
}

func TestOntologyToolExecutor_UpdateColumn_ValidSemanticTypes(t *testing.T) {
	validTypes := []string{
		"identifier", "name", "description", "amount", "quantity",
		"date", "timestamp", "status", "flag", "code", "reference", "other",
	}

	for _, st := range validTypes {
		t.Run(st, func(t *testing.T) {
			projectID := uuid.New()
			executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
				cfg.ProjectID = projectID
				cfg.OntologyRepo = &mockOntologyRepo{
					getActiveFunc: func(ctx context.Context, pID uuid.UUID) (*models.TieredOntology, error) {
						return &models.TieredOntology{
							ProjectID:     pID,
							ColumnDetails: map[string][]models.ColumnDetail{},
						}, nil
					},
				}
			})

			args, _ := json.Marshal(map[string]string{
				"table_name":    "users",
				"column_name":   "email",
				"semantic_type": st,
			})

			result, err := executor.ExecuteTool(context.Background(), "update_column", string(args))

			require.NoError(t, err)
			var resp map[string]any
			require.NoError(t, json.Unmarshal([]byte(result), &resp))
			assert.Equal(t, true, resp["success"])
		})
	}
}

func TestOntologyToolExecutor_UpdateColumn_UpdateExistingColumn(t *testing.T) {
	var savedColumns []models.ColumnDetail
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.OntologyRepo = &mockOntologyRepo{
			getActiveFunc: func(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
				return &models.TieredOntology{
					ProjectID: projectID,
					ColumnDetails: map[string][]models.ColumnDetail{
						"users": {
							{Name: "email", Description: "old desc", SemanticType: "identifier"},
						},
					},
				}, nil
			},
			updateColumnDetailsFunc: func(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
				savedColumns = columns
				return nil
			},
		}
	})

	_, err := executor.ExecuteTool(context.Background(), "update_column",
		`{"table_name": "users", "column_name": "email", "description": "new desc"}`)

	require.NoError(t, err)
	require.Len(t, savedColumns, 1)
	assert.Equal(t, "new desc", savedColumns[0].Description)
	assert.Equal(t, "identifier", savedColumns[0].SemanticType) // Preserved
}

func TestOntologyToolExecutor_UpdateColumn_CreateNewColumn(t *testing.T) {
	var savedColumns []models.ColumnDetail
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.OntologyRepo = &mockOntologyRepo{
			getActiveFunc: func(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
				return &models.TieredOntology{
					ProjectID:     projectID,
					ColumnDetails: map[string][]models.ColumnDetail{},
				}, nil
			},
			updateColumnDetailsFunc: func(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
				savedColumns = columns
				return nil
			},
		}
	})

	_, err := executor.ExecuteTool(context.Background(), "update_column",
		`{"table_name": "users", "column_name": "new_col", "description": "a new column", "semantic_type": "name"}`)

	require.NoError(t, err)
	require.Len(t, savedColumns, 1)
	assert.Equal(t, "new_col", savedColumns[0].Name)
	assert.Equal(t, "a new column", savedColumns[0].Description)
	assert.Equal(t, "name", savedColumns[0].SemanticType)
}

func TestOntologyToolExecutor_UpdateColumn_BusinessNameAddedToSynonyms(t *testing.T) {
	var savedColumns []models.ColumnDetail
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.OntologyRepo = &mockOntologyRepo{
			getActiveFunc: func(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
				return &models.TieredOntology{
					ProjectID: projectID,
					ColumnDetails: map[string][]models.ColumnDetail{
						"users": {
							{Name: "email", Synonyms: []string{"e-mail"}},
						},
					},
				}, nil
			},
			updateColumnDetailsFunc: func(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
				savedColumns = columns
				return nil
			},
		}
	})

	_, err := executor.ExecuteTool(context.Background(), "update_column",
		`{"table_name": "users", "column_name": "email", "business_name": "Email Address"}`)

	require.NoError(t, err)
	require.Len(t, savedColumns, 1)
	assert.Contains(t, savedColumns[0].Synonyms, "e-mail")
	assert.Contains(t, savedColumns[0].Synonyms, "Email Address")
}

func TestOntologyToolExecutor_UpdateColumn_NoActiveOntology(t *testing.T) {
	executor := newTestExecutor(func(cfg *OntologyToolExecutorConfig) {
		cfg.OntologyRepo = &mockOntologyRepo{
			getActiveFunc: func(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
				return nil, nil
			},
		}
	})

	result, err := executor.ExecuteTool(context.Background(), "update_column",
		`{"table_name": "users", "column_name": "email"}`)

	require.NoError(t, err) // Returns JSON error, not Go error
	assert.Contains(t, result, "No active ontology found")
}
