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
	schemaRepo         repositories.SchemaRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	tableMetadataRepo  repositories.TableMetadataRepository
	projectService     ProjectService
	logger             *zap.Logger
}

// NewOntologyContextService creates a new OntologyContextService.
func NewOntologyContextService(
	schemaRepo repositories.SchemaRepository,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	tableMetadataRepo repositories.TableMetadataRepository,
	projectService ProjectService,
	logger *zap.Logger,
) OntologyContextService {
	return &ontologyContextService{
		schemaRepo:         schemaRepo,
		columnMetadataRepo: columnMetadataRepo,
		tableMetadataRepo:  tableMetadataRepo,
		projectService:     projectService,
		logger:             logger,
	}
}

// GetDomainContext returns high-level domain information.
func (s *ontologyContextService) GetDomainContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyDomainContext, error) {
	// Get project for domain summary
	project, err := s.projectService.GetByID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	if project == nil {
		return nil, fmt.Errorf("project not found")
	}

	// Get table count from schema
	tableCount, err := s.schemaRepo.GetTableCountByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get table count: %w", err)
	}

	// Get column count from schema
	columnCount, err := s.schemaRepo.GetColumnCountByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get column count: %w", err)
	}

	domainInfo := models.DomainInfo{
		TableCount:  tableCount,
		ColumnCount: columnCount,
	}

	// Use domain summary if available (populated by Ontology Finalization)
	if project.DomainSummary != nil {
		domainInfo.Description = project.DomainSummary.Description
		domainInfo.PrimaryDomains = project.DomainSummary.Domains
		domainInfo.Conventions = project.DomainSummary.Conventions
	}

	return &models.OntologyDomainContext{
		Domain: domainInfo,
	}, nil
}

// GetTablesContext returns table summaries, optionally filtered by table names.
func (s *ontologyContextService) GetTablesContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyTablesContext, error) {
	// If no filter provided, get all selected table names from schema
	tablesToInclude := tableNames
	if len(tablesToInclude) == 0 {
		var err error
		tablesToInclude, err = s.schemaRepo.GetSelectedTableNamesByProject(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get table names: %w", err)
		}
	}

	// Get columns for the requested tables
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tablesToInclude)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Collect all schema column IDs for bulk metadata lookup
	var allColumnIDs []uuid.UUID
	for _, cols := range columnsByTable {
		for _, col := range cols {
			allColumnIDs = append(allColumnIDs, col.ID)
		}
	}

	// Fetch column metadata in bulk, indexed by SchemaColumnID
	metadataByColumnID, err := s.getColumnMetadataMap(ctx, allColumnIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get column metadata: %w", err)
	}

	// Fetch schema tables keyed by table name (for row_count)
	schemaTablesMap, err := s.schemaRepo.GetTablesByNames(ctx, projectID, tablesToInclude)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema tables: %w", err)
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

		// Build column overview from schema columns, merging column metadata
		columns := make([]models.ColumnOverview, 0, len(schemaColumns))
		for _, col := range schemaColumns {
			overview := models.ColumnOverview{
				Name:         col.ColumnName,
				Type:         col.DataType,
				IsPrimaryKey: col.IsPrimaryKey,
			}

			// Merge column metadata if available
			if meta, ok := metadataByColumnID[col.ID]; ok {
				if meta.Role != nil {
					overview.Role = *meta.Role
				}
				if meta.Features.IdentifierFeatures != nil {
					overview.FKAssociation = meta.Features.IdentifierFeatures.FKAssociation
				}
				overview.HasEnumValues = meta.Features.EnumFeatures != nil && len(meta.Features.EnumFeatures.Values) > 0
				overview.HasDescription = meta.Description != nil && *meta.Description != ""
			}

			columns = append(columns, overview)
		}

		summary := models.TableSummary{
			ColumnCount: len(schemaColumns),
			Columns:     columns,
		}

		// Populate row count from schema table
		if st, ok := schemaTablesMap[tableName]; ok && st.RowCount != nil {
			summary.RowCount = *st.RowCount
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

	// Get columns for the requested tables
	columnsByTable, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Collect all schema column IDs for bulk metadata lookup
	var allColumnIDs []uuid.UUID
	for _, cols := range columnsByTable {
		for _, col := range cols {
			allColumnIDs = append(allColumnIDs, col.ID)
		}
	}

	// Fetch column metadata in bulk, indexed by SchemaColumnID
	metadataByColumnID, err := s.getColumnMetadataMap(ctx, allColumnIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get column metadata: %w", err)
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

		// Build column details from schema + column metadata
		columnDetails := make([]models.ColumnDetailInfo, 0, len(schemaColumns))
		for _, col := range schemaColumns {
			detail := models.ColumnDetailInfo{
				Name:         col.ColumnName,
				IsPrimaryKey: col.IsPrimaryKey,
			}

			// Merge column metadata if available
			if meta, ok := metadataByColumnID[col.ID]; ok {
				if meta.Description != nil {
					detail.Description = *meta.Description
				}
				if meta.SemanticType != nil {
					detail.SemanticType = *meta.SemanticType
				}
				if meta.Role != nil {
					detail.Role = *meta.Role
				}
				detail.Synonyms = meta.Features.Synonyms

				// FK info from identifier features
				if meta.Features.IdentifierFeatures != nil {
					idFeatures := meta.Features.IdentifierFeatures
					detail.FKAssociation = idFeatures.FKAssociation
					if idFeatures.FKTargetTable != "" {
						detail.IsForeignKey = true
						detail.ForeignTable = idFeatures.FKTargetTable
					}
				}

				// Enum values from enum features
				if meta.Features.EnumFeatures != nil {
					detail.EnumValues = columnEnumValuesToEnumValues(meta.Features.EnumFeatures.Values)
				}
			}

			columnDetails = append(columnDetails, detail)
		}

		tableDetail := models.TableDetail{
			Columns: columnDetails,
		}

		// Merge table metadata if available
		if meta, ok := tableMetadataMap[tableName]; ok {
			if meta.Description != nil && *meta.Description != "" {
				tableDetail.Description = *meta.Description
			}
			if meta.UsageNotes != nil && *meta.UsageNotes != "" {
				tableDetail.UsageNotes = *meta.UsageNotes
			}
			if meta.IsEphemeral {
				tableDetail.IsEphemeral = true
			}
			if meta.PreferredAlternative != nil && *meta.PreferredAlternative != "" {
				tableDetail.PreferredAlternative = *meta.PreferredAlternative
			}
		}

		tables[tableName] = tableDetail
	}

	return &models.OntologyColumnsContext{
		Tables: tables,
	}, nil
}

// getColumnMetadataMap fetches column metadata for the given schema column IDs
// and returns them indexed by SchemaColumnID for efficient lookup.
func (s *ontologyContextService) getColumnMetadataMap(ctx context.Context, schemaColumnIDs []uuid.UUID) (map[uuid.UUID]*models.ColumnMetadata, error) {
	result := make(map[uuid.UUID]*models.ColumnMetadata)
	if len(schemaColumnIDs) == 0 || s.columnMetadataRepo == nil {
		return result, nil
	}

	metadataList, err := s.columnMetadataRepo.GetBySchemaColumnIDs(ctx, schemaColumnIDs)
	if err != nil {
		return nil, err
	}

	for _, meta := range metadataList {
		result[meta.SchemaColumnID] = meta
	}
	return result, nil
}

// columnEnumValuesToEnumValues converts ColumnEnumValue (from column features)
// to EnumValue (used in API responses).
func columnEnumValuesToEnumValues(values []models.ColumnEnumValue) []models.EnumValue {
	if len(values) == 0 {
		return nil
	}
	result := make([]models.EnumValue, 0, len(values))
	for _, v := range values {
		ev := models.EnumValue{
			Value: v.Value,
			Label: v.Label,
		}
		if v.Count > 0 {
			count := v.Count
			ev.Count = &count
		}
		if v.Percentage > 0 {
			pct := v.Percentage
			ev.Percentage = &pct
		}
		result = append(result, ev)
	}
	return result
}

// Ensure ontologyContextService implements OntologyContextService at compile time.
var _ OntologyContextService = (*ontologyContextService)(nil)
