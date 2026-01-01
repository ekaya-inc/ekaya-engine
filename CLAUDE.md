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

The project has full operational infrastructure but a minimal Go backend:
- **Working:** Build system, CI/CD, Docker, frontend (React/TypeScript/Vite)
- **Shell only:** `main.go` with just `/health`, `/ping`, and static file serving
- **Empty scaffold:** `pkg/` directories ready for implementation

## Development Commands

### Essential Commands

```bash
# Configure authentication (choose ONE)
make setup-auth-dev   # Localhost server + dev auth (WORKING - use this)
make setup-auth-local # Localhost server + localhost auth (needs ekaya-central emulator)
make setup-auth-prod  # Localhost server + prod auth (not tested)

# Development mode - run UI and API separately for hot reload
make dev-ui         # Terminal 1: UI dev server (http://localhost:5173)
make dev-server     # Terminal 2: Go API with auto-reload (port 3443)

# Run locally (builds UI and starts server on port 3443)
make run

# Build Docker image locally for testing (same as CI/CD)
make dev-build-docker

# Run tests (all via make check)
make check          # Comprehensive: format, lint, typecheck, tests (backend + frontend)

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

Configuration files are in `config/` subdirectory. The `make setup-auth-*` commands copy the appropriate template to `config.yaml` (gitignored).

## Database Access

The local PostgreSQL database is accessible via `psql` without parameters (PG* environment variables are set):
```bash
psql -c "SELECT * FROM engine_projects;"
psql -c "SELECT * FROM engine_users;"
```

## Testing

```bash
# Run all tests with summary
make test

# Run strict checks (formatting, linting, tests)
make check

# Run unit tests only (fast)
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
| `engine_ontology_workflows` | Workflow state, progress, task queue |
| `engine_ontologies` | Ontology data (entity_summaries, column_details) |
| `engine_ontology_questions` | Questions generated during analysis (decoupled from workflow) |
| `engine_ontology_chat_messages` | LLM conversation history |
| `engine_workflow_state` | Per-entity state (table/column/global). Preserved after workflow completes for assessor scripts; deleted when new extraction starts. |
| `engine_project_knowledge` | Project-level knowledge and context |

### Clear Tables Before Testing

```sql
TRUNCATE engine_ontology_workflows, engine_ontologies, engine_ontology_questions, engine_ontology_chat_messages, engine_workflow_state, engine_project_knowledge CASCADE;
```

### Monitor Workflow Progress

```sql
-- Overall workflow state and timing
SELECT state, progress->>'message' as msg,
       EXTRACT(EPOCH FROM (COALESCE(completed_at, now()) - started_at))::int as elapsed_seconds
FROM engine_ontology_workflows ORDER BY created_at DESC LIMIT 1;

-- Task queue size
SELECT jsonb_array_length(task_queue) as queue_size
FROM engine_ontology_workflows ORDER BY created_at DESC LIMIT 1;
```

### Monitor Entity States

```sql
-- Entity state summary by type and status
SELECT entity_type, status, COUNT(*)
FROM engine_workflow_state
GROUP BY entity_type, status
ORDER BY entity_type, status;

-- Find entities blocking workflow (not complete/failed)
SELECT entity_key, status, state_data
FROM engine_workflow_state
WHERE status NOT IN ('complete', 'failed');
```

### Check Questions Queue

Questions are stored in `state_data` JSONB for entities with `status = 'needs_input'`:

```sql
-- Find entities needing user input
SELECT entity_key, state_data->'questions' as questions
FROM engine_workflow_state
WHERE status = 'needs_input';

-- Count required vs optional questions (parse from state_data)
SELECT entity_key,
       jsonb_array_length(state_data->'questions') as total_questions,
       state_data->'llm_analysis'->>'has_required_questions' as has_required
FROM engine_workflow_state
WHERE status = 'needs_input';
```

### Check Entity Summaries Written

```sql
-- Count entity summaries
SELECT jsonb_object_keys(entity_summaries) as entity
FROM engine_ontologies WHERE is_active = true;
```

### What to Watch For

1. **Progress updates** - Should increment after each batch completes
2. **Parallel execution** - Work Queue should show multiple "Processing" tasks
3. **Stuck workflows** - Check for `needs_input` status blocking completion
4. **Task failures** - Check `last_error` in `engine_workflow_state`
5. **Token limit errors** - Large tables may exceed LLM context limits

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

