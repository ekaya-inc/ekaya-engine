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

export type MSSQLAuthMethod = 'sql' | 'service_principal' | 'user_delegation';

export interface MSSQLConfig {
  auth_method: MSSQLAuthMethod;
  // Service Principal fields
  tenant_id?: string;
  client_id?: string;
  client_secret?: string;
  // Connection options
  trust_server_certificate?: boolean;
  encrypt?: boolean;
  connection_timeout?: number;
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
  displayName?: string; // User-editable datasource name (separate from config.name which is database name)
  provider?: string; // PostgreSQL provider variant (supabase, neon, etc.)
  decryption_failed?: boolean;
  error_message?: string;
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
