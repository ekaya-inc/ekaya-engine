package services

import (
	"context"
	"fmt"

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

// DeterministicRelationshipService discovers entity relationships from FK constraints.
type DeterministicRelationshipService interface {
	// DiscoverRelationships discovers relationships from FK constraints for a datasource.
	// Requires entities to exist before calling.
	DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*DiscoveryResult, error)
}

type deterministicRelationshipService struct {
	datasourceService DatasourceService
	adapterFactory    datasource.DatasourceAdapterFactory
	ontologyRepo      repositories.OntologyRepository
	entityRepo        repositories.OntologyEntityRepository
	relationshipRepo  repositories.EntityRelationshipRepository
}

// NewDeterministicRelationshipService creates a new DeterministicRelationshipService.
func NewDeterministicRelationshipService(
	datasourceService DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
) DeterministicRelationshipService {
	return &deterministicRelationshipService{
		datasourceService: datasourceService,
		adapterFactory:    adapterFactory,
		ontologyRepo:      ontologyRepo,
		entityRepo:        entityRepo,
		relationshipRepo:  relationshipRepo,
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

	// Get datasource with decrypted config
	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer to get FK constraints
	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	// Check if the datasource supports FK discovery
	if !discoverer.SupportsForeignKeys() {
		return &DiscoveryResult{}, nil
	}

	// Discover FK constraints from the database
	fks, err := discoverer.DiscoverForeignKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover foreign keys: %w", err)
	}

	// Build lookup maps for efficient matching
	// entityByPK: maps "schema.table.column" to entity (for finding target entity by PK)
	entityByPK := make(map[string]*models.OntologyEntity)
	for _, entity := range entities {
		key := fmt.Sprintf("%s.%s.%s", entity.PrimarySchema, entity.PrimaryTable, entity.PrimaryColumn)
		entityByPK[key] = entity
	}

	// occByTableColumn: maps "schema.table.column" to entity ID (for finding source entity via occurrence)
	occByTableColumn := make(map[string]uuid.UUID)
	for _, occ := range occurrences {
		key := fmt.Sprintf("%s.%s.%s", occ.SchemaName, occ.TableName, occ.ColumnName)
		occByTableColumn[key] = occ.EntityID
	}

	// entityByID: for looking up entity by ID
	entityByID := make(map[uuid.UUID]*models.OntologyEntity)
	for _, entity := range entities {
		entityByID[entity.ID] = entity
	}

	// Process each FK constraint
	var fkCount int
	for _, fk := range fks {
		// Find target entity: FK target column should be an entity's primary column
		targetKey := fmt.Sprintf("%s.%s.%s", fk.TargetSchema, fk.TargetTable, fk.TargetColumn)
		targetEntity, found := entityByPK[targetKey]
		if !found {
			// FK target doesn't point to a known entity's PK - skip
			continue
		}

		// Find source entity: Look for an occurrence in the source table
		// The source entity might be found by:
		// 1. The FK column itself is an occurrence of some entity
		// 2. Any column in the source table is associated with an entity
		var sourceEntity *models.OntologyEntity

		// First try: FK column itself might be an entity occurrence
		sourceKey := fmt.Sprintf("%s.%s.%s", fk.SourceSchema, fk.SourceTable, fk.SourceColumn)
		if entityID, ok := occByTableColumn[sourceKey]; ok {
			sourceEntity = entityByID[entityID]
		}

		// If FK column isn't an occurrence, find any entity that has an occurrence in this table
		if sourceEntity == nil {
			for _, occ := range occurrences {
				if occ.SchemaName == fk.SourceSchema && occ.TableName == fk.SourceTable {
					sourceEntity = entityByID[occ.EntityID]
					break
				}
			}
		}

		// If no source entity found in this table, skip
		if sourceEntity == nil {
			continue
		}

		// Don't create self-referencing relationships
		if sourceEntity.ID == targetEntity.ID {
			continue
		}

		// Create the relationship
		rel := &models.EntityRelationship{
			OntologyID:         ontology.ID,
			SourceEntityID:     sourceEntity.ID,
			TargetEntityID:     targetEntity.ID,
			SourceColumnSchema: fk.SourceSchema,
			SourceColumnTable:  fk.SourceTable,
			SourceColumnName:   fk.SourceColumn,
			TargetColumnSchema: fk.TargetSchema,
			TargetColumnTable:  fk.TargetTable,
			TargetColumnName:   fk.TargetColumn,
			DetectionMethod:    models.DetectionMethodForeignKey,
			Confidence:         1.0,
			Status:             models.RelationshipStatusConfirmed,
		}

		err := s.relationshipRepo.Create(ctx, rel)
		if err != nil {
			return nil, fmt.Errorf("create relationship: %w", err)
		}

		fkCount++
	}

	return &DiscoveryResult{
		FKRelationships:       fkCount,
		InferredRelationships: 0, // Not implemented yet
		TotalRelationships:    fkCount,
	}, nil
}
