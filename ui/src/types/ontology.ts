/**
 * Ontology Types
 * Types for ontology visualization and knowledge representation
 * Uses Hierarchical Tiered Ontology (HTO) system
 */

export type ConfidenceLevel = 'high' | 'medium' | 'low';

export interface OntologyTable {
  name: string;
  confidence: number;
  columns: OntologyColumn[];
}

export interface OntologyColumn {
  name: string;
  type: string;
  confidence: ConfidenceLevel;
  isPrimaryKey?: boolean;
  isForeignKey?: boolean;
  description?: string;
}

export interface OntologyRelationship {
  fromTable: string;
  toTable: string;
  fromColumn: string;
  toColumn: string;
  type: 'one-to-one' | 'one-to-many' | 'many-to-many';
  confidence: ConfidenceLevel;
}

export interface OntologyMetric {
  name: string;
  value: number;
  unit?: string;
  trend?: 'up' | 'down' | 'stable';
}

export interface BusinessRule {
  id: string;
  description: string;
  tables: string[];
  confidence: ConfidenceLevel;
}

export interface OntologyQuestion {
  id: string;
  question: string;
  category: string;
  priority: 'high' | 'medium' | 'low';
  answered: boolean;
}

// Ontology Workflow API Types
// Matches the Go backend response structures

/**
 * Response from POST /ontology/v1/{project_id}/extract
 */
export interface ExtractOntologyResponse {
  workflow_id: string;
  status: string;
  message?: string;
  error?: string;
}

/**
 * Entity progress in the work queue (from backend) - LEGACY
 */
export interface EntityProgressResponse {
  entity_name: string;
  status: 'queued' | 'processing' | 'complete' | 'updating' | 'schema-changed' | 'outdated' | 'failed';
  token_count?: number;
  last_updated?: string;
  error_message?: string;
}

/**
 * Task progress in the work queue (from backend) - NEW
 */
export interface TaskProgressResponse {
  id: string;
  name: string;
  status: 'queued' | 'processing' | 'complete' | 'failed';
  requires_llm: boolean;
  started_at?: string;
  completed_at?: string;
  error_message?: string;
}

/**
 * Response from GET /ontology/v1/{project_id}/workflows/{id}/status
 */
export interface WorkflowStatusResponse {
  workflow_id: string;
  project_id?: string;
  user_id?: string;
  current_phase: string;
  completed_phases: string[];
  confidence_score: number;
  iteration_count: number;
  max_iterations?: number;
  is_complete: boolean;
  exit_reason?: string;
  created_at?: string;
  updated_at?: string;
  completed_at?: string;
  last_error?: string;
  errors?: WorkflowError[];
  // UI-friendly computed fields from backend
  status_label: string;   // Human-readable status text (e.g., "Completed", "Cancelled", "Analyzing Schema")
  status_type: 'success' | 'error' | 'warning' | 'info' | 'processing'; // For UI styling
  can_start_new: boolean; // Whether user can start a new workflow
  has_result: boolean;    // Whether there's a successful result to display
  // Entity queue for work queue UI (LEGACY)
  entity_queue?: EntityProgressResponse[];
  total_entities?: number;
  completed_entities?: number;
  // Task queue for work queue UI (NEW)
  task_queue?: TaskProgressResponse[];
  total_tasks?: number;
  completed_tasks?: number;
  answered_questions?: number;
  pending_questions_count?: number;  // Questions awaiting user answers
}

/**
 * Workflow error information
 */
export interface WorkflowError {
  phase: string;
  code?: string;           // Machine-readable error code (e.g., "llm_token_limit")
  message: string;         // User-friendly error message
  details?: string;        // Technical details for debugging
  timestamp: string;
  severity: 'error' | 'warning' | 'info';
  retryable?: boolean;
  http_status?: number;
}

/**
 * Response from GET /ontology/v1/{project_id}/workflows/{id}/result
 */
export interface WorkflowResultResponse {
  workflow_id: string;
  tiered_ontology?: TieredOntology;
  confidence_score: number;
  metadata?: Record<string, unknown>;
}

// ===================================================================
// Hierarchical Tiered Ontology (HTO) Types
// ===================================================================

/**
 * Complete tiered ontology structure
 */
export interface TieredOntology {
  domain_summary: DomainSummary;
  entity_summaries: EntitySummary[];
  column_details?: EntityColumns[];
}

/**
 * Column details for an entity
 */
export interface EntityColumns {
  table_name: string;
  columns: ColumnDetail[];
}

/**
 * Detailed column information
 */
export interface ColumnDetail {
  name: string;
  description: string;
  synonyms: string[];
  semantic_type: string;
  role: string;
  enum_values?: EnumValue[];
  nullable: boolean;
  is_primary_key: boolean;
  is_foreign_key: boolean;
  foreign_table?: string;
}

/**
 * Enum value with business meaning
 */
export interface EnumValue {
  value: string;
  meaning: string;
}

/**
 * Domain-level summary providing high-level context (~500 tokens total)
 */
export interface DomainSummary {
  domains: string[];                  // e.g., ["sales", "finance", "operations"]
  entity_names: string[];             // Table names only, no details
  relationship_graph: RelationshipEdge[]; // Edges between entities
  description: string;                // 1-2 sentence business summary
  sample_questions: string[];         // 3-5 example questions
}

/**
 * Relationship edge in the graph
 */
export interface RelationshipEdge {
  from: string;    // Source entity name
  to: string;      // Target entity name
  type: string;    // one_to_many, many_to_one, etc.
}

/**
 * Entity-level summary (~75 tokens per entity)
 */
export interface EntitySummary {
  table_name: string;
  business_name: string;
  description: string;        // 1 sentence max
  domain: string;
  synonyms: string[];         // 5-10 for semantic matching
  key_columns: KeyColumn[];   // Max 5 columns
  column_count: number;       // Total columns (not the columns themselves)
  relationships: string[];    // Direct 1-hop entity names only
}

/**
 * Key column with synonyms for matching
 */
export interface KeyColumn {
  name: string;
  synonyms: string[];         // e.g., ["revenue", "sales", "income"]
}

/**
 * Response from GET /ontology/v1/{project_id}/workflows/{id}/questions
 */
export interface WorkflowQuestionsResponse {
  workflow_id: string;
  questions: QuestionsByPriority;
}

/**
 * Workflow question
 */
export interface WorkflowQuestion {
  id: string;
  text: string;
  context: string;
  category: string;
  priority: number;
  options?: string[];
  suggested_answer?: string;
  reasoning?: string;
  affects?: string[];  // Entity names affected by this question
  created_at?: string;
  answered_at?: string;
}

/**
 * Questions grouped by priority
 */
export interface QuestionsByPriority {
  critical: WorkflowQuestion[];
  high: WorkflowQuestion[];
  medium: WorkflowQuestion[];
  low: WorkflowQuestion[];
}

/**
 * Request body for POST /ontology/v1/{project_id}/workflows/{id}/answers
 */
export interface SubmitAnswersRequest {
  answers: WorkflowAnswer[];
}

/**
 * Workflow answer
 */
export interface WorkflowAnswer {
  question_id: string;
  answer: string;
}

/**
 * Response from POST /ontology/v1/{project_id}/workflows/{id}/answers
 */
export interface SubmitAnswersResponse {
  workflow_id: string;
  status: string;
  message?: string;
  confidence_score?: number;
  is_complete?: boolean;
}

// ===================================================================
// Tiered Ontology UI Types (Work Queue Model)
// ===================================================================

/**
 * Entity processing status in the work queue
 */
export type EntityStatus =
  | 'complete'       // ✓ - Entity fully processed
  | 'processing'     // ● - Currently being processed
  | 'queued'         // ○ - Waiting to be processed
  | 'updating'       // ⟳ - Being updated after user answer
  | 'schema-changed' // ★ - Schema changed, needs re-processing
  | 'outdated'       // ⚠ - Stale data, may need refresh
  | 'failed';        // ✗ - Processing failed

/**
 * Work item in the extraction queue (LEGACY - entity-based)
 */
export interface WorkItem {
  entityName: string;
  status: EntityStatus;
  tokenCount?: number;       // Token count during processing
  lastUpdated?: string;      // ISO timestamp
  errorMessage?: string;     // Error details if failed
}

/**
 * Task status in the work queue (NEW - task-based)
 */
export type TaskStatus = 'queued' | 'processing' | 'complete' | 'failed';

/**
 * Task item in the work queue (NEW)
 */
export interface WorkQueueTaskItem {
  id: string;
  name: string;
  status: TaskStatus;
  requiresLlm: boolean;
  startedAt?: string;        // ISO timestamp
  completedAt?: string;      // ISO timestamp
  errorMessage?: string;     // Error details if failed
}

/**
 * Question requiring user input during extraction
 */
export interface ExtractionQuestion {
  id: string;
  text: string;
  affects: string[];         // Entity names affected by this question
  answer?: string;           // User's answer
  fileAttachment?: File;     // Optional file attachment
  isSubmitted: boolean;
}

/**
 * Overall workflow state
 */
export type WorkflowState =
  | 'idle'          // No extraction running
  | 'initializing'  // Starting up
  | 'building'      // Actively processing entities
  | 'paused'        // User paused extraction
  | 'complete';     // All entities processed

/**
 * Workflow progress and performance stats
 */
export interface WorkflowProgress {
  state: WorkflowState;
  current: number;           // Entities completed
  total: number;             // Total entities to process
  tokensPerSecond?: number;  // Processing rate
  timeRemainingMs?: number;  // Estimated time to completion
  startedAt?: string;        // ISO timestamp
  pausedAt?: string;         // ISO timestamp if paused
}

/**
 * Combined status for the UI
 */
export interface OntologyWorkflowStatus {
  progress: WorkflowProgress;
  workQueue: WorkItem[];                    // LEGACY: entity-based queue
  taskQueue: WorkQueueTaskItem[];           // NEW: task-based queue
  questions: ExtractionQuestion[];
  pendingQuestionCount: number;
  errors?: WorkflowError[];
  lastError?: string;
}

// ===================================================================
// Question-by-Question Flow Types (Application-Controlled State Machine)
// ===================================================================

/**
 * Single question DTO from the backend
 */
export interface QuestionDTO {
  id: string;
  text: string;
  priority: number;
  category: string;
  reasoning?: string;
  affected_tables?: string[];
  affected_columns?: string[];
}

/**
 * Request body for answering a question
 */
export interface AnswerQuestionRequest {
  answer: string;
}

/**
 * Response after answering a question
 */
export interface AnswerQuestionResponse {
  question_id: string;
  follow_up?: string;           // If non-null, LLM needs clarification
  next_question?: QuestionDTO;  // Next question if no follow-up
  all_complete: boolean;        // True if no more questions
  actions_summary?: string;     // Human-readable summary of actions taken
  thinking?: string;            // LLM's thinking process (for debugging)
}

/**
 * Response after skipping or deleting a question
 */
export interface SkipDeleteResponse {
  skipped_id?: string;
  deleted_id?: string;
  next_question?: QuestionDTO;
  all_complete: boolean;
}

/**
 * Response from GET /questions/next
 */
export interface GetNextQuestionResponse {
  question?: QuestionDTO;
  all_complete: boolean;
}

/**
 * State of the QuestionPanel component
 */
export type QuestionPanelState =
  | 'loading'           // Initial load
  | 'showing_question'  // Displaying a question
  | 'waiting_for_llm'   // Processing answer with LLM
  | 'showing_follow_up' // LLM asked for clarification
  | 'all_complete';     // No more questions
