// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ensureOntologyExists returns the active ontology, creating an empty one if needed.
// This handles backward compatibility for projects created before empty-ontology-on-creation
// was implemented, and for cases where the project was created without an ontology.
func ensureOntologyExists(
	ctx context.Context,
	ontologyRepo repositories.OntologyRepository,
	projectID uuid.UUID,
) (*models.TieredOntology, error) {
	ontology, err := ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get active ontology: %w", err)
	}
	if ontology != nil {
		return ontology, nil
	}

	// Create empty ontology for backward compatibility
	ontology = &models.TieredOntology{
		ProjectID:       projectID,
		Version:         1,
		IsActive:        true,
		EntitySummaries: make(map[string]*models.EntitySummary),
		ColumnDetails:   make(map[string][]models.ColumnDetail),
		Metadata:        make(map[string]any),
	}
	if err := ontologyRepo.Create(ctx, ontology); err != nil {
		return nil, fmt.Errorf("create empty ontology: %w", err)
	}

	return ontology, nil
}
