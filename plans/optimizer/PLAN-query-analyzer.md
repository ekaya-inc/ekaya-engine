# PLAN: Optimizer Query Analyzer

**Status:** Open
**Priority:** High
**Date:** 2026-03-26
**Source:** Consolidated from retired branch `ddanieli/add-supabase-notes`
**Related:** `plans/optimizer/PLAN-app-optimizer.md`, `plans/optimizer/PLAN-performance-advisor.md`

## Overview

Query Analyzer is Optimizer's historical query-performance applet. It builds on the existing query and explain tooling, then adds aggregation, recurring-pattern detection, and AI optimization suggestions across real workloads.

## Scope

- Track slow queries and their plans over time.
- Identify recurring problem patterns across multiple queries.
- Generate optimization suggestions, rewrites, and index recommendations.
- Show before/after performance when fixes are applied.
- Expose summaries through UI and `get_slow_queries(limit?)`.

## Existing Leverage

- `explain_query`
- Query execution history
- MCP audit timing data
- Performance Advisor findings for cross-reference

## Out of Scope

- General database health checks already owned by Performance Advisor.
- Schema drift detection.
- Automatic execution of rewritten SQL.

## Sequencing

1. Define the slow-query and plan-history storage model.
2. Capture and normalize execution-plan history for slow queries.
3. Add recurring-pattern detection and link to missing-index findings.
4. Add AI optimization guidance once the historical base is stable.

## Next Task

- Define how slow-query history is persisted and keyed so explain-plan analysis, trend views, and optimization follow-up can all build on the same canonical record.

