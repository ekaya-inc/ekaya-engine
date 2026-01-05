/**
 * Datasource Schema Types
 * Type definitions for datasource schema retrieval and representation
 */

/**
 * Column information from database schema
 */
export interface SchemaColumn {
  id?: string;
  column_name: string;
  data_type: string;
  is_nullable?: string | boolean;
  is_primary_key?: boolean;
  is_selected?: boolean;
  column_default?: string | null;
  character_maximum_length?: number | null;
  numeric_precision?: number | null;
  numeric_scale?: number | null;
  ordinal_position?: number;
  business_name?: string;
  description?: string;
  distinct_count?: number | null;
  null_count?: number | null;
  [key: string]: unknown; // Allow additional database-specific fields
}

/**
 * Table schema information
 */
export interface SchemaTable {
  id?: string;
  schema_name?: string;
  table_name: string;
  columns: SchemaColumn[];
  row_count?: number; // Optional, may not always be included
  is_selected?: boolean;
  business_name?: string;
  description?: string;
}

/**
 * Relationship between tables (foreign keys)
 */
export interface SchemaRelationship {
  constraint_name: string;
  table_name: string;
  column_name: string;
  foreign_table_name: string;
  foreign_column_name: string;
  [key: string]: unknown; // Allow additional database-specific fields
}

/**
 * Complete datasource schema response from /sdap/v1/{project_id}/schema
 */
export interface DatasourceSchema {
  tables: SchemaTable[];
  total_tables: number;
  relationships: SchemaRelationship[];
  error?: string; // Optional error message if schema retrieval partially failed
}

/**
 * Response from /sdap/v1/{project_id}/schema/selections
 * Contains saved table and column selection preferences for a project
 */
export interface SelectedTablesResponse {
  selected_tables: string[] | null; // null if no selections saved
  selected_columns: Record<string, string[]> | null; // null if no column selections saved
}

/**
 * Request body for POST /sdap/v1/{project_id}/schema/selections
 * Saves table and column selection preferences
 */
export interface SaveSelectionsRequest {
  selected_tables: string[];
  selected_columns: Record<string, string[]>;
}

/**
 * Response from POST /sdap/v1/{project_id}/schema/selections
 * Confirms successful save of schema selections
 */
export interface SaveSelectionsResponse {
  selected_tables_count: number;
  selected_columns_count: number;
}

/**
 * Relationship type constants
 */
export type RelationshipType = 'fk' | 'inferred' | 'manual';

/**
 * Cardinality type constants
 */
export type Cardinality = '1:1' | '1:N' | 'N:1' | 'N:M';

/**
 * Detailed relationship information from /sdap/v1/{project_id}/schema/relationships
 */
export interface RelationshipDetail {
  id: string;
  source_table_name: string;
  source_column_name: string;
  source_column_type: string;
  target_table_name: string;
  target_column_name: string;
  target_column_type: string;
  relationship_type: RelationshipType;
  cardinality: Cardinality | null;
  confidence: number;
  inference_method: string | null;
  is_validated: boolean;
  is_approved: boolean | null; // null = pending, true = approved, false = rejected
  description?: string;
  created_at: string;
  updated_at: string;
}

/**
 * Response from GET /sdap/v1/{project_id}/schema/relationships
 */
export interface RelationshipsResponse {
  relationships: RelationshipDetail[];
  total_count: number;
  empty_tables?: string[];   // Tables with 0 rows
  orphan_tables?: string[];  // Non-empty tables with no relationships
}

/**
 * Request body for POST /sdap/v1/{project_id}/schema/relationships
 * Creates a manual relationship between two columns
 */
export interface CreateRelationshipRequest {
  source_table: string;
  source_column: string;
  target_table: string;
  target_column: string;
}

// --- Relationship Discovery Types ---

/**
 * Results from relationship discovery
 * Response from POST /sdap/v1/{project_id}/schema/relationships/discover
 */
export interface DiscoveryResults {
  relationships_created: number;
  tables_analyzed: number;
  columns_analyzed: number;
  tables_without_relationships: number;
  empty_tables: number;
  empty_table_names?: string[];
  orphan_table_names?: string[];
}

/**
 * Status of a relationship candidate
 */
export type CandidateStatus = 'candidate' | 'verified' | 'rejected';

/**
 * Reason why a candidate was rejected
 */
export type RejectionReason =
  | 'low_match_rate'
  | 'high_orphan_rate'
  | 'join_failed'
  | 'type_mismatch'
  | 'already_exists';

/**
 * A relationship candidate (verified or rejected)
 */
export interface RelationshipCandidate {
  id: string;
  source_table: string;
  source_column: string;
  target_table: string;
  target_column: string;
  match_rate: number;
  status: CandidateStatus;
  rejection_reason?: RejectionReason;
}

/**
 * Summary of candidates
 */
export interface CandidatesSummary {
  total: number;
  verified: number;
  rejected: number;
  pending: number;
}

/**
 * Response from GET /sdap/v1/{project_id}/schema/relationships/candidates
 */
export interface RelationshipCandidatesResponse {
  candidates: RelationshipCandidate[];
  summary: CandidatesSummary;
}
