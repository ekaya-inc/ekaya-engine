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

	tables, err := a.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID, false)
	if err != nil {
		return err
	}

	columns, err := a.schemaRepo.ListColumnsByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return err
	}

	return a.svc.EnrichEntitiesWithLLM(ctx, projectID, ontologyID, datasourceID, tables, columns)
}

// FKDiscoveryAdapter adapts DeterministicRelationshipService for FK discovery.
type FKDiscoveryAdapter struct {
	svc DeterministicRelationshipService
}

// NewFKDiscoveryAdapter creates a new adapter.
func NewFKDiscoveryAdapter(svc DeterministicRelationshipService) dag.FKDiscoveryMethods {
	return &FKDiscoveryAdapter{svc: svc}
}

func (a *FKDiscoveryAdapter) DiscoverFKRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) (*dag.FKDiscoveryResult, error) {
	// Convert dag.ProgressCallback to services.RelationshipProgressCallback
	var svcCallback RelationshipProgressCallback
	if progressCallback != nil {
		svcCallback = RelationshipProgressCallback(progressCallback)
	}
	result, err := a.svc.DiscoverFKRelationships(ctx, projectID, datasourceID, svcCallback)
	if err != nil {
		return nil, err
	}
	return &dag.FKDiscoveryResult{
		FKRelationships: result.FKRelationships,
	}, nil
}

// PKMatchDiscoveryAdapter adapts DeterministicRelationshipService for pk_match discovery.
type PKMatchDiscoveryAdapter struct {
	svc DeterministicRelationshipService
}

// NewPKMatchDiscoveryAdapter creates a new adapter.
func NewPKMatchDiscoveryAdapter(svc DeterministicRelationshipService) dag.PKMatchDiscoveryMethods {
	return &PKMatchDiscoveryAdapter{svc: svc}
}

func (a *PKMatchDiscoveryAdapter) DiscoverPKMatchRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) (*dag.PKMatchDiscoveryResult, error) {
	// Convert dag.ProgressCallback to services.RelationshipProgressCallback
	var svcCallback RelationshipProgressCallback
	if progressCallback != nil {
		svcCallback = RelationshipProgressCallback(progressCallback)
	}
	result, err := a.svc.DiscoverPKMatchRelationships(ctx, projectID, datasourceID, svcCallback)
	if err != nil {
		return nil, err
	}
	return &dag.PKMatchDiscoveryResult{
		InferredRelationships: result.InferredRelationships,
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

func (a *RelationshipEnrichmentAdapter) EnrichProject(ctx context.Context, projectID uuid.UUID, progressCallback dag.ProgressCallback) (*dag.RelationshipEnrichmentResult, error) {
	result, err := a.svc.EnrichProject(ctx, projectID, progressCallback)
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

func (a *ColumnEnrichmentAdapter) EnrichProject(ctx context.Context, projectID uuid.UUID, tableNames []string, progressCallback dag.ProgressCallback) (*dag.ColumnEnrichmentResult, error) {
	result, err := a.svc.EnrichProject(ctx, projectID, tableNames, progressCallback)
	if err != nil {
		return nil, err
	}
	return &dag.ColumnEnrichmentResult{
		TablesEnriched: result.TablesEnriched,
		TablesFailed:   result.TablesFailed,
		DurationMs:     result.DurationMs,
	}, nil
}

// GlossaryDiscoveryAdapter adapts GlossaryService for the dag package.
type GlossaryDiscoveryAdapter struct {
	svc GlossaryService
}

// NewGlossaryDiscoveryAdapter creates a new adapter.
func NewGlossaryDiscoveryAdapter(svc GlossaryService) dag.GlossaryDiscoveryMethods {
	return &GlossaryDiscoveryAdapter{svc: svc}
}

func (a *GlossaryDiscoveryAdapter) DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error) {
	return a.svc.DiscoverGlossaryTerms(ctx, projectID, ontologyID)
}

// GlossaryEnrichmentAdapter adapts GlossaryService for the dag package.
type GlossaryEnrichmentAdapter struct {
	svc GlossaryService
}

// NewGlossaryEnrichmentAdapter creates a new adapter.
func NewGlossaryEnrichmentAdapter(svc GlossaryService) dag.GlossaryEnrichmentMethods {
	return &GlossaryEnrichmentAdapter{svc: svc}
}

func (a *GlossaryEnrichmentAdapter) EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error {
	return a.svc.EnrichGlossaryTerms(ctx, projectID, ontologyID)
}

// KnowledgeSeedingAdapter adapts KnowledgeService for the dag package.
type KnowledgeSeedingAdapter struct {
	svc KnowledgeService
}

// NewKnowledgeSeedingAdapter creates a new adapter.
func NewKnowledgeSeedingAdapter(svc KnowledgeService) dag.KnowledgeSeedingMethods {
	return &KnowledgeSeedingAdapter{svc: svc}
}

func (a *KnowledgeSeedingAdapter) SeedKnowledgeFromFile(ctx context.Context, projectID uuid.UUID) (int, error) {
	return a.svc.SeedKnowledgeFromFile(ctx, projectID)
}
