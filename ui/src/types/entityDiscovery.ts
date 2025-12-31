/**
 * Entity Discovery types for the standalone entity discovery workflow.
 */

/**
 * Progress information for the discovery workflow.
 */
export interface EntityDiscoveryProgress {
  current_phase: string;
  current: number;
  total: number;
  message: string;
}

/**
 * A task in the entity discovery workflow queue.
 */
export interface EntityDiscoveryTask {
  id: string;
  name: string;
  status: "queued" | "processing" | "complete" | "failed" | "paused";
  requires_llm: boolean;
  error?: string;
  retry_count?: number;
}

/**
 * Status response from GET /entities/status endpoint.
 */
export interface EntityDiscoveryStatus {
  workflow_id: string;
  phase: string;
  state: "pending" | "running" | "completed" | "failed";
  progress?: EntityDiscoveryProgress;
  task_queue?: EntityDiscoveryTask[];
  entity_count: number;
  occurrence_count: number;
}

/**
 * Response from POST /entities/discover endpoint.
 */
export interface StartEntityDiscoveryResponse {
  workflow_id: string;
  status: string;
}
