package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// GlossaryRepository provides data access for business glossary terms.
type GlossaryRepository interface {
	Create(ctx context.Context, term *models.BusinessGlossaryTerm) error
	Update(ctx context.Context, term *models.BusinessGlossaryTerm) error
	Delete(ctx context.Context, termID uuid.UUID) error
	DeleteBySource(ctx context.Context, projectID uuid.UUID, source string) error
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)
	GetByTerm(ctx context.Context, projectID uuid.UUID, term string) (*models.BusinessGlossaryTerm, error)
	GetByAlias(ctx context.Context, projectID uuid.UUID, alias string) (*models.BusinessGlossaryTerm, error)
	GetByID(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error)
	CreateAlias(ctx context.Context, glossaryID uuid.UUID, alias string) error
	DeleteAlias(ctx context.Context, glossaryID uuid.UUID, alias string) error
}

type glossaryRepository struct{}

// NewGlossaryRepository creates a new GlossaryRepository.
func NewGlossaryRepository() GlossaryRepository {
	return &glossaryRepository{}
}

var _ GlossaryRepository = (*glossaryRepository)(nil)

// ============================================================================
// CRUD Operations
// ============================================================================

func (r *glossaryRepository) Create(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()

	query := `
		INSERT INTO engine_business_glossary (
			project_id, ontology_id, term, definition, defining_sql, base_table,
			output_columns, source, enrichment_status, enrichment_error,
			created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at, updated_at`

	err := scope.Conn.QueryRow(ctx, query,
		term.ProjectID,
		term.OntologyID,
		term.Term,
		term.Definition,
		term.DefiningSQL,
		nullString(term.BaseTable),
		jsonbValue(term.OutputColumns),
		term.Source,
		nullString(term.EnrichmentStatus),
		nullString(term.EnrichmentError),
		term.CreatedBy,
		term.UpdatedBy,
		now,
		now,
	).Scan(&term.ID, &term.CreatedAt, &term.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create glossary term: %w", err)
	}

	// Create aliases if provided
	if len(term.Aliases) > 0 {
		for _, alias := range term.Aliases {
			if err := r.CreateAlias(ctx, term.ID, alias); err != nil {
				return fmt.Errorf("failed to create alias %q: %w", alias, err)
			}
		}
	}

	return nil
}

func (r *glossaryRepository) Update(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_business_glossary
		SET term = $2, definition = $3, defining_sql = $4, base_table = $5,
		    output_columns = $6, source = $7, enrichment_status = $8,
		    enrichment_error = $9, updated_by = $10
		WHERE id = $1
		RETURNING updated_at`

	err := scope.Conn.QueryRow(ctx, query,
		term.ID,
		term.Term,
		term.Definition,
		term.DefiningSQL,
		nullString(term.BaseTable),
		jsonbValue(term.OutputColumns),
		term.Source,
		nullString(term.EnrichmentStatus),
		nullString(term.EnrichmentError),
		term.UpdatedBy,
	).Scan(&term.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("failed to update glossary term: %w", err)
	}

	// Update aliases: delete all existing and create new ones
	// First, delete all existing aliases
	deleteQuery := `DELETE FROM engine_glossary_aliases WHERE glossary_id = $1`
	if _, err := scope.Conn.Exec(ctx, deleteQuery, term.ID); err != nil {
		return fmt.Errorf("failed to delete existing aliases: %w", err)
	}

	// Create new aliases if provided
	if len(term.Aliases) > 0 {
		for _, alias := range term.Aliases {
			if err := r.CreateAlias(ctx, term.ID, alias); err != nil {
				return fmt.Errorf("failed to create alias %q: %w", alias, err)
			}
		}
	}

	return nil
}

func (r *glossaryRepository) Delete(ctx context.Context, termID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_business_glossary WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, termID)
	if err != nil {
		return fmt.Errorf("failed to delete glossary term: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

// DeleteBySource deletes all glossary terms for a project with the specified source.
// This is used to clear inferred terms when ontology is reset while preserving manual terms.
func (r *glossaryRepository) DeleteBySource(ctx context.Context, projectID uuid.UUID, source string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// First delete aliases for terms that will be deleted
	aliasQuery := `
		DELETE FROM engine_glossary_aliases
		WHERE glossary_id IN (
			SELECT id FROM engine_business_glossary
			WHERE project_id = $1 AND source = $2
		)`
	if _, err := scope.Conn.Exec(ctx, aliasQuery, projectID, source); err != nil {
		return fmt.Errorf("failed to delete glossary aliases: %w", err)
	}

	// Then delete the terms
	query := `DELETE FROM engine_business_glossary WHERE project_id = $1 AND source = $2`
	if _, err := scope.Conn.Exec(ctx, query, projectID, source); err != nil {
		return fmt.Errorf("failed to delete glossary terms by source: %w", err)
	}

	return nil
}

func (r *glossaryRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT g.id, g.project_id, g.ontology_id, g.term, g.definition, g.defining_sql, g.base_table,
		       g.output_columns, g.source, g.enrichment_status, g.enrichment_error,
		       g.created_by, g.updated_by, g.created_at, g.updated_at,
		       COALESCE(
		           jsonb_agg(a.alias ORDER BY a.alias) FILTER (WHERE a.alias IS NOT NULL),
		           '[]'::jsonb
		       ) as aliases
		FROM engine_business_glossary g
		LEFT JOIN engine_glossary_aliases a ON g.id = a.glossary_id
		WHERE g.project_id = $1
		GROUP BY g.id, g.project_id, g.ontology_id, g.term, g.definition, g.defining_sql, g.base_table,
		         g.output_columns, g.source, g.enrichment_status, g.enrichment_error,
		         g.created_by, g.updated_by, g.created_at, g.updated_at
		ORDER BY g.term`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query glossary terms: %w", err)
	}
	defer rows.Close()

	var terms []*models.BusinessGlossaryTerm
	for rows.Next() {
		term, err := scanGlossaryTerm(rows)
		if err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating glossary terms: %w", err)
	}

	return terms, nil
}

func (r *glossaryRepository) GetByTerm(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT g.id, g.project_id, g.ontology_id, g.term, g.definition, g.defining_sql, g.base_table,
		       g.output_columns, g.source, g.enrichment_status, g.enrichment_error,
		       g.created_by, g.updated_by, g.created_at, g.updated_at,
		       COALESCE(
		           jsonb_agg(a.alias ORDER BY a.alias) FILTER (WHERE a.alias IS NOT NULL),
		           '[]'::jsonb
		       ) as aliases
		FROM engine_business_glossary g
		LEFT JOIN engine_glossary_aliases a ON g.id = a.glossary_id
		WHERE g.project_id = $1 AND g.term = $2
		GROUP BY g.id, g.project_id, g.ontology_id, g.term, g.definition, g.defining_sql, g.base_table,
		         g.output_columns, g.source, g.enrichment_status, g.enrichment_error,
		         g.created_by, g.updated_by, g.created_at, g.updated_at`

	row := scope.Conn.QueryRow(ctx, query, projectID, termName)
	term, err := scanGlossaryTerm(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Term not found
		}
		return nil, err
	}

	return term, nil
}

func (r *glossaryRepository) GetByAlias(ctx context.Context, projectID uuid.UUID, alias string) (*models.BusinessGlossaryTerm, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT g.id, g.project_id, g.ontology_id, g.term, g.definition, g.defining_sql, g.base_table,
		       g.output_columns, g.source, g.enrichment_status, g.enrichment_error,
		       g.created_by, g.updated_by, g.created_at, g.updated_at,
		       COALESCE(
		           jsonb_agg(a2.alias ORDER BY a2.alias) FILTER (WHERE a2.alias IS NOT NULL),
		           '[]'::jsonb
		       ) as aliases
		FROM engine_business_glossary g
		INNER JOIN engine_glossary_aliases a ON g.id = a.glossary_id
		LEFT JOIN engine_glossary_aliases a2 ON g.id = a2.glossary_id
		WHERE g.project_id = $1 AND a.alias = $2
		GROUP BY g.id, g.project_id, g.ontology_id, g.term, g.definition, g.defining_sql, g.base_table,
		         g.output_columns, g.source, g.enrichment_status, g.enrichment_error,
		         g.created_by, g.updated_by, g.created_at, g.updated_at`

	row := scope.Conn.QueryRow(ctx, query, projectID, alias)
	term, err := scanGlossaryTerm(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Term not found
		}
		return nil, err
	}

	return term, nil
}

func (r *glossaryRepository) GetByID(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT g.id, g.project_id, g.ontology_id, g.term, g.definition, g.defining_sql, g.base_table,
		       g.output_columns, g.source, g.enrichment_status, g.enrichment_error,
		       g.created_by, g.updated_by, g.created_at, g.updated_at,
		       COALESCE(
		           jsonb_agg(a.alias ORDER BY a.alias) FILTER (WHERE a.alias IS NOT NULL),
		           '[]'::jsonb
		       ) as aliases
		FROM engine_business_glossary g
		LEFT JOIN engine_glossary_aliases a ON g.id = a.glossary_id
		WHERE g.id = $1
		GROUP BY g.id, g.project_id, g.ontology_id, g.term, g.definition, g.defining_sql, g.base_table,
		         g.output_columns, g.source, g.enrichment_status, g.enrichment_error,
		         g.created_by, g.updated_by, g.created_at, g.updated_at`

	row := scope.Conn.QueryRow(ctx, query, termID)
	term, err := scanGlossaryTerm(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Term not found
		}
		return nil, err
	}

	return term, nil
}

// ============================================================================
// Alias Operations
// ============================================================================

func (r *glossaryRepository) CreateAlias(ctx context.Context, glossaryID uuid.UUID, alias string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		INSERT INTO engine_glossary_aliases (glossary_id, alias)
		VALUES ($1, $2)`

	_, err := scope.Conn.Exec(ctx, query, glossaryID, alias)
	if err != nil {
		return fmt.Errorf("failed to create alias: %w", err)
	}

	return nil
}

func (r *glossaryRepository) DeleteAlias(ctx context.Context, glossaryID uuid.UUID, alias string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_glossary_aliases WHERE glossary_id = $1 AND alias = $2`

	result, err := scope.Conn.Exec(ctx, query, glossaryID, alias)
	if err != nil {
		return fmt.Errorf("failed to delete alias: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

// ============================================================================
// Helper Functions
// ============================================================================

func scanGlossaryTerm(row pgx.Row) (*models.BusinessGlossaryTerm, error) {
	var t models.BusinessGlossaryTerm
	var baseTable, enrichmentStatus, enrichmentError *string
	var outputColumns, aliases []byte

	err := row.Scan(
		&t.ID,
		&t.ProjectID,
		&t.OntologyID,
		&t.Term,
		&t.Definition,
		&t.DefiningSQL,
		&baseTable,
		&outputColumns,
		&t.Source,
		&enrichmentStatus,
		&enrichmentError,
		&t.CreatedBy,
		&t.UpdatedBy,
		&t.CreatedAt,
		&t.UpdatedAt,
		&aliases,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan glossary term: %w", err)
	}

	// Handle nullable string fields
	if baseTable != nil {
		t.BaseTable = *baseTable
	}
	if enrichmentStatus != nil {
		t.EnrichmentStatus = *enrichmentStatus
	}
	if enrichmentError != nil {
		t.EnrichmentError = *enrichmentError
	}

	// Unmarshal JSONB fields
	if len(outputColumns) > 0 && string(outputColumns) != "null" {
		if err := jsonUnmarshal(outputColumns, &t.OutputColumns); err != nil {
			return nil, fmt.Errorf("failed to unmarshal output_columns: %w", err)
		}
	}
	if len(aliases) > 0 && string(aliases) != "null" && string(aliases) != "[]" {
		if err := jsonUnmarshal(aliases, &t.Aliases); err != nil {
			return nil, fmt.Errorf("failed to unmarshal aliases: %w", err)
		}
	}

	return &t, nil
}

// nullString returns nil if the string is empty, otherwise returns the string pointer.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// jsonbValue converts a value to JSONB format for database insertion.
// Returns nil for nil/empty slices to store NULL in the database.
func jsonbValue(v any) any {
	switch val := v.(type) {
	case []models.OutputColumn:
		if len(val) == 0 {
			return nil
		}
		return val
	default:
		return v
	}
}

// jsonUnmarshal unmarshals JSONB data from the database.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
