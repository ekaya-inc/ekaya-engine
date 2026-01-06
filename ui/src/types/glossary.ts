/**
 * Glossary Types
 * Types for business glossary term management
 */

/**
 * GlossaryFilter represents a condition in a glossary term definition.
 * Example: {"column": "transaction_state", "operator": "=", "values": ["completed"]}
 */
export interface GlossaryFilter {
  column: string;
  operator: string;  // =, IN, >, <, >=, <=, !=, BETWEEN, LIKE, IS NULL, IS NOT NULL
  values: string[];
}

/**
 * BusinessGlossaryTerm represents a business term with its technical mapping.
 * Used for reverse lookup from business term → schema/SQL pattern.
 */
export interface BusinessGlossaryTerm {
  id: string;
  term: string;
  definition: string;
  sql_pattern?: string;
  base_table?: string;
  columns_used?: string[];
  filters?: GlossaryFilter[];
  aggregation?: string;
  source: 'user' | 'suggested' | 'discovered';
  created_at: string;
  updated_at: string;
}

/**
 * GlossaryListResponse for GET /api/projects/{pid}/glossary endpoint.
 */
export interface GlossaryListResponse {
  terms: BusinessGlossaryTerm[];
  total: number;
}
