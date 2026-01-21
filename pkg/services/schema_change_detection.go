package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// SchemaChangeDetectionService detects schema changes and creates pending changes for review.
type SchemaChangeDetectionService interface {
	// DetectChanges analyzes a RefreshResult and creates pending changes for review.
	// Returns the pending changes that were created.
	DetectChanges(ctx context.Context, projectID uuid.UUID, refreshResult *models.RefreshResult) ([]*models.PendingChange, error)
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

	// Dropped tables → flag for review (don't auto-delete entities)
	for _, tableName := range refreshResult.RemovedTableNames {
		change := &models.PendingChange{
			ProjectID:       projectID,
			ChangeType:      models.ChangeTypeDroppedTable,
			ChangeSource:    models.ChangeSourceSchemaRefresh,
			TableName:       tableName,
			SuggestedAction: models.SuggestedActionReviewEntity,
			Status:          models.ChangeStatusPending,
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

	// Dropped columns → flag for review
	for _, col := range refreshResult.RemovedColumns {
		change := &models.PendingChange{
			ProjectID:       projectID,
			ChangeType:      models.ChangeTypeDroppedColumn,
			ChangeSource:    models.ChangeSourceSchemaRefresh,
			TableName:       col.TableName,
			ColumnName:      col.ColumnName,
			OldValue:        map[string]any{"type": col.DataType},
			SuggestedAction: models.SuggestedActionReviewColumn,
			Status:          models.ChangeStatusPending,
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

// toEntityName converts a table name to an entity name.
// Examples: "public.users" -> "User", "orders" -> "Order"
func toEntityName(tableName string) string {
	// Strip schema prefix if present
	name := tableName
	if idx := strings.LastIndex(tableName, "."); idx >= 0 {
		name = tableName[idx+1:]
	}

	// Convert to singular PascalCase
	// Simple heuristic: remove trailing 's' and capitalize first letter
	name = strings.TrimSuffix(name, "s")
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}

	return name
}
