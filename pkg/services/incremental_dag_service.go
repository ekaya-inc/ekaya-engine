package services

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// IncrementalDAGService handles targeted LLM enrichment for approved ontology changes.
// Unlike the full DAG which re-extracts everything, this service only enriches what changed.
type IncrementalDAGService interface {
	// ProcessChange processes a single approved change with LLM enrichment.
	// Returns nil if enrichment is skipped (no AI Config, precedence blocked, etc.)
	ProcessChange(ctx context.Context, change *models.PendingChange) error

	// ProcessChanges processes a batch of changes, grouping by type for efficiency.
	// Entities are processed before relationships since relationships depend on entities.
	ProcessChanges(ctx context.Context, changes []*models.PendingChange) error

	// ProcessChangeAsync processes a change asynchronously in a background goroutine.
	// Errors are logged but not returned.
	ProcessChangeAsync(ctx context.Context, change *models.PendingChange)

	// ProcessChangesAsync processes a batch of changes asynchronously in a background goroutine.
	// Uses a fresh background context to avoid issues with canceled request contexts.
	// Errors are logged but not returned.
	ProcessChangesAsync(projectID uuid.UUID, changes []*models.PendingChange)

	// SetChangeReviewService injects the ChangeReviewService for precedence checking.
	// This breaks the circular dependency: IncrementalDAGService needs ChangeReviewService
	// for precedence checking, while ChangeReviewService needs IncrementalDAGService for
	// triggering enrichment after approval.
	SetChangeReviewService(svc ChangeReviewService)
}

type incrementalDAGService struct {
	ontologyRepo       repositories.OntologyRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	schemaRepo         repositories.SchemaRepository
	conversationRepo   repositories.ConversationRepository
	aiConfigSvc        AIConfigService
	llmFactory         llm.LLMClientFactory
	changeReviewSvc    ChangeReviewService
	getTenantCtx       TenantContextFunc
	logger             *zap.Logger
}

// IncrementalDAGServiceDeps contains dependencies for IncrementalDAGService.
type IncrementalDAGServiceDeps struct {
	OntologyRepo       repositories.OntologyRepository
	ColumnMetadataRepo repositories.ColumnMetadataRepository
	SchemaRepo         repositories.SchemaRepository
	ConversationRepo   repositories.ConversationRepository
	AIConfigSvc        AIConfigService
	LLMFactory         llm.LLMClientFactory
	ChangeReviewSvc    ChangeReviewService
	GetTenantCtx       TenantContextFunc
	Logger             *zap.Logger
}

// NewIncrementalDAGService creates a new IncrementalDAGService.
func NewIncrementalDAGService(deps *IncrementalDAGServiceDeps) IncrementalDAGService {
	return &incrementalDAGService{
		ontologyRepo:       deps.OntologyRepo,
		columnMetadataRepo: deps.ColumnMetadataRepo,
		schemaRepo:         deps.SchemaRepo,
		conversationRepo:   deps.ConversationRepo,
		aiConfigSvc:        deps.AIConfigSvc,
		llmFactory:         deps.LLMFactory,
		changeReviewSvc:    deps.ChangeReviewSvc,
		getTenantCtx:       deps.GetTenantCtx,
		logger:             deps.Logger.Named("incremental-dag"),
	}
}

var _ IncrementalDAGService = (*incrementalDAGService)(nil)

// SetChangeReviewService injects the ChangeReviewService for precedence checking.
func (s *incrementalDAGService) SetChangeReviewService(svc ChangeReviewService) {
	s.changeReviewSvc = svc
}

// ProcessChangeAsync processes a change asynchronously.
func (s *incrementalDAGService) ProcessChangeAsync(ctx context.Context, change *models.PendingChange) {
	go func() {
		// Use background context since the original may be cancelled
		bgCtx := context.Background()

		// Get tenant context for the background operation
		tenantCtx, cleanup, err := s.getTenantCtx(bgCtx, change.ProjectID)
		if err != nil {
			s.logger.Error("Failed to get tenant context for async processing",
				zap.String("change_id", change.ID.String()),
				zap.Error(err))
			return
		}
		defer cleanup()

		if err := s.ProcessChange(tenantCtx, change); err != nil {
			s.logger.Error("Async change processing failed",
				zap.String("change_id", change.ID.String()),
				zap.String("change_type", change.ChangeType),
				zap.Error(err))
		}
	}()
}

// ProcessChangesAsync processes a batch of changes asynchronously.
// Uses a fresh background context to avoid issues with canceled request contexts.
func (s *incrementalDAGService) ProcessChangesAsync(projectID uuid.UUID, changes []*models.PendingChange) {
	if len(changes) == 0 {
		return
	}

	go func() {
		// Use background context since the original request context may be cancelled
		// by the time this goroutine runs (HTTP response already sent)
		bgCtx := context.Background()

		// Get tenant context for the background operation
		tenantCtx, cleanup, err := s.getTenantCtx(bgCtx, projectID)
		if err != nil {
			s.logger.Error("Failed to get tenant context for async batch processing",
				zap.String("project_id", projectID.String()),
				zap.Int("change_count", len(changes)),
				zap.Error(err))
			return
		}
		defer cleanup()

		if err := s.ProcessChanges(tenantCtx, changes); err != nil {
			s.logger.Error("Async batch change processing failed",
				zap.String("project_id", projectID.String()),
				zap.Int("change_count", len(changes)),
				zap.Error(err))
		}
	}()
}

// ProcessChange processes a single approved change with targeted LLM enrichment.
func (s *incrementalDAGService) ProcessChange(ctx context.Context, change *models.PendingChange) error {
	// Check if AI Config is attached - if not, skip enrichment
	aiConfig, err := s.aiConfigSvc.Get(ctx, change.ProjectID)
	if err != nil {
		s.logger.Debug("No AI config, skipping enrichment",
			zap.String("change_id", change.ID.String()),
			zap.Error(err))
		return nil
	}
	if aiConfig == nil || aiConfig.ConfigType == models.AIConfigNone {
		s.logger.Debug("AI config not configured, skipping enrichment",
			zap.String("change_id", change.ID.String()))
		return nil
	}

	s.logger.Info("Processing change for incremental enrichment",
		zap.String("change_id", change.ID.String()),
		zap.String("change_type", change.ChangeType),
		zap.String("table_name", change.TableName))

	switch change.ChangeType {
	case models.ChangeTypeNewTable:
		return s.processNewTable(ctx, change)
	case models.ChangeTypeNewColumn:
		return s.processNewColumn(ctx, change)
	case models.ChangeTypeNewFKPattern:
		return s.processNewRelationship(ctx, change)
	case models.ChangeTypeNewEnumValue:
		return s.processEnumUpdate(ctx, change)
	default:
		s.logger.Debug("Change type does not require enrichment",
			zap.String("change_type", change.ChangeType))
		return nil
	}
}

// ProcessChanges processes a batch of changes, grouping by type for efficiency.
func (s *incrementalDAGService) ProcessChanges(ctx context.Context, changes []*models.PendingChange) error {
	if len(changes) == 0 {
		return nil
	}

	// Group by type
	byType := make(map[string][]*models.PendingChange)
	for _, c := range changes {
		byType[c.ChangeType] = append(byType[c.ChangeType], c)
	}

	// Process in dependency order: tables -> columns -> relationships -> enums

	// 1. New tables first (creates entities)
	if tables := byType[models.ChangeTypeNewTable]; len(tables) > 0 {
		for _, change := range tables {
			if err := s.processNewTable(ctx, change); err != nil {
				s.logger.Error("Failed to process new table",
					zap.String("table_name", change.TableName),
					zap.Error(err))
				// Continue with other changes
			}
		}
	}

	// 2. New columns
	if columns := byType[models.ChangeTypeNewColumn]; len(columns) > 0 {
		for _, change := range columns {
			if err := s.processNewColumn(ctx, change); err != nil {
				s.logger.Error("Failed to process new column",
					zap.String("table_name", change.TableName),
					zap.String("column_name", change.ColumnName),
					zap.Error(err))
			}
		}
	}

	// 3. New FK patterns / relationships
	if rels := byType[models.ChangeTypeNewFKPattern]; len(rels) > 0 {
		for _, change := range rels {
			if err := s.processNewRelationship(ctx, change); err != nil {
				s.logger.Error("Failed to process new relationship",
					zap.String("change_id", change.ID.String()),
					zap.Error(err))
			}
		}
	}

	// 4. Enum updates (no LLM needed)
	if enums := byType[models.ChangeTypeNewEnumValue]; len(enums) > 0 {
		for _, change := range enums {
			if err := s.processEnumUpdate(ctx, change); err != nil {
				s.logger.Error("Failed to process enum update",
					zap.String("table_name", change.TableName),
					zap.String("column_name", change.ColumnName),
					zap.Error(err))
			}
		}
	}

	return nil
}

// processNewTable handles new table changes.
// Note: Entity creation has been removed for v1.0 simplification.
// This method is a no-op until table-level metadata is implemented.
func (s *incrementalDAGService) processNewTable(ctx context.Context, change *models.PendingChange) error {
	s.logger.Debug("Skipping processNewTable - entity functionality removed for v1.0",
		zap.String("table_name", change.TableName))
	return nil
}

// processNewColumn enriches a new column with LLM-generated metadata.
// TODO: This function needs to be updated for the new ColumnMetadata schema.
// The new schema uses SchemaColumnID (FK) instead of TableName/ColumnName,
// and features are stored in Features JSONB.
// See PLAN-column-schema-refactor.md for details.
func (s *incrementalDAGService) processNewColumn(_ context.Context, change *models.PendingChange) error {
	s.logger.Warn("processNewColumn not yet implemented for new column metadata schema",
		zap.String("table_name", change.TableName),
		zap.String("column_name", change.ColumnName))
	return nil
}

// processNewRelationship handles new relationship changes.
// Note: Entity relationship functionality has been removed for v1.0 simplification.
// Relationships are now stored at the schema level (SchemaRelationship), not entity level.
func (s *incrementalDAGService) processNewRelationship(ctx context.Context, change *models.PendingChange) error {
	s.logger.Debug("Skipping processNewRelationship - entity relationship functionality removed for v1.0",
		zap.String("change_id", change.ID.String()))
	return nil
}

// processEnumUpdate merges new enum values into existing column metadata.
// TODO: This function needs to be updated for the new ColumnMetadata schema.
// Enum values are now stored in Features JSONB under EnumFeatures.
// See PLAN-column-schema-refactor.md for details.
func (s *incrementalDAGService) processEnumUpdate(_ context.Context, change *models.PendingChange) error {
	s.logger.Warn("processEnumUpdate not yet implemented for new column metadata schema",
		zap.String("table_name", change.TableName),
		zap.String("column_name", change.ColumnName))
	return nil
}
