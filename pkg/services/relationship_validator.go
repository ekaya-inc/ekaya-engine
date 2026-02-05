package services

import (
	"context"
	"fmt"
	"strings"

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
	getTenantCtx     TenantContextFunc
	logger           *zap.Logger
}

// NewRelationshipValidator creates a new relationship validator service.
func NewRelationshipValidator(
	llmFactory llm.LLMClientFactory,
	workerPool *llm.WorkerPool,
	circuitBreaker *llm.CircuitBreaker,
	conversationRepo repositories.ConversationRepository,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) RelationshipValidator {
	return &relationshipValidator{
		llmFactory:       llmFactory,
		workerPool:       workerPool,
		circuitBreaker:   circuitBreaker,
		conversationRepo: conversationRepo,
		getTenantCtx:     getTenantCtx,
		logger:           logger.Named("relationship-validator"),
	}
}

var _ RelationshipValidator = (*relationshipValidator)(nil)

// minConfidenceThreshold is the minimum confidence required to accept an LLM validation.
// Below this threshold, the relationship is rejected even if LLM says is_valid_fk=true.
// Per the design philosophy: false negatives are better than false positives.
const minConfidenceThreshold = 0.7

// ValidateCandidate asks LLM if this is a valid FK relationship.
func (v *relationshipValidator) ValidateCandidate(ctx context.Context, projectID uuid.UUID, candidate *RelationshipCandidate) (*RelationshipValidationResult, error) {
	prompt := v.buildValidationPrompt(candidate)
	systemMsg := v.systemMessage()

	// Acquire fresh tenant context to avoid concurrent map writes in pgx
	workCtx := ctx
	if v.getTenantCtx != nil {
		var cleanup func()
		var err error
		workCtx, cleanup, err = v.getTenantCtx(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("acquire tenant context: %w", err)
		}
		defer cleanup()
	}

	llmClient, err := v.llmFactory.CreateForProject(workCtx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	result, err := llmClient.GenerateResponse(workCtx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	validationResult, err := v.parseValidationResponse(result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse validation response: %w", err)
	}

	// If confidence is below threshold, treat as rejection regardless of is_valid_fk
	if validationResult.IsValidFK && validationResult.Confidence < minConfidenceThreshold {
		v.logger.Debug("rejecting low-confidence validation",
			zap.String("source", fmt.Sprintf("%s.%s", candidate.SourceTable, candidate.SourceColumn)),
			zap.String("target", fmt.Sprintf("%s.%s", candidate.TargetTable, candidate.TargetColumn)),
			zap.Float64("confidence", validationResult.Confidence),
		)
		validationResult.IsValidFK = false
		validationResult.Reasoning = fmt.Sprintf("Low confidence (%.2f < %.2f threshold): %s",
			validationResult.Confidence, minConfidenceThreshold, validationResult.Reasoning)
	}

	v.logger.Debug("validated relationship candidate",
		zap.String("source", fmt.Sprintf("%s.%s", candidate.SourceTable, candidate.SourceColumn)),
		zap.String("target", fmt.Sprintf("%s.%s", candidate.TargetTable, candidate.TargetColumn)),
		zap.Bool("is_valid", validationResult.IsValidFK),
		zap.Float64("confidence", validationResult.Confidence),
		zap.String("cardinality", validationResult.Cardinality),
	)

	return validationResult, nil
}

// systemMessage returns the system prompt for relationship validation.
func (v *relationshipValidator) systemMessage() string {
	return `You are a database schema analyst. Your task is to determine if a candidate foreign key relationship is valid.
Analyze the column metadata, sample values, and join statistics to make your decision.
Be conservative - only confirm relationships where there is strong evidence the columns represent a true FK-PK relationship.
Respond with valid JSON only.`
}

// buildValidationPrompt constructs the LLM prompt for validating a relationship candidate.
func (v *relationshipValidator) buildValidationPrompt(candidate *RelationshipCandidate) string {
	var sb strings.Builder

	sb.WriteString("# Relationship Candidate Validation\n\n")

	// Source column info
	sb.WriteString("## Source Column (Potential FK)\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", candidate.SourceTable))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", candidate.SourceColumn))
	sb.WriteString(fmt.Sprintf("**Data Type:** %s\n", candidate.SourceDataType))
	sb.WriteString(fmt.Sprintf("**Is Primary Key:** %v\n", candidate.SourceIsPK))
	sb.WriteString(fmt.Sprintf("**Distinct Values:** %d\n", candidate.SourceDistinctCount))
	sb.WriteString(fmt.Sprintf("**Null Rate:** %.1f%%\n", candidate.SourceNullRate*100))

	if candidate.SourcePurpose != "" || candidate.SourceRole != "" {
		sb.WriteString(fmt.Sprintf("**Semantic Purpose:** %s\n", candidate.SourcePurpose))
		sb.WriteString(fmt.Sprintf("**Semantic Role:** %s\n", candidate.SourceRole))
	}

	if len(candidate.SourceSamples) > 0 {
		sb.WriteString("\n**Sample Values:**\n")
		for _, sample := range candidate.SourceSamples {
			sb.WriteString(fmt.Sprintf("- `%s`\n", sample))
		}
	}

	// Target column info
	sb.WriteString("\n## Target Column (Potential PK/Unique)\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", candidate.TargetTable))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", candidate.TargetColumn))
	sb.WriteString(fmt.Sprintf("**Data Type:** %s\n", candidate.TargetDataType))
	sb.WriteString(fmt.Sprintf("**Is Primary Key:** %v\n", candidate.TargetIsPK))
	sb.WriteString(fmt.Sprintf("**Distinct Values:** %d\n", candidate.TargetDistinctCount))
	sb.WriteString(fmt.Sprintf("**Null Rate:** %.1f%%\n", candidate.TargetNullRate*100))

	if candidate.TargetPurpose != "" || candidate.TargetRole != "" {
		sb.WriteString(fmt.Sprintf("**Semantic Purpose:** %s\n", candidate.TargetPurpose))
		sb.WriteString(fmt.Sprintf("**Semantic Role:** %s\n", candidate.TargetRole))
	}

	if len(candidate.TargetSamples) > 0 {
		sb.WriteString("\n**Sample Values:**\n")
		for _, sample := range candidate.TargetSamples {
			sb.WriteString(fmt.Sprintf("- `%s`\n", sample))
		}
	}

	// Join analysis results
	sb.WriteString("\n## Join Analysis Results\n\n")

	if candidate.SourceDistinctCount > 0 {
		matchRate := float64(candidate.SourceMatched) / float64(candidate.SourceDistinctCount) * 100
		orphanRate := float64(candidate.OrphanCount) / float64(candidate.SourceDistinctCount) * 100
		sb.WriteString(fmt.Sprintf("- **%d** of **%d** source values exist in target (%.1f%% match rate)\n",
			candidate.SourceMatched, candidate.SourceDistinctCount, matchRate))
		sb.WriteString(fmt.Sprintf("- **%d** source values have no match (%.1f%% orphan rate)\n",
			candidate.OrphanCount, orphanRate))
	}

	if candidate.TargetDistinctCount > 0 {
		coverageRate := float64(candidate.TargetMatched) / float64(candidate.TargetDistinctCount) * 100
		sb.WriteString(fmt.Sprintf("- **%d** of **%d** target values are referenced (%.1f%% coverage)\n",
			candidate.TargetMatched, candidate.TargetDistinctCount, coverageRate))
		sb.WriteString(fmt.Sprintf("- **%d** target values are never referenced\n", candidate.ReverseOrphans))
	}

	sb.WriteString(fmt.Sprintf("- **%d** total rows matched when joining\n", candidate.JoinCount))

	// Task description
	sb.WriteString("\n## Task\n\n")
	sb.WriteString(fmt.Sprintf("Is **%s.%s** a foreign key referencing **%s.%s**?\n\n",
		candidate.SourceTable, candidate.SourceColumn,
		candidate.TargetTable, candidate.TargetColumn))

	sb.WriteString("Consider:\n")
	sb.WriteString("- Do the sample values suggest these columns represent the same entity type?\n")
	sb.WriteString("- Is the join direction correct (FK → PK)?\n")
	sb.WriteString("- Does a high orphan rate suggest data integrity issues or a false positive?\n")
	sb.WriteString("- Do the column names semantically relate to each other?\n")
	sb.WriteString("- What semantic role does the source column play in its table (if any)?\n\n")

	// Response format
	sb.WriteString("## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"is_valid_fk\": true,\n")
	sb.WriteString("  \"confidence\": 0.85,\n")
	sb.WriteString("  \"cardinality\": \"N:1\",\n")
	sb.WriteString("  \"reasoning\": \"Brief explanation of why this is or isn't a valid FK relationship.\",\n")
	sb.WriteString("  \"source_role\": \"owner\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n\n")

	sb.WriteString("**Field definitions:**\n")
	sb.WriteString("- `is_valid_fk`: true if this is a valid FK relationship, false otherwise\n")
	sb.WriteString("- `confidence`: 0.0-1.0 confidence in your decision\n")
	sb.WriteString("- `cardinality`: \"1:1\", \"N:1\", \"1:N\", or \"N:M\" (most FKs are N:1)\n")
	sb.WriteString("- `reasoning`: Brief explanation of your decision\n")
	sb.WriteString("- `source_role`: Optional semantic role (e.g., \"owner\", \"creator\", \"assigned_to\", \"parent\")\n")

	return sb.String()
}

// parseValidationResponse parses the LLM response into a RelationshipValidationResult.
func (v *relationshipValidator) parseValidationResponse(content string) (*RelationshipValidationResult, error) {
	response, err := llm.ParseJSONResponse[RelationshipValidationResult](content)
	if err != nil {
		return nil, fmt.Errorf("parse relationship validation response: %w", err)
	}

	// Normalize cardinality to uppercase
	response.Cardinality = strings.ToUpper(response.Cardinality)

	// Validate cardinality is one of expected values
	switch response.Cardinality {
	case "1:1", "N:1", "1:N", "N:M":
		// Valid
	default:
		// Default to N:1 (most common FK relationship) if invalid
		if response.IsValidFK {
			v.logger.Debug("invalid cardinality from LLM, defaulting to N:1",
				zap.String("received", response.Cardinality))
			response.Cardinality = "N:1"
		}
	}

	return &response, nil
}

// ValidateCandidates validates multiple candidates in parallel using the LLM worker pool.
// Returns all results including rejected candidates (for debugging/audit).
// The progressCallback reports progress as candidates complete.
//
// Error handling:
// - If a single LLM call fails, the error is logged but processing continues
// - Returns partial results even when some validations fail
// - Only returns an error if ALL candidates fail or context is cancelled
func (v *relationshipValidator) ValidateCandidates(ctx context.Context, projectID uuid.UUID, candidates []*RelationshipCandidate, progressCallback dag.ProgressCallback) ([]*ValidatedRelationship, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	total := len(candidates)
	if progressCallback != nil {
		progressCallback(0, total, "Starting relationship validation")
	}

	// Build work items for parallel processing
	workItems := make([]llm.WorkItem[*ValidatedRelationship], len(candidates))
	for i, candidate := range candidates {
		// Capture loop variables for closure
		c := candidate
		workItems[i] = llm.WorkItem[*ValidatedRelationship]{
			ID: fmt.Sprintf("%s.%s->%s.%s", c.SourceTable, c.SourceColumn, c.TargetTable, c.TargetColumn),
			Execute: func(ctx context.Context) (*ValidatedRelationship, error) {
				result, err := v.ValidateCandidate(ctx, projectID, c)
				if err != nil {
					return nil, err
				}
				return &ValidatedRelationship{
					Candidate: c,
					Result:    result,
					Validated: true,
				}, nil
			},
		}
	}

	// Process all candidates with worker pool
	var lastProgressReport int
	results := llm.Process(ctx, v.workerPool, workItems, func(completed, total int) {
		// Report progress every 5 completions or at the end
		if progressCallback != nil && (completed-lastProgressReport >= 5 || completed == total) {
			progressCallback(completed, total, "Validating relationships...")
			lastProgressReport = completed
		}
	})

	// Collect results and track errors
	var validResults []*ValidatedRelationship
	var failedCount int
	var lastErr error

	for _, r := range results {
		if r.Err != nil {
			failedCount++
			lastErr = r.Err

			// Log the failure but continue processing
			v.logger.Warn("relationship validation failed",
				zap.String("candidate", r.ID),
				zap.Error(r.Err),
			)
			continue
		}

		if r.Result != nil {
			validResults = append(validResults, r.Result)
		}
	}

	// Log summary
	validCount := 0
	for _, vr := range validResults {
		if vr.Result != nil && vr.Result.IsValidFK {
			validCount++
		}
	}

	v.logger.Info("relationship validation complete",
		zap.Int("total_candidates", total),
		zap.Int("processed", len(validResults)),
		zap.Int("failed", failedCount),
		zap.Int("valid_relationships", validCount),
	)

	// Return error only if ALL candidates failed
	if failedCount == total {
		return nil, fmt.Errorf("all %d relationship validations failed, last error: %w", total, lastErr)
	}

	// Return partial results with no error - some failures are acceptable
	return validResults, nil
}
