/**
 * Relationship Workflow Types
 * Types for the intelligent relationship detection workflow API
 * Matches pkg/handlers/relationship_workflow.go response structures
 */

import type { TaskProgressResponse, WorkflowProgress } from './ontology';

// ============================================================================
// Status and Method Enums
// ============================================================================

export type RelationshipWorkflowPhase = 'relationships' | 'ontology';
export type RelationshipWorkflowState = 'pending' | 'running' | 'paused' | 'awaiting_input' | 'completed' | 'failed';
export type RelationshipCandidateStatus = 'pending' | 'accepted' | 'rejected';
export type DetectionMethod = 'value_match' | 'name_inference' | 'llm' | 'hybrid';

// ============================================================================
// API Response Types
// ============================================================================

/**
 * Response from POST /api/projects/{pid}/datasources/{dsid}/relationships/detect
 */
export interface StartDetectionResponse {
  workflow_id: string;
  status: string;
}

/**
 * Response from GET /api/projects/{pid}/datasources/{dsid}/relationships/status
 */
export interface RelationshipWorkflowStatusResponse {
  workflow_id: string;
  phase: RelationshipWorkflowPhase;
  state: RelationshipWorkflowState;
  progress?: WorkflowProgress;
  task_queue?: TaskProgressResponse[];
  confirmed_count: number;
  needs_review_count: number;
  rejected_count: number;
  can_save: boolean;
}

/**
 * Single relationship candidate from the detection workflow
 */
export interface CandidateResponse {
  id: string;
  source_table: string;
  source_column: string;
  target_table: string;
  target_column: string;
  confidence: number;
  detection_method: DetectionMethod;
  llm_reasoning?: string;
  cardinality?: string;
  status: RelationshipCandidateStatus;
  is_required: boolean;
}

/**
 * Response from GET /api/projects/{pid}/datasources/{dsid}/relationships/candidates
 */
export interface CandidatesResponse {
  confirmed: CandidateResponse[];
  needs_review: CandidateResponse[];
  rejected: CandidateResponse[];
}

/**
 * Request body for PUT /api/projects/{pid}/datasources/{dsid}/relationships/candidates/{cid}
 */
export interface CandidateDecisionRequest {
  decision: 'accepted' | 'rejected';
}

/**
 * Response from POST /api/projects/{pid}/datasources/{dsid}/relationships/save
 */
export interface SaveRelationshipsResponse {
  saved_count: number;
}

/**
 * Response from POST /api/projects/{pid}/datasources/{dsid}/relationships/cancel
 */
export interface CancelWorkflowResponse {
  status: string;
}
