package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/retry"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// ColumnEnrichmentService provides semantic enrichment for database columns.
// It uses LLM to generate descriptions, semantic types, roles, and enum value mappings.
type ColumnEnrichmentService interface {
	// EnrichTable enriches all columns for a single table.
	EnrichTable(ctx context.Context, projectID uuid.UUID, tableName string) error

	// EnrichProject enriches all tables in a project.
	// Returns the enrichment result with success/failure counts.
	// The progressCallback is called after each table to report progress (can be nil).
	EnrichProject(ctx context.Context, projectID uuid.UUID, tableNames []string, progressCallback dag.ProgressCallback) (*EnrichColumnsResult, error)
}

// EnrichColumnsResult holds the result of a column enrichment operation.
type EnrichColumnsResult struct {
	TablesEnriched []string          `json:"tables_enriched"`
	TablesFailed   map[string]string `json:"tables_failed,omitempty"`
	DurationMs     int64             `json:"duration_ms"`
}

type columnEnrichmentService struct {
	ontologyRepo     repositories.OntologyRepository
	entityRepo       repositories.OntologyEntityRepository
	relationshipRepo repositories.EntityRelationshipRepository
	schemaRepo       repositories.SchemaRepository
	conversationRepo repositories.ConversationRepository
	dsSvc            DatasourceService
	adapterFactory   datasource.DatasourceAdapterFactory
	llmFactory       llm.LLMClientFactory
	workerPool       *llm.WorkerPool
	circuitBreaker   *llm.CircuitBreaker
	getTenantCtx     TenantContextFunc
	logger           *zap.Logger
}

// NewColumnEnrichmentService creates a new column enrichment service.
func NewColumnEnrichmentService(
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	schemaRepo repositories.SchemaRepository,
	conversationRepo repositories.ConversationRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	llmFactory llm.LLMClientFactory,
	workerPool *llm.WorkerPool,
	circuitBreaker *llm.CircuitBreaker,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) ColumnEnrichmentService {
	return &columnEnrichmentService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relationshipRepo,
		schemaRepo:       schemaRepo,
		conversationRepo: conversationRepo,
		dsSvc:            dsSvc,
		adapterFactory:   adapterFactory,
		llmFactory:       llmFactory,
		workerPool:       workerPool,
		circuitBreaker:   circuitBreaker,
		getTenantCtx:     getTenantCtx,
		logger:           logger.Named("column-enrichment"),
	}
}

var _ ColumnEnrichmentService = (*columnEnrichmentService)(nil)

// EnrichProject enriches all specified tables (or all entities if empty).
func (s *columnEnrichmentService) EnrichProject(ctx context.Context, projectID uuid.UUID, tableNames []string, progressCallback dag.ProgressCallback) (*EnrichColumnsResult, error) {
	startTime := time.Now()
	result := &EnrichColumnsResult{
		TablesEnriched: []string{},
		TablesFailed:   make(map[string]string),
	}

	// Get entities if no specific tables provided
	if len(tableNames) == 0 {
		entities, err := s.entityRepo.GetByProject(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("get entities: %w", err)
		}
		for _, e := range entities {
			tableNames = append(tableNames, e.PrimaryTable)
		}
	}

	if len(tableNames) == 0 {
		result.DurationMs = time.Since(startTime).Milliseconds()
		return result, nil
	}

	// Build work items for parallel processing
	var workItems []llm.WorkItem[string]
	for _, tableName := range tableNames {
		name := tableName // Capture for closure
		workItems = append(workItems, llm.WorkItem[string]{
			ID: name,
			Execute: func(ctx context.Context) (string, error) {
				// Acquire a fresh database connection for this work item to avoid
				// concurrent access issues when multiple workers share the same context.
				// Each worker goroutine needs its own connection since pgx connections
				// are not safe for concurrent use.
				var workCtx context.Context
				var cleanup func()
				if s.getTenantCtx != nil {
					var err error
					workCtx, cleanup, err = s.getTenantCtx(ctx, projectID)
					if err != nil {
						return name, fmt.Errorf("acquire tenant context: %w", err)
					}
					defer cleanup()
				} else {
					// For tests that don't use real database connections
					workCtx = ctx
				}

				if err := s.EnrichTable(workCtx, projectID, name); err != nil {
					return name, err
				}
				return name, nil
			},
		})
	}

	// Process all tables with worker pool
	tableResults := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
		if progressCallback != nil {
			progressCallback(completed, total,
				fmt.Sprintf("Enriching columns (%d/%d tables)...", completed, total))
		}
	})

	// Aggregate results
	for _, r := range tableResults {
		if r.Err != nil {
			s.logTableFailure(r.ID, "Failed to enrich table", r.Err)
			result.TablesFailed[r.ID] = r.Err.Error()
		} else {
			result.TablesEnriched = append(result.TablesEnriched, r.ID)
		}
	}

	result.DurationMs = time.Since(startTime).Milliseconds()
	return result, nil
}

// EnrichTable enriches all columns for a single table.
func (s *columnEnrichmentService) EnrichTable(ctx context.Context, projectID uuid.UUID, tableName string) error {
	s.logger.Debug("Enriching columns for table",
		zap.String("project_id", projectID.String()),
		zap.String("table", tableName))

	// Get entity for context (business name, domain, description)
	entity, err := s.getEntityByTableName(ctx, projectID, tableName)
	if err != nil {
		return fmt.Errorf("get entity for table %s: %w", tableName, err)
	}

	// Get schema columns for this table
	columns, err := s.getColumnsForTable(ctx, projectID, tableName)
	if err != nil {
		return fmt.Errorf("get columns for table %s: %w", tableName, err)
	}

	if len(columns) == 0 {
		s.logger.Debug("No columns found for table", zap.String("table", tableName))
		return nil
	}

	// Get FK info for this table
	fkInfo, err := s.getForeignKeyInfo(ctx, projectID, tableName)
	if err != nil {
		s.logger.Warn("Failed to get FK info, continuing without",
			zap.String("table", tableName),
			zap.Error(err))
		fkInfo = make(map[string]string)
	}

	// Sample enum values for likely enum columns
	enumSamples, err := s.sampleEnumValues(ctx, projectID, entity, columns)
	if err != nil {
		s.logger.Warn("Failed to sample enum values, continuing without",
			zap.String("table", tableName),
			zap.Error(err))
		enumSamples = make(map[string][]string)
	}

	// Build and send LLM prompt
	enrichments, err := s.enrichColumnsWithLLM(ctx, projectID, entity, columns, fkInfo, enumSamples)
	if err != nil {
		return fmt.Errorf("LLM enrichment failed: %w", err)
	}

	// Convert enrichments to ColumnDetail and save
	columnDetails := s.convertToColumnDetails(enrichments, columns, fkInfo)
	if err := s.ontologyRepo.UpdateColumnDetails(ctx, projectID, tableName, columnDetails); err != nil {
		return fmt.Errorf("save column details: %w", err)
	}

	// Persist sample values for columns with low cardinality (≤50 distinct values)
	if err := s.persistSampleValues(ctx, columns, enumSamples); err != nil {
		s.logger.Warn("Failed to persist sample values, continuing",
			zap.String("table", tableName),
			zap.Error(err))
	}

	s.logger.Info("Enriched columns for table",
		zap.String("table", tableName),
		zap.Int("column_count", len(columnDetails)))

	return nil
}

// getEntityByTableName finds an entity by its primary table name.
func (s *columnEnrichmentService) getEntityByTableName(ctx context.Context, projectID uuid.UUID, tableName string) (*models.OntologyEntity, error) {
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, e := range entities {
		if e.PrimaryTable == tableName {
			return e, nil
		}
	}
	return nil, fmt.Errorf("no entity found for table %s", tableName)
}

// getColumnsForTable retrieves schema columns for a given table name.
func (s *columnEnrichmentService) getColumnsForTable(ctx context.Context, projectID uuid.UUID, tableName string) ([]*models.SchemaColumn, error) {
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, []string{tableName})
	if err != nil {
		return nil, err
	}
	return columnsByTable[tableName], nil
}

// getForeignKeyInfo returns a map of column_name -> target_table for FK columns.
func (s *columnEnrichmentService) getForeignKeyInfo(ctx context.Context, projectID uuid.UUID, tableName string) (map[string]string, error) {
	// Get relationships for this table
	relationships, err := s.relationshipRepo.GetByTables(ctx, projectID, []string{tableName})
	if err != nil {
		return nil, err
	}

	// Build FK map: source column -> target table
	fkInfo := make(map[string]string)
	for _, rel := range relationships {
		// Only include relationships where this table is the source
		if rel.SourceColumnTable == tableName && rel.SourceColumnName != "" {
			fkInfo[rel.SourceColumnName] = rel.TargetColumnTable
		}
	}

	return fkInfo, nil
}

// sampleEnumValues samples distinct values for columns likely to be enums.
func (s *columnEnrichmentService) sampleEnumValues(
	ctx context.Context,
	projectID uuid.UUID,
	entity *models.OntologyEntity,
	columns []*models.SchemaColumn,
) (map[string][]string, error) {
	result := make(map[string][]string)

	// Identify columns likely to be enums
	enumCandidates := s.identifyEnumCandidates(columns)
	if len(enumCandidates) == 0 {
		return result, nil
	}

	// Get datasource for this entity
	ds, err := s.getDatasource(ctx, projectID, entity)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer
	adapter, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, ds.ID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer adapter.Close()

	// Sample each enum candidate
	for _, col := range enumCandidates {
		values, err := adapter.GetDistinctValues(ctx, entity.PrimarySchema, entity.PrimaryTable, col.ColumnName, 50)
		if err != nil {
			s.logger.Debug("Failed to sample values for column, skipping",
				zap.String("column", col.ColumnName),
				zap.Error(err))
			continue
		}
		if len(values) > 0 && len(values) < 50 {
			result[col.ColumnName] = values
		}
	}

	return result, nil
}

// persistSampleValues persists sample values for columns with low cardinality (≤50 distinct values)
// to enable MCP tools to return sample_values without on-demand database queries.
func (s *columnEnrichmentService) persistSampleValues(ctx context.Context, columns []*models.SchemaColumn, enumSamples map[string][]string) error {
	for _, col := range columns {
		if samples, ok := enumSamples[col.ColumnName]; ok && len(samples) > 0 {
			// Update column stats with sample values
			// Pass nil for other stats to preserve existing values
			if err := s.schemaRepo.UpdateColumnStats(ctx, col.ID, nil, nil, nil, nil, samples); err != nil {
				s.logger.Warn("Failed to persist sample values for column",
					zap.String("column", col.ColumnName),
					zap.Error(err))
				// Continue with other columns even if one fails
				continue
			}
			s.logger.Debug("Persisted sample values",
				zap.String("column", col.ColumnName),
				zap.Int("sample_count", len(samples)))
		}
	}
	return nil
}

// identifyEnumCandidates identifies columns likely to contain enum values.
func (s *columnEnrichmentService) identifyEnumCandidates(columns []*models.SchemaColumn) []*models.SchemaColumn {
	var candidates []*models.SchemaColumn

	enumPatterns := []string{"status", "state", "type", "kind", "category", "_code", "level", "tier", "role"}

	for _, col := range columns {
		colNameLower := strings.ToLower(col.ColumnName)
		dataTypeLower := strings.ToLower(col.DataType)

		// Check column name patterns
		for _, pattern := range enumPatterns {
			if strings.Contains(colNameLower, pattern) {
				candidates = append(candidates, col)
				break
			}
		}

		// Also check data type + distinct count (low cardinality text columns)
		if col.DistinctCount != nil && *col.DistinctCount < 50 {
			if strings.Contains(dataTypeLower, "char") ||
				strings.Contains(dataTypeLower, "text") ||
				strings.Contains(dataTypeLower, "varchar") {
				// Avoid duplicates
				found := false
				for _, c := range candidates {
					if c.ColumnName == col.ColumnName {
						found = true
						break
					}
				}
				if !found {
					candidates = append(candidates, col)
				}
			}
		}
	}

	return candidates
}

// getDatasource retrieves the datasource for the entity's primary schema.
func (s *columnEnrichmentService) getDatasource(ctx context.Context, projectID uuid.UUID, entity *models.OntologyEntity) (*models.Datasource, error) {
	// Get all datasources and find the one containing this schema
	datasources, err := s.dsSvc.List(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// For now, just return the first datasource (most projects have one)
	// TODO: Match based on schema if multiple datasources
	if len(datasources) > 0 {
		// If the datasource failed decryption, return an error
		if datasources[0].DecryptionFailed {
			return nil, fmt.Errorf("datasource credentials were encrypted with a different key")
		}
		return datasources[0].Datasource, nil
	}

	return nil, fmt.Errorf("no datasource found for project %s", projectID)
}

// columnEnrichmentResponse wraps the LLM response for standardization.
type columnEnrichmentResponse struct {
	Columns []columnEnrichment `json:"columns"`
}

// columnEnrichment is the LLM response structure for a single column.
type columnEnrichment struct {
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	SemanticType  string             `json:"semantic_type"`
	Role          string             `json:"role"`
	Synonyms      []string           `json:"synonyms,omitempty"`
	EnumValues    []models.EnumValue `json:"enum_values,omitempty"`
	FKAssociation *string            `json:"fk_association"`
}

// enrichColumnsWithLLM uses the LLM to generate semantic metadata for columns.
// Implements chunking for large tables and retry logic for transient failures.
func (s *columnEnrichmentService) enrichColumnsWithLLM(
	ctx context.Context,
	projectID uuid.UUID,
	entity *models.OntologyEntity,
	columns []*models.SchemaColumn,
	fkInfo map[string]string,
	enumSamples map[string][]string,
) ([]columnEnrichment, error) {
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	// Chunk columns if table has many columns to avoid context limits
	const maxColumnsPerChunk = 50
	if len(columns) > maxColumnsPerChunk {
		s.logger.Info("Table has many columns, using chunked enrichment",
			zap.String("table", entity.PrimaryTable),
			zap.Int("total_columns", len(columns)),
			zap.Int("chunk_size", maxColumnsPerChunk))
		return s.enrichColumnsInChunks(ctx, projectID, llmClient, entity, columns, fkInfo, enumSamples, maxColumnsPerChunk)
	}

	// Single batch enrichment with retry
	return s.enrichColumnBatch(ctx, projectID, llmClient, entity, columns, fkInfo, enumSamples)
}

// chunkWorkItem holds metadata for a chunk work item.
type chunkWorkItem struct {
	Index int
	Start int
	End   int
}

// enrichColumnsInChunks processes columns in chunks to avoid context limits.
// For tables with >50 columns, chunks are processed in parallel using the worker pool.
func (s *columnEnrichmentService) enrichColumnsInChunks(
	ctx context.Context,
	projectID uuid.UUID,
	llmClient llm.LLMClient,
	entity *models.OntologyEntity,
	columns []*models.SchemaColumn,
	fkInfo map[string]string,
	enumSamples map[string][]string,
	chunkSize int,
) ([]columnEnrichment, error) {
	// Build work items for each chunk
	var workItems []llm.WorkItem[[]columnEnrichment]
	var chunkMetadata []chunkWorkItem

	for i := 0; i < len(columns); i += chunkSize {
		end := i + chunkSize
		if end > len(columns) {
			end = len(columns)
		}
		chunk := columns[i:end]

		// Filter FK info and enum samples to only include columns in this chunk
		chunkFKInfo := make(map[string]string)
		chunkEnumSamples := make(map[string][]string)
		for _, col := range chunk {
			if target, ok := fkInfo[col.ColumnName]; ok {
				chunkFKInfo[col.ColumnName] = target
			}
			if samples, ok := enumSamples[col.ColumnName]; ok {
				chunkEnumSamples[col.ColumnName] = samples
			}
		}

		// Capture loop variables for closure
		chunkIdx := len(workItems)
		chunkCols := chunk
		chunkFK := chunkFKInfo
		chunkEnum := chunkEnumSamples
		start := i
		chunkEnd := end

		chunkMetadata = append(chunkMetadata, chunkWorkItem{
			Index: chunkIdx,
			Start: start,
			End:   chunkEnd,
		})

		workItems = append(workItems, llm.WorkItem[[]columnEnrichment]{
			ID: fmt.Sprintf("%s-chunk-%d", entity.PrimaryTable, chunkIdx),
			Execute: func(ctx context.Context) ([]columnEnrichment, error) {
				s.logger.Debug("Enriching column chunk",
					zap.String("table", entity.PrimaryTable),
					zap.Int("chunk_start", start),
					zap.Int("chunk_end", chunkEnd),
					zap.Int("total_columns", len(columns)))

				return s.enrichColumnBatch(ctx, projectID, llmClient, entity, chunkCols, chunkFK, chunkEnum)
			},
		})
	}

	// Build ID -> chunk index map for result reassembly
	chunkIndexByID := make(map[string]int)
	for _, meta := range chunkMetadata {
		chunkIndexByID[fmt.Sprintf("%s-chunk-%d", entity.PrimaryTable, meta.Index)] = meta.Index
	}

	// Process chunks in parallel using worker pool
	results := llm.Process(ctx, s.workerPool, workItems, nil)

	// Aggregate results in order (by chunk index)
	// Results come back in completion order, so we need to map them back to chunk index
	resultsByChunk := make(map[int][]columnEnrichment)
	for _, result := range results {
		chunkIdx := chunkIndexByID[result.ID]
		if result.Err != nil {
			meta := chunkMetadata[chunkIdx]
			return nil, fmt.Errorf("chunk %d-%d failed: %w", meta.Start, meta.End, result.Err)
		}
		resultsByChunk[chunkIdx] = result.Result
	}

	// Assemble results in order by chunk index
	var allEnrichments []columnEnrichment
	for i := 0; i < len(workItems); i++ {
		allEnrichments = append(allEnrichments, resultsByChunk[i]...)
	}

	return allEnrichments, nil
}

// enrichColumnBatch enriches a batch of columns with retry logic and circuit breaker protection.
func (s *columnEnrichmentService) enrichColumnBatch(
	ctx context.Context,
	projectID uuid.UUID,
	llmClient llm.LLMClient,
	entity *models.OntologyEntity,
	columns []*models.SchemaColumn,
	fkInfo map[string]string,
	enumSamples map[string][]string,
) ([]columnEnrichment, error) {
	// Check circuit breaker before attempting LLM call
	allowed, err := s.circuitBreaker.Allow()
	if !allowed {
		s.logger.Error("Circuit breaker prevented LLM call",
			zap.String("table", entity.PrimaryTable),
			zap.String("circuit_state", s.circuitBreaker.State().String()),
			zap.Int("consecutive_failures", s.circuitBreaker.ConsecutiveFailures()),
			zap.Error(err))
		return nil, err
	}

	systemMsg := s.columnEnrichmentSystemMessage()
	prompt := s.buildColumnEnrichmentPrompt(entity, columns, fkInfo, enumSamples)

	// Retry LLM call with exponential backoff
	retryConfig := &retry.Config{
		MaxRetries:   3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
	}

	var result *llm.GenerateResponseResult
	err = retry.Do(ctx, retryConfig, func() error {
		var retryErr error
		result, retryErr = llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
		if retryErr != nil {
			// Classify error to determine if retryable
			classified := llm.ClassifyError(retryErr)
			if classified.Retryable {
				s.logger.Warn("LLM call failed, retrying",
					zap.String("table", entity.PrimaryTable),
					zap.Int("column_count", len(columns)),
					zap.String("error_type", string(classified.Type)),
					zap.Error(retryErr))
				return retryErr
			}
			// Non-retryable error, fail immediately
			s.logger.Error("LLM call failed with non-retryable error",
				zap.String("table", entity.PrimaryTable),
				zap.Int("column_count", len(columns)),
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
			zap.String("table", entity.PrimaryTable),
			zap.String("circuit_state", s.circuitBreaker.State().String()),
			zap.Int("consecutive_failures", s.circuitBreaker.ConsecutiveFailures()))
		return nil, fmt.Errorf("LLM call failed after retries: %w", err)
	}

	// Record success in circuit breaker
	s.circuitBreaker.RecordSuccess()

	// Parse response (wrapped in object for standardization)
	response, err := llm.ParseJSONResponse[columnEnrichmentResponse](result.Content)
	if err != nil {
		s.logger.Error("Failed to parse LLM response",
			zap.String("table", entity.PrimaryTable),
			zap.Int("column_count", len(columns)),
			zap.String("response_preview", truncateString(result.Content, 200)),
			zap.String("conversation_id", result.ConversationID.String()),
			zap.Error(err))

		// Update conversation status for parse failure
		if s.conversationRepo != nil {
			errorMessage := fmt.Sprintf("parse_failure: %s", err.Error())
			if updateErr := s.conversationRepo.UpdateStatus(ctx, result.ConversationID, models.LLMConversationStatusError, errorMessage); updateErr != nil {
				s.logger.Warn("Failed to update conversation status",
					zap.String("conversation_id", result.ConversationID.String()),
					zap.Error(updateErr))
			}
		}
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	return response.Columns, nil
}

func (s *columnEnrichmentService) columnEnrichmentSystemMessage() string {
	return `You are a database schema expert. Your task is to analyze database columns and provide semantic metadata that helps AI agents write accurate SQL queries.

Consider the business context of the entity when determining column purposes, semantic types, and roles.`
}

func (s *columnEnrichmentService) buildColumnEnrichmentPrompt(
	entity *models.OntologyEntity,
	columns []*models.SchemaColumn,
	fkInfo map[string]string,
	enumSamples map[string][]string,
) string {
	var sb strings.Builder

	// Entity context
	sb.WriteString(fmt.Sprintf("# Table: %s\n", entity.PrimaryTable))
	sb.WriteString(fmt.Sprintf("Entity: \"%s\"", entity.Name))
	if entity.Description != "" {
		sb.WriteString(fmt.Sprintf(" - %s", entity.Description))
	}
	sb.WriteString("\n\n")

	// Columns to analyze
	sb.WriteString("## Columns to Analyze\n")
	sb.WriteString("| Column | Type | PK | FK | Sample Values |\n")
	sb.WriteString("|--------|------|----|----|---------------|\n")

	for _, col := range columns {
		pk := "no"
		if col.IsPrimaryKey {
			pk = "yes"
		}

		fk := "no"
		if targetTable, ok := fkInfo[col.ColumnName]; ok {
			fk = fmt.Sprintf("yes->%s", targetTable)
		}

		samples := "-"
		if vals, ok := enumSamples[col.ColumnName]; ok && len(vals) > 0 {
			// Show up to 5 sample values
			showVals := vals
			if len(showVals) > 5 {
				showVals = showVals[:5]
			}
			samples = strings.Join(showVals, ", ")
			if len(vals) > 5 {
				samples += ", ..."
			}
		}

		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			col.ColumnName, col.DataType, pk, fk, samples))
	}

	// FK context for role detection
	s.writeFKContext(&sb, fkInfo)

	// Instructions
	sb.WriteString("\n## For Each Column Provide:\n")
	sb.WriteString("1. **description**: 1 sentence explaining business meaning\n")
	sb.WriteString("2. **semantic_type**: identifier, currency_cents, timestamp_utc, status, count, percentage, email, text, boolean_flag, json, etc.\n")
	sb.WriteString("3. **role**: dimension (for grouping/filtering) | measure (for aggregation) | identifier (unique IDs) | attribute (descriptive)\n")
	sb.WriteString("4. **synonyms**: alternative names users might use (optional array)\n")
	sb.WriteString("5. **enum_values**: for status/type/state columns with sampled values:\n")
	sb.WriteString("   - Return as objects: [{\"value\": \"1\", \"label\": \"Started\"}, ...]\n")
	sb.WriteString("   - Infer labels from column context and common patterns\n")
	sb.WriteString("   - For integer enums, infer meaning from column name (e.g., transaction_state [1,2,3] → Started, Ended, Waiting)\n")
	sb.WriteString("   - For string enums, use the value as label if descriptive (e.g., \"active\" → \"Active\")\n")
	sb.WriteString("   - Include description if you can infer the business meaning\n")
	sb.WriteString("6. **fk_association**: for FK columns, what association does this reference represent?\n")
	sb.WriteString("   Examples: \"owner\", \"creator\", \"assignee\", \"payer\", \"payee\", \"host\", \"visitor\"\n")
	sb.WriteString("   Set to null if it's a generic reference with no special association.\n")

	// Response format
	sb.WriteString("\n## Response Format (JSON object)\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"columns\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"name\": \"column_name\",\n")
	sb.WriteString("      \"description\": \"Business meaning of this column\",\n")
	sb.WriteString("      \"semantic_type\": \"status\",\n")
	sb.WriteString("      \"role\": \"dimension\",\n")
	sb.WriteString("      \"synonyms\": [\"alt_name1\", \"alt_name2\"],\n")
	sb.WriteString("      \"enum_values\": [{\"value\": \"active\", \"label\": \"Active\"}, {\"value\": \"pending\", \"label\": \"Pending\"}],\n")
	sb.WriteString("      \"fk_association\": null\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// writeFKContext adds context about FK columns pointing to the same table.
func (s *columnEnrichmentService) writeFKContext(sb *strings.Builder, fkInfo map[string]string) {
	// Group FK columns by target table
	fkByTarget := make(map[string][]string)
	for col, target := range fkInfo {
		fkByTarget[target] = append(fkByTarget[target], col)
	}

	// Find targets with multiple FK columns (need role differentiation)
	var multipleRoles []string
	for target, cols := range fkByTarget {
		if len(cols) > 1 {
			sort.Strings(cols)
			multipleRoles = append(multipleRoles, fmt.Sprintf("%s (%s)", target, strings.Join(cols, ", ")))
		}
	}

	if len(multipleRoles) > 0 {
		sort.Strings(multipleRoles)
		sb.WriteString("\n## FK Role Context\n")
		sb.WriteString("These columns reference the same entity - identify what role each FK represents:\n")
		for _, info := range multipleRoles {
			sb.WriteString(fmt.Sprintf("- %s\n", info))
		}
	}
}

// convertToColumnDetails converts LLM enrichments to ColumnDetail structs.
func (s *columnEnrichmentService) convertToColumnDetails(
	enrichments []columnEnrichment,
	columns []*models.SchemaColumn,
	fkInfo map[string]string,
) []models.ColumnDetail {
	// Build a map for quick lookup
	enrichmentByName := make(map[string]columnEnrichment)
	for _, e := range enrichments {
		enrichmentByName[e.Name] = e
	}

	// Build column details with schema overlay
	details := make([]models.ColumnDetail, 0, len(columns))
	for _, col := range columns {
		detail := models.ColumnDetail{
			Name:         col.ColumnName,
			IsPrimaryKey: col.IsPrimaryKey,
		}

		// Check FK status from fkInfo
		if targetTable, ok := fkInfo[col.ColumnName]; ok {
			detail.IsForeignKey = true
			detail.ForeignTable = targetTable
		}

		// Overlay enrichment data if available
		if enrichment, ok := enrichmentByName[col.ColumnName]; ok {
			detail.Description = enrichment.Description
			detail.SemanticType = enrichment.SemanticType
			detail.Role = enrichment.Role
			detail.Synonyms = enrichment.Synonyms
			detail.EnumValues = enrichment.EnumValues
			if enrichment.FKAssociation != nil {
				detail.FKAssociation = *enrichment.FKAssociation
			}
		}

		details = append(details, detail)
	}

	return details
}

// logTableFailure logs detailed information about a failed table enrichment.
func (s *columnEnrichmentService) logTableFailure(
	tableName string,
	reason string,
	err error,
) {
	fields := []zap.Field{
		zap.String("table", tableName),
		zap.String("reason", reason),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
	}

	s.logger.Error("Table enrichment failed", fields...)
}
