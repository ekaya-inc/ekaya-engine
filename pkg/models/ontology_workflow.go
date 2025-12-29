package models

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Workflow States
// ============================================================================

// WorkflowState represents the current state of an ontology extraction workflow.
type WorkflowState string

const (
	WorkflowStatePending       WorkflowState = "pending"
	WorkflowStateRunning       WorkflowState = "running"
	WorkflowStatePaused        WorkflowState = "paused"
	WorkflowStateAwaitingInput WorkflowState = "awaiting_input" // Questions need answers
	WorkflowStateCompleted     WorkflowState = "completed"
	WorkflowStateFailed        WorkflowState = "failed"
)

// ValidWorkflowStates contains all valid workflow state values.
var ValidWorkflowStates = []WorkflowState{
	WorkflowStatePending,
	WorkflowStateRunning,
	WorkflowStatePaused,
	WorkflowStateAwaitingInput,
	WorkflowStateCompleted,
	WorkflowStateFailed,
}

// IsValidWorkflowState checks if the given state is valid.
func IsValidWorkflowState(s WorkflowState) bool {
	for _, v := range ValidWorkflowStates {
		if v == s {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the workflow state is terminal (completed or failed).
func (s WorkflowState) IsTerminal() bool {
	return s == WorkflowStateCompleted || s == WorkflowStateFailed
}

// CanTransitionTo returns true if transitioning from this state to the target is valid.
func (s WorkflowState) CanTransitionTo(target WorkflowState) bool {
	switch s {
	case WorkflowStatePending:
		return target == WorkflowStateRunning || target == WorkflowStateFailed
	case WorkflowStateRunning:
		return target == WorkflowStatePaused || target == WorkflowStateAwaitingInput ||
			target == WorkflowStateCompleted || target == WorkflowStateFailed
	case WorkflowStatePaused:
		return target == WorkflowStateRunning || target == WorkflowStateFailed
	case WorkflowStateAwaitingInput:
		// Can continue (running), complete, or fail
		return target == WorkflowStateRunning || target == WorkflowStateCompleted || target == WorkflowStateFailed
	case WorkflowStateCompleted, WorkflowStateFailed:
		return target == WorkflowStatePending // Can restart
	default:
		return false
	}
}

// ============================================================================
// Workflow Progress
// ============================================================================

// WorkflowProgress tracks the progress of an ontology extraction workflow.
type WorkflowProgress struct {
	CurrentPhase    string  `json:"current_phase,omitempty"`
	Current         int     `json:"current"`
	Total           int     `json:"total"`
	TokensPerSecond float64 `json:"tokens_per_second,omitempty"`
	TimeRemainingMs int64   `json:"time_remaining_ms,omitempty"`
	Message         string  `json:"message,omitempty"`
	OntologyReady   bool    `json:"ontology_ready,omitempty"`
}

// Percentage returns the completion percentage (0-100).
func (p *WorkflowProgress) Percentage() int {
	if p == nil || p.Total == 0 {
		return 0
	}
	return int(float64(p.Current) / float64(p.Total) * 100)
}

// Workflow phases
const (
	WorkflowPhaseInitializing          = "initializing"
	WorkflowPhaseScanning              = "scanning"  // SQL-only data scanning phase
	WorkflowPhaseAnalyzing             = "analyzing" // LLM analysis phase
	WorkflowPhaseDescriptionProcessing = "description_processing"
	WorkflowPhaseSchemaAnalysis        = "schema_analysis"
	WorkflowPhaseDataProfiling         = "data_profiling"
	WorkflowPhaseQuestionGeneration    = "question_generation"
	WorkflowPhaseTier1Building         = "tier1_building"
	WorkflowPhaseTier0Building         = "tier0_building"
	WorkflowPhaseAwaitingInput         = "awaiting_input"
	WorkflowPhaseCompleting            = "completing"
)

// ============================================================================
// Workflow Tasks
// ============================================================================

// WorkflowTask represents a single task in the workflow queue.
type WorkflowTask struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"` // queued, processing, complete, failed
	RequiresLLM bool   `json:"requires_llm"`
	TableName   string `json:"table_name,omitempty"` // For per-table tasks
	Error       string `json:"error,omitempty"`
	RetryCount  int    `json:"retry_count,omitempty"` // Number of retry attempts made
}

// Task statuses
const (
	TaskStatusQueued     = "queued"
	TaskStatusProcessing = "processing"
	TaskStatusComplete   = "complete"
	TaskStatusFailed     = "failed"
	TaskStatusPaused     = "paused"
)

// Task types (used as Name field)
const (
	TaskTypeProfileTable      = "profile_table"
	TaskTypeUnderstandSchema  = "understand_schema"
	TaskTypeBuildTier0And1    = "build_tier0_and_tier1"
	TaskTypeGenerateQuestions = "generate_questions"
)

// ============================================================================
// Workflow Configuration
// ============================================================================

// WorkflowConfig contains configuration for an ontology extraction workflow.
type WorkflowConfig struct {
	DatasourceID       uuid.UUID   `json:"datasource_id"`
	IncludeAllTables   bool        `json:"include_all_tables,omitempty"`
	SelectedTableIDs   []uuid.UUID `json:"selected_table_ids,omitempty"`
	SkipDataProfiling  bool        `json:"skip_data_profiling,omitempty"`
	SkipQuestions      bool        `json:"skip_questions,omitempty"`
	MaxTablesPerBatch  int         `json:"max_tables_per_batch,omitempty"`
	ProjectDescription string      `json:"project_description,omitempty"`
}

// DefaultWorkflowConfig returns a default configuration.
func DefaultWorkflowConfig() *WorkflowConfig {
	return &WorkflowConfig{
		IncludeAllTables:  true,
		MaxTablesPerBatch: 20,
	}
}

// ============================================================================
// Ontology Workflow Model
// ============================================================================

// OntologyWorkflow represents an ontology extraction workflow.
type OntologyWorkflow struct {
	ID           uuid.UUID         `json:"id"`
	ProjectID    uuid.UUID         `json:"project_id"`
	OntologyID   uuid.UUID         `json:"ontology_id"`
	State        WorkflowState     `json:"state"`
	Progress     *WorkflowProgress `json:"progress,omitempty"`
	TaskQueue    []WorkflowTask    `json:"task_queue,omitempty"`
	Config       *WorkflowConfig   `json:"config,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
	StartedAt    *time.Time        `json:"started_at,omitempty"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`

	// Ownership fields for multi-server robustness
	OwnerID       *uuid.UUID `json:"owner_id,omitempty"`       // Server instance that owns this workflow
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"` // Last heartbeat from owner
}

// IsRunning returns true if the workflow is currently executing.
func (w *OntologyWorkflow) IsRunning() bool {
	return w.State == WorkflowStateRunning
}

// IsPaused returns true if the workflow is paused.
func (w *OntologyWorkflow) IsPaused() bool {
	return w.State == WorkflowStatePaused
}

// IsComplete returns true if the workflow completed successfully.
func (w *OntologyWorkflow) IsComplete() bool {
	return w.State == WorkflowStateCompleted
}

// HasFailed returns true if the workflow failed.
func (w *OntologyWorkflow) HasFailed() bool {
	return w.State == WorkflowStateFailed
}

// PendingTaskCount returns the number of tasks not yet complete.
func (w *OntologyWorkflow) PendingTaskCount() int {
	count := 0
	for _, task := range w.TaskQueue {
		if task.Status == TaskStatusQueued || task.Status == TaskStatusProcessing {
			count++
		}
	}
	return count
}

// CompletedTaskCount returns the number of completed tasks.
func (w *OntologyWorkflow) CompletedTaskCount() int {
	count := 0
	for _, task := range w.TaskQueue {
		if task.Status == TaskStatusComplete {
			count++
		}
	}
	return count
}
