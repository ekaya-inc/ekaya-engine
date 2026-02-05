package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// KnowledgeSeedingMethods defines the methods needed for knowledge seeding from project overview.
// This interface allows the node to call service methods without causing import cycles.
type KnowledgeSeedingMethods interface {
	// ExtractKnowledgeFromOverview extracts domain knowledge facts from the project overview.
	// Returns the number of facts stored.
	ExtractKnowledgeFromOverview(ctx context.Context, projectID, datasourceID uuid.UUID) (int, error)
}

// KnowledgeSeedingNode extracts domain knowledge facts from the project overview.
// This node processes the user-provided project overview (if available) along with
// schema context to extract business rules, conventions, and domain terms
// that will be used to improve downstream ontology extraction.
type KnowledgeSeedingNode struct {
	*BaseNode
	knowledgeSeeding KnowledgeSeedingMethods
}

// NewKnowledgeSeedingNode creates a new knowledge seeding node.
func NewKnowledgeSeedingNode(
	dagRepo repositories.OntologyDAGRepository,
	knowledgeSeeding KnowledgeSeedingMethods,
	logger *zap.Logger,
) *KnowledgeSeedingNode {
	return &KnowledgeSeedingNode{
		BaseNode:         NewBaseNode(models.DAGNodeKnowledgeSeeding, dagRepo, logger),
		knowledgeSeeding: knowledgeSeeding,
	}
}

// Execute runs the knowledge seeding phase.
// It extracts domain knowledge facts from the project overview using LLM.
//
// Note: This node does not require dag.OntologyID. Knowledge seeding operates at
// the project/datasource level, extracting domain facts from the project overview
// before any ontology-specific processing occurs.
func (n *KnowledgeSeedingNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting knowledge seeding",
		zap.String("project_id", dag.ProjectID.String()),
		zap.String("datasource_id", dag.DatasourceID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 1, "Extracting domain knowledge from overview..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Check if knowledge seeding is configured (nil means no-op mode for backward compatibility)
	if n.knowledgeSeeding == nil {
		n.Logger().Info("Knowledge seeding skipped (inference-based)",
			zap.String("project_id", dag.ProjectID.String()))
		if err := n.ReportProgress(ctx, 1, 1, "Knowledge seeding complete (inference-based)"); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
		return nil
	}

	// Call the underlying service method to extract knowledge from overview
	factsStored, err := n.knowledgeSeeding.ExtractKnowledgeFromOverview(ctx, dag.ProjectID, dag.DatasourceID)
	if err != nil {
		// Connection/endpoint errors indicate LLM config problems that will affect ALL nodes.
		// These must propagate to fail the DAG and show the user what's wrong.
		errType := llm.GetErrorType(err)
		if errType == llm.ErrorTypeEndpoint || errType == llm.ErrorTypeAuth {
			return fmt.Errorf("LLM configuration error: %w", err)
		}

		// Other errors (parsing, rate limits, etc.) - continue without seeded knowledge
		// Knowledge seeding enhances but isn't required for extraction to succeed.
		n.Logger().Warn("Failed to extract knowledge from overview - continuing without seeded knowledge",
			zap.String("project_id", dag.ProjectID.String()),
			zap.String("degradation_type", "knowledge_seeding"),
			zap.Error(err))
		factsStored = 0
	}

	// Report completion
	var progressMsg string
	if factsStored == 0 {
		progressMsg = "No knowledge facts extracted"
	} else {
		progressMsg = fmt.Sprintf("Extracted %d domain facts", factsStored)
	}

	if err := n.ReportProgress(ctx, 1, 1, progressMsg); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Knowledge seeding complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("facts_stored", factsStored))

	return nil
}
