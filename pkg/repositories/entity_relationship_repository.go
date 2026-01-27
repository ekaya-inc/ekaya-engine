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

// EntityRelationshipRepository provides data access for entity relationships.
type EntityRelationshipRepository interface {
	Create(ctx context.Context, rel *models.EntityRelationship) error
	GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error)
	GetByOntologyGroupedByTarget(ctx context.Context, ontologyID uuid.UUID) (map[uuid.UUID][]*models.EntityRelationship, error)
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error)
	GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error)
	GetByTargetEntity(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error)
	GetByEntityPair(ctx context.Context, ontologyID uuid.UUID, fromEntityID uuid.UUID, toEntityID uuid.UUID) (*models.EntityRelationship, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.EntityRelationship, error)
	Upsert(ctx context.Context, rel *models.EntityRelationship) error
	Update(ctx context.Context, rel *models.EntityRelationship) error
	UpdateDescription(ctx context.Context, id uuid.UUID, description string) error
	UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error
	DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error

	// Stale marking for incremental refresh
	MarkInferenceRelationshipsStale(ctx context.Context, ontologyID uuid.UUID) error
	ClearStaleFlag(ctx context.Context, relationshipID uuid.UUID) error
	GetStaleRelationships(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error)
}

type entityRelationshipRepository struct{}

// NewEntityRelationshipRepository creates a new EntityRelationshipRepository.
func NewEntityRelationshipRepository() EntityRelationshipRepository {
	return &entityRelationshipRepository{}
}

var _ EntityRelationshipRepository = (*entityRelationshipRepository)(nil)

func (r *entityRelationshipRepository) Create(ctx context.Context, rel *models.EntityRelationship) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Extract provenance from context
	prov, ok := models.GetProvenance(ctx)
	if !ok {
		return fmt.Errorf("provenance context required")
	}

	rel.CreatedAt = time.Now()

	if rel.ID == uuid.Nil {
		rel.ID = uuid.New()
	}

	// Default cardinality to "unknown" if not specified
	if rel.Cardinality == "" {
		rel.Cardinality = "unknown"
	}

	// Set provenance fields from context (only if not already set explicitly)
	if rel.Source == "" {
		rel.Source = prov.Source.String()
	}
	// Only set CreatedBy if there's a valid user ID (not the nil UUID)
	if prov.UserID != uuid.Nil {
		rel.CreatedBy = &prov.UserID
	} else {
		rel.CreatedBy = nil
	}

	// Use ON CONFLICT DO UPDATE to handle re-discovery during ontology refresh.
	// When an existing relationship is re-discovered:
	// - Clear is_stale flag (relationship still exists in schema)
	// - Update confidence and detection_method (may have changed)
	// - Update column IDs (may have changed if schema was refreshed)
	// - Preserve description/association if already enriched (don't overwrite with NULL)
	// - On conflict, also set last_edit_source and updated_by
	now := time.Now()
	// Use subquery to get project_id from source entity
	query := `
		INSERT INTO engine_entity_relationships (
			id, project_id, ontology_id, source_entity_id, target_entity_id,
			source_column_schema, source_column_table, source_column_name, source_column_id,
			target_column_schema, target_column_table, target_column_name, target_column_id,
			detection_method, confidence, status, cardinality, description, association,
			is_stale, source, created_by, created_at
		) VALUES ($1, (SELECT project_id FROM engine_ontology_entities WHERE id = $3), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (ontology_id, source_entity_id, target_entity_id,
			source_column_schema, source_column_table, source_column_name,
			target_column_schema, target_column_table, target_column_name)
		DO UPDATE SET
			is_stale = false,
			confidence = EXCLUDED.confidence,
			detection_method = EXCLUDED.detection_method,
			source_column_id = EXCLUDED.source_column_id,
			target_column_id = EXCLUDED.target_column_id,
			last_edit_source = EXCLUDED.source,
			updated_by = EXCLUDED.created_by,
			updated_at = $23`

	_, err := scope.Conn.Exec(ctx, query,
		rel.ID, rel.OntologyID, rel.SourceEntityID, rel.TargetEntityID,
		rel.SourceColumnSchema, rel.SourceColumnTable, rel.SourceColumnName, rel.SourceColumnID,
		rel.TargetColumnSchema, rel.TargetColumnTable, rel.TargetColumnName, rel.TargetColumnID,
		rel.DetectionMethod, rel.Confidence, rel.Status, rel.Cardinality, rel.Description, rel.Association,
		rel.IsStale, rel.Source, rel.CreatedBy, rel.CreatedAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("failed to create entity relationship: %w", err)
	}

	return nil
}

func (r *entityRelationshipRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, source_entity_id, target_entity_id,
		       source_column_schema, source_column_table, source_column_name,
		       target_column_schema, target_column_table, target_column_name,
		       detection_method, confidence, status, cardinality, description, association,
		       is_stale, source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_entity_relationships
		WHERE ontology_id = $1
		ORDER BY source_column_table, source_column_name`

	rows, err := scope.Conn.Query(ctx, query, ontologyID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity relationships: %w", err)
	}
	defer rows.Close()

	var relationships []*models.EntityRelationship
	for rows.Next() {
		rel, err := scanEntityRelationship(rows)
		if err != nil {
			return nil, err
		}
		relationships = append(relationships, rel)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity relationships: %w", err)
	}

	return relationships, nil
}

func (r *entityRelationshipRepository) GetByOntologyGroupedByTarget(ctx context.Context, ontologyID uuid.UUID) (map[uuid.UUID][]*models.EntityRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, source_entity_id, target_entity_id,
		       source_column_schema, source_column_table, source_column_name,
		       target_column_schema, target_column_table, target_column_name,
		       detection_method, confidence, status, cardinality, description, association,
		       is_stale, source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_entity_relationships
		WHERE ontology_id = $1
		ORDER BY target_entity_id, source_column_table, source_column_name`

	rows, err := scope.Conn.Query(ctx, query, ontologyID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity relationships: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]*models.EntityRelationship)
	for rows.Next() {
		rel, err := scanEntityRelationship(rows)
		if err != nil {
			return nil, err
		}
		result[rel.TargetEntityID] = append(result[rel.TargetEntityID], rel)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity relationships: %w", err)
	}

	return result, nil
}

func (r *entityRelationshipRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// JOIN with engine_schema_columns to get column types for source and target columns
	query := `
		SELECT r.id, r.project_id, r.ontology_id, r.source_entity_id, r.target_entity_id,
		       r.source_column_schema, r.source_column_table, r.source_column_name,
		       r.source_column_id, r.target_column_schema, r.target_column_table, r.target_column_name,
		       r.target_column_id, r.detection_method, r.confidence, r.status, r.cardinality,
		       r.description, r.association, r.is_stale, r.source, r.last_edit_source,
		       r.created_by, r.updated_by, r.created_at, r.updated_at,
		       COALESCE(sc.data_type, '') as source_column_type,
		       COALESCE(tc.data_type, '') as target_column_type
		FROM engine_entity_relationships r
		JOIN engine_ontologies o ON r.ontology_id = o.id
		LEFT JOIN engine_schema_columns sc ON r.source_column_id = sc.id
		LEFT JOIN engine_schema_columns tc ON r.target_column_id = tc.id
		WHERE o.project_id = $1 AND o.is_active = true
		ORDER BY r.source_column_table, r.source_column_name`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity relationships by project: %w", err)
	}
	defer rows.Close()

	var relationships []*models.EntityRelationship
	for rows.Next() {
		rel, err := scanEntityRelationshipWithColumnTypes(rows)
		if err != nil {
			return nil, err
		}
		relationships = append(relationships, rel)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity relationships: %w", err)
	}

	return relationships, nil
}

func (r *entityRelationshipRepository) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	if len(tableNames) == 0 {
		return []*models.EntityRelationship{}, nil
	}

	query := `
		SELECT r.id, r.project_id, r.ontology_id, r.source_entity_id, r.target_entity_id,
		       r.source_column_schema, r.source_column_table, r.source_column_name,
		       r.target_column_schema, r.target_column_table, r.target_column_name,
		       r.detection_method, r.confidence, r.status, r.cardinality, r.description, r.association,
		       r.is_stale, r.source, r.last_edit_source, r.created_by, r.updated_by, r.created_at, r.updated_at
		FROM engine_entity_relationships r
		JOIN engine_ontologies o ON r.ontology_id = o.id
		WHERE o.project_id = $1 AND o.is_active = true
		  AND (r.source_column_table = ANY($2) OR r.target_column_table = ANY($2))
		ORDER BY r.source_column_table, r.source_column_name`

	rows, err := scope.Conn.Query(ctx, query, projectID, tableNames)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity relationships by tables: %w", err)
	}
	defer rows.Close()

	var relationships []*models.EntityRelationship
	for rows.Next() {
		rel, err := scanEntityRelationship(rows)
		if err != nil {
			return nil, err
		}
		relationships = append(relationships, rel)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity relationships: %w", err)
	}

	return relationships, nil
}

func (r *entityRelationshipRepository) GetByTargetEntity(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, source_entity_id, target_entity_id,
		       source_column_schema, source_column_table, source_column_name,
		       target_column_schema, target_column_table, target_column_name,
		       detection_method, confidence, status, cardinality, description, association,
		       is_stale, source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_entity_relationships
		WHERE target_entity_id = $1
		ORDER BY source_column_table, source_column_name`

	rows, err := scope.Conn.Query(ctx, query, entityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity relationships by target entity: %w", err)
	}
	defer rows.Close()

	var relationships []*models.EntityRelationship
	for rows.Next() {
		rel, err := scanEntityRelationship(rows)
		if err != nil {
			return nil, err
		}
		relationships = append(relationships, rel)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating entity relationships: %w", err)
	}

	return relationships, nil
}

func (r *entityRelationshipRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.EntityRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, source_entity_id, target_entity_id,
		       source_column_schema, source_column_table, source_column_name,
		       target_column_schema, target_column_table, target_column_name,
		       detection_method, confidence, status, cardinality, description, association,
		       is_stale, source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_entity_relationships
		WHERE id = $1`

	row := scope.Conn.QueryRow(ctx, query, id)
	rel, err := scanEntityRelationship(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return rel, nil
}

func (r *entityRelationshipRepository) Update(ctx context.Context, rel *models.EntityRelationship) error {
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
	rel.UpdatedAt = &now

	// Set provenance fields from context
	lastEditSource := prov.Source.String()
	rel.LastEditSource = &lastEditSource
	// Only set UpdatedBy if there's a valid user ID (not the nil UUID)
	if prov.UserID != uuid.Nil {
		rel.UpdatedBy = &prov.UserID
	} else {
		rel.UpdatedBy = nil
	}

	query := `
		UPDATE engine_entity_relationships
		SET cardinality = $2, description = $3, association = $4,
		    status = $5, confidence = $6, is_stale = $7,
		    last_edit_source = $8, updated_by = $9, updated_at = $10
		WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query,
		rel.ID, rel.Cardinality, rel.Description, rel.Association,
		rel.Status, rel.Confidence, rel.IsStale,
		rel.LastEditSource, rel.UpdatedBy, rel.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update entity relationship: %w", err)
	}

	return nil
}

func (r *entityRelationshipRepository) UpdateDescription(ctx context.Context, id uuid.UUID, description string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Also clears is_stale since enrichment means the relationship was re-evaluated
	now := time.Now()
	query := `UPDATE engine_entity_relationships SET description = $1, is_stale = false, updated_at = $3 WHERE id = $2`

	_, err := scope.Conn.Exec(ctx, query, description, id, now)
	if err != nil {
		return fmt.Errorf("failed to update entity relationship description: %w", err)
	}

	return nil
}

func (r *entityRelationshipRepository) UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Also clears is_stale since enrichment means the relationship was re-evaluated
	now := time.Now()
	query := `UPDATE engine_entity_relationships SET description = $1, association = $2, is_stale = false, updated_at = $4 WHERE id = $3`

	_, err := scope.Conn.Exec(ctx, query, description, association, id, now)
	if err != nil {
		return fmt.Errorf("failed to update entity relationship description and association: %w", err)
	}

	return nil
}

func (r *entityRelationshipRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_entity_relationships WHERE ontology_id = $1`

	_, err := scope.Conn.Exec(ctx, query, ontologyID)
	if err != nil {
		return fmt.Errorf("failed to delete entity relationships: %w", err)
	}

	return nil
}

// DeleteBySource deletes all relationships for a project where source matches the given value.
// This supports re-extraction policy: delete inference items while preserving mcp/manual items.
func (r *entityRelationshipRepository) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Join with ontologies to filter by project_id since relationships don't have project_id directly
	query := `
		DELETE FROM engine_entity_relationships r
		USING engine_ontologies o
		WHERE r.ontology_id = o.id
		  AND o.project_id = $1
		  AND r.source = $2`

	_, err := scope.Conn.Exec(ctx, query, projectID, source.String())
	if err != nil {
		return fmt.Errorf("failed to delete relationships by source: %w", err)
	}

	return nil
}

// MarkInferenceRelationshipsStale marks all inference-created relationships as stale for re-enrichment.
// This is used during ontology refresh to preserve manual/MCP relationships while allowing
// inference relationships to be re-evaluated.
func (r *entityRelationshipRepository) MarkInferenceRelationshipsStale(ctx context.Context, ontologyID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	query := `
		UPDATE engine_entity_relationships
		SET is_stale = true, updated_at = $2
		WHERE ontology_id = $1 AND source = 'inferred'`

	_, err := scope.Conn.Exec(ctx, query, ontologyID, now)
	if err != nil {
		return fmt.Errorf("failed to mark inference relationships as stale: %w", err)
	}

	return nil
}

// ClearStaleFlag clears the is_stale flag after re-enrichment.
func (r *entityRelationshipRepository) ClearStaleFlag(ctx context.Context, relationshipID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	query := `UPDATE engine_entity_relationships SET is_stale = false, updated_at = $2 WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query, relationshipID, now)
	if err != nil {
		return fmt.Errorf("failed to clear stale flag: %w", err)
	}

	return nil
}

// GetStaleRelationships returns all relationships marked as stale for the given ontology.
func (r *entityRelationshipRepository) GetStaleRelationships(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, ontology_id, source_entity_id, target_entity_id,
		       source_column_schema, source_column_table, source_column_name,
		       target_column_schema, target_column_table, target_column_name,
		       detection_method, confidence, status, cardinality, description, association,
		       is_stale, source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_entity_relationships
		WHERE ontology_id = $1 AND is_stale = true
		ORDER BY source_column_table, source_column_name`

	rows, err := scope.Conn.Query(ctx, query, ontologyID)
	if err != nil {
		return nil, fmt.Errorf("failed to query stale relationships: %w", err)
	}
	defer rows.Close()

	var relationships []*models.EntityRelationship
	for rows.Next() {
		rel, err := scanEntityRelationship(rows)
		if err != nil {
			return nil, err
		}
		relationships = append(relationships, rel)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating stale relationships: %w", err)
	}

	return relationships, nil
}

func (r *entityRelationshipRepository) GetByEntityPair(ctx context.Context, ontologyID uuid.UUID, fromEntityID uuid.UUID, toEntityID uuid.UUID) (*models.EntityRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Query for a relationship matching the entity pair
	// Note: There may be multiple relationships between the same entity pair (different columns)
	// We return the first one found
	query := `
		SELECT id, project_id, ontology_id, source_entity_id, target_entity_id,
		       source_column_schema, source_column_table, source_column_name,
		       target_column_schema, target_column_table, target_column_name,
		       detection_method, confidence, status, cardinality, description, association,
		       is_stale, source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_entity_relationships
		WHERE ontology_id = $1 AND source_entity_id = $2 AND target_entity_id = $3
		ORDER BY created_at DESC
		LIMIT 1`

	row := scope.Conn.QueryRow(ctx, query, ontologyID, fromEntityID, toEntityID)
	rel, err := scanEntityRelationship(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, err
	}

	return rel, nil
}

func (r *entityRelationshipRepository) Upsert(ctx context.Context, rel *models.EntityRelationship) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Extract provenance from context
	prov, ok := models.GetProvenance(ctx)
	if !ok {
		return fmt.Errorf("provenance context required")
	}

	if rel.ID == uuid.Nil {
		rel.ID = uuid.New()
	}

	if rel.CreatedAt.IsZero() {
		rel.CreatedAt = time.Now()
	}

	// Set provenance fields from context (only if not already set explicitly)
	if rel.Source == "" {
		rel.Source = prov.Source.String()
	}
	if prov.UserID != uuid.Nil {
		rel.CreatedBy = &prov.UserID
	} else {
		rel.CreatedBy = nil
	}

	now := time.Now()

	// Upsert: Insert or update on conflict
	// The unique constraint is on (ontology_id, source_entity_id, target_entity_id, source/target column details)
	// For MCP tools, we want to upsert based on entity pair, so we'll use ON CONFLICT DO UPDATE
	// On conflict, also set last_edit_source and updated_by from provenance
	// Use subquery to get project_id from source entity
	query := `
		INSERT INTO engine_entity_relationships (
			id, project_id, ontology_id, source_entity_id, target_entity_id,
			source_column_schema, source_column_table, source_column_name, source_column_id,
			target_column_schema, target_column_table, target_column_name, target_column_id,
			detection_method, confidence, status, cardinality, description, association,
			is_stale, source, created_by, created_at
		) VALUES ($1, (SELECT project_id FROM engine_ontology_entities WHERE id = $3), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (ontology_id, source_entity_id, target_entity_id,
			source_column_schema, source_column_table, source_column_name,
			target_column_schema, target_column_table, target_column_name)
		DO UPDATE SET
			cardinality = EXCLUDED.cardinality,
			description = EXCLUDED.description,
			association = EXCLUDED.association,
			status = EXCLUDED.status,
			confidence = EXCLUDED.confidence,
			detection_method = EXCLUDED.detection_method,
			source_column_id = EXCLUDED.source_column_id,
			target_column_id = EXCLUDED.target_column_id,
			is_stale = EXCLUDED.is_stale,
			last_edit_source = EXCLUDED.source,
			updated_by = EXCLUDED.created_by,
			updated_at = $23`

	_, err := scope.Conn.Exec(ctx, query,
		rel.ID, rel.OntologyID, rel.SourceEntityID, rel.TargetEntityID,
		rel.SourceColumnSchema, rel.SourceColumnTable, rel.SourceColumnName, rel.SourceColumnID,
		rel.TargetColumnSchema, rel.TargetColumnTable, rel.TargetColumnName, rel.TargetColumnID,
		rel.DetectionMethod, rel.Confidence, rel.Status, rel.Cardinality, rel.Description, rel.Association,
		rel.IsStale, rel.Source, rel.CreatedBy, rel.CreatedAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert entity relationship: %w", err)
	}

	return nil
}

func (r *entityRelationshipRepository) Delete(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_entity_relationships WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete entity relationship: %w", err)
	}

	return nil
}

func scanEntityRelationship(row pgx.Row) (*models.EntityRelationship, error) {
	var rel models.EntityRelationship

	err := row.Scan(
		&rel.ID, &rel.ProjectID, &rel.OntologyID, &rel.SourceEntityID, &rel.TargetEntityID,
		&rel.SourceColumnSchema, &rel.SourceColumnTable, &rel.SourceColumnName,
		&rel.TargetColumnSchema, &rel.TargetColumnTable, &rel.TargetColumnName,
		&rel.DetectionMethod, &rel.Confidence, &rel.Status, &rel.Cardinality, &rel.Description, &rel.Association,
		&rel.IsStale, &rel.Source, &rel.LastEditSource, &rel.CreatedBy, &rel.UpdatedBy, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan entity relationship: %w", err)
	}

	return &rel, nil
}

// scanEntityRelationshipWithColumnTypes scans a row that includes source_column_id, target_column_id,
// and the joined column types from engine_schema_columns.
func scanEntityRelationshipWithColumnTypes(row pgx.Row) (*models.EntityRelationship, error) {
	var rel models.EntityRelationship

	err := row.Scan(
		&rel.ID, &rel.ProjectID, &rel.OntologyID, &rel.SourceEntityID, &rel.TargetEntityID,
		&rel.SourceColumnSchema, &rel.SourceColumnTable, &rel.SourceColumnName,
		&rel.SourceColumnID, &rel.TargetColumnSchema, &rel.TargetColumnTable, &rel.TargetColumnName,
		&rel.TargetColumnID, &rel.DetectionMethod, &rel.Confidence, &rel.Status, &rel.Cardinality,
		&rel.Description, &rel.Association, &rel.IsStale, &rel.Source, &rel.LastEditSource,
		&rel.CreatedBy, &rel.UpdatedBy, &rel.CreatedAt, &rel.UpdatedAt,
		&rel.SourceColumnType, &rel.TargetColumnType,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan entity relationship with column types: %w", err)
	}

	return &rel, nil
}
