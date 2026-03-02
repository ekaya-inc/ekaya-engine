package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jinzhu/inflection"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ResolvedChangesResult contains the counts of approved and rejected changes.
type ResolvedChangesResult struct {
	ApprovedCount int
	RejectedCount int
}

// RejectAllResult contains the count of rejected changes.
type RejectAllResult struct {
	RejectedCount int
}

// SchemaChangeDetectionService detects schema changes and creates pending changes for review.
type SchemaChangeDetectionService interface {
	// DetectChanges analyzes a RefreshResult and creates pending changes for review.
	// Returns the pending changes that were created.
	DetectChanges(ctx context.Context, projectID uuid.UUID, refreshResult *models.RefreshResult) ([]*models.PendingChange, error)

	// ListPendingChanges returns pending changes for a project filtered by status.
	ListPendingChanges(ctx context.Context, projectID uuid.UUID, status string, limit int) ([]*models.PendingChange, error)

	// ResolvePendingChanges approves or rejects pending schema changes based on selections.
	// selectedTableNames: table_name -> selected (e.g. "public.users" -> true)
	// selectedColumnNames: "table_name.column_name" -> selected (e.g. "public.users.id" -> true)
	// Auto-applied changes are skipped.
	ResolvePendingChanges(ctx context.Context, projectID uuid.UUID, selectedTableNames map[string]bool, selectedColumnNames map[string]bool) (*ResolvedChangesResult, error)

	// RejectAllPendingChanges rejects all pending changes for a project.
	// Auto-applied changes are skipped.
	RejectAllPendingChanges(ctx context.Context, projectID uuid.UUID) (*RejectAllResult, error)
}

type schemaChangeDetectionService struct {
	pendingChangeRepo repositories.PendingChangeRepository
	schemaRepo        repositories.SchemaRepository
	logger            *zap.Logger
}

// NewSchemaChangeDetectionService creates a new SchemaChangeDetectionService.
func NewSchemaChangeDetectionService(
	pendingChangeRepo repositories.PendingChangeRepository,
	schemaRepo repositories.SchemaRepository,
	logger *zap.Logger,
) SchemaChangeDetectionService {
	return &schemaChangeDetectionService{
		pendingChangeRepo: pendingChangeRepo,
		schemaRepo:        schemaRepo,
		logger:            logger,
	}
}

var _ SchemaChangeDetectionService = (*schemaChangeDetectionService)(nil)

// DetectChanges creates pending change entries from a schema refresh result.
func (s *schemaChangeDetectionService) DetectChanges(
	ctx context.Context,
	projectID uuid.UUID,
	refreshResult *models.RefreshResult,
) ([]*models.PendingChange, error) {
	if refreshResult == nil {
		return nil, nil
	}

	var changes []*models.PendingChange

	// New tables → suggest entity creation
	for _, tableName := range refreshResult.NewTableNames {
		change := &models.PendingChange{
			ProjectID:       projectID,
			ChangeType:      models.ChangeTypeNewTable,
			ChangeSource:    models.ChangeSourceSchemaRefresh,
			TableName:       tableName,
			SuggestedAction: models.SuggestedActionCreateEntity,
			SuggestedPayload: map[string]any{
				"name":          toEntityName(tableName),
				"primary_table": tableName,
			},
			Status: models.ChangeStatusPending,
		}
		changes = append(changes, change)
	}

	// Dropped tables → auto-applied (the table is already gone from the datasource)
	for _, tableName := range refreshResult.RemovedTableNames {
		change := &models.PendingChange{
			ProjectID:       projectID,
			ChangeType:      models.ChangeTypeDroppedTable,
			ChangeSource:    models.ChangeSourceSchemaRefresh,
			TableName:       tableName,
			SuggestedAction: models.SuggestedActionReviewEntity,
			Status:          models.ChangeStatusAutoApplied,
		}
		changes = append(changes, change)
	}

	// New columns → suggest column metadata creation
	for _, col := range refreshResult.NewColumns {
		change := &models.PendingChange{
			ProjectID:       projectID,
			ChangeType:      models.ChangeTypeNewColumn,
			ChangeSource:    models.ChangeSourceSchemaRefresh,
			TableName:       col.TableName,
			ColumnName:      col.ColumnName,
			NewValue:        map[string]any{"type": col.DataType},
			SuggestedAction: models.SuggestedActionCreateColumnMetadata,
			Status:          models.ChangeStatusPending,
		}
		changes = append(changes, change)
	}

	// Dropped columns → auto-applied (the column is already gone from the datasource)
	for _, col := range refreshResult.RemovedColumns {
		change := &models.PendingChange{
			ProjectID:       projectID,
			ChangeType:      models.ChangeTypeDroppedColumn,
			ChangeSource:    models.ChangeSourceSchemaRefresh,
			TableName:       col.TableName,
			ColumnName:      col.ColumnName,
			OldValue:        map[string]any{"type": col.DataType},
			SuggestedAction: models.SuggestedActionReviewColumn,
			Status:          models.ChangeStatusAutoApplied,
		}
		changes = append(changes, change)
	}

	// Modified columns (type changed) → suggest metadata update
	for _, col := range refreshResult.ModifiedColumns {
		change := &models.PendingChange{
			ProjectID:       projectID,
			ChangeType:      models.ChangeTypeModifiedColumn,
			ChangeSource:    models.ChangeSourceSchemaRefresh,
			TableName:       col.TableName,
			ColumnName:      col.ColumnName,
			OldValue:        map[string]any{"type": col.OldType},
			NewValue:        map[string]any{"type": col.NewType},
			SuggestedAction: models.SuggestedActionUpdateColumnMetadata,
			Status:          models.ChangeStatusPending,
		}
		changes = append(changes, change)
	}

	// Persist changes if any
	if len(changes) > 0 {
		if err := s.pendingChangeRepo.CreateBatch(ctx, changes); err != nil {
			return nil, fmt.Errorf("persist pending changes: %w", err)
		}

		s.logger.Info("Created pending changes from schema refresh",
			zap.String("project_id", projectID.String()),
			zap.Int("new_tables", len(refreshResult.NewTableNames)),
			zap.Int("dropped_tables", len(refreshResult.RemovedTableNames)),
			zap.Int("new_columns", len(refreshResult.NewColumns)),
			zap.Int("dropped_columns", len(refreshResult.RemovedColumns)),
			zap.Int("modified_columns", len(refreshResult.ModifiedColumns)),
			zap.Int("total_changes", len(changes)),
		)
	}

	return changes, nil
}

// ListPendingChanges returns pending changes for a project filtered by status.
func (s *schemaChangeDetectionService) ListPendingChanges(ctx context.Context, projectID uuid.UUID, status string, limit int) ([]*models.PendingChange, error) {
	return s.pendingChangeRepo.List(ctx, projectID, status, limit)
}

// ResolvePendingChanges approves or rejects pending schema changes based on name-based selections.
func (s *schemaChangeDetectionService) ResolvePendingChanges(
	ctx context.Context,
	projectID uuid.UUID,
	selectedTableNames map[string]bool,
	selectedColumnNames map[string]bool,
) (*ResolvedChangesResult, error) {
	pendingChanges, err := s.pendingChangeRepo.List(ctx, projectID, models.ChangeStatusPending, 0)
	if err != nil {
		return nil, fmt.Errorf("list pending changes: %w", err)
	}

	if len(pendingChanges) == 0 {
		return &ResolvedChangesResult{}, nil
	}

	result := &ResolvedChangesResult{}

	for _, change := range pendingChanges {
		var shouldApprove bool

		switch change.ChangeType {
		case models.ChangeTypeNewTable:
			shouldApprove = selectedTableNames[change.TableName]
		case models.ChangeTypeNewColumn, models.ChangeTypeModifiedColumn:
			key := change.TableName + "." + change.ColumnName
			shouldApprove = selectedColumnNames[key]
		default:
			// Skip other change types (dropped_table, dropped_column are auto_applied)
			continue
		}

		reviewedBy := "schema_selection"
		if shouldApprove {
			if err := s.pendingChangeRepo.UpdateStatus(ctx, change.ID, models.ChangeStatusApproved, reviewedBy); err != nil {
				s.logger.Warn("Failed to approve pending change",
					zap.String("change_id", change.ID.String()),
					zap.Error(err))
				continue
			}
			result.ApprovedCount++
		} else {
			if err := s.pendingChangeRepo.UpdateStatus(ctx, change.ID, models.ChangeStatusRejected, reviewedBy); err != nil {
				s.logger.Warn("Failed to reject pending change",
					zap.String("change_id", change.ID.String()),
					zap.Error(err))
				continue
			}
			result.RejectedCount++
		}
	}

	if result.ApprovedCount > 0 || result.RejectedCount > 0 {
		s.logger.Info("Resolved pending changes from schema selection",
			zap.String("project_id", projectID.String()),
			zap.Int("approved", result.ApprovedCount),
			zap.Int("rejected", result.RejectedCount),
		)
	}

	return result, nil
}

// RejectAllPendingChanges rejects all pending changes for a project.
// For new_table and new_column changes, this also deselects the corresponding
// table or column so that rejection means "do not include in schema."
func (s *schemaChangeDetectionService) RejectAllPendingChanges(
	ctx context.Context,
	projectID uuid.UUID,
) (*RejectAllResult, error) {
	pendingChanges, err := s.pendingChangeRepo.List(ctx, projectID, models.ChangeStatusPending, 0)
	if err != nil {
		return nil, fmt.Errorf("list pending changes: %w", err)
	}

	if len(pendingChanges) == 0 {
		return &RejectAllResult{}, nil
	}

	// Deselect auto-selected tables and columns before marking as rejected
	if s.schemaRepo != nil {
		s.deselectRejectedChanges(ctx, projectID, pendingChanges)
	}

	result := &RejectAllResult{}
	reviewedBy := "schema_selection_reject_all"

	for _, change := range pendingChanges {
		if err := s.pendingChangeRepo.UpdateStatus(ctx, change.ID, models.ChangeStatusRejected, reviewedBy); err != nil {
			s.logger.Warn("Failed to reject pending change",
				zap.String("change_id", change.ID.String()),
				zap.Error(err))
			continue
		}
		result.RejectedCount++
	}

	if result.RejectedCount > 0 {
		s.logger.Info("Rejected all pending changes",
			zap.String("project_id", projectID.String()),
			zap.Int("rejected", result.RejectedCount),
		)
	}

	return result, nil
}

// deselectRejectedChanges deselects tables and columns that were auto-selected
// during schema refresh. Only applies to new_table and new_column change types.
func (s *schemaChangeDetectionService) deselectRejectedChanges(
	ctx context.Context,
	projectID uuid.UUID,
	changes []*models.PendingChange,
) {
	// Collect unique table names referenced by pending changes.
	// Pending changes store qualified names ("public.orders") but the DB
	// stores schema_name and table_name separately, so strip the prefix.
	tableNames := make(map[string]bool)
	for _, change := range changes {
		if change.TableName != "" {
			tableNames[stripSchemaPrefix(change.TableName)] = true
		}
	}

	// Look up tables by bare name to get their IDs
	nameList := make([]string, 0, len(tableNames))
	for name := range tableNames {
		nameList = append(nameList, name)
	}
	tables, err := s.schemaRepo.GetTablesByNames(ctx, projectID, nameList)
	if err != nil {
		s.logger.Warn("Failed to look up tables for deselection", zap.Error(err))
		return
	}

	for _, change := range changes {
		bareName := stripSchemaPrefix(change.TableName)

		switch change.ChangeType {
		case models.ChangeTypeNewTable:
			table, ok := tables[bareName]
			if !ok {
				continue
			}
			if err := s.schemaRepo.UpdateTableSelection(ctx, projectID, table.ID, false); err != nil {
				s.logger.Warn("Failed to deselect rejected table",
					zap.String("table", change.TableName),
					zap.Error(err))
			}

		case models.ChangeTypeNewColumn:
			table, ok := tables[bareName]
			if !ok {
				continue
			}
			col, err := s.schemaRepo.GetColumnByName(ctx, table.ID, change.ColumnName)
			if err != nil || col == nil {
				s.logger.Warn("Failed to look up column for deselection",
					zap.String("table", change.TableName),
					zap.String("column", change.ColumnName),
					zap.Error(err))
				continue
			}
			if err := s.schemaRepo.UpdateColumnSelection(ctx, projectID, col.ID, false); err != nil {
				s.logger.Warn("Failed to deselect rejected column",
					zap.String("table", change.TableName),
					zap.String("column", change.ColumnName),
					zap.Error(err))
			}
		}
	}
}

// stripSchemaPrefix removes the "schema." prefix from a qualified table name.
// e.g. "public.orders" -> "orders", "orders" -> "orders"
func stripSchemaPrefix(tableName string) string {
	if idx := strings.LastIndex(tableName, "."); idx >= 0 {
		return tableName[idx+1:]
	}
	return tableName
}

// toEntityName converts a table name to an entity name.
// Examples: "public.users" -> "User", "orders" -> "Order", "categories" -> "Category"
func toEntityName(tableName string) string {
	// Strip schema prefix if present
	name := tableName
	if idx := strings.LastIndex(tableName, "."); idx >= 0 {
		name = tableName[idx+1:]
	}

	// Singularize using proper English rules
	name = inflection.Singular(name)

	// Capitalize first letter
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}

	return name
}
