package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/retry"
)

// FKCandidate represents a potential FK relationship candidate for LLM evaluation.
// This bundles the column, table, and join analysis results for semantic evaluation.
type FKCandidate struct {
	// Source column info
	SourceSchema   string
	SourceTable    string
	SourceColumn   string
	SourceDataType string
	SourceDistinct *int64   // Distinct values in source column
	SourceRowCount *int64   // Total rows in source table
	SourceSamples  []string // Sample values from source column

	// Target column info
	TargetSchema   string
	TargetTable    string
	TargetColumn   string
	TargetDataType string
	TargetDistinct *int64 // Distinct values in target column

	// Table descriptions from TableFeatureExtraction
	// These provide semantic context for the LLM to understand table purposes
	SourceTableDescription string // What the source table represents
	TargetTableDescription string // What the target table represents

	// Join validation results
	JoinCount      int64
	SourceMatched  int64
	TargetMatched  int64
	OrphanCount    int64
	MaxSourceValue *int64

	// Target entity context (if available)
	TargetEntityName        string
	TargetEntityDescription string
}

// FKEvaluationResult represents the LLM's semantic evaluation of a FK candidate.
type FKEvaluationResult struct {
	// IsFK indicates whether this is a genuine FK relationship
	IsFK bool `json:"is_fk"`

	// Confidence score 0.0-1.0 (optional, LLM's confidence in its decision)
	Confidence float64 `json:"confidence,omitempty"`

	// SemanticRole describes the relationship meaning (e.g., "owner", "parent", "creator", "reference")
	SemanticRole string `json:"semantic_role,omitempty"`

	// Reasoning explains why the LLM made this decision
	Reasoning string `json:"reasoning,omitempty"`

	// ShouldInclude indicates if this should be included in the ontology
	// (may be false even if IsFK is true, e.g., for internal/technical FKs)
	ShouldInclude bool `json:"should_include"`
}

// FKSemanticEvaluationService evaluates FK candidates using LLMs for semantic judgment.
// This replaces deterministic name-pattern-based FK detection with LLM-based semantic analysis.
type FKSemanticEvaluationService interface {
	// EvaluateCandidates evaluates a batch of FK candidates and returns semantic judgments.
	// Candidates should already have passed join validation (low/zero orphan count).
	EvaluateCandidates(ctx context.Context, projectID uuid.UUID, candidates []FKCandidate) (map[int]*FKEvaluationResult, error)

	// EvaluateCandidate evaluates a single FK candidate.
	// This is a convenience method that wraps EvaluateCandidates.
	EvaluateCandidate(ctx context.Context, projectID uuid.UUID, candidate FKCandidate) (*FKEvaluationResult, error)
}

type fkSemanticEvaluationService struct {
	knowledgeRepo    repositories.KnowledgeRepository
	conversationRepo repositories.ConversationRepository
	llmFactory       llm.LLMClientFactory
	workerPool       *llm.WorkerPool
	circuitBreaker   *llm.CircuitBreaker
	logger           *zap.Logger
}

// NewFKSemanticEvaluationService creates a new FK semantic evaluation service.
func NewFKSemanticEvaluationService(
	knowledgeRepo repositories.KnowledgeRepository,
	conversationRepo repositories.ConversationRepository,
	llmFactory llm.LLMClientFactory,
	workerPool *llm.WorkerPool,
	circuitBreaker *llm.CircuitBreaker,
	logger *zap.Logger,
) FKSemanticEvaluationService {
	return &fkSemanticEvaluationService{
		knowledgeRepo:    knowledgeRepo,
		conversationRepo: conversationRepo,
		llmFactory:       llmFactory,
		workerPool:       workerPool,
		circuitBreaker:   circuitBreaker,
		logger:           logger.Named("fk-semantic-evaluation"),
	}
}

var _ FKSemanticEvaluationService = (*fkSemanticEvaluationService)(nil)

// EvaluateCandidates evaluates a batch of FK candidates using LLM.
func (s *fkSemanticEvaluationService) EvaluateCandidates(ctx context.Context, projectID uuid.UUID, candidates []FKCandidate) (map[int]*FKEvaluationResult, error) {
	if len(candidates) == 0 {
		return make(map[int]*FKEvaluationResult), nil
	}

	s.logger.Debug("Evaluating FK candidates",
		zap.String("project_id", projectID.String()),
		zap.Int("candidate_count", len(candidates)))

	// Check circuit breaker before attempting LLM call
	allowed, err := s.circuitBreaker.Allow()
	if !allowed {
		s.logger.Error("Circuit breaker prevented LLM call",
			zap.String("project_id", projectID.String()),
			zap.Int("candidate_count", len(candidates)),
			zap.String("circuit_state", s.circuitBreaker.State().String()),
			zap.Error(err))
		return nil, fmt.Errorf("circuit breaker open: %w", err)
	}

	// Fetch project knowledge for domain context
	var knowledgeFacts []*models.KnowledgeFact
	if s.knowledgeRepo != nil {
		knowledgeFacts, err = s.knowledgeRepo.GetByProject(ctx, projectID)
		if err != nil {
			s.logger.Warn("Failed to fetch project knowledge, continuing without it",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Continue without knowledge - don't fail the entire operation
		}
	}

	// Create LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	// Build prompt
	systemMsg := s.buildSystemMessage()
	prompt := s.buildPrompt(candidates, knowledgeFacts)

	// Retry LLM call with exponential backoff
	retryConfig := &retry.Config{
		MaxRetries:   3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
	}

	var llmResult *llm.GenerateResponseResult
	err = retry.Do(ctx, retryConfig, func() error {
		var retryErr error
		llmResult, retryErr = llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
		if retryErr != nil {
			classified := llm.ClassifyError(retryErr)
			if classified.Retryable {
				s.logger.Warn("LLM call failed, retrying",
					zap.String("error_type", string(classified.Type)),
					zap.Error(retryErr))
				return retryErr
			}
			s.logger.Error("LLM call failed with non-retryable error",
				zap.String("error_type", string(classified.Type)),
				zap.Error(retryErr))
			return retryErr
		}
		return nil
	})

	if err != nil {
		s.circuitBreaker.RecordFailure()
		s.logger.Error("LLM call failed after retries",
			zap.String("project_id", projectID.String()),
			zap.Int("candidate_count", len(candidates)),
			zap.Error(err))
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	s.circuitBreaker.RecordSuccess()

	// Parse response
	response, err := llm.ParseJSONResponse[fkEvaluationResponse](llmResult.Content)
	if err != nil {
		s.logger.Error("Failed to parse LLM response",
			zap.String("project_id", projectID.String()),
			zap.String("response_preview", truncateString(llmResult.Content, 200)),
			zap.Error(err))
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	// Build result map by ID
	results := make(map[int]*FKEvaluationResult)
	for _, eval := range response.Evaluations {
		// Validate ID is in range
		if eval.ID < 1 || eval.ID > len(candidates) {
			s.logger.Warn("LLM returned invalid candidate ID",
				zap.Int("id", eval.ID),
				zap.Int("max_valid", len(candidates)))
			continue
		}

		results[eval.ID] = &FKEvaluationResult{
			IsFK:          eval.IsFK,
			Confidence:    eval.Confidence,
			SemanticRole:  eval.SemanticRole,
			Reasoning:     eval.Reasoning,
			ShouldInclude: eval.ShouldInclude,
		}
	}

	s.logger.Debug("FK evaluation complete",
		zap.String("project_id", projectID.String()),
		zap.Int("candidates_evaluated", len(results)),
		zap.Int("candidates_total", len(candidates)))

	return results, nil
}

// EvaluateCandidate evaluates a single FK candidate.
func (s *fkSemanticEvaluationService) EvaluateCandidate(ctx context.Context, projectID uuid.UUID, candidate FKCandidate) (*FKEvaluationResult, error) {
	results, err := s.EvaluateCandidates(ctx, projectID, []FKCandidate{candidate})
	if err != nil {
		return nil, err
	}

	result, ok := results[1] // 1-indexed
	if !ok {
		return nil, fmt.Errorf("no evaluation result for candidate")
	}

	return result, nil
}

// fkEvaluationResponse wraps the LLM response for parsing.
type fkEvaluationResponse struct {
	Evaluations []fkEvaluationItem `json:"evaluations"`
}

// fkEvaluationItem represents a single candidate evaluation from the LLM.
type fkEvaluationItem struct {
	ID            int     `json:"id"`
	IsFK          bool    `json:"is_fk"`
	Confidence    float64 `json:"confidence,omitempty"`
	SemanticRole  string  `json:"semantic_role,omitempty"`
	Reasoning     string  `json:"reasoning"`
	ShouldInclude bool    `json:"should_include"`
}

func (s *fkSemanticEvaluationService) buildSystemMessage() string {
	return `You are a database relationship expert. Your task is to evaluate whether columns represent genuine foreign key relationships based on statistical evidence and semantic analysis.

Focus on:
1. Whether the column semantically represents a reference to another entity
2. What role this reference plays in the business domain (owner, parent, creator, etc.)
3. Whether this relationship is meaningful for end-users to query

Reject candidates that are:
- Coincidental matches (e.g., small integers matching unrelated lookup tables)
- Internal/technical columns (e.g., version counters, retry counts)
- Aggregate or derived columns that happen to match IDs

Accept candidates that are:
- Clear entity references (e.g., user_id referencing users)
- Parent/child relationships (e.g., parent_account referencing accounts)
- Association columns (e.g., host_id, visitor_id referencing users with specific roles)`
}

func (s *fkSemanticEvaluationService) buildPrompt(candidates []FKCandidate, knowledgeFacts []*models.KnowledgeFact) string {
	var sb strings.Builder

	sb.WriteString("# FK Candidate Semantic Evaluation\n\n")
	sb.WriteString("Evaluate whether each candidate column represents a genuine foreign key relationship.\n\n")

	// Include domain knowledge if available
	if len(knowledgeFacts) > 0 {
		sb.WriteString("## Domain Knowledge\n\n")
		sb.WriteString("Use these domain-specific facts to inform your evaluation:\n\n")

		factsByType := make(map[string][]*models.KnowledgeFact)
		for _, fact := range knowledgeFacts {
			factsByType[fact.FactType] = append(factsByType[fact.FactType], fact)
		}

		typeOrder := []string{"terminology", "business_rule", "enumeration", "convention"}
		for _, factType := range typeOrder {
			facts, exists := factsByType[factType]
			if !exists || len(facts) == 0 {
				continue
			}

			sb.WriteString(fmt.Sprintf("**%s:**\n", capitalizeWords(factType)))
			for _, fact := range facts {
				sb.WriteString(fmt.Sprintf("- %s", fact.Value))
				if fact.Context != "" {
					sb.WriteString(fmt.Sprintf(" (%s)", fact.Context))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}

	// Candidates table
	sb.WriteString("## Candidates\n\n")
	sb.WriteString("| ID | Source Column | Target Column | Join Stats | Cardinality | Target Entity |\n")
	sb.WriteString("|----|---------------|---------------|------------|-------------|---------------|\n")

	for i, c := range candidates {
		id := i + 1 // 1-indexed

		// Format source column info
		sourceCol := fmt.Sprintf("%s.%s.%s (%s)", c.SourceSchema, c.SourceTable, c.SourceColumn, c.SourceDataType)

		// Format target column info
		targetCol := fmt.Sprintf("%s.%s.%s (%s)", c.TargetSchema, c.TargetTable, c.TargetColumn, c.TargetDataType)

		// Format join stats
		var joinStats string
		if c.OrphanCount == 0 {
			joinStats = "0% orphans"
		} else {
			orphanPct := float64(c.OrphanCount) / float64(c.SourceMatched+c.OrphanCount) * 100
			joinStats = fmt.Sprintf("%.1f%% orphans (%d)", orphanPct, c.OrphanCount)
		}

		// Format cardinality
		var cardinality string
		if c.SourceDistinct != nil && c.SourceRowCount != nil && *c.SourceRowCount > 0 {
			ratio := float64(*c.SourceDistinct) / float64(*c.SourceRowCount) * 100
			cardinality = fmt.Sprintf("%d distinct (%.1f%%)", *c.SourceDistinct, ratio)
		} else if c.SourceDistinct != nil {
			cardinality = fmt.Sprintf("%d distinct", *c.SourceDistinct)
		} else {
			cardinality = "unknown"
		}

		// Format target entity
		targetEntity := "unknown"
		if c.TargetEntityName != "" {
			targetEntity = c.TargetEntityName
			if c.TargetEntityDescription != "" && len(c.TargetEntityDescription) < 50 {
				targetEntity = fmt.Sprintf("%s - %s", c.TargetEntityName, c.TargetEntityDescription)
			}
		}

		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s |\n",
			id, sourceCol, targetCol, joinStats, cardinality, targetEntity))
	}

	// Add detail sections for candidates with additional context
	sb.WriteString("\n## Additional Context\n\n")
	for i, c := range candidates {
		id := i + 1

		// Include if we have meaningful additional info (table descriptions, samples, or max value)
		hasTableDescriptions := c.SourceTableDescription != "" || c.TargetTableDescription != ""
		hasSamplesOrMax := len(c.SourceSamples) > 0 || c.MaxSourceValue != nil

		if !hasTableDescriptions && !hasSamplesOrMax {
			continue
		}

		sb.WriteString(fmt.Sprintf("### Candidate %d: %s.%s → %s.%s\n", id, c.SourceTable, c.SourceColumn, c.TargetTable, c.TargetColumn))

		// Table descriptions provide crucial semantic context for FK validation
		// Example: "Does it make sense for identity_provider (enum column in account_authentications,
		// which tracks authentication methods) to reference jobs.id (PK of jobs table, which tracks
		// background processing tasks)?" - LLM can immediately see this is nonsensical.
		if c.SourceTableDescription != "" {
			sb.WriteString(fmt.Sprintf("- **Source table purpose**: %s\n", c.SourceTableDescription))
		}
		if c.TargetTableDescription != "" {
			sb.WriteString(fmt.Sprintf("- **Target table purpose**: %s\n", c.TargetTableDescription))
		}

		if len(c.SourceSamples) > 0 {
			sb.WriteString(fmt.Sprintf("- Sample values: %s\n", strings.Join(c.SourceSamples[:min(5, len(c.SourceSamples))], ", ")))
		}

		if c.MaxSourceValue != nil {
			sb.WriteString(fmt.Sprintf("- Max value in column: %d\n", *c.MaxSourceValue))
		}

		sb.WriteString("\n")
	}

	// Instructions
	sb.WriteString("## Instructions\n\n")
	sb.WriteString("For each candidate, evaluate:\n\n")
	sb.WriteString("1. **is_fk**: Is this a genuine foreign key relationship? Consider:\n")
	sb.WriteString("   - **Table purposes**: Do the source and target table purposes make semantic sense together?\n")
	sb.WriteString("     (e.g., orders.customer_id → customers.id makes sense; settings.retry_count → jobs.id does NOT)\n")
	sb.WriteString("   - Does the column name suggest a reference? (user_id, account_ref, parent, owner)\n")
	sb.WriteString("   - Do the join statistics support a FK relationship? (low orphan count = good)\n")
	sb.WriteString("   - Is the cardinality reasonable for a FK? (not too low, not 1:1 with all rows)\n")
	sb.WriteString("   - Could this be a coincidental match? (small integers, status codes)\n\n")
	sb.WriteString("2. **semantic_role**: What role does this reference represent?\n")
	sb.WriteString("   - Examples: owner, parent, creator, reviewer, host, visitor, buyer, seller\n")
	sb.WriteString("   - Use domain knowledge to infer role semantics\n")
	sb.WriteString("   - Leave empty if it's just a generic reference\n\n")
	sb.WriteString("3. **should_include**: Should this relationship be included in the ontology?\n")
	sb.WriteString("   - True for business-meaningful relationships users would query\n")
	sb.WriteString("   - False for internal/technical relationships (version counters, retry IDs)\n\n")
	sb.WriteString("4. **reasoning**: Explain your decision (1-2 sentences)\n\n")

	// Response format
	sb.WriteString("## Response Format (JSON)\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"evaluations\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"id\": 1,\n")
	sb.WriteString("      \"is_fk\": true,\n")
	sb.WriteString("      \"confidence\": 0.95,\n")
	sb.WriteString("      \"semantic_role\": \"owner\",\n")
	sb.WriteString("      \"reasoning\": \"Column name 'owner_id' clearly indicates ownership, 0% orphans confirms FK integrity.\",\n")
	sb.WriteString("      \"should_include\": true\n")
	sb.WriteString("    },\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"id\": 2,\n")
	sb.WriteString("      \"is_fk\": false,\n")
	sb.WriteString("      \"confidence\": 0.85,\n")
	sb.WriteString("      \"semantic_role\": \"\",\n")
	sb.WriteString("      \"reasoning\": \"Column 'retry_count' happens to match IDs but represents a counter, not a reference.\",\n")
	sb.WriteString("      \"should_include\": false\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// FKCandidateFromAnalysis creates an FKCandidate from column and join analysis data.
// This is a helper for integrating with the existing pk_match discovery flow.
// Table metadata is optional but recommended for semantic context in LLM evaluation.
func FKCandidateFromAnalysis(
	sourceCol *models.SchemaColumn,
	sourceTable *models.SchemaTable,
	targetCol *models.SchemaColumn,
	targetTable *models.SchemaTable,
	joinResult *datasource.JoinAnalysis,
	targetEntity *models.OntologyEntity,
	sourceTableMeta *models.TableMetadata,
	targetTableMeta *models.TableMetadata,
) FKCandidate {
	candidate := FKCandidate{
		SourceSchema:   sourceTable.SchemaName,
		SourceTable:    sourceTable.TableName,
		SourceColumn:   sourceCol.ColumnName,
		SourceDataType: sourceCol.DataType,
		SourceDistinct: sourceCol.DistinctCount,
		SourceRowCount: sourceTable.RowCount,

		TargetSchema:   targetTable.SchemaName,
		TargetTable:    targetTable.TableName,
		TargetColumn:   targetCol.ColumnName,
		TargetDataType: targetCol.DataType,
		TargetDistinct: targetCol.DistinctCount,

		JoinCount:      joinResult.JoinCount,
		SourceMatched:  joinResult.SourceMatched,
		TargetMatched:  joinResult.TargetMatched,
		OrphanCount:    joinResult.OrphanCount,
		MaxSourceValue: joinResult.MaxSourceValue,
	}

	// Add sample values if available
	if len(sourceCol.SampleValues) > 0 {
		candidate.SourceSamples = sourceCol.SampleValues
	}

	// Add table descriptions from TableFeatureExtraction
	// These help LLM understand if relationship makes semantic sense
	if sourceTableMeta != nil && sourceTableMeta.Description != nil {
		candidate.SourceTableDescription = *sourceTableMeta.Description
	}
	if targetTableMeta != nil && targetTableMeta.Description != nil {
		candidate.TargetTableDescription = *targetTableMeta.Description
	}

	// Add entity context if available
	if targetEntity != nil {
		candidate.TargetEntityName = targetEntity.Name
		if targetEntity.Description != "" {
			candidate.TargetEntityDescription = targetEntity.Description
		}
	}

	return candidate
}

// Helper to get minimum of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
