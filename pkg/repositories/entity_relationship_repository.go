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
	Upsert(ctx context.Context, rel *models.EntityRelationship) error
	UpdateDescription(ctx context.Context, id uuid.UUID, description string) error
	UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error
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

	rel.CreatedAt = time.Now()

	if rel.ID == uuid.Nil {
		rel.ID = uuid.New()
	}

	query := `
		INSERT INTO engine_entity_relationships (
			id, ontology_id, source_entity_id, target_entity_id,
			source_column_schema, source_column_table, source_column_name,
			target_column_schema, target_column_table, target_column_name,
			detection_method, confidence, status, cardinality, description, association, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (ontology_id, source_entity_id, target_entity_id,
			source_column_schema, source_column_table, source_column_name,
			target_column_schema, target_column_table, target_column_name)
		DO NOTHING`

	_, err := scope.Conn.Exec(ctx, query,
		rel.ID, rel.OntologyID, rel.SourceEntityID, rel.TargetEntityID,
		rel.SourceColumnSchema, rel.SourceColumnTable, rel.SourceColumnName,
		rel.TargetColumnSchema, rel.TargetColumnTable, rel.TargetColumnName,
		rel.DetectionMethod, rel.Confidence, rel.Status, rel.Cardinality, rel.Description, rel.Association, rel.CreatedAt,
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
		SELECT id, ontology_id, source_entity_id, target_entity_id,
		       source_column_schema, source_column_table, source_column_name,
		       target_column_schema, target_column_table, target_column_name,
		       detection_method, confidence, status, cardinality, description, association, created_at
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
		SELECT id, ontology_id, source_entity_id, target_entity_id,
		       source_column_schema, source_column_table, source_column_name,
		       target_column_schema, target_column_table, target_column_name,
		       detection_method, confidence, status, cardinality, description, association, created_at
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

	query := `
		SELECT r.id, r.ontology_id, r.source_entity_id, r.target_entity_id,
		       r.source_column_schema, r.source_column_table, r.source_column_name,
		       r.target_column_schema, r.target_column_table, r.target_column_name,
		       r.detection_method, r.confidence, r.status, r.cardinality, r.description, r.association, r.created_at
		FROM engine_entity_relationships r
		JOIN engine_ontologies o ON r.ontology_id = o.id
		WHERE o.project_id = $1 AND o.is_active = true
		ORDER BY r.source_column_table, r.source_column_name`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entity relationships by project: %w", err)
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

func (r *entityRelationshipRepository) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	if len(tableNames) == 0 {
		return []*models.EntityRelationship{}, nil
	}

	query := `
		SELECT r.id, r.ontology_id, r.source_entity_id, r.target_entity_id,
		       r.source_column_schema, r.source_column_table, r.source_column_name,
		       r.target_column_schema, r.target_column_table, r.target_column_name,
		       r.detection_method, r.confidence, r.status, r.cardinality, r.description, r.association, r.created_at
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
		SELECT id, ontology_id, source_entity_id, target_entity_id,
		       source_column_schema, source_column_table, source_column_name,
		       target_column_schema, target_column_table, target_column_name,
		       detection_method, confidence, status, cardinality, description, association, created_at
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

func (r *entityRelationshipRepository) UpdateDescription(ctx context.Context, id uuid.UUID, description string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `UPDATE engine_entity_relationships SET description = $1 WHERE id = $2`

	_, err := scope.Conn.Exec(ctx, query, description, id)
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

	query := `UPDATE engine_entity_relationships SET description = $1, association = $2 WHERE id = $3`

	_, err := scope.Conn.Exec(ctx, query, description, association, id)
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

func (r *entityRelationshipRepository) GetByEntityPair(ctx context.Context, ontologyID uuid.UUID, fromEntityID uuid.UUID, toEntityID uuid.UUID) (*models.EntityRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Query for a relationship matching the entity pair
	// Note: There may be multiple relationships between the same entity pair (different columns)
	// We return the first one found
	query := `
		SELECT id, ontology_id, source_entity_id, target_entity_id,
		       source_column_schema, source_column_table, source_column_name,
		       target_column_schema, target_column_table, target_column_name,
		       detection_method, confidence, status, cardinality, description, association, created_at
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

	if rel.ID == uuid.Nil {
		rel.ID = uuid.New()
	}

	if rel.CreatedAt.IsZero() {
		rel.CreatedAt = time.Now()
	}

	// Upsert: Insert or update on conflict
	// The unique constraint is on (ontology_id, source_entity_id, target_entity_id, source/target column details)
	// For MCP tools, we want to upsert based on entity pair, so we'll use ON CONFLICT DO UPDATE
	query := `
		INSERT INTO engine_entity_relationships (
			id, ontology_id, source_entity_id, target_entity_id,
			source_column_schema, source_column_table, source_column_name,
			target_column_schema, target_column_table, target_column_name,
			detection_method, confidence, status, cardinality, description, association, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (ontology_id, source_entity_id, target_entity_id,
			source_column_schema, source_column_table, source_column_name,
			target_column_schema, target_column_table, target_column_name)
		DO UPDATE SET
			cardinality = EXCLUDED.cardinality,
			description = EXCLUDED.description,
			association = EXCLUDED.association,
			status = EXCLUDED.status,
			confidence = EXCLUDED.confidence,
			detection_method = EXCLUDED.detection_method`

	_, err := scope.Conn.Exec(ctx, query,
		rel.ID, rel.OntologyID, rel.SourceEntityID, rel.TargetEntityID,
		rel.SourceColumnSchema, rel.SourceColumnTable, rel.SourceColumnName,
		rel.TargetColumnSchema, rel.TargetColumnTable, rel.TargetColumnName,
		rel.DetectionMethod, rel.Confidence, rel.Status, rel.Cardinality, rel.Description, rel.Association, rel.CreatedAt,
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
		&rel.ID, &rel.OntologyID, &rel.SourceEntityID, &rel.TargetEntityID,
		&rel.SourceColumnSchema, &rel.SourceColumnTable, &rel.SourceColumnName,
		&rel.TargetColumnSchema, &rel.TargetColumnTable, &rel.TargetColumnName,
		&rel.DetectionMethod, &rel.Confidence, &rel.Status, &rel.Cardinality, &rel.Description, &rel.Association, &rel.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan entity relationship: %w", err)
	}

	return &rel, nil
}
