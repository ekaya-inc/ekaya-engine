package services

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// RelationshipValidator validates relationship candidates using LLM.
// Each candidate is evaluated independently for parallelization.
type RelationshipValidator interface {
	// ValidateCandidate asks LLM if this is a valid FK relationship.
	// One candidate per call for parallelization and progress reporting.
	ValidateCandidate(ctx context.Context, projectID uuid.UUID, candidate *RelationshipCandidate) (*RelationshipValidationResult, error)

	// ValidateCandidates validates multiple candidates in parallel using worker pool.
	// Returns all results including rejected candidates (for debugging/audit).
	// progressCallback reports progress as candidates complete.
	ValidateCandidates(ctx context.Context, projectID uuid.UUID, candidates []*RelationshipCandidate, progressCallback dag.ProgressCallback) ([]*ValidatedRelationship, error)
}

type relationshipValidator struct {
	llmFactory       llm.LLMClientFactory
	workerPool       *llm.WorkerPool
	circuitBreaker   *llm.CircuitBreaker
	conversationRepo repositories.ConversationRepository
	logger           *zap.Logger
}

// NewRelationshipValidator creates a new relationship validator service.
func NewRelationshipValidator(
	llmFactory llm.LLMClientFactory,
	workerPool *llm.WorkerPool,
	circuitBreaker *llm.CircuitBreaker,
	conversationRepo repositories.ConversationRepository,
	logger *zap.Logger,
) RelationshipValidator {
	return &relationshipValidator{
		llmFactory:       llmFactory,
		workerPool:       workerPool,
		circuitBreaker:   circuitBreaker,
		conversationRepo: conversationRepo,
		logger:           logger.Named("relationship-validator"),
	}
}

var _ RelationshipValidator = (*relationshipValidator)(nil)

// ValidateCandidate asks LLM if this is a valid FK relationship.
// Implementation will be added in Task 3.2.
func (v *relationshipValidator) ValidateCandidate(ctx context.Context, projectID uuid.UUID, candidate *RelationshipCandidate) (*RelationshipValidationResult, error) {
	// Placeholder - implementation in Task 3.2
	panic("not implemented: ValidateCandidate - see Task 3.2")
}

// ValidateCandidates validates multiple candidates in parallel using worker pool.
// Implementation will be added in Task 3.3.
func (v *relationshipValidator) ValidateCandidates(ctx context.Context, projectID uuid.UUID, candidates []*RelationshipCandidate, progressCallback dag.ProgressCallback) ([]*ValidatedRelationship, error) {
	// Placeholder - implementation in Task 3.3
	panic("not implemented: ValidateCandidates - see Task 3.3")
}
