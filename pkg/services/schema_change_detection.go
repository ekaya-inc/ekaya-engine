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
}

type schemaChangeDetectionService struct {
	pendingChangeRepo repositories.PendingChangeRepository
	logger            *zap.Logger
}

// NewSchemaChangeDetectionService creates a new SchemaChangeDetectionService.
func NewSchemaChangeDetectionService(
	pendingChangeRepo repositories.PendingChangeRepository,
	logger *zap.Logger,
) SchemaChangeDetectionService {
	return &schemaChangeDetectionService{
		pendingChangeRepo: pendingChangeRepo,
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
	pendingChanges, err := s.pendingChangeRepo.List(ctx, projectID, models.ChangeStatusPending, 1000)
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
