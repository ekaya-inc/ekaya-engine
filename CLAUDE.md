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
make dev-server       # Run Go backend with hot reload (air)
make dev-ui           # Run frontend dev server (vite)
make build            # Build Go binary
make build-ui         # Build frontend (output to ui/dist/)
```

## Key Architecture Concepts

- **Ontology DAG**: 7-node sequential pipeline that extracts semantic metadata from database schemas. Nodes: KnowledgeSeeding → ColumnFeatureExtraction → FKDiscovery → TableFeatureExtraction → RelationshipDiscovery → ColumnEnrichment → OntologyFinalization.
- **Provenance**: All ontology metadata tracks `source` (inferred/mcp/manual) and `last_edit_source` to distinguish engine-generated from user-curated data.
- **Tenant scoping**: Multi-tenant via PostgreSQL schemas. All DB queries go through `database.GetTenantScope(ctx)`.
- **Pending changes**: Schema and data changes are detected and queued for user approval before being applied to the ontology.
- **MCP tools**: Claude-facing tools for querying/updating ontology, executing SQL, managing glossary terms, etc.
