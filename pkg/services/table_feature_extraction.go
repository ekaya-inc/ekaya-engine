package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// TableFeatureExtractionService generates table-level descriptions based on column features.
// This runs after ColumnFeatureExtraction to synthesize column-level analysis into
// meaningful table descriptions that help users understand what each table represents.
//
// Inputs per table:
//   - Table name, schema
//   - All columns with their ColumnFeatures (PKs, FKs, enums, semantic types, purposes)
//   - Declared FK relationships from schema introspection
//   - Row count
//
// Outputs per table (stored in engine_table_metadata):
//   - description: What this table represents
//   - usage_notes: When to use/not use this table
//   - is_ephemeral: Whether it's transient/temp data
type TableFeatureExtractionService interface {
	// ExtractTableFeatures generates descriptions for all selected tables in the datasource.
	// Returns the number of tables processed.
	ExtractTableFeatures(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) (int, error)
}

type tableFeatureExtractionService struct {
	schemaRepo        repositories.SchemaRepository
	tableMetadataRepo repositories.TableMetadataRepository
	llmFactory        llm.LLMClientFactory
	workerPool        *llm.WorkerPool
	getTenantCtx      TenantContextFunc
	logger            *zap.Logger
}

// NewTableFeatureExtractionService creates a table feature extraction service with LLM support.
func NewTableFeatureExtractionService(
	schemaRepo repositories.SchemaRepository,
	tableMetadataRepo repositories.TableMetadataRepository,
	llmFactory llm.LLMClientFactory,
	workerPool *llm.WorkerPool,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) TableFeatureExtractionService {
	return &tableFeatureExtractionService{
		schemaRepo:        schemaRepo,
		tableMetadataRepo: tableMetadataRepo,
		llmFactory:        llmFactory,
		workerPool:        workerPool,
		getTenantCtx:      getTenantCtx,
		logger:            logger.Named("table-feature-extraction"),
	}
}

// tableContext holds all the data needed to analyze a single table.
type tableContext struct {
	Table         *models.SchemaTable
	Columns       []*models.SchemaColumn
	Relationships []*models.RelationshipDetail
}

// ExtractTableFeatures generates descriptions for all selected tables in the datasource.
func (s *tableFeatureExtractionService) ExtractTableFeatures(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	progressCallback dag.ProgressCallback,
) (int, error) {
	s.logger.Info("Starting table feature extraction",
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()))

	// Report initial progress
	if progressCallback != nil {
		progressCallback(0, 1, "Loading table data...")
	}

	// Get all selected tables
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)
	if err != nil {
		return 0, fmt.Errorf("failed to list tables: %w", err)
	}

	if len(tables) == 0 {
		s.logger.Info("No selected tables found")
		if progressCallback != nil {
			progressCallback(1, 1, "No tables to process")
		}
		return 0, nil
	}

	// Get columns with features grouped by table
	columnsByTable, err := s.schemaRepo.GetColumnsWithFeaturesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get columns with features: %w", err)
	}

	// Get relationships for context (using RelationshipDetails for names)
	relationships, err := s.schemaRepo.GetRelationshipDetails(ctx, projectID, datasourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get relationship details: %w", err)
	}

	// Build table contexts
	tableContexts := s.buildTableContexts(tables, columnsByTable, relationships)

	if len(tableContexts) == 0 {
		s.logger.Info("No tables with column features found")
		if progressCallback != nil {
			progressCallback(1, 1, "No tables with column features to process")
		}
		return 0, nil
	}

	s.logger.Info("Processing tables",
		zap.Int("tables_with_features", len(tableContexts)),
		zap.Int("total_selected_tables", len(tables)))

	// Report progress
	if progressCallback != nil {
		progressCallback(0, len(tableContexts), "Analyzing tables...")
	}

	// Build work items - one LLM call per table
	workItems := make([]llm.WorkItem[*tableFeatureResult], 0, len(tableContexts))
	for _, tc := range tableContexts {
		workItems = append(workItems, llm.WorkItem[*tableFeatureResult]{
			ID: tc.Table.TableName,
			Execute: func(ctx context.Context) (*tableFeatureResult, error) {
				return s.analyzeTable(ctx, projectID, tc)
			},
		})
	}

	// Process in parallel with progress updates
	results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
		if progressCallback != nil {
			progressCallback(completed, total, fmt.Sprintf("Analyzing table %d/%d", completed, total))
		}
	})

	// Store results
	successCount := 0
	var failedTables []string
	for _, r := range results {
		if r.Err != nil {
			s.logger.Error("Table feature extraction failed",
				zap.String("table", r.ID),
				zap.Error(r.Err))
			failedTables = append(failedTables, r.ID)
			continue
		}

		// Store the metadata
		if err := s.storeTableMetadata(ctx, projectID, datasourceID, r.Result); err != nil {
			s.logger.Error("Failed to store table metadata",
				zap.String("table", r.ID),
				zap.Error(err))
			failedTables = append(failedTables, r.ID)
			continue
		}

		successCount++
	}

	// Report final progress
	if progressCallback != nil {
		summary := fmt.Sprintf("Analyzed %d tables", successCount)
		if len(failedTables) > 0 {
			summary += fmt.Sprintf(" (%d failed)", len(failedTables))
		}
		progressCallback(len(tableContexts), len(tableContexts), summary)
	}

	s.logger.Info("Table feature extraction complete",
		zap.Int("tables_processed", successCount),
		zap.Int("tables_failed", len(failedTables)))

	return successCount, nil
}

// buildTableContexts creates tableContext objects for tables that have column features.
func (s *tableFeatureExtractionService) buildTableContexts(
	tables []*models.SchemaTable,
	columnsByTable map[string][]*models.SchemaColumn,
	relationships []*models.RelationshipDetail,
) []*tableContext {
	// Build a lookup of relationships by source table name
	relsByTable := make(map[string][]*models.RelationshipDetail)
	for _, rel := range relationships {
		relsByTable[rel.SourceTableName] = append(relsByTable[rel.SourceTableName], rel)
	}

	// Build table contexts for tables with column features
	contexts := make([]*tableContext, 0)
	for _, table := range tables {
		columns, hasFeatures := columnsByTable[table.TableName]
		if !hasFeatures || len(columns) == 0 {
			continue
		}

		contexts = append(contexts, &tableContext{
			Table:         table,
			Columns:       columns,
			Relationships: relsByTable[table.TableName],
		})
	}

	return contexts
}

// tableFeatureResult holds the LLM analysis result for a table.
type tableFeatureResult struct {
	TableName   string
	Description string
	UsageNotes  string
	IsEphemeral bool
}

// analyzeTable sends an LLM request to analyze a single table.
func (s *tableFeatureExtractionService) analyzeTable(
	ctx context.Context,
	projectID uuid.UUID,
	tc *tableContext,
) (*tableFeatureResult, error) {
	// Acquire fresh connection for this analysis to avoid "conn busy" errors
	workCtx := ctx
	if s.getTenantCtx != nil {
		var cleanup func()
		var err error
		workCtx, cleanup, err = s.getTenantCtx(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("acquire tenant context: %w", err)
		}
		defer cleanup()
	}

	// Build the prompt
	prompt := s.buildPrompt(tc)
	systemMsg := s.systemMessage()

	// Get LLM client
	llmClient, err := s.llmFactory.CreateForProject(workCtx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	// Call LLM with low temperature for consistent classification
	result, err := llmClient.GenerateResponse(workCtx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse the response
	return s.parseResponse(tc.Table.TableName, result.Content)
}

func (s *tableFeatureExtractionService) systemMessage() string {
	return `You are a database schema analyst. Your task is to synthesize column-level features into a coherent table description.

Focus on:
1. What business entity or concept this table represents
2. Whether it's transactional (events/actions) vs reference (static lookups) vs logging (audit/history)
3. Key columns and their roles in the table's purpose
4. Any indicators that the table is ephemeral/temporary (session data, caches, queues)

Respond with valid JSON only.`
}

func (s *tableFeatureExtractionService) buildPrompt(tc *tableContext) string {
	var sb strings.Builder

	sb.WriteString("# Table Analysis\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", tc.Table.TableName))
	if tc.Table.SchemaName != "" && tc.Table.SchemaName != "public" {
		sb.WriteString(fmt.Sprintf("**Schema:** %s\n", tc.Table.SchemaName))
	}
	if tc.Table.RowCount != nil {
		sb.WriteString(fmt.Sprintf("**Row count:** %d\n", *tc.Table.RowCount))
	}
	sb.WriteString(fmt.Sprintf("**Column count:** %d\n", len(tc.Columns)))

	// Summarize column features
	sb.WriteString("\n## Column Features Summary\n\n")

	// Group columns by role/purpose
	var pks, fks, timestamps, enums, measures, identifiers, others []*models.SchemaColumn
	for _, col := range tc.Columns {
		features := col.GetColumnFeatures()
		if features == nil {
			others = append(others, col)
			continue
		}

		switch features.Role {
		case models.RolePrimaryKey:
			pks = append(pks, col)
		case models.RoleForeignKey:
			fks = append(fks, col)
		case models.RoleMeasure:
			measures = append(measures, col)
		default:
			switch features.Purpose {
			case models.PurposeTimestamp:
				timestamps = append(timestamps, col)
			case models.PurposeEnum:
				enums = append(enums, col)
			case models.PurposeIdentifier:
				identifiers = append(identifiers, col)
			default:
				others = append(others, col)
			}
		}
	}

	// Write summaries by category
	if len(pks) > 0 {
		sb.WriteString("**Primary Keys:**\n")
		for _, col := range pks {
			s.writeColumnSummary(&sb, col)
		}
	}

	if len(fks) > 0 {
		sb.WriteString("\n**Foreign Keys:**\n")
		for _, col := range fks {
			s.writeColumnSummary(&sb, col)
		}
	}

	if len(timestamps) > 0 {
		sb.WriteString("\n**Timestamps:**\n")
		for _, col := range timestamps {
			s.writeColumnSummary(&sb, col)
		}
	}

	if len(enums) > 0 {
		sb.WriteString("\n**Enums/Status:**\n")
		for _, col := range enums {
			s.writeColumnSummary(&sb, col)
		}
	}

	if len(measures) > 0 {
		sb.WriteString("\n**Measures:**\n")
		for _, col := range measures {
			s.writeColumnSummary(&sb, col)
		}
	}

	if len(identifiers) > 0 {
		sb.WriteString("\n**Identifiers:**\n")
		for _, col := range identifiers {
			s.writeColumnSummary(&sb, col)
		}
	}

	if len(others) > 0 {
		sb.WriteString("\n**Other Columns:**\n")
		for _, col := range others {
			s.writeColumnSummary(&sb, col)
		}
	}

	// Add relationship context
	if len(tc.Relationships) > 0 {
		sb.WriteString("\n## Relationships (Outgoing)\n\n")
		for _, rel := range tc.Relationships {
			sb.WriteString(fmt.Sprintf("- `%s` → `%s.%s`",
				rel.SourceColumnName, rel.TargetTableName, rel.TargetColumnName))
			if rel.Cardinality != "" {
				sb.WriteString(fmt.Sprintf(" [%s]", rel.Cardinality))
			}
			sb.WriteString("\n")
		}
	}

	// Task and response format
	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Based on the column features and relationships, determine:\n")
	sb.WriteString("1. What this table represents (1-2 sentences)\n")
	sb.WriteString("2. Usage notes: when to use or not use this table for queries\n")
	sb.WriteString("3. Whether the table is ephemeral (session data, caches, temporary processing)\n")

	sb.WriteString("\n**Table Type Indicators:**\n")
	sb.WriteString("- **Transactional:** Has created_at/updated_at, event timestamps, references to other entities\n")
	sb.WriteString("- **Reference/Lookup:** Small row count, few FKs, mostly static data (countries, status codes)\n")
	sb.WriteString("- **Logging/Audit:** append-only pattern, high volume, timestamp-indexed\n")
	sb.WriteString("- **Ephemeral:** session_, tmp_, cache_, queue_ patterns; job/task tables with short retention\n")

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"description\": \"Stores user account information including authentication credentials and profile data.\",\n")
	sb.WriteString("  \"usage_notes\": \"Primary table for user data. Join with user_profiles for extended attributes.\",\n")
	sb.WriteString("  \"is_ephemeral\": false\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// writeColumnSummary writes a concise summary of a column and its features.
func (s *tableFeatureExtractionService) writeColumnSummary(sb *strings.Builder, col *models.SchemaColumn) {
	features := col.GetColumnFeatures()
	if features == nil {
		fmt.Fprintf(sb, "- `%s` (%s)\n", col.ColumnName, col.DataType)
		return
	}

	// Start with column name and type
	fmt.Fprintf(sb, "- `%s` (%s)", col.ColumnName, col.DataType)

	// Add semantic type if different from purpose
	if features.SemanticType != "" && features.SemanticType != features.Purpose {
		fmt.Fprintf(sb, " [%s]", features.SemanticType)
	}

	// Add description if available
	if features.Description != "" {
		fmt.Fprintf(sb, ": %s", features.Description)
	}

	// Add FK target if available
	if features.IdentifierFeatures != nil && features.IdentifierFeatures.FKTargetTable != "" {
		fmt.Fprintf(sb, " → %s", features.IdentifierFeatures.FKTargetTable)
	}

	sb.WriteString("\n")
}

// tableAnalysisResponse is the expected JSON response from the LLM.
type tableAnalysisResponse struct {
	Description string `json:"description"`
	UsageNotes  string `json:"usage_notes"`
	IsEphemeral bool   `json:"is_ephemeral"`
}

func (s *tableFeatureExtractionService) parseResponse(tableName, content string) (*tableFeatureResult, error) {
	response, err := llm.ParseJSONResponse[tableAnalysisResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse table analysis response: %w", err)
	}

	return &tableFeatureResult{
		TableName:   tableName,
		Description: response.Description,
		UsageNotes:  response.UsageNotes,
		IsEphemeral: response.IsEphemeral,
	}, nil
}

// storeTableMetadata persists the analysis result to the database.
func (s *tableFeatureExtractionService) storeTableMetadata(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	result *tableFeatureResult,
) error {
	meta := &models.TableMetadata{
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		TableName:    result.TableName,
		Description:  &result.Description,
		UsageNotes:   &result.UsageNotes,
		IsEphemeral:  result.IsEphemeral,
		Source:       "inferred",
	}

	return s.tableMetadataRepo.Upsert(ctx, meta)
}

// Ensure the service implements the dag.TableFeatureExtractionMethods interface.
var _ dag.TableFeatureExtractionMethods = (*tableFeatureExtractionService)(nil)
