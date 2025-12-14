/**
 * Datasource Connection Types
 * Used by DatasourceConnectionContext and related components
 */

export type DatasourceType =
  | 'postgres'
  | 'mysql'
  | 'clickhouse'
  | 'mssql'
  | 'snowflake'
  | 'bigquery'
  | 'databricks'
  | 'redshift'
  | 'sqlite';

export type SSLMode = 'disable' | 'allow' | 'prefer' | 'require' | 'verify-ca' | 'verify-full';

export type MSSQLAuthMethod = 'sql' | 'windows' | 'azuread' | 'azuread_password';

export interface MSSQLConfig {
  auth_method: MSSQLAuthMethod;
  instance?: string;
  domain?: string;
  azure_tenant_id?: string;
  azure_client_id?: string;
  azure_client_secret?: string;
  application_client_id?: string;
  trust_server_certificate?: boolean;
  encrypt?: string;
  connection_timeout?: number;
  application_name?: string;
}

export interface DatasourceConfig {
  host: string;
  port: number;
  user?: string;
  password?: string;
  name: string;
  ssl_mode: SSLMode;
  extra?: Record<string, unknown>;
}

export interface ConnectionDetails extends DatasourceConfig {
  datasourceId?: string;
  projectId?: string;
  type: DatasourceType;
}

export interface ConnectionStatus {
  success: boolean;
  message: string;
  timestamp: string;
}

export interface TestConnectionRequest extends DatasourceConfig {
  type: DatasourceType;
}

export interface TestConnectionResponse {
  success: boolean;
  message: string;
}
