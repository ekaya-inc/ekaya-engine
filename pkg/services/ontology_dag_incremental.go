package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ComputeChangeSet calculates what has changed in the schema since the last successful extraction.
// It queries schema tables/columns by timestamp and column/table metadata for user-edited exclusions.
func (s *ontologyDAGService) ComputeChangeSet(ctx context.Context, projectID uuid.UUID, builtAt time.Time) (*models.ChangeSet, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	cs := &models.ChangeSet{
		BuiltAt:          builtAt,
		AffectedTableIDs: make(map[uuid.UUID]bool),
		UserEditedIDs:    make(map[uuid.UUID]bool),
	}

	// Query added tables (created after last build, not deleted)
	if err := s.queryAddedTables(ctx, scope, cs, projectID, builtAt); err != nil {
		return nil, err
	}

	// Query modified tables (updated after last build, but created before it — not new)
	if err := s.queryModifiedTables(ctx, scope, cs, projectID, builtAt); err != nil {
		return nil, err
	}

	// Query deleted tables (deleted after last build)
	if err := s.queryDeletedTables(ctx, scope, cs, projectID, builtAt); err != nil {
		return nil, err
	}

	// Query deselected tables (is_selected changed to false after last build)
	if err := s.queryDeselectedTables(ctx, scope, cs, projectID, builtAt); err != nil {
		return nil, err
	}

	// Query added columns
	if err := s.queryAddedColumns(ctx, scope, cs, projectID, builtAt); err != nil {
		return nil, err
	}

	// Query modified columns
	if err := s.queryModifiedColumns(ctx, scope, cs, projectID, builtAt); err != nil {
		return nil, err
	}

	// Query deleted columns
	if err := s.queryDeletedColumns(ctx, scope, cs, projectID, builtAt); err != nil {
		return nil, err
	}

	// Query deselected columns (is_selected changed to false after last build)
	if err := s.queryDeselectedColumns(ctx, scope, cs, projectID, builtAt); err != nil {
		return nil, err
	}

	// Query user-edited metadata IDs to skip during re-extraction
	if err := s.queryUserEditedIDs(ctx, scope, cs, projectID); err != nil {
		return nil, err
	}

	s.logger.Info("Computed ChangeSet",
		zap.String("project_id", projectID.String()),
		zap.Int("added_tables", len(cs.AddedTables)),
		zap.Int("modified_tables", len(cs.ModifiedTables)),
		zap.Int("deleted_tables", len(cs.DeletedTables)),
		zap.Int("added_columns", len(cs.AddedColumns)),
		zap.Int("modified_columns", len(cs.ModifiedColumns)),
		zap.Int("deleted_columns", len(cs.DeletedColumns)),
		zap.Int("user_edited_ids", len(cs.UserEditedIDs)),
		zap.Int("affected_table_ids", len(cs.AffectedTableIDs)))

	return cs, nil
}

// CleanupDeletedItems removes ontology artifacts for deleted tables/columns before DAG nodes run.
func (s *ontologyDAGService) CleanupDeletedItems(ctx context.Context, projectID uuid.UUID, changeSet *models.ChangeSet) error {
	if changeSet == nil || !changeSet.HasDeletedItems() {
		return nil
	}

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// 1. Delete column metadata for deleted columns
	for _, col := range changeSet.DeletedColumns {
		_, err := scope.Conn.Exec(ctx,
			`DELETE FROM engine_ontology_column_metadata WHERE schema_column_id = $1 AND project_id = $2`,
			col.ID, projectID)
		if err != nil {
			s.logger.Warn("Failed to delete column metadata for deleted column",
				zap.String("column_id", col.ID.String()),
				zap.Error(err))
		}
	}

	// 2. Delete table metadata for deleted tables (and their columns' metadata)
	for _, tbl := range changeSet.DeletedTables {
		_, err := scope.Conn.Exec(ctx,
			`DELETE FROM engine_ontology_table_metadata WHERE schema_table_id = $1 AND project_id = $2`,
			tbl.ID, projectID)
		if err != nil {
			s.logger.Warn("Failed to delete table metadata for deleted table",
				zap.String("table_id", tbl.ID.String()),
				zap.Error(err))
		}

		// Delete column metadata for all columns of deleted tables
		_, err = scope.Conn.Exec(ctx,
			`DELETE FROM engine_ontology_column_metadata
			 WHERE project_id = $1 AND schema_column_id IN (
				SELECT id FROM engine_schema_columns WHERE schema_table_id = $2
			 )`, projectID, tbl.ID)
		if err != nil {
			s.logger.Warn("Failed to delete column metadata for deleted table's columns",
				zap.String("table_id", tbl.ID.String()),
				zap.Error(err))
		}
	}

	// 3. Soft-delete relationships where source or target column was deleted
	deletedColumnIDs := s.collectDeletedColumnIDs(ctx, scope, changeSet)

	if len(deletedColumnIDs) > 0 {
		result, err := scope.Conn.Exec(ctx,
			`UPDATE engine_schema_relationships
			 SET deleted_at = NOW()
			 WHERE project_id = $1 AND deleted_at IS NULL
			   AND (source_column_id = ANY($2) OR target_column_id = ANY($2))`,
			projectID, deletedColumnIDs)
		if err != nil {
			s.logger.Warn("Failed to soft-delete relationships for deleted columns", zap.Error(err))
		} else {
			s.logger.Info("Soft-deleted relationships for deleted columns",
				zap.Int64("count", result.RowsAffected()))
		}
	}

	// 4. Delete questions whose affects reference only deleted tables/columns
	s.cleanupOrphanedQuestions(ctx, projectID, changeSet)

	s.logger.Info("Cleanup for deleted items complete",
		zap.String("project_id", projectID.String()),
		zap.Int("deleted_tables", len(changeSet.DeletedTables)),
		zap.Int("deleted_columns", len(changeSet.DeletedColumns)))

	return nil
}

// GetLastCompletedDAG returns the most recently completed DAG for a datasource.
func (s *ontologyDAGService) GetLastCompletedDAG(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id,
		       status, current_node, schema_fingerprint,
		       owner_id, last_heartbeat,
		       is_incremental, change_summary,
		       started_at, completed_at, created_at, updated_at
		FROM engine_ontology_dag
		WHERE datasource_id = $1 AND status = 'completed'
		ORDER BY completed_at DESC
		LIMIT 1`

	var dag models.OntologyDAG
	var changeSummaryJSON []byte
	err := scope.Conn.QueryRow(ctx, query, datasourceID).Scan(
		&dag.ID, &dag.ProjectID, &dag.DatasourceID,
		&dag.Status, &dag.CurrentNode, &dag.SchemaFingerprint,
		&dag.OwnerID, &dag.LastHeartbeat,
		&dag.IsIncremental, &changeSummaryJSON,
		&dag.StartedAt, &dag.CompletedAt, &dag.CreatedAt, &dag.UpdatedAt,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("query last completed DAG: %w", err)
	}

	if len(changeSummaryJSON) > 0 {
		summary := &models.ChangeSummary{}
		if err := json.Unmarshal(changeSummaryJSON, summary); err != nil {
			s.logger.Warn("Failed to unmarshal change summary", zap.Error(err))
		} else {
			dag.ChangeSummary = summary
		}
	}

	return &dag, nil
}

// GetOntologyStatus returns the current ontology status with change detection.
func (s *ontologyDAGService) GetOntologyStatus(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyStatusResponse, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Get the latest completed DAG
	lastDAG, err := s.GetLastCompletedDAG(ctx, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get last completed DAG: %w", err)
	}

	completionState, err := loadOntologyCompletionState(ctx, scope.Conn, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("load ontology completion state: %w", err)
	}

	resp := &models.OntologyStatusResponse{
		HasOntology: completionState.Provenance.IsValid() || lastDAG != nil,
	}

	switch {
	case completionState.Provenance.IsValid():
		resp.CompletionProvenance = completionState.Provenance
	case lastDAG != nil:
		resp.CompletionProvenance = models.OntologyCompletionProvenanceExtracted
	}

	switch {
	case completionState.CompletedAt != nil:
		resp.LastBuiltAt = completionState.CompletedAt
	case lastDAG != nil && lastDAG.CompletedAt != nil:
		resp.LastBuiltAt = lastDAG.CompletedAt
	}

	if resp.LastBuiltAt == nil {
		return resp, nil
	}

	// Compute what has changed since the last build
	changeSet, err := s.ComputeChangeSet(ctx, projectID, *resp.LastBuiltAt)
	if err != nil {
		return nil, fmt.Errorf("compute change set: %w", err)
	}

	resp.SchemaChangedSinceBuild = !changeSet.IsEmpty()
	resp.ChangeSummary = changeSet.ToSummary()

	return resp, nil
}

// ============================================================================
// Private helpers for ChangeSet computation
// ============================================================================

func (s *ontologyDAGService) queryAddedTables(ctx context.Context, scope *database.TenantScope, cs *models.ChangeSet, projectID uuid.UUID, builtAt time.Time) error {
	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name, is_selected, row_count, created_at, updated_at
		FROM engine_schema_tables
		WHERE project_id = $1 AND created_at > $2 AND deleted_at IS NULL AND is_selected = true`

	rows, err := scope.Conn.Query(ctx, query, projectID, builtAt)
	if err != nil {
		return fmt.Errorf("query added tables: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t models.SchemaTable
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.DatasourceID, &t.SchemaName, &t.TableName,
			&t.IsSelected, &t.RowCount, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return fmt.Errorf("scan added table: %w", err)
		}
		cs.AddedTables = append(cs.AddedTables, t)
		cs.AffectedTableIDs[t.ID] = true
	}
	return rows.Err()
}

func (s *ontologyDAGService) queryModifiedTables(ctx context.Context, scope *database.TenantScope, cs *models.ChangeSet, projectID uuid.UUID, builtAt time.Time) error {
	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name, is_selected, row_count, created_at, updated_at
		FROM engine_schema_tables
		WHERE project_id = $1 AND updated_at > $2 AND created_at <= $2 AND deleted_at IS NULL AND is_selected = true`

	rows, err := scope.Conn.Query(ctx, query, projectID, builtAt)
	if err != nil {
		return fmt.Errorf("query modified tables: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t models.SchemaTable
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.DatasourceID, &t.SchemaName, &t.TableName,
			&t.IsSelected, &t.RowCount, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return fmt.Errorf("scan modified table: %w", err)
		}
		cs.ModifiedTables = append(cs.ModifiedTables, t)
		cs.AffectedTableIDs[t.ID] = true
	}
	return rows.Err()
}

func (s *ontologyDAGService) queryDeletedTables(ctx context.Context, scope *database.TenantScope, cs *models.ChangeSet, projectID uuid.UUID, builtAt time.Time) error {
	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name, is_selected, row_count, created_at, updated_at
		FROM engine_schema_tables
		WHERE project_id = $1 AND deleted_at IS NOT NULL AND deleted_at > $2`

	rows, err := scope.Conn.Query(ctx, query, projectID, builtAt)
	if err != nil {
		return fmt.Errorf("query deleted tables: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t models.SchemaTable
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.DatasourceID, &t.SchemaName, &t.TableName,
			&t.IsSelected, &t.RowCount, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return fmt.Errorf("scan deleted table: %w", err)
		}
		cs.DeletedTables = append(cs.DeletedTables, t)
	}
	return rows.Err()
}

func (s *ontologyDAGService) queryDeselectedTables(ctx context.Context, scope *database.TenantScope, cs *models.ChangeSet, projectID uuid.UUID, builtAt time.Time) error {
	// Tables that were previously selected (part of the ontology) but have been deselected.
	// From the ontology's perspective, deselection = deletion.
	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name, is_selected, row_count, created_at, updated_at
		FROM engine_schema_tables
		WHERE project_id = $1 AND updated_at > $2 AND created_at <= $2 AND deleted_at IS NULL AND is_selected = false`

	rows, err := scope.Conn.Query(ctx, query, projectID, builtAt)
	if err != nil {
		return fmt.Errorf("query deselected tables: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t models.SchemaTable
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.DatasourceID, &t.SchemaName, &t.TableName,
			&t.IsSelected, &t.RowCount, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return fmt.Errorf("scan deselected table: %w", err)
		}
		cs.DeletedTables = append(cs.DeletedTables, t)
	}
	return rows.Err()
}

func (s *ontologyDAGService) queryAddedColumns(ctx context.Context, scope *database.TenantScope, cs *models.ChangeSet, projectID uuid.UUID, builtAt time.Time) error {
	query := `
		SELECT c.id, c.project_id, c.schema_table_id, c.column_name, c.data_type,
		       c.is_nullable, c.is_primary_key, c.is_unique, c.is_selected, c.ordinal_position,
		       c.default_value, c.distinct_count, c.null_count, c.min_length, c.max_length,
		       c.created_at, c.updated_at
		FROM engine_schema_columns c
		JOIN engine_schema_tables t ON c.schema_table_id = t.id
		WHERE c.project_id = $1 AND c.created_at > $2 AND c.deleted_at IS NULL AND c.is_selected = true
		  AND t.deleted_at IS NULL AND t.is_selected = true`

	rows, err := scope.Conn.Query(ctx, query, projectID, builtAt)
	if err != nil {
		return fmt.Errorf("query added columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		col, err := scanChangeSetColumn(rows)
		if err != nil {
			return fmt.Errorf("scan added column: %w", err)
		}
		cs.AddedColumns = append(cs.AddedColumns, *col)
		cs.AffectedTableIDs[col.SchemaTableID] = true
	}
	return rows.Err()
}

func (s *ontologyDAGService) queryModifiedColumns(ctx context.Context, scope *database.TenantScope, cs *models.ChangeSet, projectID uuid.UUID, builtAt time.Time) error {
	query := `
		SELECT c.id, c.project_id, c.schema_table_id, c.column_name, c.data_type,
		       c.is_nullable, c.is_primary_key, c.is_unique, c.is_selected, c.ordinal_position,
		       c.default_value, c.distinct_count, c.null_count, c.min_length, c.max_length,
		       c.created_at, c.updated_at
		FROM engine_schema_columns c
		JOIN engine_schema_tables t ON c.schema_table_id = t.id
		WHERE c.project_id = $1 AND c.updated_at > $2 AND c.created_at <= $2 AND c.deleted_at IS NULL AND c.is_selected = true
		  AND t.deleted_at IS NULL AND t.is_selected = true`

	rows, err := scope.Conn.Query(ctx, query, projectID, builtAt)
	if err != nil {
		return fmt.Errorf("query modified columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		col, err := scanChangeSetColumn(rows)
		if err != nil {
			return fmt.Errorf("scan modified column: %w", err)
		}
		cs.ModifiedColumns = append(cs.ModifiedColumns, *col)
		cs.AffectedTableIDs[col.SchemaTableID] = true
	}
	return rows.Err()
}

func (s *ontologyDAGService) queryDeletedColumns(ctx context.Context, scope *database.TenantScope, cs *models.ChangeSet, projectID uuid.UUID, builtAt time.Time) error {
	query := `
		SELECT c.id, c.project_id, c.schema_table_id, c.column_name, c.data_type,
		       c.is_nullable, c.is_primary_key, c.is_unique, c.is_selected, c.ordinal_position,
		       c.default_value, c.distinct_count, c.null_count, c.min_length, c.max_length,
		       c.created_at, c.updated_at
		FROM engine_schema_columns c
		WHERE c.project_id = $1 AND c.deleted_at IS NOT NULL AND c.deleted_at > $2`

	rows, err := scope.Conn.Query(ctx, query, projectID, builtAt)
	if err != nil {
		return fmt.Errorf("query deleted columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		col, err := scanChangeSetColumn(rows)
		if err != nil {
			return fmt.Errorf("scan deleted column: %w", err)
		}
		cs.DeletedColumns = append(cs.DeletedColumns, *col)
	}
	return rows.Err()
}

func (s *ontologyDAGService) queryDeselectedColumns(ctx context.Context, scope *database.TenantScope, cs *models.ChangeSet, projectID uuid.UUID, builtAt time.Time) error {
	// Columns that were previously selected but have been deselected by the user.
	// From the ontology's perspective, deselection = deletion.
	// Also catches columns on deselected tables (t.is_selected = false).
	query := `
		SELECT c.id, c.project_id, c.schema_table_id, c.column_name, c.data_type,
		       c.is_nullable, c.is_primary_key, c.is_unique, c.is_selected, c.ordinal_position,
		       c.default_value, c.distinct_count, c.null_count, c.min_length, c.max_length,
		       c.created_at, c.updated_at
		FROM engine_schema_columns c
		WHERE c.project_id = $1 AND c.updated_at > $2 AND c.created_at <= $2 AND c.deleted_at IS NULL AND c.is_selected = false`

	rows, err := scope.Conn.Query(ctx, query, projectID, builtAt)
	if err != nil {
		return fmt.Errorf("query deselected columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		col, err := scanChangeSetColumn(rows)
		if err != nil {
			return fmt.Errorf("scan deselected column: %w", err)
		}
		cs.DeletedColumns = append(cs.DeletedColumns, *col)
	}
	return rows.Err()
}

func (s *ontologyDAGService) queryUserEditedIDs(ctx context.Context, scope *database.TenantScope, cs *models.ChangeSet, projectID uuid.UUID) error {
	// Query user-edited column metadata IDs
	colQuery := `SELECT schema_column_id FROM engine_ontology_column_metadata
		WHERE project_id = $1 AND last_edit_source IS NOT NULL`

	rows, err := scope.Conn.Query(ctx, colQuery, projectID)
	if err != nil {
		return fmt.Errorf("query user-edited columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan user-edited column: %w", err)
		}
		cs.UserEditedIDs[id] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate user-edited columns: %w", err)
	}

	// Query user-edited table metadata IDs
	tblQuery := `SELECT schema_table_id FROM engine_ontology_table_metadata
		WHERE project_id = $1 AND last_edit_source IS NOT NULL`

	rows, err = scope.Conn.Query(ctx, tblQuery, projectID)
	if err != nil {
		return fmt.Errorf("query user-edited tables: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan user-edited table: %w", err)
		}
		cs.UserEditedIDs[id] = true
	}
	return rows.Err()
}

func (s *ontologyDAGService) collectDeletedColumnIDs(ctx context.Context, scope *database.TenantScope, changeSet *models.ChangeSet) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(changeSet.DeletedColumns))
	for _, col := range changeSet.DeletedColumns {
		ids = append(ids, col.ID)
	}

	// Also include columns from deleted tables
	for _, tbl := range changeSet.DeletedTables {
		rows, err := scope.Conn.Query(ctx,
			`SELECT id FROM engine_schema_columns WHERE schema_table_id = $1`, tbl.ID)
		if err != nil {
			s.logger.Warn("Failed to query columns for deleted table",
				zap.String("table_id", tbl.ID.String()),
				zap.Error(err))
			continue
		}
		for rows.Next() {
			var colID uuid.UUID
			if err := rows.Scan(&colID); err != nil {
				s.logger.Warn("Failed to scan column ID", zap.Error(err))
				continue
			}
			ids = append(ids, colID)
		}
		rows.Close()
	}

	return ids
}

func (s *ontologyDAGService) cleanupOrphanedQuestions(ctx context.Context, projectID uuid.UUID, changeSet *models.ChangeSet) {
	deletedTableNames := make(map[string]bool)
	for _, tbl := range changeSet.DeletedTables {
		deletedTableNames[tbl.TableName] = true
	}

	if len(deletedTableNames) == 0 {
		return
	}

	questions, err := s.questionRepo.ListPending(ctx, projectID)
	if err != nil {
		s.logger.Warn("Failed to list questions for cleanup", zap.Error(err))
		return
	}

	for _, q := range questions {
		if q.Affects == nil || len(q.Affects.Tables) == 0 {
			continue
		}
		// Check if ALL affected tables are deleted
		allDeleted := true
		for _, tableName := range q.Affects.Tables {
			if !deletedTableNames[tableName] {
				allDeleted = false
				break
			}
		}
		if allDeleted {
			if err := s.questionRepo.UpdateStatus(ctx, q.ID, models.QuestionStatusDeleted); err != nil {
				s.logger.Warn("Failed to delete orphaned question",
					zap.String("question_id", q.ID.String()),
					zap.Error(err))
			}
		}
	}
}

// scanChangeSetColumn scans a row into a SchemaColumn for ChangeSet queries.
type changeSetColumnScanner interface {
	Scan(dest ...any) error
}

func scanChangeSetColumn(row changeSetColumnScanner) (*models.SchemaColumn, error) {
	var col models.SchemaColumn
	err := row.Scan(
		&col.ID, &col.ProjectID, &col.SchemaTableID, &col.ColumnName, &col.DataType,
		&col.IsNullable, &col.IsPrimaryKey, &col.IsUnique, &col.IsSelected, &col.OrdinalPosition,
		&col.DefaultValue, &col.DistinctCount, &col.NullCount, &col.MinLength, &col.MaxLength,
		&col.CreatedAt, &col.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &col, nil
}
