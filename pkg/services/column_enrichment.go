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
)

// ColumnEnrichmentService provides semantic enrichment for database columns.
// It uses LLM to generate descriptions, semantic types, roles, and enum value mappings.
type ColumnEnrichmentService interface {
	// EnrichTable enriches all columns for a single table.
	EnrichTable(ctx context.Context, projectID uuid.UUID, tableName string) error

	// EnrichProject enriches all tables in a project.
	// Returns the enrichment result with success/failure counts.
	EnrichProject(ctx context.Context, projectID uuid.UUID, tableNames []string) (*EnrichColumnsResult, error)
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
	dsSvc            DatasourceService
	adapterFactory   datasource.DatasourceAdapterFactory
	llmFactory       llm.LLMClientFactory
	getTenantCtx     TenantContextFunc
	logger           *zap.Logger
}

// NewColumnEnrichmentService creates a new column enrichment service.
func NewColumnEnrichmentService(
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	schemaRepo repositories.SchemaRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) ColumnEnrichmentService {
	return &columnEnrichmentService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relationshipRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            dsSvc,
		adapterFactory:   adapterFactory,
		llmFactory:       llmFactory,
		getTenantCtx:     getTenantCtx,
		logger:           logger.Named("column-enrichment"),
	}
}

var _ ColumnEnrichmentService = (*columnEnrichmentService)(nil)

// EnrichProject enriches all specified tables (or all entities if empty).
func (s *columnEnrichmentService) EnrichProject(ctx context.Context, projectID uuid.UUID, tableNames []string) (*EnrichColumnsResult, error) {
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

	// Enrich each table, continuing on failure
	for _, tableName := range tableNames {
		if err := s.EnrichTable(ctx, projectID, tableName); err != nil {
			s.logger.Error("Failed to enrich table",
				zap.String("table", tableName),
				zap.Error(err))
			result.TablesFailed[tableName] = err.Error()
		} else {
			result.TablesEnriched = append(result.TablesEnriched, tableName)
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
		return datasources[0], nil
	}

	return nil, fmt.Errorf("no datasource found for project %s", projectID)
}

// columnEnrichment is the LLM response structure for column enrichment.
type columnEnrichment struct {
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	SemanticType string             `json:"semantic_type"`
	Role         string             `json:"role"`
	Synonyms     []string           `json:"synonyms,omitempty"`
	EnumValues   []models.EnumValue `json:"enum_values,omitempty"`
	FKRole       *string            `json:"fk_role"`
}

// enrichColumnsWithLLM uses the LLM to generate semantic metadata for columns.
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

	systemMsg := s.columnEnrichmentSystemMessage()
	prompt := s.buildColumnEnrichmentPrompt(entity, columns, fkInfo, enumSamples)

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.3, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	enrichments, err := llm.ParseJSONResponse[[]columnEnrichment](result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	return enrichments, nil
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
	sb.WriteString("5. **enum_values**: if status/type column, array of {value, label, description}\n")
	sb.WriteString("6. **fk_role**: if FK column and another column references same table, what role does this FK represent?\n")
	sb.WriteString("   (e.g., payer_user_id -> \"payer\", payee_user_id -> \"payee\")\n")

	// Response format
	sb.WriteString("\n## Response Format (JSON array)\n")
	sb.WriteString("```json\n")
	sb.WriteString("[\n")
	sb.WriteString("  {\n")
	sb.WriteString("    \"name\": \"column_name\",\n")
	sb.WriteString("    \"description\": \"Business meaning of this column\",\n")
	sb.WriteString("    \"semantic_type\": \"status\",\n")
	sb.WriteString("    \"role\": \"dimension\",\n")
	sb.WriteString("    \"synonyms\": [\"alt_name1\", \"alt_name2\"],\n")
	sb.WriteString("    \"enum_values\": [\n")
	sb.WriteString("      {\"value\": \"active\", \"label\": \"Active\", \"description\": \"Entity is currently active\"}\n")
	sb.WriteString("    ],\n")
	sb.WriteString("    \"fk_role\": null\n")
	sb.WriteString("  }\n")
	sb.WriteString("]\n")
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
			if enrichment.FKRole != nil {
				detail.FKRole = *enrichment.FKRole
			}
		}

		details = append(details, detail)
	}

	return details
}
