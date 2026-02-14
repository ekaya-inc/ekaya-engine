package services

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// OntologyFinalizationService generates domain-level summary from schema analysis.
type OntologyFinalizationService interface {
	// Finalize generates domain description and discovers project conventions from schema.
	Finalize(ctx context.Context, projectID uuid.UUID) error
}

type ontologyFinalizationService struct {
	ontologyRepo       repositories.OntologyRepository
	schemaRepo         repositories.SchemaRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	conversationRepo   repositories.ConversationRepository
	llmFactory         llm.LLMClientFactory
	getTenantCtx       TenantContextFunc
	logger             *zap.Logger
}

// NewOntologyFinalizationService creates a new ontology finalization service.
func NewOntologyFinalizationService(
	ontologyRepo repositories.OntologyRepository,
	schemaRepo repositories.SchemaRepository,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	conversationRepo repositories.ConversationRepository,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) OntologyFinalizationService {
	return &ontologyFinalizationService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: columnMetadataRepo,
		conversationRepo:   conversationRepo,
		llmFactory:         llmFactory,
		getTenantCtx:       getTenantCtx,
		logger:             logger.Named("ontology-finalization"),
	}
}

var _ OntologyFinalizationService = (*ontologyFinalizationService)(nil)

func (s *ontologyFinalizationService) Finalize(ctx context.Context, projectID uuid.UUID) error {
	s.logger.Info("Starting ontology finalization", zap.String("project_id", projectID.String()))

	// Get the active ontology to retrieve datasource ID
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		s.logger.Info("No active ontology found, skipping finalization", zap.String("project_id", projectID.String()))
		return nil
	}

	// Get all tables for the project to build conventions
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, uuid.Nil) // uuid.Nil gets all datasources
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	if len(tables) == 0 {
		s.logger.Info("No tables found, skipping finalization", zap.String("project_id", projectID.String()))
		return nil
	}

	// Get table names for column lookup
	tableNames := make([]string, 0, len(tables))
	for _, t := range tables {
		tableNames = append(tableNames, t.TableName)
	}

	// Get all columns for these tables (needed for ColumnFeatures analysis and convention discovery)
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames, true)
	if err != nil {
		return fmt.Errorf("get columns by tables: %w", err)
	}

	// Collect all column IDs for metadata lookup
	var allColumnIDs []uuid.UUID
	for _, columns := range columnsByTable {
		for _, col := range columns {
			allColumnIDs = append(allColumnIDs, col.ID)
		}
	}

	// Fetch column metadata for all columns
	metadataByColumnID := make(map[uuid.UUID]*models.ColumnMetadata)
	if len(allColumnIDs) > 0 {
		metadataList, err := s.columnMetadataRepo.GetBySchemaColumnIDs(ctx, allColumnIDs)
		if err != nil {
			s.logger.Warn("Failed to fetch column metadata for finalization, continuing without",
				zap.Error(err))
		} else {
			for _, meta := range metadataList {
				metadataByColumnID[meta.SchemaColumnID] = meta
			}
		}
	}

	// Extract insights from ColumnFeatures (now in ColumnMetadata)
	insights := s.extractColumnFeatureInsights(columnsByTable, metadataByColumnID)

	s.logger.Debug("Extracted column feature insights",
		zap.Int("soft_delete_tables", len(insights.softDeleteTables)),
		zap.Int("audit_created_tables", len(insights.auditCreatedTables)),
		zap.Int("audit_updated_tables", len(insights.auditUpdatedTables)),
		zap.Int("monetary_total", insights.monetaryTotal),
		zap.Int("monetary_with_currency", insights.monetaryWithCurrency),
		zap.Int("external_services", len(insights.externalServices)),
	)

	// Generate domain description via LLM based on tables
	description, err := s.generateDomainDescription(ctx, projectID, tables, insights)
	if err != nil {
		return fmt.Errorf("generate domain description: %w", err)
	}

	// Discover project conventions using pre-extracted insights
	conventions, err := s.discoverConventionsWithInsights(ctx, projectID, tables, columnsByTable, insights)
	if err != nil {
		s.logger.Debug("Failed to discover conventions, continuing without", zap.Error(err))
		// Non-fatal - continue without conventions
	}

	// Save to domain_summary JSONB
	domainSummary := &models.DomainSummary{
		Description:     description,
		Domains:         nil, // No domains without entities
		Conventions:     conventions,
		SampleQuestions: nil, // Feature removed, may be reimplemented later
	}

	if err := s.ontologyRepo.UpdateDomainSummary(ctx, projectID, domainSummary); err != nil {
		return fmt.Errorf("update domain summary: %w", err)
	}

	s.logger.Info("Ontology finalization complete",
		zap.String("project_id", projectID.String()),
		zap.Int("table_count", len(tables)),
	)

	return nil
}

// generateDomainDescription calls the LLM to generate a business description based on tables.
func (s *ontologyFinalizationService) generateDomainDescription(
	ctx context.Context,
	projectID uuid.UUID,
	tables []*models.SchemaTable,
	insights *columnFeatureInsights,
) (string, error) {
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("create LLM client: %w", err)
	}

	systemMessage := s.domainDescriptionSystemMessage()
	prompt := s.buildDomainDescriptionPrompt(tables, insights)

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMessage, 0.3, false)
	if err != nil {
		return "", fmt.Errorf("LLM generate response: %w", err)
	}

	s.logger.Debug("LLM response received",
		zap.String("project_id", projectID.String()),
		zap.Int("prompt_tokens", result.PromptTokens),
		zap.Int("completion_tokens", result.CompletionTokens),
	)

	// Parse the response
	parsed, err := s.parseDomainDescriptionResponse(result.Content)
	if err != nil {
		s.logger.Error("Failed to parse domain description response",
			zap.String("project_id", projectID.String()),
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
		return "", fmt.Errorf("parse domain description response: %w", err)
	}

	return parsed.Description, nil
}

func (s *ontologyFinalizationService) domainDescriptionSystemMessage() string {
	return `You are a data modeling expert. Your task is to analyze a database schema and provide a concise business description of what it represents.`
}

func (s *ontologyFinalizationService) buildDomainDescriptionPrompt(
	tables []*models.SchemaTable,
	insights *columnFeatureInsights,
) string {
	var sb strings.Builder

	sb.WriteString("# Database Schema Analysis\n\n")
	sb.WriteString("Based on the following tables and schema patterns, provide a 2-3 sentence business description of what this database represents.\n\n")

	sb.WriteString("## Tables\n\n")
	for _, t := range tables {
		// Note: Description now lives in TableMetadata (engine_ontology_table_metadata)
		sb.WriteString(fmt.Sprintf("- **%s**\n", t.TableName))
	}

	// Include feature-derived insights if available
	if insights != nil {
		insightLines := s.buildFeatureInsightsSection(insights, len(tables))
		if len(insightLines) > 0 {
			sb.WriteString("\n## Technical Patterns Detected\n\n")
			for _, line := range insightLines {
				sb.WriteString(fmt.Sprintf("- %s\n", line))
			}
		}
	}

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("Respond with a JSON object:\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"description\": \"A 2-3 sentence business summary of what this database represents.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// buildFeatureInsightsSection generates human-readable insights from ColumnFeatures analysis.
func (s *ontologyFinalizationService) buildFeatureInsightsSection(
	insights *columnFeatureInsights,
	totalTables int,
) []string {
	var lines []string

	// Soft-delete pattern
	if len(insights.softDeleteTables) > 0 {
		lines = append(lines, fmt.Sprintf("Soft-delete pattern detected on %d tables (column: %s)",
			len(insights.softDeleteTables), insights.softDeleteColumn))
	}

	// Monetary columns with currency pairing
	if insights.monetaryTotal > 0 {
		if insights.monetaryWithCurrency > 0 {
			lines = append(lines, fmt.Sprintf("Monetary columns: %d total, %d paired with currency codes",
				insights.monetaryTotal, insights.monetaryWithCurrency))
		} else {
			lines = append(lines, fmt.Sprintf("Monetary columns: %d total", insights.monetaryTotal))
		}
	}

	// External service integrations
	if len(insights.externalServices) > 0 {
		services := make([]string, 0, len(insights.externalServices))
		for svc, count := range insights.externalServices {
			services = append(services, fmt.Sprintf("%s (%d)", svc, count))
		}
		sort.Strings(services)
		lines = append(lines, fmt.Sprintf("External ID integrations: %s", strings.Join(services, ", ")))
	}

	// Audit column coverage
	createdPct := 0.0
	updatedPct := 0.0
	if totalTables > 0 {
		createdPct = float64(len(insights.auditCreatedTables)) / float64(totalTables) * 100
		updatedPct = float64(len(insights.auditUpdatedTables)) / float64(totalTables) * 100
	}
	if createdPct >= 50 || updatedPct >= 50 {
		lines = append(lines, fmt.Sprintf("Audit timestamps: created_at (%.0f%% coverage), updated_at (%.0f%% coverage)",
			createdPct, updatedPct))
	}

	return lines
}

// domainDescriptionResponse is the expected LLM response structure.
type domainDescriptionResponse struct {
	Description string `json:"description"`
}

func (s *ontologyFinalizationService) parseDomainDescriptionResponse(content string) (*domainDescriptionResponse, error) {
	parsed, err := llm.ParseJSONResponse[domainDescriptionResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse domain description JSON: %w", err)
	}
	return &parsed, nil
}

// ============================================================================
// Convention Discovery
// ============================================================================

// conventionThreshold is the minimum fraction of tables that must have a pattern
// for it to be considered a project-wide convention.
const conventionThreshold = 0.5

// columnFeatureInsights holds aggregated insights from ColumnFeatures analysis.
// These insights are used to enhance convention detection and LLM prompts.
type columnFeatureInsights struct {
	// Soft-delete tables detected via ColumnFeatures
	softDeleteTables []string
	softDeleteColumn string

	// Audit column coverage
	auditCreatedTables []string
	auditUpdatedTables []string

	// Monetary columns with paired currency codes
	monetaryWithCurrency int
	monetaryTotal        int
	monetaryColumnNames  []string       // actual column names where IsMonetary=true
	currencyUnitCounts   map[string]int // CurrencyUnit -> count ("cents": 5, "dollars": 1)

	// External service integrations
	externalServices map[string]int // service -> count of columns
}

// discoverConventionsWithInsights analyzes schema columns to detect project-wide conventions.
// Uses pre-extracted ColumnFeatures insights for more accurate detection.
func (s *ontologyFinalizationService) discoverConventionsWithInsights(
	_ context.Context,
	_ uuid.UUID,
	tables []*models.SchemaTable,
	columnsByTable map[string][]*models.SchemaColumn,
	insights *columnFeatureInsights,
) (*models.ProjectConventions, error) {
	if len(tables) == 0 {
		return nil, nil
	}

	totalTables := len(tables)
	conventions := &models.ProjectConventions{}

	// Detect soft delete convention (prefer ColumnFeatures if available)
	if len(insights.softDeleteTables) > 0 {
		coverage := float64(len(insights.softDeleteTables)) / float64(totalTables)
		if coverage >= conventionThreshold {
			conventions.SoftDelete = &models.SoftDeleteConvention{
				Enabled:    true,
				Column:     insights.softDeleteColumn,
				ColumnType: "timestamp",
				Filter:     fmt.Sprintf("%s IS NULL", insights.softDeleteColumn),
				Coverage:   coverage,
			}
		}
	}
	// Fall back to pattern-based detection if ColumnFeatures didn't find soft delete
	if conventions.SoftDelete == nil {
		conventions.SoftDelete = s.detectSoftDelete(columnsByTable, totalTables)
	}

	// Detect currency convention (prefer ColumnFeatures if available)
	if insights.monetaryTotal > 0 {
		conventions.Currency = s.buildCurrencyFromInsights(insights)
	}
	// Fall back to pattern-based detection if ColumnFeatures didn't find currency convention
	if conventions.Currency == nil {
		conventions.Currency = s.detectCurrency(columnsByTable)
	}

	// Detect audit columns (prefer ColumnFeatures if available)
	if len(insights.auditCreatedTables) > 0 || len(insights.auditUpdatedTables) > 0 {
		conventions.AuditColumns = s.buildAuditColumnsFromInsights(insights, totalTables)
	}
	// Fall back to pattern-based detection if ColumnFeatures didn't find audit columns
	if len(conventions.AuditColumns) == 0 {
		conventions.AuditColumns = s.detectAuditColumns(columnsByTable, totalTables)
	}

	// Return nil if no conventions found
	if conventions.SoftDelete == nil && conventions.Currency == nil && len(conventions.AuditColumns) == 0 {
		return nil, nil
	}

	return conventions, nil
}

// extractColumnFeatureInsights aggregates insights from ColumnMetadataFeatures across all columns.
func (s *ontologyFinalizationService) extractColumnFeatureInsights(
	columnsByTable map[string][]*models.SchemaColumn,
	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata,
) *columnFeatureInsights {
	insights := &columnFeatureInsights{
		externalServices:   make(map[string]int),
		currencyUnitCounts: make(map[string]int),
	}

	softDeleteByColumn := make(map[string][]string) // column name -> table names

	for tableName, columns := range columnsByTable {
		for _, col := range columns {
			// Get column metadata (features are now stored in ColumnMetadata.Features)
			meta := metadataByColumnID[col.ID]
			if meta == nil {
				continue
			}

			// Check for soft-delete pattern using TimestampFeatures
			if tsFeatures := meta.GetTimestampFeatures(); tsFeatures != nil && tsFeatures.IsSoftDelete {
				softDeleteByColumn[col.ColumnName] = append(softDeleteByColumn[col.ColumnName], tableName)
			}

			// Check for audit columns using TimestampFeatures
			if tsFeatures := meta.GetTimestampFeatures(); tsFeatures != nil && tsFeatures.IsAuditField {
				switch tsFeatures.TimestampPurpose {
				case models.TimestampPurposeAuditCreated:
					insights.auditCreatedTables = append(insights.auditCreatedTables, tableName)
				case models.TimestampPurposeAuditUpdated:
					insights.auditUpdatedTables = append(insights.auditUpdatedTables, tableName)
				}
			}

			// Check for monetary columns using MonetaryFeatures
			if monFeatures := meta.GetMonetaryFeatures(); monFeatures != nil && monFeatures.IsMonetary {
				insights.monetaryTotal++
				insights.monetaryColumnNames = append(insights.monetaryColumnNames, col.ColumnName)
				if monFeatures.CurrencyUnit != "" {
					insights.currencyUnitCounts[monFeatures.CurrencyUnit]++
				}
				if monFeatures.PairedCurrencyColumn != "" {
					insights.monetaryWithCurrency++
				}
			}

			// Check for external service identifiers using IdentifierFeatures
			if idFeatures := meta.GetIdentifierFeatures(); idFeatures != nil && idFeatures.ExternalService != "" {
				insights.externalServices[idFeatures.ExternalService]++
			}
		}
	}

	// Find the most common soft-delete column
	var maxCount int
	for colName, tables := range softDeleteByColumn {
		if len(tables) > maxCount {
			maxCount = len(tables)
			insights.softDeleteColumn = colName
			insights.softDeleteTables = tables
		}
	}

	return insights
}

// buildAuditColumnsFromInsights creates AuditColumnInfo from ColumnFeatures insights.
func (s *ontologyFinalizationService) buildAuditColumnsFromInsights(
	insights *columnFeatureInsights,
	totalTables int,
) []models.AuditColumnInfo {
	var result []models.AuditColumnInfo

	if len(insights.auditCreatedTables) > 0 {
		coverage := float64(len(insights.auditCreatedTables)) / float64(totalTables)
		if coverage >= conventionThreshold {
			result = append(result, models.AuditColumnInfo{
				Column:   "created_at",
				Coverage: coverage,
			})
		}
	}

	if len(insights.auditUpdatedTables) > 0 {
		coverage := float64(len(insights.auditUpdatedTables)) / float64(totalTables)
		if coverage >= conventionThreshold {
			result = append(result, models.AuditColumnInfo{
				Column:   "updated_at",
				Coverage: coverage,
			})
		}
	}

	return result
}

// buildCurrencyFromInsights creates a CurrencyConvention from LLM-derived MonetaryFeatures.
func (s *ontologyFinalizationService) buildCurrencyFromInsights(
	insights *columnFeatureInsights,
) *models.CurrencyConvention {
	if insights.monetaryTotal < 2 {
		return nil
	}

	// Find the dominant CurrencyUnit
	if len(insights.currencyUnitCounts) == 0 {
		return nil
	}

	var dominantUnit string
	var maxCount int
	for unit, count := range insights.currencyUnitCounts {
		if count > maxCount {
			maxCount = count
			dominantUnit = unit
		}
	}

	// Map CurrencyUnit to Format/Transform
	var format, transform string
	switch dominantUnit {
	case models.CurrencyUnitCents:
		format = "cents"
		transform = "divide_by_100"
	case models.CurrencyUnitBasisPoints:
		format = "basis_points"
		transform = "divide_by_10000"
	case models.CurrencyUnitDollars:
		format = "dollars"
		transform = "none"
	default:
		// Unknown unit (e.g., a raw currency code like "USD") — treat as dollars
		format = "dollars"
		transform = "none"
	}

	return &models.CurrencyConvention{
		DefaultCurrency: "USD",
		Format:          format,
		ColumnPatterns:  deriveColumnPatterns(insights.monetaryColumnNames),
		Transform:       transform,
	}
}

// deriveColumnPatterns extracts wildcard suffix patterns from column names.
// e.g., ["total_amount", "fee_amount", "unit_price"] → ["*_amount", "*_price"]
func deriveColumnPatterns(columnNames []string) []string {
	patternSet := make(map[string]struct{})
	for _, name := range columnNames {
		idx := strings.LastIndex(name, "_")
		if idx >= 0 {
			patternSet["*"+name[idx:]] = struct{}{}
		}
	}

	patterns := make([]string, 0, len(patternSet))
	for p := range patternSet {
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)
	return patterns
}

// detectSoftDelete looks for soft-delete patterns across tables.
// Supports: deleted_at (timestamp), is_deleted (boolean), deleted (boolean)
func (s *ontologyFinalizationService) detectSoftDelete(
	columnsByTable map[string][]*models.SchemaColumn,
	totalTables int,
) *models.SoftDeleteConvention {
	if totalTables == 0 {
		return nil
	}

	// Track counts for each soft-delete pattern
	type softDeletePattern struct {
		column     string
		columnType string
		filter     string
	}

	patterns := []softDeletePattern{
		{"deleted_at", "timestamp", "deleted_at IS NULL"},
		{"is_deleted", "boolean", "is_deleted = false"},
		{"deleted", "boolean", "deleted = false"},
	}

	patternCounts := make(map[string]int)

	for _, columns := range columnsByTable {
		for _, col := range columns {
			colNameLower := strings.ToLower(col.ColumnName)
			dataTypeLower := strings.ToLower(col.DataType)

			for _, pattern := range patterns {
				if colNameLower == pattern.column {
					// Validate type matches expected pattern
					if pattern.columnType == "timestamp" {
						if strings.Contains(dataTypeLower, "timestamp") && col.IsNullable {
							patternCounts[pattern.column]++
							break
						}
					} else if pattern.columnType == "boolean" {
						if dataTypeLower == "boolean" || dataTypeLower == "bool" {
							patternCounts[pattern.column]++
							break
						}
					}
				}
			}
		}
	}

	// Find the most common pattern that meets threshold
	var bestPattern *softDeletePattern
	var bestCount int

	for _, pattern := range patterns {
		count := patternCounts[pattern.column]
		coverage := float64(count) / float64(totalTables)
		if coverage >= conventionThreshold && count > bestCount {
			p := pattern // copy
			bestPattern = &p
			bestCount = count
		}
	}

	if bestPattern == nil {
		return nil
	}

	return &models.SoftDeleteConvention{
		Enabled:    true,
		Column:     bestPattern.column,
		ColumnType: bestPattern.columnType,
		Filter:     bestPattern.filter,
		Coverage:   float64(bestCount) / float64(totalTables),
	}
}

// detectCurrency looks for currency column patterns and determines format.
func (s *ontologyFinalizationService) detectCurrency(
	columnsByTable map[string][]*models.SchemaColumn,
) *models.CurrencyConvention {
	// Currency column patterns to detect
	currencyPatterns := []string{"_amount", "_price", "_cost", "_total", "_fee"}

	var integerCount, decimalCount int
	matchedPatterns := make(map[string]struct{})

	for _, columns := range columnsByTable {
		for _, col := range columns {
			colNameLower := strings.ToLower(col.ColumnName)
			dataTypeLower := strings.ToLower(col.DataType)

			for _, pattern := range currencyPatterns {
				if strings.HasSuffix(colNameLower, pattern) {
					matchedPatterns[pattern] = struct{}{}

					// Check data type
					if isIntegerType(dataTypeLower) {
						integerCount++
					} else if isDecimalType(dataTypeLower) {
						decimalCount++
					}
					break
				}
			}
		}
	}

	// Need at least some currency columns to report a convention
	if integerCount+decimalCount < 2 {
		return nil
	}

	// Build matched patterns list
	patterns := make([]string, 0, len(matchedPatterns))
	for pattern := range matchedPatterns {
		patterns = append(patterns, "*"+pattern)
	}
	sort.Strings(patterns)

	// Determine format based on dominant type
	var format, transform string
	if integerCount >= decimalCount {
		format = "cents"
		transform = "divide_by_100"
	} else {
		format = "dollars"
		transform = "none"
	}

	return &models.CurrencyConvention{
		DefaultCurrency: "USD", // Default assumption
		Format:          format,
		ColumnPatterns:  patterns,
		Transform:       transform,
	}
}

// detectAuditColumns looks for common audit columns across tables.
func (s *ontologyFinalizationService) detectAuditColumns(
	columnsByTable map[string][]*models.SchemaColumn,
	totalTables int,
) []models.AuditColumnInfo {
	if totalTables == 0 {
		return nil
	}

	// Audit columns to detect
	auditColumnNames := []string{"created_at", "updated_at", "deleted_at", "created_by", "updated_by"}

	columnCounts := make(map[string]int)

	for _, columns := range columnsByTable {
		// Track which audit columns this table has (avoid counting duplicates)
		tableHas := make(map[string]bool)
		for _, col := range columns {
			colNameLower := strings.ToLower(col.ColumnName)
			for _, auditCol := range auditColumnNames {
				if colNameLower == auditCol && !tableHas[auditCol] {
					columnCounts[auditCol]++
					tableHas[auditCol] = true
				}
			}
		}
	}

	// Build result with coverage info, only include columns meeting threshold
	var result []models.AuditColumnInfo
	for _, auditCol := range auditColumnNames {
		count := columnCounts[auditCol]
		coverage := float64(count) / float64(totalTables)
		if coverage >= conventionThreshold {
			result = append(result, models.AuditColumnInfo{
				Column:   auditCol,
				Coverage: coverage,
			})
		}
	}

	return result
}

// isIntegerType returns true if the data type represents an integer.
func isIntegerType(dataType string) bool {
	intTypes := []string{"int", "integer", "bigint", "smallint", "tinyint", "int2", "int4", "int8"}
	for _, t := range intTypes {
		if strings.Contains(dataType, t) {
			return true
		}
	}
	return false
}

// isDecimalType returns true if the data type represents a decimal/numeric.
func isDecimalType(dataType string) bool {
	decimalTypes := []string{"decimal", "numeric", "money", "real", "double", "float"}
	for _, t := range decimalTypes {
		if strings.Contains(dataType, t) {
			return true
		}
	}
	return false
}
