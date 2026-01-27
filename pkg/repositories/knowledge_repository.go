package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// KnowledgeRepository provides data access for ontology knowledge facts.
type KnowledgeRepository interface {
	Upsert(ctx context.Context, fact *models.KnowledgeFact) error
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error)
	GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error)
	GetByKey(ctx context.Context, projectID uuid.UUID, factType, key string) (*models.KnowledgeFact, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByProject(ctx context.Context, projectID uuid.UUID) error
	DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error
}

type knowledgeRepository struct{}

// NewKnowledgeRepository creates a new KnowledgeRepository.
func NewKnowledgeRepository() KnowledgeRepository {
	return &knowledgeRepository{}
}

var _ KnowledgeRepository = (*knowledgeRepository)(nil)

func (r *knowledgeRepository) Upsert(ctx context.Context, fact *models.KnowledgeFact) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Extract provenance from context
	prov, ok := models.GetProvenance(ctx)
	if !ok {
		return fmt.Errorf("provenance context required")
	}

	now := time.Now()
	fact.UpdatedAt = now

	// If ID is provided, update by ID (explicit update mode)
	if fact.ID != uuid.Nil {
		// Set provenance fields for update
		lastEditSource := prov.Source.String()
		fact.LastEditSource = &lastEditSource
		// Only set UpdatedBy if there's a valid user ID (not the nil UUID)
		if prov.UserID != uuid.Nil {
			fact.UpdatedBy = &prov.UserID
		} else {
			fact.UpdatedBy = nil
		}

		query := `
			UPDATE engine_project_knowledge
			SET fact_type = $2, key = $3, value = $4, context = $5,
			    last_edit_source = $6, updated_by = $7, updated_at = $8
			WHERE id = $1
			RETURNING created_at`

		err := scope.Conn.QueryRow(ctx, query,
			fact.ID, fact.FactType, fact.Key, fact.Value, fact.Context,
			fact.LastEditSource, fact.UpdatedBy, fact.UpdatedAt,
		).Scan(&fact.CreatedAt)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("fact with id %s not found", fact.ID)
			}
			return fmt.Errorf("failed to update knowledge fact: %w", err)
		}
		return nil
	}

	// No ID provided - upsert by (project_id, ontology_id, fact_type, key)
	fact.ID = uuid.New()
	fact.CreatedAt = now

	// Set provenance fields for create
	fact.Source = prov.Source.String()
	// Only set CreatedBy if there's a valid user ID (not the nil UUID)
	if prov.UserID != uuid.Nil {
		fact.CreatedBy = &prov.UserID
	} else {
		fact.CreatedBy = nil
	}

	query := `
		INSERT INTO engine_project_knowledge (
			id, project_id, ontology_id, fact_type, key, value, context,
			source, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (project_id, ontology_id, fact_type, key)
		DO UPDATE SET
			value = EXCLUDED.value,
			context = EXCLUDED.context,
			last_edit_source = EXCLUDED.source,
			updated_by = EXCLUDED.created_by,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at`

	err := scope.Conn.QueryRow(ctx, query,
		fact.ID, fact.ProjectID, fact.OntologyID, fact.FactType, fact.Key, fact.Value, fact.Context,
		fact.Source, fact.CreatedBy, fact.CreatedAt, fact.UpdatedAt,
	).Scan(&fact.ID, &fact.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to upsert knowledge fact: %w", err)
	}

	return nil
}

func (r *knowledgeRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, fact_type, key, value, context,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_project_knowledge
		WHERE project_id = $1
		ORDER BY fact_type, key`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get knowledge facts: %w", err)
	}
	defer rows.Close()

	facts := make([]*models.KnowledgeFact, 0)
	for rows.Next() {
		f, err := scanKnowledgeFactRows(rows)
		if err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating facts: %w", err)
	}

	return facts, nil
}

func (r *knowledgeRepository) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, fact_type, key, value, context,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_project_knowledge
		WHERE project_id = $1 AND fact_type = $2
		ORDER BY key`

	rows, err := scope.Conn.Query(ctx, query, projectID, factType)
	if err != nil {
		return nil, fmt.Errorf("failed to get knowledge facts: %w", err)
	}
	defer rows.Close()

	facts := make([]*models.KnowledgeFact, 0)
	for rows.Next() {
		f, err := scanKnowledgeFactRows(rows)
		if err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating facts: %w", err)
	}

	return facts, nil
}

func (r *knowledgeRepository) GetByKey(ctx context.Context, projectID uuid.UUID, factType, key string) (*models.KnowledgeFact, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, fact_type, key, value, context,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_project_knowledge
		WHERE project_id = $1 AND fact_type = $2 AND key = $3`

	row := scope.Conn.QueryRow(ctx, query, projectID, factType, key)
	fact, err := scanKnowledgeFactRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, err
	}
	return fact, nil
}

func (r *knowledgeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_project_knowledge WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete knowledge fact: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

func (r *knowledgeRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_project_knowledge WHERE project_id = $1`

	_, err := scope.Conn.Exec(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("delete knowledge by project: %w", err)
	}

	return nil
}

// DeleteBySource deletes all knowledge facts for a project where source matches the given value.
// This supports re-extraction policy: delete inference items while preserving mcp/manual items.
func (r *knowledgeRepository) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_project_knowledge WHERE project_id = $1 AND source = $2`

	_, err := scope.Conn.Exec(ctx, query, projectID, source.String())
	if err != nil {
		return fmt.Errorf("failed to delete knowledge facts by source: %w", err)
	}

	return nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanKnowledgeFactRow(row pgx.Row) (*models.KnowledgeFact, error) {
	var f models.KnowledgeFact
	var context *string

	err := row.Scan(
		&f.ID, &f.ProjectID, &f.OntologyID, &f.FactType, &f.Key, &f.Value, &context,
		&f.Source, &f.LastEditSource, &f.CreatedBy, &f.UpdatedBy, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan knowledge fact: %w", err)
	}

	if context != nil {
		f.Context = *context
	}

	return &f, nil
}

func scanKnowledgeFactRows(rows pgx.Rows) (*models.KnowledgeFact, error) {
	var f models.KnowledgeFact
	var context *string

	err := rows.Scan(
		&f.ID, &f.ProjectID, &f.OntologyID, &f.FactType, &f.Key, &f.Value, &context,
		&f.Source, &f.LastEditSource, &f.CreatedBy, &f.UpdatedBy, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan knowledge fact: %w", err)
	}

	if context != nil {
		f.Context = *context
	}

	return &f, nil
}
