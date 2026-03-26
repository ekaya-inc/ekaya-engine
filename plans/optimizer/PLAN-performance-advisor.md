# PLAN: Optimizer Performance Advisor

**Status:** Open
**Priority:** High
**Date:** 2026-03-26
**Source:** Consolidated from retired branch `ddanieli/add-supabase-notes`
**Related:** `plans/optimizer/PLAN-app-optimizer.md`

## Overview

Performance Advisor is Optimizer's direct answer to Supabase's performance advisors. It should start by covering the high-value generic PostgreSQL checks that Supabase exposes, then add AI explanations, generated fix SQL, prioritization, and historical trending.

## Scope

- Detect common performance issues with read-only catalog queries.
- Return normalized findings with severity, evidence, and suggested action.
- Generate business-aware explanations using ontology and table context.
- Persist findings over time so regressions and improvements are visible.
- Expose the applet in UI and through `get_performance_advisors()`.

## MVP Checks

- Unindexed foreign keys
- Tables without primary keys
- Unused indexes
- Duplicate indexes
- Table bloat

## Phase 2 Checks

- Tables with no indexes
- Stale statistics
- High dead-tuple ratios
- Lock contention
- Long-running transactions
- High-null or oversized columns where planning cost matters

## Existing Leverage

- `explain_query`
- Column probing and statistics
- Datasource schema discovery
- Ontology metadata for AI explanations

## Out of Scope

- Schema drift history
- Query-history aggregation
- Cross-product security workflows

## Sequencing

1. Define the finding schema and implement the MVP checks.
2. Persist finding snapshots for history and trend views.
3. Add AI explanation and fix-generation hooks.
4. Add broader health checks once the base output shape is stable.

## Next Task

- Implement the canonical finding shape and first-pass PostgreSQL checks so the applet can ship a useful advisor result without waiting on AI or trend infrastructure.

