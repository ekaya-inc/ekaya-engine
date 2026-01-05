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
	ontologyRepo     repositories.OntologyRepository
	entityRepo       repositories.OntologyEntityRepository
	relationshipRepo repositories.EntityRelationshipRepository
	schemaRepo       repositories.SchemaRepository
	projectService   ProjectService // Reserved for Phase 2/3: project-level domain aggregation
	logger           *zap.Logger
}

// NewOntologyContextService creates a new OntologyContextService.
func NewOntologyContextService(
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	schemaRepo repositories.SchemaRepository,
	projectService ProjectService,
	logger *zap.Logger,
) OntologyContextService {
	return &ontologyContextService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relationshipRepo,
		schemaRepo:       schemaRepo,
		projectService:   projectService,
		logger:           logger,
	}
}

// GetDomainContext returns high-level domain information.
func (s *ontologyContextService) GetDomainContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyDomainContext, error) {
	// Get active ontology (only for checking it exists and domain summary)
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// Get entities from normalized table
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
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
func (s *ontologyContextService) GetEntitiesContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyEntitiesContext, error) {
	// Get active ontology
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// Get entities
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
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
				Table:  rel.SourceColumnTable,
				Column: rel.SourceColumnName,
				Role:   rel.Association,
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
	// Get active ontology (contains enriched column_details for FK roles)
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// Build enriched column lookup from ontology.ColumnDetails for FK roles
	// Key: tableName -> columnName -> ColumnDetail
	enrichedColumns := make(map[string]map[string]models.ColumnDetail)
	if ontology.ColumnDetails != nil {
		for tableName, cols := range ontology.ColumnDetails {
			enrichedColumns[tableName] = make(map[string]models.ColumnDetail)
			for _, col := range cols {
				enrichedColumns[tableName][col.Name] = col
			}
		}
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

	// Get columns for the requested tables
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tablesToInclude)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Get relationships involving these tables
	relationships, err := s.relationshipRepo.GetByTables(ctx, projectID, tablesToInclude)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationships: %w", err)
	}

	// Build relationship index by source table
	relationshipsByTable := make(map[string][]*models.EntityRelationship)
	for _, rel := range relationships {
		relationshipsByTable[rel.SourceColumnTable] = append(relationshipsByTable[rel.SourceColumnTable], rel)
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
		tableEnriched := enrichedColumns[tableName] // nil if not enriched

		// Build column overview from schema columns, merging enriched data
		columns := make([]models.ColumnOverview, 0, len(schemaColumns))
		for _, col := range schemaColumns {
			overview := models.ColumnOverview{
				Name:         col.ColumnName,
				Type:         col.DataType,
				IsPrimaryKey: col.IsPrimaryKey,
			}

			// Merge enriched data if available (Role, FKRole, HasEnumValues)
			if enriched, ok := tableEnriched[col.ColumnName]; ok {
				overview.Role = enriched.Role
				overview.FKRole = enriched.FKRole
				overview.HasEnumValues = len(enriched.EnumValues) > 0
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

		tables[tableName] = models.TableSummary{
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

	// Get active ontology (also contains enriched column_details)
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// Build enriched column lookup from ontology.ColumnDetails
	// Key: tableName -> columnName -> ColumnDetail
	enrichedColumns := make(map[string]map[string]models.ColumnDetail)
	if ontology.ColumnDetails != nil {
		for tableName, cols := range ontology.ColumnDetails {
			enrichedColumns[tableName] = make(map[string]models.ColumnDetail)
			for _, col := range cols {
				enrichedColumns[tableName][col.Name] = col
			}
		}
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

	// Get columns for the requested tables
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Get relationships to determine FK info
	relationships, err := s.relationshipRepo.GetByTables(ctx, projectID, tableNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationships: %w", err)
	}

	// Build FK info index: table -> column -> target table
	fkInfo := make(map[string]map[string]string)
	for _, rel := range relationships {
		if fkInfo[rel.SourceColumnTable] == nil {
			fkInfo[rel.SourceColumnTable] = make(map[string]string)
		}
		fkInfo[rel.SourceColumnTable][rel.SourceColumnName] = rel.TargetColumnTable
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
		tableEnriched := enrichedColumns[tableName] // nil if not enriched

		// Build column details by merging enriched data with schema
		columnDetails := make([]models.ColumnDetail, 0, len(schemaColumns))
		for _, col := range schemaColumns {
			// Get current FK info from relationships (source of truth)
			tableFKInfo := fkInfo[tableName]
			foreignTable := ""
			isForeignKey := false
			if tableFKInfo != nil {
				if ft, ok := tableFKInfo[col.ColumnName]; ok {
					foreignTable = ft
					isForeignKey = true
				}
			}

			// Check if we have enriched data for this column
			if enriched, ok := tableEnriched[col.ColumnName]; ok {
				// Use enriched data + overlay current schema PK/FK info
				enriched.IsPrimaryKey = col.IsPrimaryKey
				enriched.IsForeignKey = isForeignKey
				enriched.ForeignTable = foreignTable
				columnDetails = append(columnDetails, enriched)
			} else {
				// Fall back to schema-only (no enrichment yet)
				columnDetails = append(columnDetails, models.ColumnDetail{
					Name:         col.ColumnName,
					IsPrimaryKey: col.IsPrimaryKey,
					IsForeignKey: isForeignKey,
					ForeignTable: foreignTable,
				})
			}
		}

		tables[tableName] = models.TableDetail{
			Schema:       entity.PrimarySchema,
			BusinessName: entity.Name,
			Description:  entity.Description,
			Columns:      columnDetails,
		}
	}

	return &models.OntologyColumnsContext{
		Tables: tables,
	}, nil
}

// computeEntityOccurrences derives entity occurrences from inbound relationships.
// Each inbound relationship represents an occurrence of this entity at the source column location.
func (s *ontologyContextService) computeEntityOccurrences(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	// Get all inbound relationships (where this entity is the target)
	relationships, err := s.relationshipRepo.GetByTargetEntity(ctx, entityID)
	if err != nil {
		s.logger.Error("Failed to get relationships by target entity",
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get relationships by target entity: %w", err)
	}

	// Convert relationships to occurrences
	occurrences := make([]*models.OntologyEntityOccurrence, 0, len(relationships))
	for _, rel := range relationships {
		occurrences = append(occurrences, &models.OntologyEntityOccurrence{
			ID:         rel.ID, // Use relationship ID as occurrence ID
			EntityID:   entityID,
			SchemaName: rel.SourceColumnSchema,
			TableName:  rel.SourceColumnTable,
			ColumnName: rel.SourceColumnName,
			Role:       rel.Association, // Association becomes the role
			Confidence: rel.Confidence,
			CreatedAt:  rel.CreatedAt,
		})
	}

	return occurrences, nil
}

// Ensure ontologyContextService implements OntologyContextService at compile time.
var _ OntologyContextService = (*ontologyContextService)(nil)
