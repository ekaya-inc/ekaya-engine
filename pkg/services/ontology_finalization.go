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

	// Aggregate unique domains from entity.Domain fields
	primaryDomains := s.aggregateUniqueDomains(entities)

	// Build entity lookups for relationship display and graph building
	entityNameByID := make(map[uuid.UUID]string, len(entities))
	entityByID := make(map[uuid.UUID]*models.OntologyEntity, len(entities))
	for _, e := range entities {
		entityNameByID[e.ID] = e.Name
		entityByID[e.ID] = e
	}

	// Generate domain description via LLM
	description, err := s.generateDomainDescription(ctx, projectID, entities, relationships, entityNameByID)
	if err != nil {
		return fmt.Errorf("generate domain description: %w", err)
	}

	// Discover project conventions (soft delete, currency, audit columns)
	conventions, err := s.discoverConventions(ctx, projectID, entities)
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
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames)
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
) (string, error) {
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("create LLM client: %w", err)
	}

	systemMessage := s.domainDescriptionSystemMessage()
	prompt := s.buildDomainDescriptionPrompt(entities, relationships, entityNameByID)

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

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("Respond with a JSON object:\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"description\": \"A 2-3 sentence business summary of what this database represents.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
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

// discoverConventions analyzes schema columns to detect project-wide conventions.
func (s *ontologyFinalizationService) discoverConventions(
	ctx context.Context,
	projectID uuid.UUID,
	entities []*models.OntologyEntity,
) (*models.ProjectConventions, error) {
	if len(entities) == 0 {
		return nil, nil
	}

	// Get table names from entities
	tableNames := make([]string, 0, len(entities))
	for _, e := range entities {
		tableNames = append(tableNames, e.PrimaryTable)
	}

	// Get all columns for these tables
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames)
	if err != nil {
		return nil, fmt.Errorf("get columns by tables: %w", err)
	}

	totalTables := len(tableNames)
	conventions := &models.ProjectConventions{}

	// Detect soft delete convention
	conventions.SoftDelete = s.detectSoftDelete(columnsByTable, totalTables)

	// Detect currency convention
	conventions.Currency = s.detectCurrency(columnsByTable)

	// Detect audit columns
	conventions.AuditColumns = s.detectAuditColumns(columnsByTable, totalTables)

	// Return nil if no conventions found
	if conventions.SoftDelete == nil && conventions.Currency == nil && len(conventions.AuditColumns) == 0 {
		return nil, nil
	}

	return conventions, nil
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
