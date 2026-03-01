package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// These adapters convert between the services package types and the dag package types.
// This allows the dag package to remain independent of the services package,
// avoiding import cycles.

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

// LLMRelationshipDiscoveryAdapter adapts LLMRelationshipDiscoveryService for the dag package.
// This is the new LLM-validated relationship discovery that replaces the threshold-based approach.
type LLMRelationshipDiscoveryAdapter struct {
	svc LLMRelationshipDiscoveryService
}

// NewLLMRelationshipDiscoveryAdapter creates a new adapter.
func NewLLMRelationshipDiscoveryAdapter(svc LLMRelationshipDiscoveryService) dag.LLMRelationshipDiscoveryMethods {
	return &LLMRelationshipDiscoveryAdapter{svc: svc}
}

func (a *LLMRelationshipDiscoveryAdapter) DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) (*dag.LLMRelationshipDiscoveryResult, error) {
	result, err := a.svc.DiscoverRelationships(ctx, projectID, datasourceID, progressCallback)
	if err != nil {
		return nil, err
	}
	return &dag.LLMRelationshipDiscoveryResult{
		CandidatesEvaluated:   result.CandidatesEvaluated,
		RelationshipsCreated:  result.RelationshipsCreated,
		RelationshipsRejected: result.RelationshipsRejected,
		PreservedDBFKs:        result.PreservedDBFKs,
		PreservedColumnFKs:    result.PreservedColumnFKs,
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

func (a *GlossaryDiscoveryAdapter) DiscoverGlossaryTerms(ctx context.Context, projectID uuid.UUID) (int, error) {
	return a.svc.DiscoverGlossaryTerms(ctx, projectID)
}

// GlossaryEnrichmentAdapter adapts GlossaryService for the dag package.
type GlossaryEnrichmentAdapter struct {
	svc GlossaryService
}

// NewGlossaryEnrichmentAdapter creates a new adapter.
func NewGlossaryEnrichmentAdapter(svc GlossaryService) dag.GlossaryEnrichmentMethods {
	return &GlossaryEnrichmentAdapter{svc: svc}
}

func (a *GlossaryEnrichmentAdapter) EnrichGlossaryTerms(ctx context.Context, projectID uuid.UUID) error {
	return a.svc.EnrichGlossaryTerms(ctx, projectID)
}
