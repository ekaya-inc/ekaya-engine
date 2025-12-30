package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// SchemaEntityRepository provides data access for schema entities and their occurrences.
type SchemaEntityRepository interface {
	// Entity operations
	Create(ctx context.Context, entity *models.SchemaEntity) error
	GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.SchemaEntity, error)
	GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.SchemaEntity, error)
	DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error

	// Occurrence operations
	CreateOccurrence(ctx context.Context, occ *models.SchemaEntityOccurrence) error
	GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.SchemaEntityOccurrence, error)
	GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.SchemaEntityOccurrence, error)
}

type schemaEntityRepository struct{}

// NewSchemaEntityRepository creates a new SchemaEntityRepository.
func NewSchemaEntityRepository() SchemaEntityRepository {
	return &schemaEntityRepository{}
}

var _ SchemaEntityRepository = (*schemaEntityRepository)(nil)

// ============================================================================
// Entity Operations
// ============================================================================

func (r *schemaEntityRepository) Create(ctx context.Context, entity *models.SchemaEntity) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	entity.CreatedAt = now
	entity.UpdatedAt = now

	if entity.ID == uuid.Nil {
		entity.ID = uuid.New()
	}

	query := `
		INSERT INTO engine_schema_entities (
			id, project_id, ontology_id, name, description,
			primary_schema, primary_table, primary_column,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := scope.Conn.Exec(ctx, query,
		entity.ID, entity.ProjectID, entity.OntologyID, entity.Name, entity.Description,
		entity.PrimarySchema, entity.PrimaryTable, entity.PrimaryColumn,
		entity.CreatedAt, entity.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create schema entity: %w", err)
	}

	return nil
}

func (r *schemaEntityRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.SchemaEntity, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, name, description,
		       primary_schema, primary_table, primary_column,
		       created_at, updated_at
		FROM engine_schema_entities
		WHERE ontology_id = $1
		ORDER BY name`

	rows, err := scope.Conn.Query(ctx, query, ontologyID)
	if err != nil {
		return nil, fmt.Errorf("failed to query schema entities: %w", err)
	}
	defer rows.Close()

	var entities []*models.SchemaEntity
	for rows.Next() {
		entity, err := scanSchemaEntity(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating schema entities: %w", err)
	}

	return entities, nil
}

func (r *schemaEntityRepository) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.SchemaEntity, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, name, description,
		       primary_schema, primary_table, primary_column,
		       created_at, updated_at
		FROM engine_schema_entities
		WHERE ontology_id = $1 AND name = $2`

	row := scope.Conn.QueryRow(ctx, query, ontologyID, name)
	entity, err := scanSchemaEntity(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Entity not found
		}
		return nil, err
	}

	return entity, nil
}

func (r *schemaEntityRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_schema_entities WHERE ontology_id = $1`

	_, err := scope.Conn.Exec(ctx, query, ontologyID)
	if err != nil {
		return fmt.Errorf("failed to delete schema entities: %w", err)
	}

	return nil
}

// ============================================================================
// Occurrence Operations
// ============================================================================

func (r *schemaEntityRepository) CreateOccurrence(ctx context.Context, occ *models.SchemaEntityOccurrence) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	occ.CreatedAt = time.Now()

	if occ.ID == uuid.Nil {
		occ.ID = uuid.New()
	}

	query := `
		INSERT INTO engine_schema_entity_occurrences (
			id, entity_id, schema_name, table_name, column_name, role, confidence, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := scope.Conn.Exec(ctx, query,
		occ.ID, occ.EntityID, occ.SchemaName, occ.TableName, occ.ColumnName, occ.Role, occ.Confidence, occ.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create schema entity occurrence: %w", err)
	}

	return nil
}

func (r *schemaEntityRepository) GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.SchemaEntityOccurrence, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, entity_id, schema_name, table_name, column_name, role, confidence, created_at
		FROM engine_schema_entity_occurrences
		WHERE entity_id = $1
		ORDER BY schema_name, table_name, column_name`

	rows, err := scope.Conn.Query(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity occurrences: %w", err)
	}
	defer rows.Close()

	var occurrences []*models.SchemaEntityOccurrence
	for rows.Next() {
		occ, err := scanSchemaEntityOccurrence(rows)
		if err != nil {
			return nil, err
		}
		occurrences = append(occurrences, occ)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity occurrences: %w", err)
	}

	return occurrences, nil
}

func (r *schemaEntityRepository) GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.SchemaEntityOccurrence, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT o.id, o.entity_id, o.schema_name, o.table_name, o.column_name, o.role, o.confidence, o.created_at
		FROM engine_schema_entity_occurrences o
		JOIN engine_schema_entities e ON o.entity_id = e.id
		WHERE e.ontology_id = $1 AND o.schema_name = $2 AND o.table_name = $3
		ORDER BY o.column_name`

	rows, err := scope.Conn.Query(ctx, query, ontologyID, schema, table)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity occurrences by table: %w", err)
	}
	defer rows.Close()

	var occurrences []*models.SchemaEntityOccurrence
	for rows.Next() {
		occ, err := scanSchemaEntityOccurrence(rows)
		if err != nil {
			return nil, err
		}
		occurrences = append(occurrences, occ)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity occurrences: %w", err)
	}

	return occurrences, nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanSchemaEntity(row pgx.Row) (*models.SchemaEntity, error) {
	var e models.SchemaEntity

	err := row.Scan(
		&e.ID, &e.ProjectID, &e.OntologyID, &e.Name, &e.Description,
		&e.PrimarySchema, &e.PrimaryTable, &e.PrimaryColumn,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan schema entity: %w", err)
	}

	return &e, nil
}

func scanSchemaEntityOccurrence(row pgx.Row) (*models.SchemaEntityOccurrence, error) {
	var o models.SchemaEntityOccurrence

	err := row.Scan(
		&o.ID, &o.EntityID, &o.SchemaName, &o.TableName, &o.ColumnName, &o.Role, &o.Confidence, &o.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan schema entity occurrence: %w", err)
	}

	return &o, nil
}
