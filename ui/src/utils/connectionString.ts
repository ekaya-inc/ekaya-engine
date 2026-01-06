import { detectProviderFromUrl } from '../constants/adapters';

/**
 * Parsed PostgreSQL connection string components.
 */
export interface ParsedConnectionString {
  host: string;
  port: number;
  user: string;
  password: string;
  database: string;
  sslMode: string;
  provider: string | undefined; // Auto-detected from URL
}

/**
 * Parse a PostgreSQL connection URL into its components.
 * Handles both postgresql:// and postgres:// schemes.
 *
 * @example
 * // Standard PostgreSQL
 * parsePostgresUrl("postgresql://user:pass@localhost:5432/mydb")
 *
 * @example
 * // Supabase (pooler)
 * parsePostgresUrl("postgresql://postgres.abcdefgh:pass@aws-0-us-west-1.pooler.supabase.com:6543/postgres")
 *
 * @example
 * // Neon with SSL mode
 * parsePostgresUrl("postgresql://user:pass@ep-cool-darkness-123456.us-east-1.aws.neon.tech/neondb?sslmode=require")
 *
 * @param url - PostgreSQL connection URL
 * @returns Parsed connection components or null if URL is invalid
 */
export function parsePostgresUrl(url: string): ParsedConnectionString | null {
  // Handle both postgresql:// and postgres:// schemes
  // URL format: postgres[ql]://[user[:password]@]host[:port][/database][?query]
  const match = url.match(
    /^postgres(?:ql)?:\/\/(?:([^:@]+)(?::([^@]*))?@)?([^:/]+)(?::(\d+))?(?:\/([^?]+))?(?:\?(.*))?$/
  );

  if (!match) return null;

  const [, user, password, host, port, database, queryString] = match;

  // Parse query parameters for sslmode
  const params = new URLSearchParams(queryString ?? '');
  const sslMode = params.get('sslmode') ?? 'require';

  // Auto-detect provider from hostname
  const detectedProvider = detectProviderFromUrl(url);

  return {
    host: host ?? '',
    port: port ? parseInt(port, 10) : (detectedProvider?.defaultPort ?? 5432),
    user: decodeURIComponent(user ?? ''),
    password: decodeURIComponent(password ?? ''),
    database: database ?? '',
    sslMode,
    provider: detectedProvider?.id,
  };
}
