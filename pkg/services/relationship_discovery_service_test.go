//go:build ignore
// +build ignore

// TODO: This test has constructor signature mismatches for NewLLMRelationshipDiscoveryService
// and SchemaColumn.Metadata references. Needs refactoring to use ColumnMetadataRepository.

package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// TestNewLLMRelationshipDiscoveryService tests service creation
func TestNewLLMRelationshipDiscoveryService(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, // candidateCollector - tested in relationship_candidate_collector_test.go
		nil, // validator - tested in relationship_validator_test.go
		nil, // datasourceService
		nil, // adapterFactory
		nil, // schemaRepo
		logger,
	)

	require.NotNil(t, svc, "NewLLMRelationshipDiscoveryService should return non-nil service")
}

// TestLLMRelationshipDiscoveryResult verifies the result structure
func TestLLMRelationshipDiscoveryResult(t *testing.T) {
	result := &LLMRelationshipDiscoveryResult{
		CandidatesEvaluated:   100,
		RelationshipsCreated:  15,
		RelationshipsRejected: 85,
		PreservedDBFKs:        10,
		PreservedColumnFKs:    5,
		DurationMs:            1500,
	}

	assert.Equal(t, 100, result.CandidatesEvaluated)
	assert.Equal(t, 15, result.RelationshipsCreated)
	assert.Equal(t, 85, result.RelationshipsRejected)
	assert.Equal(t, 10, result.PreservedDBFKs)
	assert.Equal(t, 5, result.PreservedColumnFKs)
	assert.Equal(t, int64(1500), result.DurationMs)
}

// TestBuildExistingSchemaRelationshipSet verifies deduplication set creation for schema relationships
func TestBuildExistingSchemaRelationshipSet(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	// Create test tables and columns
	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	productsTableID := uuid.New()

	userIDColID := uuid.New()
	orderUserIDColID := uuid.New()
	productIDColID := uuid.New()
	orderProductIDColID := uuid.New()

	tableByID := map[uuid.UUID]*models.SchemaTable{
		usersTableID:    {ID: usersTableID, TableName: "users"},
		ordersTableID:   {ID: ordersTableID, TableName: "orders"},
		productsTableID: {ID: productsTableID, TableName: "products"},
	}

	columnByID := map[uuid.UUID]*models.SchemaColumn{
		userIDColID:         {ID: userIDColID, ColumnName: "id"},
		orderUserIDColID:    {ID: orderUserIDColID, ColumnName: "user_id"},
		productIDColID:      {ID: productIDColID, ColumnName: "id"},
		orderProductIDColID: {ID: orderProductIDColID, ColumnName: "product_id"},
	}

	relationships := []*models.SchemaRelationship{
		{
			SourceTableID:  ordersTableID,
			SourceColumnID: orderUserIDColID,
			TargetTableID:  usersTableID,
			TargetColumnID: userIDColID,
		},
		{
			SourceTableID:  ordersTableID,
			SourceColumnID: orderProductIDColID,
			TargetTableID:  productsTableID,
			TargetColumnID: productIDColID,
		},
	}

	relSet := svc.buildExistingSchemaRelationshipSet(relationships, tableByID, columnByID)

	require.Len(t, relSet, 2)
	assert.True(t, relSet["orders.user_id->users.id"])
	assert.True(t, relSet["orders.product_id->products.id"])
	assert.False(t, relSet["orders.status_id->statuses.id"], "non-existent relationship should not be in set")
}

// TestBuildExistingSchemaRelationshipSet_EmptyInput tests handling of empty relationship list
func TestBuildExistingSchemaRelationshipSet_EmptyInput(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	relSet := svc.buildExistingSchemaRelationshipSet(
		[]*models.SchemaRelationship{},
		map[uuid.UUID]*models.SchemaTable{},
		map[uuid.UUID]*models.SchemaColumn{},
	)

	require.Empty(t, relSet, "empty input should produce empty set")
}

// TestBuildExistingSchemaRelationshipSet_NilInput tests handling of nil relationship list
func TestBuildExistingSchemaRelationshipSet_NilInput(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	relSet := svc.buildExistingSchemaRelationshipSet(nil, nil, nil)

	require.Empty(t, relSet, "nil input should produce empty set")
}

// ============================================================================
// Integration Tests for LLMRelationshipDiscoveryService (Task 7.3.2)
// ============================================================================

// mockRelDiscoveryLLMClient is a mock LLM client that tracks calls and returns configurable responses
type mockRelDiscoveryLLMClient struct {
	mu        sync.Mutex
	calls     []string // Track what candidates were sent (keyed by source.col->target.col)
	responses map[string]*RelationshipValidationResult
}

func (m *mockRelDiscoveryLLMClient) GenerateResponse(_ context.Context, prompt string, _ string, _ float64, _ bool) (*llm.GenerateResponseResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Extract candidate key from prompt for tracking and response lookup
	candidateKey := extractCandidateKeyFromPrompt(prompt)
	m.calls = append(m.calls, candidateKey)

	// Look up the response for this candidate
	if response, ok := m.responses[candidateKey]; ok {
		return &llm.GenerateResponseResult{
			Content: formatValidationResponse(response),
		}, nil
	}

	// Default response: reject unknown candidates
	return &llm.GenerateResponseResult{
		Content: `{"is_valid_fk": false, "confidence": 0.5, "cardinality": "", "reasoning": "Unknown candidate", "source_role": ""}`,
	}, nil
}

func (m *mockRelDiscoveryLLMClient) CreateEmbedding(_ context.Context, _ string, _ string) ([]float32, error) {
	return nil, nil
}

func (m *mockRelDiscoveryLLMClient) CreateEmbeddings(_ context.Context, _ []string, _ string) ([][]float32, error) {
	return nil, nil
}

func (m *mockRelDiscoveryLLMClient) GetModel() string {
	return "test-model"
}

func (m *mockRelDiscoveryLLMClient) GetEndpoint() string {
	return "https://test.endpoint"
}

var _ llm.LLMClient = (*mockRelDiscoveryLLMClient)(nil)

// extractCandidateKeyFromPrompt extracts a "source_table.source_column->target_table.target_column" key from a prompt
func extractCandidateKeyFromPrompt(prompt string) string {
	// Look for patterns like "Is **orders.user_id** a foreign key referencing **users.id**?"
	// This is a simplified extraction - just look for the key elements

	// Find source table.column between first "**" pair
	sourceStart := strings.Index(prompt, "Is **")
	if sourceStart == -1 {
		return "unknown->unknown"
	}
	sourceStart += 5 // Skip "Is **"
	sourceEnd := strings.Index(prompt[sourceStart:], "**")
	if sourceEnd == -1 {
		return "unknown->unknown"
	}
	source := prompt[sourceStart : sourceStart+sourceEnd]

	// Find target table.column between "referencing **" and next "**"
	targetPrefix := "referencing **"
	targetPrefixIdx := strings.Index(prompt, targetPrefix)
	if targetPrefixIdx == -1 {
		return source + "->unknown"
	}
	targetStart := targetPrefixIdx + len(targetPrefix)
	targetEnd := strings.Index(prompt[targetStart:], "**")
	if targetEnd == -1 {
		return source + "->unknown"
	}
	target := prompt[targetStart : targetStart+targetEnd]

	return source + "->" + target
}

// formatValidationResponse converts a RelationshipValidationResult to JSON string
func formatValidationResponse(result *RelationshipValidationResult) string {
	return fmt.Sprintf(`{
		"is_valid_fk": %v,
		"confidence": %f,
		"cardinality": "%s",
		"reasoning": "%s",
		"source_role": "%s"
	}`, result.IsValidFK, result.Confidence, result.Cardinality, result.Reasoning, result.SourceRole)
}

// mockRelDiscoveryLLMClientFactory creates mockRelDiscoveryLLMClient instances
type mockRelDiscoveryLLMClientFactory struct {
	client *mockRelDiscoveryLLMClient
}

func (m *mockRelDiscoveryLLMClientFactory) CreateForProject(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return m.client, nil
}

func (m *mockRelDiscoveryLLMClientFactory) CreateEmbeddingClient(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return m.client, nil
}

func (m *mockRelDiscoveryLLMClientFactory) CreateStreamingClient(_ context.Context, _ uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

var _ llm.LLMClientFactory = (*mockRelDiscoveryLLMClientFactory)(nil)

// mockRelDiscoveryCandidateCollector is a mock that returns specified candidates
type mockRelDiscoveryCandidateCollector struct {
	candidates []*RelationshipCandidate
	err        error
}

func (m *mockRelDiscoveryCandidateCollector) CollectCandidates(_ context.Context, _, _ uuid.UUID, _ dag.ProgressCallback) ([]*RelationshipCandidate, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.candidates, nil
}

var _ RelationshipCandidateCollector = (*mockRelDiscoveryCandidateCollector)(nil)

// mockRelDiscoveryValidator is a mock that delegates to the LLM client but tracks calls
type mockRelDiscoveryValidator struct {
	llmClient *mockRelDiscoveryLLMClient
	logger    *zap.Logger
}

func (m *mockRelDiscoveryValidator) ValidateCandidate(ctx context.Context, projectID uuid.UUID, candidate *RelationshipCandidate) (*RelationshipValidationResult, error) {
	// Build the candidate key
	key := fmt.Sprintf("%s.%s->%s.%s", candidate.SourceTable, candidate.SourceColumn, candidate.TargetTable, candidate.TargetColumn)

	m.llmClient.mu.Lock()
	defer m.llmClient.mu.Unlock()

	// Track the call
	m.llmClient.calls = append(m.llmClient.calls, key)

	// Return configured response
	if response, ok := m.llmClient.responses[key]; ok {
		return response, nil
	}

	// Default: reject
	return &RelationshipValidationResult{
		IsValidFK:   false,
		Confidence:  0.5,
		Reasoning:   "Unknown candidate",
		Cardinality: "",
	}, nil
}

func (m *mockRelDiscoveryValidator) ValidateCandidates(ctx context.Context, projectID uuid.UUID, candidates []*RelationshipCandidate, progressCallback dag.ProgressCallback) ([]*ValidatedRelationship, error) {
	var results []*ValidatedRelationship

	for i, candidate := range candidates {
		result, err := m.ValidateCandidate(ctx, projectID, candidate)
		if err != nil {
			continue
		}

		results = append(results, &ValidatedRelationship{
			Candidate: candidate,
			Result:    result,
			Validated: true,
		})

		if progressCallback != nil {
			progressCallback(i+1, len(candidates), "Validating...")
		}
	}

	return results, nil
}

var _ RelationshipValidator = (*mockRelDiscoveryValidator)(nil)

// TestRelationshipDiscoveryService_ColumnFeaturesFKPreserved tests that columns with
// ColumnFeatures containing fk_target_table are preserved without LLM validation.
func TestRelationshipDiscoveryService_ColumnFeaturesFKPreserved(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	ordersTableID := uuid.New()
	usersTableID := uuid.New()
	sourceEntityID := uuid.New()
	targetEntityID := uuid.New()

	// Create a mock LLM client that tracks calls
	mockLLMClient := &mockRelDiscoveryLLMClient{
		calls:     []string{},
		responses: map[string]*RelationshipValidationResult{},
	}

	// Create column with ColumnFeatures containing FK target info (high confidence)
	paymentUserIDCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "user_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
		Metadata: map[string]any{
			"column_features": map[string]any{
				"classification_path": "uuid",
				"role":                "foreign_key",
				"purpose":             "identifier",
				"identifier_features": map[string]any{
					"fk_target_table":  "users",
					"fk_target_column": "id",
					"fk_confidence":    0.95, // High confidence - should be preserved
				},
			},
		},
	}

	// Target column (PK)
	usersPKCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "id",
		DataType:      "uuid",
		IsPrimaryKey:  true,
	}

	// Mock repos and services
	_ = &mockOntologyRepoForRelDiscovery{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}

	_ = &mockEntityRepoForRelDiscovery{
		entities: []*models.OntologyEntity{
			{ID: sourceEntityID, Name: "Order", PrimaryTable: "orders"},
			{ID: targetEntityID, Name: "User", PrimaryTable: "users"},
		},
	}

	_ = &mockRelationshipRepoForRelDiscovery{
		relationships: []*models.EntityRelationship{},
		createdRels:   []*models.EntityRelationship{},
	}

	mockSchemaRepo := &mockSchemaRepoForRelDiscovery{
		tables: []*models.SchemaTable{
			{ID: ordersTableID, SchemaName: "public", TableName: "orders"},
			{ID: usersTableID, SchemaName: "public", TableName: "users"},
		},
		columns:       []*models.SchemaColumn{paymentUserIDCol, usersPKCol},
		relationships: []*models.SchemaRelationship{}, // No DB-declared FKs
	}

	mockDatasourceSvc := &mockDatasourceServiceForRelDiscovery{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			Name:           "test-db",
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	mockAdapterFactory := &mockAdapterFactoryForRelDiscovery{
		schemaDiscoverer: &mockSchemaDiscovererForRelDiscovery{
			joinAnalysis: &datasource.JoinAnalysis{
				JoinCount:          100,
				SourceMatched:      95,
				TargetMatched:      90,
				OrphanCount:        5,
				ReverseOrphanCount: 10,
			},
		},
	}

	// The candidate collector should NOT return this as a candidate since it's already
	// handled by preserveColumnFeaturesFKs. But we'll use a collector that returns empty
	// to verify no LLM calls are made for the pre-resolved FK.
	mockCollector := &mockRelDiscoveryCandidateCollector{
		candidates: []*RelationshipCandidate{}, // No candidates - the FK is pre-resolved
	}

	mockValidator := &mockRelDiscoveryValidator{
		llmClient: mockLLMClient,
		logger:    logger,
	}

	svc := NewLLMRelationshipDiscoveryService(
		mockCollector,
		mockValidator,
		mockDatasourceSvc,
		mockAdapterFactory,
		mockSchemaRepo,
		logger,
	)

	// Execute
	result, err := svc.DiscoverRelationships(context.Background(), projectID, datasourceID, nil)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// Key assertion: No LLM calls should have been made for the ColumnFeatures FK
	mockLLMClient.mu.Lock()
	llmCalls := mockLLMClient.calls
	mockLLMClient.mu.Unlock()

	assert.Empty(t, llmCalls, "no LLM calls should be made for ColumnFeatures FK with high confidence")

	// Verify the relationship was created from ColumnFeatures
	assert.Equal(t, 1, result.PreservedColumnFKs, "should have 1 preserved ColumnFeatures FK")
	assert.Equal(t, 0, result.CandidatesEvaluated, "no candidates should be evaluated (FK was pre-resolved)")
}

// TestRelationshipDiscoveryService_UUIDTextToUUIDPK_LLMValidates tests that text columns
// containing UUIDs that reference UUID PKs are validated by LLM when no FK is declared.
func TestRelationshipDiscoveryService_UUIDTextToUUIDPK_LLMValidates(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	ordersTableID := uuid.New()
	usersTableID := uuid.New()
	sourceEntityID := uuid.New()
	targetEntityID := uuid.New()

	// Create a mock LLM client that returns valid FK for orders.user_id -> users.id
	mockLLMClient := &mockRelDiscoveryLLMClient{
		calls: []string{},
		responses: map[string]*RelationshipValidationResult{
			"orders.user_id->users.id": {
				IsValidFK:   true,
				Confidence:  0.92,
				Cardinality: "N:1",
				Reasoning:   "Column user_id contains UUIDs that match users.id values",
				SourceRole:  "owner",
			},
		},
	}

	// Mock repos and services
	_ = &mockOntologyRepoForRelDiscovery{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}

	_ = &mockEntityRepoForRelDiscovery{
		entities: []*models.OntologyEntity{
			{ID: sourceEntityID, Name: "Order", PrimaryTable: "orders"},
			{ID: targetEntityID, Name: "User", PrimaryTable: "users"},
		},
	}

	_ = &mockRelationshipRepoForRelDiscovery{
		relationships: []*models.EntityRelationship{},
		createdRels:   []*models.EntityRelationship{},
	}

	mockSchemaRepo := &mockSchemaRepoForRelDiscovery{
		tables: []*models.SchemaTable{
			{ID: ordersTableID, SchemaName: "public", TableName: "orders"},
			{ID: usersTableID, SchemaName: "public", TableName: "users"},
		},
		columns:       []*models.SchemaColumn{},
		relationships: []*models.SchemaRelationship{}, // No DB-declared FKs
	}

	mockDatasourceSvc := &mockDatasourceServiceForRelDiscovery{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			Name:           "test-db",
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	mockAdapterFactory := &mockAdapterFactoryForRelDiscovery{
		schemaDiscoverer: &mockSchemaDiscovererForRelDiscovery{
			joinAnalysis: &datasource.JoinAnalysis{
				JoinCount:          100,
				SourceMatched:      95,
				TargetMatched:      90,
				OrphanCount:        5,
				ReverseOrphanCount: 10,
			},
		},
	}

	// Create a candidate that needs LLM validation
	userIDColID := uuid.New()
	usersIDColID := uuid.New()

	mockCollector := &mockRelDiscoveryCandidateCollector{
		candidates: []*RelationshipCandidate{
			{
				SourceTable:         "orders",
				SourceColumn:        "user_id",
				SourceDataType:      "text", // Text type but contains UUIDs
				SourceIsPK:          false,
				SourceDistinctCount: 100,
				SourceNullRate:      0.0,
				SourceSamples:       []string{"550e8400-e29b-41d4-a716-446655440000", "550e8400-e29b-41d4-a716-446655440001"},
				SourceColumnID:      userIDColID,
				SourcePurpose:       models.PurposeIdentifier,
				SourceRole:          models.RoleForeignKey,

				TargetTable:         "users",
				TargetColumn:        "id",
				TargetDataType:      "text", // Also text but PK
				TargetIsPK:          true,
				TargetDistinctCount: 500,
				TargetNullRate:      0.0,
				TargetSamples:       []string{"550e8400-e29b-41d4-a716-446655440000", "550e8400-e29b-41d4-a716-446655440001"},
				TargetColumnID:      usersIDColID,

				JoinCount:      100,
				SourceMatched:  95,
				TargetMatched:  90,
				OrphanCount:    5,
				ReverseOrphans: 10,
			},
		},
	}

	mockValidator := &mockRelDiscoveryValidator{
		llmClient: mockLLMClient,
		logger:    logger,
	}

	svc := NewLLMRelationshipDiscoveryService(
		mockCollector,
		mockValidator,
		mockDatasourceSvc,
		mockAdapterFactory,
		mockSchemaRepo,
		logger,
	)

	// Execute
	result, err := svc.DiscoverRelationships(context.Background(), projectID, datasourceID, nil)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// Key assertion: LLM should have been called for orders.user_id -> users.id
	mockLLMClient.mu.Lock()
	llmCalls := mockLLMClient.calls
	mockLLMClient.mu.Unlock()

	assert.Len(t, llmCalls, 1, "LLM should be called once for the UUID text candidate")
	assert.Equal(t, "orders.user_id->users.id", llmCalls[0], "LLM should be called for orders.user_id -> users.id")

	// Verify the relationship was created with LLM-provided cardinality
	assert.Equal(t, 1, result.CandidatesEvaluated, "should evaluate 1 candidate")
	assert.Equal(t, 1, result.RelationshipsCreated, "should create 1 relationship (LLM accepted)")
	assert.Equal(t, 0, result.RelationshipsRejected, "should reject 0 relationships")

	// Verify the relationship was stored with correct cardinality
	assert.Len(t, mockSchemaRepo.createdRels, 1, "should create 1 relationship in repo")
	if len(mockSchemaRepo.createdRels) > 0 {
		createdRel := mockSchemaRepo.createdRels[0]
		assert.Equal(t, "N:1", createdRel.Cardinality, "relationship should have N:1 cardinality from LLM")
	}
}

// TestRelationshipDiscoveryService_IDToTimestamp_LLMRejects tests that nonsensical FK
// candidates (like id -> timestamp) are rejected by LLM validation.
func TestRelationshipDiscoveryService_IDToTimestamp_LLMRejects(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	eventsTableID := uuid.New()
	logsTableID := uuid.New()
	sourceEntityID := uuid.New()
	targetEntityID := uuid.New()

	// Create a mock LLM client that rejects the nonsensical FK candidate
	mockLLMClient := &mockRelDiscoveryLLMClient{
		calls: []string{},
		responses: map[string]*RelationshipValidationResult{
			"events.id->logs.created_at": {
				IsValidFK:   false,
				Confidence:  0.95,
				Cardinality: "",
				Reasoning:   "created_at is a timestamp column, not an identifier. PKs should not reference timestamps.",
				SourceRole:  "",
			},
		},
	}

	// Mock repos and services
	_ = &mockOntologyRepoForRelDiscovery{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}

	_ = &mockEntityRepoForRelDiscovery{
		entities: []*models.OntologyEntity{
			{ID: sourceEntityID, Name: "Event", PrimaryTable: "events"},
			{ID: targetEntityID, Name: "Log", PrimaryTable: "logs"},
		},
	}

	_ = &mockRelationshipRepoForRelDiscovery{
		relationships: []*models.EntityRelationship{},
		createdRels:   []*models.EntityRelationship{},
	}

	mockSchemaRepo := &mockSchemaRepoForRelDiscovery{
		tables: []*models.SchemaTable{
			{ID: eventsTableID, SchemaName: "public", TableName: "events"},
			{ID: logsTableID, SchemaName: "public", TableName: "logs"},
		},
		columns:       []*models.SchemaColumn{},
		relationships: []*models.SchemaRelationship{}, // No DB-declared FKs
	}

	mockDatasourceSvc := &mockDatasourceServiceForRelDiscovery{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			Name:           "test-db",
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	mockAdapterFactory := &mockAdapterFactoryForRelDiscovery{
		schemaDiscoverer: &mockSchemaDiscovererForRelDiscovery{
			joinAnalysis: &datasource.JoinAnalysis{
				JoinCount:          0, // No matches - nonsensical join
				SourceMatched:      0,
				TargetMatched:      0,
				OrphanCount:        1000,
				ReverseOrphanCount: 500,
			},
		},
	}

	// Create a nonsensical candidate: events.id -> logs.created_at
	// In real usage, this would be filtered out by type compatibility check,
	// but we test that even if it gets through, LLM rejects it
	eventsIDColID := uuid.New()
	logsCreatedAtColID := uuid.New()

	mockCollector := &mockRelDiscoveryCandidateCollector{
		candidates: []*RelationshipCandidate{
			{
				SourceTable:         "events",
				SourceColumn:        "id",
				SourceDataType:      "integer",
				SourceIsPK:          true, // This is a PK pointing to timestamp (nonsensical)
				SourceDistinctCount: 1000,
				SourceNullRate:      0.0,
				SourceSamples:       []string{"1", "2", "3", "4", "5"},
				SourceColumnID:      eventsIDColID,

				TargetTable:         "logs",
				TargetColumn:        "created_at",
				TargetDataType:      "timestamp", // This should not be a FK target
				TargetIsPK:          false,
				TargetDistinctCount: 500,
				TargetNullRate:      0.0,
				TargetSamples:       []string{"2024-01-01 00:00:00", "2024-01-02 00:00:00"},
				TargetColumnID:      logsCreatedAtColID,

				JoinCount:      0,
				SourceMatched:  0,
				TargetMatched:  0,
				OrphanCount:    1000,
				ReverseOrphans: 500,
			},
		},
	}

	mockValidator := &mockRelDiscoveryValidator{
		llmClient: mockLLMClient,
		logger:    logger,
	}

	svc := NewLLMRelationshipDiscoveryService(
		mockCollector,
		mockValidator,
		mockDatasourceSvc,
		mockAdapterFactory,
		mockSchemaRepo,
		logger,
	)

	// Execute
	result, err := svc.DiscoverRelationships(context.Background(), projectID, datasourceID, nil)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// Key assertion: LLM should have been called and rejected the candidate
	mockLLMClient.mu.Lock()
	llmCalls := mockLLMClient.calls
	mockLLMClient.mu.Unlock()

	assert.Len(t, llmCalls, 1, "LLM should be called once for the id->timestamp candidate")
	assert.Equal(t, "events.id->logs.created_at", llmCalls[0], "LLM should be called for events.id -> logs.created_at")

	// Verify the relationship was rejected
	assert.Equal(t, 1, result.CandidatesEvaluated, "should evaluate 1 candidate")
	assert.Equal(t, 0, result.RelationshipsCreated, "should create 0 relationships (LLM rejected)")
	assert.Equal(t, 1, result.RelationshipsRejected, "should reject 1 relationship")

	// Verify no relationship was stored
	assert.Len(t, mockSchemaRepo.createdRels, 0, "should not create any relationships in repo")
}

// TestRelationshipDiscoveryService_VerifyLLMCallsSlice tests that we can correctly
// verify which candidates were sent to LLM for validation.
func TestRelationshipDiscoveryService_VerifyLLMCallsSlice(t *testing.T) {
	logger := zap.NewNop()
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	ordersTableID := uuid.New()
	usersTableID := uuid.New()
	productsTableID := uuid.New()
	sourceEntityID := uuid.New()
	targetEntityID := uuid.New()
	productEntityID := uuid.New()

	// Create a mock LLM client with multiple responses
	mockLLMClient := &mockRelDiscoveryLLMClient{
		calls: []string{},
		responses: map[string]*RelationshipValidationResult{
			"orders.user_id->users.id": {
				IsValidFK:   true,
				Confidence:  0.95,
				Cardinality: "N:1",
				Reasoning:   "Valid FK relationship",
			},
			"orders.product_id->products.id": {
				IsValidFK:   true,
				Confidence:  0.90,
				Cardinality: "N:1",
				Reasoning:   "Valid FK relationship",
			},
			"orders.status->users.id": {
				IsValidFK:   false,
				Confidence:  0.85,
				Cardinality: "",
				Reasoning:   "status is not a reference to users",
			},
		},
	}

	// Mock repos and services
	_ = &mockOntologyRepoForRelDiscovery{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}

	_ = &mockEntityRepoForRelDiscovery{
		entities: []*models.OntologyEntity{
			{ID: sourceEntityID, Name: "Order", PrimaryTable: "orders"},
			{ID: targetEntityID, Name: "User", PrimaryTable: "users"},
			{ID: productEntityID, Name: "Product", PrimaryTable: "products"},
		},
	}

	_ = &mockRelationshipRepoForRelDiscovery{
		relationships: []*models.EntityRelationship{},
		createdRels:   []*models.EntityRelationship{},
	}

	mockSchemaRepo := &mockSchemaRepoForRelDiscovery{
		tables: []*models.SchemaTable{
			{ID: ordersTableID, SchemaName: "public", TableName: "orders"},
			{ID: usersTableID, SchemaName: "public", TableName: "users"},
			{ID: productsTableID, SchemaName: "public", TableName: "products"},
		},
		columns:       []*models.SchemaColumn{},
		relationships: []*models.SchemaRelationship{},
	}

	mockDatasourceSvc := &mockDatasourceServiceForRelDiscovery{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			Name:           "test-db",
			DatasourceType: "postgres",
			Config:         map[string]any{},
		},
	}

	mockAdapterFactory := &mockAdapterFactoryForRelDiscovery{
		schemaDiscoverer: &mockSchemaDiscovererForRelDiscovery{
			joinAnalysis: &datasource.JoinAnalysis{
				JoinCount:     100,
				SourceMatched: 95,
			},
		},
	}

	// Multiple candidates to verify LLM calls tracking
	mockCollector := &mockRelDiscoveryCandidateCollector{
		candidates: []*RelationshipCandidate{
			{
				SourceTable:    "orders",
				SourceColumn:   "user_id",
				SourceDataType: "uuid",
				SourceColumnID: uuid.New(),
				TargetTable:    "users",
				TargetColumn:   "id",
				TargetDataType: "uuid",
				TargetIsPK:     true,
				TargetColumnID: uuid.New(),
			},
			{
				SourceTable:    "orders",
				SourceColumn:   "product_id",
				SourceDataType: "uuid",
				SourceColumnID: uuid.New(),
				TargetTable:    "products",
				TargetColumn:   "id",
				TargetDataType: "uuid",
				TargetIsPK:     true,
				TargetColumnID: uuid.New(),
			},
			{
				SourceTable:    "orders",
				SourceColumn:   "status",
				SourceDataType: "text",
				SourceColumnID: uuid.New(),
				TargetTable:    "users",
				TargetColumn:   "id",
				TargetDataType: "uuid",
				TargetIsPK:     true,
				TargetColumnID: uuid.New(),
			},
		},
	}

	mockValidator := &mockRelDiscoveryValidator{
		llmClient: mockLLMClient,
		logger:    logger,
	}

	svc := NewLLMRelationshipDiscoveryService(
		mockCollector,
		mockValidator,
		mockDatasourceSvc,
		mockAdapterFactory,
		mockSchemaRepo,
		logger,
	)

	// Execute
	result, err := svc.DiscoverRelationships(context.Background(), projectID, datasourceID, nil)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)

	// Key assertion: Verify all expected candidates were sent to LLM
	mockLLMClient.mu.Lock()
	llmCalls := mockLLMClient.calls
	mockLLMClient.mu.Unlock()

	assert.Len(t, llmCalls, 3, "LLM should be called for all 3 candidates")

	// Verify specific candidates were sent
	callsSet := make(map[string]bool)
	for _, call := range llmCalls {
		callsSet[call] = true
	}

	assert.True(t, callsSet["orders.user_id->users.id"], "orders.user_id->users.id should be sent to LLM")
	assert.True(t, callsSet["orders.product_id->products.id"], "orders.product_id->products.id should be sent to LLM")
	assert.True(t, callsSet["orders.status->users.id"], "orders.status->users.id should be sent to LLM")

	// Verify results
	assert.Equal(t, 3, result.CandidatesEvaluated, "should evaluate 3 candidates")
	assert.Equal(t, 2, result.RelationshipsCreated, "should create 2 relationships (LLM accepted)")
	assert.Equal(t, 1, result.RelationshipsRejected, "should reject 1 relationship (status->users.id)")
}

// ============================================================================
// Mock implementations for integration tests
// ============================================================================

type mockOntologyRepoForRelDiscovery struct {
	repositories.OntologyRepository
	activeOntology *models.TieredOntology
}

func (m *mockOntologyRepoForRelDiscovery) GetActive(_ context.Context, _ uuid.UUID) (*models.TieredOntology, error) {
	return m.activeOntology, nil
}

type mockEntityRepoForRelDiscovery struct {
	repositories.OntologyEntityRepository
	entities []*models.OntologyEntity
}

func (m *mockEntityRepoForRelDiscovery) GetByOntology(_ context.Context, _ uuid.UUID) ([]*models.OntologyEntity, error) {
	return m.entities, nil
}

type mockRelationshipRepoForRelDiscovery struct {
	repositories.EntityRelationshipRepository
	relationships []*models.EntityRelationship
	createdRels   []*models.EntityRelationship
}

func (m *mockRelationshipRepoForRelDiscovery) GetByOntology(_ context.Context, _ uuid.UUID) ([]*models.EntityRelationship, error) {
	return m.relationships, nil
}

func (m *mockRelationshipRepoForRelDiscovery) Create(_ context.Context, rel *models.EntityRelationship) error {
	m.createdRels = append(m.createdRels, rel)
	return nil
}

type mockSchemaRepoForRelDiscovery struct {
	repositories.SchemaRepository
	tables        []*models.SchemaTable
	columns       []*models.SchemaColumn
	relationships []*models.SchemaRelationship
	createdRels   []*models.SchemaRelationship // Track relationships created via UpsertRelationshipWithMetrics
}

func (m *mockSchemaRepoForRelDiscovery) ListTablesByDatasource(_ context.Context, _, _ uuid.UUID, _ bool) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockSchemaRepoForRelDiscovery) ListColumnsByDatasource(_ context.Context, _, _ uuid.UUID) ([]*models.SchemaColumn, error) {
	return m.columns, nil
}

func (m *mockSchemaRepoForRelDiscovery) ListRelationshipsByDatasource(_ context.Context, _, _ uuid.UUID) ([]*models.SchemaRelationship, error) {
	return m.relationships, nil
}

func (m *mockSchemaRepoForRelDiscovery) GetRelationshipsByMethod(_ context.Context, _, _ uuid.UUID, _ string) ([]*models.SchemaRelationship, error) {
	// Return DB FKs from relationships slice (filter by method='fk')
	var result []*models.SchemaRelationship
	for _, rel := range m.relationships {
		if rel.InferenceMethod != nil && *rel.InferenceMethod == models.InferenceMethodForeignKey {
			result = append(result, rel)
		}
	}
	return result, nil
}

func (m *mockSchemaRepoForRelDiscovery) UpsertRelationshipWithMetrics(_ context.Context, rel *models.SchemaRelationship, _ *models.DiscoveryMetrics) error {
	m.createdRels = append(m.createdRels, rel)
	return nil
}

type mockDatasourceServiceForRelDiscovery struct {
	DatasourceService
	datasource *models.Datasource
}

func (m *mockDatasourceServiceForRelDiscovery) Get(_ context.Context, _, _ uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

type mockAdapterFactoryForRelDiscovery struct {
	datasource.DatasourceAdapterFactory
	schemaDiscoverer datasource.SchemaDiscoverer
}

func (m *mockAdapterFactoryForRelDiscovery) NewSchemaDiscoverer(_ context.Context, _ string, _ map[string]any, _, _ uuid.UUID, _ string) (datasource.SchemaDiscoverer, error) {
	return m.schemaDiscoverer, nil
}

type mockSchemaDiscovererForRelDiscovery struct {
	datasource.SchemaDiscoverer
	joinAnalysis *datasource.JoinAnalysis
}

func (m *mockSchemaDiscovererForRelDiscovery) AnalyzeJoin(_ context.Context, _, _, _, _, _, _ string) (*datasource.JoinAnalysis, error) {
	return m.joinAnalysis, nil
}

func (m *mockSchemaDiscovererForRelDiscovery) Close() error {
	return nil
}
