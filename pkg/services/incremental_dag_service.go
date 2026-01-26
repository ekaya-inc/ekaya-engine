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
)

// IncrementalDAGService handles targeted LLM enrichment for approved ontology changes.
// Unlike the full DAG which re-extracts everything, this service only enriches what changed.
type IncrementalDAGService interface {
	// ProcessChange processes a single approved change with LLM enrichment.
	// Returns nil if enrichment is skipped (no AI Config, precedence blocked, etc.)
	ProcessChange(ctx context.Context, change *models.PendingChange) error

	// ProcessChanges processes a batch of changes, grouping by type for efficiency.
	// Entities are processed before relationships since relationships depend on entities.
	ProcessChanges(ctx context.Context, changes []*models.PendingChange) error

	// ProcessChangeAsync processes a change asynchronously in a background goroutine.
	// Errors are logged but not returned.
	ProcessChangeAsync(ctx context.Context, change *models.PendingChange)

	// SetChangeReviewService injects the ChangeReviewService for precedence checking.
	// This breaks the circular dependency: IncrementalDAGService needs ChangeReviewService
	// for precedence checking, while ChangeReviewService needs IncrementalDAGService for
	// triggering enrichment after approval.
	SetChangeReviewService(svc ChangeReviewService)
}

type incrementalDAGService struct {
	ontologyRepo       repositories.OntologyRepository
	entityRepo         repositories.OntologyEntityRepository
	relationshipRepo   repositories.EntityRelationshipRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	schemaRepo         repositories.SchemaRepository
	conversationRepo   repositories.ConversationRepository
	aiConfigSvc        AIConfigService
	llmFactory         llm.LLMClientFactory
	changeReviewSvc    ChangeReviewService
	getTenantCtx       TenantContextFunc
	logger             *zap.Logger
}

// IncrementalDAGServiceDeps contains dependencies for IncrementalDAGService.
type IncrementalDAGServiceDeps struct {
	OntologyRepo       repositories.OntologyRepository
	EntityRepo         repositories.OntologyEntityRepository
	RelationshipRepo   repositories.EntityRelationshipRepository
	ColumnMetadataRepo repositories.ColumnMetadataRepository
	SchemaRepo         repositories.SchemaRepository
	ConversationRepo   repositories.ConversationRepository
	AIConfigSvc        AIConfigService
	LLMFactory         llm.LLMClientFactory
	ChangeReviewSvc    ChangeReviewService
	GetTenantCtx       TenantContextFunc
	Logger             *zap.Logger
}

// NewIncrementalDAGService creates a new IncrementalDAGService.
func NewIncrementalDAGService(deps *IncrementalDAGServiceDeps) IncrementalDAGService {
	return &incrementalDAGService{
		ontologyRepo:       deps.OntologyRepo,
		entityRepo:         deps.EntityRepo,
		relationshipRepo:   deps.RelationshipRepo,
		columnMetadataRepo: deps.ColumnMetadataRepo,
		schemaRepo:         deps.SchemaRepo,
		conversationRepo:   deps.ConversationRepo,
		aiConfigSvc:        deps.AIConfigSvc,
		llmFactory:         deps.LLMFactory,
		changeReviewSvc:    deps.ChangeReviewSvc,
		getTenantCtx:       deps.GetTenantCtx,
		logger:             deps.Logger.Named("incremental-dag"),
	}
}

var _ IncrementalDAGService = (*incrementalDAGService)(nil)

// SetChangeReviewService injects the ChangeReviewService for precedence checking.
func (s *incrementalDAGService) SetChangeReviewService(svc ChangeReviewService) {
	s.changeReviewSvc = svc
}

// ProcessChangeAsync processes a change asynchronously.
func (s *incrementalDAGService) ProcessChangeAsync(ctx context.Context, change *models.PendingChange) {
	go func() {
		// Use background context since the original may be cancelled
		bgCtx := context.Background()

		// Get tenant context for the background operation
		tenantCtx, cleanup, err := s.getTenantCtx(bgCtx, change.ProjectID)
		if err != nil {
			s.logger.Error("Failed to get tenant context for async processing",
				zap.String("change_id", change.ID.String()),
				zap.Error(err))
			return
		}
		defer cleanup()

		if err := s.ProcessChange(tenantCtx, change); err != nil {
			s.logger.Error("Async change processing failed",
				zap.String("change_id", change.ID.String()),
				zap.String("change_type", change.ChangeType),
				zap.Error(err))
		}
	}()
}

// ProcessChange processes a single approved change with targeted LLM enrichment.
func (s *incrementalDAGService) ProcessChange(ctx context.Context, change *models.PendingChange) error {
	// Check if AI Config is attached - if not, skip enrichment
	aiConfig, err := s.aiConfigSvc.Get(ctx, change.ProjectID)
	if err != nil {
		s.logger.Debug("No AI config, skipping enrichment",
			zap.String("change_id", change.ID.String()),
			zap.Error(err))
		return nil
	}
	if aiConfig == nil || aiConfig.ConfigType == models.AIConfigNone {
		s.logger.Debug("AI config not configured, skipping enrichment",
			zap.String("change_id", change.ID.String()))
		return nil
	}

	s.logger.Info("Processing change for incremental enrichment",
		zap.String("change_id", change.ID.String()),
		zap.String("change_type", change.ChangeType),
		zap.String("table_name", change.TableName))

	switch change.ChangeType {
	case models.ChangeTypeNewTable:
		return s.processNewTable(ctx, change)
	case models.ChangeTypeNewColumn:
		return s.processNewColumn(ctx, change)
	case models.ChangeTypeNewFKPattern:
		return s.processNewRelationship(ctx, change)
	case models.ChangeTypeNewEnumValue:
		return s.processEnumUpdate(ctx, change)
	default:
		s.logger.Debug("Change type does not require enrichment",
			zap.String("change_type", change.ChangeType))
		return nil
	}
}

// ProcessChanges processes a batch of changes, grouping by type for efficiency.
func (s *incrementalDAGService) ProcessChanges(ctx context.Context, changes []*models.PendingChange) error {
	if len(changes) == 0 {
		return nil
	}

	// Group by type
	byType := make(map[string][]*models.PendingChange)
	for _, c := range changes {
		byType[c.ChangeType] = append(byType[c.ChangeType], c)
	}

	// Process in dependency order: tables -> columns -> relationships -> enums

	// 1. New tables first (creates entities)
	if tables := byType[models.ChangeTypeNewTable]; len(tables) > 0 {
		for _, change := range tables {
			if err := s.processNewTable(ctx, change); err != nil {
				s.logger.Error("Failed to process new table",
					zap.String("table_name", change.TableName),
					zap.Error(err))
				// Continue with other changes
			}
		}
	}

	// 2. New columns
	if columns := byType[models.ChangeTypeNewColumn]; len(columns) > 0 {
		for _, change := range columns {
			if err := s.processNewColumn(ctx, change); err != nil {
				s.logger.Error("Failed to process new column",
					zap.String("table_name", change.TableName),
					zap.String("column_name", change.ColumnName),
					zap.Error(err))
			}
		}
	}

	// 3. New FK patterns / relationships
	if rels := byType[models.ChangeTypeNewFKPattern]; len(rels) > 0 {
		for _, change := range rels {
			if err := s.processNewRelationship(ctx, change); err != nil {
				s.logger.Error("Failed to process new relationship",
					zap.String("change_id", change.ID.String()),
					zap.Error(err))
			}
		}
	}

	// 4. Enum updates (no LLM needed)
	if enums := byType[models.ChangeTypeNewEnumValue]; len(enums) > 0 {
		for _, change := range enums {
			if err := s.processEnumUpdate(ctx, change); err != nil {
				s.logger.Error("Failed to process enum update",
					zap.String("table_name", change.TableName),
					zap.String("column_name", change.ColumnName),
					zap.Error(err))
			}
		}
	}

	return nil
}

// processNewTable creates an entity for a new table and enriches it with LLM.
func (s *incrementalDAGService) processNewTable(ctx context.Context, change *models.PendingChange) error {
	// Get the active ontology
	ontology, err := s.ontologyRepo.GetActive(ctx, change.ProjectID)
	if err != nil {
		return fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return fmt.Errorf("no active ontology found")
	}

	// Check if entity already exists for this table
	existingEntity, err := s.getEntityByTable(ctx, change.ProjectID, change.TableName)
	if err != nil {
		return fmt.Errorf("check existing entity: %w", err)
	}

	if existingEntity != nil {
		// Entity exists - check precedence before enriching
		if !s.changeReviewSvc.CanModify(existingEntity.Source, existingEntity.LastEditSource, models.ProvenanceInferred) {
			s.logger.Info("Skipping entity enrichment due to precedence",
				zap.String("entity", existingEntity.Name),
				zap.String("source", existingEntity.Source))
			return nil
		}
	}

	// Get table schema for LLM context
	columns, err := s.getTableColumns(ctx, change.ProjectID, change.TableName)
	if err != nil {
		s.logger.Warn("Could not get columns for table, using basic enrichment",
			zap.String("table_name", change.TableName),
			zap.Error(err))
		columns = nil
	}

	// Create LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, change.ProjectID)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Run LLM entity discovery for just this table
	enrichment, err := s.enrichEntityWithLLM(ctx, llmClient, change.TableName, columns)
	if err != nil {
		s.logger.Warn("LLM enrichment failed, using table name as entity",
			zap.String("table_name", change.TableName),
			zap.Error(err))
		// Fall back to basic entity creation
		enrichment = &singleEntityEnrichment{
			EntityName:  toTitleCase(change.TableName),
			Description: fmt.Sprintf("Entity representing the %s table", change.TableName),
			Domain:      "general",
		}
	}

	if existingEntity != nil {
		// Update existing entity with enrichment
		// Note: UpdatedBy and LastEditSource are set by the repository from provenance context
		existingEntity.Name = enrichment.EntityName
		existingEntity.Description = enrichment.Description
		existingEntity.Domain = enrichment.Domain

		if err := s.entityRepo.Update(ctx, existingEntity); err != nil {
			return fmt.Errorf("update entity: %w", err)
		}

		s.logger.Info("Updated existing entity with LLM enrichment",
			zap.String("entity_id", existingEntity.ID.String()),
			zap.String("entity_name", enrichment.EntityName))
	} else {
		// Get primary column from NewValue if available
		primaryColumn := "id"
		if change.NewValue != nil {
			if cols, ok := change.NewValue["columns"].([]any); ok {
				for _, col := range cols {
					if colMap, ok := col.(map[string]any); ok {
						if isPK, _ := colMap["is_primary_key"].(bool); isPK {
							if colName, ok := colMap["column_name"].(string); ok {
								primaryColumn = colName
								break
							}
						}
					}
				}
			}
		}

		// Create new entity
		// Note: Source and CreatedBy are set by the repository from provenance context
		entity := &models.OntologyEntity{
			ProjectID:     change.ProjectID,
			OntologyID:    ontology.ID,
			Name:          enrichment.EntityName,
			Description:   enrichment.Description,
			Domain:        enrichment.Domain,
			PrimarySchema: "public",
			PrimaryTable:  change.TableName,
			PrimaryColumn: primaryColumn,
		}

		if err := s.entityRepo.Create(ctx, entity); err != nil {
			return fmt.Errorf("create entity: %w", err)
		}

		// Create key columns if provided
		for _, kc := range enrichment.KeyColumns {
			keyCol := &models.OntologyEntityKeyColumn{
				EntityID:   entity.ID,
				ColumnName: kc.Name,
				Synonyms:   kc.Synonyms,
			}
			if err := s.entityRepo.CreateKeyColumn(ctx, keyCol); err != nil {
				s.logger.Warn("Failed to create key column",
					zap.String("column_name", kc.Name),
					zap.Error(err))
			}
		}

		// Create aliases if provided
		for _, alias := range enrichment.AlternativeNames {
			aliasRecord := &models.OntologyEntityAlias{
				EntityID: entity.ID,
				Alias:    alias,
				Source:   ptrStr("incremental_discovery"),
			}
			if err := s.entityRepo.CreateAlias(ctx, aliasRecord); err != nil {
				s.logger.Warn("Failed to create entity alias",
					zap.String("alias", alias),
					zap.Error(err))
			}
		}

		s.logger.Info("Created new entity with LLM enrichment",
			zap.String("entity_id", entity.ID.String()),
			zap.String("entity_name", enrichment.EntityName),
			zap.String("table_name", change.TableName))
	}

	return nil
}

// processNewColumn enriches a new column with LLM-generated metadata.
func (s *incrementalDAGService) processNewColumn(ctx context.Context, change *models.PendingChange) error {
	// Check existing metadata and precedence
	existing, err := s.columnMetadataRepo.GetByTableColumn(ctx, change.ProjectID, change.TableName, change.ColumnName)
	if err != nil {
		return fmt.Errorf("check existing column metadata: %w", err)
	}

	if existing != nil {
		if !s.changeReviewSvc.CanModify(existing.CreatedBy, existing.UpdatedBy, models.ProvenanceInferred) {
			s.logger.Info("Skipping column enrichment due to precedence",
				zap.String("table", change.TableName),
				zap.String("column", change.ColumnName),
				zap.String("created_by", existing.CreatedBy))
			return nil
		}
	}

	// Get entity context for LLM
	entity, err := s.getEntityByTable(ctx, change.ProjectID, change.TableName)
	if err != nil {
		s.logger.Debug("No entity found for table",
			zap.String("table_name", change.TableName),
			zap.Error(err))
		entity = nil
	}

	// Get column schema info
	columnInfo, err := s.getColumnInfo(ctx, change.ProjectID, change.TableName, change.ColumnName)
	if err != nil {
		s.logger.Warn("Could not get column info",
			zap.String("column", change.ColumnName),
			zap.Error(err))
	}

	// Create LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, change.ProjectID)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Run LLM column enrichment
	enrichment, err := s.enrichColumnWithLLM(ctx, llmClient, entity, columnInfo)
	if err != nil {
		s.logger.Warn("LLM column enrichment failed",
			zap.String("column", change.ColumnName),
			zap.Error(err))
		return nil // Don't fail - column works without enrichment
	}

	// Create or update column metadata
	metadata := &models.ColumnMetadata{
		ProjectID:   change.ProjectID,
		TableName:   change.TableName,
		ColumnName:  change.ColumnName,
		Description: &enrichment.Description,
		Role:        &enrichment.Role,
		CreatedBy:   models.ProvenanceInferred,
	}

	if existing != nil {
		metadata.ID = existing.ID
		metadata.CreatedBy = existing.CreatedBy
		metadata.CreatedAt = existing.CreatedAt
		metadata.UpdatedBy = ptrStr(models.ProvenanceInferred)
	}

	if len(enrichment.EnumValues) > 0 {
		var enumStrings []string
		for _, ev := range enrichment.EnumValues {
			if ev.Description != "" {
				enumStrings = append(enumStrings, fmt.Sprintf("%s - %s", ev.Value, ev.Description))
			} else {
				enumStrings = append(enumStrings, ev.Value)
			}
		}
		metadata.EnumValues = enumStrings
	}

	if err := s.columnMetadataRepo.Upsert(ctx, metadata); err != nil {
		return fmt.Errorf("save column metadata: %w", err)
	}

	s.logger.Info("Enriched column with LLM",
		zap.String("table", change.TableName),
		zap.String("column", change.ColumnName),
		zap.String("role", enrichment.Role))

	return nil
}

// processNewRelationship enriches a new relationship with LLM-generated description.
func (s *incrementalDAGService) processNewRelationship(ctx context.Context, change *models.PendingChange) error {
	if change.SuggestedPayload == nil {
		return fmt.Errorf("missing suggested_payload for new relationship")
	}

	// Extract relationship details from payload
	sourceTable, _ := change.SuggestedPayload["source_table"].(string)
	targetTable, _ := change.SuggestedPayload["target_table"].(string)

	if sourceTable == "" || targetTable == "" {
		return fmt.Errorf("missing source_table or target_table in payload")
	}

	// Get entities for these tables
	sourceEntity, err := s.getEntityByTable(ctx, change.ProjectID, sourceTable)
	if err != nil || sourceEntity == nil {
		s.logger.Debug("Source entity not found, skipping relationship enrichment",
			zap.String("source_table", sourceTable))
		return nil
	}

	targetEntity, err := s.getEntityByTable(ctx, change.ProjectID, targetTable)
	if err != nil || targetEntity == nil {
		s.logger.Debug("Target entity not found, skipping relationship enrichment",
			zap.String("target_table", targetTable))
		return nil
	}

	// Check if relationship already exists - need ontology ID for lookup
	ontology, err := s.ontologyRepo.GetActive(ctx, change.ProjectID)
	if err != nil || ontology == nil {
		return fmt.Errorf("get active ontology: %w", err)
	}

	existingRel, err := s.relationshipRepo.GetByEntityPair(ctx, ontology.ID, sourceEntity.ID, targetEntity.ID)
	if err != nil {
		return fmt.Errorf("check existing relationship: %w", err)
	}

	if existingRel != nil {
		if !s.changeReviewSvc.CanModify(existingRel.Source, existingRel.LastEditSource, models.ProvenanceInferred) {
			s.logger.Info("Skipping relationship enrichment due to precedence",
				zap.String("from", sourceEntity.Name),
				zap.String("to", targetEntity.Name),
				zap.String("source", existingRel.Source))
			return nil
		}
	}

	// Create LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, change.ProjectID)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Extract FK info from payload
	sourceColumn, _ := change.SuggestedPayload["source_column"].(string)
	cardinality, _ := change.SuggestedPayload["cardinality"].(string)
	if cardinality == "" {
		cardinality = "N:1" // Default for FK relationships
	}

	// Run LLM relationship enrichment
	enrichment, err := s.enrichRelationshipWithLLM(ctx, llmClient, sourceEntity, targetEntity, sourceColumn)
	if err != nil {
		s.logger.Warn("LLM relationship enrichment failed, using basic relationship",
			zap.String("from", sourceEntity.Name),
			zap.String("to", targetEntity.Name),
			zap.Error(err))
		// Fall back to basic relationship
		enrichment = &incrementalRelEnrichment{
			Description: fmt.Sprintf("%s references %s", sourceEntity.Name, targetEntity.Name),
			Association: "references",
		}
	}

	if existingRel != nil {
		// Update existing relationship
		// Note: UpdatedBy and LastEditSource are set by the repository from provenance context
		existingRel.Description = &enrichment.Description
		existingRel.Association = &enrichment.Association

		if err := s.relationshipRepo.Update(ctx, existingRel); err != nil {
			return fmt.Errorf("update relationship: %w", err)
		}

		s.logger.Info("Updated relationship with LLM enrichment",
			zap.String("from", sourceEntity.Name),
			zap.String("to", targetEntity.Name))
	} else {
		// Get active ontology for new relationship
		ontology, err := s.ontologyRepo.GetActive(ctx, change.ProjectID)
		if err != nil || ontology == nil {
			return fmt.Errorf("get active ontology: %w", err)
		}

		// Create new relationship
		// Note: Source and CreatedBy are set by the repository from provenance context
		rel := &models.EntityRelationship{
			OntologyID:        ontology.ID,
			SourceEntityID:    sourceEntity.ID,
			TargetEntityID:    targetEntity.ID,
			SourceColumnTable: sourceTable,
			SourceColumnName:  sourceColumn,
			TargetColumnTable: targetTable,
			TargetColumnName:  "id", // Assume FK references PK
			DetectionMethod:   "incremental_enrichment",
			Confidence:        0.9,
			Status:            "confirmed",
			Cardinality:       cardinality,
			Description:       &enrichment.Description,
			Association:       &enrichment.Association,
		}

		if err := s.relationshipRepo.Create(ctx, rel); err != nil {
			return fmt.Errorf("create relationship: %w", err)
		}

		s.logger.Info("Created relationship with LLM enrichment",
			zap.String("from", sourceEntity.Name),
			zap.String("to", targetEntity.Name),
			zap.String("association", enrichment.Association))
	}

	return nil
}

// processEnumUpdate merges new enum values into existing column metadata.
// This does not require LLM - just merges the new values.
func (s *incrementalDAGService) processEnumUpdate(ctx context.Context, change *models.PendingChange) error {
	if change.NewValue == nil {
		return nil
	}

	// Extract new enum values from the change
	newValues, ok := change.NewValue["new_values"].([]any)
	if !ok || len(newValues) == 0 {
		return nil
	}

	// Get existing column metadata
	existing, err := s.columnMetadataRepo.GetByTableColumn(ctx, change.ProjectID, change.TableName, change.ColumnName)
	if err != nil {
		return fmt.Errorf("get existing column metadata: %w", err)
	}

	// Check precedence if metadata exists
	if existing != nil {
		if !s.changeReviewSvc.CanModify(existing.CreatedBy, existing.UpdatedBy, models.ProvenanceInferred) {
			s.logger.Info("Skipping enum update due to precedence",
				zap.String("table", change.TableName),
				zap.String("column", change.ColumnName))
			return nil
		}
	}

	// Build set of existing values
	existingSet := make(map[string]bool)
	var mergedValues []string
	if existing != nil {
		for _, v := range existing.EnumValues {
			existingSet[v] = true
			mergedValues = append(mergedValues, v)
		}
	}

	// Add new values that don't already exist
	for _, v := range newValues {
		if strVal, ok := v.(string); ok {
			if !existingSet[strVal] {
				mergedValues = append(mergedValues, strVal)
				existingSet[strVal] = true
			}
		}
	}

	// Create or update column metadata with merged values
	metadata := &models.ColumnMetadata{
		ProjectID:  change.ProjectID,
		TableName:  change.TableName,
		ColumnName: change.ColumnName,
		EnumValues: mergedValues,
		CreatedBy:  models.ProvenanceInferred,
	}

	if existing != nil {
		metadata.ID = existing.ID
		metadata.CreatedBy = existing.CreatedBy
		metadata.CreatedAt = existing.CreatedAt
		metadata.Description = existing.Description
		metadata.Entity = existing.Entity
		metadata.Role = existing.Role
		metadata.UpdatedBy = ptrStr(models.ProvenanceInferred)
	}

	if err := s.columnMetadataRepo.Upsert(ctx, metadata); err != nil {
		return fmt.Errorf("save column metadata: %w", err)
	}

	s.logger.Info("Merged new enum values",
		zap.String("table", change.TableName),
		zap.String("column", change.ColumnName),
		zap.Int("total_values", len(mergedValues)))

	return nil
}

// Helper methods

func (s *incrementalDAGService) getEntityByTable(ctx context.Context, projectID uuid.UUID, tableName string) (*models.OntologyEntity, error) {
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, e := range entities {
		if e.PrimaryTable == tableName {
			return e, nil
		}
	}
	return nil, nil
}

func (s *incrementalDAGService) getTableColumns(ctx context.Context, projectID uuid.UUID, tableName string) ([]*models.SchemaColumn, error) {
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, []string{tableName}, true)
	if err != nil {
		return nil, err
	}
	return columnsByTable[tableName], nil
}

func (s *incrementalDAGService) getColumnInfo(ctx context.Context, projectID uuid.UUID, tableName, columnName string) (*models.SchemaColumn, error) {
	columns, err := s.getTableColumns(ctx, projectID, tableName)
	if err != nil {
		return nil, err
	}
	for _, c := range columns {
		if c.ColumnName == columnName {
			return c, nil
		}
	}
	return nil, fmt.Errorf("column not found: %s.%s", tableName, columnName)
}

// ptrStr returns a pointer to the given string.
func ptrStr(s string) *string {
	return &s
}

// toTitleCase converts a snake_case string to Title Case.
func toTitleCase(s string) string {
	words := strings.Split(s, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(string(w[0])) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}
