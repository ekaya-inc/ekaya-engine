package dag

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// NodeExecutor defines the interface for DAG node execution.
// Each node wraps an existing service method and reports progress.
type NodeExecutor interface {
	// Name returns the node name (e.g., "EntityDiscovery")
	Name() models.DAGNodeName

	// Execute runs the node's work. Returns an error if the node fails.
	Execute(ctx context.Context, dag *models.OntologyDAG) error
}

// ProgressReporter provides a way for nodes to report execution progress.
type ProgressReporter interface {
	// ReportProgress updates the node's progress in the database.
	ReportProgress(ctx context.Context, nodeID uuid.UUID, current, total int, message string) error
}

// ProgressCallback is a function that reports progress updates.
type ProgressCallback func(current, total int, message string)

// BaseNode provides common functionality for all DAG nodes.
type BaseNode struct {
	nodeName      models.DAGNodeName
	dagRepo       repositories.OntologyDAGRepository
	logger        *zap.Logger
	currentNodeID uuid.UUID
}

// NewBaseNode creates a new base node with common dependencies.
func NewBaseNode(
	nodeName models.DAGNodeName,
	dagRepo repositories.OntologyDAGRepository,
	logger *zap.Logger,
) *BaseNode {
	return &BaseNode{
		nodeName: nodeName,
		dagRepo:  dagRepo,
		logger:   logger.Named(string(nodeName)),
	}
}

// Name returns the node name.
func (b *BaseNode) Name() models.DAGNodeName {
	return b.nodeName
}

// SetCurrentNodeID sets the node ID for progress reporting.
func (b *BaseNode) SetCurrentNodeID(nodeID uuid.UUID) {
	b.currentNodeID = nodeID
}

// ReportProgress updates the node's progress in the database.
func (b *BaseNode) ReportProgress(ctx context.Context, current, total int, message string) error {
	if b.currentNodeID == uuid.Nil {
		return nil // No node ID set, skip progress update
	}

	progress := &models.DAGNodeProgress{
		Current: current,
		Total:   total,
		Message: message,
	}

	return b.dagRepo.UpdateNodeProgress(ctx, b.currentNodeID, progress)
}

// Logger returns the node's logger.
func (b *BaseNode) Logger() *zap.Logger {
	return b.logger
}

// ExecutionContext provides context for node execution.
// This is passed to nodes to provide access to common data.
type ExecutionContext struct {
	DAG          *models.OntologyDAG
	NodeID       uuid.UUID
	ProjectID    uuid.UUID
	DatasourceID uuid.UUID
	OntologyID   uuid.UUID
}

// NewExecutionContext creates an execution context from a DAG and node.
func NewExecutionContext(dag *models.OntologyDAG, node *models.DAGNode) *ExecutionContext {
	var ontologyID uuid.UUID
	if dag.OntologyID != nil {
		ontologyID = *dag.OntologyID
	}

	return &ExecutionContext{
		DAG:          dag,
		NodeID:       node.ID,
		ProjectID:    dag.ProjectID,
		DatasourceID: dag.DatasourceID,
		OntologyID:   ontologyID,
	}
}
