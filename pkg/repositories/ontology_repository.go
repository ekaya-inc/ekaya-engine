package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// OntologyRepository provides data access for tiered ontologies.
type OntologyRepository interface {
	Create(ctx context.Context, ontology *models.TieredOntology) error
	GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error)
	UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error
	UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error
	UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error
	UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error
	GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error)
	DeleteByProject(ctx context.Context, projectID uuid.UUID) error
}

type ontologyRepository struct{}

// NewOntologyRepository creates a new OntologyRepository.
func NewOntologyRepository() OntologyRepository {
	return &ontologyRepository{}
}

var _ OntologyRepository = (*ontologyRepository)(nil)

func (r *ontologyRepository) Create(ctx context.Context, ontology *models.TieredOntology) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	ontology.CreatedAt = time.Now()
	if ontology.ID == uuid.Nil {
		ontology.ID = uuid.New()
	}

	domainJSON, err := json.Marshal(ontology.DomainSummary)
	if err != nil {
		return fmt.Errorf("failed to marshal domain_summary: %w", err)
	}
	if ontology.DomainSummary == nil {
		domainJSON = nil
	}

	entitiesJSON, err := json.Marshal(ontology.EntitySummaries)
	if err != nil {
		return fmt.Errorf("failed to marshal entity_summaries: %w", err)
	}
	if ontology.EntitySummaries == nil {
		entitiesJSON = nil
	}

	columnsJSON, err := json.Marshal(ontology.ColumnDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal column_details: %w", err)
	}
	if ontology.ColumnDetails == nil {
		columnsJSON = nil
	}

	metadataJSON, err := json.Marshal(ontology.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if ontology.Metadata == nil {
		metadataJSON = []byte("{}")
	}

	ontology.UpdatedAt = ontology.CreatedAt

	// Use transaction to atomically deactivate prior ontologies and create new one
	tx, err := scope.Conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on defer is best-effort

	// Deactivate all existing active ontologies for this project (if creating an active one)
	if ontology.IsActive {
		_, err = tx.Exec(ctx,
			"UPDATE engine_ontologies SET is_active = false, updated_at = $2 WHERE project_id = $1 AND is_active = true",
			ontology.ProjectID, ontology.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to deactivate prior ontologies: %w", err)
		}
	}

	query := `
		INSERT INTO engine_ontologies (
			id, project_id, version, is_active, domain_summary,
			entity_summaries, column_details, metadata, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err = tx.Exec(ctx, query,
		ontology.ID, ontology.ProjectID, ontology.Version, ontology.IsActive,
		domainJSON, entitiesJSON, columnsJSON, metadataJSON, ontology.CreatedAt, ontology.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create ontology: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *ontologyRepository) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, version, is_active,
		       domain_summary, entity_summaries, column_details, metadata, created_at, updated_at
		FROM engine_ontologies
		WHERE project_id = $1 AND is_active = true
		ORDER BY version DESC
		LIMIT 1`

	row := scope.Conn.QueryRow(ctx, query, projectID)
	ontology, err := scanOntologyRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No active ontology
		}
		return nil, err
	}
	return ontology, nil
}

func (r *ontologyRepository) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	domainJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal domain_summary: %w", err)
	}

	query := `
		UPDATE engine_ontologies
		SET domain_summary = $2
		WHERE project_id = $1 AND is_active = true`

	result, err := scope.Conn.Exec(ctx, query, projectID, domainJSON)
	if err != nil {
		return fmt.Errorf("failed to update domain summary: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no active ontology found")
	}

	return nil
}

func (r *ontologyRepository) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal entity summary: %w", err)
	}

	// Use JSONB set to update a single entity
	query := `
		UPDATE engine_ontologies
		SET entity_summaries = COALESCE(entity_summaries, '{}'::jsonb) || jsonb_build_object($2::text, $3::jsonb)
		WHERE project_id = $1 AND is_active = true`

	result, err := scope.Conn.Exec(ctx, query, projectID, tableName, summaryJSON)
	if err != nil {
		return fmt.Errorf("failed to update entity summary: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no active ontology found")
	}

	return nil
}

func (r *ontologyRepository) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	summariesJSON, err := json.Marshal(summaries)
	if err != nil {
		return fmt.Errorf("failed to marshal entity summaries: %w", err)
	}

	// Merge new summaries with existing ones
	query := `
		UPDATE engine_ontologies
		SET entity_summaries = COALESCE(entity_summaries, '{}'::jsonb) || $2::jsonb
		WHERE project_id = $1 AND is_active = true`

	result, err := scope.Conn.Exec(ctx, query, projectID, summariesJSON)
	if err != nil {
		return fmt.Errorf("failed to update entity summaries: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no active ontology found")
	}

	return nil
}

func (r *ontologyRepository) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	columnsJSON, err := json.Marshal(columns)
	if err != nil {
		return fmt.Errorf("failed to marshal column details: %w", err)
	}

	// Use JSONB set to update column details for a single table
	query := `
		UPDATE engine_ontologies
		SET column_details = COALESCE(column_details, '{}'::jsonb) || jsonb_build_object($2::text, $3::jsonb)
		WHERE project_id = $1 AND is_active = true`

	result, err := scope.Conn.Exec(ctx, query, projectID, tableName, columnsJSON)
	if err != nil {
		return fmt.Errorf("failed to update column details: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no active ontology found")
	}

	return nil
}

func (r *ontologyRepository) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM engine_ontologies
		WHERE project_id = $1`

	var nextVersion int
	err := scope.Conn.QueryRow(ctx, query, projectID).Scan(&nextVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to get next version: %w", err)
	}

	return nextVersion, nil
}

func (r *ontologyRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Use transaction to ensure atomicity of all cleanup operations
	tx, err := scope.Conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on defer is best-effort

	// 1. Clean up project_knowledge (fallback - also has CASCADE via ontology_id FK)
	_, err = tx.Exec(ctx, `DELETE FROM engine_project_knowledge WHERE project_id = $1`, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete project knowledge: %w", err)
	}

	// 2. Clean up business_glossary (fallback - also has CASCADE via ontology_id FK)
	_, err = tx.Exec(ctx, `DELETE FROM engine_business_glossary WHERE project_id = $1`, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete business glossary: %w", err)
	}

	// 3. Delete ontologies (cascades to other ontology tables like entities, relationships, etc.)
	_, err = tx.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete ontologies: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanOntologyRow(row pgx.Row) (*models.TieredOntology, error) {
	var o models.TieredOntology
	var domainJSON, entitiesJSON, columnsJSON, metadataJSON []byte

	err := row.Scan(
		&o.ID, &o.ProjectID, &o.Version, &o.IsActive,
		&domainJSON, &entitiesJSON, &columnsJSON, &metadataJSON, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan ontology: %w", err)
	}

	if len(domainJSON) > 0 {
		o.DomainSummary = &models.DomainSummary{}
		if err := json.Unmarshal(domainJSON, o.DomainSummary); err != nil {
			return nil, fmt.Errorf("failed to unmarshal domain_summary: %w", err)
		}
	}

	if len(entitiesJSON) > 0 {
		o.EntitySummaries = make(map[string]*models.EntitySummary)
		if err := json.Unmarshal(entitiesJSON, &o.EntitySummaries); err != nil {
			return nil, fmt.Errorf("failed to unmarshal entity_summaries: %w", err)
		}
	}

	if len(columnsJSON) > 0 {
		o.ColumnDetails = make(map[string][]models.ColumnDetail)
		if err := json.Unmarshal(columnsJSON, &o.ColumnDetails); err != nil {
			return nil, fmt.Errorf("failed to unmarshal column_details: %w", err)
		}
	}

	if len(metadataJSON) > 0 {
		o.Metadata = make(map[string]any)
		if err := json.Unmarshal(metadataJSON, &o.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &o, nil
}
