package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// OntologyContextService assembles ontology responses at different depth levels.
type OntologyContextService interface {
	// GetDomainContext returns high-level domain information (~200-500 tokens).
	GetDomainContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyDomainContext, error)

	// GetEntitiesContext returns entity summaries with occurrences (~500-1500 tokens).
	GetEntitiesContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyEntitiesContext, error)

	// GetTablesContext returns table summaries, optionally filtered by table names.
	GetTablesContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyTablesContext, error)

	// GetColumnsContext returns full column details for specified tables.
	// Always requires table filter to manage response size.
	GetColumnsContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyColumnsContext, error)
}

type ontologyContextService struct {
	ontologyRepo       repositories.OntologyRepository
	entityRepo         repositories.OntologyEntityRepository
	relationshipRepo   repositories.EntityRelationshipRepository
	schemaRepo         repositories.SchemaRepository
	tableMetadataRepo  repositories.TableMetadataRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	projectService     ProjectService // Reserved for Phase 2/3: project-level domain aggregation
	logger             *zap.Logger
}

// NewOntologyContextService creates a new OntologyContextService.
func NewOntologyContextService(
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	schemaRepo repositories.SchemaRepository,
	tableMetadataRepo repositories.TableMetadataRepository,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	projectService ProjectService,
	logger *zap.Logger,
) OntologyContextService {
	return &ontologyContextService{
		ontologyRepo:       ontologyRepo,
		entityRepo:         entityRepo,
		relationshipRepo:   relationshipRepo,
		schemaRepo:         schemaRepo,
		tableMetadataRepo:  tableMetadataRepo,
		columnMetadataRepo: columnMetadataRepo,
		projectService:     projectService,
		logger:             logger,
	}
}

// GetDomainContext returns high-level domain information.
// Only returns promoted entities (is_promoted=true) to filter out low-value entities.
func (s *ontologyContextService) GetDomainContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyDomainContext, error) {
	// Get active ontology (only for checking it exists and domain summary)
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// Get only promoted entities - demoted entities are filtered out
	entities, err := s.entityRepo.GetPromotedByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entities: %w", err)
	}

	// Get entity relationships from normalized table
	entityRelationships, err := s.relationshipRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity relationships: %w", err)
	}

	// Get column count from schema tables
	columnCount, err := s.schemaRepo.GetColumnCountByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get column count: %w", err)
	}

	// Build occurrence count map from inbound relationships
	// Each inbound relationship = one occurrence at the source column location
	occurrenceCountByEntityID := make(map[uuid.UUID]int)
	for _, rel := range entityRelationships {
		// Count inbound relationships (where entity is the target)
		occurrenceCountByEntityID[rel.TargetEntityID]++
	}

	// Build entity ID to name map for relationship edges
	entityNameByID := make(map[uuid.UUID]string)
	for _, entity := range entities {
		entityNameByID[entity.ID] = entity.Name
	}

	// Build domain info - TableCount = entity count, ColumnCount from schema
	domainInfo := models.DomainInfo{
		TableCount:  len(entities),
		ColumnCount: columnCount,
	}

	// Use domain summary if available (populated by Ontology Finalization)
	if ontology.DomainSummary != nil {
		domainInfo.Description = ontology.DomainSummary.Description
		domainInfo.PrimaryDomains = ontology.DomainSummary.Domains
		domainInfo.Conventions = ontology.DomainSummary.Conventions
	}

	// Build entity briefs
	entityBriefs := make([]models.EntityBrief, 0, len(entities))
	for _, entity := range entities {
		entityBriefs = append(entityBriefs, models.EntityBrief{
			Name:            entity.Name,
			Description:     entity.Description,
			PrimaryTable:    entity.PrimaryTable,
			OccurrenceCount: occurrenceCountByEntityID[entity.ID],
		})
	}

	// Build relationships from normalized entity_relationships table
	// Deduplicate by source→target pair, keeping the longest label for more context
	relationships := make([]models.RelationshipEdge, 0, len(entityRelationships))
	seen := make(map[string]int) // key -> index in relationships slice
	for _, rel := range entityRelationships {
		sourceName := entityNameByID[rel.SourceEntityID]
		targetName := entityNameByID[rel.TargetEntityID]
		if sourceName == "" || targetName == "" {
			continue // Skip if entity names not found
		}

		var label string
		if rel.Description != nil {
			label = *rel.Description
		}

		key := sourceName + "→" + targetName
		if idx, exists := seen[key]; exists {
			// Keep the longer label for more context
			if len(label) > len(relationships[idx].Label) {
				relationships[idx].Label = label
			}
			continue
		}

		seen[key] = len(relationships)
		relationships = append(relationships, models.RelationshipEdge{
			From:  sourceName,
			To:    targetName,
			Label: label,
		})
	}

	return &models.OntologyDomainContext{
		Domain:        domainInfo,
		Entities:      entityBriefs,
		Relationships: relationships,
	}, nil
}

// GetEntitiesContext returns entity summaries with occurrences.
// Only returns promoted entities (is_promoted=true) to filter out low-value entities.
func (s *ontologyContextService) GetEntitiesContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyEntitiesContext, error) {
	// Get active ontology
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// Get only promoted entities - demoted entities are filtered out
	entities, err := s.entityRepo.GetPromotedByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entities: %w", err)
	}

	// Get all entity aliases in one query (avoids N+1)
	allAliases, err := s.entityRepo.GetAllAliasesByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity aliases: %w", err)
	}

	// Convert to string slices for synonyms
	entityAliasesMap := make(map[uuid.UUID][]string)
	for entityID, aliases := range allAliases {
		synonyms := make([]string, 0, len(aliases))
		for _, alias := range aliases {
			synonyms = append(synonyms, alias.Alias)
		}
		entityAliasesMap[entityID] = synonyms
	}

	// Get all key columns in one query (avoids N+1)
	allKeyColumns, err := s.entityRepo.GetAllKeyColumnsByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity key columns: %w", err)
	}

	// Get all relationships once (avoids N+1)
	allRelationships, err := s.relationshipRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationships: %w", err)
	}

	// Group by target entity ID
	relationshipsByTarget := make(map[uuid.UUID][]*models.EntityRelationship)
	for _, rel := range allRelationships {
		relationshipsByTarget[rel.TargetEntityID] = append(relationshipsByTarget[rel.TargetEntityID], rel)
	}

	// Build occurrence map by entity ID from grouped relationships
	occurrencesByEntityID := make(map[uuid.UUID][]models.EntityOccurrence)
	for _, entity := range entities {
		rels := relationshipsByTarget[entity.ID]
		entityOccurrences := make([]models.EntityOccurrence, 0, len(rels))
		for _, rel := range rels {
			entityOccurrences = append(entityOccurrences, models.EntityOccurrence{
				Table:       rel.SourceColumnTable,
				Column:      rel.SourceColumnName,
				Association: rel.Association,
			})
		}
		occurrencesByEntityID[entity.ID] = entityOccurrences
	}

	// Build entity details map
	entityDetails := make(map[string]models.EntityDetail)
	for _, entity := range entities {
		// Get key columns from normalized table
		var keyColumns []models.KeyColumnInfo
		if kcs, ok := allKeyColumns[entity.ID]; ok {
			for _, kc := range kcs {
				keyColumns = append(keyColumns, models.KeyColumnInfo{
					Name:     kc.ColumnName,
					Synonyms: kc.Synonyms,
				})
			}
		}

		entityDetails[entity.Name] = models.EntityDetail{
			PrimaryTable: entity.PrimaryTable,
			Description:  entity.Description,
			Synonyms:     entityAliasesMap[entity.ID],
			KeyColumns:   keyColumns,
			Occurrences:  occurrencesByEntityID[entity.ID],
		}
	}

	// Build entity relationships
	var entityRelationships []models.OntologyEntityRelationship
	// TODO: Derive entity relationships from schema relationships
	// For now, return empty list - will be implemented when needed

	return &models.OntologyEntitiesContext{
		Entities:      entityDetails,
		Relationships: entityRelationships,
	}, nil
}

// GetTablesContext returns table summaries, optionally filtered by table names.
func (s *ontologyContextService) GetTablesContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyTablesContext, error) {
	// Get active ontology (for checking it exists)
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// Get entities from normalized table to get business names/descriptions
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entities: %w", err)
	}

	// Build entity index by primary table
	entityByTable := make(map[string]*models.OntologyEntity)
	for _, entity := range entities {
		entityByTable[entity.PrimaryTable] = entity
	}

	// Get all entity aliases in one query (avoids N+1)
	allAliases, err := s.entityRepo.GetAllAliasesByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity aliases: %w", err)
	}

	// Convert to string slices for synonyms
	entityAliasesMap := make(map[uuid.UUID][]string)
	for entityID, aliases := range allAliases {
		synonyms := make([]string, 0, len(aliases))
		for _, alias := range aliases {
			synonyms = append(synonyms, alias.Alias)
		}
		entityAliasesMap[entityID] = synonyms
	}

	// If no filter provided, return all entity tables
	tablesToInclude := tableNames
	if len(tablesToInclude) == 0 {
		for _, entity := range entities {
			tablesToInclude = append(tablesToInclude, entity.PrimaryTable)
		}
	}

	// Get columns for the requested tables (selectedOnly=true to exclude deselected columns)
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tablesToInclude, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Collect all column IDs for metadata lookup
	var allColumnIDs []uuid.UUID
	for _, columns := range columnsByTable {
		for _, col := range columns {
			allColumnIDs = append(allColumnIDs, col.ID)
		}
	}

	// Fetch column metadata from engine_ontology_column_metadata
	columnMetadataByID := make(map[uuid.UUID]*models.ColumnMetadata)
	if len(allColumnIDs) > 0 && s.columnMetadataRepo != nil {
		metadataList, err := s.columnMetadataRepo.GetBySchemaColumnIDs(ctx, allColumnIDs)
		if err != nil {
			s.logger.Warn("Failed to fetch column metadata for tables context, continuing without",
				zap.Error(err))
		} else {
			for _, meta := range metadataList {
				columnMetadataByID[meta.SchemaColumnID] = meta
			}
		}
	}

	// Get relationships involving these tables
	relationships, err := s.relationshipRepo.GetByTables(ctx, projectID, tablesToInclude)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationships: %w", err)
	}

	// Build relationship index by source table
	relationshipsByTable := make(map[string][]*models.EntityRelationship)
	// Also build index by source column ID for FK association lookup
	relationshipBySourceColumnID := make(map[uuid.UUID]*models.EntityRelationship)
	for _, rel := range relationships {
		relationshipsByTable[rel.SourceColumnTable] = append(relationshipsByTable[rel.SourceColumnTable], rel)
		if rel.SourceColumnID != nil {
			relationshipBySourceColumnID[*rel.SourceColumnID] = rel
		}
	}

	// Fetch table metadata if repository is available
	var tableMetadataMap map[string]*models.TableMetadata
	if s.tableMetadataRepo != nil {
		dsID, err := s.projectService.GetDefaultDatasourceID(ctx, projectID)
		if err != nil {
			s.logger.Warn("Failed to get default datasource for table metadata",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
		} else {
			metaList, err := s.tableMetadataRepo.List(ctx, projectID, dsID)
			if err != nil {
				s.logger.Warn("Failed to get table metadata",
					zap.String("project_id", projectID.String()),
					zap.Error(err))
			} else if len(metaList) > 0 {
				tableMetadataMap = make(map[string]*models.TableMetadata, len(metaList))
				for _, meta := range metaList {
					tableMetadataMap[meta.TableName] = meta
				}
			}
		}
	}

	// Build table summaries
	tables := make(map[string]models.TableSummary)
	for _, tableName := range tablesToInclude {
		entity := entityByTable[tableName]
		if entity == nil {
			// Expected when user requests a non-entity table
			s.logger.Debug("No entity found for table",
				zap.String("table", tableName),
			)
			continue
		}

		schemaColumns := columnsByTable[tableName]

		// Build column overview from schema columns, merging metadata and relationship data
		columns := make([]models.ColumnOverview, 0, len(schemaColumns))
		for _, col := range schemaColumns {
			overview := models.ColumnOverview{
				Name:         col.ColumnName,
				Type:         col.DataType,
				IsPrimaryKey: col.IsPrimaryKey,
			}

			// Merge column metadata if available (Role, HasEnumValues)
			if meta, ok := columnMetadataByID[col.ID]; ok {
				if meta.Role != nil {
					overview.Role = *meta.Role
				}
				// Check for enum values in features
				if enumFeatures := meta.GetEnumFeatures(); enumFeatures != nil && len(enumFeatures.Values) > 0 {
					overview.HasEnumValues = true
				}
			}

			// Get FK association from relationship (source of truth for FK semantics)
			if rel, ok := relationshipBySourceColumnID[col.ID]; ok {
				if rel.Association != nil {
					overview.FKAssociation = *rel.Association
				}
			}

			columns = append(columns, overview)
		}

		// Build table relationships
		tableRels := relationshipsByTable[tableName]
		tableRelationships := make([]models.TableRelationship, 0, len(tableRels))
		for _, rel := range tableRels {
			tableRelationships = append(tableRelationships, models.TableRelationship{
				Column:     rel.SourceColumnName,
				References: rel.TargetColumnTable + "." + rel.TargetColumnName,
			})
		}

		// Get row count from columns (they all share the same table)
		var rowCount int64
		// Row count would need to come from schema_tables, but we don't have that data easily
		// For now, leave as 0 - this could be improved later

		summary := models.TableSummary{
			Schema:        entity.PrimarySchema,
			BusinessName:  entity.Name,
			Description:   entity.Description,
			Domain:        entity.Domain,
			RowCount:      rowCount,
			ColumnCount:   len(schemaColumns),
			Synonyms:      entityAliasesMap[entity.ID],
			Columns:       columns,
			Relationships: tableRelationships,
		}

		// Merge table metadata if available
		if tableMetadataMap != nil {
			if meta, ok := tableMetadataMap[tableName]; ok {
				if meta.UsageNotes != nil && *meta.UsageNotes != "" {
					summary.UsageNotes = *meta.UsageNotes
				}
				if meta.IsEphemeral {
					summary.IsEphemeral = true
				}
				if meta.PreferredAlternative != nil && *meta.PreferredAlternative != "" {
					summary.PreferredAlternative = *meta.PreferredAlternative
				}
			}
		}

		tables[tableName] = summary
	}

	return &models.OntologyTablesContext{
		Tables: tables,
	}, nil
}

// MaxColumnsDepthTables is the maximum number of tables allowed for columns depth.
const MaxColumnsDepthTables = 10

// GetColumnsContext returns full column details for specified tables.
func (s *ontologyContextService) GetColumnsContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyColumnsContext, error) {
	if len(tableNames) == 0 {
		return nil, fmt.Errorf("table names required for columns depth")
	}
	if len(tableNames) > MaxColumnsDepthTables {
		return nil, fmt.Errorf("too many tables requested: maximum %d tables allowed for columns depth, got %d", MaxColumnsDepthTables, len(tableNames))
	}

	// Get active ontology (for checking it exists)
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// Get entities for business names/descriptions
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entities: %w", err)
	}

	// Build entity index by primary table
	entityByTable := make(map[string]*models.OntologyEntity)
	for _, entity := range entities {
		entityByTable[entity.PrimaryTable] = entity
	}

	// Get columns for the requested tables (selectedOnly=true to exclude deselected columns)
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Collect all column IDs for metadata lookup
	var allColumnIDs []uuid.UUID
	for _, columns := range columnsByTable {
		for _, col := range columns {
			allColumnIDs = append(allColumnIDs, col.ID)
		}
	}

	// Fetch column metadata from engine_ontology_column_metadata
	columnMetadataByID := make(map[uuid.UUID]*models.ColumnMetadata)
	if len(allColumnIDs) > 0 && s.columnMetadataRepo != nil {
		metadataList, err := s.columnMetadataRepo.GetBySchemaColumnIDs(ctx, allColumnIDs)
		if err != nil {
			s.logger.Warn("Failed to fetch column metadata for columns context, continuing without",
				zap.Error(err))
		} else {
			for _, meta := range metadataList {
				columnMetadataByID[meta.SchemaColumnID] = meta
			}
		}
	}

	// Get relationships to determine FK info
	relationships, err := s.relationshipRepo.GetByTables(ctx, projectID, tableNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationships: %w", err)
	}

	// Build FK info index by source column ID for FK target table and association
	type fkData struct {
		targetTable string
		association string
	}
	fkByColumnID := make(map[uuid.UUID]fkData)
	for _, rel := range relationships {
		if rel.SourceColumnID != nil {
			data := fkData{targetTable: rel.TargetColumnTable}
			if rel.Association != nil {
				data.association = *rel.Association
			}
			fkByColumnID[*rel.SourceColumnID] = data
		}
	}

	// Fetch table metadata if repository is available
	var tableMetadataMap map[string]*models.TableMetadata
	if s.tableMetadataRepo != nil {
		dsID, err := s.projectService.GetDefaultDatasourceID(ctx, projectID)
		if err != nil {
			s.logger.Warn("Failed to get default datasource for table metadata",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
		} else {
			metaList, err := s.tableMetadataRepo.List(ctx, projectID, dsID)
			if err != nil {
				s.logger.Warn("Failed to get table metadata",
					zap.String("project_id", projectID.String()),
					zap.Error(err))
			} else if len(metaList) > 0 {
				tableMetadataMap = make(map[string]*models.TableMetadata, len(metaList))
				for _, meta := range metaList {
					tableMetadataMap[meta.TableName] = meta
				}
			}
		}
	}

	// Build table details
	tables := make(map[string]models.TableDetail)
	for _, tableName := range tableNames {
		entity := entityByTable[tableName]
		if entity == nil {
			// Expected when user requests a non-entity table
			s.logger.Debug("No entity found for table",
				zap.String("table", tableName),
			)
			continue
		}

		schemaColumns := columnsByTable[tableName]

		// Build column details by merging column metadata with schema
		columnDetails := make([]models.ColumnDetail, 0, len(schemaColumns))
		for _, col := range schemaColumns {
			detail := models.ColumnDetail{
				Name:         col.ColumnName,
				IsPrimaryKey: col.IsPrimaryKey,
			}

			// Get FK info from relationships (source of truth for FK target and association)
			if fkData, ok := fkByColumnID[col.ID]; ok {
				detail.IsForeignKey = true
				detail.ForeignTable = fkData.targetTable
				detail.FKAssociation = fkData.association
			}

			// Merge column metadata if available (Description, SemanticType, Role, EnumValues)
			if meta, ok := columnMetadataByID[col.ID]; ok {
				if meta.Description != nil {
					detail.Description = *meta.Description
				}
				if meta.SemanticType != nil {
					detail.SemanticType = *meta.SemanticType
				}
				if meta.Role != nil {
					detail.Role = *meta.Role
				}
				// Convert enum features to EnumValue slice
				if enumFeatures := meta.GetEnumFeatures(); enumFeatures != nil && len(enumFeatures.Values) > 0 {
					detail.EnumValues = make([]models.EnumValue, 0, len(enumFeatures.Values))
					for _, v := range enumFeatures.Values {
						detail.EnumValues = append(detail.EnumValues, models.EnumValue{
							Value: v.Value,
							Label: v.Label,
						})
					}
				}
			}

			columnDetails = append(columnDetails, detail)
		}

		detail := models.TableDetail{
			Schema:       entity.PrimarySchema,
			BusinessName: entity.Name,
			Description:  entity.Description,
			Columns:      columnDetails,
		}

		// Merge table metadata if available
		if tableMetadataMap != nil {
			if meta, ok := tableMetadataMap[tableName]; ok {
				if meta.UsageNotes != nil && *meta.UsageNotes != "" {
					detail.UsageNotes = *meta.UsageNotes
				}
				if meta.IsEphemeral {
					detail.IsEphemeral = true
				}
				if meta.PreferredAlternative != nil && *meta.PreferredAlternative != "" {
					detail.PreferredAlternative = *meta.PreferredAlternative
				}
			}
		}

		tables[tableName] = detail
	}

	return &models.OntologyColumnsContext{
		Tables: tables,
	}, nil
}

// Ensure ontologyContextService implements OntologyContextService at compile time.
var _ OntologyContextService = (*ontologyContextService)(nil)
