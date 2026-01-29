/**
 * Project Knowledge Types
 * Types for project knowledge (domain facts) management
 */

/**
 * Source of a project knowledge fact
 */
export type ProjectKnowledgeSource = 'inference' | 'mcp' | 'manual';

/**
 * ProjectKnowledge represents a project-level fact learned during refinement.
 * Used for storing business rules, conventions, and domain knowledge.
 */
export interface ProjectKnowledge {
  id: string;
  project_id: string;
  ontology_id?: string;
  fact_type: string;
  key: string;
  value: string;
  context?: string;
  source: ProjectKnowledgeSource;
  last_edit_source?: string;
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
}

/**
 * ProjectKnowledgeListResponse for GET /api/projects/{pid}/project-knowledge endpoint.
 */
export interface ProjectKnowledgeListResponse {
  facts: ProjectKnowledge[];
  total: number;
}

/**
 * CreateProjectKnowledgeRequest for POST /api/projects/{pid}/project-knowledge endpoint.
 */
export interface CreateProjectKnowledgeRequest {
  fact_type: string;
  key: string;
  value: string;
  context?: string;
}

/**
 * UpdateProjectKnowledgeRequest for PUT /api/projects/{pid}/project-knowledge/{id} endpoint.
 */
export interface UpdateProjectKnowledgeRequest {
  fact_type: string;
  key: string;
  value: string;
  context?: string;
}

/**
 * ParseProjectKnowledgeResponse for POST /api/projects/{pid}/project-knowledge/parse endpoint.
 */
export interface ParseProjectKnowledgeResponse {
  facts: ProjectKnowledge[];
}
