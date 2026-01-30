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
	Create(ctx context.Context, fact *models.KnowledgeFact) error
	Update(ctx context.Context, fact *models.KnowledgeFact) error
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error)
	GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error)
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

func (r *knowledgeRepository) Create(ctx context.Context, fact *models.KnowledgeFact) error {
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
	fact.ID = uuid.New()
	fact.CreatedAt = now
	fact.UpdatedAt = now

	// Set provenance fields for create
	fact.Source = prov.Source.String()
	if prov.UserID != uuid.Nil {
		fact.CreatedBy = &prov.UserID
	} else {
		fact.CreatedBy = nil
	}

	query := `
		INSERT INTO engine_project_knowledge (
			id, project_id, fact_type, value, context,
			source, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := scope.Conn.Exec(ctx, query,
		fact.ID, fact.ProjectID, fact.FactType, fact.Value, fact.Context,
		fact.Source, fact.CreatedBy, fact.CreatedAt, fact.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create knowledge fact: %w", err)
	}

	return nil
}

func (r *knowledgeRepository) Update(ctx context.Context, fact *models.KnowledgeFact) error {
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

	// Set provenance fields for update
	lastEditSource := prov.Source.String()
	fact.LastEditSource = &lastEditSource
	if prov.UserID != uuid.Nil {
		fact.UpdatedBy = &prov.UserID
	} else {
		fact.UpdatedBy = nil
	}

	query := `
		UPDATE engine_project_knowledge
		SET fact_type = $2, value = $3, context = $4,
		    last_edit_source = $5, updated_by = $6, updated_at = $7
		WHERE id = $1
		RETURNING created_at`

	err := scope.Conn.QueryRow(ctx, query,
		fact.ID, fact.FactType, fact.Value, fact.Context,
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

func (r *knowledgeRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, fact_type, value, context,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_project_knowledge
		WHERE project_id = $1
		ORDER BY fact_type, created_at`

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
		SELECT id, project_id, fact_type, value, context,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_project_knowledge
		WHERE project_id = $1 AND fact_type = $2
		ORDER BY created_at`

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
		&f.ID, &f.ProjectID, &f.FactType, &f.Value, &context,
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
		&f.ID, &f.ProjectID, &f.FactType, &f.Value, &context,
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
