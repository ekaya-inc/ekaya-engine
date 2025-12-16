// Shared adapter metadata for UI components

export interface AdapterInfo {
  name: string;
  icon: string | null;
}

// Map adapter type identifier to icon path
export const ADAPTER_ICON_PATHS: Record<string, string> = {
  postgres: "/icons/adapters/PostgreSQL.png",
  mssql: "/icons/adapters/MSSQL.png",
  clickhouse: "/icons/adapters/ClickHouse.png",
  mysql: "/icons/adapters/MySQL.png",
  snowflake: "/icons/adapters/Snowflake.png",
  bigquery: "/icons/adapters/BigQuery.png",
  databricks: "/icons/adapters/Databricks.png",
  redshift: "/icons/adapters/AmazonRedshift.png",
};

// Map adapter type identifier to display info
const ADAPTER_INFO: Record<string, AdapterInfo> = {
  postgres: { name: "PostgreSQL", icon: ADAPTER_ICON_PATHS.postgres ?? null },
  mssql: { name: "Microsoft SQL Server", icon: ADAPTER_ICON_PATHS.mssql ?? null },
  clickhouse: { name: "ClickHouse", icon: ADAPTER_ICON_PATHS.clickhouse ?? null },
  mysql: { name: "MySQL", icon: ADAPTER_ICON_PATHS.mysql ?? null },
  snowflake: { name: "Snowflake", icon: ADAPTER_ICON_PATHS.snowflake ?? null },
  bigquery: { name: "Google BigQuery", icon: ADAPTER_ICON_PATHS.bigquery ?? null },
  databricks: { name: "Databricks", icon: ADAPTER_ICON_PATHS.databricks ?? null },
  redshift: { name: "Amazon Redshift", icon: ADAPTER_ICON_PATHS.redshift ?? null },
};

const DEFAULT_ADAPTER_INFO: AdapterInfo = { name: "Datasource", icon: null };

export const getAdapterInfo = (adapterId?: string): AdapterInfo => {
  if (!adapterId) return DEFAULT_ADAPTER_INFO;
  return ADAPTER_INFO[adapterId] ?? DEFAULT_ADAPTER_INFO;
};
