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

// OntologyFinalizationService generates domain-level summary after entity and relationship extraction.
type OntologyFinalizationService interface {
	// Finalize generates domain description and aggregates primary domains from entities.
	Finalize(ctx context.Context, projectID uuid.UUID) error
}

type ontologyFinalizationService struct {
	ontologyRepo     repositories.OntologyRepository
	entityRepo       repositories.OntologyEntityRepository
	relationshipRepo repositories.EntityRelationshipRepository
	schemaRepo       repositories.SchemaRepository
	conversationRepo repositories.ConversationRepository
	llmFactory       llm.LLMClientFactory
	getTenantCtx     TenantContextFunc
	logger           *zap.Logger
}

// NewOntologyFinalizationService creates a new ontology finalization service.
func NewOntologyFinalizationService(
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	schemaRepo repositories.SchemaRepository,
	conversationRepo repositories.ConversationRepository,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) OntologyFinalizationService {
	return &ontologyFinalizationService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relationshipRepo,
		schemaRepo:       schemaRepo,
		conversationRepo: conversationRepo,
		llmFactory:       llmFactory,
		getTenantCtx:     getTenantCtx,
		logger:           logger.Named("ontology-finalization"),
	}
}

var _ OntologyFinalizationService = (*ontologyFinalizationService)(nil)

func (s *ontologyFinalizationService) Finalize(ctx context.Context, projectID uuid.UUID) error {
	s.logger.Info("Starting ontology finalization", zap.String("project_id", projectID.String()))

	// Get all entities (with domains populated from entity extraction)
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get entities: %w", err)
	}

	if len(entities) == 0 {
		s.logger.Info("No entities found, skipping finalization", zap.String("project_id", projectID.String()))
		return nil
	}

	// Get all relationships
	relationships, err := s.relationshipRepo.GetByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get relationships: %w", err)
	}

	// Get table names from entities for column lookup
	tableNames := make([]string, 0, len(entities))
	for _, e := range entities {
		tableNames = append(tableNames, e.PrimaryTable)
	}

	// Get all columns for these tables (needed for ColumnFeatures analysis)
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames, true)
	if err != nil {
		return fmt.Errorf("get columns by tables: %w", err)
	}

	// Extract insights from ColumnFeatures
	insights := s.extractColumnFeatureInsights(columnsByTable)

	s.logger.Debug("Extracted column feature insights",
		zap.Int("soft_delete_tables", len(insights.softDeleteTables)),
		zap.Int("audit_created_tables", len(insights.auditCreatedTables)),
		zap.Int("audit_updated_tables", len(insights.auditUpdatedTables)),
		zap.Int("monetary_total", insights.monetaryTotal),
		zap.Int("monetary_with_currency", insights.monetaryWithCurrency),
		zap.Int("external_services", len(insights.externalServices)),
	)

	// Aggregate unique domains from entity.Domain fields
	primaryDomains := s.aggregateUniqueDomains(entities)

	// Build entity lookups for relationship display and graph building
	entityNameByID := make(map[uuid.UUID]string, len(entities))
	entityByID := make(map[uuid.UUID]*models.OntologyEntity, len(entities))
	for _, e := range entities {
		entityNameByID[e.ID] = e.Name
		entityByID[e.ID] = e
	}

	// Generate domain description via LLM (include ColumnFeature insights for richer context)
	description, err := s.generateDomainDescription(ctx, projectID, entities, relationships, entityNameByID, insights)
	if err != nil {
		return fmt.Errorf("generate domain description: %w", err)
	}

	// Discover project conventions using pre-extracted insights
	conventions, err := s.discoverConventionsWithInsights(ctx, projectID, entities, columnsByTable, insights)
	if err != nil {
		s.logger.Debug("Failed to discover conventions, continuing without", zap.Error(err))
		// Non-fatal - continue without conventions
	}

	// Build relationship graph from confirmed/pending relationships
	var relationshipGraph []models.RelationshipEdge
	for _, rel := range relationships {
		if rel.Status == models.RelationshipStatusRejected {
			continue
		}
		source := entityByID[rel.SourceEntityID]
		target := entityByID[rel.TargetEntityID]
		if source != nil && target != nil {
			edge := models.RelationshipEdge{
				From: source.Name,
				To:   target.Name,
			}
			if rel.Description != nil && *rel.Description != "" {
				edge.Label = *rel.Description
			}
			relationshipGraph = append(relationshipGraph, edge)
		}
	}

	// Build and save entity summaries
	entitySummaries, err := s.buildEntitySummaries(ctx, projectID, entities, relationships, entityByID)
	if err != nil {
		return fmt.Errorf("build entity summaries: %w", err)
	}

	if err := s.ontologyRepo.UpdateEntitySummaries(ctx, projectID, entitySummaries); err != nil {
		return fmt.Errorf("update entity summaries: %w", err)
	}

	// Save to domain_summary JSONB
	domainSummary := &models.DomainSummary{
		Description:       description,
		Domains:           primaryDomains,
		Conventions:       conventions,
		RelationshipGraph: relationshipGraph,
		SampleQuestions:   nil, // Feature removed, may be reimplemented later
	}

	if err := s.ontologyRepo.UpdateDomainSummary(ctx, projectID, domainSummary); err != nil {
		return fmt.Errorf("update domain summary: %w", err)
	}

	s.logger.Info("Ontology finalization complete",
		zap.String("project_id", projectID.String()),
		zap.Int("entity_count", len(entities)),
		zap.Int("domain_count", len(primaryDomains)),
	)

	return nil
}

// aggregateUniqueDomains collects unique domain values from entities.
// Returns domains sorted alphabetically for deterministic output.
func (s *ontologyFinalizationService) aggregateUniqueDomains(entities []*models.OntologyEntity) []string {
	domainSet := make(map[string]struct{})
	for _, e := range entities {
		if e.Domain != "" {
			domainSet[e.Domain] = struct{}{}
		}
	}

	domains := make([]string, 0, len(domainSet))
	for domain := range domainSet {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	return domains
}

// buildEntitySummaries creates entity summaries from entities, their key columns, aliases, and relationships.
func (s *ontologyFinalizationService) buildEntitySummaries(
	ctx context.Context,
	projectID uuid.UUID,
	entities []*models.OntologyEntity,
	relationships []*models.EntityRelationship,
	entityByID map[uuid.UUID]*models.OntologyEntity,
) (map[string]*models.EntitySummary, error) {
	// Get all aliases by entity ID
	aliasesByEntity, err := s.entityRepo.GetAllAliasesByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get aliases: %w", err)
	}

	// Get all key columns by entity ID
	keyColumnsByEntity, err := s.entityRepo.GetAllKeyColumnsByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get key columns: %w", err)
	}

	// Get table names for column count lookup
	tableNames := make([]string, 0, len(entities))
	for _, e := range entities {
		tableNames = append(tableNames, e.PrimaryTable)
	}

	// Get column counts per table
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames, true)
	if err != nil {
		return nil, fmt.Errorf("get columns by tables: %w", err)
	}

	// Build relationship targets by source entity ID
	relationshipTargets := make(map[uuid.UUID][]string)
	for _, rel := range relationships {
		if rel.Status == models.RelationshipStatusRejected {
			continue
		}
		targetEntity := entityByID[rel.TargetEntityID]
		if targetEntity != nil {
			relationshipTargets[rel.SourceEntityID] = append(relationshipTargets[rel.SourceEntityID], targetEntity.Name)
		}
	}

	// Build entity summaries
	summaries := make(map[string]*models.EntitySummary, len(entities))
	for _, e := range entities {
		// Get aliases as synonyms
		var synonyms []string
		if aliases, ok := aliasesByEntity[e.ID]; ok {
			for _, alias := range aliases {
				synonyms = append(synonyms, alias.Alias)
			}
		}

		// Get key columns
		var keyColumns []models.KeyColumn
		if kcs, ok := keyColumnsByEntity[e.ID]; ok {
			for _, kc := range kcs {
				keyColumns = append(keyColumns, models.KeyColumn{
					Name:     kc.ColumnName,
					Synonyms: kc.Synonyms,
				})
			}
		}

		// Get column count
		columnCount := 0
		if cols, ok := columnsByTable[e.PrimaryTable]; ok {
			columnCount = len(cols)
		}

		// Get relationship targets
		var rels []string
		if targets, ok := relationshipTargets[e.ID]; ok {
			rels = targets
		}

		summaries[e.Name] = &models.EntitySummary{
			TableName:     e.PrimaryTable,
			BusinessName:  e.Name,
			Description:   e.Description,
			Domain:        e.Domain,
			Synonyms:      synonyms,
			KeyColumns:    keyColumns,
			ColumnCount:   columnCount,
			Relationships: rels,
		}
	}

	return summaries, nil
}

// generateDomainDescription calls the LLM to generate a business description.
func (s *ontologyFinalizationService) generateDomainDescription(
	ctx context.Context,
	projectID uuid.UUID,
	entities []*models.OntologyEntity,
	relationships []*models.EntityRelationship,
	entityNameByID map[uuid.UUID]string,
	insights *columnFeatureInsights,
) (string, error) {
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("create LLM client: %w", err)
	}

	systemMessage := s.domainDescriptionSystemMessage()
	prompt := s.buildDomainDescriptionPrompt(entities, relationships, entityNameByID, insights)

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
	entities []*models.OntologyEntity,
	relationships []*models.EntityRelationship,
	entityNameByID map[uuid.UUID]string,
	insights *columnFeatureInsights,
) string {
	var sb strings.Builder

	sb.WriteString("# Database Schema Analysis\n\n")
	sb.WriteString("Based on the following entities and their relationships, provide a 2-3 sentence business description of what this database represents.\n\n")

	sb.WriteString("## Entities\n\n")
	for _, e := range entities {
		domain := e.Domain
		if domain == "" {
			domain = "general"
		}
		sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", e.Name, domain, e.Description))
	}

	if len(relationships) > 0 {
		sb.WriteString("\n## Key Relationships\n\n")
		for _, rel := range relationships {
			sourceName := entityNameByID[rel.SourceEntityID]
			targetName := entityNameByID[rel.TargetEntityID]
			if sourceName == "" || targetName == "" {
				s.logger.Debug("Skipping relationship with missing entity name",
					zap.String("source_entity_id", rel.SourceEntityID.String()),
					zap.String("target_entity_id", rel.TargetEntityID.String()),
					zap.String("source_name", sourceName),
					zap.String("target_name", targetName))
				continue
			}
			if rel.Description != nil && *rel.Description != "" {
				sb.WriteString(fmt.Sprintf("- %s → %s (%s)\n", sourceName, targetName, *rel.Description))
			} else {
				sb.WriteString(fmt.Sprintf("- %s → %s\n", sourceName, targetName))
			}
		}
	}

	// Include feature-derived insights if available
	if insights != nil {
		insightLines := s.buildFeatureInsightsSection(insights, len(entities))
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
	totalEntities int,
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
	if totalEntities > 0 {
		createdPct = float64(len(insights.auditCreatedTables)) / float64(totalEntities) * 100
		updatedPct = float64(len(insights.auditUpdatedTables)) / float64(totalEntities) * 100
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

	// External service integrations
	externalServices map[string]int // service -> count of columns
}

// discoverConventionsWithInsights analyzes schema columns to detect project-wide conventions.
// Uses pre-extracted ColumnFeatures insights for more accurate detection.
func (s *ontologyFinalizationService) discoverConventionsWithInsights(
	_ context.Context,
	_ uuid.UUID,
	entities []*models.OntologyEntity,
	columnsByTable map[string][]*models.SchemaColumn,
	insights *columnFeatureInsights,
) (*models.ProjectConventions, error) {
	if len(entities) == 0 {
		return nil, nil
	}

	totalTables := len(entities)
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

	// Detect currency convention
	conventions.Currency = s.detectCurrency(columnsByTable)

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

// extractColumnFeatureInsights aggregates insights from ColumnFeatures across all columns.
func (s *ontologyFinalizationService) extractColumnFeatureInsights(
	columnsByTable map[string][]*models.SchemaColumn,
) *columnFeatureInsights {
	insights := &columnFeatureInsights{
		externalServices: make(map[string]int),
	}

	softDeleteByColumn := make(map[string][]string) // column name -> table names

	for tableName, columns := range columnsByTable {
		for _, col := range columns {
			features := col.GetColumnFeatures()
			if features == nil {
				continue
			}

			// Check for soft-delete pattern
			if features.TimestampFeatures != nil && features.TimestampFeatures.IsSoftDelete {
				softDeleteByColumn[col.ColumnName] = append(softDeleteByColumn[col.ColumnName], tableName)
			}

			// Check for audit columns
			if features.TimestampFeatures != nil && features.TimestampFeatures.IsAuditField {
				switch features.TimestampFeatures.TimestampPurpose {
				case models.TimestampPurposeAuditCreated:
					insights.auditCreatedTables = append(insights.auditCreatedTables, tableName)
				case models.TimestampPurposeAuditUpdated:
					insights.auditUpdatedTables = append(insights.auditUpdatedTables, tableName)
				}
			}

			// Check for monetary columns
			if features.MonetaryFeatures != nil && features.MonetaryFeatures.IsMonetary {
				insights.monetaryTotal++
				if features.MonetaryFeatures.PairedCurrencyColumn != "" {
					insights.monetaryWithCurrency++
				}
			}

			// Check for external service identifiers
			if features.IdentifierFeatures != nil && features.IdentifierFeatures.ExternalService != "" {
				insights.externalServices[features.IdentifierFeatures.ExternalService]++
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
