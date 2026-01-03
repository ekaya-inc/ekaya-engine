package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// These adapters convert between the services package types and the dag package types.
// This allows the dag package to remain independent of the services package,
// avoiding import cycles.

// EntityDiscoveryAdapter adapts EntityDiscoveryService for the dag package.
type EntityDiscoveryAdapter struct {
	svc EntityDiscoveryService
}

// NewEntityDiscoveryAdapter creates a new adapter.
func NewEntityDiscoveryAdapter(svc EntityDiscoveryService) dag.EntityDiscoveryMethods {
	return &EntityDiscoveryAdapter{svc: svc}
}

func (a *EntityDiscoveryAdapter) IdentifyEntitiesFromDDL(ctx context.Context, projectID, ontologyID, datasourceID uuid.UUID) (int, []*models.SchemaTable, []*models.SchemaColumn, error) {
	return a.svc.IdentifyEntitiesFromDDL(ctx, projectID, ontologyID, datasourceID)
}

// EntityEnrichmentAdapter adapts EntityDiscoveryService for entity enrichment.
type EntityEnrichmentAdapter struct {
	svc        EntityDiscoveryService
	schemaRepo repositories.SchemaRepository
	getTenant  TenantContextFunc
}

// NewEntityEnrichmentAdapter creates a new adapter.
func NewEntityEnrichmentAdapter(svc EntityDiscoveryService, schemaRepo repositories.SchemaRepository, getTenant TenantContextFunc) dag.EntityEnrichmentMethods {
	return &EntityEnrichmentAdapter{svc: svc, schemaRepo: schemaRepo, getTenant: getTenant}
}

func (a *EntityEnrichmentAdapter) EnrichEntitiesWithLLM(ctx context.Context, projectID, ontologyID, datasourceID uuid.UUID) error {
	// Get tables and columns for enrichment context
	tenantCtx, cleanup, err := a.getTenant(ctx, projectID)
	if err != nil {
		return err
	}
	defer cleanup()

	tables, err := a.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return err
	}

	columns, err := a.schemaRepo.ListColumnsByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return err
	}

	return a.svc.EnrichEntitiesWithLLM(ctx, projectID, ontologyID, datasourceID, tables, columns)
}

// RelationshipDiscoveryAdapter adapts DeterministicRelationshipService for the dag package.
type RelationshipDiscoveryAdapter struct {
	svc DeterministicRelationshipService
}

// NewRelationshipDiscoveryAdapter creates a new adapter.
func NewRelationshipDiscoveryAdapter(svc DeterministicRelationshipService) dag.DeterministicRelationshipMethods {
	return &RelationshipDiscoveryAdapter{svc: svc}
}

func (a *RelationshipDiscoveryAdapter) DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*dag.RelationshipDiscoveryResult, error) {
	result, err := a.svc.DiscoverRelationships(ctx, projectID, datasourceID)
	if err != nil {
		return nil, err
	}
	return &dag.RelationshipDiscoveryResult{
		FKRelationships:       result.FKRelationships,
		InferredRelationships: result.InferredRelationships,
		TotalRelationships:    result.TotalRelationships,
	}, nil
}

// RelationshipEnrichmentAdapter adapts RelationshipEnrichmentService for the dag package.
type RelationshipEnrichmentAdapter struct {
	svc RelationshipEnrichmentService
}

// NewRelationshipEnrichmentAdapter creates a new adapter.
func NewRelationshipEnrichmentAdapter(svc RelationshipEnrichmentService) dag.RelationshipEnrichmentMethods {
	return &RelationshipEnrichmentAdapter{svc: svc}
}

func (a *RelationshipEnrichmentAdapter) EnrichProject(ctx context.Context, projectID uuid.UUID) (*dag.RelationshipEnrichmentResult, error) {
	result, err := a.svc.EnrichProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &dag.RelationshipEnrichmentResult{
		RelationshipsEnriched: result.RelationshipsEnriched,
		RelationshipsFailed:   result.RelationshipsFailed,
		DurationMs:            result.DurationMs,
	}, nil
}

// OntologyFinalizationAdapter adapts OntologyFinalizationService for the dag package.
type OntologyFinalizationAdapter struct {
	svc OntologyFinalizationService
}

// NewOntologyFinalizationAdapter creates a new adapter.
func NewOntologyFinalizationAdapter(svc OntologyFinalizationService) dag.OntologyFinalizationMethods {
	return &OntologyFinalizationAdapter{svc: svc}
}

func (a *OntologyFinalizationAdapter) Finalize(ctx context.Context, projectID uuid.UUID) error {
	return a.svc.Finalize(ctx, projectID)
}

// ColumnEnrichmentAdapter adapts ColumnEnrichmentService for the dag package.
type ColumnEnrichmentAdapter struct {
	svc ColumnEnrichmentService
}

// NewColumnEnrichmentAdapter creates a new adapter.
func NewColumnEnrichmentAdapter(svc ColumnEnrichmentService) dag.ColumnEnrichmentMethods {
	return &ColumnEnrichmentAdapter{svc: svc}
}

func (a *ColumnEnrichmentAdapter) EnrichProject(ctx context.Context, projectID uuid.UUID, tableNames []string) (*dag.ColumnEnrichmentResult, error) {
	result, err := a.svc.EnrichProject(ctx, projectID, tableNames)
	if err != nil {
		return nil, err
	}
	return &dag.ColumnEnrichmentResult{
		TablesEnriched: result.TablesEnriched,
		TablesFailed:   result.TablesFailed,
		DurationMs:     result.DurationMs,
	}, nil
}
