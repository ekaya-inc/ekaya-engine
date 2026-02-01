package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// Mock LLM client factory for testing
type mockRelValLLMClientFactory struct{}

func (m *mockRelValLLMClientFactory) CreateForProject(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return nil, nil
}

func (m *mockRelValLLMClientFactory) CreateEmbeddingClient(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return nil, nil
}

func (m *mockRelValLLMClientFactory) CreateStreamingClient(_ context.Context, _ uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

var _ llm.LLMClientFactory = (*mockRelValLLMClientFactory)(nil)

// Mock conversation repository for testing
type mockRelValConversationRepo struct{}

func (m *mockRelValConversationRepo) Save(_ context.Context, _ *models.LLMConversation) error {
	return nil
}

func (m *mockRelValConversationRepo) Update(_ context.Context, _ *models.LLMConversation) error {
	return nil
}

func (m *mockRelValConversationRepo) UpdateStatus(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}

func (m *mockRelValConversationRepo) GetByProject(_ context.Context, _ uuid.UUID, _ int) ([]*models.LLMConversation, error) {
	return nil, nil
}

func (m *mockRelValConversationRepo) GetByContext(_ context.Context, _ uuid.UUID, _, _ string) ([]*models.LLMConversation, error) {
	return nil, nil
}

func (m *mockRelValConversationRepo) GetByConversationID(_ context.Context, _ uuid.UUID) ([]*models.LLMConversation, error) {
	return nil, nil
}

func (m *mockRelValConversationRepo) DeleteByProject(_ context.Context, _ uuid.UUID) error {
	return nil
}

var _ repositories.ConversationRepository = (*mockRelValConversationRepo)(nil)

func TestNewRelationshipValidator(t *testing.T) {
	logger := zap.NewNop()
	llmFactory := &mockRelValLLMClientFactory{}
	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), logger)
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())
	conversationRepo := &mockRelValConversationRepo{}

	validator := NewRelationshipValidator(
		llmFactory,
		workerPool,
		circuitBreaker,
		conversationRepo,
		logger,
	)

	require.NotNil(t, validator, "NewRelationshipValidator should return a non-nil validator")

	// The return type is already RelationshipValidator interface, so
	// we just verify it's not nil (interface compliance is compile-time checked)
}

func TestNewRelationshipValidator_WithNilDependencies(t *testing.T) {
	// Test that constructor doesn't panic with nil dependencies
	// (actual nil-safety is enforced at method call time)
	logger := zap.NewNop()

	validator := NewRelationshipValidator(
		nil, // llmFactory
		nil, // workerPool
		nil, // circuitBreaker
		nil, // conversationRepo
		logger,
	)

	require.NotNil(t, validator, "constructor should not panic with nil dependencies")
}

func TestValidatedRelationship_ValidatedField(t *testing.T) {
	tests := []struct {
		name      string
		validated bool
		desc      string
	}{
		{
			name:      "LLM was called",
			validated: true,
			desc:      "Validated should be true when LLM validation was performed",
		},
		{
			name:      "LLM was skipped (DB-declared FK)",
			validated: false,
			desc:      "Validated should be false when LLM was skipped (e.g., DB-declared FK)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vr := &ValidatedRelationship{
				Candidate: &RelationshipCandidate{
					SourceTable:  "orders",
					SourceColumn: "user_id",
					TargetTable:  "users",
					TargetColumn: "id",
				},
				Result: &RelationshipValidationResult{
					IsValidFK:   true,
					Confidence:  0.95,
					Cardinality: "N:1",
					Reasoning:   "Valid relationship",
				},
				Validated: tt.validated,
			}

			assert.Equal(t, tt.validated, vr.Validated, tt.desc)
		})
	}
}

func TestValidatedRelationship_Composition_WithValidatedField(t *testing.T) {
	// Test that ValidatedRelationship correctly composes Candidate, Result, and Validated
	candidate := &RelationshipCandidate{
		SourceTable:         "orders",
		SourceColumn:        "customer_id",
		SourceDataType:      "uuid",
		TargetTable:         "customers",
		TargetColumn:        "id",
		TargetDataType:      "uuid",
		TargetIsPK:          true,
		SourceDistinctCount: 1000,
		TargetDistinctCount: 500,
		SourceMatched:       1000,
		OrphanCount:         0,
	}

	result := &RelationshipValidationResult{
		IsValidFK:   true,
		Confidence:  0.98,
		Cardinality: "N:1",
		Reasoning:   "All customer_id values reference valid customers",
		SourceRole:  "buyer",
	}

	// Case 1: LLM-validated relationship
	validatedByLLM := &ValidatedRelationship{
		Candidate: candidate,
		Result:    result,
		Validated: true,
	}

	assert.Equal(t, "orders", validatedByLLM.Candidate.SourceTable)
	assert.Equal(t, "customer_id", validatedByLLM.Candidate.SourceColumn)
	assert.True(t, validatedByLLM.Result.IsValidFK)
	assert.Equal(t, "N:1", validatedByLLM.Result.Cardinality)
	assert.True(t, validatedByLLM.Validated, "should be true for LLM-validated relationship")

	// Case 2: DB-declared FK (skipped LLM)
	dbDeclaredFK := &ValidatedRelationship{
		Candidate: candidate,
		Result:    result,
		Validated: false,
	}

	assert.False(t, dbDeclaredFK.Validated, "should be false for DB-declared FK that skipped LLM")
}

func TestRelationshipValidatorInterface_Compliance(t *testing.T) {
	// Verify that the concrete type satisfies the interface at compile time
	// This is also done in relationship_validator.go but we test it explicitly
	var _ RelationshipValidator = (*relationshipValidator)(nil)
}

// ============================================================================
// Mock LLM Client and Factory for ValidateCandidate tests
// ============================================================================

// mockValidatorLLMClient mocks the LLM client for relationship validation testing
type mockValidatorLLMClient struct {
	responseContent string
	generateErr     error
	callCount       int
	lastPrompt      string
}

func (m *mockValidatorLLMClient) GenerateResponse(_ context.Context, prompt string, _ string, _ float64, _ bool) (*llm.GenerateResponseResult, error) {
	m.callCount++
	m.lastPrompt = prompt
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

func (m *mockValidatorLLMClient) CreateEmbedding(_ context.Context, _ string, _ string) ([]float32, error) {
	return nil, nil
}

func (m *mockValidatorLLMClient) CreateEmbeddings(_ context.Context, _ []string, _ string) ([][]float32, error) {
	return nil, nil
}

func (m *mockValidatorLLMClient) GetModel() string {
	return "test-model"
}

func (m *mockValidatorLLMClient) GetEndpoint() string {
	return "https://test.endpoint"
}

var _ llm.LLMClient = (*mockValidatorLLMClient)(nil)

// mockValidatorLLMClientFactory creates mockValidatorLLMClient instances
type mockValidatorLLMClientFactory struct {
	client *mockValidatorLLMClient
}

func (m *mockValidatorLLMClientFactory) CreateForProject(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return m.client, nil
}

func (m *mockValidatorLLMClientFactory) CreateEmbeddingClient(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return m.client, nil
}

func (m *mockValidatorLLMClientFactory) CreateStreamingClient(_ context.Context, _ uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

var _ llm.LLMClientFactory = (*mockValidatorLLMClientFactory)(nil)

// ============================================================================
// ValidateCandidate Tests
// ============================================================================

func TestValidateCandidate_ValidFK(t *testing.T) {
	// Setup: candidate where user_id -> users.id
	// Mock LLM returns is_valid_fk=true, confidence=0.95, cardinality="N:1"
	mockClient := &mockValidatorLLMClient{
		responseContent: `{
			"is_valid_fk": true,
			"confidence": 0.95,
			"cardinality": "N:1",
			"reasoning": "Column name user_id clearly references users table. All values match.",
			"source_role": "owner"
		}`,
	}

	validator := NewRelationshipValidator(
		&mockValidatorLLMClientFactory{client: mockClient},
		nil,
		nil,
		&mockRelValConversationRepo{},
		zap.NewNop(),
	)

	candidate := &RelationshipCandidate{
		SourceTable:         "orders",
		SourceColumn:        "user_id",
		SourceDataType:      "uuid",
		SourceDistinctCount: 100,
		SourceMatched:       100,
		OrphanCount:         0,
		SourceSamples:       []string{"a1b2c3d4-e5f6-7890-abcd-ef1234567890"},
		TargetTable:         "users",
		TargetColumn:        "id",
		TargetDataType:      "uuid",
		TargetIsPK:          true,
		TargetDistinctCount: 500,
		TargetSamples:       []string{"a1b2c3d4-e5f6-7890-abcd-ef1234567890"},
	}

	result, err := validator.ValidateCandidate(context.Background(), uuid.New(), candidate)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsValidFK)
	assert.Equal(t, 0.95, result.Confidence)
	assert.Equal(t, "N:1", result.Cardinality)
	assert.Equal(t, "owner", result.SourceRole)
	assert.Contains(t, result.Reasoning, "user_id")
	assert.Equal(t, 1, mockClient.callCount)
}

func TestValidateCandidate_InvalidFK(t *testing.T) {
	// Setup: candidate where id -> messages.nonce (bad inference)
	// Mock LLM returns is_valid_fk=false, reasoning="nonce is not an identifier"
	mockClient := &mockValidatorLLMClient{
		responseContent: `{
			"is_valid_fk": false,
			"confidence": 0.90,
			"cardinality": "",
			"reasoning": "nonce is a random value for deduplication, not an identifier. The id column should not reference nonce.",
			"source_role": ""
		}`,
	}

	validator := NewRelationshipValidator(
		&mockValidatorLLMClientFactory{client: mockClient},
		nil,
		nil,
		&mockRelValConversationRepo{},
		zap.NewNop(),
	)

	candidate := &RelationshipCandidate{
		SourceTable:         "billing_activities",
		SourceColumn:        "id",
		SourceDataType:      "uuid",
		SourceIsPK:          true,
		SourceDistinctCount: 1000,
		SourceMatched:       50, // Low match rate
		OrphanCount:         950,
		TargetTable:         "billing_activity_messages",
		TargetColumn:        "nonce",
		TargetDataType:      "uuid",
		TargetIsPK:          false,
		TargetDistinctCount: 800,
	}

	result, err := validator.ValidateCandidate(context.Background(), uuid.New(), candidate)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsValidFK)
	assert.Contains(t, result.Reasoning, "nonce")
}

func TestValidateCandidate_LowConfidence_RejectsValidation(t *testing.T) {
	// Setup: ambiguous candidate
	// Mock LLM returns is_valid_fk=true but confidence=0.4 (below 0.7 threshold)
	// Result should be rejected due to low confidence
	mockClient := &mockValidatorLLMClient{
		responseContent: `{
			"is_valid_fk": true,
			"confidence": 0.4,
			"cardinality": "N:1",
			"reasoning": "Possibly a FK but not enough evidence.",
			"source_role": ""
		}`,
	}

	validator := NewRelationshipValidator(
		&mockValidatorLLMClientFactory{client: mockClient},
		nil,
		nil,
		&mockRelValConversationRepo{},
		zap.NewNop(),
	)

	candidate := &RelationshipCandidate{
		SourceTable:         "logs",
		SourceColumn:        "ref_id",
		SourceDataType:      "uuid",
		SourceDistinctCount: 500,
		SourceMatched:       300,
		OrphanCount:         200,
		TargetTable:         "entities",
		TargetColumn:        "id",
		TargetDataType:      "uuid",
		TargetIsPK:          true,
		TargetDistinctCount: 1000,
	}

	result, err := validator.ValidateCandidate(context.Background(), uuid.New(), candidate)

	require.NoError(t, err)
	require.NotNil(t, result)
	// Key assertion: low confidence should cause rejection
	assert.False(t, result.IsValidFK, "should reject low-confidence validation")
	assert.Equal(t, 0.4, result.Confidence)
	assert.Contains(t, result.Reasoning, "Low confidence")
	assert.Contains(t, result.Reasoning, "0.70 threshold")
}

func TestValidateCandidate_ConfidenceAtThreshold(t *testing.T) {
	// Test confidence exactly at threshold (0.7) - should be accepted
	mockClient := &mockValidatorLLMClient{
		responseContent: `{
			"is_valid_fk": true,
			"confidence": 0.7,
			"cardinality": "N:1",
			"reasoning": "Just enough confidence.",
			"source_role": ""
		}`,
	}

	validator := NewRelationshipValidator(
		&mockValidatorLLMClientFactory{client: mockClient},
		nil,
		nil,
		&mockRelValConversationRepo{},
		zap.NewNop(),
	)

	candidate := &RelationshipCandidate{
		SourceTable:         "items",
		SourceColumn:        "category_id",
		SourceDataType:      "int",
		SourceDistinctCount: 50,
		TargetTable:         "categories",
		TargetColumn:        "id",
		TargetDataType:      "int",
		TargetIsPK:          true,
		TargetDistinctCount: 100,
	}

	result, err := validator.ValidateCandidate(context.Background(), uuid.New(), candidate)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsValidFK, "confidence at threshold should be accepted")
}

func TestValidateCandidate_LLMError(t *testing.T) {
	// Test error propagation from LLM
	mockClient := &mockValidatorLLMClient{
		generateErr: assert.AnError,
	}

	validator := NewRelationshipValidator(
		&mockValidatorLLMClientFactory{client: mockClient},
		nil,
		nil,
		&mockRelValConversationRepo{},
		zap.NewNop(),
	)

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	result, err := validator.ValidateCandidate(context.Background(), uuid.New(), candidate)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "LLM call failed")
}

func TestValidateCandidate_InvalidJSONResponse(t *testing.T) {
	// Test handling of malformed JSON from LLM
	mockClient := &mockValidatorLLMClient{
		responseContent: "not valid json",
	}

	validator := NewRelationshipValidator(
		&mockValidatorLLMClientFactory{client: mockClient},
		nil,
		nil,
		&mockRelValConversationRepo{},
		zap.NewNop(),
	)

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	result, err := validator.ValidateCandidate(context.Background(), uuid.New(), candidate)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "parse validation response")
}

func TestValidateCandidate_CardinalityNormalization(t *testing.T) {
	tests := []struct {
		name        string
		llmResponse string
		expected    string
	}{
		{
			name: "lowercase cardinality",
			llmResponse: `{
				"is_valid_fk": true,
				"confidence": 0.9,
				"cardinality": "n:1",
				"reasoning": "Valid FK"
			}`,
			expected: "N:1",
		},
		{
			name: "1:1 cardinality",
			llmResponse: `{
				"is_valid_fk": true,
				"confidence": 0.9,
				"cardinality": "1:1",
				"reasoning": "One-to-one relationship"
			}`,
			expected: "1:1",
		},
		{
			name: "1:N cardinality",
			llmResponse: `{
				"is_valid_fk": true,
				"confidence": 0.9,
				"cardinality": "1:n",
				"reasoning": "One-to-many relationship"
			}`,
			expected: "1:N",
		},
		{
			name: "N:M cardinality",
			llmResponse: `{
				"is_valid_fk": true,
				"confidence": 0.9,
				"cardinality": "n:m",
				"reasoning": "Many-to-many relationship"
			}`,
			expected: "N:M",
		},
		{
			name: "invalid cardinality defaults to N:1",
			llmResponse: `{
				"is_valid_fk": true,
				"confidence": 0.9,
				"cardinality": "many-to-one",
				"reasoning": "Valid FK"
			}`,
			expected: "N:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockValidatorLLMClient{
				responseContent: tt.llmResponse,
			}

			validator := NewRelationshipValidator(
				&mockValidatorLLMClientFactory{client: mockClient},
				nil,
				nil,
				&mockRelValConversationRepo{},
				zap.NewNop(),
			)

			candidate := &RelationshipCandidate{
				SourceTable:  "orders",
				SourceColumn: "user_id",
				TargetTable:  "users",
				TargetColumn: "id",
			}

			result, err := validator.ValidateCandidate(context.Background(), uuid.New(), candidate)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expected, result.Cardinality)
		})
	}
}

// ============================================================================
// buildValidationPrompt Tests
// ============================================================================

func TestBuildValidationPrompt_IncludesAllData(t *testing.T) {
	validator := &relationshipValidator{
		logger: zap.NewNop(),
	}

	candidate := &RelationshipCandidate{
		SourceTable:         "orders",
		SourceColumn:        "customer_id",
		SourceDataType:      "uuid",
		SourceIsPK:          false,
		SourceDistinctCount: 1000,
		SourceNullRate:      0.05,
		SourceSamples:       []string{"abc-123", "def-456"},
		SourcePurpose:       "identifier",
		SourceRole:          "foreign_key",
		TargetTable:         "customers",
		TargetColumn:        "id",
		TargetDataType:      "uuid",
		TargetIsPK:          true,
		TargetDistinctCount: 500,
		TargetNullRate:      0.0,
		TargetSamples:       []string{"abc-123", "xyz-789"},
		TargetPurpose:       "identifier",
		TargetRole:          "primary_key",
		JoinCount:           5000,
		SourceMatched:       950,
		OrphanCount:         50,
		TargetMatched:       400,
		ReverseOrphans:      100,
	}

	prompt := validator.buildValidationPrompt(candidate)

	// Verify all key sections are present
	assert.Contains(t, prompt, "# Relationship Candidate Validation")
	assert.Contains(t, prompt, "## Source Column")
	assert.Contains(t, prompt, "## Target Column")
	assert.Contains(t, prompt, "## Join Analysis Results")
	assert.Contains(t, prompt, "## Task")
	assert.Contains(t, prompt, "## Response Format")

	// Verify source column data
	assert.Contains(t, prompt, "orders")
	assert.Contains(t, prompt, "customer_id")
	assert.Contains(t, prompt, "uuid")
	assert.Contains(t, prompt, "1000") // distinct count
	assert.Contains(t, prompt, "5.0%") // null rate
	assert.Contains(t, prompt, "abc-123")
	assert.Contains(t, prompt, "identifier")
	assert.Contains(t, prompt, "foreign_key")

	// Verify target column data
	assert.Contains(t, prompt, "customers")
	assert.Contains(t, prompt, "id")
	assert.Contains(t, prompt, "primary_key")

	// Verify join statistics
	assert.Contains(t, prompt, "950")  // source matched
	assert.Contains(t, prompt, "50")   // orphan count
	assert.Contains(t, prompt, "5000") // join count
	assert.Contains(t, prompt, "400")  // target matched
	assert.Contains(t, prompt, "100")  // reverse orphans

	// Verify JSON format example
	assert.Contains(t, prompt, "is_valid_fk")
	assert.Contains(t, prompt, "confidence")
	assert.Contains(t, prompt, "cardinality")
	assert.Contains(t, prompt, "reasoning")
	assert.Contains(t, prompt, "source_role")
}

func TestBuildValidationPrompt_HandlesEmptySamples(t *testing.T) {
	validator := &relationshipValidator{
		logger: zap.NewNop(),
	}

	candidate := &RelationshipCandidate{
		SourceTable:         "orders",
		SourceColumn:        "user_id",
		SourceDataType:      "int",
		SourceDistinctCount: 100,
		TargetTable:         "users",
		TargetColumn:        "id",
		TargetDataType:      "int",
		TargetDistinctCount: 50,
		// No samples
	}

	prompt := validator.buildValidationPrompt(candidate)

	// Should not have empty sample sections
	assert.NotContains(t, prompt, "Sample Values:\n- ``")
	// Should still have the structure
	assert.Contains(t, prompt, "orders.user_id")
	assert.Contains(t, prompt, "users.id")
}

func TestBuildValidationPrompt_CalculatesRatesCorrectly(t *testing.T) {
	validator := &relationshipValidator{
		logger: zap.NewNop(),
	}

	candidate := &RelationshipCandidate{
		SourceTable:         "orders",
		SourceColumn:        "user_id",
		SourceDataType:      "uuid",
		SourceDistinctCount: 1000,
		SourceMatched:       900, // 90% match rate
		OrphanCount:         100, // 10% orphan rate
		TargetTable:         "users",
		TargetColumn:        "id",
		TargetDataType:      "uuid",
		TargetDistinctCount: 500,
		TargetMatched:       250, // 50% coverage
		ReverseOrphans:      250,
	}

	prompt := validator.buildValidationPrompt(candidate)

	assert.Contains(t, prompt, "90.0% match rate")
	assert.Contains(t, prompt, "10.0% orphan rate")
	assert.Contains(t, prompt, "50.0% coverage")
}

func TestBuildValidationPrompt_HandlesZeroDistinctCount(t *testing.T) {
	validator := &relationshipValidator{
		logger: zap.NewNop(),
	}

	candidate := &RelationshipCandidate{
		SourceTable:         "empty_table",
		SourceColumn:        "ref_id",
		SourceDataType:      "uuid",
		SourceDistinctCount: 0, // Zero distinct values
		TargetTable:         "targets",
		TargetColumn:        "id",
		TargetDataType:      "uuid",
		TargetDistinctCount: 0, // Also zero
	}

	// Should not panic or divide by zero
	prompt := validator.buildValidationPrompt(candidate)

	assert.Contains(t, prompt, "empty_table.ref_id")
	assert.Contains(t, prompt, "targets.id")
}

// ============================================================================
// ValidateCandidates Tests (Task 3.3 - Worker Pool Parallel Validation)
// ============================================================================

// mockParallelLLMClient tracks calls to allow testing parallel execution
type mockParallelLLMClient struct {
	responseFunc func(candidate string) (string, error)
	callCount    int
	mu           sync.Mutex
	callOrder    []string // Records order of calls for parallelism testing
}

func (m *mockParallelLLMClient) GenerateResponse(_ context.Context, prompt string, _ string, _ float64, _ bool) (*llm.GenerateResponseResult, error) {
	m.mu.Lock()
	m.callCount++
	// Extract candidate ID from prompt for tracking
	m.callOrder = append(m.callOrder, prompt[:50]) // First 50 chars as identifier
	m.mu.Unlock()

	response, err := m.responseFunc(prompt)
	if err != nil {
		return nil, err
	}
	return &llm.GenerateResponseResult{
		Content:          response,
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}, nil
}

func (m *mockParallelLLMClient) CreateEmbedding(_ context.Context, _ string, _ string) ([]float32, error) {
	return nil, nil
}

func (m *mockParallelLLMClient) CreateEmbeddings(_ context.Context, _ []string, _ string) ([][]float32, error) {
	return nil, nil
}

func (m *mockParallelLLMClient) GetModel() string {
	return "test-model"
}

func (m *mockParallelLLMClient) GetEndpoint() string {
	return "https://test.endpoint"
}

var _ llm.LLMClient = (*mockParallelLLMClient)(nil)

type mockParallelLLMClientFactory struct {
	client *mockParallelLLMClient
}

func (m *mockParallelLLMClientFactory) CreateForProject(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return m.client, nil
}

func (m *mockParallelLLMClientFactory) CreateEmbeddingClient(_ context.Context, _ uuid.UUID) (llm.LLMClient, error) {
	return m.client, nil
}

func (m *mockParallelLLMClientFactory) CreateStreamingClient(_ context.Context, _ uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

var _ llm.LLMClientFactory = (*mockParallelLLMClientFactory)(nil)

func TestValidateCandidates_EmptySlice(t *testing.T) {
	validator := NewRelationshipValidator(
		nil,
		nil,
		nil,
		nil,
		zap.NewNop(),
	)

	results, err := validator.ValidateCandidates(context.Background(), uuid.New(), nil, nil)

	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestValidateCandidates_SingleCandidate(t *testing.T) {
	mockClient := &mockParallelLLMClient{
		responseFunc: func(_ string) (string, error) {
			return `{
				"is_valid_fk": true,
				"confidence": 0.95,
				"cardinality": "N:1",
				"reasoning": "Valid FK relationship",
				"source_role": "owner"
			}`, nil
		},
	}

	logger := zap.NewNop()
	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), logger)

	validator := NewRelationshipValidator(
		&mockParallelLLMClientFactory{client: mockClient},
		workerPool,
		nil,
		nil,
		logger,
	)

	candidates := []*RelationshipCandidate{
		{
			SourceTable:  "orders",
			SourceColumn: "user_id",
			TargetTable:  "users",
			TargetColumn: "id",
		},
	}

	var progressCalls []string
	progressCallback := func(current, total int, msg string) {
		progressCalls = append(progressCalls, fmt.Sprintf("%d/%d: %s", current, total, msg))
	}

	results, err := validator.ValidateCandidates(context.Background(), uuid.New(), candidates, progressCallback)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Result.IsValidFK)
	assert.True(t, results[0].Validated)
	assert.Equal(t, 1, mockClient.callCount)

	// Verify progress was reported
	assert.NotEmpty(t, progressCalls)
}

func TestValidateCandidates_MultipleCandidates(t *testing.T) {
	mockClient := &mockParallelLLMClient{
		responseFunc: func(prompt string) (string, error) {
			// Return different results based on prompt content
			if strings.Contains(prompt, "user_id") {
				return `{"is_valid_fk": true, "confidence": 0.95, "cardinality": "N:1", "reasoning": "Valid user FK"}`, nil
			}
			if strings.Contains(prompt, "product_id") {
				return `{"is_valid_fk": true, "confidence": 0.9, "cardinality": "N:1", "reasoning": "Valid product FK"}`, nil
			}
			if strings.Contains(prompt, "invalid_ref") {
				return `{"is_valid_fk": false, "confidence": 0.85, "cardinality": "", "reasoning": "Not a valid FK"}`, nil
			}
			return `{"is_valid_fk": false, "confidence": 0.5, "cardinality": "", "reasoning": "Unknown"}`, nil
		},
	}

	logger := zap.NewNop()
	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), logger)

	validator := NewRelationshipValidator(
		&mockParallelLLMClientFactory{client: mockClient},
		workerPool,
		nil,
		nil,
		logger,
	)

	candidates := []*RelationshipCandidate{
		{SourceTable: "orders", SourceColumn: "user_id", TargetTable: "users", TargetColumn: "id"},
		{SourceTable: "orders", SourceColumn: "product_id", TargetTable: "products", TargetColumn: "id"},
		{SourceTable: "logs", SourceColumn: "invalid_ref", TargetTable: "entities", TargetColumn: "id"},
	}

	results, err := validator.ValidateCandidates(context.Background(), uuid.New(), candidates, nil)

	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, 3, mockClient.callCount)

	// Count valid FKs
	validCount := 0
	for _, r := range results {
		if r.Result.IsValidFK {
			validCount++
		}
	}
	assert.Equal(t, 2, validCount, "should have 2 valid FKs")
}

func TestValidateCandidates_PartialFailure(t *testing.T) {
	// Test that partial failures don't fail the entire batch
	callNum := 0
	mockClient := &mockParallelLLMClient{
		responseFunc: func(_ string) (string, error) {
			callNum++
			if callNum == 2 {
				// Fail the second call
				return "", fmt.Errorf("LLM service unavailable")
			}
			return `{"is_valid_fk": true, "confidence": 0.9, "cardinality": "N:1", "reasoning": "Valid"}`, nil
		},
	}

	logger := zap.NewNop()
	workerPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, logger) // Single worker to ensure order

	validator := NewRelationshipValidator(
		&mockParallelLLMClientFactory{client: mockClient},
		workerPool,
		nil,
		nil,
		logger,
	)

	candidates := []*RelationshipCandidate{
		{SourceTable: "t1", SourceColumn: "c1", TargetTable: "t_target", TargetColumn: "id"},
		{SourceTable: "t2", SourceColumn: "c2", TargetTable: "t_target", TargetColumn: "id"}, // This one fails
		{SourceTable: "t3", SourceColumn: "c3", TargetTable: "t_target", TargetColumn: "id"},
	}

	results, err := validator.ValidateCandidates(context.Background(), uuid.New(), candidates, nil)

	// Should NOT return error when some candidates fail
	require.NoError(t, err)
	// Should have 2 successful results (not 3)
	require.Len(t, results, 2)
}

func TestValidateCandidates_AllFailures(t *testing.T) {
	// Test that ALL failures returns an error
	mockClient := &mockParallelLLMClient{
		responseFunc: func(_ string) (string, error) {
			return "", fmt.Errorf("LLM service unavailable")
		},
	}

	logger := zap.NewNop()
	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), logger)

	validator := NewRelationshipValidator(
		&mockParallelLLMClientFactory{client: mockClient},
		workerPool,
		nil,
		nil,
		logger,
	)

	candidates := []*RelationshipCandidate{
		{SourceTable: "t1", SourceColumn: "c1", TargetTable: "t_target", TargetColumn: "id"},
		{SourceTable: "t2", SourceColumn: "c2", TargetTable: "t_target", TargetColumn: "id"},
	}

	results, err := validator.ValidateCandidates(context.Background(), uuid.New(), candidates, nil)

	// Should return error when ALL candidates fail
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 2 relationship validations failed")
	assert.Nil(t, results)
}

func TestValidateCandidates_ContextCancellation(t *testing.T) {
	// Create a slow mock that will be interrupted
	mockClient := &mockParallelLLMClient{
		responseFunc: func(_ string) (string, error) {
			// Simulate slow LLM call
			time.Sleep(100 * time.Millisecond)
			return `{"is_valid_fk": true, "confidence": 0.9, "cardinality": "N:1", "reasoning": "Valid"}`, nil
		},
	}

	logger := zap.NewNop()
	workerPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 2}, logger)

	validator := NewRelationshipValidator(
		&mockParallelLLMClientFactory{client: mockClient},
		workerPool,
		nil,
		nil,
		logger,
	)

	// Create many candidates
	var candidates []*RelationshipCandidate
	for i := 0; i < 10; i++ {
		candidates = append(candidates, &RelationshipCandidate{
			SourceTable:  fmt.Sprintf("t%d", i),
			SourceColumn: "ref_id",
			TargetTable:  "target",
			TargetColumn: "id",
		})
	}

	// Cancel context after short delay
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	results, err := validator.ValidateCandidates(ctx, uuid.New(), candidates, nil)

	// Should complete with partial or no results due to cancellation
	// The exact behavior depends on timing, but should not panic
	_ = results
	_ = err
	// Just verify it doesn't hang or panic
}

func TestValidateCandidates_ProgressCallback(t *testing.T) {
	mockClient := &mockParallelLLMClient{
		responseFunc: func(_ string) (string, error) {
			return `{"is_valid_fk": true, "confidence": 0.9, "cardinality": "N:1", "reasoning": "Valid"}`, nil
		},
	}

	logger := zap.NewNop()
	workerPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 2}, logger)

	validator := NewRelationshipValidator(
		&mockParallelLLMClientFactory{client: mockClient},
		workerPool,
		nil,
		nil,
		logger,
	)

	// Create 10 candidates to test progress reporting
	var candidates []*RelationshipCandidate
	for i := 0; i < 10; i++ {
		candidates = append(candidates, &RelationshipCandidate{
			SourceTable:  fmt.Sprintf("t%d", i),
			SourceColumn: "ref_id",
			TargetTable:  "target",
			TargetColumn: "id",
		})
	}

	var progressCalls []struct {
		current int
		total   int
		msg     string
	}
	var mu sync.Mutex

	progressCallback := func(current, total int, msg string) {
		mu.Lock()
		defer mu.Unlock()
		progressCalls = append(progressCalls, struct {
			current int
			total   int
			msg     string
		}{current, total, msg})
	}

	results, err := validator.ValidateCandidates(context.Background(), uuid.New(), candidates, progressCallback)

	require.NoError(t, err)
	require.Len(t, results, 10)

	// Verify progress was reported
	mu.Lock()
	defer mu.Unlock()

	assert.NotEmpty(t, progressCalls, "progress should be reported")

	// First call should be at 0
	if len(progressCalls) > 0 {
		assert.Equal(t, 0, progressCalls[0].current)
		assert.Equal(t, 10, progressCalls[0].total)
	}

	// Last call should be at 10/10
	if len(progressCalls) > 1 {
		lastCall := progressCalls[len(progressCalls)-1]
		assert.Equal(t, 10, lastCall.current)
		assert.Equal(t, 10, lastCall.total)
	}
}

func TestValidateCandidates_NilProgressCallback(t *testing.T) {
	// Ensure nil progress callback doesn't cause panic
	mockClient := &mockParallelLLMClient{
		responseFunc: func(_ string) (string, error) {
			return `{"is_valid_fk": true, "confidence": 0.9, "cardinality": "N:1", "reasoning": "Valid"}`, nil
		},
	}

	logger := zap.NewNop()
	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), logger)

	validator := NewRelationshipValidator(
		&mockParallelLLMClientFactory{client: mockClient},
		workerPool,
		nil,
		nil,
		logger,
	)

	candidates := []*RelationshipCandidate{
		{SourceTable: "orders", SourceColumn: "user_id", TargetTable: "users", TargetColumn: "id"},
	}

	// Should not panic with nil progress callback
	results, err := validator.ValidateCandidates(context.Background(), uuid.New(), candidates, nil)

	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestValidateCandidates_ResultsMatchCandidates(t *testing.T) {
	// Verify that results can be correlated back to their candidates
	mockClient := &mockParallelLLMClient{
		responseFunc: func(prompt string) (string, error) {
			if strings.Contains(prompt, "orders") {
				return `{"is_valid_fk": true, "confidence": 0.95, "cardinality": "N:1", "reasoning": "orders FK", "source_role": "buyer"}`, nil
			}
			return `{"is_valid_fk": false, "confidence": 0.8, "cardinality": "", "reasoning": "not a FK"}`, nil
		},
	}

	logger := zap.NewNop()
	workerPool := llm.NewWorkerPool(llm.DefaultWorkerPoolConfig(), logger)

	validator := NewRelationshipValidator(
		&mockParallelLLMClientFactory{client: mockClient},
		workerPool,
		nil,
		nil,
		logger,
	)

	orderCandidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "customer_id",
		TargetTable:  "customers",
		TargetColumn: "id",
	}
	logCandidate := &RelationshipCandidate{
		SourceTable:  "logs",
		SourceColumn: "ref_id",
		TargetTable:  "entities",
		TargetColumn: "id",
	}

	candidates := []*RelationshipCandidate{orderCandidate, logCandidate}

	results, err := validator.ValidateCandidates(context.Background(), uuid.New(), candidates, nil)

	require.NoError(t, err)
	require.Len(t, results, 2)

	// Find the orders result
	var ordersResult *ValidatedRelationship
	for _, r := range results {
		if r.Candidate.SourceTable == "orders" {
			ordersResult = r
			break
		}
	}

	require.NotNil(t, ordersResult, "should find orders result")
	assert.True(t, ordersResult.Result.IsValidFK)
	assert.Equal(t, "buyer", ordersResult.Result.SourceRole)
	assert.True(t, ordersResult.Validated)
}
