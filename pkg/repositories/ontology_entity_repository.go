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

// OntologyEntityRepository provides data access for ontology entities.
type OntologyEntityRepository interface {
	// Entity operations
	Create(ctx context.Context, entity *models.OntologyEntity) error
	GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error)
	GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error)
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error)
	GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error)
	GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error)
	DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error
	DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error
	DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error
	Update(ctx context.Context, entity *models.OntologyEntity) error

	// Stale marking for incremental refresh
	MarkInferenceEntitiesStale(ctx context.Context, ontologyID uuid.UUID) error
	ClearStaleFlag(ctx context.Context, entityID uuid.UUID) error
	GetStaleEntities(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error)

	// Soft delete operations
	SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error
	Restore(ctx context.Context, entityID uuid.UUID) error

	// Alias operations
	CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error
	GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error)
	GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error)
	DeleteAlias(ctx context.Context, aliasID uuid.UUID) error

	// Key column operations
	CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error
	GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error)
	GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error)

	// Occurrence operations
	CountOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) (int, error)
	GetOccurrenceTablesByEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]string, error)
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

	// Extract provenance from context
	prov, ok := models.GetProvenance(ctx)
	if !ok {
		return fmt.Errorf("provenance context required")
	}

	now := time.Now()
	entity.CreatedAt = now
	entity.UpdatedAt = now

	if entity.ID == uuid.Nil {
		entity.ID = uuid.New()
	}

	// Set provenance fields from context
	entity.Source = prov.Source.String()
	entity.CreatedBy = &prov.UserID

	// Use ON CONFLICT to handle duplicate entity names within the same ontology.
	// On conflict, merge descriptions by preferring the new description if it's non-empty,
	// otherwise keep the existing one. On conflict, also set last_edit_source and updated_by.
	query := `
		INSERT INTO engine_ontology_entities (
			id, project_id, ontology_id, name, description, domain,
			primary_schema, primary_table, primary_column,
			confidence, is_stale,
			source, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (ontology_id, name) DO UPDATE SET
			description = COALESCE(NULLIF(EXCLUDED.description, ''), engine_ontology_entities.description),
			domain = COALESCE(EXCLUDED.domain, engine_ontology_entities.domain),
			primary_schema = EXCLUDED.primary_schema,
			primary_table = EXCLUDED.primary_table,
			primary_column = EXCLUDED.primary_column,
			confidence = EXCLUDED.confidence,
			is_stale = EXCLUDED.is_stale,
			last_edit_source = EXCLUDED.source,
			updated_by = EXCLUDED.created_by,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at`

	row := scope.Conn.QueryRow(ctx, query,
		entity.ID, entity.ProjectID, entity.OntologyID, entity.Name, entity.Description, entity.Domain,
		entity.PrimarySchema, entity.PrimaryTable, entity.PrimaryColumn,
		entity.Confidence, entity.IsStale,
		entity.Source, entity.CreatedBy, entity.CreatedAt, entity.UpdatedAt,
	)

	// Retrieve the actual ID and created_at (may be different if row already existed)
	var actualID uuid.UUID
	var actualCreatedAt time.Time
	if err := row.Scan(&actualID, &actualCreatedAt); err != nil {
		return fmt.Errorf("failed to create/update ontology entity: %w", err)
	}

	// Update the entity struct with actual values from the database
	entity.ID = actualID
	entity.CreatedAt = actualCreatedAt

	return nil
}

func (r *ontologyEntityRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, name, description, domain,
		       primary_schema, primary_table, primary_column,
		       is_deleted, deletion_reason,
		       confidence, is_stale,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
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
		SELECT e.id, e.project_id, e.ontology_id, e.name, e.description, e.domain,
		       e.primary_schema, e.primary_table, e.primary_column,
		       e.is_deleted, e.deletion_reason,
		       e.confidence, e.is_stale,
		       e.source, e.last_edit_source, e.created_by, e.updated_by, e.created_at, e.updated_at
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
		SELECT id, project_id, ontology_id, name, description, domain,
		       primary_schema, primary_table, primary_column,
		       is_deleted, deletion_reason,
		       confidence, is_stale,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
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

// GetByProjectAndName finds an entity by name within the active ontology for a project.
// This uses a JOIN to engine_ontologies (like GetByProject) rather than requiring an exact ontology_id.
func (r *ontologyEntityRepository) GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT e.id, e.project_id, e.ontology_id, e.name, e.description, e.domain,
		       e.primary_schema, e.primary_table, e.primary_column,
		       e.is_deleted, e.deletion_reason,
		       e.confidence, e.is_stale,
		       e.source, e.last_edit_source, e.created_by, e.updated_by, e.created_at, e.updated_at
		FROM engine_ontology_entities e
		JOIN engine_ontologies o ON e.ontology_id = o.id
		WHERE e.project_id = $1 AND e.name = $2 AND o.is_active = true AND NOT e.is_deleted`

	row := scope.Conn.QueryRow(ctx, query, projectID, name)
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

// DeleteInferenceEntitiesByOntology deletes only inference-created entities.
// Preserves entities created by manual or MCP sources.
func (r *ontologyEntityRepository) DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_entities WHERE ontology_id = $1 AND source = 'inference'`

	_, err := scope.Conn.Exec(ctx, query, ontologyID)
	if err != nil {
		return fmt.Errorf("failed to delete inference entities: %w", err)
	}

	return nil
}

// DeleteBySource deletes all entities for a project where source matches the given value.
// This supports re-extraction policy: delete inference items while preserving mcp/manual items.
func (r *ontologyEntityRepository) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_entities WHERE project_id = $1 AND source = $2`

	_, err := scope.Conn.Exec(ctx, query, projectID, source.String())
	if err != nil {
		return fmt.Errorf("failed to delete entities by source: %w", err)
	}

	return nil
}

// MarkInferenceEntitiesStale marks all inference-created entities as stale for re-enrichment.
// This is used during ontology refresh to preserve manual/MCP entities while allowing
// inference entities to be re-evaluated.
func (r *ontologyEntityRepository) MarkInferenceEntitiesStale(ctx context.Context, ontologyID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_ontology_entities
		SET is_stale = true, updated_at = $2
		WHERE ontology_id = $1 AND source = 'inference' AND NOT is_deleted`

	_, err := scope.Conn.Exec(ctx, query, ontologyID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to mark inference entities as stale: %w", err)
	}

	return nil
}

// ClearStaleFlag clears the is_stale flag after re-enrichment.
func (r *ontologyEntityRepository) ClearStaleFlag(ctx context.Context, entityID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `UPDATE engine_ontology_entities SET is_stale = false, updated_at = $2 WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query, entityID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to clear stale flag: %w", err)
	}

	return nil
}

// GetStaleEntities returns all entities marked as stale for the given ontology.
func (r *ontologyEntityRepository) GetStaleEntities(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, name, description, domain,
		       primary_schema, primary_table, primary_column,
		       is_deleted, deletion_reason,
		       confidence, is_stale,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_ontology_entities
		WHERE ontology_id = $1 AND is_stale = true AND NOT is_deleted
		ORDER BY name`

	rows, err := scope.Conn.Query(ctx, query, ontologyID)
	if err != nil {
		return nil, fmt.Errorf("failed to query stale entities: %w", err)
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
		return nil, fmt.Errorf("error iterating stale entities: %w", err)
	}

	return entities, nil
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
		SELECT id, project_id, ontology_id, name, description, domain,
		       primary_schema, primary_table, primary_column,
		       is_deleted, deletion_reason,
		       confidence, is_stale,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
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

	// Extract provenance from context
	prov, ok := models.GetProvenance(ctx)
	if !ok {
		return fmt.Errorf("provenance context required")
	}

	entity.UpdatedAt = time.Now()

	// Set provenance fields from context
	lastEditSource := prov.Source.String()
	entity.LastEditSource = &lastEditSource
	entity.UpdatedBy = &prov.UserID

	query := `
		UPDATE engine_ontology_entities
		SET name = $2, description = $3, domain = $4,
		    primary_schema = $5, primary_table = $6, primary_column = $7,
		    confidence = $8, is_stale = $9,
		    last_edit_source = $10, updated_by = $11, updated_at = $12
		WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query,
		entity.ID, entity.Name, entity.Description, entity.Domain,
		entity.PrimarySchema, entity.PrimaryTable, entity.PrimaryColumn,
		entity.Confidence, entity.IsStale,
		entity.LastEditSource, entity.UpdatedBy, entity.UpdatedAt,
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

func (r *ontologyEntityRepository) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT a.id, a.entity_id, a.alias, a.source, a.created_at
		FROM engine_ontology_entity_aliases a
		JOIN engine_ontology_entities e ON a.entity_id = e.id
		JOIN engine_ontologies o ON e.ontology_id = o.id
		WHERE e.project_id = $1 AND o.is_active = true AND NOT e.is_deleted
		ORDER BY a.entity_id, a.alias`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query all entity aliases by project: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]*models.OntologyEntityAlias)
	for rows.Next() {
		alias, err := scanOntologyEntityAlias(rows)
		if err != nil {
			return nil, err
		}
		result[alias.EntityID] = append(result[alias.EntityID], alias)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity aliases: %w", err)
	}

	return result, nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanOntologyEntity(row pgx.Row) (*models.OntologyEntity, error) {
	var e models.OntologyEntity
	var domain *string // domain can be NULL in database

	err := row.Scan(
		&e.ID, &e.ProjectID, &e.OntologyID, &e.Name, &e.Description, &domain,
		&e.PrimarySchema, &e.PrimaryTable, &e.PrimaryColumn,
		&e.IsDeleted, &e.DeletionReason,
		&e.Confidence, &e.IsStale,
		&e.Source, &e.LastEditSource, &e.CreatedBy, &e.UpdatedBy, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan ontology entity: %w", err)
	}

	if domain != nil {
		e.Domain = *domain
	}

	return &e, nil
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

// ============================================================================
// Key Column Operations
// ============================================================================

func (r *ontologyEntityRepository) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	keyColumn.CreatedAt = time.Now()

	if keyColumn.ID == uuid.Nil {
		keyColumn.ID = uuid.New()
	}

	query := `
		INSERT INTO engine_ontology_entity_key_columns (id, entity_id, column_name, synonyms, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (entity_id, column_name) DO NOTHING`

	_, err := scope.Conn.Exec(ctx, query,
		keyColumn.ID, keyColumn.EntityID, keyColumn.ColumnName, keyColumn.Synonyms, keyColumn.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create entity key column: %w", err)
	}

	return nil
}

func (r *ontologyEntityRepository) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, entity_id, column_name, synonyms, created_at
		FROM engine_ontology_entity_key_columns
		WHERE entity_id = $1
		ORDER BY column_name`

	rows, err := scope.Conn.Query(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity key columns: %w", err)
	}
	defer rows.Close()

	var keyColumns []*models.OntologyEntityKeyColumn
	for rows.Next() {
		kc, err := scanOntologyEntityKeyColumn(rows)
		if err != nil {
			return nil, err
		}
		keyColumns = append(keyColumns, kc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity key columns: %w", err)
	}

	return keyColumns, nil
}

func (r *ontologyEntityRepository) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT k.id, k.entity_id, k.column_name, k.synonyms, k.created_at
		FROM engine_ontology_entity_key_columns k
		JOIN engine_ontology_entities e ON k.entity_id = e.id
		JOIN engine_ontologies o ON e.ontology_id = o.id
		WHERE e.project_id = $1 AND o.is_active = true AND NOT e.is_deleted
		ORDER BY k.entity_id, k.column_name`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query all entity key columns by project: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]*models.OntologyEntityKeyColumn)
	for rows.Next() {
		kc, err := scanOntologyEntityKeyColumn(rows)
		if err != nil {
			return nil, err
		}
		result[kc.EntityID] = append(result[kc.EntityID], kc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity key columns: %w", err)
	}

	return result, nil
}

func scanOntologyEntityKeyColumn(row pgx.Row) (*models.OntologyEntityKeyColumn, error) {
	var kc models.OntologyEntityKeyColumn

	err := row.Scan(
		&kc.ID, &kc.EntityID, &kc.ColumnName, &kc.Synonyms, &kc.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan entity key column: %w", err)
	}

	return &kc, nil
}

// ============================================================================
// Occurrence Operations
// ============================================================================
// NOTE: The engine_ontology_entity_occurrences table was dropped in migration 030.
// These methods are kept for interface compatibility but return empty results.

// CountOccurrencesByEntity returns the count of non-deleted occurrences for an entity.
// NOTE: Returns 0 - the occurrences table was dropped in migration 030.
func (r *ontologyEntityRepository) CountOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) (int, error) {
	// Table dropped in migration 030 - return 0 for interface compatibility
	return 0, nil
}

// GetOccurrenceTablesByEntity returns distinct table names where an entity has occurrences.
// NOTE: Returns empty slice - the occurrences table was dropped in migration 030.
func (r *ontologyEntityRepository) GetOccurrenceTablesByEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]string, error) {
	// Table dropped in migration 030 - return empty for interface compatibility
	return []string{}, nil
}
