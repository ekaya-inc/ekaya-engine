# Ekaya Engine

Ekaya Engine is a Go backend + SvelteKit frontend that extracts, manages, and serves database ontology metadata. It connects to customer datasources, discovers schema, infers semantic relationships via LLM, and exposes the enriched ontology through REST APIs and MCP tools.

## Project Structure

```
pkg/                  Go backend
├── handlers/         HTTP API handlers (REST endpoints)
├── services/         Business logic (ontology DAG, schema, enrichment)
├── services/dag/     DAG node implementations (extraction pipeline)
├── models/           Data models and types
├── repositories/     Database access layer (PostgreSQL)
├── mcp/              MCP server and tool implementations
├── adapters/         Datasource adapters (postgres, mssql, etc.)
├── llm/              LLM client and prompt execution
├── auth/             Authentication and authorization
├── config/           Configuration loading
└── database/         Database connection and tenant scoping

ui/                   SvelteKit frontend (TypeScript, Tailwind CSS)
├── src/
│   ├── routes/       SvelteKit pages and layouts
│   ├── lib/          Shared components, stores, API clients
│   └── app.html      HTML shell
├── package.json
└── vite.config.ts

migrations/           PostgreSQL migrations (numbered, up/down)
plans/                Implementation plans, fixes, designs (PLAN-*, FIX-*, DESIGN-*, ISSUE-*)
tests/                Integration and MCP test suites
deploy/               Docker and deployment configuration
scripts/              Build and utility scripts
```

## Development Commands

```bash
make lint             # Go + frontend linting + typecheck
make test             # Go + frontend tests
make check            # Full validation (lint + test + build) — run before committing
make dev-server       # Run Go backend with hot reload (air)
make dev-ui           # Run frontend dev server (vite)
make build            # Build Go binary
make build-ui         # Build frontend (output to ui/dist/)
```

Use `make lint` and `make test` for quick feedback during development.

**IMPORTANT:** Always run `make check` before considering any implementation task complete. Do not skip this step.

## Running Integration Tests

Integration tests use Docker via testcontainers-go. The `DOCKER_HOST` env var must be set (auto-detected by `make` targets but not by raw `go test`).

```bash
# Run ALL integration tests (preferred — uses make)
make test-integration

# Run specific integration tests directly
go test -tags="integration,all_adapters" \
  -run 'TestName' ./pkg/path/to/package/ -v -count=1 -timeout 2m

# Postgres adapter tests need the postgres build tag too
go test -tags="integration,all_adapters,postgres" \
  -run 'TestName' ./pkg/adapters/datasource/postgres/ -v -count=1 -timeout 2m
```

Key points:
- Build tags required: `integration,all_adapters` (add `postgres` for postgres adapter tests)
- If Docker detection fails (e.g. OrbStack), set `DOCKER_HOST` first: `export DOCKER_HOST=$(docker context inspect --format '{{.Endpoints.docker.Host}}')`
- Tests spin up a fresh container per test run via the `ghcr.io/ekaya-inc/ekaya-engine-test-image:latest` image
- Use `-run 'TestPattern'` to target specific tests for fast feedback during TDD

## Key Architecture Concepts

- **Ontology DAG**: 7-node sequential pipeline that extracts semantic metadata from database schemas. Nodes: KnowledgeSeeding → ColumnFeatureExtraction → FKDiscovery → TableFeatureExtraction → RelationshipDiscovery → ColumnEnrichment → OntologyFinalization.
- **Provenance**: All ontology metadata tracks `source` (inferred/mcp/manual) and `last_edit_source` to distinguish engine-generated from user-curated data.
- **Tenant scoping**: Multi-tenant via PostgreSQL schemas. All DB queries go through `database.GetTenantScope(ctx)`.
- **Pending changes**: Schema and data changes are detected and queued for user approval before being applied to the ontology.
- **MCP tools**: Claude-facing tools for querying/updating ontology, executing SQL, managing glossary terms, etc.
