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
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)
	GetByTerm(ctx context.Context, projectID uuid.UUID, term string) (*models.BusinessGlossaryTerm, error)
	GetByID(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error)
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
			project_id, term, definition, sql_pattern, base_table,
			columns_used, filters, aggregation, source, created_by,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`

	err := scope.Conn.QueryRow(ctx, query,
		term.ProjectID,
		term.Term,
		term.Definition,
		nullString(term.SQLPattern),
		nullString(term.BaseTable),
		jsonbValue(term.ColumnsUsed),
		jsonbValue(term.Filters),
		nullString(term.Aggregation),
		term.Source,
		term.CreatedBy,
		now,
		now,
	).Scan(&term.ID, &term.CreatedAt, &term.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create glossary term: %w", err)
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
		SET term = $2, definition = $3, sql_pattern = $4, base_table = $5,
		    columns_used = $6, filters = $7, aggregation = $8, source = $9
		WHERE id = $1
		RETURNING updated_at`

	err := scope.Conn.QueryRow(ctx, query,
		term.ID,
		term.Term,
		term.Definition,
		nullString(term.SQLPattern),
		nullString(term.BaseTable),
		jsonbValue(term.ColumnsUsed),
		jsonbValue(term.Filters),
		nullString(term.Aggregation),
		term.Source,
	).Scan(&term.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("failed to update glossary term: %w", err)
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

func (r *glossaryRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, term, definition, sql_pattern, base_table,
		       columns_used, filters, aggregation, source, created_by,
		       created_at, updated_at
		FROM engine_business_glossary
		WHERE project_id = $1
		ORDER BY term`

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
		SELECT id, project_id, term, definition, sql_pattern, base_table,
		       columns_used, filters, aggregation, source, created_by,
		       created_at, updated_at
		FROM engine_business_glossary
		WHERE project_id = $1 AND term = $2`

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

func (r *glossaryRepository) GetByID(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, term, definition, sql_pattern, base_table,
		       columns_used, filters, aggregation, source, created_by,
		       created_at, updated_at
		FROM engine_business_glossary
		WHERE id = $1`

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
// Helper Functions
// ============================================================================

func scanGlossaryTerm(row pgx.Row) (*models.BusinessGlossaryTerm, error) {
	var t models.BusinessGlossaryTerm
	var sqlPattern, baseTable, aggregation *string
	var columnsUsed, filters []byte

	err := row.Scan(
		&t.ID,
		&t.ProjectID,
		&t.Term,
		&t.Definition,
		&sqlPattern,
		&baseTable,
		&columnsUsed,
		&filters,
		&aggregation,
		&t.Source,
		&t.CreatedBy,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan glossary term: %w", err)
	}

	// Handle nullable string fields
	if sqlPattern != nil {
		t.SQLPattern = *sqlPattern
	}
	if baseTable != nil {
		t.BaseTable = *baseTable
	}
	if aggregation != nil {
		t.Aggregation = *aggregation
	}

	// Unmarshal JSONB fields
	if len(columnsUsed) > 0 && string(columnsUsed) != "null" {
		if err := jsonUnmarshal(columnsUsed, &t.ColumnsUsed); err != nil {
			return nil, fmt.Errorf("failed to unmarshal columns_used: %w", err)
		}
	}
	if len(filters) > 0 && string(filters) != "null" {
		if err := jsonUnmarshal(filters, &t.Filters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal filters: %w", err)
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
	case []string:
		if len(val) == 0 {
			return nil
		}
		return val
	case []models.Filter:
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
