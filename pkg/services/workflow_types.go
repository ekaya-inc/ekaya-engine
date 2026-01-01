package services

import (
	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// taskQueueUpdate holds data for a task queue database update.
// DEPRECATED: This type is used by relationship_workflow.go and entity_discovery_service.go
// until they are migrated to use the workflow.TaskQueueUpdate type from pkg/services/workflow.
type taskQueueUpdate struct {
	projectID  uuid.UUID
	workflowID uuid.UUID
	tasks      []models.WorkflowTask
}

// taskQueueWriter manages serialized writes for a single workflow.
// DEPRECATED: This type is used by relationship_workflow.go and entity_discovery_service.go
// until they are migrated to use the shared infrastructure from pkg/services/workflow.
type taskQueueWriter struct {
	updates chan taskQueueUpdate
	done    chan struct{}
}
