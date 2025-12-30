package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// Confidence thresholds for automatic decision making
const (
	// HighConfidenceThreshold - decisions with ≥85% confidence are auto-applied
	HighConfidenceThreshold = 0.85

	// LowConfidenceThreshold - decisions with <50% confidence are auto-rejected
	LowConfidenceThreshold = 0.50
)

// AnalyzeRelationshipsTask runs LLM analysis on all relationship candidates
// to confirm, reject, or mark them for user review based on schema context,
// naming patterns, join metrics, and data quality signals.
type AnalyzeRelationshipsTask struct {
	workqueue.BaseTask
	candidateRepo repositories.RelationshipCandidateRepository
	schemaRepo    repositories.SchemaRepository
	llmFactory    llm.LLMClientFactory
	getTenantCtx  TenantContextFunc
	projectID     uuid.UUID
	workflowID    uuid.UUID
	datasourceID  uuid.UUID
	logger        *zap.Logger
}

// NewAnalyzeRelationshipsTask creates a new LLM-based relationship analysis task.
func NewAnalyzeRelationshipsTask(
	candidateRepo repositories.RelationshipCandidateRepository,
	schemaRepo repositories.SchemaRepository,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	datasourceID uuid.UUID,
	logger *zap.Logger,
) *AnalyzeRelationshipsTask {
	return &AnalyzeRelationshipsTask{
		BaseTask:      workqueue.NewBaseTask("Analyze relationship candidates", true), // LLM task
		candidateRepo: candidateRepo,
		schemaRepo:    schemaRepo,
		llmFactory:    llmFactory,
		getTenantCtx:  getTenantCtx,
		projectID:     projectID,
		workflowID:    workflowID,
		datasourceID:  datasourceID,
		logger:        logger,
	}
}

// Execute implements workqueue.Task.
// Analyzes all candidates with LLM and applies decisions based on confidence thresholds.
func (t *AnalyzeRelationshipsTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Add workflow context for conversation recording
	tenantCtx = llm.WithWorkflowID(tenantCtx, t.workflowID)

	// Get all candidates for this workflow
	candidates, err := t.candidateRepo.GetByWorkflow(tenantCtx, t.workflowID)
	if err != nil {
		return fmt.Errorf("get candidates: %w", err)
	}

	if len(candidates) == 0 {
		t.logger.Info("No candidates to analyze")
		return nil
	}

	t.logger.Info("Starting LLM relationship analysis",
		zap.Int("candidate_count", len(candidates)))

	// Build schema context
	tables, err := t.schemaRepo.ListTablesByDatasource(tenantCtx, t.projectID, t.datasourceID)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	// Build table context map with columns
	tableContextMap, err := t.buildTableContextMap(tenantCtx, tables)
	if err != nil {
		return fmt.Errorf("build table context: %w", err)
	}

	// Build candidate context for LLM
	candidateContexts, err := t.buildCandidateContexts(tenantCtx, candidates, tableContextMap)
	if err != nil {
		return fmt.Errorf("build candidate contexts: %w", err)
	}

	// Build prompt
	prompt := t.buildAnalysisPrompt(tableContextMap, candidateContexts)
	systemMessage := t.buildSystemMessage()

	// Call LLM
	llmClient, err := t.llmFactory.CreateForProject(tenantCtx, t.projectID)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}
	result, err := llmClient.GenerateResponse(tenantCtx, prompt, systemMessage, 0.2, true) // Low temperature, enable thinking
	if err != nil {
		return fmt.Errorf("LLM analysis failed: %w", err)
	}

	t.logger.Info("LLM analysis completed",
		zap.Int("tokens", result.TotalTokens))

	// Parse LLM output
	output, err := t.parseAnalysisOutput(result.Content)
	if err != nil {
		return fmt.Errorf("parse LLM output: %w", err)
	}

	// Apply decisions to candidates
	if err := t.applyDecisions(tenantCtx, candidates, output.Decisions); err != nil {
		return fmt.Errorf("apply decisions: %w", err)
	}

	// Create new candidates from LLM inferences
	if err := t.createInferredCandidates(tenantCtx, output.NewRelationships, tableContextMap); err != nil {
		return fmt.Errorf("create inferred candidates: %w", err)
	}

	t.logger.Info("Relationship analysis completed",
		zap.Int("decisions_applied", len(output.Decisions)),
		zap.Int("new_candidates", len(output.NewRelationships)))

	return nil
}

// ============================================================================
// Context Building
// ============================================================================

// TableContextMap maps table names to their context information.
type TableContextMap map[string]*TableContext

// TableContext provides full schema context for a table.
type TableContext struct {
	Table   *models.SchemaTable
	Columns []*ColumnContext
}

// ColumnContext provides column details for LLM analysis.
type ColumnContext struct {
	Column         *models.SchemaColumn
	LooksLikeForeignKey bool   // Naming pattern suggests FK (e.g., ends with _id)
	RelatedTableName    string // Inferred from naming pattern
}

func (t *AnalyzeRelationshipsTask) buildTableContextMap(ctx context.Context, tables []*models.SchemaTable) (TableContextMap, error) {
	contextMap := make(TableContextMap)

	for _, table := range tables {
		// Get columns for this table
		columns, err := t.schemaRepo.ListColumnsByTable(ctx, t.projectID, table.ID)
		if err != nil {
			return nil, fmt.Errorf("list columns for table %s: %w", table.TableName, err)
		}

		// Build column contexts with FK hints
		columnContexts := make([]*ColumnContext, len(columns))
		for i, col := range columns {
			colCtx := &ColumnContext{
				Column: col,
			}

			// Check for FK-like naming pattern
			if strings.HasSuffix(col.ColumnName, "_id") {
				colCtx.LooksLikeForeignKey = true
				// Extract potential table name (e.g., "user_id" -> "user" or "users")
				colCtx.RelatedTableName = strings.TrimSuffix(col.ColumnName, "_id")
			}

			columnContexts[i] = colCtx
		}

		contextMap[table.TableName] = &TableContext{
			Table:   table,
			Columns: columnContexts,
		}
	}

	return contextMap, nil
}

// CandidateContext provides candidate details for LLM analysis.
type CandidateContext struct {
	ID               string   `json:"id"`
	SourceTable      string   `json:"source_table"`
	SourceColumn     string   `json:"source_column"`
	SourceColumnType string   `json:"source_column_type"`
	TargetTable      string   `json:"target_table"`
	TargetColumn     string   `json:"target_column"`
	TargetColumnType string   `json:"target_column_type"`
	DetectionMethod  string   `json:"detection_method"`
	ValueMatchRate   *float64 `json:"value_match_rate,omitempty"`
	Cardinality      *string  `json:"cardinality,omitempty"`
	JoinMatchRate    *float64 `json:"join_match_rate,omitempty"`
	OrphanRate       *float64 `json:"orphan_rate,omitempty"`
	TargetCoverage   *float64 `json:"target_coverage,omitempty"`
	SourceRowCount   *int64   `json:"source_row_count,omitempty"`
	TargetRowCount   *int64   `json:"target_row_count,omitempty"`
}

func (t *AnalyzeRelationshipsTask) buildCandidateContexts(
	ctx context.Context,
	candidates []*models.RelationshipCandidate,
	tableContextMap TableContextMap,
) ([]*CandidateContext, error) {
	contexts := make([]*CandidateContext, 0, len(candidates))

	for _, c := range candidates {
		// Get source column info
		sourceCol, err := t.schemaRepo.GetColumnByID(ctx, t.projectID, c.SourceColumnID)
		if err != nil {
			return nil, fmt.Errorf("get source column: %w", err)
		}

		// Get target column info
		targetCol, err := t.schemaRepo.GetColumnByID(ctx, t.projectID, c.TargetColumnID)
		if err != nil {
			return nil, fmt.Errorf("get target column: %w", err)
		}

		// Get table info
		sourceTable, ok := tableContextMap[getTableNameForColumn(sourceCol, tableContextMap)]
		if !ok {
			t.logger.Warn("Source table not found in context map",
				zap.String("column_id", sourceCol.ID.String()))
			continue
		}

		targetTable, ok := tableContextMap[getTableNameForColumn(targetCol, tableContextMap)]
		if !ok {
			t.logger.Warn("Target table not found in context map",
				zap.String("column_id", targetCol.ID.String()))
			continue
		}

		contexts = append(contexts, &CandidateContext{
			ID:               c.ID.String(),
			SourceTable:      sourceTable.Table.TableName,
			SourceColumn:     sourceCol.ColumnName,
			SourceColumnType: sourceCol.DataType,
			TargetTable:      targetTable.Table.TableName,
			TargetColumn:     targetCol.ColumnName,
			TargetColumnType: targetCol.DataType,
			DetectionMethod:  string(c.DetectionMethod),
			ValueMatchRate:   c.ValueMatchRate,
			Cardinality:      c.Cardinality,
			JoinMatchRate:    c.JoinMatchRate,
			OrphanRate:       c.OrphanRate,
			TargetCoverage:   c.TargetCoverage,
			SourceRowCount:   c.SourceRowCount,
			TargetRowCount:   c.TargetRowCount,
		})
	}

	return contexts, nil
}

// Helper to get table name for a column
func getTableNameForColumn(col *models.SchemaColumn, tableMap TableContextMap) string {
	for tableName, ctx := range tableMap {
		if ctx.Table.ID == col.SchemaTableID {
			return tableName
		}
	}
	return ""
}

// ============================================================================
// Prompt Building
// ============================================================================

func (t *AnalyzeRelationshipsTask) buildSystemMessage() string {
	return `You are a database relationship analysis expert. Your task is to review detected relationship candidates and determine which are true foreign key relationships.`
}

func (t *AnalyzeRelationshipsTask) buildAnalysisPrompt(tableContextMap TableContextMap, candidates []*CandidateContext) string {
	var prompt strings.Builder

	prompt.WriteString("# Database Relationship Analysis\n\n")
	prompt.WriteString("Analyze the following relationship candidates and determine which are true foreign key relationships.\n\n")

	// Schema context
	prompt.WriteString("## Database Schema\n\n")
	for tableName, ctx := range tableContextMap {
		prompt.WriteString(fmt.Sprintf("### %s\n", tableName))
		if ctx.Table.RowCount != nil {
			prompt.WriteString(fmt.Sprintf("Row count: %d\n", *ctx.Table.RowCount))
		}
		prompt.WriteString("Columns:\n")
		for _, colCtx := range ctx.Columns {
			col := colCtx.Column
			flags := ""
			if col.IsPrimaryKey {
				flags += " [PK]"
			}
			if colCtx.LooksLikeForeignKey {
				flags += " [looks like FK]"
			}
			prompt.WriteString(fmt.Sprintf("- %s (%s)%s\n", col.ColumnName, col.DataType, flags))
		}
		prompt.WriteString("\n")
	}

	// Candidates to analyze
	prompt.WriteString("## Relationship Candidates\n\n")
	prompt.WriteString("For each candidate, you have the following information:\n\n")
	for i, c := range candidates {
		prompt.WriteString(fmt.Sprintf("### Candidate %d: %s.%s → %s.%s\n",
			i+1, c.SourceTable, c.SourceColumn, c.TargetTable, c.TargetColumn))
		prompt.WriteString(fmt.Sprintf("- **Detection method**: %s\n", c.DetectionMethod))
		prompt.WriteString(fmt.Sprintf("- **Column types**: %s → %s\n", c.SourceColumnType, c.TargetColumnType))

		if c.ValueMatchRate != nil {
			prompt.WriteString(fmt.Sprintf("- **Sample value match rate**: %.1f%%\n", *c.ValueMatchRate*100))
		}

		if c.Cardinality != nil {
			prompt.WriteString(fmt.Sprintf("- **Cardinality**: %s\n", *c.Cardinality))
		}

		if c.JoinMatchRate != nil {
			prompt.WriteString(fmt.Sprintf("- **Join match rate**: %.1f%% (%d source rows)\n",
				*c.JoinMatchRate*100, valueOrZero(c.SourceRowCount)))
		}

		if c.OrphanRate != nil {
			prompt.WriteString(fmt.Sprintf("- **Orphan rate**: %.1f%% (source rows with no match)\n",
				*c.OrphanRate*100))
		}

		if c.TargetCoverage != nil {
			prompt.WriteString(fmt.Sprintf("- **Target coverage**: %.1f%% (of %d target rows)\n",
				*c.TargetCoverage*100, valueOrZero(c.TargetRowCount)))
		}

		prompt.WriteString("\n")
	}

	// Analysis guidelines
	prompt.WriteString("## Analysis Guidelines\n\n")
	prompt.WriteString("**Strong signals for CONFIRM**:\n")
	prompt.WriteString("- Cardinality is N:1 (many source rows → one target row, typical FK pattern)\n")
	prompt.WriteString("- Column naming follows FK convention (e.g., user_id → users.id)\n")
	prompt.WriteString("- High join match rate (>70%) and low orphan rate (<30%)\n")
	prompt.WriteString("- Target column is a primary key\n")
	prompt.WriteString("- Type match is exact (uuid→uuid, int→int, text→text)\n\n")

	prompt.WriteString("**Strong signals for REJECT**:\n")
	prompt.WriteString("- High orphan rate (>50%) suggests weak or coincidental relationship\n")
	prompt.WriteString("- Cardinality is N:M without clear junction table semantics\n")
	prompt.WriteString("- Column names suggest different domains (e.g., order_total → user_id)\n")
	prompt.WriteString("- Neither column is a key (both are regular data columns)\n\n")

	prompt.WriteString("**Mark as NEEDS_REVIEW when**:\n")
	prompt.WriteString("- Uncertain about business meaning (e.g., ambiguous column names)\n")
	prompt.WriteString("- Moderate orphan rate (30-50%) - could be data quality issue or optional FK\n")
	prompt.WriteString("- Cardinality unexpected for naming pattern\n\n")

	prompt.WriteString("## Interpretation Guide\n")
	prompt.WriteString("- **Orphan Rate**: Percentage of source rows that don't match any target row\n")
	prompt.WriteString("  - 0-10%: Normal for optional FKs or data in transition\n")
	prompt.WriteString("  - 10-30%: Warning sign, but may still be valid FK with data quality issues\n")
	prompt.WriteString("  - >50%: Likely not a real FK relationship\n")
	prompt.WriteString("- **Orphan Rate vs Null Rate**: Orphans are non-null values that don't match; different from NULL FKs\n")
	prompt.WriteString("- **Target Coverage**: Low coverage means few target rows are actually referenced (may indicate unused/stale data)\n\n")

	prompt.WriteString("## Output Format\n\n")
	prompt.WriteString("Respond in JSON with:\n")
	prompt.WriteString("- `decisions`: Array of decisions for each candidate\n")
	prompt.WriteString("  - `candidate_id`: The candidate ID from above\n")
	prompt.WriteString("  - `action`: One of \"confirm\", \"reject\", \"needs_review\"\n")
	prompt.WriteString("  - `confidence`: 0.0-1.0 (how confident you are in this decision)\n")
	prompt.WriteString("  - `reasoning`: Brief explanation (1-2 sentences)\n")
	prompt.WriteString("- `new_relationships`: Array of additional relationships you infer from schema (may be empty)\n")
	prompt.WriteString("  - `source_table`, `source_column`, `target_table`, `target_column`\n")
	prompt.WriteString("  - `confidence`: 0.0-1.0\n")
	prompt.WriteString("  - `reasoning`: Why you think this is a relationship\n\n")

	prompt.WriteString("Example:\n")
	prompt.WriteString("```json\n")
	prompt.WriteString(`{
  "decisions": [
    {
      "candidate_id": "abc-123",
      "action": "confirm",
      "confidence": 0.95,
      "reasoning": "Clear FK pattern: orders.user_id → users.id. N:1 cardinality with 98% match rate confirms strong relationship."
    }
  ],
  "new_relationships": [
    {
      "source_table": "order_items",
      "source_column": "product_id",
      "target_table": "products",
      "target_column": "id",
      "confidence": 0.85,
      "reasoning": "Standard FK naming pattern not detected by value matching. Column name clearly indicates product reference."
    }
  ]
}
`)
	prompt.WriteString("```\n\n")

	prompt.WriteString("Return ONLY the JSON, no additional text.\n")

	return prompt.String()
}

func valueOrZero(ptr *int64) int64 {
	if ptr != nil {
		return *ptr
	}
	return 0
}

// ============================================================================
// Output Parsing
// ============================================================================

// RelationshipAnalysisOutput is the structured output from the LLM.
type RelationshipAnalysisOutput struct {
	Decisions        []CandidateDecision    `json:"decisions"`
	NewRelationships []InferredRelationship `json:"new_relationships"`
}

// CandidateDecision represents the LLM's decision on a candidate.
type CandidateDecision struct {
	CandidateID string  `json:"candidate_id"`
	Action      string  `json:"action"`     // "confirm", "reject", "needs_review"
	Confidence  float64 `json:"confidence"` // 0.0-1.0
	Reasoning   string  `json:"reasoning"`
}

// InferredRelationship represents a new relationship inferred by the LLM.
type InferredRelationship struct {
	SourceTable  string  `json:"source_table"`
	SourceColumn string  `json:"source_column"`
	TargetTable  string  `json:"target_table"`
	TargetColumn string  `json:"target_column"`
	Confidence   float64 `json:"confidence"`
	Reasoning    string  `json:"reasoning"`
}

func (t *AnalyzeRelationshipsTask) parseAnalysisOutput(content string) (*RelationshipAnalysisOutput, error) {
	// Strip markdown code fences if present
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var output RelationshipAnalysisOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, fmt.Errorf("unmarshal LLM output: %w", err)
	}

	return &output, nil
}

// ============================================================================
// Decision Application
// ============================================================================

func (t *AnalyzeRelationshipsTask) applyDecisions(
	ctx context.Context,
	candidates []*models.RelationshipCandidate,
	decisions []CandidateDecision,
) error {
	// Build candidate lookup map
	candidateMap := make(map[string]*models.RelationshipCandidate)
	for _, c := range candidates {
		candidateMap[c.ID.String()] = c
	}

	// Apply each decision
	for _, decision := range decisions {
		candidate, ok := candidateMap[decision.CandidateID]
		if !ok {
			t.logger.Warn("Decision for unknown candidate",
				zap.String("candidate_id", decision.CandidateID))
			continue
		}

		// Update candidate with LLM analysis
		candidate.Confidence = decision.Confidence
		reasoning := decision.Reasoning
		candidate.LLMReasoning = &reasoning

		// Upgrade detection method to include LLM
		if candidate.DetectionMethod != models.DetectionMethodLLM {
			if candidate.DetectionMethod == models.DetectionMethodValueMatch ||
				candidate.DetectionMethod == models.DetectionMethodNameInference {
				candidate.DetectionMethod = models.DetectionMethodHybrid
			}
		}

		// Apply status based on action and confidence
		switch decision.Action {
		case "confirm":
			if decision.Confidence >= HighConfidenceThreshold {
				// Auto-accept with high confidence
				candidate.Status = models.RelCandidateStatusAccepted
				candidate.IsRequired = false
			} else {
				// Needs user review
				candidate.Status = models.RelCandidateStatusPending
				candidate.IsRequired = true
			}

		case "reject":
			if decision.Confidence >= HighConfidenceThreshold {
				// Auto-reject with high confidence
				candidate.Status = models.RelCandidateStatusRejected
				candidate.IsRequired = false
			} else {
				// Needs user confirmation of rejection
				candidate.Status = models.RelCandidateStatusPending
				candidate.IsRequired = true
			}

		case "needs_review":
			// Always requires user review
			candidate.Status = models.RelCandidateStatusPending
			candidate.IsRequired = true

		default:
			t.logger.Warn("Unknown action in decision",
				zap.String("action", decision.Action),
				zap.String("candidate_id", decision.CandidateID))
			continue
		}

		// Update candidate in database
		if err := t.candidateRepo.Update(ctx, candidate); err != nil {
			t.logger.Error("Failed to update candidate",
				zap.String("candidate_id", candidate.ID.String()),
				zap.Error(err))
			// Continue with other candidates
			continue
		}

		t.logger.Info("Applied LLM decision",
			zap.String("candidate_id", candidate.ID.String()),
			zap.String("action", decision.Action),
			zap.Float64("confidence", decision.Confidence),
			zap.String("status", string(candidate.Status)),
			zap.Bool("requires_review", candidate.IsRequired))
	}

	return nil
}

func (t *AnalyzeRelationshipsTask) createInferredCandidates(
	ctx context.Context,
	newRelationships []InferredRelationship,
	tableContextMap TableContextMap,
) error {
	for _, rel := range newRelationships {
		// Get source table and column
		sourceTableCtx, ok := tableContextMap[rel.SourceTable]
		if !ok {
			t.logger.Warn("Source table not found for inferred relationship",
				zap.String("table", rel.SourceTable))
			continue
		}

		var sourceCol *models.SchemaColumn
		for _, colCtx := range sourceTableCtx.Columns {
			if colCtx.Column.ColumnName == rel.SourceColumn {
				sourceCol = colCtx.Column
				break
			}
		}
		if sourceCol == nil {
			t.logger.Warn("Source column not found for inferred relationship",
				zap.String("column", rel.SourceColumn))
			continue
		}

		// Get target table and column
		targetTableCtx, ok := tableContextMap[rel.TargetTable]
		if !ok {
			t.logger.Warn("Target table not found for inferred relationship",
				zap.String("table", rel.TargetTable))
			continue
		}

		var targetCol *models.SchemaColumn
		for _, colCtx := range targetTableCtx.Columns {
			if colCtx.Column.ColumnName == rel.TargetColumn {
				targetCol = colCtx.Column
				break
			}
		}
		if targetCol == nil {
			t.logger.Warn("Target column not found for inferred relationship",
				zap.String("column", rel.TargetColumn))
			continue
		}

		// Create new candidate
		candidate := &models.RelationshipCandidate{
			WorkflowID:       t.workflowID,
			DatasourceID:     t.datasourceID,
			SourceColumnID:   sourceCol.ID,
			TargetColumnID:   targetCol.ID,
			DetectionMethod:  models.DetectionMethodLLM,
			Confidence:       rel.Confidence,
			LLMReasoning:     &rel.Reasoning,
			ValueMatchRate:   nil, // Not determined by value matching
			NameSimilarity:   nil, // Not determined by name inference
			Cardinality:      nil, // Not yet tested
			JoinMatchRate:    nil,
			OrphanRate:       nil,
			TargetCoverage:   nil,
			SourceRowCount:   nil,
			TargetRowCount:   nil,
			MatchedRows:      nil,
			OrphanRows:       nil,
		}

		// Apply status based on confidence
		if rel.Confidence >= HighConfidenceThreshold {
			candidate.Status = models.RelCandidateStatusAccepted
			candidate.IsRequired = false
		} else {
			candidate.Status = models.RelCandidateStatusPending
			candidate.IsRequired = true
		}

		// Create candidate
		if err := t.candidateRepo.Create(ctx, candidate); err != nil {
			t.logger.Error("Failed to create inferred candidate",
				zap.String("source", fmt.Sprintf("%s.%s", rel.SourceTable, rel.SourceColumn)),
				zap.String("target", fmt.Sprintf("%s.%s", rel.TargetTable, rel.TargetColumn)),
				zap.Error(err))
			// Continue with other candidates
			continue
		}

		t.logger.Info("Created inferred relationship candidate",
			zap.String("source", fmt.Sprintf("%s.%s", rel.SourceTable, rel.SourceColumn)),
			zap.String("target", fmt.Sprintf("%s.%s", rel.TargetTable, rel.TargetColumn)),
			zap.Float64("confidence", rel.Confidence))
	}

	return nil
}
