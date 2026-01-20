/**
 * SDAP (Structured Data Access Protocol) Types
 * API types for the SDAP REST API endpoints
 */

import type { DatasourceType, DatasourceConfig } from './datasource';

/**
 * Base datasource interface representing a database connection
 */
export interface Datasource {
  datasource_id: string;
  project_id: string;
  name: string;
  type: DatasourceType;
  provider?: string;
  config: DatasourceConfig;
  created_at: string;
  updated_at: string;
  decryption_failed?: boolean;
  error_message?: string;
}

/**
 * Input data for creating or updating a datasource
 */
export interface DatasourceInput {
  project_id: string;
  name: string;
  type: DatasourceType;
  provider?: string;
  config: DatasourceConfig;
}

export type CreateDatasourceRequest = DatasourceInput;
export type UpdateDatasourceRequest = DatasourceInput;

export type CreateDatasourceResponse = Datasource;
export type GetDatasourceResponse = Datasource;
export type UpdateDatasourceResponse = Omit<Datasource, 'created_at'>;

export interface ListDatasourcesResponse {
  datasources: Datasource[];
}

export interface DeleteDatasourceResponse {
  success: boolean;
  message: string;
}

/**
 * Schema refresh request
 */
export interface SchemaRefreshRequest {
  datasource_id: string;
}

/**
 * Schema refresh result returned from the API
 */
export interface SchemaRefreshResult {
  tables_upserted: number;
  tables_deleted: number;
  columns_upserted: number;
  columns_deleted: number;
  relationships_created: number;
}

/**
 * Schema refresh response
 */
export interface SchemaRefreshResponse {
  datasource_id: string;
  project_id: string;
  schema_refresh: SchemaRefreshResult;
}
