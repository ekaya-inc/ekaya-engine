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

// ColumnMetadataRepository provides data access for column metadata.
// Column metadata is keyed by schema_column_id (FK to engine_schema_columns).
type ColumnMetadataRepository interface {
	// Upsert creates or updates column metadata based on project_id + schema_column_id.
	// Used for MCP tools and manual edits. Respects the source parameter and updates last_edit_source.
	Upsert(ctx context.Context, meta *models.ColumnMetadata) error

	// UpsertFromExtraction creates or updates column metadata from the extraction pipeline.
	// Always sets source='inferred'. Does not set last_edit_source (this is a creation, not an edit).
	UpsertFromExtraction(ctx context.Context, meta *models.ColumnMetadata) error

	// GetBySchemaColumnID retrieves column metadata by schema column ID.
	GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error)

	// GetByProject retrieves all column metadata for a project.
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error)

	// GetBySchemaColumnIDs retrieves column metadata for multiple schema column IDs.
	GetBySchemaColumnIDs(ctx context.Context, schemaColumnIDs []uuid.UUID) ([]*models.ColumnMetadata, error)

	// Delete removes column metadata by ID.
	Delete(ctx context.Context, id uuid.UUID) error

	// DeleteBySchemaColumnID removes column metadata by schema column ID.
	DeleteBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) error
}

type columnMetadataRepository struct{}

// NewColumnMetadataRepository creates a new ColumnMetadataRepository.
func NewColumnMetadataRepository() ColumnMetadataRepository {
	return &columnMetadataRepository{}
}

var _ ColumnMetadataRepository = (*columnMetadataRepository)(nil)

// Upsert creates or updates column metadata for MCP/manual edits.
// Respects the provided source and sets last_edit_source on update.
func (r *columnMetadataRepository) Upsert(ctx context.Context, meta *models.ColumnMetadata) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	if meta.SchemaColumnID == uuid.Nil {
		return fmt.Errorf("schema_column_id is required")
	}

	now := time.Now()

	// Default source to 'manual' if not set (MCP/manual edits should specify source)
	if meta.Source == "" {
		meta.Source = models.ProvenanceManual
	}

	// Get features JSONB value
	featuresValue, err := meta.Features.Value()
	if err != nil {
		return fmt.Errorf("failed to marshal features: %w", err)
	}

	query := `
		INSERT INTO engine_ontology_column_metadata (
			project_id, schema_column_id,
			classification_path, purpose, semantic_type, role, description, confidence,
			features,
			needs_enum_analysis, needs_fk_resolution, needs_cross_column_check,
			needs_clarification, clarification_question,
			is_sensitive,
			analyzed_at, llm_model_used,
			source, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		ON CONFLICT (project_id, schema_column_id)
		DO UPDATE SET
			classification_path = COALESCE(EXCLUDED.classification_path, engine_ontology_column_metadata.classification_path),
			purpose = COALESCE(EXCLUDED.purpose, engine_ontology_column_metadata.purpose),
			semantic_type = COALESCE(EXCLUDED.semantic_type, engine_ontology_column_metadata.semantic_type),
			role = COALESCE(EXCLUDED.role, engine_ontology_column_metadata.role),
			description = COALESCE(EXCLUDED.description, engine_ontology_column_metadata.description),
			confidence = COALESCE(EXCLUDED.confidence, engine_ontology_column_metadata.confidence),
			features = COALESCE(EXCLUDED.features, engine_ontology_column_metadata.features),
			needs_enum_analysis = EXCLUDED.needs_enum_analysis,
			needs_fk_resolution = EXCLUDED.needs_fk_resolution,
			needs_cross_column_check = EXCLUDED.needs_cross_column_check,
			needs_clarification = EXCLUDED.needs_clarification,
			clarification_question = COALESCE(EXCLUDED.clarification_question, engine_ontology_column_metadata.clarification_question),
			is_sensitive = COALESCE(EXCLUDED.is_sensitive, engine_ontology_column_metadata.is_sensitive),
			analyzed_at = COALESCE(EXCLUDED.analyzed_at, engine_ontology_column_metadata.analyzed_at),
			llm_model_used = COALESCE(EXCLUDED.llm_model_used, engine_ontology_column_metadata.llm_model_used),
			last_edit_source = $22,
			updated_by = $23,
			updated_at = $24
		RETURNING id, created_at, updated_at`

	err = scope.Conn.QueryRow(ctx, query,
		meta.ProjectID,
		meta.SchemaColumnID,
		meta.ClassificationPath,
		meta.Purpose,
		meta.SemanticType,
		meta.Role,
		meta.Description,
		meta.Confidence,
		featuresValue,
		meta.NeedsEnumAnalysis,
		meta.NeedsFKResolution,
		meta.NeedsCrossColumnCheck,
		meta.NeedsClarification,
		meta.ClarificationQuestion,
		meta.IsSensitive,
		meta.AnalyzedAt,
		meta.LLMModelUsed,
		meta.Source,
		meta.CreatedBy,
		now,
		now,
		meta.Source,      // last_edit_source on update
		meta.UpdatedBy,   // updated_by
		now,              // updated_at
	).Scan(&meta.ID, &meta.CreatedAt, &meta.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert column metadata: %w", err)
	}

	return nil
}

// UpsertFromExtraction creates or updates column metadata from the extraction pipeline.
// Always sets source='inferred'. Does not set last_edit_source.
func (r *columnMetadataRepository) UpsertFromExtraction(ctx context.Context, meta *models.ColumnMetadata) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	if meta.SchemaColumnID == uuid.Nil {
		return fmt.Errorf("schema_column_id is required")
	}

	now := time.Now()

	// Always use 'inferred' for extraction pipeline
	meta.Source = models.ProvenanceInferred

	// Get features JSONB value
	featuresValue, err := meta.Features.Value()
	if err != nil {
		return fmt.Errorf("failed to marshal features: %w", err)
	}

	// Extraction inserts/updates all fields.
	// On conflict, replace all inferred fields except user overrides (is_sensitive with provenance).
	query := `
		INSERT INTO engine_ontology_column_metadata (
			project_id, schema_column_id,
			classification_path, purpose, semantic_type, role, description, confidence,
			features,
			needs_enum_analysis, needs_fk_resolution, needs_cross_column_check,
			needs_clarification, clarification_question,
			analyzed_at, llm_model_used,
			source, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		ON CONFLICT (project_id, schema_column_id)
		DO UPDATE SET
			classification_path = EXCLUDED.classification_path,
			purpose = EXCLUDED.purpose,
			semantic_type = EXCLUDED.semantic_type,
			role = EXCLUDED.role,
			description = EXCLUDED.description,
			confidence = EXCLUDED.confidence,
			features = EXCLUDED.features,
			needs_enum_analysis = EXCLUDED.needs_enum_analysis,
			needs_fk_resolution = EXCLUDED.needs_fk_resolution,
			needs_cross_column_check = EXCLUDED.needs_cross_column_check,
			needs_clarification = EXCLUDED.needs_clarification,
			clarification_question = EXCLUDED.clarification_question,
			analyzed_at = EXCLUDED.analyzed_at,
			llm_model_used = EXCLUDED.llm_model_used,
			updated_at = EXCLUDED.updated_at
			-- Note: source, last_edit_source, is_sensitive are NOT updated
			-- This preserves manual/MCP overrides when re-running extraction
		RETURNING id, created_at, updated_at`

	err = scope.Conn.QueryRow(ctx, query,
		meta.ProjectID,
		meta.SchemaColumnID,
		meta.ClassificationPath,
		meta.Purpose,
		meta.SemanticType,
		meta.Role,
		meta.Description,
		meta.Confidence,
		featuresValue,
		meta.NeedsEnumAnalysis,
		meta.NeedsFKResolution,
		meta.NeedsCrossColumnCheck,
		meta.NeedsClarification,
		meta.ClarificationQuestion,
		meta.AnalyzedAt,
		meta.LLMModelUsed,
		meta.Source,
		meta.CreatedBy,
		now,
		now,
	).Scan(&meta.ID, &meta.CreatedAt, &meta.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert column metadata from extraction: %w", err)
	}

	return nil
}

// GetBySchemaColumnID retrieves column metadata by schema column ID.
func (r *columnMetadataRepository) GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, schema_column_id,
		       classification_path, purpose, semantic_type, role, description, confidence,
		       features,
		       needs_enum_analysis, needs_fk_resolution, needs_cross_column_check,
		       needs_clarification, clarification_question,
		       is_sensitive,
		       analyzed_at, llm_model_used,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_ontology_column_metadata
		WHERE schema_column_id = $1`

	row := scope.Conn.QueryRow(ctx, query, schemaColumnID)
	return scanColumnMetadata(row)
}

// GetByProject retrieves all column metadata for a project.
func (r *columnMetadataRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, schema_column_id,
		       classification_path, purpose, semantic_type, role, description, confidence,
		       features,
		       needs_enum_analysis, needs_fk_resolution, needs_cross_column_check,
		       needs_clarification, clarification_question,
		       is_sensitive,
		       analyzed_at, llm_model_used,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_ontology_column_metadata
		WHERE project_id = $1
		ORDER BY schema_column_id`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query column metadata: %w", err)
	}
	defer rows.Close()

	return scanColumnMetadataRows(rows)
}

// GetBySchemaColumnIDs retrieves column metadata for multiple schema column IDs.
func (r *columnMetadataRepository) GetBySchemaColumnIDs(ctx context.Context, schemaColumnIDs []uuid.UUID) ([]*models.ColumnMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	if len(schemaColumnIDs) == 0 {
		return []*models.ColumnMetadata{}, nil
	}

	query := `
		SELECT id, project_id, schema_column_id,
		       classification_path, purpose, semantic_type, role, description, confidence,
		       features,
		       needs_enum_analysis, needs_fk_resolution, needs_cross_column_check,
		       needs_clarification, clarification_question,
		       is_sensitive,
		       analyzed_at, llm_model_used,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_ontology_column_metadata
		WHERE schema_column_id = ANY($1)
		ORDER BY schema_column_id`

	rows, err := scope.Conn.Query(ctx, query, schemaColumnIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query column metadata: %w", err)
	}
	defer rows.Close()

	return scanColumnMetadataRows(rows)
}

// Delete removes column metadata by ID.
func (r *columnMetadataRepository) Delete(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_column_metadata WHERE id = $1`
	_, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete column metadata: %w", err)
	}

	return nil
}

// DeleteBySchemaColumnID removes column metadata by schema column ID.
func (r *columnMetadataRepository) DeleteBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_column_metadata WHERE schema_column_id = $1`
	_, err := scope.Conn.Exec(ctx, query, schemaColumnID)
	if err != nil {
		return fmt.Errorf("failed to delete column metadata: %w", err)
	}

	return nil
}

func scanColumnMetadata(row pgx.Row) (*models.ColumnMetadata, error) {
	var m models.ColumnMetadata

	err := row.Scan(
		&m.ID, &m.ProjectID, &m.SchemaColumnID,
		&m.ClassificationPath, &m.Purpose, &m.SemanticType, &m.Role, &m.Description, &m.Confidence,
		&m.Features,
		&m.NeedsEnumAnalysis, &m.NeedsFKResolution, &m.NeedsCrossColumnCheck,
		&m.NeedsClarification, &m.ClarificationQuestion,
		&m.IsSensitive,
		&m.AnalyzedAt, &m.LLMModelUsed,
		&m.Source, &m.LastEditSource, &m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan column metadata: %w", err)
	}

	return &m, nil
}

func scanColumnMetadataRows(rows pgx.Rows) ([]*models.ColumnMetadata, error) {
	var result []*models.ColumnMetadata
	for rows.Next() {
		var m models.ColumnMetadata

		err := rows.Scan(
			&m.ID, &m.ProjectID, &m.SchemaColumnID,
			&m.ClassificationPath, &m.Purpose, &m.SemanticType, &m.Role, &m.Description, &m.Confidence,
			&m.Features,
			&m.NeedsEnumAnalysis, &m.NeedsFKResolution, &m.NeedsCrossColumnCheck,
			&m.NeedsClarification, &m.ClarificationQuestion,
			&m.IsSensitive,
			&m.AnalyzedAt, &m.LLMModelUsed,
			&m.Source, &m.LastEditSource, &m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan column metadata: %w", err)
		}

		result = append(result, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating column metadata: %w", err)
	}

	return result, nil
}
