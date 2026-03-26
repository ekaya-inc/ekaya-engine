# PLAN: Optimizer Application Shell

**Status:** Open
**Priority:** High
**Date:** 2026-03-26
**Source:** Consolidated from retired branch `ddanieli/add-supabase-notes`
**Related:** `plans/optimizer/PLAN-performance-advisor.md`, `plans/optimizer/PLAN-schema-monitor.md`, `plans/optimizer/PLAN-query-analyzer.md`, `plans/guardian/PLAN-app-data-guardian.md`

## Overview

Optimizer is the working product name for the application previously described as "Database Ops." It is the performance and database-health counterpart to Data Guardian: Guardian answers "is the data safe and governed?" while Optimizer answers "is the database healthy, fast, and stable?"

## Product Decisions

- Product name: `Optimizer`
- Positioning: AI-powered database health monitoring and optimization
- Buyer: DBAs, data engineers, platform engineers
- Billing model: Live Demo -> 7-Day Trial -> Paid
- Initial applets:
  - Performance Advisor
  - Schema Monitor
  - Query Analyzer

## Application-Level Scope

- Add an installable Optimizer application shell with its own route and tile.
- Give each applet a first-class screen instead of burying performance work inside generic project tools.
- Expose the application through MCP so agents can proactively inspect health during development and operations.
- Implement Go-native services first; treat WASM as a later packaging option, not a prerequisite.

## MCP Surface

- `get_performance_advisors()`
- `get_schema_changes(since?)`
- `get_slow_queries(limit?)`
- `get_index_recommendations()`

## Cross-Product Contract

- Optimizer and Data Guardian should remain separate installable products.
- Findings should still be linkable across products: slow queries should point at security-sensitive tables when relevant, and Guardian findings should be able to reference performance risk where it matters.
- Shared infrastructure belongs in engine services, not in application-specific glue.

## Sequencing

1. Define the installed-app identity, shell route, and surface contract.
2. Land Performance Advisor as the first applet.
3. Add Schema Monitor and Query Analyzer behind the same shell.
4. Add cross-applet dashboards and trend views.

## Next Task

- Define the Optimizer app identity, route model, and shell responsibilities so the applet plans can target a stable install/runtime contract under the new product name.

