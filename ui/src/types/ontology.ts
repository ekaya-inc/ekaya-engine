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
  status: 'queued' | 'processing' | 'complete' | 'failed' | 'paused';
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
  completed_entities?: number;
  // Task queue for work queue UI (NEW)
  task_queue?: TaskProgressResponse[];
  total_tasks?: number;
  completed_tasks?: number;
  answered_questions?: number;
  pending_questions_count?: number;  // Questions awaiting user answers
  // UX improvement fields (NEW)
  ontology_ready?: boolean;     // True once Tier 0/1 building is complete
  total_entities?: number;      // Total entities (tables) to process
  current_entity?: number;      // Current entity progress count
  progress_message?: string;    // Human-readable progress message
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

/**
 * Response from GET /api/projects/{project_id}/ontology/enrichment
 */
export interface EnrichmentResponse {
  entity_summaries: EntitySummary[];
  column_details: EntityColumns[];
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
export type TaskStatus = 'queued' | 'processing' | 'complete' | 'failed' | 'paused';

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
  | 'idle'            // No extraction running
  | 'initializing'    // Starting up
  | 'building'        // Actively processing entities
  | 'awaiting_input'  // Questions need user answers
  | 'complete'        // All entities processed
  | 'error';          // Connection or fetch error

/**
 * Workflow progress and performance stats
 */
export interface WorkflowProgress {
  state: WorkflowState;
  current: number;           // Entities completed (global + tables + columns)
  total: number;             // Total entities to process (global + tables + columns)
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
  // UX improvement fields (NEW)
  ontologyReady?: boolean;                  // True once Tier 0/1 building is complete
  progressMessage?: string;                 // Human-readable progress message
}

// ===================================================================
// Question-by-Question Flow Types (Application-Controlled State Machine)
// ===================================================================

/**
 * Counts of pending questions by type
 */
export interface QuestionCounts {
  required: number;
  optional: number;
}

/**
 * Single question DTO from the backend
 */
export interface QuestionDTO {
  id: string;
  text: string;
  priority: number;
  is_required: boolean;
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
  counts?: QuestionCounts;      // Updated question counts
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
  counts?: QuestionCounts;
}

/**
 * State of the QuestionPanel component
 */
export type QuestionPanelState =
  | 'loading'              // Initial load
  | 'waiting_for_questions' // No questions available yet (extraction in progress)
  | 'showing_question'     // Displaying a question
  | 'waiting_for_llm'      // Processing answer with LLM
  | 'showing_follow_up'    // LLM asked for clarification
  | 'all_complete';        // No more questions

// ===================================================================
// DAG Workflow Types (New Unified Ontology Extraction)
// ===================================================================

/**
 * DAG status values
 */
export type DAGStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';

/**
 * DAG node status values
 */
export type DAGNodeStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped';

/**
 * DAG node names in execution order
 */
export type DAGNodeName =
  | 'KnowledgeSeeding'
  | 'EntityDiscovery'
  | 'EntityEnrichment'
  | 'FKDiscovery'
  | 'TableFeatureExtraction'
  | 'PKMatchDiscovery'
  | 'RelationshipDiscovery'
  | 'RelationshipEnrichment'
  | 'ColumnFeatureExtraction'
  | 'ColumnEnrichment'
  | 'OntologyFinalization'
  | 'GlossaryDiscovery'
  | 'GlossaryEnrichment';

/**
 * Progress within a DAG node
 */
export interface DAGNodeProgress {
  current: number;
  total: number;
  message?: string;
}

/**
 * A node within the DAG
 */
export interface DAGNode {
  name: DAGNodeName;
  status: DAGNodeStatus;
  progress?: DAGNodeProgress;
  error?: string;
}

/**
 * Response from DAG status API
 * Matches pkg/handlers/ontology_dag_handler.go DAGStatusResponse
 */
export interface DAGStatusResponse {
  dag_id: string;
  status: DAGStatus;
  current_node?: string;
  nodes: DAGNode[];
  started_at?: string;
  completed_at?: string;
}

// ===================================================================
// Extraction Phase Types (Multi-Phase Progress within DAG Nodes)
// ===================================================================

/**
 * Status of an extraction phase
 */
export type ExtractionPhaseStatus = 'pending' | 'in_progress' | 'complete' | 'failed';

/**
 * Represents progress within a single extraction phase
 * Used for multi-phase progress display (e.g., Column Feature Extraction)
 */
export interface ExtractionPhase {
  /** Unique phase identifier (e.g., "phase1", "phase2") */
  id: string;
  /** Display name for the phase */
  name: string;
  /** Current status of the phase */
  status: ExtractionPhaseStatus;
  /** Total items to process in this phase (known at phase start) */
  totalItems?: number;
  /** Number of items completed so far */
  completedItems?: number;
  /** Description of what's currently being processed */
  currentItem?: string;
}

/**
 * Progress for Column Feature Extraction (multi-phase mini-DAG)
 * Matches models.FeatureExtractionProgress from the backend
 */
export interface ColumnFeatureExtractionProgress {
  /** Current phase ID */
  currentPhase: string;
  /** Human-readable description of current activity */
  phaseDescription: string;
  /** Total items in current phase */
  totalItems: number;
  /** Completed items in current phase */
  completedItems: number;
  /** Total columns discovered */
  totalColumns: number;
  /** Number of enum candidates found */
  enumCandidates: number;
  /** Number of FK candidates found */
  fkCandidates: number;
  /** Number of tables needing cross-column analysis */
  crossColumnCandidates: number;
  /** Detailed phase progress */
  phases: ExtractionPhase[];
}

/**
 * Human-readable descriptions for each extraction phase
 */
export const ExtractionPhaseDescriptions: Record<string, { title: string; description: string }> = {
  phase1: {
    title: 'Collecting column metadata',
    description: 'Gathering statistics, samples, and patterns from each column',
  },
  phase2: {
    title: 'Classifying columns',
    description: 'Determining column purpose using AI analysis',
  },
  phase3: {
    title: 'Analyzing enum values',
    description: 'Labeling enumeration values and detecting state machines',
  },
  phase4: {
    title: 'Resolving FK candidates',
    description: 'Determining foreign key relationships via data overlap',
  },
  phase5: {
    title: 'Cross-column analysis',
    description: 'Detecting monetary pairs and soft delete patterns',
  },
  phase6: {
    title: 'Saving results',
    description: 'Persisting extracted features to the database',
  },
};

/**
 * Human-readable descriptions for each DAG node
 */
export const DAGNodeDescriptions: Record<DAGNodeName, { title: string; description: string }> = {
  KnowledgeSeeding: {
    title: 'Seeding Knowledge',
    description: 'Initialize project knowledge from description and schema',
  },
  EntityDiscovery: {
    title: 'Discovering Entities',
    description: 'Identifying entities from schema constraints',
  },
  EntityEnrichment: {
    title: 'Enriching Entities',
    description: 'Generating entity names and descriptions',
  },
  FKDiscovery: {
    title: 'Discovering Foreign Keys',
    description: 'Discovering foreign key relationships',
  },
  TableFeatureExtraction: {
    title: 'Extracting Table Features',
    description: 'Analyzing table metadata and usage patterns',
  },
  PKMatchDiscovery: {
    title: 'Discovering Primary Key Matches',
    description: 'Discovering relationships via primary key matching',
  },
  RelationshipDiscovery: {
    title: 'Discovering Relationships',
    description: 'Discovering foreign key relationships',
  },
  RelationshipEnrichment: {
    title: 'Enriching Relationships',
    description: 'Generating relationship descriptions',
  },
  ColumnFeatureExtraction: {
    title: 'Extracting Column Features',
    description: 'Analyzing column statistics and patterns',
  },
  ColumnEnrichment: {
    title: 'Enriching Columns',
    description: 'Generating column descriptions and semantic types',
  },
  OntologyFinalization: {
    title: 'Finalizing Ontology',
    description: 'Generating domain summary and conventions',
  },
  GlossaryDiscovery: {
    title: 'Discovering Glossary Terms',
    description: 'Discovering business glossary terms and definitions',
  },
  GlossaryEnrichment: {
    title: 'Enriching Glossary',
    description: 'Generating SQL definitions for glossary terms',
  },
};
