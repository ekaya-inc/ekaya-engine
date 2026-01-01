package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// DiscoveryResult contains the results of deterministic relationship discovery.
type DiscoveryResult struct {
	FKRelationships       int `json:"fk_relationships"`
	InferredRelationships int `json:"inferred_relationships"`
	TotalRelationships    int `json:"total_relationships"`
}

// DeterministicRelationshipService discovers entity relationships from FK constraints
// and PK-match inference.
type DeterministicRelationshipService interface {
	// DiscoverRelationships discovers relationships from FK constraints and PK-match
	// inference for a datasource. Requires entities to exist before calling.
	DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*DiscoveryResult, error)

	// GetByProject returns all entity relationships for a project.
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error)
}

type deterministicRelationshipService struct {
	datasourceService DatasourceService
	adapterFactory    datasource.DatasourceAdapterFactory
	ontologyRepo      repositories.OntologyRepository
	entityRepo        repositories.OntologyEntityRepository
	relationshipRepo  repositories.EntityRelationshipRepository
	schemaRepo        repositories.SchemaRepository
}

// NewDeterministicRelationshipService creates a new DeterministicRelationshipService.
func NewDeterministicRelationshipService(
	datasourceService DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	schemaRepo repositories.SchemaRepository,
) DeterministicRelationshipService {
	return &deterministicRelationshipService{
		datasourceService: datasourceService,
		adapterFactory:    adapterFactory,
		ontologyRepo:      ontologyRepo,
		entityRepo:        entityRepo,
		relationshipRepo:  relationshipRepo,
		schemaRepo:        schemaRepo,
	}
}

func (s *deterministicRelationshipService) DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*DiscoveryResult, error) {
	// Get active ontology for the project
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found for project")
	}

	// Get all entities for this ontology
	entities, err := s.entityRepo.GetByOntology(ctx, ontology.ID)
	if err != nil {
		return nil, fmt.Errorf("get entities: %w", err)
	}
	if len(entities) == 0 {
		return &DiscoveryResult{}, nil // No entities, no relationships to discover
	}

	// Get all entity occurrences for this project
	occurrences, err := s.entityRepo.GetAllOccurrencesByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get occurrences: %w", err)
	}

	// Build lookup maps for efficient entity matching
	// entityByID: for looking up entity by ID
	entityByID := make(map[uuid.UUID]*models.OntologyEntity)
	for _, entity := range entities {
		entityByID[entity.ID] = entity
	}

	// occByTable: maps "schema.table" to first entity with occurrence in that table
	occByTable := make(map[string]*models.OntologyEntity)
	for _, occ := range occurrences {
		key := fmt.Sprintf("%s.%s", occ.SchemaName, occ.TableName)
		if _, exists := occByTable[key]; !exists {
			occByTable[key] = entityByID[occ.EntityID]
		}
	}

	// =========================================================================
	// Phase 1: FK Relationships (Gold Standard)
	// All FKs from engine_schema_relationships become entity relationships.
	// Find entities by table occurrence, not by PK match.
	// =========================================================================

	// Get all schema relationships (FKs) for this datasource
	schemaRels, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list schema relationships: %w", err)
	}

	// Load tables and columns to resolve IDs to names
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	for _, t := range tables {
		tableByID[t.ID] = t
	}

	columns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}
	columnByID := make(map[uuid.UUID]*models.SchemaColumn)
	for _, c := range columns {
		columnByID[c.ID] = c
	}

	// Process each FK from schema relationships
	var fkCount int
	for _, schemaRel := range schemaRels {
		// Resolve source column/table
		sourceCol := columnByID[schemaRel.SourceColumnID]
		sourceTable := tableByID[schemaRel.SourceTableID]
		if sourceCol == nil || sourceTable == nil {
			continue
		}

		// Resolve target column/table
		targetCol := columnByID[schemaRel.TargetColumnID]
		targetTable := tableByID[schemaRel.TargetTableID]
		if targetCol == nil || targetTable == nil {
			continue
		}

		// Find source entity by table occurrence
		sourceKey := fmt.Sprintf("%s.%s", sourceTable.SchemaName, sourceTable.TableName)
		sourceEntity := occByTable[sourceKey]
		if sourceEntity == nil {
			continue // No entity associated with source table
		}

		// Find target entity by table occurrence
		targetKey := fmt.Sprintf("%s.%s", targetTable.SchemaName, targetTable.TableName)
		targetEntity := occByTable[targetKey]
		if targetEntity == nil {
			continue // No entity associated with target table
		}

		// Don't create self-referencing relationships
		if sourceEntity.ID == targetEntity.ID {
			continue
		}

		// Create the relationship (DB handles duplicates via ON CONFLICT)
		rel := &models.EntityRelationship{
			OntologyID:         ontology.ID,
			SourceEntityID:     sourceEntity.ID,
			TargetEntityID:     targetEntity.ID,
			SourceColumnSchema: sourceTable.SchemaName,
			SourceColumnTable:  sourceTable.TableName,
			SourceColumnName:   sourceCol.ColumnName,
			TargetColumnSchema: targetTable.SchemaName,
			TargetColumnTable:  targetTable.TableName,
			TargetColumnName:   targetCol.ColumnName,
			DetectionMethod:    models.DetectionMethodForeignKey,
			Confidence:         1.0,
			Status:             models.RelationshipStatusConfirmed,
		}

		err := s.relationshipRepo.Create(ctx, rel)
		if err != nil {
			return nil, fmt.Errorf("create FK relationship: %w", err)
		}

		fkCount++
	}

	// =========================================================================
	// Phase 2: PK-Match Inference
	// Find non-PK columns that can join to entity PKs by testing actual SQL joins.
	// Skip if both columns are auto-increment PKs.
	// =========================================================================

	// Get datasource to create schema discoverer for join analysis
	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	// Build list of "entity reference columns" - columns in entity tables that could be
	// join targets (PKs, unique columns, *_id naming, high cardinality)
	type entityRefColumn struct {
		entity *models.OntologyEntity
		column *models.SchemaColumn
		schema string
		table  string
	}
	var entityRefColumns []entityRefColumn

	for _, entity := range entities {
		// Find all columns in this entity's primary table
		for _, col := range columns {
			table, ok := tableByID[col.SchemaTableID]
			if !ok {
				continue
			}
			if table.SchemaName != entity.PrimarySchema || table.TableName != entity.PrimaryTable {
				continue
			}

			// Include if: PK, unique, or passes candidate filters
			isCandidate := col.IsPrimaryKey || col.IsUnique ||
				isEntityReferenceName(col.ColumnName) ||
				(col.DistinctCount != nil && *col.DistinctCount >= 20)

			if !isCandidate {
				continue
			}

			// Exclude types unlikely to be join keys
			if isPKMatchExcludedType(col) {
				continue
			}

			entityRefColumns = append(entityRefColumns, entityRefColumn{
				entity: entity,
				column: col,
				schema: table.SchemaName,
				table:  table.TableName,
			})
		}
	}

	// Build list of filtered FK candidate columns (columns that could reference entity columns)
	// Group by type for efficient matching
	candidatesByType := make(map[string][]*pkMatchCandidate)
	for _, col := range columns {
		table, ok := tableByID[col.SchemaTableID]
		if !ok {
			continue
		}

		// Exclude types unlikely to be entity references
		if isPKMatchExcludedType(col) {
			continue
		}

		// Exclude names unlikely to be entity references
		if isPKMatchExcludedName(col.ColumnName) {
			continue
		}

		// Check cardinality if stats available
		if col.DistinctCount != nil {
			if *col.DistinctCount < 20 {
				continue
			}
			if table.RowCount != nil && *table.RowCount > 0 {
				ratio := float64(*col.DistinctCount) / float64(*table.RowCount)
				if ratio < 0.05 {
					continue
				}
			}
		}

		cwt := &pkMatchCandidate{
			column: col,
			schema: table.SchemaName,
			table:  table.TableName,
		}
		candidatesByType[col.DataType] = append(candidatesByType[col.DataType], cwt)
	}

	// For each entity reference column, find candidates with matching type and test joins
	var inferredCount int
	for _, ref := range entityRefColumns {
		refType := ref.column.DataType
		candidates := candidatesByType[refType]

		for _, candidate := range candidates {
			// Skip if same table (self-reference)
			if candidate.schema == ref.schema && candidate.table == ref.table {
				continue
			}

			// Skip PK-to-PK matches (both auto-increment)
			if ref.column.IsPrimaryKey && candidate.column.IsPrimaryKey {
				continue
			}

			// Find source entity for candidate's table
			sourceEntity := occByTable[fmt.Sprintf("%s.%s", candidate.schema, candidate.table)]
			if sourceEntity == nil {
				continue
			}

			// Don't create self-referencing entity relationships
			if sourceEntity.ID == ref.entity.ID {
				continue
			}

			// Run join analysis
			joinResult, err := discoverer.AnalyzeJoin(ctx,
				candidate.schema, candidate.table, candidate.column.ColumnName,
				ref.schema, ref.table, ref.column.ColumnName)
			if err != nil {
				// Skip this candidate on join error
				continue
			}

			// Calculate orphan rate
			// Use table's row count if available, otherwise use join result approximation
			var sourceRowCount int64
			if table, ok := tableByID[candidate.column.SchemaTableID]; ok && table.RowCount != nil && *table.RowCount > 0 {
				sourceRowCount = *table.RowCount
			} else if joinResult.SourceMatched > 0 {
				// Approximate from join result
				sourceRowCount = joinResult.SourceMatched + joinResult.OrphanCount
			} else {
				continue // Can't calculate orphan rate
			}

			orphanRate := float64(joinResult.OrphanCount) / float64(sourceRowCount)

			// Determine status based on orphan rate thresholds
			var status string
			if orphanRate < 0.05 {
				// < 5% orphans: confirmed relationship
				status = models.RelationshipStatusConfirmed
			} else if orphanRate < 0.20 {
				// 5-20% orphans: pending (needs review)
				status = models.RelationshipStatusPending
			} else {
				// > 20% orphans: skip, not a valid relationship
				continue
			}

			// Calculate confidence: lower orphan rate = higher confidence
			confidence := 0.9 - (orphanRate * 2) // 0% orphans = 0.9, 20% orphans = 0.5
			if confidence < 0.5 {
				confidence = 0.5
			}

			// Create the relationship (database handles duplicates via ON CONFLICT)
			rel := &models.EntityRelationship{
				OntologyID:         ontology.ID,
				SourceEntityID:     sourceEntity.ID,
				TargetEntityID:     ref.entity.ID,
				SourceColumnSchema: candidate.schema,
				SourceColumnTable:  candidate.table,
				SourceColumnName:   candidate.column.ColumnName,
				TargetColumnSchema: ref.schema,
				TargetColumnTable:  ref.table,
				TargetColumnName:   ref.column.ColumnName,
				DetectionMethod:    models.DetectionMethodPKMatch,
				Confidence:         confidence,
				Status:             status,
			}

			err = s.relationshipRepo.Create(ctx, rel)
			if err != nil {
				return nil, fmt.Errorf("create pk-match relationship: %w", err)
			}

			inferredCount++
		}
	}

	return &DiscoveryResult{
		FKRelationships:       fkCount,
		InferredRelationships: inferredCount,
		TotalRelationships:    fkCount + inferredCount,
	}, nil
}

func (s *deterministicRelationshipService) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	return s.relationshipRepo.GetByProject(ctx, projectID)
}

// pkMatchCandidate bundles a column with its table location info for PK-match discovery.
type pkMatchCandidate struct {
	column *models.SchemaColumn
	schema string
	table  string
}

// isPKMatchExcludedType returns true for types unlikely to be entity references.
// For text columns, checks if they have uniform length (likely UUIDs or other ID formats).
func isPKMatchExcludedType(col *models.SchemaColumn) bool {
	lower := strings.ToLower(col.DataType)

	// Boolean types
	if strings.Contains(lower, "bool") {
		return true
	}

	// Timestamp/date types
	if strings.Contains(lower, "timestamp") || strings.Contains(lower, "date") {
		return true
	}

	// Text types - allow if uniform length (likely UUID or other ID format)
	if lower == "text" || strings.Contains(lower, "varchar") || strings.Contains(lower, "char") {
		// If we have length stats and they're uniform, this could be an ID field
		if col.MinLength != nil && col.MaxLength != nil && *col.MinLength == *col.MaxLength && *col.MinLength > 0 {
			return false // Uniform length text - could be UUID, include it
		}
		return true // Variable length text - exclude
	}

	// JSON types
	if strings.Contains(lower, "json") {
		return true
	}

	return false
}

// isPKMatchExcludedName returns true for column names unlikely to be entity references.
func isPKMatchExcludedName(columnName string) bool {
	lower := strings.ToLower(columnName)

	// Timestamp patterns
	if strings.HasSuffix(lower, "_at") || strings.HasSuffix(lower, "_date") {
		return true
	}

	// Boolean flag patterns
	if strings.HasPrefix(lower, "is_") || strings.HasPrefix(lower, "has_") {
		return true
	}

	// Status/type/flag patterns (often low-cardinality enums stored as int)
	if strings.HasSuffix(lower, "_status") || strings.HasSuffix(lower, "_type") || strings.HasSuffix(lower, "_flag") {
		return true
	}

	// Count/amount patterns
	if strings.HasSuffix(lower, "_count") || strings.HasSuffix(lower, "_amount") || strings.HasSuffix(lower, "_total") {
		return true
	}

	return false
}
