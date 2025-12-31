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

// OntologyEntityRepository provides data access for ontology entities and their occurrences.
type OntologyEntityRepository interface {
	// Entity operations
	Create(ctx context.Context, entity *models.OntologyEntity) error
	GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error)
	GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error)
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error)
	GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error)
	DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error
	Update(ctx context.Context, entity *models.OntologyEntity) error

	// Soft delete operations
	SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error
	Restore(ctx context.Context, entityID uuid.UUID) error

	// Occurrence operations
	CreateOccurrence(ctx context.Context, occ *models.OntologyEntityOccurrence) error
	GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityOccurrence, error)
	GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.OntologyEntityOccurrence, error)
	GetAllOccurrencesByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntityOccurrence, error)

	// Alias operations
	CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error
	GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error)
	DeleteAlias(ctx context.Context, aliasID uuid.UUID) error
}

type ontologyEntityRepository struct{}

// NewOntologyEntityRepository creates a new OntologyEntityRepository.
func NewOntologyEntityRepository() OntologyEntityRepository {
	return &ontologyEntityRepository{}
}

var _ OntologyEntityRepository = (*ontologyEntityRepository)(nil)

// ============================================================================
// Entity Operations
// ============================================================================

func (r *ontologyEntityRepository) Create(ctx context.Context, entity *models.OntologyEntity) error {
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
		return fmt.Errorf("failed to create ontology entity: %w", err)
	}

	return nil
}

func (r *ontologyEntityRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
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
		return nil, fmt.Errorf("failed to query ontology entities: %w", err)
	}
	defer rows.Close()

	var entities []*models.OntologyEntity
	for rows.Next() {
		entity, err := scanOntologyEntity(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating ontology entities: %w", err)
	}

	return entities, nil
}

func (r *ontologyEntityRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
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
		return nil, fmt.Errorf("failed to query ontology entities by project: %w", err)
	}
	defer rows.Close()

	var entities []*models.OntologyEntity
	for rows.Next() {
		entity, err := scanOntologyEntity(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating ontology entities: %w", err)
	}

	return entities, nil
}

func (r *ontologyEntityRepository) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
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
	entity, err := scanOntologyEntity(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Entity not found
		}
		return nil, err
	}

	return entity, nil
}

func (r *ontologyEntityRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_entities WHERE ontology_id = $1`

	_, err := scope.Conn.Exec(ctx, query, ontologyID)
	if err != nil {
		return fmt.Errorf("failed to delete ontology entities: %w", err)
	}

	return nil
}

// ============================================================================
// Occurrence Operations
// ============================================================================

func (r *ontologyEntityRepository) CreateOccurrence(ctx context.Context, occ *models.OntologyEntityOccurrence) error {
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
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (entity_id, schema_name, table_name, column_name) DO NOTHING`

	_, err := scope.Conn.Exec(ctx, query,
		occ.ID, occ.EntityID, occ.SchemaName, occ.TableName, occ.ColumnName, occ.Role, occ.Confidence, occ.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create ontology entity occurrence: %w", err)
	}

	return nil
}

func (r *ontologyEntityRepository) GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
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

	var occurrences []*models.OntologyEntityOccurrence
	for rows.Next() {
		occ, err := scanOntologyEntityOccurrence(rows)
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

func (r *ontologyEntityRepository) GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.OntologyEntityOccurrence, error) {
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

	var occurrences []*models.OntologyEntityOccurrence
	for rows.Next() {
		occ, err := scanOntologyEntityOccurrence(rows)
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

func (r *ontologyEntityRepository) GetAllOccurrencesByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
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

	var occurrences []*models.OntologyEntityOccurrence
	for rows.Next() {
		occ, err := scanOntologyEntityOccurrence(rows)
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

func (r *ontologyEntityRepository) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
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
	entity, err := scanOntologyEntity(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return entity, nil
}

func (r *ontologyEntityRepository) Update(ctx context.Context, entity *models.OntologyEntity) error {
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

func (r *ontologyEntityRepository) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
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

func (r *ontologyEntityRepository) Restore(ctx context.Context, entityID uuid.UUID) error {
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

func (r *ontologyEntityRepository) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
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

func (r *ontologyEntityRepository) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
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

func (r *ontologyEntityRepository) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
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

func scanOntologyEntity(row pgx.Row) (*models.OntologyEntity, error) {
	var e models.OntologyEntity

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
		return nil, fmt.Errorf("failed to scan ontology entity: %w", err)
	}

	return &e, nil
}

func scanOntologyEntityOccurrence(row pgx.Row) (*models.OntologyEntityOccurrence, error) {
	var o models.OntologyEntityOccurrence

	err := row.Scan(
		&o.ID, &o.EntityID, &o.SchemaName, &o.TableName, &o.ColumnName, &o.Role, &o.Confidence, &o.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan ontology entity occurrence: %w", err)
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
