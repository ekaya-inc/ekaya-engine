package services

import (
	"context"
	"testing"

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
