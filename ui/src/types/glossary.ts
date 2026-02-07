/**
 * Glossary Types
 * Types for business glossary term management
 */

import type { OutputColumn } from './query';

/**
 * Source of a glossary term
 */
export type GlossaryTermSource = 'inferred' | 'manual' | 'client';

/**
 * GlossaryTerm represents a business term with its SQL definition.
 * Used for reverse lookup from business term → executable SQL query.
 */
export interface GlossaryTerm {
  id: string;
  project_id: string;
  term: string;
  definition: string;
  defining_sql: string;
  base_table?: string;
  output_columns?: OutputColumn[];
  aliases?: string[];
  source: GlossaryTermSource;
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
}

/**
 * GlossaryGenerationStatus tracks the progress of automated glossary generation.
 */
export interface GlossaryGenerationStatus {
  status: 'idle' | 'discovering' | 'enriching' | 'completed' | 'failed';
  message: string;
  error?: string;
  started_at?: string;
}

/**
 * GlossaryListResponse for GET /api/projects/{pid}/glossary endpoint.
 */
export interface GlossaryListResponse {
  terms: GlossaryTerm[];
  total: number;
  generation_status?: GlossaryGenerationStatus;
}

/**
 * TestSQLResult represents the result of SQL validation.
 * Returned by POST /api/projects/{pid}/glossary/test-sql endpoint.
 */
export interface TestSQLResult {
  valid: boolean;
  error?: string;
  output_columns?: OutputColumn[];
  sample_row?: Record<string, unknown>;
}

/**
 * CreateGlossaryTermRequest for POST /api/projects/{pid}/glossary endpoint.
 */
export interface CreateGlossaryTermRequest {
  term: string;
  definition: string;
  defining_sql: string;
  base_table?: string;
  aliases?: string[];
}

/**
 * UpdateGlossaryTermRequest for PUT /api/projects/{pid}/glossary/{id} endpoint.
 */
export interface UpdateGlossaryTermRequest {
  term?: string;
  definition?: string;
  defining_sql?: string;
  base_table?: string;
  aliases?: string[];
}

/**
 * TestSQLRequest for POST /api/projects/{pid}/glossary/test-sql endpoint.
 */
export interface TestSQLRequest {
  sql: string;
}
