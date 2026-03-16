package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

type glossaryPipelineLLMClient struct {
	responses []string
	callCount int
}

func (m *glossaryPipelineLLMClient) GenerateResponse(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
	if m.callCount >= len(m.responses) {
		return nil, fmt.Errorf("unexpected LLM call %d for system message %q", m.callCount+1, systemMessage)
	}
	response := m.responses[m.callCount]
	m.callCount++
	return &llm.GenerateResponseResult{Content: response}, nil
}

func (m *glossaryPipelineLLMClient) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	return nil, nil
}

func (m *glossaryPipelineLLMClient) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	return nil, nil
}

func (m *glossaryPipelineLLMClient) GetModel() string {
	return "test-model"
}

func (m *glossaryPipelineLLMClient) GetEndpoint() string {
	return "test-endpoint"
}

type glossaryPipelineQueryExecutor struct {
	rowCounts    map[string]int64
	nonNullCount map[string]int64
	joinCounts   map[string]int64
}

func (m *glossaryPipelineQueryExecutor) Query(ctx context.Context, sqlQuery string, limit int) (*datasource.QueryExecutionResult, error) {
	lower := strings.ToLower(sqlQuery)

	switch {
	case strings.Contains(lower, "as row_count"):
		table := extractTestTableName(lower)
		return testSingleValueResult("row_count", m.rowCounts[table]), nil
	case strings.Contains(lower, "as non_null_count"):
		table := extractTestTableName(lower)
		column := extractTestQuotedIdentifier(lower, "count(")
		return testSingleValueResult("non_null_count", m.nonNullCount[table+"."+column]), nil
	case strings.Contains(lower, "as join_count"):
		source := extractTestTableName(lower)
		target := extractTestJoinTarget(lower)
		return testSingleValueResult("join_count", m.joinCounts[source+"->"+target]), nil
	default:
		alias := extractTestMetricAlias(sqlQuery)
		if alias == "" {
			alias = "metric_value"
		}
		return &datasource.QueryExecutionResult{
			Columns: []datasource.ColumnInfo{{Name: alias, Type: "numeric"}},
			Rows:    []map[string]any{{alias: float64(42)}},
		}, nil
	}
}

func (m *glossaryPipelineQueryExecutor) QueryWithParams(ctx context.Context, sqlQuery string, params []any, limit int) (*datasource.QueryExecutionResult, error) {
	return m.Query(ctx, sqlQuery, limit)
}

func (m *glossaryPipelineQueryExecutor) Execute(ctx context.Context, sqlStatement string) (*datasource.ExecuteResult, error) {
	return &datasource.ExecuteResult{}, nil
}

func (m *glossaryPipelineQueryExecutor) ExecuteWithParams(ctx context.Context, sqlStatement string, params []any) (*datasource.ExecuteResult, error) {
	return &datasource.ExecuteResult{}, nil
}

func (m *glossaryPipelineQueryExecutor) ValidateQuery(ctx context.Context, sqlQuery string) error {
	return nil
}

func (m *glossaryPipelineQueryExecutor) ExplainQuery(ctx context.Context, sqlQuery string) (*datasource.ExplainResult, error) {
	return &datasource.ExplainResult{}, nil
}

func (m *glossaryPipelineQueryExecutor) QuoteIdentifier(name string) string {
	return `"` + name + `"`
}

func (m *glossaryPipelineQueryExecutor) Close() error {
	return nil
}

type glossaryPipelineAdapterFactory struct {
	executor datasource.QueryExecutor
}

func (m *glossaryPipelineAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, nil
}

func (m *glossaryPipelineAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	return nil, nil
}

func (m *glossaryPipelineAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return m.executor, nil
}

func (m *glossaryPipelineAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

func TestGlossaryService_RunAutoGenerate_SQLFirstPersistsQualifiedTerms(t *testing.T) {
	projectID := uuid.New()
	orderTableID := uuid.New()
	orderIDColumnID := uuid.New()
	amountColumnID := uuid.New()
	statusColumnID := uuid.New()
	createdAtColumnID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	schemaRepo := &mockSchemaRepoForGlossary{
		tables: []*models.SchemaTable{
			{ID: orderTableID, ProjectID: projectID, TableName: "orders", SchemaName: "public"},
		},
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": {
				{ID: orderIDColumnID, ProjectID: projectID, SchemaTableID: orderTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ID: amountColumnID, ProjectID: projectID, SchemaTableID: orderTableID, ColumnName: "amount_cents", DataType: "bigint"},
				{ID: statusColumnID, ProjectID: projectID, SchemaTableID: orderTableID, ColumnName: "status", DataType: "text"},
				{ID: createdAtColumnID, ProjectID: projectID, SchemaTableID: orderTableID, ColumnName: "created_at", DataType: "timestamp"},
			},
		},
	}
	columnMetadataRepo := &mockColumnMetadataRepoForGlossary{
		projectMetadata: []*models.ColumnMetadata{
			{ProjectID: projectID, SchemaColumnID: amountColumnID, Role: glossaryStrPtr("measure"), SemanticType: glossaryStrPtr("currency_cents")},
			{ProjectID: projectID, SchemaColumnID: statusColumnID, Role: glossaryStrPtr("dimension"), Features: models.ColumnMetadataFeatures{
				EnumFeatures: &models.EnumFeatures{Values: []models.ColumnEnumValue{
					{Value: "completed"},
					{Value: "pending"},
				}},
			}},
			{ProjectID: projectID, SchemaColumnID: createdAtColumnID, SemanticType: glossaryStrPtr("event_time")},
		},
	}
	llmClient := &glossaryPipelineLLMClient{
		responses: []string{
			`{"areas":[{"rank":1,"area":"revenue path","business_rationale":"Completed orders produce reliable revenue examples.","tables":["orders"],"focus_columns":["amount_cents","status","created_at"]},{"rank":2,"area":"completed order count","business_rationale":"Order completion is supported directly by the status column.","tables":["orders"],"focus_columns":["status","created_at"]}]}`,
			`{"sql":"SELECT SUM(amount_cents) / 100.0 AS total_revenue FROM orders WHERE status = 'completed'","base_table":"orders","metric_rationale":"Completed orders sum directly from the measure column.","used_tables":["orders"]}`,
			`{"term":"Total Revenue","definition":"Total completed order revenue with documented currency normalization applied.","aliases":["Revenue"]}`,
			`{"sql":"SELECT COUNT(*) AS completed_orders FROM orders WHERE status = 'completed'","base_table":"orders","metric_rationale":"Completed orders are counted directly from the status column.","used_tables":["orders"]}`,
			`{"term":"Completed Orders","definition":"Count of orders whose status is completed.","aliases":["Order Count"]}`,
		},
	}
	llmFactory := &mockLLMFactoryForGlossary{client: llmClient}
	executor := &glossaryPipelineQueryExecutor{
		rowCounts: map[string]int64{
			"orders": 250,
		},
		nonNullCount: map[string]int64{
			"orders.amount_cents": 250,
			"orders.status":       250,
			"orders.created_at":   250,
		},
	}
	svc := NewGlossaryService(
		glossaryRepo,
		columnMetadataRepo,
		nil,
		schemaRepo,
		&mockProjectServiceForGlossary{project: &models.Project{ID: projectID}},
		&mockDatasourceServiceForGlossary{},
		&glossaryPipelineAdapterFactory{executor: executor},
		llmFactory,
		mockGetTenant(),
		zap.NewNop(),
		"test",
	)

	err := svc.RunAutoGenerate(context.Background(), projectID)
	require.NoError(t, err)

	status := svc.GetGenerationStatus(projectID)
	assert.Equal(t, "completed", status.Status)
	assert.Contains(t, status.Message, "Generated 2 verified glossary terms")

	terms, err := glossaryRepo.GetByProject(context.Background(), projectID)
	require.NoError(t, err)
	require.Len(t, terms, 2)
	assert.Equal(t, "inferred", terms[0].Source)
	for _, term := range terms {
		assert.NotEmpty(t, term.DefiningSQL)
		assert.NotEmpty(t, term.OutputColumns)
	}
}

func TestGlossaryService_RunAutoGenerate_NoQualifiedTermsPreservesExistingInferred(t *testing.T) {
	projectID := uuid.New()
	userTableID := uuid.New()
	loginCountColumnID := uuid.New()
	createdAtColumnID := uuid.New()

	glossaryRepo := newMockGlossaryRepo()
	glossaryRepo.terms[uuid.New()] = &models.BusinessGlossaryTerm{
		ID:          uuid.New(),
		ProjectID:   projectID,
		Term:        "Legacy Inferred Term",
		Definition:  "Previous inferred metric",
		DefiningSQL: "SELECT 1 AS legacy_metric",
		Source:      models.ProvenanceInferred,
	}

	schemaRepo := &mockSchemaRepoForGlossary{
		tables: []*models.SchemaTable{
			{ID: userTableID, ProjectID: projectID, TableName: "users", SchemaName: "public"},
		},
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": {
				{ID: loginCountColumnID, ProjectID: projectID, SchemaTableID: userTableID, ColumnName: "login_count", DataType: "bigint"},
				{ID: createdAtColumnID, ProjectID: projectID, SchemaTableID: userTableID, ColumnName: "created_at", DataType: "timestamp"},
			},
		},
	}
	columnMetadataRepo := &mockColumnMetadataRepoForGlossary{
		projectMetadata: []*models.ColumnMetadata{
			{ProjectID: projectID, SchemaColumnID: loginCountColumnID, Role: glossaryStrPtr("measure")},
			{ProjectID: projectID, SchemaColumnID: createdAtColumnID, SemanticType: glossaryStrPtr("event_time")},
		},
	}
	llmClient := &glossaryPipelineLLMClient{
		responses: []string{
			`{"areas":[{"rank":1,"area":"recent activity","business_rationale":"Recent user activity could be interesting if the time window were documented.","tables":["users"],"focus_columns":["login_count","created_at"]}]}`,
			`{"sql":"SELECT SUM(login_count) AS active_logins FROM users WHERE created_at >= NOW() - INTERVAL '30 days'","base_table":"users","metric_rationale":"Counts activity from the last 30 days.","used_tables":["users"]}`,
		},
	}
	llmFactory := &mockLLMFactoryForGlossary{client: llmClient}
	executor := &glossaryPipelineQueryExecutor{
		rowCounts: map[string]int64{
			"users": 80,
		},
		nonNullCount: map[string]int64{
			"users.login_count": 80,
			"users.created_at":  80,
		},
	}
	svc := NewGlossaryService(
		glossaryRepo,
		columnMetadataRepo,
		nil,
		schemaRepo,
		&mockProjectServiceForGlossary{project: &models.Project{ID: projectID}},
		&mockDatasourceServiceForGlossary{},
		&glossaryPipelineAdapterFactory{executor: executor},
		llmFactory,
		mockGetTenant(),
		zap.NewNop(),
		"test",
	)

	err := svc.RunAutoGenerate(context.Background(), projectID)
	require.NoError(t, err)

	status := svc.GetGenerationStatus(projectID)
	assert.Equal(t, glossaryGenerationStatusNoQualifiedTerms, status.Status)
	assert.Contains(t, status.Message, "preserved")

	terms, err := glossaryRepo.GetByProject(context.Background(), projectID)
	require.NoError(t, err)
	require.Len(t, terms, 1)
	assert.Equal(t, "Legacy Inferred Term", terms[0].Term)
}

func testSingleValueResult(column string, value int64) *datasource.QueryExecutionResult {
	return &datasource.QueryExecutionResult{
		Columns: []datasource.ColumnInfo{{Name: column, Type: "bigint"}},
		Rows:    []map[string]any{{column: value}},
	}
}

func extractTestTableName(lowerSQL string) string {
	fromPattern := regexp.MustCompile(`from\s+"?([a-zA-Z_][\w]*)"?`)
	matches := fromPattern.FindStringSubmatch(lowerSQL)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func extractTestJoinTarget(lowerSQL string) string {
	joinPattern := regexp.MustCompile(`join\s+"?([a-zA-Z_][\w]*)"?`)
	matches := joinPattern.FindStringSubmatch(lowerSQL)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func extractTestQuotedIdentifier(lowerSQL string, prefix string) string {
	index := strings.Index(lowerSQL, prefix)
	if index == -1 {
		return ""
	}
	rest := lowerSQL[index+len(prefix):]
	start := strings.Index(rest, `"`)
	if start == -1 {
		return ""
	}
	rest = rest[start+1:]
	end := strings.Index(rest, `"`)
	if end == -1 {
		return ""
	}
	return rest[:end]
}

func extractTestMetricAlias(sql string) string {
	aliasPattern := regexp.MustCompile(`(?i)\bas\s+([a-zA-Z_][\w]*)`)
	matches := aliasPattern.FindStringSubmatch(sql)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func glossaryStrPtr(value string) *string {
	return &value
}
