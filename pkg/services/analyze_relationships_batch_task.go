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
	"github.com/ekaya-inc/ekaya-engine/pkg/prompts"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// AnalyzeRelationshipsBatchTask runs LLM analysis on a batch of relationship candidates.
// Unlike AnalyzeRelationshipsTask which processes ALL candidates in one request,
// this task processes a subset with minimal schema context for better token efficiency.
type AnalyzeRelationshipsBatchTask struct {
	workqueue.BaseTask
	candidateRepo repositories.RelationshipCandidateRepository
	schemaRepo    repositories.SchemaRepository
	llmFactory    llm.LLMClientFactory
	getTenantCtx  TenantContextFunc
	projectID     uuid.UUID
	workflowID    uuid.UUID
	datasourceID  uuid.UUID
	batchName     string                          // e.g., "users table relationships"
	candidates    []*models.RelationshipCandidate // Pre-loaded candidates for this batch
	logger        *zap.Logger
}

// NewAnalyzeRelationshipsBatchTask creates a new LLM-based relationship analysis task for a batch.
func NewAnalyzeRelationshipsBatchTask(
	candidateRepo repositories.RelationshipCandidateRepository,
	schemaRepo repositories.SchemaRepository,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	datasourceID uuid.UUID,
	batchName string,
	candidates []*models.RelationshipCandidate,
	logger *zap.Logger,
) *AnalyzeRelationshipsBatchTask {
	return &AnalyzeRelationshipsBatchTask{
		BaseTask:      workqueue.NewBaseTask(fmt.Sprintf("Analyze: %s", batchName), true), // LLM task
		candidateRepo: candidateRepo,
		schemaRepo:    schemaRepo,
		llmFactory:    llmFactory,
		getTenantCtx:  getTenantCtx,
		projectID:     projectID,
		workflowID:    workflowID,
		datasourceID:  datasourceID,
		batchName:     batchName,
		candidates:    candidates,
		logger:        logger,
	}
}

// Execute implements workqueue.Task.
// Analyzes the batch of candidates with LLM and applies decisions based on confidence thresholds.
func (t *AnalyzeRelationshipsBatchTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	if len(t.candidates) == 0 {
		t.logger.Info("No candidates in batch", zap.String("batch", t.batchName))
		return nil
	}

	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Add workflow context for conversation recording
	tenantCtx = llm.WithWorkflowID(tenantCtx, t.workflowID)

	t.logger.Info("Starting batch LLM relationship analysis",
		zap.String("batch", t.batchName),
		zap.Int("candidate_count", len(t.candidates)))

	// Collect unique tables referenced by this batch's candidates
	tableIDs := t.collectReferencedTableIDs(tenantCtx)

	// Build minimal table context map (only tables referenced by batch)
	tableContextMap, err := t.buildMinimalTableContextMap(tenantCtx, tableIDs)
	if err != nil {
		return fmt.Errorf("build table context: %w", err)
	}

	// Build candidate context for LLM
	candidateContexts, err := t.buildCandidateContexts(tenantCtx, tableContextMap)
	if err != nil {
		return fmt.Errorf("build candidate contexts: %w", err)
	}

	// Convert to prompt package types
	promptTables := t.convertToPromptTables(tableContextMap)
	promptCandidates := t.convertToPromptCandidates(candidateContexts)

	// Build prompt using prompts package
	prompt := prompts.BuildRelationshipAnalysisPrompt(promptTables, promptCandidates)
	systemMessage := prompts.BuildRelationshipAnalysisSystemMessage()

	// Call LLM
	llmClient, err := t.llmFactory.CreateForProject(tenantCtx, t.projectID)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}
	result, err := llmClient.GenerateResponse(tenantCtx, prompt, systemMessage, 0.2, false)
	if err != nil {
		return fmt.Errorf("LLM analysis failed: %w", err)
	}

	t.logger.Info("Batch LLM analysis completed",
		zap.String("batch", t.batchName),
		zap.Int("tokens", result.TotalTokens))

	// Parse LLM output
	output, err := t.parseAnalysisOutput(result.Content)
	if err != nil {
		return fmt.Errorf("parse LLM output: %w", err)
	}

	// Apply decisions to candidates
	if err := t.applyDecisions(tenantCtx, output.Decisions); err != nil {
		return fmt.Errorf("apply decisions: %w", err)
	}

	// Create new candidates from LLM inferences
	if err := t.createInferredCandidates(tenantCtx, output.NewRelationships, tableContextMap); err != nil {
		return fmt.Errorf("create inferred candidates: %w", err)
	}

	t.logger.Info("Batch relationship analysis completed",
		zap.String("batch", t.batchName),
		zap.Int("decisions_applied", len(output.Decisions)),
		zap.Int("new_candidates", len(output.NewRelationships)))

	return nil
}

// collectReferencedTableIDs returns the set of table IDs referenced by this batch's candidates.
func (t *AnalyzeRelationshipsBatchTask) collectReferencedTableIDs(ctx context.Context) map[uuid.UUID]bool {
	tableIDs := make(map[uuid.UUID]bool)

	for _, c := range t.candidates {
		// Get source column to find its table
		sourceCol, err := t.schemaRepo.GetColumnByID(ctx, t.projectID, c.SourceColumnID)
		if err == nil && sourceCol != nil {
			tableIDs[sourceCol.SchemaTableID] = true
		}

		// Get target column to find its table
		targetCol, err := t.schemaRepo.GetColumnByID(ctx, t.projectID, c.TargetColumnID)
		if err == nil && targetCol != nil {
			tableIDs[targetCol.SchemaTableID] = true
		}
	}

	return tableIDs
}

// buildMinimalTableContextMap builds context only for tables in the provided set.
func (t *AnalyzeRelationshipsBatchTask) buildMinimalTableContextMap(ctx context.Context, tableIDs map[uuid.UUID]bool) (TableContextMap, error) {
	contextMap := make(TableContextMap)

	for tableID := range tableIDs {
		// Get table info
		table, err := t.schemaRepo.GetTableByID(ctx, t.projectID, tableID)
		if err != nil {
			t.logger.Warn("Failed to get table", zap.String("table_id", tableID.String()), zap.Error(err))
			continue
		}
		if table == nil {
			continue
		}

		// Get columns for this table
		columns, err := t.schemaRepo.ListColumnsByTable(ctx, t.projectID, tableID)
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

// buildCandidateContexts builds contexts for the batch's candidates.
func (t *AnalyzeRelationshipsBatchTask) buildCandidateContexts(
	ctx context.Context,
	tableContextMap TableContextMap,
) ([]*CandidateContext, error) {
	contexts := make([]*CandidateContext, 0, len(t.candidates))

	for _, c := range t.candidates {
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

// convertToPromptTables converts internal TableContextMap to prompts package types.
func (t *AnalyzeRelationshipsBatchTask) convertToPromptTables(tableContextMap TableContextMap) []prompts.TableContext {
	tables := make([]prompts.TableContext, 0, len(tableContextMap))

	for tableName, ctx := range tableContextMap {
		// Find primary key column name
		pkColumn := ""
		for _, colCtx := range ctx.Columns {
			if colCtx.Column.IsPrimaryKey {
				pkColumn = colCtx.Column.ColumnName
				break
			}
		}

		// Convert columns
		promptColumns := make([]prompts.ColumnContext, 0, len(ctx.Columns))
		for _, colCtx := range ctx.Columns {
			col := colCtx.Column

			promptColumns = append(promptColumns, prompts.ColumnContext{
				Name:                col.ColumnName,
				DataType:            col.DataType,
				IsNullable:          col.IsNullable,
				NullPercent:         0.0,
				IsPrimaryKey:        col.IsPrimaryKey,
				IsForeignKey:        false,
				ForeignKeyTarget:    "",
				LooksLikeForeignKey: colCtx.LooksLikeForeignKey,
			})
		}

		tables = append(tables, prompts.TableContext{
			Name:     tableName,
			RowCount: ctx.Table.RowCount,
			PKColumn: pkColumn,
			Columns:  promptColumns,
		})
	}

	return tables
}

// convertToPromptCandidates converts internal CandidateContext to prompts package types.
func (t *AnalyzeRelationshipsBatchTask) convertToPromptCandidates(candidates []*CandidateContext) []prompts.CandidateContext {
	promptCandidates := make([]prompts.CandidateContext, 0, len(candidates))

	for _, c := range candidates {
		promptCandidates = append(promptCandidates, prompts.CandidateContext{
			ID:               c.ID,
			SourceTable:      c.SourceTable,
			SourceColumn:     c.SourceColumn,
			SourceColumnType: c.SourceColumnType,
			TargetTable:      c.TargetTable,
			TargetColumn:     c.TargetColumn,
			TargetColumnType: c.TargetColumnType,
			DetectionMethod:  c.DetectionMethod,
			ValueMatchRate:   c.ValueMatchRate,
			Cardinality:      c.Cardinality,
			JoinMatchRate:    c.JoinMatchRate,
			OrphanRate:       c.OrphanRate,
			TargetCoverage:   c.TargetCoverage,
			SourceRowCount:   c.SourceRowCount,
			TargetRowCount:   c.TargetRowCount,
		})
	}

	return promptCandidates
}

func (t *AnalyzeRelationshipsBatchTask) parseAnalysisOutput(content string) (*RelationshipAnalysisOutput, error) {
	// Reuse the parsing logic from AnalyzeRelationshipsTask
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

func (t *AnalyzeRelationshipsBatchTask) applyDecisions(
	ctx context.Context,
	decisions []CandidateDecision,
) error {
	// Build candidate lookup map
	candidateMap := make(map[string]*models.RelationshipCandidate)
	for _, c := range t.candidates {
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
				candidate.Status = models.RelCandidateStatusAccepted
				candidate.IsRequired = false
			} else {
				candidate.Status = models.RelCandidateStatusPending
				candidate.IsRequired = true
			}

		case "reject":
			if decision.Confidence >= HighConfidenceThreshold {
				candidate.Status = models.RelCandidateStatusRejected
				candidate.IsRequired = false
			} else {
				candidate.Status = models.RelCandidateStatusPending
				candidate.IsRequired = true
			}

		case "needs_review":
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
			continue
		}

		t.logger.Debug("Applied LLM decision",
			zap.String("batch", t.batchName),
			zap.String("candidate_id", candidate.ID.String()),
			zap.String("action", decision.Action),
			zap.Float64("confidence", decision.Confidence))
	}

	return nil
}

func (t *AnalyzeRelationshipsBatchTask) createInferredCandidates(
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
			WorkflowID:      t.workflowID,
			DatasourceID:    t.datasourceID,
			SourceColumnID:  sourceCol.ID,
			TargetColumnID:  targetCol.ID,
			DetectionMethod: models.DetectionMethodLLM,
			Confidence:      rel.Confidence,
			LLMReasoning:    &rel.Reasoning,
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
			continue
		}

		t.logger.Debug("Created inferred relationship candidate",
			zap.String("batch", t.batchName),
			zap.String("source", fmt.Sprintf("%s.%s", rel.SourceTable, rel.SourceColumn)),
			zap.String("target", fmt.Sprintf("%s.%s", rel.TargetTable, rel.TargetColumn)))
	}

	return nil
}
