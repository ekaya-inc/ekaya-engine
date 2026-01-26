package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestFKSemanticEvaluationService_EvaluateCandidates_ValidFK(t *testing.T) {
	projectID := uuid.New()

	// Create a candidate that looks like a valid FK
	candidates := []FKCandidate{
		{
			SourceSchema:   "public",
			SourceTable:    "orders",
			SourceColumn:   "user_id",
			SourceDataType: "uuid",
			SourceDistinct: ptr(int64(500)),
			SourceRowCount: ptr(int64(10000)),

			TargetSchema:   "public",
			TargetTable:    "users",
			TargetColumn:   "id",
			TargetDataType: "uuid",
			TargetDistinct: ptr(int64(500)),

			JoinCount:     10000,
			SourceMatched: 10000,
			TargetMatched: 500,
			OrphanCount:   0,

			TargetEntityName:        "User",
			TargetEntityDescription: "A platform user",
		},
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		// Verify prompt contains expected context
		assert.Contains(t, prompt, "orders.user_id")
		assert.Contains(t, prompt, "users.id")
		assert.Contains(t, prompt, "0% orphans")
		assert.Contains(t, prompt, "User")

		return &llm.GenerateResponseResult{
			Content: `{"evaluations": [{"id": 1, "is_fk": true, "confidence": 0.95, "semantic_role": "owner", "reasoning": "Clear FK relationship with 0% orphans and matching entity reference.", "should_include": true}]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())

	service := NewFKSemanticEvaluationService(nil, nil, mockFactory, testPool, circuitBreaker, zap.NewNop())

	// Execute
	results, err := service.EvaluateCandidates(context.Background(), projectID, candidates)

	// Assert
	require.NoError(t, err)
	require.Len(t, results, 1)

	result := results[1] // 1-indexed
	assert.True(t, result.IsFK)
	assert.Equal(t, 0.95, result.Confidence)
	assert.Equal(t, "owner", result.SemanticRole)
	assert.True(t, result.ShouldInclude)
	assert.Contains(t, result.Reasoning, "Clear FK relationship")
}

func TestFKSemanticEvaluationService_EvaluateCandidates_NotFK(t *testing.T) {
	projectID := uuid.New()

	// Create a candidate that looks like a coincidental match (small integers)
	candidates := []FKCandidate{
		{
			SourceSchema:   "public",
			SourceTable:    "products",
			SourceColumn:   "rating",
			SourceDataType: "integer",
			SourceDistinct: ptr(int64(5)),
			SourceRowCount: ptr(int64(1000)),
			MaxSourceValue: ptr(int64(5)),

			TargetSchema:   "public",
			TargetTable:    "categories",
			TargetColumn:   "id",
			TargetDataType: "integer",
			TargetDistinct: ptr(int64(5)),

			JoinCount:     1000,
			SourceMatched: 1000,
			TargetMatched: 5,
			OrphanCount:   0,

			TargetEntityName:        "Category",
			TargetEntityDescription: "Product category",
		},
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{"evaluations": [{"id": 1, "is_fk": false, "confidence": 0.90, "semantic_role": "", "reasoning": "Column 'rating' with max value 5 represents a score, not a category reference.", "should_include": false}]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())

	service := NewFKSemanticEvaluationService(nil, nil, mockFactory, testPool, circuitBreaker, zap.NewNop())

	// Execute
	results, err := service.EvaluateCandidates(context.Background(), projectID, candidates)

	// Assert
	require.NoError(t, err)
	require.Len(t, results, 1)

	result := results[1]
	assert.False(t, result.IsFK)
	assert.False(t, result.ShouldInclude)
	assert.Contains(t, result.Reasoning, "rating")
}

func TestFKSemanticEvaluationService_EvaluateCandidates_MultipleCandidates(t *testing.T) {
	projectID := uuid.New()

	// Create multiple candidates
	candidates := []FKCandidate{
		{
			SourceSchema:   "public",
			SourceTable:    "orders",
			SourceColumn:   "user_id",
			SourceDataType: "uuid",

			TargetSchema:   "public",
			TargetTable:    "users",
			TargetColumn:   "id",
			TargetDataType: "uuid",

			JoinCount:     1000,
			SourceMatched: 1000,
			OrphanCount:   0,

			TargetEntityName: "User",
		},
		{
			SourceSchema:   "public",
			SourceTable:    "orders",
			SourceColumn:   "retry_count",
			SourceDataType: "integer",

			TargetSchema:   "public",
			TargetTable:    "status_codes",
			TargetColumn:   "id",
			TargetDataType: "integer",

			JoinCount:     1000,
			SourceMatched: 1000,
			OrphanCount:   0,

			TargetEntityName: "StatusCode",
		},
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{"evaluations": [
				{"id": 1, "is_fk": true, "confidence": 0.95, "semantic_role": "owner", "reasoning": "Valid FK to User entity.", "should_include": true},
				{"id": 2, "is_fk": false, "confidence": 0.85, "semantic_role": "", "reasoning": "Column 'retry_count' is a counter, not a reference.", "should_include": false}
			]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())

	service := NewFKSemanticEvaluationService(nil, nil, mockFactory, testPool, circuitBreaker, zap.NewNop())

	// Execute
	results, err := service.EvaluateCandidates(context.Background(), projectID, candidates)

	// Assert
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.True(t, results[1].IsFK)
	assert.True(t, results[1].ShouldInclude)

	assert.False(t, results[2].IsFK)
	assert.False(t, results[2].ShouldInclude)
}

func TestFKSemanticEvaluationService_EvaluateCandidates_WithDomainKnowledge(t *testing.T) {
	projectID := uuid.New()

	candidates := []FKCandidate{
		{
			SourceSchema:   "public",
			SourceTable:    "engagements",
			SourceColumn:   "host_id",
			SourceDataType: "uuid",

			TargetSchema:   "public",
			TargetTable:    "users",
			TargetColumn:   "id",
			TargetDataType: "uuid",

			JoinCount:     1000,
			SourceMatched: 1000,
			OrphanCount:   0,

			TargetEntityName: "User",
		},
	}

	// Mock knowledge repository with domain facts
	knowledgeRepo := &mockKnowledgeRepoForFKEval{
		facts: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "terminology",
				Key:       "Host definition",
				Value:     "Host is a content creator who receives payments",
				Context:   "User role",
			},
		},
	}

	mockFactory := llm.NewMockClientFactory()
	var capturedPrompt string
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		capturedPrompt = prompt
		return &llm.GenerateResponseResult{
			Content: `{"evaluations": [{"id": 1, "is_fk": true, "confidence": 0.98, "semantic_role": "host", "reasoning": "FK to User in host role based on domain knowledge.", "should_include": true}]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())

	service := NewFKSemanticEvaluationService(knowledgeRepo, nil, mockFactory, testPool, circuitBreaker, zap.NewNop())

	// Execute
	results, err := service.EvaluateCandidates(context.Background(), projectID, candidates)

	// Assert
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Verify domain knowledge was included in prompt
	assert.Contains(t, capturedPrompt, "Domain Knowledge")
	assert.Contains(t, capturedPrompt, "Host is a content creator")

	result := results[1]
	assert.True(t, result.IsFK)
	assert.Equal(t, "host", result.SemanticRole)
}

func TestFKSemanticEvaluationService_EvaluateCandidates_EmptyCandidates(t *testing.T) {
	projectID := uuid.New()

	mockFactory := llm.NewMockClientFactory()
	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())

	service := NewFKSemanticEvaluationService(nil, nil, mockFactory, testPool, circuitBreaker, zap.NewNop())

	// Execute with empty candidates
	results, err := service.EvaluateCandidates(context.Background(), projectID, []FKCandidate{})

	// Assert
	require.NoError(t, err)
	require.Len(t, results, 0)
	assert.Equal(t, 0, mockFactory.MockClient.GenerateResponseCalls, "LLM should not be called for empty candidates")
}

func TestFKSemanticEvaluationService_EvaluateCandidates_LLMError(t *testing.T) {
	projectID := uuid.New()

	candidates := []FKCandidate{
		{
			SourceSchema:   "public",
			SourceTable:    "orders",
			SourceColumn:   "user_id",
			SourceDataType: "uuid",

			TargetSchema:   "public",
			TargetTable:    "users",
			TargetColumn:   "id",
			TargetDataType: "uuid",

			OrphanCount: 0,
		},
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return nil, llm.NewError(llm.ErrorTypeAuth, "invalid api key", false, errors.New("401 unauthorized"))
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())

	service := NewFKSemanticEvaluationService(nil, nil, mockFactory, testPool, circuitBreaker, zap.NewNop())

	// Execute
	results, err := service.EvaluateCandidates(context.Background(), projectID, candidates)

	// Assert
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "LLM call failed")
}

func TestFKSemanticEvaluationService_EvaluateCandidates_InvalidJSONResponse(t *testing.T) {
	projectID := uuid.New()

	candidates := []FKCandidate{
		{
			SourceSchema:   "public",
			SourceTable:    "orders",
			SourceColumn:   "user_id",
			SourceDataType: "uuid",

			TargetSchema:   "public",
			TargetTable:    "users",
			TargetColumn:   "id",
			TargetDataType: "uuid",

			OrphanCount: 0,
		},
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `This is not valid JSON`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())

	service := NewFKSemanticEvaluationService(nil, nil, mockFactory, testPool, circuitBreaker, zap.NewNop())

	// Execute
	results, err := service.EvaluateCandidates(context.Background(), projectID, candidates)

	// Assert
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "parse LLM response")
}

func TestFKSemanticEvaluationService_EvaluateCandidates_RetryOnTransientError(t *testing.T) {
	projectID := uuid.New()

	candidates := []FKCandidate{
		{
			SourceSchema:   "public",
			SourceTable:    "orders",
			SourceColumn:   "user_id",
			SourceDataType: "uuid",

			TargetSchema:   "public",
			TargetTable:    "users",
			TargetColumn:   "id",
			TargetDataType: "uuid",

			OrphanCount: 0,
		},
	}

	mockFactory := llm.NewMockClientFactory()
	callCount := 0
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		callCount++
		if callCount == 1 {
			return nil, llm.NewError(llm.ErrorTypeEndpoint, "timeout", true, errors.New("timeout"))
		}
		return &llm.GenerateResponseResult{
			Content: `{"evaluations": [{"id": 1, "is_fk": true, "confidence": 0.95, "semantic_role": "owner", "reasoning": "Valid FK.", "should_include": true}]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())

	service := NewFKSemanticEvaluationService(nil, nil, mockFactory, testPool, circuitBreaker, zap.NewNop())

	// Execute
	results, err := service.EvaluateCandidates(context.Background(), projectID, candidates)

	// Assert
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[1].IsFK)
	assert.Equal(t, 2, callCount, "Should retry once after transient error")
}

func TestFKSemanticEvaluationService_EvaluateCandidate_Single(t *testing.T) {
	projectID := uuid.New()

	candidate := FKCandidate{
		SourceSchema:   "public",
		SourceTable:    "orders",
		SourceColumn:   "user_id",
		SourceDataType: "uuid",

		TargetSchema:   "public",
		TargetTable:    "users",
		TargetColumn:   "id",
		TargetDataType: "uuid",

		OrphanCount: 0,

		TargetEntityName: "User",
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{"evaluations": [{"id": 1, "is_fk": true, "confidence": 0.95, "semantic_role": "owner", "reasoning": "Valid FK.", "should_include": true}]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())

	service := NewFKSemanticEvaluationService(nil, nil, mockFactory, testPool, circuitBreaker, zap.NewNop())

	// Execute
	result, err := service.EvaluateCandidate(context.Background(), projectID, candidate)

	// Assert
	require.NoError(t, err)
	assert.True(t, result.IsFK)
	assert.Equal(t, "owner", result.SemanticRole)
}

func TestFKSemanticEvaluationService_EvaluateCandidates_InvalidIDs(t *testing.T) {
	projectID := uuid.New()

	candidates := []FKCandidate{
		{
			SourceSchema:   "public",
			SourceTable:    "orders",
			SourceColumn:   "user_id",
			SourceDataType: "uuid",

			TargetSchema:   "public",
			TargetTable:    "users",
			TargetColumn:   "id",
			TargetDataType: "uuid",

			OrphanCount: 0,
		},
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		// LLM returns an invalid ID (hallucinated)
		return &llm.GenerateResponseResult{
			Content: `{"evaluations": [
				{"id": 1, "is_fk": true, "confidence": 0.95, "semantic_role": "owner", "reasoning": "Valid FK.", "should_include": true},
				{"id": 999, "is_fk": true, "confidence": 0.90, "semantic_role": "other", "reasoning": "Hallucinated.", "should_include": true}
			]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())

	service := NewFKSemanticEvaluationService(nil, nil, mockFactory, testPool, circuitBreaker, zap.NewNop())

	// Execute
	results, err := service.EvaluateCandidates(context.Background(), projectID, candidates)

	// Assert - should ignore invalid ID 999 but still return valid result for ID 1
	require.NoError(t, err)
	require.Len(t, results, 1) // Only the valid ID should be in results
	assert.True(t, results[1].IsFK)
}

func TestFKCandidateFromAnalysis(t *testing.T) {
	// Create test data
	sourceCol := &models.SchemaColumn{
		ColumnName:    "user_id",
		DataType:      "uuid",
		DistinctCount: ptr(int64(500)),
		SampleValues:  []string{"a1b2c3", "d4e5f6", "g7h8i9"},
	}
	sourceTable := &models.SchemaTable{
		SchemaName: "public",
		TableName:  "orders",
		RowCount:   ptr(int64(10000)),
	}
	targetCol := &models.SchemaColumn{
		ColumnName:    "id",
		DataType:      "uuid",
		DistinctCount: ptr(int64(500)),
	}
	targetTable := &models.SchemaTable{
		SchemaName: "public",
		TableName:  "users",
	}
	joinResult := &datasource.JoinAnalysis{
		JoinCount:      10000,
		SourceMatched:  10000,
		TargetMatched:  500,
		OrphanCount:    0,
		MaxSourceValue: ptr(int64(12345)),
	}
	targetEntity := &models.OntologyEntity{
		Name:        "User",
		Description: "A platform user account",
	}

	// Execute
	candidate := FKCandidateFromAnalysis(sourceCol, sourceTable, targetCol, targetTable, joinResult, targetEntity)

	// Assert
	assert.Equal(t, "public", candidate.SourceSchema)
	assert.Equal(t, "orders", candidate.SourceTable)
	assert.Equal(t, "user_id", candidate.SourceColumn)
	assert.Equal(t, "uuid", candidate.SourceDataType)
	assert.Equal(t, int64(500), *candidate.SourceDistinct)
	assert.Equal(t, int64(10000), *candidate.SourceRowCount)
	assert.Equal(t, []string{"a1b2c3", "d4e5f6", "g7h8i9"}, candidate.SourceSamples)

	assert.Equal(t, "public", candidate.TargetSchema)
	assert.Equal(t, "users", candidate.TargetTable)
	assert.Equal(t, "id", candidate.TargetColumn)
	assert.Equal(t, "uuid", candidate.TargetDataType)
	assert.Equal(t, int64(500), *candidate.TargetDistinct)

	assert.Equal(t, int64(10000), candidate.JoinCount)
	assert.Equal(t, int64(10000), candidate.SourceMatched)
	assert.Equal(t, int64(500), candidate.TargetMatched)
	assert.Equal(t, int64(0), candidate.OrphanCount)
	assert.Equal(t, int64(12345), *candidate.MaxSourceValue)

	assert.Equal(t, "User", candidate.TargetEntityName)
	assert.Equal(t, "A platform user account", candidate.TargetEntityDescription)
}

func TestFKCandidateFromAnalysis_NilEntity(t *testing.T) {
	sourceCol := &models.SchemaColumn{
		ColumnName: "user_id",
		DataType:   "uuid",
	}
	sourceTable := &models.SchemaTable{
		SchemaName: "public",
		TableName:  "orders",
	}
	targetCol := &models.SchemaColumn{
		ColumnName: "id",
		DataType:   "uuid",
	}
	targetTable := &models.SchemaTable{
		SchemaName: "public",
		TableName:  "users",
	}
	joinResult := &datasource.JoinAnalysis{
		OrphanCount: 0,
	}

	// Execute with nil entity
	candidate := FKCandidateFromAnalysis(sourceCol, sourceTable, targetCol, targetTable, joinResult, nil)

	// Assert - entity fields should be empty
	assert.Equal(t, "", candidate.TargetEntityName)
	assert.Equal(t, "", candidate.TargetEntityDescription)
}

// mockKnowledgeRepoForFKEval is a test mock for KnowledgeRepository.
type mockKnowledgeRepoForFKEval struct {
	facts []*models.KnowledgeFact
}

func (m *mockKnowledgeRepoForFKEval) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	return m.facts, nil
}

func (m *mockKnowledgeRepoForFKEval) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	var result []*models.KnowledgeFact
	for _, f := range m.facts {
		if f.FactType == factType {
			result = append(result, f)
		}
	}
	return result, nil
}

func (m *mockKnowledgeRepoForFKEval) GetByKey(ctx context.Context, projectID uuid.UUID, factType, key string) (*models.KnowledgeFact, error) {
	for _, f := range m.facts {
		if f.FactType == factType && f.Key == key {
			return f, nil
		}
	}
	return nil, nil
}

func (m *mockKnowledgeRepoForFKEval) Upsert(ctx context.Context, fact *models.KnowledgeFact) error {
	return nil
}

func (m *mockKnowledgeRepoForFKEval) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockKnowledgeRepoForFKEval) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockKnowledgeRepoForFKEval) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}

// ptr is a helper to get a pointer to a value
func ptr[T any](v T) *T {
	return &v
}
