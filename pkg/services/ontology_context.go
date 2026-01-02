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
	ontologyRepo repositories.OntologyRepository
	entityRepo   repositories.OntologyEntityRepository
	schemaRepo   repositories.SchemaRepository
	logger       *zap.Logger
}

// NewOntologyContextService creates a new OntologyContextService.
func NewOntologyContextService(
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	schemaRepo repositories.SchemaRepository,
	logger *zap.Logger,
) OntologyContextService {
	return &ontologyContextService{
		ontologyRepo: ontologyRepo,
		entityRepo:   entityRepo,
		schemaRepo:   schemaRepo,
		logger:       logger,
	}
}

// GetDomainContext returns high-level domain information.
func (s *ontologyContextService) GetDomainContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyDomainContext, error) {
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

	// Get all entity occurrences for counting
	occurrences, err := s.entityRepo.GetAllOccurrencesByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity occurrences: %w", err)
	}

	// Build occurrence count map
	occurrenceCountByEntityID := make(map[uuid.UUID]int)
	for _, occ := range occurrences {
		occurrenceCountByEntityID[occ.EntityID]++
	}

	// Build domain info
	domainInfo := models.DomainInfo{
		TableCount:  ontology.TableCount(),
		ColumnCount: ontology.ColumnCount(),
	}

	if ontology.DomainSummary != nil {
		domainInfo.Description = ontology.DomainSummary.Description
		domainInfo.PrimaryDomains = ontology.DomainSummary.Domains
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

	// Build relationships from domain summary
	var relationships []models.RelationshipEdge
	if ontology.DomainSummary != nil && ontology.DomainSummary.RelationshipGraph != nil {
		relationships = ontology.DomainSummary.RelationshipGraph
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

	// Get all entity occurrences
	occurrences, err := s.entityRepo.GetAllOccurrencesByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity occurrences: %w", err)
	}

	// Get entity aliases for synonyms
	entityAliasesMap := make(map[uuid.UUID][]string)
	for _, entity := range entities {
		aliases, err := s.entityRepo.GetAliasesByEntity(ctx, entity.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get aliases for entity %s: %w", entity.Name, err)
		}
		synonyms := make([]string, 0, len(aliases))
		for _, alias := range aliases {
			synonyms = append(synonyms, alias.Alias)
		}
		entityAliasesMap[entity.ID] = synonyms
	}

	// Build occurrence map by entity ID
	occurrencesByEntityID := make(map[uuid.UUID][]models.EntityOccurrence)
	for _, occ := range occurrences {
		occurrencesByEntityID[occ.EntityID] = append(occurrencesByEntityID[occ.EntityID], models.EntityOccurrence{
			Table:  occ.TableName,
			Column: occ.ColumnName,
			Role:   occ.Role,
		})
	}

	// Build entity details map
	entityDetails := make(map[string]models.EntityDetail)
	for _, entity := range entities {
		// Get key columns from entity summary if available
		var keyColumns []models.KeyColumnInfo
		if ontology.EntitySummaries != nil {
			if summary, ok := ontology.EntitySummaries[entity.PrimaryTable]; ok && summary != nil {
				for _, kc := range summary.KeyColumns {
					keyColumns = append(keyColumns, models.KeyColumnInfo{
						Name:     kc.Name,
						Synonyms: kc.Synonyms,
					})
				}
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
	// Get active ontology
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// If no filter provided, return all tables
	tablesToInclude := tableNames
	if len(tablesToInclude) == 0 && ontology.EntitySummaries != nil {
		for tableName := range ontology.EntitySummaries {
			tablesToInclude = append(tablesToInclude, tableName)
		}
	}

	// Build table summaries
	tables := make(map[string]models.TableSummary)
	for _, tableName := range tablesToInclude {
		// Get entity summary from ontology
		entitySummary := ontology.EntitySummaries[tableName]
		if entitySummary == nil {
			s.logger.Warn("Table not found in ontology entity summaries",
				zap.String("table", tableName),
			)
			continue
		}

		// Get column details from ontology
		columnDetails := ontology.ColumnDetails[tableName]

		// Build column overview
		columns := make([]models.ColumnOverview, 0, len(columnDetails))
		for _, col := range columnDetails {
			columns = append(columns, models.ColumnOverview{
				Name:          col.Name,
				Type:          "", // Not available in ColumnDetail, will need schema info
				Role:          col.Role,
				IsPrimaryKey:  col.IsPrimaryKey,
				HasEnumValues: len(col.EnumValues) > 0,
			})
		}

		// Build table summary
		tables[tableName] = models.TableSummary{
			Schema:        "public", // Default, should be enriched from schema_tables
			BusinessName:  entitySummary.BusinessName,
			Description:   entitySummary.Description,
			Domain:        entitySummary.Domain,
			RowCount:      0, // Should be enriched from schema_tables
			ColumnCount:   entitySummary.ColumnCount,
			Synonyms:      entitySummary.Synonyms,
			Columns:       columns,
			Relationships: []models.TableRelationship{}, // TODO: derive from schema relationships
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

	// Get active ontology
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// Build table details
	tables := make(map[string]models.TableDetail)
	for _, tableName := range tableNames {
		// Get entity summary
		entitySummary := ontology.EntitySummaries[tableName]
		if entitySummary == nil {
			s.logger.Warn("Table not found in ontology entity summaries",
				zap.String("table", tableName),
			)
			continue
		}

		// Get column details from ontology (already in the right format)
		columnDetails := ontology.ColumnDetails[tableName]
		if columnDetails == nil {
			columnDetails = []models.ColumnDetail{}
		}

		tables[tableName] = models.TableDetail{
			Schema:       "public", // Default, should be enriched from schema_tables
			BusinessName: entitySummary.BusinessName,
			Description:  entitySummary.Description,
			Columns:      columnDetails,
		}
	}

	return &models.OntologyColumnsContext{
		Tables: tables,
	}, nil
}

// Ensure ontologyContextService implements OntologyContextService at compile time.
var _ OntologyContextService = (*ontologyContextService)(nil)
