package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ChangeReviewService handles review/approval workflow for pending ontology changes.
// It implements the precedence model: Manual (UI) > MCP (Claude) > Inference (Engine)
type ChangeReviewService interface {
	// ListPendingChanges returns pending changes awaiting review.
	ListPendingChanges(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.PendingChange, error)

	// ApproveChange approves a pending change and applies it to the ontology.
	// Returns an error if the change cannot be applied due to precedence rules.
	ApproveChange(ctx context.Context, changeID uuid.UUID, reviewerSource string) (*models.PendingChange, error)

	// RejectChange rejects a pending change without applying it.
	RejectChange(ctx context.Context, changeID uuid.UUID, reviewerSource string) (*models.PendingChange, error)

	// ApproveAllChanges approves all pending changes that can be applied by the reviewer.
	// Returns the number of changes approved and any changes that were skipped due to precedence.
	ApproveAllChanges(ctx context.Context, projectID uuid.UUID, reviewerSource string) (*ApproveAllResult, error)

	// CanModify checks if a source can modify an ontology element based on precedence.
	// Returns true if the modification is allowed, false if blocked by higher precedence.
	CanModify(elementCreatedBy string, elementUpdatedBy *string, modifierSource string) bool
}

// ApproveAllResult contains the result of approving all changes.
type ApproveAllResult struct {
	Approved      int      `json:"approved"`
	Skipped       int      `json:"skipped"`
	SkippedReason []string `json:"skipped_reason,omitempty"`
}

type changeReviewService struct {
	pendingChangeRepo  repositories.PendingChangeRepository
	entityRepo         repositories.OntologyEntityRepository
	relationshipRepo   repositories.EntityRelationshipRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	ontologyRepo       repositories.OntologyRepository
	precedenceChecker  PrecedenceChecker     // Precedence validation service
	incrementalDAG     IncrementalDAGService // Optional: triggers LLM enrichment after approval
	logger             *zap.Logger
}

// ChangeReviewServiceDeps contains dependencies for ChangeReviewService.
type ChangeReviewServiceDeps struct {
	PendingChangeRepo  repositories.PendingChangeRepository
	EntityRepo         repositories.OntologyEntityRepository
	RelationshipRepo   repositories.EntityRelationshipRepository
	ColumnMetadataRepo repositories.ColumnMetadataRepository
	OntologyRepo       repositories.OntologyRepository
	PrecedenceChecker  PrecedenceChecker     // Optional: defaults to NewPrecedenceChecker() if nil
	IncrementalDAG     IncrementalDAGService // Optional: set to enable LLM enrichment after approval
	Logger             *zap.Logger
}

// NewChangeReviewService creates a new ChangeReviewService.
func NewChangeReviewService(deps *ChangeReviewServiceDeps) ChangeReviewService {
	precedenceChecker := deps.PrecedenceChecker
	if precedenceChecker == nil {
		precedenceChecker = NewPrecedenceChecker()
	}
	return &changeReviewService{
		pendingChangeRepo:  deps.PendingChangeRepo,
		entityRepo:         deps.EntityRepo,
		relationshipRepo:   deps.RelationshipRepo,
		columnMetadataRepo: deps.ColumnMetadataRepo,
		ontologyRepo:       deps.OntologyRepo,
		precedenceChecker:  precedenceChecker,
		incrementalDAG:     deps.IncrementalDAG,
		logger:             deps.Logger,
	}
}

// CanModify delegates to the PrecedenceChecker service.
func (s *changeReviewService) CanModify(elementCreatedBy string, elementUpdatedBy *string, modifierSource string) bool {
	return s.precedenceChecker.CanModify(elementCreatedBy, elementUpdatedBy, modifierSource)
}

func (s *changeReviewService) ListPendingChanges(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.PendingChange, error) {
	return s.pendingChangeRepo.List(ctx, projectID, models.ChangeStatusPending, limit)
}

func (s *changeReviewService) ApproveChange(ctx context.Context, changeID uuid.UUID, reviewerSource string) (*models.PendingChange, error) {
	// Get the change
	change, err := s.pendingChangeRepo.GetByID(ctx, changeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending change: %w", err)
	}
	if change == nil {
		return nil, fmt.Errorf("pending change not found")
	}

	// Verify change is still pending
	if change.Status != models.ChangeStatusPending {
		return nil, fmt.Errorf("change is not pending (status: %s)", change.Status)
	}

	// Apply the change based on its type
	if err := s.applyChange(ctx, change, reviewerSource); err != nil {
		return nil, fmt.Errorf("failed to apply change: %w", err)
	}

	// Update the change status
	if err := s.pendingChangeRepo.UpdateStatus(ctx, changeID, models.ChangeStatusApproved, reviewerSource); err != nil {
		return nil, fmt.Errorf("failed to update change status: %w", err)
	}

	change.Status = models.ChangeStatusApproved
	change.ReviewedBy = &reviewerSource

	// Trigger incremental LLM enrichment asynchronously if configured
	if s.incrementalDAG != nil {
		s.incrementalDAG.ProcessChangeAsync(ctx, change)
	}

	return change, nil
}

func (s *changeReviewService) RejectChange(ctx context.Context, changeID uuid.UUID, reviewerSource string) (*models.PendingChange, error) {
	// Get the change
	change, err := s.pendingChangeRepo.GetByID(ctx, changeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending change: %w", err)
	}
	if change == nil {
		return nil, fmt.Errorf("pending change not found")
	}

	// Verify change is still pending
	if change.Status != models.ChangeStatusPending {
		return nil, fmt.Errorf("change is not pending (status: %s)", change.Status)
	}

	// Update the change status
	if err := s.pendingChangeRepo.UpdateStatus(ctx, changeID, models.ChangeStatusRejected, reviewerSource); err != nil {
		return nil, fmt.Errorf("failed to update change status: %w", err)
	}

	change.Status = models.ChangeStatusRejected
	change.ReviewedBy = &reviewerSource

	return change, nil
}

func (s *changeReviewService) ApproveAllChanges(ctx context.Context, projectID uuid.UUID, reviewerSource string) (*ApproveAllResult, error) {
	result := &ApproveAllResult{
		SkippedReason: []string{},
	}

	// Get all pending changes
	changes, err := s.pendingChangeRepo.List(ctx, projectID, models.ChangeStatusPending, 500)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending changes: %w", err)
	}

	var approvedChanges []*models.PendingChange

	for _, change := range changes {
		// Try to apply the change
		if err := s.applyChange(ctx, change, reviewerSource); err != nil {
			// Check if it's a precedence error
			result.Skipped++
			result.SkippedReason = append(result.SkippedReason, fmt.Sprintf("%s: %v", change.ID, err))
			continue
		}

		// Update the change status
		if err := s.pendingChangeRepo.UpdateStatus(ctx, change.ID, models.ChangeStatusApproved, reviewerSource); err != nil {
			result.Skipped++
			result.SkippedReason = append(result.SkippedReason, fmt.Sprintf("%s: failed to update status: %v", change.ID, err))
			continue
		}

		change.Status = models.ChangeStatusApproved
		approvedChanges = append(approvedChanges, change)
		result.Approved++
	}

	// Trigger batch incremental LLM enrichment asynchronously if configured
	if s.incrementalDAG != nil && len(approvedChanges) > 0 {
		go func() {
			if err := s.incrementalDAG.ProcessChanges(ctx, approvedChanges); err != nil {
				s.logger.Error("Failed to process approved changes for incremental enrichment",
					zap.Error(err),
					zap.Int("change_count", len(approvedChanges)))
			}
		}()
	}

	return result, nil
}

// applyChange applies a pending change to the ontology based on its type.
func (s *changeReviewService) applyChange(ctx context.Context, change *models.PendingChange, reviewerSource string) error {
	switch change.SuggestedAction {
	case models.SuggestedActionCreateEntity:
		return s.applyCreateEntity(ctx, change, reviewerSource)
	case models.SuggestedActionReviewEntity:
		return s.applyReviewEntity(ctx, change, reviewerSource)
	case models.SuggestedActionCreateColumnMetadata:
		return s.applyCreateColumnMetadata(ctx, change, reviewerSource)
	case models.SuggestedActionUpdateColumnMetadata:
		return s.applyUpdateColumnMetadata(ctx, change, reviewerSource)
	case models.SuggestedActionCreateRelationship:
		return s.applyCreateRelationship(ctx, change, reviewerSource)
	case models.SuggestedActionUpdateRelationship:
		return s.applyUpdateRelationship(ctx, change, reviewerSource)
	default:
		// For unknown or no-action changes, just mark as approved without applying
		s.logger.Info("no action to apply for change",
			zap.String("change_id", change.ID.String()),
			zap.String("change_type", change.ChangeType),
			zap.String("suggested_action", change.SuggestedAction),
		)
		return nil
	}
}

func (s *changeReviewService) applyCreateEntity(ctx context.Context, change *models.PendingChange, reviewerSource string) error {
	payload := change.SuggestedPayload
	if payload == nil {
		return fmt.Errorf("missing suggested_payload for create_entity")
	}

	// Get the active ontology
	ontology, err := s.ontologyRepo.GetActive(ctx, change.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return fmt.Errorf("no active ontology found")
	}

	// Extract entity details from payload
	name, _ := payload["name"].(string)
	if name == "" {
		name = change.TableName // Fall back to table name
	}

	// Note: Source and CreatedBy are set by the repository from provenance context
	entity := &models.OntologyEntity{
		ProjectID:     change.ProjectID,
		OntologyID:    ontology.ID,
		Name:          name,
		PrimaryTable:  change.TableName,
		PrimarySchema: "public",
		PrimaryColumn: "id", // Default, can be overridden by payload
	}

	if desc, ok := payload["description"].(string); ok {
		entity.Description = desc
	}
	if primaryColumn, ok := payload["primary_column"].(string); ok {
		entity.PrimaryColumn = primaryColumn
	}

	return s.entityRepo.Create(ctx, entity)
}

func (s *changeReviewService) applyReviewEntity(ctx context.Context, change *models.PendingChange, reviewerSource string) error {
	// Review entity typically means updating an existing entity
	// For now, we treat this as a no-op if there's nothing specific to update
	s.logger.Info("review_entity action - no automatic changes applied",
		zap.String("change_id", change.ID.String()),
		zap.String("table_name", change.TableName),
	)
	return nil
}

func (s *changeReviewService) applyCreateColumnMetadata(ctx context.Context, change *models.PendingChange, reviewerSource string) error {
	payload := change.SuggestedPayload
	if payload == nil {
		return fmt.Errorf("missing suggested_payload for create_column_metadata")
	}

	meta := &models.ColumnMetadata{
		ProjectID:  change.ProjectID,
		TableName:  change.TableName,
		ColumnName: change.ColumnName,
		CreatedBy:  reviewerSource,
	}

	if desc, ok := payload["description"].(string); ok {
		meta.Description = &desc
	}
	if entity, ok := payload["entity"].(string); ok {
		meta.Entity = &entity
	}
	if role, ok := payload["role"].(string); ok {
		meta.Role = &role
	}
	if enumVals, ok := payload["enum_values"].([]any); ok {
		var vals []string
		for _, v := range enumVals {
			if s, ok := v.(string); ok {
				vals = append(vals, s)
			}
		}
		meta.EnumValues = vals
	}

	return s.columnMetadataRepo.Upsert(ctx, meta)
}

func (s *changeReviewService) applyUpdateColumnMetadata(ctx context.Context, change *models.PendingChange, reviewerSource string) error {
	// Get existing metadata to check precedence
	existing, err := s.columnMetadataRepo.GetByTableColumn(ctx, change.ProjectID, change.TableName, change.ColumnName)
	if err != nil {
		return fmt.Errorf("failed to get existing column metadata: %w", err)
	}

	if existing != nil {
		// Check precedence
		if !s.CanModify(existing.CreatedBy, existing.UpdatedBy, reviewerSource) {
			return fmt.Errorf("cannot modify column metadata: precedence blocked (existing: %s, reviewer: %s)",
				s.precedenceChecker.GetEffectiveSource(existing.CreatedBy, existing.UpdatedBy), reviewerSource)
		}
	}

	// Apply the update
	payload := change.SuggestedPayload
	if payload == nil {
		return fmt.Errorf("missing suggested_payload for update_column_metadata")
	}

	meta := &models.ColumnMetadata{
		ProjectID:  change.ProjectID,
		TableName:  change.TableName,
		ColumnName: change.ColumnName,
		CreatedBy:  reviewerSource,
		UpdatedBy:  &reviewerSource,
	}

	if existing != nil {
		meta.ID = existing.ID
		meta.CreatedBy = existing.CreatedBy
		meta.CreatedAt = existing.CreatedAt
	}

	if desc, ok := payload["description"].(string); ok {
		meta.Description = &desc
	}
	if entity, ok := payload["entity"].(string); ok {
		meta.Entity = &entity
	}
	if role, ok := payload["role"].(string); ok {
		meta.Role = &role
	}
	if enumVals, ok := payload["enum_values"].([]any); ok {
		var vals []string
		for _, v := range enumVals {
			if s, ok := v.(string); ok {
				vals = append(vals, s)
			}
		}
		meta.EnumValues = vals
	}

	return s.columnMetadataRepo.Upsert(ctx, meta)
}

func (s *changeReviewService) applyCreateRelationship(ctx context.Context, change *models.PendingChange, reviewerSource string) error {
	payload := change.SuggestedPayload
	if payload == nil {
		return fmt.Errorf("missing suggested_payload for create_relationship")
	}

	// Get the active ontology
	ontology, err := s.ontologyRepo.GetActive(ctx, change.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return fmt.Errorf("no active ontology found")
	}

	// Extract relationship details from payload
	sourceEntityID, _ := payload["source_entity_id"].(string)
	targetEntityID, _ := payload["target_entity_id"].(string)

	if sourceEntityID == "" || targetEntityID == "" {
		return fmt.Errorf("missing source_entity_id or target_entity_id in payload")
	}

	sourceUUID, err := uuid.Parse(sourceEntityID)
	if err != nil {
		return fmt.Errorf("invalid source_entity_id: %w", err)
	}
	targetUUID, err := uuid.Parse(targetEntityID)
	if err != nil {
		return fmt.Errorf("invalid target_entity_id: %w", err)
	}

	// Note: Source and CreatedBy are set by the repository from provenance context
	rel := &models.EntityRelationship{
		OntologyID:        ontology.ID,
		SourceEntityID:    sourceUUID,
		TargetEntityID:    targetUUID,
		SourceColumnTable: change.TableName,
		SourceColumnName:  change.ColumnName,
		DetectionMethod:   "pending_change",
		Confidence:        0.8,
		Status:            "confirmed",
		Cardinality:       "unknown",
	}

	if desc, ok := payload["description"].(string); ok {
		rel.Description = &desc
	}
	if assoc, ok := payload["association"].(string); ok {
		rel.Association = &assoc
	}
	if card, ok := payload["cardinality"].(string); ok {
		rel.Cardinality = card
	}

	return s.relationshipRepo.Create(ctx, rel)
}

func (s *changeReviewService) applyUpdateRelationship(ctx context.Context, change *models.PendingChange, reviewerSource string) error {
	payload := change.SuggestedPayload
	if payload == nil {
		return fmt.Errorf("missing suggested_payload for update_relationship")
	}

	relIDStr, ok := payload["relationship_id"].(string)
	if !ok || relIDStr == "" {
		return fmt.Errorf("missing relationship_id in payload")
	}

	relID, err := uuid.Parse(relIDStr)
	if err != nil {
		return fmt.Errorf("invalid relationship_id: %w", err)
	}

	// Get existing relationship to check precedence
	existing, err := s.relationshipRepo.GetByID(ctx, relID)
	if err != nil {
		return fmt.Errorf("failed to get existing relationship: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("relationship not found")
	}

	// Check precedence using Source and LastEditSource (the method strings)
	if !s.CanModify(existing.Source, existing.LastEditSource, reviewerSource) {
		return fmt.Errorf("cannot modify relationship: precedence blocked (existing: %s, reviewer: %s)",
			s.precedenceChecker.GetEffectiveSource(existing.Source, existing.LastEditSource), reviewerSource)
	}

	// Apply updates
	// Note: UpdatedBy and LastEditSource are set by the repository from provenance context
	if desc, ok := payload["description"].(string); ok {
		existing.Description = &desc
	}
	if assoc, ok := payload["association"].(string); ok {
		existing.Association = &assoc
	}
	if card, ok := payload["cardinality"].(string); ok {
		existing.Cardinality = card
	}

	return s.relationshipRepo.Update(ctx, existing)
}
