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
	GetByID(ctx context.Context, entityID uuid.UUID) (*models.SchemaEntity, error)
	GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.SchemaEntity, error)
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.SchemaEntity, error)
	GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.SchemaEntity, error)
	DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error
	Update(ctx context.Context, entity *models.SchemaEntity) error

	// Soft delete operations
	SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error
	Restore(ctx context.Context, entityID uuid.UUID) error

	// Occurrence operations
	CreateOccurrence(ctx context.Context, occ *models.SchemaEntityOccurrence) error
	GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.SchemaEntityOccurrence, error)
	GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.SchemaEntityOccurrence, error)
	GetAllOccurrencesByProject(ctx context.Context, projectID uuid.UUID) ([]*models.SchemaEntityOccurrence, error)

	// Alias operations
	CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error
	GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error)
	DeleteAlias(ctx context.Context, aliasID uuid.UUID) error
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
		INSERT INTO engine_ontology_entities (
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
		       is_deleted, deletion_reason,
		       created_at, updated_at
		FROM engine_ontology_entities
		WHERE ontology_id = $1 AND NOT is_deleted
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

func (r *schemaEntityRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.SchemaEntity, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT e.id, e.project_id, e.ontology_id, e.name, e.description,
		       e.primary_schema, e.primary_table, e.primary_column,
		       e.is_deleted, e.deletion_reason,
		       e.created_at, e.updated_at
		FROM engine_ontology_entities e
		JOIN engine_ontologies o ON e.ontology_id = o.id
		WHERE e.project_id = $1 AND o.is_active = true AND NOT e.is_deleted
		ORDER BY e.name`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query schema entities by project: %w", err)
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
		       is_deleted, deletion_reason,
		       created_at, updated_at
		FROM engine_ontology_entities
		WHERE ontology_id = $1 AND name = $2 AND NOT is_deleted`

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

	query := `DELETE FROM engine_ontology_entities WHERE ontology_id = $1`

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
		INSERT INTO engine_ontology_entity_occurrences (
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
		FROM engine_ontology_entity_occurrences
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
		FROM engine_ontology_entity_occurrences o
		JOIN engine_ontology_entities e ON o.entity_id = e.id
		WHERE e.ontology_id = $1 AND o.schema_name = $2 AND o.table_name = $3 AND NOT e.is_deleted
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

func (r *schemaEntityRepository) GetAllOccurrencesByProject(ctx context.Context, projectID uuid.UUID) ([]*models.SchemaEntityOccurrence, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT o.id, o.entity_id, o.schema_name, o.table_name, o.column_name, o.role, o.confidence, o.created_at
		FROM engine_ontology_entity_occurrences o
		JOIN engine_ontology_entities e ON o.entity_id = e.id
		JOIN engine_ontologies ont ON e.ontology_id = ont.id
		WHERE e.project_id = $1 AND ont.is_active = true AND NOT e.is_deleted
		ORDER BY o.schema_name, o.table_name, o.column_name`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity occurrences by project: %w", err)
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
// Entity CRUD Operations
// ============================================================================

func (r *schemaEntityRepository) GetByID(ctx context.Context, entityID uuid.UUID) (*models.SchemaEntity, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, name, description,
		       primary_schema, primary_table, primary_column,
		       is_deleted, deletion_reason,
		       created_at, updated_at
		FROM engine_ontology_entities
		WHERE id = $1`

	row := scope.Conn.QueryRow(ctx, query, entityID)
	entity, err := scanSchemaEntity(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return entity, nil
}

func (r *schemaEntityRepository) Update(ctx context.Context, entity *models.SchemaEntity) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	entity.UpdatedAt = time.Now()

	query := `
		UPDATE engine_ontology_entities
		SET name = $2, description = $3,
		    primary_schema = $4, primary_table = $5, primary_column = $6,
		    updated_at = $7
		WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query,
		entity.ID, entity.Name, entity.Description,
		entity.PrimarySchema, entity.PrimaryTable, entity.PrimaryColumn,
		entity.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update entity: %w", err)
	}

	return nil
}

// ============================================================================
// Soft Delete Operations
// ============================================================================

func (r *schemaEntityRepository) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_entities
		SET is_deleted = true, deletion_reason = $2, updated_at = $3
		WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query, entityID, reason, time.Now())
	if err != nil {
		return fmt.Errorf("failed to soft delete entity: %w", err)
	}

	return nil
}

func (r *schemaEntityRepository) Restore(ctx context.Context, entityID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_entities
		SET is_deleted = false, deletion_reason = NULL, updated_at = $2
		WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query, entityID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to restore entity: %w", err)
	}

	return nil
}

// ============================================================================
// Alias Operations
// ============================================================================

func (r *schemaEntityRepository) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	alias.CreatedAt = time.Now()

	if alias.ID == uuid.Nil {
		alias.ID = uuid.New()
	}

	query := `
		INSERT INTO engine_ontology_entity_aliases (id, entity_id, alias, source, created_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := scope.Conn.Exec(ctx, query,
		alias.ID, alias.EntityID, alias.Alias, alias.Source, alias.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create entity alias: %w", err)
	}

	return nil
}

func (r *schemaEntityRepository) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, entity_id, alias, source, created_at
		FROM engine_ontology_entity_aliases
		WHERE entity_id = $1
		ORDER BY alias`

	rows, err := scope.Conn.Query(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity aliases: %w", err)
	}
	defer rows.Close()

	var aliases []*models.OntologyEntityAlias
	for rows.Next() {
		alias, err := scanOntologyEntityAlias(rows)
		if err != nil {
			return nil, err
		}
		aliases = append(aliases, alias)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity aliases: %w", err)
	}

	return aliases, nil
}

func (r *schemaEntityRepository) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_entity_aliases WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query, aliasID)
	if err != nil {
		return fmt.Errorf("failed to delete entity alias: %w", err)
	}

	return nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanSchemaEntity(row pgx.Row) (*models.SchemaEntity, error) {
	var e models.SchemaEntity

	err := row.Scan(
		&e.ID, &e.ProjectID, &e.OntologyID, &e.Name, &e.Description,
		&e.PrimarySchema, &e.PrimaryTable, &e.PrimaryColumn,
		&e.IsDeleted, &e.DeletionReason,
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

func scanOntologyEntityAlias(row pgx.Row) (*models.OntologyEntityAlias, error) {
	var a models.OntologyEntityAlias

	err := row.Scan(
		&a.ID, &a.EntityID, &a.Alias, &a.Source, &a.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan entity alias: %w", err)
	}

	return &a, nil
}
