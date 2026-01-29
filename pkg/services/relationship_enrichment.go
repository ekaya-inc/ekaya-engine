package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/retry"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// RelationshipEnrichmentService provides semantic enrichment for entity relationships.
// It uses LLM to generate business-meaningful descriptions for relationships.
type RelationshipEnrichmentService interface {
	// EnrichProject enriches all relationships in a project with descriptions.
	// The progressCallback is called after each batch to report progress (can be nil).
	EnrichProject(ctx context.Context, projectID uuid.UUID, progressCallback dag.ProgressCallback) (*EnrichRelationshipsResult, error)
}

// EnrichRelationshipsResult holds the result of a relationship enrichment operation.
type EnrichRelationshipsResult struct {
	RelationshipsEnriched int   `json:"relationships_enriched"`
	RelationshipsFailed   int   `json:"relationships_failed"`
	DurationMs            int64 `json:"duration_ms"`
}

// batchResult holds the result of enriching a single batch.
type batchResult struct {
	Enriched  int
	Failed    int
	BatchSize int
}

type relationshipEnrichmentService struct {
	relationshipRepo repositories.EntityRelationshipRepository
	entityRepo       repositories.OntologyEntityRepository
	knowledgeRepo    repositories.KnowledgeRepository
	conversationRepo repositories.ConversationRepository
	questionService  OntologyQuestionService
	ontologyRepo     repositories.OntologyRepository
	schemaRepo       repositories.SchemaRepository
	llmFactory       llm.LLMClientFactory
	workerPool       *llm.WorkerPool
	circuitBreaker   *llm.CircuitBreaker
	getTenantCtx     TenantContextFunc
	logger           *zap.Logger
}

// NewRelationshipEnrichmentService creates a new relationship enrichment service.
func NewRelationshipEnrichmentService(
	relationshipRepo repositories.EntityRelationshipRepository,
	entityRepo repositories.OntologyEntityRepository,
	knowledgeRepo repositories.KnowledgeRepository,
	conversationRepo repositories.ConversationRepository,
	questionService OntologyQuestionService,
	ontologyRepo repositories.OntologyRepository,
	schemaRepo repositories.SchemaRepository,
	llmFactory llm.LLMClientFactory,
	workerPool *llm.WorkerPool,
	circuitBreaker *llm.CircuitBreaker,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) RelationshipEnrichmentService {
	return &relationshipEnrichmentService{
		relationshipRepo: relationshipRepo,
		entityRepo:       entityRepo,
		knowledgeRepo:    knowledgeRepo,
		conversationRepo: conversationRepo,
		questionService:  questionService,
		ontologyRepo:     ontologyRepo,
		schemaRepo:       schemaRepo,
		llmFactory:       llmFactory,
		workerPool:       workerPool,
		circuitBreaker:   circuitBreaker,
		getTenantCtx:     getTenantCtx,
		logger:           logger.Named("relationship-enrichment"),
	}
}

var _ RelationshipEnrichmentService = (*relationshipEnrichmentService)(nil)

// EnrichProject enriches all relationships in a project.
func (s *relationshipEnrichmentService) EnrichProject(ctx context.Context, projectID uuid.UUID, progressCallback dag.ProgressCallback) (*EnrichRelationshipsResult, error) {
	startTime := time.Now()
	result := &EnrichRelationshipsResult{}

	// Get all relationships for the project
	relationships, err := s.relationshipRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get relationships: %w", err)
	}

	if len(relationships) == 0 {
		result.DurationMs = time.Since(startTime).Milliseconds()
		return result, nil
	}

	// Get all entities for context
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get entities: %w", err)
	}

	// Build entity lookup
	entityByID := make(map[uuid.UUID]*models.OntologyEntity)
	for _, e := range entities {
		entityByID[e.ID] = e
	}

	// Fetch project knowledge for role/association context
	var knowledgeFacts []*models.KnowledgeFact
	if s.knowledgeRepo != nil {
		knowledgeFacts, err = s.knowledgeRepo.GetByProject(ctx, projectID)
		if err != nil {
			s.logger.Error("Failed to fetch project knowledge, continuing without it",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Continue without knowledge - don't fail the entire operation
		}
	}

	// Validate relationships before enrichment
	validRelationships := s.validateRelationships(relationships, entityByID)
	if len(validRelationships) < len(relationships) {
		skipped := len(relationships) - len(validRelationships)
		s.logger.Warn("Skipped invalid relationships",
			zap.Int("skipped", skipped),
			zap.Int("total", len(relationships)))
		result.RelationshipsFailed += skipped
	}

	totalRelationships := len(validRelationships)
	if totalRelationships == 0 {
		result.DurationMs = time.Since(startTime).Milliseconds()
		return result, nil
	}

	// Collect unique table names from relationships for column features lookup
	tableNames := s.collectTableNames(validRelationships)

	// Fetch columns with features for all involved tables
	columnFeaturesByKey := make(map[string]*models.ColumnFeatures)
	if s.schemaRepo != nil && len(tableNames) > 0 {
		columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames, false)
		if err != nil {
			s.logger.Error("Failed to fetch columns for relationship enrichment, continuing without column features",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Continue without column features - don't fail the entire operation
		} else {
			// Build lookup by "table.column" key
			for tableName, columns := range columnsByTable {
				for _, col := range columns {
					if features := col.GetColumnFeatures(); features != nil {
						key := tableName + "." + col.ColumnName
						columnFeaturesByKey[key] = features
					}
				}
			}
			s.logger.Debug("Loaded column features for relationship enrichment",
				zap.Int("table_count", len(tableNames)),
				zap.Int("features_count", len(columnFeaturesByKey)))
		}
	}

	// Build work items for parallel processing
	const batchSize = 20
	var workItems []llm.WorkItem[*batchResult]
	for i := 0; i < len(validRelationships); i += batchSize {
		end := i + batchSize
		if end > len(validRelationships) {
			end = len(validRelationships)
		}
		batch := validRelationships[i:end]
		batchID := fmt.Sprintf("batch-%d", i/batchSize)

		// Capture batch for closure
		batchCopy := batch
		workItems = append(workItems, llm.WorkItem[*batchResult]{
			ID: batchID,
			Execute: func(ctx context.Context) (*batchResult, error) {
				return s.enrichBatchInternal(ctx, projectID, batchCopy, entityByID, knowledgeFacts, columnFeaturesByKey)
			},
		})
	}

	// Process all batches with worker pool
	batchResults := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
		if progressCallback != nil {
			// Estimate relationship progress based on batch completion
			relProgress := (completed * len(validRelationships)) / total
			progressCallback(relProgress, len(relationships),
				fmt.Sprintf("Enriching relationships (%d/%d)...", relProgress, len(relationships)))
		}
	})

	// Aggregate results
	for _, r := range batchResults {
		if r.Err != nil {
			s.logger.Error("Batch enrichment failed",
				zap.String("batch_id", r.ID),
				zap.Error(r.Err))
			// Count batch size as failed (result may be nil on error)
			if r.Result != nil {
				result.RelationshipsFailed += r.Result.BatchSize
			}
		} else {
			result.RelationshipsEnriched += r.Result.Enriched
			result.RelationshipsFailed += r.Result.Failed
		}
	}

	// Final progress update with exact counts
	if progressCallback != nil {
		processed := result.RelationshipsEnriched + result.RelationshipsFailed
		msg := fmt.Sprintf("Enriching relationships (%d/%d)...", processed, len(relationships))
		progressCallback(processed, len(relationships), msg)
	}

	result.DurationMs = time.Since(startTime).Milliseconds()

	s.logger.Info("Relationship enrichment complete",
		zap.String("project_id", projectID.String()),
		zap.Int("enriched", result.RelationshipsEnriched),
		zap.Int("failed", result.RelationshipsFailed),
		zap.Int64("duration_ms", result.DurationMs))

	return result, nil
}

// enrichBatchInternal enriches a batch of relationships via LLM with retry logic.
// Each batch acquires its own database connection to avoid concurrent access issues
// when multiple batches run in parallel via the worker pool.
func (s *relationshipEnrichmentService) enrichBatchInternal(
	ctx context.Context,
	projectID uuid.UUID,
	relationships []*models.EntityRelationship,
	entityByID map[uuid.UUID]*models.OntologyEntity,
	knowledgeFacts []*models.KnowledgeFact,
	columnFeaturesByKey map[string]*models.ColumnFeatures,
) (*batchResult, error) {
	result := &batchResult{
		BatchSize: len(relationships),
	}

	// Acquire a fresh database connection for this batch to avoid concurrent access
	// issues when multiple workers share the same context. Each worker goroutine
	// needs its own connection since pgx connections are not safe for concurrent use.
	var batchCtx context.Context
	var cleanup func()
	if s.getTenantCtx != nil {
		var err error
		batchCtx, cleanup, err = s.getTenantCtx(ctx, projectID)
		if err != nil {
			s.logger.Error("Failed to acquire tenant context for batch",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			result.Failed = len(relationships)
			return result, fmt.Errorf("acquire tenant context: %w", err)
		}
		defer cleanup()
	} else {
		// For tests that don't use real database connections
		batchCtx = ctx
	}

	// Check circuit breaker before attempting LLM call
	allowed, err := s.circuitBreaker.Allow()
	if !allowed {
		s.logger.Error("Circuit breaker prevented LLM call",
			zap.String("project_id", projectID.String()),
			zap.Int("batch_size", len(relationships)),
			zap.String("circuit_state", s.circuitBreaker.State().String()),
			zap.Int("consecutive_failures", s.circuitBreaker.ConsecutiveFailures()),
			zap.Error(err))
		// Log the specific relationships that failed
		for _, rel := range relationships {
			s.logRelationshipFailure(batchCtx, rel, "Circuit breaker open", err, uuid.Nil)
		}
		result.Failed = len(relationships)
		return result, err
	}

	// Build prompt with relationship context
	llmClient, err := s.llmFactory.CreateForProject(batchCtx, projectID)
	if err != nil {
		s.logger.Error("Failed to create LLM client",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		result.Failed = len(relationships)
		return result, err
	}

	systemMsg := s.relationshipEnrichmentSystemMessage()
	prompt := s.buildRelationshipEnrichmentPrompt(relationships, entityByID, knowledgeFacts, columnFeaturesByKey)

	// Retry LLM call with exponential backoff
	retryConfig := &retry.Config{
		MaxRetries:   3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
	}

	var llmResult *llm.GenerateResponseResult
	err = retry.Do(batchCtx, retryConfig, func() error {
		var retryErr error
		llmResult, retryErr = llmClient.GenerateResponse(batchCtx, prompt, systemMsg, 0.3, false)
		if retryErr != nil {
			// Classify error to determine if retryable
			classified := llm.ClassifyError(retryErr)
			if classified.Retryable {
				s.logger.Warn("LLM call failed, retrying",
					zap.String("error_type", string(classified.Type)),
					zap.Error(retryErr))
				return retryErr
			}
			// Non-retryable error, fail immediately
			s.logger.Error("LLM call failed with non-retryable error",
				zap.String("error_type", string(classified.Type)),
				zap.Error(retryErr))
			return retryErr
		}
		return nil
	})

	if err != nil {
		// Record failure in circuit breaker
		s.circuitBreaker.RecordFailure()
		s.logger.Error("Circuit breaker recorded failure",
			zap.String("project_id", projectID.String()),
			zap.String("circuit_state", s.circuitBreaker.State().String()),
			zap.Int("consecutive_failures", s.circuitBreaker.ConsecutiveFailures()))
		s.logger.Error("LLM call failed after retries",
			zap.String("project_id", projectID.String()),
			zap.Int("batch_size", len(relationships)),
			zap.Error(err))
		// Log the specific relationships that failed
		// llmResult may have ConversationID even on error (from RecordingClient)
		var convID uuid.UUID
		if llmResult != nil {
			convID = llmResult.ConversationID
		}
		for _, rel := range relationships {
			s.logRelationshipFailure(batchCtx, rel, "LLM call failed", err, convID)
		}
		result.Failed = len(relationships)
		return result, err
	}

	// Record success in circuit breaker
	s.circuitBreaker.RecordSuccess()

	// Parse response (wrapped in object for standardization)
	response, err := llm.ParseJSONResponse[relationshipEnrichmentResponse](llmResult.Content)
	if err != nil {
		s.logger.Error("Failed to parse LLM response",
			zap.String("project_id", projectID.String()),
			zap.Int("batch_size", len(relationships)),
			zap.String("response_preview", truncateString(llmResult.Content, 200)),
			zap.Error(err))
		// Log the specific relationships that failed
		for _, rel := range relationships {
			s.logRelationshipFailure(batchCtx, rel, "Failed to parse LLM response", err, llmResult.ConversationID)
		}
		result.Failed = len(relationships)
		return result, err
	}
	enrichments := response.Relationships

	// Store questions generated during enrichment
	if len(response.Questions) > 0 {
		s.logger.Info("LLM generated questions during relationship enrichment",
			zap.Int("question_count", len(response.Questions)),
			zap.String("project_id", projectID.String()))

		// Get active ontology for question storage
		ontology, err := s.ontologyRepo.GetActive(batchCtx, projectID)
		if err != nil {
			s.logger.Error("failed to get active ontology for question storage", zap.Error(err))
			// Non-fatal: continue even if we can't store questions
		} else if ontology != nil && s.questionService != nil {
			questionInputs := make([]OntologyQuestionInput, len(response.Questions))
			for i, q := range response.Questions {
				questionInputs[i] = OntologyQuestionInput{
					Category: q.Category,
					Priority: q.Priority,
					Question: q.Question,
					Context:  q.Context,
				}
			}
			questionModels := ConvertQuestionInputs(questionInputs, projectID, ontology.ID, nil)
			if len(questionModels) > 0 {
				if err := s.questionService.CreateQuestions(batchCtx, questionModels); err != nil {
					s.logger.Error("failed to store ontology questions from relationship enrichment",
						zap.Int("question_count", len(questionModels)),
						zap.Error(err))
					// Non-fatal: continue even if question storage fails
				} else {
					s.logger.Debug("Stored ontology questions from relationship enrichment",
						zap.Int("question_count", len(questionModels)))
				}
			}
		}
	}

	// Build lookup by ID -> enrichment
	enrichmentByID := make(map[int]relationshipEnrichment)
	maxExpectedID := len(relationships)
	for _, e := range enrichments {
		enrichmentByID[e.ID] = e

		// Detect hallucinated IDs (LLM returned an ID we didn't ask for)
		if e.ID < 1 || e.ID > maxExpectedID {
			s.logger.Error("LLM returned invalid relationship ID",
				zap.String("project_id", projectID.String()),
				zap.Int("hallucinated_id", e.ID),
				zap.Int("max_valid_id", maxExpectedID),
				zap.String("description", e.Description),
				zap.String("association", e.Association),
				zap.String("conversation_id", llmResult.ConversationID.String()))

			// Update conversation status for hallucination
			if s.conversationRepo != nil {
				errorMessage := fmt.Sprintf("hallucination: invalid ID %d (expected 1-%d)", e.ID, maxExpectedID)
				if updateErr := s.conversationRepo.UpdateStatus(batchCtx, llmResult.ConversationID, models.LLMConversationStatusError, errorMessage); updateErr != nil {
					s.logger.Error("Failed to update conversation status for hallucination",
						zap.String("conversation_id", llmResult.ConversationID.String()),
						zap.Error(updateErr))
				}
			}
		}
	}

	// Update each relationship by ID (1-indexed)
	for i, rel := range relationships {
		id := i + 1 // 1-indexed to match prompt
		enrichment, ok := enrichmentByID[id]
		if !ok || enrichment.Description == "" {
			s.logRelationshipFailure(batchCtx, rel, "No enrichment found in LLM response", nil, llmResult.ConversationID)
			result.Failed++
			continue
		}

		// Update both description and association
		if enrichment.Association == "" {
			// If association is missing, just update description (fallback for incomplete LLM responses)
			if err := s.relationshipRepo.UpdateDescription(batchCtx, rel.ID, enrichment.Description); err != nil {
				s.logRelationshipFailure(batchCtx, rel, "Failed to update database", err, llmResult.ConversationID)
				result.Failed++
				continue
			}
		} else {
			if err := s.relationshipRepo.UpdateDescriptionAndAssociation(batchCtx, rel.ID, enrichment.Description, enrichment.Association); err != nil {
				s.logRelationshipFailure(batchCtx, rel, "Failed to update database", err, llmResult.ConversationID)
				result.Failed++
				continue
			}
		}

		result.Enriched++
	}

	return result, nil
}

// relationshipEnrichmentResponse wraps the LLM response for standardization.
type relationshipEnrichmentResponse struct {
	Relationships []relationshipEnrichment            `json:"relationships"`
	Questions     []relationshipOntologyQuestionInput `json:"questions,omitempty"`
}

// relationshipOntologyQuestionInput represents a question generated by the LLM during relationship enrichment.
// These questions identify areas of uncertainty where user clarification would improve accuracy.
type relationshipOntologyQuestionInput struct {
	Category string `json:"category"` // terminology | enumeration | relationship | business_rules | temporal | data_quality
	Priority int    `json:"priority"` // 1=critical | 2=important | 3=nice-to-have
	Question string `json:"question"` // Clear question for domain expert
	Context  string `json:"context"`  // Relevant schema/data context
}

// relationshipEnrichment is the LLM response structure for a single relationship.
// Uses numeric ID to avoid table/column format parsing errors from smaller models.
type relationshipEnrichment struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	Association string `json:"association"`
}

func (s *relationshipEnrichmentService) relationshipEnrichmentSystemMessage() string {
	return `You are a database relationship expert. Your task is to generate clear, business-meaningful descriptions for database relationships that help users understand how entities are connected.

Focus on the business meaning, not technical details. Describe what the relationship represents in terms users would understand.`
}

func (s *relationshipEnrichmentService) buildRelationshipEnrichmentPrompt(
	relationships []*models.EntityRelationship,
	entityByID map[uuid.UUID]*models.OntologyEntity,
	knowledgeFacts []*models.KnowledgeFact,
	columnFeaturesByKey map[string]*models.ColumnFeatures,
) string {
	var sb strings.Builder

	sb.WriteString("# Relationship Description Generation\n\n")
	sb.WriteString("Generate business-meaningful descriptions for each relationship below.\n\n")

	// Include domain knowledge section if available - this provides context for roles like host/visitor
	if len(knowledgeFacts) > 0 {
		sb.WriteString("## Domain Knowledge\n\n")
		sb.WriteString("Use the following domain-specific facts to inform your relationship descriptions, especially for role semantics:\n\n")

		// Group facts by type for better organization
		factsByType := make(map[string][]*models.KnowledgeFact)
		for _, fact := range knowledgeFacts {
			factsByType[fact.FactType] = append(factsByType[fact.FactType], fact)
		}

		// Order of presentation
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

	// Entity context
	sb.WriteString("## Entities\n")
	seenEntities := make(map[uuid.UUID]bool)
	for _, rel := range relationships {
		if !seenEntities[rel.SourceEntityID] {
			if e, ok := entityByID[rel.SourceEntityID]; ok {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Name, e.Description))
				seenEntities[rel.SourceEntityID] = true
			}
		}
		if !seenEntities[rel.TargetEntityID] {
			if e, ok := entityByID[rel.TargetEntityID]; ok {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Name, e.Description))
				seenEntities[rel.TargetEntityID] = true
			}
		}
	}

	// Column context from ColumnFeatures - provides role semantics and descriptions
	columnContextItems := s.buildColumnContext(relationships, columnFeaturesByKey)
	if len(columnContextItems) > 0 {
		sb.WriteString("\n## Column Context\n")
		sb.WriteString("Additional context about the source columns in these relationships:\n\n")
		for _, item := range columnContextItems {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", item.column, item.context))
		}
	}

	// Relationships to describe (with numeric IDs for reliable matching)
	sb.WriteString("\n## Relationships\n")
	sb.WriteString("| ID | Source | Target | Detection | Confidence |\n")
	sb.WriteString("|----|--------|--------|-----------|------------|\n")

	for i, rel := range relationships {
		sourceEntity := ""
		targetEntity := ""
		if e, ok := entityByID[rel.SourceEntityID]; ok {
			sourceEntity = e.Name
		}
		if e, ok := entityByID[rel.TargetEntityID]; ok {
			targetEntity = e.Name
		}

		sb.WriteString(fmt.Sprintf("| %d | %s.%s (%s) | %s.%s (%s) | %s | %.0f%% |\n",
			i+1, // 1-indexed ID
			rel.SourceColumnTable, rel.SourceColumnName, sourceEntity,
			rel.TargetColumnTable, rel.TargetColumnName, targetEntity,
			rel.DetectionMethod, rel.Confidence*100))
	}

	// Instructions
	sb.WriteString("\n## Guidelines\n")
	sb.WriteString("For each relationship, provide:\n")
	sb.WriteString("1. **description**: A 1-2 sentence explanation of the business meaning of the relationship\n")
	sb.WriteString("   - Use entity names (not table names) when referring to what's connected\n")
	sb.WriteString("   - Describe the nature of the relationship (e.g., 'belongs to', 'created by', 'references')\n")
	sb.WriteString("   - Include cardinality hints if clear (e.g., 'each order can have many items')\n")
	sb.WriteString("   - **IMPORTANT**: If column names indicate roles (like host_id, visitor_id, buyer_id, seller_id), ")
	sb.WriteString("use the Domain Knowledge above to describe what those roles mean in business terms.\n")
	sb.WriteString("     For example, if domain knowledge says 'Host is a content creator who receives payments', ")
	sb.WriteString("include that meaning in the description.\n")
	sb.WriteString("2. **association**: A short verb/label (1-2 words) that describes this direction of the relationship\n")
	sb.WriteString("   - Examples: \"placed_by\", \"owns\", \"contains\", \"managed_by\", \"belongs_to\", \"created_by\"\n")
	sb.WriteString("   - For role-based columns (host_id, visitor_id), use the role as the association (e.g., \"as_host\", \"as_visitor\")\n")
	sb.WriteString("   - Use lowercase with underscores for multi-word associations\n")
	sb.WriteString("   - Should read naturally: \"Order [association] User\" (e.g., \"Order placed_by User\")\n")

	// Questions section
	sb.WriteString("\n## Questions for Clarification\n\n")
	sb.WriteString("Additionally, identify any areas of uncertainty where user clarification would improve accuracy.\n")
	sb.WriteString("For each uncertainty, provide:\n")
	sb.WriteString("- **category**: terminology | enumeration | relationship | business_rules | temporal | data_quality\n")
	sb.WriteString("- **priority**: 1 (critical) | 2 (important) | 3 (nice-to-have)\n")
	sb.WriteString("- **question**: A clear question for the domain expert\n")
	sb.WriteString("- **context**: Relevant schema/data context\n\n")
	sb.WriteString("Examples of good questions:\n")
	sb.WriteString("- \"Is users.referrer_id a self-reference to the same users table?\" (relationship, priority 1)\n")
	sb.WriteString("- \"Can a user be both host and visitor in the same transaction?\" (business_rules, priority 2)\n")
	sb.WriteString("- \"What is the relationship between accounts and organizations?\" (relationship, priority 1)\n\n")

	// Response format
	sb.WriteString("\n## Response Format (JSON object)\n")
	sb.WriteString("Return a JSON object with the relationship ID, description, association, and any questions:\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"relationships\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"id\": 1,\n")
	sb.WriteString("      \"description\": \"Links each order to the user who placed it. A user can place many orders.\",\n")
	sb.WriteString("      \"association\": \"placed_by\"\n")
	sb.WriteString("    },\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"id\": 2,\n")
	sb.WriteString("      \"description\": \"...\",\n")
	sb.WriteString("      \"association\": \"...\"\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"questions\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"category\": \"relationship\",\n")
	sb.WriteString("      \"priority\": 1,\n")
	sb.WriteString("      \"question\": \"Is users.referrer_id a self-reference to the same users table?\",\n")
	sb.WriteString("      \"context\": \"Column referrer_id in users table appears to reference another user, but this creates a self-referencing relationship.\"\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// validateRelationships filters out malformed relationships before enrichment.
func (s *relationshipEnrichmentService) validateRelationships(
	relationships []*models.EntityRelationship,
	entityByID map[uuid.UUID]*models.OntologyEntity,
) []*models.EntityRelationship {
	valid := make([]*models.EntityRelationship, 0, len(relationships))

	for _, rel := range relationships {
		// Check required fields
		if rel.SourceColumnTable == "" || rel.SourceColumnName == "" ||
			rel.TargetColumnTable == "" || rel.TargetColumnName == "" {
			s.logger.Error("Relationship missing required fields",
				zap.String("relationship_id", rel.ID.String()),
				zap.String("source_table", rel.SourceColumnTable),
				zap.String("source_column", rel.SourceColumnName),
				zap.String("target_table", rel.TargetColumnTable),
				zap.String("target_column", rel.TargetColumnName))
			continue
		}

		// Check that source and target entities exist
		_, sourceExists := entityByID[rel.SourceEntityID]
		_, targetExists := entityByID[rel.TargetEntityID]
		if !sourceExists || !targetExists {
			s.logger.Error("Relationship references missing entities",
				zap.String("relationship_id", rel.ID.String()),
				zap.String("source_entity_id", rel.SourceEntityID.String()),
				zap.String("target_entity_id", rel.TargetEntityID.String()),
				zap.Bool("source_exists", sourceExists),
				zap.Bool("target_exists", targetExists))
			continue
		}

		valid = append(valid, rel)
	}

	return valid
}

// collectTableNames extracts unique table names from relationships for column lookup.
func (s *relationshipEnrichmentService) collectTableNames(relationships []*models.EntityRelationship) []string {
	tableSet := make(map[string]struct{})
	for _, rel := range relationships {
		if rel.SourceColumnTable != "" {
			tableSet[rel.SourceColumnTable] = struct{}{}
		}
		if rel.TargetColumnTable != "" {
			tableSet[rel.TargetColumnTable] = struct{}{}
		}
	}

	tables := make([]string, 0, len(tableSet))
	for table := range tableSet {
		tables = append(tables, table)
	}
	return tables
}

// columnContextItem represents context information for a column in the prompt.
type columnContextItem struct {
	column  string
	context string
}

// buildColumnContext extracts role and description context from ColumnFeatures for source columns.
// This provides semantic information like "host", "visitor", "payer" roles to the LLM.
func (s *relationshipEnrichmentService) buildColumnContext(
	relationships []*models.EntityRelationship,
	columnFeaturesByKey map[string]*models.ColumnFeatures,
) []columnContextItem {
	if len(columnFeaturesByKey) == 0 {
		return nil
	}

	// Track seen columns to avoid duplicates
	seen := make(map[string]struct{})
	var items []columnContextItem

	for _, rel := range relationships {
		// Build context for source column
		sourceKey := rel.SourceColumnTable + "." + rel.SourceColumnName
		if _, exists := seen[sourceKey]; !exists {
			seen[sourceKey] = struct{}{}
			if features, ok := columnFeaturesByKey[sourceKey]; ok {
				context := s.buildSingleColumnContext(features)
				if context != "" {
					items = append(items, columnContextItem{
						column:  sourceKey,
						context: context,
					})
				}
			}
		}
	}

	return items
}

// buildSingleColumnContext builds a context string from a single column's features.
func (s *relationshipEnrichmentService) buildSingleColumnContext(features *models.ColumnFeatures) string {
	var parts []string

	// Include entity role if available (e.g., "host", "visitor", "payer")
	if features.IdentifierFeatures != nil && features.IdentifierFeatures.EntityReferenced != "" {
		parts = append(parts, fmt.Sprintf("Role: %s", features.IdentifierFeatures.EntityReferenced))
	}

	// Include description if available
	if features.Description != "" {
		parts = append(parts, features.Description)
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, " - ")
}

// logRelationshipFailure logs detailed information about a failed relationship enrichment
// and updates the conversation status in the database.
// conversationID allows correlating with LLM debug logs (zero UUID if not applicable).
func (s *relationshipEnrichmentService) logRelationshipFailure(
	ctx context.Context,
	rel *models.EntityRelationship,
	reason string,
	err error,
	conversationID uuid.UUID,
) {
	fields := []zap.Field{
		zap.String("relationship_id", rel.ID.String()),
		zap.String("source", fmt.Sprintf("%s.%s", rel.SourceColumnTable, rel.SourceColumnName)),
		zap.String("target", fmt.Sprintf("%s.%s", rel.TargetColumnTable, rel.TargetColumnName)),
		zap.String("detection_method", rel.DetectionMethod),
		zap.Float64("confidence", rel.Confidence),
		zap.String("reason", reason),
	}

	if conversationID != uuid.Nil {
		fields = append(fields, zap.String("conversation_id", conversationID.String()))
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
	}

	s.logger.Error("Relationship enrichment failed", fields...)

	// Update conversation status in database if we have a conversation ID
	if conversationID != uuid.Nil && s.conversationRepo != nil {
		errorMessage := reason
		if err != nil {
			errorMessage = fmt.Sprintf("%s: %s", reason, err.Error())
		}
		if updateErr := s.conversationRepo.UpdateStatus(ctx, conversationID, models.LLMConversationStatusError, errorMessage); updateErr != nil {
			s.logger.Error("Failed to update conversation status",
				zap.String("conversation_id", conversationID.String()),
				zap.Error(updateErr))
		}
	}
}

// truncateString truncates a string to a maximum length for logging.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
