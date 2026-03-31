# REVIEW PLAN: RLS Access Is Enforced

**Status:** NOT STARTED
**Branch:** ddanieli/add-ai-agents

## Objective

Review the metadata-store database access model end to end to verify that tenant isolation is actually enforced by PostgreSQL Row-Level Security (RLS) using the correct tenant key for each table, usually `project_id`.

This is a review and verification plan only. It does not authorize code changes by itself.

## Scope

In scope:

- PostgreSQL metadata-store schema in `migrations/`
- RLS helper functions and policies
- Application database role assumptions and privilege model
- Tenant-scope plumbing in `pkg/database/`
- Repository, service, handler, MCP, background-job, and retention code paths that read or write the metadata store
- Integration and runtime checks that prove cross-project isolation

Out of scope:

- Customer datasource access in adapters such as `pkg/adapters/datasource/*`
- Non-Postgres systems
- Remediation implementation beyond documenting findings and proposed fixes

## Context

The current design relies on:

- `rls_tenant_id()` from [001_foundation.up.sql](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/migrations/001_foundation.up.sql#L1), which reads `app.current_project_id`
- Tenant-scoped connections created in [tenant.go](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/pkg/database/tenant.go#L24)
- Context propagation via [context.go](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/pkg/database/context.go#L12) and [middleware.go](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/pkg/database/middleware.go#L13)
- Repository access patterns that are expected to go through `database.GetTenantScope(ctx)`

The main risks to verify are:

- A table lacks RLS, lacks `FORCE ROW LEVEL SECURITY`, or uses the wrong key
- A child table should scope through a parent relation but instead permits broader access
- The app role has `SUPERUSER` or `BYPASSRLS`
- A code path uses `WithoutTenant` or a raw pool/connection when tenant scoping is required
- A connection leaks `app.current_project_id` across requests
- Some tables are intentionally global, but that exception is undocumented

## Review Questions

1. Which metadata-store tables are tenant-scoped, and what is the correct scoping key for each one?
2. Does every tenant-scoped table have RLS enabled and forced?
3. Does every tenant-scoped table have a policy that matches the intended tenant boundary?
4. Are there any tables that should scope by a parent join rather than direct `project_id`?
5. Is the application database role incapable of bypassing RLS?
6. Does every metadata-store query run under a tenant-scoped connection unless the access is intentionally global?
7. Are all intentional non-tenant code paths explicitly justified and documented?
8. Do runtime checks prove that one tenant cannot read or mutate another tenant's rows?

## Deliverables

The review should produce:

- A table-by-table RLS inventory
- A code-path inventory of metadata-store access points
- A list of explicit exceptions and whether they are justified
- A findings document grouped by severity
- A remediation checklist for any gaps discovered
- Tests to add or strengthen, if the current suite does not prove an important boundary

## Method

### Phase 1: Inventory the Metadata Schema

Build a complete inventory of application-owned metadata-store tables from `migrations/`.

For each table, record:

- Table name
- Owning migration
- Whether it is tenant-scoped, globally shared, or derived through a parent tenant-scoped relation
- Tenant key used by design: direct `project_id`, `engine_projects.id`, composite key, or parent join
- Whether RLS is enabled
- Whether `FORCE ROW LEVEL SECURITY` is enabled
- Policy names and expressions
- Whether the policy includes both `USING` and `WITH CHECK` where writes require it

Expected output:

- A review matrix covering every metadata-store table created by migrations `001` through `017`

### Phase 2: Classify the Correct Tenant Boundary Per Table

For each table in the inventory, decide what the correct scoping rule should be.

Examples:

- `engine_projects`: tenant key is `id`
- Most project-local tables: tenant key is `project_id`
- Child tables such as query-grant or DAG-child tables may need parent-based scoping through the owning table
- Truly global tables, if any, must be explicitly marked and justified

Special attention:

- Tables with no direct `project_id`
- Join tables
- Audit/history tables
- Tables added in down migrations that might be missed by the main inventory

### Phase 3: Audit the RLS Policy Definitions

Review every migration that defines or alters policies.

Checks:

- Policy uses the correct tenant key or parent relation
- Policy does not accidentally allow write access broader than read access
- Policy does not omit `WITH CHECK` where inserts/updates are possible
- `FORCE ROW LEVEL SECURITY` is present on tenant-scoped tables
- Indexing supports the policy shape well enough to be practical
- No later migration accidentally drops or weakens a policy

Evidence sources:

- `migrations/*.up.sql`
- `pg_class`, `pg_policies`, and `information_schema` in a live database

### Phase 4: Audit the Application Role and Connection Model

Verify the runtime assumptions that make RLS meaningful.

Checks:

- App role used by the server is not `SUPERUSER`
- App role does not have `BYPASSRLS`
- Table ownership does not weaken enforcement because `FORCE ROW LEVEL SECURITY` is present where needed
- `WithTenant()` always sets `app.current_project_id`
- `TenantScope.Close()` always resets `app.current_project_id`
- Any use of `WithoutTenant()` is cataloged and justified

Special attention:

- Startup and migration paths in `main.go`
- Background jobs and retention jobs
- Test helpers that might hide privilege problems

### Phase 5: Audit Code Paths That Touch the Metadata Store

Inventory all metadata-store access paths in application code.

Search targets:

- `database.GetTenantScope(ctx)`
- `database.SetTenantScope(...)`
- `db.WithTenant(...)`
- `db.WithoutTenant(...)`
- direct pool acquisition or raw connection usage
- repository implementations under `pkg/repositories/`
- service methods that execute SQL directly

For each access path, record:

- File and function
- Whether it requires tenant-scoped access
- How the tenant scope is established
- Whether it relies on RLS, explicit `WHERE project_id = ...`, or both
- Whether the path is intentionally global/admin-level

Required outcome:

- No tenant-scoped metadata access path should rely only on application-side filtering while bypassing tenant-scoped DB connections

### Phase 6: Review Intentional Global Access

Identify code that is expected to run without tenant context.

Examples to review:

- Project creation/bootstrap flows
- Migrations
- Retention sweeps and maintenance tasks
- Cross-project admin operations, if any

For each global path:

- Confirm it is necessary
- Confirm it is narrow in scope
- Confirm it does not accidentally expose broad query helpers to ordinary request paths
- Document why RLS is not the primary enforcement mechanism there

### Phase 7: Prove Enforcement With Runtime Checks

Create a runtime verification matrix for representative tables and access patterns.

Checks should cover:

- No tenant set
- Matching tenant set
- Non-matching tenant set
- Read operations
- Insert/update/delete operations where applicable
- Child-table enforcement through parent relations

Representative table families:

- Core tenant tables: projects, users
- Query/config tables
- Ontology metadata tables
- Audit/history tables
- Newly added agent tables

This phase should also verify that the live app role behaves differently from a superuser account when RLS is active.

### Phase 8: Test Coverage Review

Review existing tests to determine whether they actually prove tenant isolation.

Look for:

- Integration tests that set up two projects and verify isolation
- Tests that use the app role rather than a superuser connection
- Tests that confirm `WithTenant()` and `RESET app.current_project_id` behavior
- Tables or policy families with no isolation tests at all

Output:

- A prioritized list of missing or weak tests

### Phase 9: Findings and Remediation Plan

Summarize the results as:

- Critical: tenant escape or RLS bypass
- Important: missing policy, wrong key, missing `FORCE RLS`, unjustified `WithoutTenant`, or weak test coverage on high-risk paths
- Minor: inconsistent policy style, missing documentation, or unnecessary duplication between RLS and application filters

Each finding should include:

- File or migration reference
- Why it is wrong or risky
- Whether it is exploitable in production or only a maintenance hazard
- Recommended fix

## Acceptance Criteria

This review is complete when all of the following are true:

- Every metadata-store table is classified with its intended tenant boundary
- Every tenant-scoped table has been checked for `ENABLE RLS`, `FORCE RLS`, and correct policy expressions
- The server's database role has been verified for `SUPERUSER` and `BYPASSRLS`
- Every metadata-store DB access path has been categorized as tenant-scoped or intentionally global
- Every use of `WithoutTenant()` or equivalent bypass has been reviewed and justified
- Runtime verification has been defined for representative read and write paths
- Test coverage gaps are documented
- Findings are ready to be turned into one or more implementation plans if needed

## Expected Output Files After Review

The review itself should eventually produce one or more of:

- `plans/ISSUE-*.md` for concrete defects
- `plans/FIX-*.md` for narrow remediations
- `plans/PLAN-*.md` for larger cross-cutting changes

This file is only the review plan.
