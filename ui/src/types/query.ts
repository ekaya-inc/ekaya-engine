/**
 * Query Types
 * Types for the query management and execution system
 */

import type { DatasourceType } from './datasource';

/**
 * SQL dialect types supported by CodeMirror
 */
export type SqlDialect = 'PostgreSQL' | 'MySQL' | 'SQLite' | 'MSSQL';

/**
 * Parameter types supported for parameterized queries
 */
export type ParameterType =
  | 'string'
  | 'integer'
  | 'decimal'
  | 'boolean'
  | 'date'
  | 'timestamp'
  | 'uuid'
  | 'string[]'
  | 'integer[]';

/**
 * Query parameter definition
 */
export interface QueryParameter {
  name: string;
  type: ParameterType;
  description: string;
  required: boolean;
  default: unknown | null;
}

/**
 * Output column definition describing a column returned by the query
 */
export interface OutputColumn {
  name: string;
  type: string;
  description: string;
}

/**
 * Maps datasource types to CodeMirror SQL dialects
 */
export const datasourceTypeToDialect: Record<DatasourceType, SqlDialect> = {
  postgres: 'PostgreSQL',
  mysql: 'MySQL',
  sqlite: 'SQLite',
  mssql: 'MSSQL',
  clickhouse: 'PostgreSQL', // Uses PostgreSQL-like syntax
  snowflake: 'PostgreSQL', // Uses PostgreSQL-like syntax
  bigquery: 'PostgreSQL', // Uses PostgreSQL-like syntax
  databricks: 'PostgreSQL', // Uses PostgreSQL-like syntax
  redshift: 'PostgreSQL', // Uses PostgreSQL-like syntax
};

/**
 * Query model matching backend QueryResponse
 */
export interface Query {
  query_id: string;
  project_id: string;
  datasource_id: string;
  natural_language_prompt: string;
  additional_context: string | null;
  sql_query: string;
  dialect: string;
  is_enabled: boolean;
  allows_modification: boolean;
  usage_count: number;
  last_used_at: string | null;
  created_at: string;
  updated_at: string;
  parameters: QueryParameter[];
  output_columns?: OutputColumn[];
  constraints?: string | null;
}

/**
 * Request to create a new query
 * Note: Dialect is derived from datasource type by the backend
 */
export interface CreateQueryRequest {
  natural_language_prompt: string;
  additional_context?: string;
  sql_query: string;
  is_enabled: boolean;
  allows_modification?: boolean;
  parameters?: QueryParameter[];
  output_columns?: OutputColumn[];
  constraints?: string;
}

/**
 * Request to update an existing query
 * All fields are optional - only provided fields are updated
 * Note: Dialect cannot be updated - it's derived from datasource type
 */
export interface UpdateQueryRequest {
  natural_language_prompt?: string;
  additional_context?: string | undefined;
  sql_query?: string;
  is_enabled?: boolean;
  allows_modification?: boolean;
  parameters?: QueryParameter[];
  output_columns?: OutputColumn[];
  constraints?: string;
}

/**
 * Request to execute a saved query
 */
export interface ExecuteQueryRequest {
  limit?: number;
  parameters?: Record<string, unknown>;
}

/**
 * Request to test a SQL query without saving
 */
export interface TestQueryRequest {
  sql_query: string;
  limit?: number;
  parameter_definitions?: QueryParameter[];
  parameter_values?: Record<string, unknown>;
}

/**
 * Request to validate SQL syntax
 */
export interface ValidateQueryRequest {
  sql_query: string;
}

/**
 * Column information from query execution
 */
export interface ColumnInfo {
  name: string;
  type: string;
}

/**
 * Response from query execution (execute or test)
 */
export interface ExecuteQueryResponse {
  columns: ColumnInfo[];
  rows: Record<string, unknown>[];
  row_count: number;
}

/**
 * Response from SQL validation
 */
export interface ValidateQueryResponse {
  valid: boolean;
  message?: string;
  warnings?: string[];
}

/**
 * Response wrapper for list queries endpoint
 */
export interface ListQueriesResponse {
  queries: Query[];
}

/**
 * Response from delete query endpoint
 */
export interface DeleteQueryResponse {
  success: boolean;
  message: string;
}
