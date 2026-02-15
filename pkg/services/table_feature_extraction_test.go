package services

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ============================================================================
// Helper: build ColumnMetadata from purpose/role/description/semanticType/identifierFeatures
// ============================================================================

func tfeColMeta(colID uuid.UUID, purpose, role, description, semanticType string, idFeatures *models.IdentifierFeatures) *models.ColumnMetadata {
	meta := &models.ColumnMetadata{
		SchemaColumnID: colID,
	}
	if purpose != "" {
		meta.Purpose = &purpose
	}
	if role != "" {
		meta.Role = &role
	}
	if description != "" {
		meta.Description = &description
	}
	if semanticType != "" {
		meta.SemanticType = &semanticType
	}
	if idFeatures != nil {
		meta.Features.IdentifierFeatures = idFeatures
	}
	return meta
}

// ============================================================================
// Prompt Building Tests
// ============================================================================

func TestTableFeatureExtraction_BuildPrompt(t *testing.T) {
	svc := &tableFeatureExtractionService{
		logger: zap.NewNop(),
	}

	// Create a table with columns that have various features
	tableID := uuid.New()
	rowCount := int64(1000)
	table := &models.SchemaTable{
		ID:        tableID,
		TableName: "users",
		RowCount:  &rowCount,
	}

	// Create columns with different feature types
	colID1 := uuid.New()
	colID2 := uuid.New()
	colID3 := uuid.New()
	colID4 := uuid.New()

	columns := []*models.SchemaColumn{
		{
			ID:           colID1,
			ColumnName:   "id",
			DataType:     "uuid",
			IsPrimaryKey: true,
		},
		{
			ID:         colID2,
			ColumnName: "email",
			DataType:   "varchar(255)",
		},
		{
			ID:         colID3,
			ColumnName: "created_at",
			DataType:   "timestamp",
		},
		{
			ID:         colID4,
			ColumnName: "status",
			DataType:   "varchar(50)",
		},
	}

	// Build metadata map
	metadataByColumnID := map[uuid.UUID]*models.ColumnMetadata{
		colID1: tfeColMeta(colID1, "identifier", "primary_key", "Unique user identifier", "uuid", nil),
		colID2: tfeColMeta(colID2, "identifier", "attribute", "User email address", "email", nil),
		colID3: tfeColMeta(colID3, "timestamp", "attribute", "When the user account was created", "audit_created", nil),
		colID4: tfeColMeta(colID4, "enum", "attribute", "Account status", "status_enum", nil),
	}

	// Create a relationship
	relationships := []*models.RelationshipDetail{
		{
			SourceTableName:  "users",
			SourceColumnName: "id",
			TargetTableName:  "orders",
			TargetColumnName: "user_id",
			Cardinality:      "1:N",
		},
	}

	tc := &tableContext{
		Table:              table,
		Columns:            columns,
		Relationships:      relationships,
		MetadataByColumnID: metadataByColumnID,
	}

	prompt := svc.buildPrompt(tc)

	// Verify the prompt contains expected sections
	if !strings.Contains(prompt, "**Table:** users") {
		t.Error("Prompt should contain table name")
	}
	if !strings.Contains(prompt, "**Row count:** 1000") {
		t.Error("Prompt should contain row count")
	}
	if !strings.Contains(prompt, "**Column count:** 4") {
		t.Error("Prompt should contain column count")
	}
	if !strings.Contains(prompt, "Primary Keys:") {
		t.Error("Prompt should have Primary Keys section")
	}
	if !strings.Contains(prompt, "Timestamps:") {
		t.Error("Prompt should have Timestamps section")
	}
	if !strings.Contains(prompt, "Relationships (Outgoing)") {
		t.Error("Prompt should have Relationships section")
	}
	if !strings.Contains(prompt, "`id` → `orders.user_id`") {
		t.Error("Prompt should contain relationship detail")
	}
	if !strings.Contains(prompt, "Response Format") {
		t.Error("Prompt should have Response Format section")
	}
}

func TestTableFeatureExtraction_BuildPrompt_NoRelationships(t *testing.T) {
	svc := &tableFeatureExtractionService{
		logger: zap.NewNop(),
	}

	table := &models.SchemaTable{
		ID:        uuid.New(),
		TableName: "settings",
	}

	colID := uuid.New()
	columns := []*models.SchemaColumn{
		{
			ID:         colID,
			ColumnName: "key",
			DataType:   "varchar(100)",
		},
	}

	metadataByColumnID := map[uuid.UUID]*models.ColumnMetadata{
		colID: tfeColMeta(colID, "identifier", "attribute", "", "", nil),
	}

	tc := &tableContext{
		Table:              table,
		Columns:            columns,
		Relationships:      nil,
		MetadataByColumnID: metadataByColumnID,
	}

	prompt := svc.buildPrompt(tc)

	// Should not contain relationships section
	if strings.Contains(prompt, "Relationships (Outgoing)") {
		t.Error("Prompt should not have Relationships section when no relationships exist")
	}
}

func TestTableFeatureExtraction_BuildPrompt_ColumnGrouping(t *testing.T) {
	svc := &tableFeatureExtractionService{
		logger: zap.NewNop(),
	}

	table := &models.SchemaTable{
		ID:        uuid.New(),
		TableName: "orders",
	}

	// Create columns with various roles
	colID1 := uuid.New()
	colID2 := uuid.New()
	colID3 := uuid.New()
	colID4 := uuid.New()

	columns := []*models.SchemaColumn{
		// Primary key
		{
			ID:         colID1,
			ColumnName: "id",
			DataType:   "uuid",
		},
		// Foreign key
		{
			ID:         colID2,
			ColumnName: "user_id",
			DataType:   "uuid",
		},
		// Measure
		{
			ID:         colID3,
			ColumnName: "total_amount",
			DataType:   "numeric(10,2)",
		},
		// Enum
		{
			ID:         colID4,
			ColumnName: "status",
			DataType:   "varchar(50)",
		},
	}

	metadataByColumnID := map[uuid.UUID]*models.ColumnMetadata{
		colID1: tfeColMeta(colID1, "identifier", "primary_key", "", "", nil),
		colID2: tfeColMeta(colID2, "identifier", "foreign_key", "", "", &models.IdentifierFeatures{FKTargetTable: "users"}),
		colID3: tfeColMeta(colID3, "measure", "measure", "", "", nil),
		colID4: tfeColMeta(colID4, "enum", "attribute", "", "", nil),
	}

	tc := &tableContext{
		Table:              table,
		Columns:            columns,
		Relationships:      nil,
		MetadataByColumnID: metadataByColumnID,
	}

	prompt := svc.buildPrompt(tc)

	// Verify all sections are present
	if !strings.Contains(prompt, "Primary Keys:") {
		t.Error("Prompt should have Primary Keys section")
	}
	if !strings.Contains(prompt, "Foreign Keys:") {
		t.Error("Prompt should have Foreign Keys section")
	}
	if !strings.Contains(prompt, "Measures:") {
		t.Error("Prompt should have Measures section")
	}
	if !strings.Contains(prompt, "Enums/Status:") {
		t.Error("Prompt should have Enums/Status section")
	}
}

// ============================================================================
// Response Parsing Tests
// ============================================================================

func TestTableFeatureExtraction_ParseResponse(t *testing.T) {
	svc := &tableFeatureExtractionService{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name            string
		tableName       string
		response        string
		wantDescription string
		wantUsageNotes  string
		wantEphemeral   bool
		wantErr         bool
	}{
		{
			name:      "valid response",
			tableName: "users",
			response: `{
				"description": "Stores user account information including authentication credentials.",
				"usage_notes": "Primary table for user data. Join with profiles for extended attributes.",
				"is_ephemeral": false
			}`,
			wantDescription: "Stores user account information including authentication credentials.",
			wantUsageNotes:  "Primary table for user data. Join with profiles for extended attributes.",
			wantEphemeral:   false,
			wantErr:         false,
		},
		{
			name:      "ephemeral table",
			tableName: "session_tokens",
			response: `{
				"description": "Temporary storage for active user session tokens.",
				"usage_notes": "Do not use for analytics. Data expires after 24 hours.",
				"is_ephemeral": true
			}`,
			wantDescription: "Temporary storage for active user session tokens.",
			wantUsageNotes:  "Do not use for analytics. Data expires after 24 hours.",
			wantEphemeral:   true,
			wantErr:         false,
		},
		{
			name:      "response with markdown code block",
			tableName: "orders",
			response: "```json\n" + `{
				"description": "Records customer orders and their line items.",
				"usage_notes": "Contains all orders. Filter by status for active orders.",
				"is_ephemeral": false
			}` + "\n```",
			wantDescription: "Records customer orders and their line items.",
			wantUsageNotes:  "Contains all orders. Filter by status for active orders.",
			wantEphemeral:   false,
			wantErr:         false,
		},
		{
			name:      "invalid JSON",
			tableName: "broken",
			response:  "this is not valid json",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaTableID := uuid.New()
			result, err := svc.parseResponse(schemaTableID, tt.tableName, tt.response)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.TableName != tt.tableName {
				t.Errorf("TableName = %q, want %q", result.TableName, tt.tableName)
			}
			if result.Description != tt.wantDescription {
				t.Errorf("Description = %q, want %q", result.Description, tt.wantDescription)
			}
			if result.UsageNotes != tt.wantUsageNotes {
				t.Errorf("UsageNotes = %q, want %q", result.UsageNotes, tt.wantUsageNotes)
			}
			if result.IsEphemeral != tt.wantEphemeral {
				t.Errorf("IsEphemeral = %v, want %v", result.IsEphemeral, tt.wantEphemeral)
			}
		})
	}
}

// ============================================================================
// Table Context Building Tests
// ============================================================================

func TestTableFeatureExtraction_BuildTableContexts(t *testing.T) {
	svc := &tableFeatureExtractionService{
		logger: zap.NewNop(),
	}

	// Create tables
	tableID1 := uuid.New()
	tableID2 := uuid.New()
	tableID3 := uuid.New()

	tables := []*models.SchemaTable{
		{ID: tableID1, TableName: "users"},
		{ID: tableID2, TableName: "orders"},
		{ID: tableID3, TableName: "settings"}, // No columns
	}

	// Create columns by table (only users and orders have columns)
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ID: uuid.New(), ColumnName: "id"},
		},
		"orders": {
			{ID: uuid.New(), ColumnName: "id"},
			{ID: uuid.New(), ColumnName: "user_id"},
		},
	}

	// Create relationships
	relationships := []*models.RelationshipDetail{
		{
			SourceTableName:  "orders",
			SourceColumnName: "user_id",
			TargetTableName:  "users",
			TargetColumnName: "id",
		},
	}

	metadataByColumnID := map[uuid.UUID]*models.ColumnMetadata{}

	contexts := svc.buildTableContexts(tables, columnsByTable, relationships, metadataByColumnID)

	// Should have 2 contexts (users and orders, not settings)
	if len(contexts) != 2 {
		t.Fatalf("Expected 2 contexts, got %d", len(contexts))
	}

	// Verify each context
	var usersCtx, ordersCtx *tableContext
	for _, ctx := range contexts {
		switch ctx.Table.TableName {
		case "users":
			usersCtx = ctx
		case "orders":
			ordersCtx = ctx
		}
	}

	if usersCtx == nil {
		t.Error("Expected users table context")
	} else {
		if len(usersCtx.Columns) != 1 {
			t.Errorf("users should have 1 column, got %d", len(usersCtx.Columns))
		}
		if len(usersCtx.Relationships) != 0 {
			t.Errorf("users should have 0 outgoing relationships, got %d", len(usersCtx.Relationships))
		}
	}

	if ordersCtx == nil {
		t.Error("Expected orders table context")
	} else {
		if len(ordersCtx.Columns) != 2 {
			t.Errorf("orders should have 2 columns, got %d", len(ordersCtx.Columns))
		}
		if len(ordersCtx.Relationships) != 1 {
			t.Errorf("orders should have 1 outgoing relationship, got %d", len(ordersCtx.Relationships))
		}
	}
}

// ============================================================================
// Column Summary Tests
// ============================================================================

func TestTableFeatureExtraction_WriteColumnSummary(t *testing.T) {
	svc := &tableFeatureExtractionService{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name       string
		column     *models.SchemaColumn
		meta       *models.ColumnMetadata
		wantSubstr []string
	}{
		{
			name: "column without features",
			column: &models.SchemaColumn{
				ColumnName: "raw_col",
				DataType:   "text",
			},
			meta:       nil,
			wantSubstr: []string{"raw_col", "text"},
		},
		{
			name: "column with features and description",
			column: &models.SchemaColumn{
				ColumnName: "email",
				DataType:   "varchar(255)",
			},
			meta:       tfeColMeta(uuid.New(), "identifier", "", "User email address", "email", nil),
			wantSubstr: []string{"email", "varchar(255)", "User email address"},
		},
		{
			name: "FK column with target",
			column: &models.SchemaColumn{
				ColumnName: "user_id",
				DataType:   "uuid",
			},
			meta:       tfeColMeta(uuid.New(), "identifier", "foreign_key", "", "", &models.IdentifierFeatures{FKTargetTable: "users"}),
			wantSubstr: []string{"user_id", "uuid", "→ users"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			svc.writeColumnSummary(&sb, tt.column, tt.meta)
			result := sb.String()

			for _, substr := range tt.wantSubstr {
				if !strings.Contains(result, substr) {
					t.Errorf("Expected summary to contain %q, got: %s", substr, result)
				}
			}
		})
	}
}

// ============================================================================
// Integration Tests (Mock-based)
// ============================================================================

// mockSchemaRepoForTableFeatures provides mock schema data for table feature extraction.
type mockSchemaRepoForTableFeatures struct {
	repositories.SchemaRepository
	tables                    []*models.SchemaTable
	columns                   []*models.SchemaColumn
	relationshipDetails       []*models.RelationshipDetail
	listTablesErr             error
	listColumnsErr            error
	getRelationshipDetailsErr error
}

func (m *mockSchemaRepoForTableFeatures) ListTablesByDatasource(_ context.Context, _, _ uuid.UUID) ([]*models.SchemaTable, error) {
	if m.listTablesErr != nil {
		return nil, m.listTablesErr
	}
	return m.tables, nil
}

func (m *mockSchemaRepoForTableFeatures) ListAllTablesByDatasource(_ context.Context, _, _ uuid.UUID) ([]*models.SchemaTable, error) {
	if m.listTablesErr != nil {
		return nil, m.listTablesErr
	}
	return m.tables, nil
}

func (m *mockSchemaRepoForTableFeatures) ListColumnsByDatasource(_ context.Context, _, _ uuid.UUID) ([]*models.SchemaColumn, error) {
	if m.listColumnsErr != nil {
		return nil, m.listColumnsErr
	}
	return m.columns, nil
}

func (m *mockSchemaRepoForTableFeatures) GetRelationshipDetails(_ context.Context, _, _ uuid.UUID) ([]*models.RelationshipDetail, error) {
	if m.getRelationshipDetailsErr != nil {
		return nil, m.getRelationshipDetailsErr
	}
	return m.relationshipDetails, nil
}

// mockColumnMetadataRepoForTableFeatures provides mock column metadata for table feature extraction.
type mockColumnMetadataRepoForTableFeatures struct {
	repositories.ColumnMetadataRepository
	metadataList []*models.ColumnMetadata
	err          error
}

func (m *mockColumnMetadataRepoForTableFeatures) GetBySchemaColumnIDs(_ context.Context, _ []uuid.UUID) ([]*models.ColumnMetadata, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.metadataList, nil
}

// mockTableMetadataRepoForTableFeatures tracks upserts for testing.
type mockTableMetadataRepoForTableFeatures struct {
	repositories.TableMetadataRepository
	upsertedMetadata []*models.TableMetadata
	upsertErr        error
}

func (m *mockTableMetadataRepoForTableFeatures) UpsertFromExtraction(_ context.Context, meta *models.TableMetadata) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upsertedMetadata = append(m.upsertedMetadata, meta)
	return nil
}

// mockLLMClientForTableFeatures provides mock LLM responses for table feature extraction.
type mockLLMClientForTableFeatures struct {
	responseContent string
	generateErr     error
	callCount       int32
}

func (m *mockLLMClientForTableFeatures) GenerateResponse(_ context.Context, _ string, _ string, _ float64, _ bool) (*llm.GenerateResponseResult, error) {
	atomic.AddInt32(&m.callCount, 1)
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	return &llm.GenerateResponseResult{
		Content:          m.responseContent,
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}, nil
}

func (m *mockLLMClientForTableFeatures) CreateEmbedding(_ context.Context, _ string, _ string) ([]float32, error) {
	return nil, nil
}

func (m *mockLLMClientForTableFeatures) CreateEmbeddings(_ context.Context, _ []string, _ string) ([][]float32, error) {
	return nil, nil
}

func (m *mockLLMClientForTableFeatures) GetModel() string {
	return "test-model"
}

func (m *mockLLMClientForTableFeatures) GetEndpoint() string {
	return "test-endpoint"
}

// mockLLMFactoryForTableFeatures provides the mock LLM client.
type mockLLMFactoryForTableFeatures struct {
	client *mockLLMClientForTableFeatures
}

func (m *mockLLMFactoryForTableFeatures) CreateForProject(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return m.client, nil
}

func (m *mockLLMFactoryForTableFeatures) CreateEmbeddingClient(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return m.client, nil
}

func (m *mockLLMFactoryForTableFeatures) CreateStreamingClient(_ context.Context, _ uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

func TestTableFeatureExtraction_ExtractTableFeatures_Success(t *testing.T) {
	// Create mock response
	response := tableAnalysisResponse{
		Description: "Stores user account information.",
		UsageNotes:  "Primary table for user data.",
		IsEphemeral: false,
	}
	responseJSON, _ := json.Marshal(response)

	// Create mock LLM
	mockLLM := &mockLLMClientForTableFeatures{
		responseContent: string(responseJSON),
	}

	// Create mock repositories
	tableID := uuid.New()
	colID := uuid.New()
	mockSchemaRepo := &mockSchemaRepoForTableFeatures{
		tables: []*models.SchemaTable{
			{ID: tableID, TableName: "users"},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            colID,
				SchemaTableID: tableID,
				ColumnName:    "id",
				DataType:      "uuid",
			},
		},
		relationshipDetails: nil,
	}

	mockColMetadataRepo := &mockColumnMetadataRepoForTableFeatures{
		metadataList: []*models.ColumnMetadata{
			tfeColMeta(colID, "identifier", "", "", "", nil),
		},
	}

	mockMetadataRepo := &mockTableMetadataRepoForTableFeatures{}

	// Create service
	workerPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 2}, zap.NewNop())
	svc := NewTableFeatureExtractionService(
		mockSchemaRepo,
		mockColMetadataRepo,
		mockMetadataRepo,
		&mockLLMFactoryForTableFeatures{client: mockLLM},
		workerPool,
		nil, // no tenant context needed for test
		zap.NewNop(),
	)

	// Execute
	projectID := uuid.New()
	datasourceID := uuid.New()
	count, err := svc.ExtractTableFeatures(context.Background(), projectID, datasourceID, nil)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 table processed, got %d", count)
	}
	if len(mockMetadataRepo.upsertedMetadata) != 1 {
		t.Fatalf("Expected 1 metadata upsert, got %d", len(mockMetadataRepo.upsertedMetadata))
	}

	meta := mockMetadataRepo.upsertedMetadata[0]
	if *meta.Description != response.Description {
		t.Errorf("Description = %q, want %q", *meta.Description, response.Description)
	}
	if *meta.UsageNotes != response.UsageNotes {
		t.Errorf("UsageNotes = %q, want %q", *meta.UsageNotes, response.UsageNotes)
	}
	if meta.IsEphemeral != response.IsEphemeral {
		t.Errorf("IsEphemeral = %v, want %v", meta.IsEphemeral, response.IsEphemeral)
	}
}

func TestTableFeatureExtraction_ExtractTableFeatures_NoTables(t *testing.T) {
	mockSchemaRepo := &mockSchemaRepoForTableFeatures{
		tables:              []*models.SchemaTable{},
		columns:             []*models.SchemaColumn{},
		relationshipDetails: nil,
	}

	mockColMetadataRepo := &mockColumnMetadataRepoForTableFeatures{}
	mockMetadataRepo := &mockTableMetadataRepoForTableFeatures{}

	workerPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 2}, zap.NewNop())
	svc := NewTableFeatureExtractionService(
		mockSchemaRepo,
		mockColMetadataRepo,
		mockMetadataRepo,
		nil, // no LLM needed
		workerPool,
		nil,
		zap.NewNop(),
	)

	count, err := svc.ExtractTableFeatures(context.Background(), uuid.New(), uuid.New(), nil)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 tables processed, got %d", count)
	}
}

func TestTableFeatureExtraction_ExtractTableFeatures_NoColumnsWithFeatures(t *testing.T) {
	mockSchemaRepo := &mockSchemaRepoForTableFeatures{
		tables: []*models.SchemaTable{
			{ID: uuid.New(), TableName: "users"},
		},
		columns:             []*models.SchemaColumn{}, // No columns
		relationshipDetails: nil,
	}

	mockColMetadataRepo := &mockColumnMetadataRepoForTableFeatures{}
	mockMetadataRepo := &mockTableMetadataRepoForTableFeatures{}

	workerPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 2}, zap.NewNop())
	svc := NewTableFeatureExtractionService(
		mockSchemaRepo,
		mockColMetadataRepo,
		mockMetadataRepo,
		nil,
		workerPool,
		nil,
		zap.NewNop(),
	)

	count, err := svc.ExtractTableFeatures(context.Background(), uuid.New(), uuid.New(), nil)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 tables processed, got %d", count)
	}
}

func TestTableFeatureExtraction_ExtractTableFeatures_ProgressCallback(t *testing.T) {
	response := tableAnalysisResponse{
		Description: "Test table description.",
		UsageNotes:  "Test usage notes.",
		IsEphemeral: false,
	}
	responseJSON, _ := json.Marshal(response)

	mockLLM := &mockLLMClientForTableFeatures{
		responseContent: string(responseJSON),
	}

	tableID1 := uuid.New()
	tableID2 := uuid.New()
	colID1 := uuid.New()
	colID2 := uuid.New()

	mockSchemaRepo := &mockSchemaRepoForTableFeatures{
		tables: []*models.SchemaTable{
			{ID: tableID1, TableName: "users"},
			{ID: tableID2, TableName: "orders"},
		},
		columns: []*models.SchemaColumn{
			{ID: colID1, SchemaTableID: tableID1, ColumnName: "id"},
			{ID: colID2, SchemaTableID: tableID2, ColumnName: "id"},
		},
		relationshipDetails: nil,
	}

	mockColMetadataRepo := &mockColumnMetadataRepoForTableFeatures{
		metadataList: []*models.ColumnMetadata{
			{SchemaColumnID: colID1},
			{SchemaColumnID: colID2},
		},
	}

	mockMetadataRepo := &mockTableMetadataRepoForTableFeatures{}

	workerPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 2}, zap.NewNop())
	svc := NewTableFeatureExtractionService(
		mockSchemaRepo,
		mockColMetadataRepo,
		mockMetadataRepo,
		&mockLLMFactoryForTableFeatures{client: mockLLM},
		workerPool,
		nil,
		zap.NewNop(),
	)

	// Track progress calls
	var progressCalls []struct {
		current int
		total   int
		message string
	}

	progressCallback := func(current, total int, message string) {
		progressCalls = append(progressCalls, struct {
			current int
			total   int
			message string
		}{current, total, message})
	}

	count, err := svc.ExtractTableFeatures(context.Background(), uuid.New(), uuid.New(), progressCallback)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 tables processed, got %d", count)
	}

	// Should have progress updates
	if len(progressCalls) < 2 {
		t.Errorf("Expected at least 2 progress calls, got %d", len(progressCalls))
	}
}
