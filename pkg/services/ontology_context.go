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

	// GetTablesContext returns table summaries, optionally filtered by table names.
	GetTablesContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyTablesContext, error)

	// GetColumnsContext returns full column details for specified tables.
	// Always requires table filter to manage response size.
	GetColumnsContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyColumnsContext, error)
}

type ontologyContextService struct {
	ontologyRepo      repositories.OntologyRepository
	schemaRepo        repositories.SchemaRepository
	tableMetadataRepo repositories.TableMetadataRepository
	projectService    ProjectService
	logger            *zap.Logger
}

// NewOntologyContextService creates a new OntologyContextService.
func NewOntologyContextService(
	ontologyRepo repositories.OntologyRepository,
	schemaRepo repositories.SchemaRepository,
	tableMetadataRepo repositories.TableMetadataRepository,
	projectService ProjectService,
	logger *zap.Logger,
) OntologyContextService {
	return &ontologyContextService{
		ontologyRepo:      ontologyRepo,
		schemaRepo:        schemaRepo,
		tableMetadataRepo: tableMetadataRepo,
		projectService:    projectService,
		logger:            logger,
	}
}

// GetDomainContext returns high-level domain information.
func (s *ontologyContextService) GetDomainContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyDomainContext, error) {
	// Get active ontology for domain summary
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found")
	}

	// Get column count from schema tables
	columnCount, err := s.schemaRepo.GetColumnCountByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get column count: %w", err)
	}

	// Build domain info - TableCount from ontology column_details keys, ColumnCount from schema
	domainInfo := models.DomainInfo{
		TableCount:  ontology.TableCount(),
		ColumnCount: columnCount,
	}

	// Use domain summary if available (populated by Ontology Finalization)
	if ontology.DomainSummary != nil {
		domainInfo.Description = ontology.DomainSummary.Description
		domainInfo.PrimaryDomains = ontology.DomainSummary.Domains
		domainInfo.Conventions = ontology.DomainSummary.Conventions
	}

	return &models.OntologyDomainContext{
		Domain: domainInfo,
	}, nil
}

// GetTablesContext returns table summaries, optionally filtered by table names.
func (s *ontologyContextService) GetTablesContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyTablesContext, error) {
	// Get active ontology (contains enriched column_details)
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

	// If no filter provided, return all tables from ontology column_details
	tablesToInclude := tableNames
	if len(tablesToInclude) == 0 {
		for tableName := range ontology.ColumnDetails {
			tablesToInclude = append(tablesToInclude, tableName)
		}
	}

	// Get columns for the requested tables
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tablesToInclude)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Fetch table metadata keyed by table name (joins with engine_schema_tables)
	var tableMetadataMap map[string]*models.TableMetadata
	if s.tableMetadataRepo != nil {
		tableMetadataMap, err = s.tableMetadataRepo.ListByTableNames(ctx, projectID, tablesToInclude)
		if err != nil {
			return nil, fmt.Errorf("failed to get table metadata: %w", err)
		}
	}

	// Build table summaries
	tables := make(map[string]models.TableSummary)
	for _, tableName := range tablesToInclude {
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

			// Merge enriched data if available (Role, FKAssociation, HasEnumValues)
			if enriched, ok := tableEnriched[col.ColumnName]; ok {
				overview.Role = enriched.Role
				overview.FKAssociation = enriched.FKAssociation
				overview.HasEnumValues = len(enriched.EnumValues) > 0
			}

			columns = append(columns, overview)
		}

		summary := models.TableSummary{
			ColumnCount: len(schemaColumns),
			Columns:     columns,
		}

		// Merge table metadata if available
		if meta, ok := tableMetadataMap[tableName]; ok {
			if meta.Description != nil && *meta.Description != "" {
				summary.Description = *meta.Description
			}
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

	// Get active ontology (contains enriched column_details)
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

	// Get columns for the requested tables
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Fetch table metadata keyed by table name (joins with engine_schema_tables)
	var tableMetadataMap map[string]*models.TableMetadata
	if s.tableMetadataRepo != nil {
		tableMetadataMap, err = s.tableMetadataRepo.ListByTableNames(ctx, projectID, tableNames)
		if err != nil {
			return nil, fmt.Errorf("failed to get table metadata: %w", err)
		}
	}

	// Build table details
	tables := make(map[string]models.TableDetail)
	for _, tableName := range tableNames {
		schemaColumns := columnsByTable[tableName]
		tableEnriched := enrichedColumns[tableName] // nil if not enriched

		// Build column details by merging enriched data with schema
		columnDetails := make([]models.ColumnDetail, 0, len(schemaColumns))
		for _, col := range schemaColumns {
			// Check if we have enriched data for this column
			if enriched, ok := tableEnriched[col.ColumnName]; ok {
				// Use enriched data + overlay current schema PK info
				enriched.IsPrimaryKey = col.IsPrimaryKey
				columnDetails = append(columnDetails, enriched)
			} else {
				// Fall back to schema-only (no enrichment yet)
				columnDetails = append(columnDetails, models.ColumnDetail{
					Name:         col.ColumnName,
					IsPrimaryKey: col.IsPrimaryKey,
				})
			}
		}

		detail := models.TableDetail{
			Columns: columnDetails,
		}

		// Merge table metadata if available
		if meta, ok := tableMetadataMap[tableName]; ok {
			if meta.Description != nil && *meta.Description != "" {
				detail.Description = *meta.Description
			}
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

		tables[tableName] = detail
	}

	return &models.OntologyColumnsContext{
		Tables: tables,
	}, nil
}

// Ensure ontologyContextService implements OntologyContextService at compile time.
var _ OntologyContextService = (*ontologyContextService)(nil)
