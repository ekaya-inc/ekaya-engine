package models

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// DAG Status
// ============================================================================

// DAGStatus represents the execution status of an ontology DAG.
type DAGStatus string

const (
	DAGStatusPending   DAGStatus = "pending"
	DAGStatusRunning   DAGStatus = "running"
	DAGStatusCompleted DAGStatus = "completed"
	DAGStatusFailed    DAGStatus = "failed"
	DAGStatusCancelled DAGStatus = "cancelled"
)

// ValidDAGStatuses contains all valid DAG status values.
var ValidDAGStatuses = []DAGStatus{
	DAGStatusPending,
	DAGStatusRunning,
	DAGStatusCompleted,
	DAGStatusFailed,
	DAGStatusCancelled,
}

// IsValidDAGStatus checks if the given status is valid.
func IsValidDAGStatus(s DAGStatus) bool {
	for _, v := range ValidDAGStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the DAG status is terminal (completed, failed, or cancelled).
func (s DAGStatus) IsTerminal() bool {
	return s == DAGStatusCompleted || s == DAGStatusFailed || s == DAGStatusCancelled
}

// IsActive returns true if the DAG is currently active (pending or running).
func (s DAGStatus) IsActive() bool {
	return s == DAGStatusPending || s == DAGStatusRunning
}

// ============================================================================
// DAG Node Status
// ============================================================================

// DAGNodeStatus represents the execution status of a DAG node.
type DAGNodeStatus string

const (
	DAGNodeStatusPending   DAGNodeStatus = "pending"
	DAGNodeStatusRunning   DAGNodeStatus = "running"
	DAGNodeStatusCompleted DAGNodeStatus = "completed"
	DAGNodeStatusFailed    DAGNodeStatus = "failed"
	DAGNodeStatusSkipped   DAGNodeStatus = "skipped"
)

// ValidDAGNodeStatuses contains all valid node status values.
var ValidDAGNodeStatuses = []DAGNodeStatus{
	DAGNodeStatusPending,
	DAGNodeStatusRunning,
	DAGNodeStatusCompleted,
	DAGNodeStatusFailed,
	DAGNodeStatusSkipped,
}

// IsValidDAGNodeStatus checks if the given status is valid.
func IsValidDAGNodeStatus(s DAGNodeStatus) bool {
	for _, v := range ValidDAGNodeStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the node status is terminal.
func (s DAGNodeStatus) IsTerminal() bool {
	return s == DAGNodeStatusCompleted || s == DAGNodeStatusFailed || s == DAGNodeStatusSkipped
}

// ============================================================================
// DAG Node Names
// ============================================================================

// DAGNodeName represents the name of a node in the ontology DAG.
type DAGNodeName string

const (
	DAGNodeEntityDiscovery        DAGNodeName = "EntityDiscovery"
	DAGNodeEntityEnrichment       DAGNodeName = "EntityEnrichment"
	DAGNodeFKDiscovery            DAGNodeName = "FKDiscovery"
	DAGNodeColumnEnrichment       DAGNodeName = "ColumnEnrichment"
	DAGNodePKMatchDiscovery       DAGNodeName = "PKMatchDiscovery"
	DAGNodeRelationshipEnrichment DAGNodeName = "RelationshipEnrichment"
	DAGNodeOntologyFinalization   DAGNodeName = "OntologyFinalization"
	DAGNodeGlossaryDiscovery      DAGNodeName = "GlossaryDiscovery"
	DAGNodeGlossaryEnrichment     DAGNodeName = "GlossaryEnrichment"
)

// DAGNodeOrder defines the execution order for each node.
var DAGNodeOrder = map[DAGNodeName]int{
	DAGNodeEntityDiscovery:        1,
	DAGNodeEntityEnrichment:       2,
	DAGNodeFKDiscovery:            3,
	DAGNodeColumnEnrichment:       4,
	DAGNodePKMatchDiscovery:       5,
	DAGNodeRelationshipEnrichment: 6,
	DAGNodeOntologyFinalization:   7,
	DAGNodeGlossaryDiscovery:      8,
	DAGNodeGlossaryEnrichment:     9,
}

// AllDAGNodes returns all DAG node names in execution order.
func AllDAGNodes() []DAGNodeName {
	return []DAGNodeName{
		DAGNodeEntityDiscovery,
		DAGNodeEntityEnrichment,
		DAGNodeFKDiscovery,
		DAGNodeColumnEnrichment,
		DAGNodePKMatchDiscovery,
		DAGNodeRelationshipEnrichment,
		DAGNodeOntologyFinalization,
		DAGNodeGlossaryDiscovery,
		DAGNodeGlossaryEnrichment,
	}
}

// ============================================================================
// DAG Node Progress
// ============================================================================

// DAGNodeProgress tracks the progress of a DAG node.
type DAGNodeProgress struct {
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Message string `json:"message,omitempty"`
}

// Percentage returns the completion percentage (0-100).
func (p *DAGNodeProgress) Percentage() int {
	if p == nil || p.Total == 0 {
		return 0
	}
	return int(float64(p.Current) / float64(p.Total) * 100)
}

// ============================================================================
// Ontology DAG Model
// ============================================================================

// OntologyDAG represents a DAG execution for ontology extraction.
type OntologyDAG struct {
	ID           uuid.UUID  `json:"id"`
	ProjectID    uuid.UUID  `json:"project_id"`
	DatasourceID uuid.UUID  `json:"datasource_id"`
	OntologyID   *uuid.UUID `json:"ontology_id,omitempty"`

	// Execution state
	Status      DAGStatus `json:"status"`
	CurrentNode *string   `json:"current_node,omitempty"`

	// Schema tracking
	SchemaFingerprint *string `json:"schema_fingerprint,omitempty"`

	// Ownership (multi-server support)
	OwnerID       *uuid.UUID `json:"owner_id,omitempty"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`

	// Timing
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	// Nodes (populated when fetching with nodes)
	Nodes []DAGNode `json:"nodes,omitempty"`
}

// IsRunning returns true if the DAG is currently running.
func (d *OntologyDAG) IsRunning() bool {
	return d.Status == DAGStatusRunning
}

// IsComplete returns true if the DAG completed successfully.
func (d *OntologyDAG) IsComplete() bool {
	return d.Status == DAGStatusCompleted
}

// HasFailed returns true if the DAG failed.
func (d *OntologyDAG) HasFailed() bool {
	return d.Status == DAGStatusFailed
}

// IsCancelled returns true if the DAG was cancelled.
func (d *OntologyDAG) IsCancelled() bool {
	return d.Status == DAGStatusCancelled
}

// CompletedNodeCount returns the number of completed nodes.
func (d *OntologyDAG) CompletedNodeCount() int {
	count := 0
	for _, node := range d.Nodes {
		if node.Status == DAGNodeStatusCompleted {
			count++
		}
	}
	return count
}

// TotalNodeCount returns the total number of nodes.
func (d *OntologyDAG) TotalNodeCount() int {
	return len(d.Nodes)
}

// ============================================================================
// DAG Node Model
// ============================================================================

// DAGNode represents a node within an ontology DAG.
type DAGNode struct {
	ID    uuid.UUID `json:"id"`
	DAGID uuid.UUID `json:"dag_id"`

	// Node identification
	NodeName  string `json:"node_name"`
	NodeOrder int    `json:"node_order"`

	// Execution state
	Status   DAGNodeStatus    `json:"status"`
	Progress *DAGNodeProgress `json:"progress,omitempty"`

	// Timing
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	DurationMs  *int       `json:"duration_ms,omitempty"`

	// Error handling
	ErrorMessage *string `json:"error_message,omitempty"`
	RetryCount   int     `json:"retry_count"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsRunning returns true if the node is currently running.
func (n *DAGNode) IsRunning() bool {
	return n.Status == DAGNodeStatusRunning
}

// IsComplete returns true if the node completed successfully.
func (n *DAGNode) IsComplete() bool {
	return n.Status == DAGNodeStatusCompleted
}

// HasFailed returns true if the node failed.
func (n *DAGNode) HasFailed() bool {
	return n.Status == DAGNodeStatusFailed
}

// IsSkipped returns true if the node was skipped.
func (n *DAGNode) IsSkipped() bool {
	return n.Status == DAGNodeStatusSkipped
}
