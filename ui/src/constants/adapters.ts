// Shared adapter metadata for UI components

export interface AdapterInfo {
  name: string;
  icon: string | null;
}

// Provider metadata for PostgreSQL-compatible database services
export interface ProviderInfo {
  id: string;
  name: string;
  icon: string | null;
  adapterType: string; // Backend adapter type (always "postgres" for these)
  defaultPort: number;
  defaultSSL: string;
  urlPattern?: RegExp; // For auto-detection from connection string
  helpUrl?: string; // Link to provider's connection docs
  connectionStringHelp?: string;
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
  supabase: "/icons/adapters/Supabase.png",
  neon: "/icons/adapters/Neon.png",
  cockroachdb: "/icons/adapters/CockroachDB.png",
  yugabytedb: "/icons/adapters/YugabyteDB.png",
  aurora: "/icons/adapters/AuroraPostgreSQL.png",
  alloydb: "/icons/adapters/AlloyDB.png",
  timescale: "/icons/adapters/TimescaleDB.png",
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

// PostgreSQL-compatible database providers
export const POSTGRES_PROVIDERS: ProviderInfo[] = [
  {
    id: "postgres",
    name: "PostgreSQL",
    icon: ADAPTER_ICON_PATHS.postgres ?? null,
    adapterType: "postgres",
    defaultPort: 5432,
    defaultSSL: "require",
    helpUrl: "https://www.postgresql.org/docs/current/libpq-connect.html",
  },
  {
    id: "supabase",
    name: "Supabase",
    icon: ADAPTER_ICON_PATHS.supabase ?? null,
    adapterType: "postgres",
    defaultPort: 6543,
    defaultSSL: "require",
    urlPattern: /\.supabase\.com/i,
    helpUrl: "https://supabase.com/docs/guides/database/connecting-to-postgres",
    connectionStringHelp: "Find in: Project Settings → Database → Connection string",
  },
  {
    id: "neon",
    name: "Neon",
    icon: ADAPTER_ICON_PATHS.neon ?? null,
    adapterType: "postgres",
    defaultPort: 5432,
    defaultSSL: "require",
    urlPattern: /\.neon\.tech/i,
    helpUrl: "https://neon.tech/docs/connect/connect-from-any-app",
    connectionStringHelp: "Find in: Dashboard → Connection Details",
  },
  {
    id: "cockroachdb",
    name: "CockroachDB",
    icon: ADAPTER_ICON_PATHS.cockroachdb ?? null,
    adapterType: "postgres",
    defaultPort: 26257,
    defaultSSL: "verify-full",
    urlPattern: /cockroachlabs\.cloud/i,
    helpUrl: "https://www.cockroachlabs.com/docs/stable/connect-to-the-database.html",
  },
  {
    id: "yugabytedb",
    name: "YugabyteDB",
    icon: ADAPTER_ICON_PATHS.yugabytedb ?? null,
    adapterType: "postgres",
    defaultPort: 5433,
    defaultSSL: "require",
    urlPattern: /\.yugabyte\.cloud/i,
    helpUrl: "https://docs.yugabyte.com/preview/drivers-orms/",
  },
  {
    id: "aurora",
    name: "Amazon Aurora PostgreSQL",
    icon: ADAPTER_ICON_PATHS.aurora ?? null,
    adapterType: "postgres",
    defaultPort: 5432,
    defaultSSL: "require",
    urlPattern: /\.rds\.amazonaws\.com/i,
    helpUrl: "https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/Aurora.Connecting.html",
  },
  {
    id: "alloydb",
    name: "Google AlloyDB",
    icon: ADAPTER_ICON_PATHS.alloydb ?? null,
    adapterType: "postgres",
    defaultPort: 5432,
    defaultSSL: "require",
    helpUrl: "https://cloud.google.com/alloydb/docs/connect-overview",
  },
  {
    id: "timescale",
    name: "TimescaleDB",
    icon: ADAPTER_ICON_PATHS.timescale ?? null,
    adapterType: "postgres",
    defaultPort: 5432,
    defaultSSL: "require",
    urlPattern: /\.timescaledb\.io|tsdb\.cloud\.timescale\.com/i,
    helpUrl: "https://docs.timescale.com/getting-started/latest/",
  },
  {
    id: "redshift",
    name: "Amazon Redshift",
    icon: ADAPTER_ICON_PATHS.redshift ?? null,
    adapterType: "postgres",
    defaultPort: 5439,
    defaultSSL: "require",
    urlPattern: /\.redshift\.amazonaws\.com/i,
    helpUrl: "https://docs.aws.amazon.com/redshift/latest/mgmt/connecting-to-cluster.html",
  },
];

// Helper to find a provider by its ID
export const getProviderById = (id: string): ProviderInfo | undefined =>
  POSTGRES_PROVIDERS.find((p) => p.id === id);

// Helper to auto-detect provider from a connection URL
export const detectProviderFromUrl = (url: string): ProviderInfo | undefined =>
  POSTGRES_PROVIDERS.find((p) => p.urlPattern?.test(url));
