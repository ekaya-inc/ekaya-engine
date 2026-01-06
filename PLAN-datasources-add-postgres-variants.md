# PLAN: Add PostgreSQL-Compatible Database Variants

## Overview

Support multiple PostgreSQL-compatible database services (Supabase, Neon, CockroachDB, etc.) in the UI while keeping the backend abstract at the Adapter level. The backend continues to use the single `postgres` adapter with the pgx driver; the frontend handles provider-specific branding, URL parsing, and configuration guidance.

## Design Principles

1. **Backend stays abstract**: One adapter (`postgres`) handles all PostgreSQL wire protocol databases
2. **Frontend handles specialization**: Provider selection, logos, URL parsing, help text
3. **Provider is metadata**: Stored in config JSON, ignored by backend, used by UI for display
4. **User can manually parse**: URL/DSN parsing is a convenience; form fields always work

## Current Architecture

### Backend (No Changes Needed to Core Logic)

```
pkg/adapters/datasource/
├── registry.go           # Adapter registration system
├── factory.go            # Creates adapters by type
├── postgres/
│   ├── config.go         # Config struct: host, port, user, password, database, ssl_mode
│   ├── adapter.go        # Connection tester using pgx
│   ├── schema.go         # Schema discovery
│   ├── query_executor.go # Query execution
│   └── register.go       # Registers "postgres" type
```

The `postgres.Config` struct already supports all needed fields:
```go
type Config struct {
    Host     string
    Port     int
    User     string
    Password string
    Database string
    SSLMode  string // "disable", "require", "verify-ca", "verify-full"
}
```

### Frontend (Changes Here)

```
ui/src/
├── constants/adapters.ts           # Adapter metadata (icons, names)
├── components/
│   ├── DatasourceAdapterSelection.tsx  # Adapter type picker
│   └── DatasourceConfiguration.tsx     # Connection form
```

## Proposed Design

### Provider Concept

Introduce "provider" as a UI concept that represents the specific service offering a PostgreSQL-compatible database:

| Provider | Adapter Type | Default Port | SSL Default | Notes |
|----------|--------------|--------------|-------------|-------|
| `postgres` | postgres | 5432 | require | Self-hosted PostgreSQL |
| `supabase` | postgres | 6543 (pooler) / 5432 (direct) | require | Pooler vs direct connection |
| `neon` | postgres | 5432 | require | Serverless, always pooled |
| `cockroachdb` | postgres | 26257 | verify-full | Distributed SQL |
| `yugabytedb` | postgres | 5433 | require | Distributed SQL |
| `aurora` | postgres | 5432 | require | AWS managed |
| `alloydb` | postgres | 5432 | require | Google managed |
| `timescale` | postgres | 5432 | require | Time-series extension |
| `redshift` | postgres | 5439 | require | Data warehouse (partial compat) |

### Config Schema Extension

Store provider in config JSON (backend ignores, UI uses):

```json
{
  "provider": "supabase",
  "host": "aws-0-us-west-1.pooler.supabase.com",
  "port": 6543,
  "user": "postgres.abcdefgh",
  "password": "...",
  "database": "postgres",
  "ssl_mode": "require"
}
```

The `provider` field is:
- Optional (defaults to `postgres` if missing)
- Ignored by backend `postgres.FromMap()` (unknown fields are silently ignored)
- Used by frontend for display (logo, name, help text)

### URL Parsing

PostgreSQL connection URL format:
```
postgresql://[user[:password]@][host][:port][/database][?param=value&...]
```

Provider-specific examples:

**Supabase:**
```
postgresql://postgres.[project-ref]:[password]@aws-0-[region].pooler.supabase.com:6543/postgres
```

**Neon:**
```
postgresql://[user]:[password]@ep-[endpoint].us-east-1.aws.neon.tech/[database]?sslmode=require
```

**CockroachDB Cloud:**
```
postgresql://[user]:[password]@[cluster].[region].cockroachlabs.cloud:26257/[database]?sslmode=verify-full
```

The UI should parse these into individual fields and auto-detect provider from hostname patterns.

## Implementation

### Phase 1: Frontend Provider Support

#### 1.1 Extend Adapter Constants [x] COMPLETE

**File:** `ui/src/constants/adapters.ts`

**What was implemented:**
- Added `ProviderInfo` interface with all specified fields
- Added `POSTGRES_PROVIDERS` array with 9 providers (postgres, supabase, neon, cockroachdb, yugabytedb, aurora, alloydb, timescale, redshift)
- Added icon paths to `ADAPTER_ICON_PATHS` for all new providers
- Added `getProviderById()` and `detectProviderFromUrl()` helper functions
- Added URL patterns for auto-detection (Supabase, Neon, CockroachDB, YugabyteDB, Aurora, Timescale, Redshift)

**Additional URL patterns added (not in original plan):**
- YugabyteDB: `.yugabyte.cloud`
- Aurora: `.rds.amazonaws.com` (covers Aurora PostgreSQL on RDS)
- Timescale: `.timescaledb.io` and `tsdb.cloud.timescale.com`
- Redshift: `.redshift.amazonaws.com`

**Tests:** `ui/src/constants/adapters.test.ts` - 23 tests covering:
- Provider presence and count validation
- Default port correctness per provider
- SSL mode validation (cockroachdb uses verify-full, others use require)
- `getProviderById()` lookups
- `detectProviderFromUrl()` for all providers with URL patterns
- Case-insensitive hostname matching

**Note:** Icons are not yet added to `ui/public/icons/adapters/` - this is covered in Phase 2.

```typescript
export interface ProviderInfo {
  id: string;
  name: string;
  icon: string | null;
  adapterType: string;        // Backend adapter type (always "postgres" for these)
  defaultPort: number;
  defaultSSL: string;
  urlPattern?: RegExp;        // For auto-detection from connection string
  helpUrl?: string;           // Link to provider's connection docs
  connectionStringHelp?: string;
}

export const POSTGRES_PROVIDERS: ProviderInfo[] = [
  {
    id: "postgres",
    name: "PostgreSQL",
    icon: "/icons/adapters/PostgreSQL.png",
    adapterType: "postgres",
    defaultPort: 5432,
    defaultSSL: "require",
    helpUrl: "https://www.postgresql.org/docs/current/libpq-connect.html",
  },
  {
    id: "supabase",
    name: "Supabase",
    icon: "/icons/adapters/Supabase.png",
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
    icon: "/icons/adapters/Neon.png",
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
    icon: "/icons/adapters/CockroachDB.png",
    adapterType: "postgres",
    defaultPort: 26257,
    defaultSSL: "verify-full",
    urlPattern: /cockroachlabs\.cloud/i,
    helpUrl: "https://www.cockroachlabs.com/docs/stable/connect-to-the-database.html",
  },
  {
    id: "yugabytedb",
    name: "YugabyteDB",
    icon: "/icons/adapters/YugabyteDB.png",
    adapterType: "postgres",
    defaultPort: 5433,
    defaultSSL: "require",
  },
  {
    id: "aurora",
    name: "Amazon Aurora PostgreSQL",
    icon: "/icons/adapters/AuroraPostgreSQL.png",
    adapterType: "postgres",
    defaultPort: 5432,
    defaultSSL: "require",
  },
  {
    id: "alloydb",
    name: "Google AlloyDB",
    icon: "/icons/adapters/AlloyDB.png",
    adapterType: "postgres",
    defaultPort: 5432,
    defaultSSL: "require",
  },
  {
    id: "timescale",
    name: "TimescaleDB",
    icon: "/icons/adapters/TimescaleDB.png",
    adapterType: "postgres",
    defaultPort: 5432,
    defaultSSL: "require",
  },
  {
    id: "redshift",
    name: "Amazon Redshift",
    icon: "/icons/adapters/AmazonRedshift.png",
    adapterType: "postgres",
    defaultPort: 5439,
    defaultSSL: "require",
  },
];

export const getProviderById = (id: string): ProviderInfo | undefined =>
  POSTGRES_PROVIDERS.find(p => p.id === id);

export const detectProviderFromUrl = (url: string): ProviderInfo | undefined =>
  POSTGRES_PROVIDERS.find(p => p.urlPattern?.test(url));
```

#### 1.2 URL Parser Utility [x] COMPLETE

**File:** `ui/src/utils/connectionString.ts`

**What was implemented:**
- Created `ParsedConnectionString` interface with all required fields (host, port, user, password, database, sslMode, provider)
- Implemented `parsePostgresUrl()` function that handles both `postgresql://` and `postgres://` schemes
- Extracts and URL-decodes user/password from URL
- Parses `sslmode` from query parameters, defaults to "require"
- Auto-detects provider from hostname using `detectProviderFromUrl()` and applies provider-specific default port when port is not explicit in URL

**Tests:** `ui/src/utils/connectionString.test.ts` - 22 tests covering:
- Standard URL parsing (postgresql:// and postgres:// schemes)
- SSL mode extraction from query parameters
- Default port handling (uses provider default when port not specified)
- URL without port, database, user/password
- URL-encoded characters in user/password
- Invalid URL rejection
- Provider auto-detection for all providers (Supabase, Neon, CockroachDB, YugabyteDB, Aurora, TimescaleDB, Redshift)
- Edge cases (complex usernames, multiple query parameters, empty password)

```typescript
export interface ParsedConnectionString {
  host: string;
  port: number;
  user: string;
  password: string;
  database: string;
  sslMode: string;
  provider: string | undefined;  // Auto-detected from URL
}

export function parsePostgresUrl(url: string): ParsedConnectionString | null {
  // Handle both postgresql:// and postgres:// schemes
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
```

#### 1.3 Update Adapter Selection UI [x] COMPLETE

**File:** `ui/src/components/DatasourceAdapterSelection.tsx`

**What was implemented:**
- Reorganized adapter selection into two sections: "PostgreSQL-Compatible" and "Other Databases"
- PostgreSQL-Compatible section displays all 9 providers from `POSTGRES_PROVIDERS` (PostgreSQL, Supabase, Neon, CockroachDB, YugabyteDB, Aurora, AlloyDB, TimescaleDB, Redshift)
- Other Databases section displays non-postgres adapters from the API (MySQL, ClickHouse, etc.)
- When a PostgreSQL provider is selected, passes both adapter type (`postgres`) and provider info to configuration
- Provider cards show name and connection string help text where available (Supabase, Neon have help text)
- Updated page title to "Select Your Database" with subtitle "Choose your database provider to get started"
- Updated `onAdapterSelect` callback signature to include optional `ProviderInfo` parameter

**Additional files modified:**
- `ui/src/pages/DatasourcePage.tsx` - Updated to:
  - Track `selectedProvider` state alongside `selectedAdapter`
  - Pass `selectedProvider` to `DatasourceConfiguration` component
  - Reset provider when returning to selection
- `ui/src/components/DatasourceConfiguration.tsx` - Updated to:
  - Accept `selectedProvider` prop (optional `ProviderInfo`)
  - Use `displayInfo` (provider or adapter fallback) for icon and name in header
  - Initialize form with provider-specific defaults:
    - Default port (e.g., 6543 for Supabase, 26257 for CockroachDB, 5433 for YugabyteDB)
    - Default SSL mode (require for most, verify-full for CockroachDB)
  - Use provider name as default datasource display name

**Tests:** `ui/src/components/__tests__/DatasourceAdapterSelection.test.tsx` - 14 tests covering:
- Loading and error states
- PostgreSQL-Compatible section rendering with all providers
- Other Databases section rendering
- Provider selection callbacks with provider info (verifies adapter type "postgres" + provider object)
- Non-postgres adapter selection callbacks (verifies just adapter ID)
- Section visibility based on available adapters (no PostgreSQL section if API doesn't return postgres)
- Disabled state handling with existing datasources (only matching adapter type is clickable)
- Navigation (back button) and UI elements (title, subtitle, help text)

#### 1.4 Update Configuration Form [x] COMPLETE

**File:** `ui/src/components/DatasourceConfiguration.tsx`

**What was implemented:**
1. Connection string paste field with "Parse" button at top of form
   - Only shows for postgres adapters (not when editing existing)
   - Parses `postgresql://` and `postgres://` URLs
   - Auto-fills form fields from parsed values
   - Auto-detects provider from URL hostname and updates display
   - Shows error messages for invalid URLs
2. Provider-specific help text and documentation links
   - Shows `connectionStringHelp` from provider (e.g., "Find in: Project Settings → Database → Connection string" for Supabase)
   - Shows "View documentation" link with external link icon when `helpUrl` is available
3. Auto-fill port based on provider (was already partially implemented in 1.3)
   - Default port comes from `selectedProvider.defaultPort`
4. Store provider in config when saving
   - Added `provider` field to `apiConfig` object
   - Uses `activeProvider` (can be updated by connection string parsing) or falls back to `selectedProvider`

**Additional changes:**
- Added `provider?: string` to `ConnectionDetails` interface in `ui/src/types/datasource.ts`
- Added `activeProvider` state to track provider from selection or connection string parsing
- Added effects to sync provider from `selectedProvider` prop and load from existing config

**Tests:** `ui/src/components/__tests__/DatasourceConfiguration.test.tsx` - 21 tests covering:
- Connection string parser rendering (shows for postgres, hides for other adapters)
- Parsing valid connection strings into form fields
- Error handling for invalid/empty connection strings
- Auto-detection of providers (Supabase, Neon) from URLs
- Provider-specific default ports (Supabase 6543, CockroachDB 26257, YugabyteDB 5433)
- Provider-specific SSL defaults
- Provider help text and documentation links display
- Provider persistence in saved config
- Page header showing correct provider name
- Connection string parser hidden when editing existing datasource
- Loading provider from existing config when editing

### Phase 2: Provider Icons [x] COMPLETE

Add icon files for each provider:

```
ui/public/icons/adapters/
├── PostgreSQL.png      # Existing
├── Supabase.png        # NEW
├── Neon.png            # NEW
├── CockroachDB.png     # NEW
├── YugabyteDB.png      # NEW
├── AuroraPostgreSQL.png # NEW
├── AlloyDB.png         # NEW
├── TimescaleDB.png     # NEW
└── AmazonRedshift.png  # Existing
```

### Phase 3: Datasource List Display

Update datasource list to show provider icon/name instead of generic "PostgreSQL":

**File:** `ui/src/components/DatasourceCard.tsx` (or equivalent)

```typescript
// Read provider from config, fall back to adapter type
const provider = datasource.config?.provider || datasource.type;
const providerInfo = getProviderById(provider) || getAdapterInfo(datasource.type);
```

## Backend Considerations

### No Changes Required

The backend `postgres` adapter already works with all PostgreSQL-compatible databases because:

1. `postgres.FromMap()` ignores unknown config fields (like `provider`)
2. All providers use standard PostgreSQL connection parameters
3. The pgx driver handles protocol differences transparently

### Optional Future Enhancements

If needed later, we could add:

1. **Provider-specific validation**: Warn if CockroachDB is on port 5432 (unusual)
2. **Provider-specific error messages**: "Supabase requires pooler connection" hints
3. **Connection metadata**: Track provider in telemetry for support diagnostics

These would require backend changes but are not needed for MVP.

## Testing

### Frontend Unit Tests

1. URL parsing for each provider format
2. Provider auto-detection from URL
3. Default port/SSL selection by provider

### Manual Testing

1. Connect to Supabase using connection string paste
2. Connect to Neon using connection string paste
3. Connect to self-hosted PostgreSQL with manual fields
4. Verify provider icon displays in datasource list
5. Edit existing datasource, verify provider preserved

## Migration

Existing datasources without `provider` field:
- UI defaults to showing PostgreSQL icon/name
- No data migration needed
- Users can edit to set provider if desired

## Files to Create/Modify

### New Files
- `ui/src/utils/connectionString.ts` - URL parser
- `ui/public/icons/adapters/Supabase.png`
- `ui/public/icons/adapters/Neon.png`
- `ui/public/icons/adapters/CockroachDB.png`
- `ui/public/icons/adapters/YugabyteDB.png`
- `ui/public/icons/adapters/AuroraPostgreSQL.png`
- `ui/public/icons/adapters/AlloyDB.png`
- `ui/public/icons/adapters/TimescaleDB.png`

### Modified Files
- `ui/src/constants/adapters.ts` - Add provider definitions
- `ui/src/components/DatasourceAdapterSelection.tsx` - Provider selection UI
- `ui/src/components/DatasourceConfiguration.tsx` - Connection string parsing, provider storage
- `ui/src/types/index.ts` - Add provider types

### No Backend Changes
- `pkg/adapters/datasource/postgres/*` - Works as-is
