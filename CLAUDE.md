# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is **ekaya-engine** - a clean architecture rebuild of the Ekaya regional controller. The project was initialized from ekaya-region's infrastructure but with a minimal Go backend shell, designed for incremental, well-architected development.

**Design Philosophy:**
- Clean architecture with separation of concerns
- Dependency injection for testability
- Controllers (thin HTTP handlers) → Services (business logic) → Repositories (data access)
- Fail-fast error handling (same as ekaya-region)

**Reference:** For existing implementation patterns (auth, SDAP, MCP), see `../ekaya-region/`

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

The UI is a React SPA built with TypeScript, Vite, and TailwindCSS (same as ekaya-region).

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

The idea for integration tests is that the test suite would create a single container, then use that container for all of the tests. If there are destructive tests, order them at the end so that they do not interfere.

- Use `TestMain(m *testing.M)` to create shared Docker container once
- Each test gets unique `project_id` for metadata isolation
- Name destructive tests with `Test_Z_Destructive_*` prefix so Go runs them last alphabetically

## GitHub Actions & Pull Request Merging

### Important: Wait for CI/CD Checks
When merging pull requests, **ALWAYS wait approximately 3-4 minutes** for GitHub Actions to complete before merging.
- ekaya-engine is the clean and architecturally sound migration from ../ekaya-region/. You may look in that directory for reference but make sure to migrate any artifact over (code, documentation, configuration) in the context of an extermely well-written service.