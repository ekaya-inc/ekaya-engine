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

// TableMetadataRepository provides data access for table metadata.
// Stores semantic annotations in engine_ontology_table_metadata table.
type TableMetadataRepository interface {
	// GetBySchemaTableID retrieves table metadata by schema_table_id.
	GetBySchemaTableID(ctx context.Context, schemaTableID uuid.UUID) (*models.TableMetadata, error)

	// UpsertFromExtraction creates or updates table metadata from extraction pipeline.
	// Always sets source='inferred'.
	UpsertFromExtraction(ctx context.Context, meta *models.TableMetadata) error

	// Upsert creates or updates table metadata for MCP/manual edits.
	// Respects the source parameter and updates last_edit_source.
	Upsert(ctx context.Context, meta *models.TableMetadata) error

	// List retrieves all table metadata for a project.
	List(ctx context.Context, projectID uuid.UUID) ([]*models.TableMetadata, error)

	// ListByTableNames retrieves table metadata for specified table names, returning a map keyed by table_name.
	// Joins with engine_schema_tables to resolve schema_table_id to table names.
	ListByTableNames(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string]*models.TableMetadata, error)

	// Delete removes table metadata by schema_table_id.
	Delete(ctx context.Context, schemaTableID uuid.UUID) error
}

type tableMetadataRepository struct{}

// NewTableMetadataRepository creates a new TableMetadataRepository.
func NewTableMetadataRepository() TableMetadataRepository {
	return &tableMetadataRepository{}
}

var _ TableMetadataRepository = (*tableMetadataRepository)(nil)

func (r *tableMetadataRepository) GetBySchemaTableID(ctx context.Context, schemaTableID uuid.UUID) (*models.TableMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, schema_table_id,
		       table_type, description, usage_notes, is_ephemeral, preferred_alternative, confidence,
		       features, analyzed_at, llm_model_used,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_ontology_table_metadata
		WHERE schema_table_id = $1`

	row := scope.Conn.QueryRow(ctx, query, schemaTableID)
	return scanTableMetadata(row)
}

func (r *tableMetadataRepository) UpsertFromExtraction(ctx context.Context, meta *models.TableMetadata) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	meta.Source = "inferred"

	query := `
		INSERT INTO engine_ontology_table_metadata (
			project_id, schema_table_id,
			table_type, description, usage_notes, is_ephemeral, preferred_alternative, confidence,
			features, analyzed_at, llm_model_used,
			source, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (project_id, schema_table_id)
		DO UPDATE SET
			table_type = COALESCE(EXCLUDED.table_type, engine_ontology_table_metadata.table_type),
			description = COALESCE(EXCLUDED.description, engine_ontology_table_metadata.description),
			usage_notes = COALESCE(EXCLUDED.usage_notes, engine_ontology_table_metadata.usage_notes),
			is_ephemeral = EXCLUDED.is_ephemeral,
			preferred_alternative = COALESCE(EXCLUDED.preferred_alternative, engine_ontology_table_metadata.preferred_alternative),
			confidence = COALESCE(EXCLUDED.confidence, engine_ontology_table_metadata.confidence),
			features = EXCLUDED.features,
			analyzed_at = EXCLUDED.analyzed_at,
			llm_model_used = EXCLUDED.llm_model_used,
			updated_at = $14
		RETURNING id, created_at, updated_at`

	err := scope.Conn.QueryRow(ctx, query,
		meta.ProjectID,
		meta.SchemaTableID,
		meta.TableType,
		meta.Description,
		meta.UsageNotes,
		meta.IsEphemeral,
		meta.PreferredAlternative,
		meta.Confidence,
		meta.Features,
		meta.AnalyzedAt,
		meta.LLMModelUsed,
		meta.Source,
		now,
		now,
	).Scan(&meta.ID, &meta.CreatedAt, &meta.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert table metadata from extraction: %w", err)
	}

	return nil
}

func (r *tableMetadataRepository) Upsert(ctx context.Context, meta *models.TableMetadata) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Get provenance from context
	prov, ok := models.GetProvenance(ctx)
	if !ok {
		return fmt.Errorf("provenance context required")
	}

	now := time.Now()

	// Default source to provenance source if not set
	if meta.Source == "" {
		meta.Source = prov.Source.String()
	}

	query := `
		INSERT INTO engine_ontology_table_metadata (
			project_id, schema_table_id,
			table_type, description, usage_notes, is_ephemeral, preferred_alternative, confidence,
			features, analyzed_at, llm_model_used,
			source, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (project_id, schema_table_id)
		DO UPDATE SET
			table_type = COALESCE(EXCLUDED.table_type, engine_ontology_table_metadata.table_type),
			description = COALESCE(EXCLUDED.description, engine_ontology_table_metadata.description),
			usage_notes = COALESCE(EXCLUDED.usage_notes, engine_ontology_table_metadata.usage_notes),
			is_ephemeral = EXCLUDED.is_ephemeral,
			preferred_alternative = COALESCE(EXCLUDED.preferred_alternative, engine_ontology_table_metadata.preferred_alternative),
			confidence = COALESCE(EXCLUDED.confidence, engine_ontology_table_metadata.confidence),
			features = EXCLUDED.features,
			analyzed_at = EXCLUDED.analyzed_at,
			llm_model_used = EXCLUDED.llm_model_used,
			last_edit_source = $15,
			updated_by = $16,
			updated_at = $17
		RETURNING id, created_at, updated_at`

	err := scope.Conn.QueryRow(ctx, query,
		meta.ProjectID,
		meta.SchemaTableID,
		meta.TableType,
		meta.Description,
		meta.UsageNotes,
		meta.IsEphemeral,
		meta.PreferredAlternative,
		meta.Confidence,
		meta.Features,
		meta.AnalyzedAt,
		meta.LLMModelUsed,
		meta.Source,
		prov.UserID,
		now,
		prov.Source.String(),
		prov.UserID,
		now,
	).Scan(&meta.ID, &meta.CreatedAt, &meta.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert table metadata: %w", err)
	}

	// Set provenance fields on the returned object
	meta.CreatedBy = &prov.UserID
	meta.LastEditSource = nil // Only set on actual update
	meta.UpdatedBy = nil

	return nil
}

func (r *tableMetadataRepository) List(ctx context.Context, projectID uuid.UUID) ([]*models.TableMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, schema_table_id,
		       table_type, description, usage_notes, is_ephemeral, preferred_alternative, confidence,
		       features, analyzed_at, llm_model_used,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_ontology_table_metadata
		WHERE project_id = $1
		ORDER BY created_at`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query table metadata: %w", err)
	}
	defer rows.Close()

	return scanTableMetadataRows(rows)
}

func (r *tableMetadataRepository) ListByTableNames(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string]*models.TableMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	if len(tableNames) == 0 {
		return make(map[string]*models.TableMetadata), nil
	}

	query := `
		SELECT m.id, m.project_id, m.schema_table_id,
		       m.table_type, m.description, m.usage_notes, m.is_ephemeral, m.preferred_alternative, m.confidence,
		       m.features, m.analyzed_at, m.llm_model_used,
		       m.source, m.last_edit_source, m.created_by, m.updated_by, m.created_at, m.updated_at,
		       t.table_name
		FROM engine_ontology_table_metadata m
		JOIN engine_schema_tables t ON m.schema_table_id = t.id
		WHERE m.project_id = $1
		  AND t.table_name = ANY($2)
		  AND t.deleted_at IS NULL
		ORDER BY t.table_name`

	rows, err := scope.Conn.Query(ctx, query, projectID, tableNames)
	if err != nil {
		return nil, fmt.Errorf("failed to query table metadata by names: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*models.TableMetadata)
	for rows.Next() {
		var m models.TableMetadata
		var tableName string

		err := rows.Scan(
			&m.ID, &m.ProjectID, &m.SchemaTableID,
			&m.TableType, &m.Description, &m.UsageNotes, &m.IsEphemeral, &m.PreferredAlternative, &m.Confidence,
			&m.Features, &m.AnalyzedAt, &m.LLMModelUsed,
			&m.Source, &m.LastEditSource, &m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt,
			&tableName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan table metadata: %w", err)
		}

		result[tableName] = &m
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating table metadata: %w", err)
	}

	return result, nil
}

func (r *tableMetadataRepository) Delete(ctx context.Context, schemaTableID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_table_metadata WHERE schema_table_id = $1`
	_, err := scope.Conn.Exec(ctx, query, schemaTableID)
	if err != nil {
		return fmt.Errorf("failed to delete table metadata: %w", err)
	}

	return nil
}

func scanTableMetadata(row pgx.Row) (*models.TableMetadata, error) {
	var m models.TableMetadata

	err := row.Scan(
		&m.ID, &m.ProjectID, &m.SchemaTableID,
		&m.TableType, &m.Description, &m.UsageNotes, &m.IsEphemeral, &m.PreferredAlternative, &m.Confidence,
		&m.Features, &m.AnalyzedAt, &m.LLMModelUsed,
		&m.Source, &m.LastEditSource, &m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan table metadata: %w", err)
	}

	return &m, nil
}

func scanTableMetadataRows(rows pgx.Rows) ([]*models.TableMetadata, error) {
	var result []*models.TableMetadata
	for rows.Next() {
		var m models.TableMetadata

		err := rows.Scan(
			&m.ID, &m.ProjectID, &m.SchemaTableID,
			&m.TableType, &m.Description, &m.UsageNotes, &m.IsEphemeral, &m.PreferredAlternative, &m.Confidence,
			&m.Features, &m.AnalyzedAt, &m.LLMModelUsed,
			&m.Source, &m.LastEditSource, &m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan table metadata: %w", err)
		}

		result = append(result, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating table metadata: %w", err)
	}

	return result, nil
}
