# PLAN: Optimizer Schema Monitor

**Status:** Open
**Priority:** High
**Date:** 2026-03-26
**Source:** Consolidated from retired branch `ddanieli/add-supabase-notes`
**Related:** `plans/optimizer/PLAN-app-optimizer.md`, `plans/guardian/PLAN-data-quality-monitor.md`

## Overview

Schema Monitor is Optimizer's schema-drift applet. It snapshots schema state, detects change over time, records the resulting timeline, and explains likely downstream impact using query, ontology, and glossary context.

## Scope

- Periodically snapshot datasource schema.
- Detect additive, destructive, and modified schema changes.
- Store a durable history of change events.
- Classify severity and likely impact.
- Expose history through UI and `get_schema_changes(since?)`.

## Existing Leverage

- `SchemaDiscoverer`
- `SchemaChangeDetectionService`
- Query and glossary metadata for downstream impact analysis
- Existing datasource adapters for PostgreSQL and MSSQL support

## Out of Scope

- Full data-quality anomaly detection
- Alerting infrastructure beyond what is needed for this applet
- Live query-plan analysis

## Sequencing

1. Define snapshot and change-event storage.
2. Wrap the existing schema change detection service with scheduled snapshots.
3. Add impact analysis over saved queries and glossary terms.
4. Add alert hooks and timeline UI.

## Next Task

- Define the snapshot and change-event persistence schema so the existing change-detection service can be wrapped without inventing a second schema-diff model later.

