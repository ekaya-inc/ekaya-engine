# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is **ekaya-engine** - a clean architecture rebuild of the Ekaya regional controller. The project was initialized from ekaya-region's infrastructure but with a minimal Go backend shell, designed for incremental, well-architected development.

**Design Philosophy:**
- Clean architecture with separation of concerns
- Dependency injection for testability
- Controllers (thin HTTP handlers) → Services (business logic) → Repositories (data access)
- Fail-fast error handling

## Current State

The project has full operational infrastructure with a complete Go backend:
- **Infrastructure:** Build system, CI/CD, Docker, frontend (React/TypeScript/Vite)
- **Backend:** Full ontology extraction DAG, MCP server, schema discovery, relationship inference
- **MCP Tools:** Developer tools for database context, queries, and ontology management

## Greenfield Project: No Backward Compatibility

**This is a pre-launch project with no users and no external developers.** Assume:

- **No backward compatibility required** - APIs, schemas, and interfaces can change freely
- **No data migrations needed** - The database can be dropped and recreated at will
- **No legacy code concerns** - Delete and rewrite code without preserving old behavior
- **No deprecation periods** - Remove features immediately without warnings or shims

When making changes, prefer clean solutions over compatibility hacks. If a refactor requires dropping the database, that's acceptable.

## Critical: Server Management

**NEVER kill or start the dev server from Claude Code.** The user runs `make dev-server` and `make dev-ui` in separate terminals. Do not use `pkill`, `nohup`, or any command that would start/stop/restart these servers. If changes require a server restart, inform the user to restart it manually.

## Development Commands

### Essential Commands

```bash
# Development mode - run UI and API separately for hot reload
make dev-ui         # Terminal 1: UI dev server (http://localhost:5173)
make dev-server     # Terminal 2: Go API with auto-reload (port 3443)

# Run locally (builds UI and starts server on port 3443)
make run

# Build Docker image locally for testing (same as CI/CD)
make dev-build-docker

# Run all checks (requires Docker for integration tests)
make check          # Format, lint, typecheck, unit + integration tests (backend + frontend)

# Format code
go fmt ./...

# Lint code (requires golangci-lint)
golangci-lint run
```

### Cloud Run Deployment

**Deployment is automated via GitHub Actions:**
- Push to `main` branch → Automatic deployment to DEV environment
- Push to `prod` branch → Automatic deployment to PROD environment

### Building Scripts

Scripts in `./scripts/` should be built to `bin/` (which is gitignored):
```bash
# Build a script
go build -o bin/assess-deterministic ./scripts/assess-deterministic/...

# Run a script (uses go run, no binary left behind)
./scripts/assess-deterministic.sh <project-id>
```

## Architecture Patterns

### Package Structure

```
pkg/
├── config/       # Configuration loading (cleanenv)
├── server/       # HTTP server setup, middleware
├── handlers/     # HTTP handlers (thin, delegate to services)
├── services/     # Business logic
├── models/       # Domain types
└── repositories/ # Data access layer
```

### Key Principles

1. **Handlers are thin** - Parse request, call service, format response
2. **Services contain logic** - Business rules, orchestration
3. **Repositories are data-only** - SQL queries, external API calls
4. **Dependency injection** - Services accept interfaces, not concrete types
5. **Fail fast** - Return errors immediately, don't continue on failure

### Example Pattern

```go
// handlers/user_handler.go
type UserHandler struct {
    userService services.UserService
}

func NewUserHandler(us services.UserService) *UserHandler {
    return &UserHandler{userService: us}
}

func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]
    user, err := h.userService.GetByID(r.Context(), id)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    json.NewEncoder(w).Encode(user)
}

// services/user_service.go
type UserService interface {
    GetByID(ctx context.Context, id string) (*models.User, error)
}

type userService struct {
    repo repositories.UserRepository
}

func NewUserService(repo repositories.UserRepository) UserService {
    return &userService{repo: repo}
}

func (s *userService) GetByID(ctx context.Context, id string) (*models.User, error) {
    return s.repo.FindByID(ctx, id)
}
```

## Critical Architecture Rules

These rules prevent common architectural violations. See `plans/ISSUE-*.md` and `plans/FIX-*.md` for known violations being addressed.

### 1. Handlers Must Not Import Repositories

Handlers must only depend on services, never repositories. This maintains the layering: **Handlers → Services → Repositories**.

```go
// WRONG - handler importing repository
import "github.com/ekaya-inc/ekaya-engine/pkg/repositories"

type BadHandler struct {
    userRepo repositories.UserRepository  // VIOLATION
}

// CORRECT - handler depends only on services
type GoodHandler struct {
    userService services.UserService
}
```

If a handler needs data, add a method to the appropriate service.

### 2. Services Must Not Contain SQL

Services must access the engine database through repositories, never with raw SQL. This maintains the layering: **Handlers → Services → Repositories**.

```go
// WRONG - service executing raw SQL
func (s *myService) Delete(ctx context.Context, projectID uuid.UUID) error {
    scope, _ := database.GetTenantScope(ctx)
    _, err := scope.Conn.Exec(ctx, "DELETE FROM engine_foo WHERE project_id = $1", projectID)
    return err
}

// CORRECT - service delegates to repository
func (s *myService) Delete(ctx context.Context, projectID uuid.UUID) error {
    return s.fooRepo.DeleteByProject(ctx, projectID)
}
```

If a service needs a new database operation, add a method to the appropriate repository.

### 3. Customer Datasource Access Must Use Adapters

There are two databases in this system:
- **Engine metadata database** (`ekaya_engine`) - Always PostgreSQL, accessed via `pkg/repositories/` using pgx directly. This is correct.
- **Customer datasources** - Can be PostgreSQL, MSSQL, or other databases. Must ALWAYS go through `pkg/adapters/datasource/` interfaces.

```go
// WRONG - using pgx directly for customer data
import "github.com/jackc/pgx/v5"
rows, _ := customerConn.Query(ctx, "SELECT * FROM their_table")

// CORRECT - use adapter factory
adapter := adapterFactory.NewSchemaDiscoverer(ctx, datasource)
tables, _ := adapter.DiscoverTables(ctx)
```

Key interfaces in `pkg/adapters/datasource/interfaces.go`:
- `SchemaDiscoverer` - Schema discovery, stats, FK relationships
- `QueryExecutor` - Query execution, validation, explain plans
- `ConnectionTester` - Connection testing

### 4. DAG Nodes Must Follow Consistent Patterns

All DAG nodes in `pkg/services/dag/` must:
- Inherit from `BaseNode` (use `*BaseNode` embedding)
- Follow fail-fast error handling (return errors, don't log and continue)
- Validate required fields (e.g., `dag.OntologyID`) at the start of `Execute()`
- Use the service interface pattern (accept minimal interfaces, not concrete types)

```go
// CORRECT DAG node pattern
type MyNode struct {
    *BaseNode
    mySvc MyServiceMethods  // Minimal interface
}

func (n *MyNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
    if dag.OntologyID == uuid.Nil {
        return fmt.Errorf("ontology ID is required")
    }

    result, err := n.mySvc.DoWork(ctx, dag.ProjectID)
    if err != nil {
        return err  // Fail fast - don't log and continue
    }

    return nil
}
```

### 5. Never Classify Columns by Name Patterns

Do NOT use column/table name patterns (suffixes, prefixes, substrings) to classify or make decisions. This is fragile and breaks across different naming conventions.

```go
// WRONG - fragile name-based classification
if strings.HasSuffix(columnName, "_id") {
    // Assume it's a foreign key
}
if strings.Contains(columnName, "amount") {
    // Assume it's currency
}

// CORRECT - use ColumnMetadata from ontology table
metadata, err := columnMetadataRepo.GetBySchemaColumnID(ctx, column.ID)
if err == nil && metadata != nil && metadata.Role != nil && *metadata.Role == "foreign_key" {
    // Confirmed FK from analysis
}
```

The `column_feature_extraction` service analyzes columns using data sampling and LLM classification. Column metadata is stored in `engine_ontology_column_metadata` (linked via `schema_column_id`). Use `ColumnMetadata` fields instead of name heuristics:
- `metadata.Role` - "primary_key", "foreign_key", "attribute", etc.
- `metadata.SemanticType` - "currency", "timestamp", "identifier", etc.
- `metadata.Purpose` - Business purpose description
- `metadata.GetTimestampFeatures()`, `GetBooleanFeatures()`, etc. - Type-specific features

`ColumnMetadata` is populated by the `column_feature_extraction` DAG step and should be available to every DAG step that runs after it. Do not assume metadata is unavailable — verify by checking the code path. If a service fetches `ColumnMetadata` and a record exists for a column, use it instead of name heuristics. The `meta == nil` case should only occur for columns that were added to the schema after the last extraction run (e.g., in data change detection for newly discovered columns).

### 6. MCP Tool Errors Must Return JSON Success Responses

MCP tool validation errors must be returned as **successful MCP responses containing JSON error objects**, not as Go errors that become MCP protocol errors (e.g., `-32603`).

**Why:** Some MCP clients do not pass protocol-level errors to the LLM—they may flash an error on screen or swallow it entirely. This makes Ekaya appear broken when the tool simply received invalid input. The LLM needs to see the error message to correct its approach.

```go
// WRONG - returns MCP protocol error, may not reach the LLM
if query == "" {
    return nil, fmt.Errorf("query parameter cannot be empty")
}

// CORRECT - returns successful MCP response with JSON error body
if query == "" {
    return NewErrorResult("invalid_parameters", "query parameter cannot be empty"), nil
}
```

**Error types:**
- **Actionable errors** (validation, invalid params, not found) → Use `NewErrorResult()` or `NewErrorResultWithDetails()`
- **System errors** (DB failures, connection errors) → Return Go errors (these are genuine failures)

See `pkg/mcp/tools/errors.go` for the `NewErrorResult()` helper and error code conventions.

## Error Handling Philosophy: Fail Fast

**This project follows a strict "fail fast" error handling policy.**

- **All errors should stop execution immediately** - do not continue with partial results
- **No silent error suppression** - never use `_` to ignore errors
- **No fallback logic** - if the primary approach fails, fail the operation
- **No "best effort" patterns** - either succeed completely or fail completely
- **Errors should propagate up** - return errors to callers, don't log and continue
- **Always log errors at ERROR level** - never use `logger.Warn` for errors, use `logger.Error`

## UI/Frontend Development

The UI is a React SPA built with TypeScript, Vite, and TailwindCSS.

### Technology Stack
- **React 18** with TypeScript (strict mode enabled)
- **Vite** for build tooling and dev server
- **TailwindCSS** for styling with custom theme
- **React Router** for client-side routing
- **Radix UI** for accessible components
- **Lucide React** for icons

### TypeScript Configuration
The UI uses **strict TypeScript mode** with additional strictness options:
- `exactOptionalPropertyTypes: true` - Strict optional property handling
- `noUncheckedIndexedAccess: true` - Index access safety
- `noImplicitReturns: true` - Explicit returns required

## Versioning

Version is managed through git tags and injected at build time:
- Source of truth: Git tags (e.g., `v0.0.1`)
- Version appears in `/ping` endpoint response
- No hardcoded versions - everything reads from `git describe`

## Configuration

### Initial Setup

**Required:**
1. Copy `config.yaml.example` to `config.yaml`

That's it - `config.yaml.example` includes working defaults for local development.

**Optional:**
- Copy `.env.example` to `.env` for environment-specific overrides

### Configuration Files

- `config.yaml` (gitignored) - Main configuration file with all settings
- `.env` (gitignored, optional) - Environment variables that override config.yaml values

### Secrets

Both secrets accept any passphrase (SHA-256 hashed to derive a key). For production, generate with: `openssl rand -base64 32`

| Secret | Purpose | Warning |
|--------|---------|---------|
| `project_credentials_key` | Encrypts datasource credentials in database | Cannot change after storing credentials |
| `oauth_session_secret` | Signs OAuth session cookies (~10 min lifetime) | All servers in cluster must share same value |

Environment variables `PROJECT_CREDENTIALS_KEY` and `OAUTH_SESSION_SECRET` override config.yaml values.

### Other Configuration

**Database Connection** - Configure in `config.yaml` under `engine_database:` or use `PG*` environment variables:
- `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE`, etc.

**TLS/HTTPS** - Optional, for custom domains:
- Set `base_url`, `tls_cert_path`, `tls_key_path` in config.yaml
- Or use environment variables: `BASE_URL`, `TLS_CERT_PATH`, `TLS_KEY_PATH`

## Database Access

The local PostgreSQL database is accessible via `psql` without parameters (PG* environment variables are set):
```bash
psql -c "SELECT * FROM engine_projects;"
psql -c "SELECT * FROM engine_users;"
```

### Row-Level Security (RLS)

Most tables use RLS for tenant isolation. To query RLS-protected tables (ontology, schema, datasources, etc.), set the tenant context first:

```bash
# Set tenant context before querying
psql -c "SELECT set_config('app.current_project_id', '<project-id>', false); SELECT * FROM engine_ontologies;"
```

Tables **without** RLS (admin tables): `engine_projects`, `engine_users`

### Critical: Always Filter by project_id

**When running analytics or exploratory queries, ALWAYS filter by `project_id` explicitly.** Do not rely on RLS via `set_config()` alone—use explicit WHERE clauses.

Multiple projects share the same database. Queries without `project_id` filters will return data from ALL projects, leading to:
- Inflated counts (e.g., 1,793 columns instead of 575)
- False "duplicate" detection (same table name exists in different projects)
- Incorrect analysis and wasted debugging time

```sql
-- WRONG: RLS may not filter as expected in all contexts
SELECT set_config('app.current_project_id', '<project-id>', false);
SELECT COUNT(*) FROM engine_schema_columns WHERE deleted_at IS NULL;

-- CORRECT: Always use explicit project_id filter
SELECT COUNT(*)
FROM engine_schema_columns
WHERE deleted_at IS NULL
  AND project_id = '<project-id>';
```

## Testing

```bash
# Run all checks including integration tests (requires Docker)
make check

# Run unit tests only (fast, no Docker required)
make test-short
```

### Integration Test Policy

Integration tests share a single Docker container via `sync.Once` in `pkg/testhelpers/containers.go`:

- `testhelpers.GetTestDB(t)` → Connection to `test_data` database (pre-loaded test data)
- `testhelpers.GetEngineDB(t)` → Connection to `ekaya_engine_test` database (migrations applied)

The container is created lazily on first use and reused across all test packages. Tests are isolated via unique `project_id` values and cleanup functions. Name destructive tests with `Test_Z_Destructive_*` prefix so Go runs them last alphabetically.

## Manual Testing: Ontology Extraction Workflow

When manually testing the ontology extraction workflow, use Chrome browser integration to watch the UI while simultaneously querying backend state.

### Ontology Tables

| Table | Purpose |
|-------|---------|
| `engine_ontology_dag` | DAG workflow state, status, current_node |
| `engine_dag_nodes` | Individual DAG node states (one per step) |
| `engine_ontologies` | Tiered ontology storage (domain_summary, column_details) |
| `engine_ontology_column_metadata` | Column semantic annotations with provenance (linked via schema_column_id) |
| `engine_ontology_table_metadata` | Table semantic annotations with provenance (linked via schema_table_id) |
| `engine_ontology_questions` | Questions generated during analysis for user clarification |
| `engine_ontology_chat_messages` | Ontology refinement chat history |
| `engine_llm_conversations` | Verbatim LLM request/response logs for debugging and analytics |
| `engine_project_knowledge` | Project-level facts learned during refinement |

### Clear Tables Before Testing

```sql
-- Clear ALL projects (use only when single project or full reset needed)
TRUNCATE engine_ontology_dag, engine_dag_nodes, engine_ontologies, engine_ontology_column_metadata, engine_ontology_table_metadata, engine_ontology_questions, engine_ontology_chat_messages, engine_llm_conversations, engine_project_knowledge CASCADE;
```

**Note:** When multiple projects exist in different states, scope deletes to a specific project:
```sql
-- Clear ontology data for a specific project (CASCADE handles child tables)
DELETE FROM engine_ontologies WHERE project_id = '<project-id>';
DELETE FROM engine_ontology_dag WHERE project_id = '<project-id>';
DELETE FROM engine_llm_conversations WHERE project_id = '<project-id>';
DELETE FROM engine_project_knowledge WHERE project_id = '<project-id>';
```

### Monitor DAG Workflow Progress

```sql
-- Overall DAG state and timing
SELECT status, current_node,
       EXTRACT(EPOCH FROM (COALESCE(completed_at, now()) - started_at))::int as elapsed_seconds
FROM engine_ontology_dag ORDER BY created_at DESC LIMIT 1;

-- DAG node states
SELECT node_name, status, started_at, completed_at
FROM engine_dag_nodes ORDER BY node_order;
```

### Monitor LLM Conversations

```sql
-- Recent LLM calls with timing and token usage
SELECT model, status, duration_ms, prompt_tokens, completion_tokens, created_at
FROM engine_llm_conversations ORDER BY created_at DESC LIMIT 10;

-- LLM errors
SELECT model, error_message, context, created_at
FROM engine_llm_conversations WHERE status != 'success' ORDER BY created_at DESC;

-- Token usage by model
SELECT model, COUNT(*) as calls, SUM(total_tokens) as total_tokens, AVG(duration_ms)::int as avg_ms
FROM engine_llm_conversations GROUP BY model;
```

### What to Watch For

1. **DAG state progression** - Should move through steps: KnowledgeSeeding → ColumnFeatureExtraction → FKDiscovery → TableFeatureExtraction → PKMatchDiscovery → ColumnEnrichment → OntologyFinalization → GlossaryDiscovery → GlossaryEnrichment
2. **Parallel execution** - Multiple LLM calls processed concurrently within each step
3. **Column/table metadata counts** - Check `engine_ontology_column_metadata` and `engine_ontology_table_metadata` are populated
4. **Token limit errors** - Large tables may exceed LLM context limits
5. **LLM conversation logging** - Check `engine_llm_conversations` for failed calls, high latency, or token spikes

### LLM Debug Logging

When built with the `debug` tag, LLM request/response pairs are written to:
```
$TMPDIR/ekaya-engine-llm-conversations
```

Files are named with timestamps: `<timestamp>_<model>_request.txt` and `<timestamp>_<model>_response.txt`.

## Manual Testing: MCP Server

ekaya-engine exposes an MCP (Model Context Protocol) server. Claude Code can act as the MCP client for testing.

### Connecting Claude Code as MCP Client

The MCP server URL is project-specific: `http://localhost:3443/mcp/{project-id}`

Configure in Claude Code's MCP settings or use `/mcp` to connect.

### Architecture Notes

- **Stateless mode** - The server uses `WithStateLess(true)`, meaning no persistent sessions
- **Tool filtering** - Tools are registered once and filtered per-request based on project config (`pkg/mcp/tools/developer.go:NewToolFilter`)
- **No change notifications** - Because of stateless mode, the server cannot push `tools/list_changed` notifications. If you toggle tools in the UI, you must reconnect (`/mcp`) to see changes.

### Available Tools

Tools are controlled via the UI at `/projects/{pid}/mcp-server`:

| Tool | Description | Requires |
|------|-------------|----------|
| `health` | Server health check | Always available |
| `get_schema` | Database schema with semantic annotations | Developer Tools enabled |
| `list_approved_queries` | List pre-approved SQL queries | Developer Tools enabled |
| `execute_approved_query` | Run a pre-approved query by ID | Developer Tools enabled |

### Key Files

- `pkg/mcp/server.go` - MCP server wrapper
- `pkg/mcp/tools/` - Tool implementations
- `pkg/handlers/mcp_handler.go` - HTTP handler for MCP endpoint
- `pkg/handlers/mcp_config.go` - Tool configuration API

## GitHub Actions & Pull Request Merging

### Important: Wait for CI/CD Checks
When merging pull requests, **ALWAYS wait approximately 3-4 minutes** for GitHub Actions to complete before merging.

