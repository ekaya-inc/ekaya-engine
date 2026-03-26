# PLAN: Data Guardian Data Quality Monitor

**Status:** Open
**Priority:** High
**Date:** 2026-03-26
**Source:** Consolidated from retired branch `ddanieli/add-supabase-notes`
**Related:** `plans/guardian/PLAN-app-data-guardian.md`, `plans/guardian/DESIGN-guardian-data-quality.md`, `plans/optimizer/PLAN-schema-monitor.md`

## Overview

Data Quality Monitor is Data Guardian's quality applet for profiling data, generating baseline expectations, detecting anomalies, and explaining failures with business-aware context. This is the Data Guardian interpretation of the older "AI Data Guardian" concept, narrowed to an applet that complements security and compliance work.

## Scope

- Profile columns and tables for completeness, freshness, uniqueness, volume, distribution, and referential integrity.
- Auto-generate initial expectations from those baselines.
- Run scheduled checks and record pass/fail results over time.
- Explain anomalies with AI and correlate them with schema changes where useful.
- Support human-approved fix suggestions but never automatic data modification.

## Existing Leverage

- `probe_columns` and related column statistics tooling.
- Column feature extraction and distribution helpers.
- Ontology metadata for semantic context.
- Existing schema change detection work for cross-referencing.

## Out of Scope

- General-purpose alerting infrastructure outside this applet.
- Direct execution of corrective SQL.
- Non-database source monitoring.

## Sequencing

1. Define the quality expectation model and storage layout.
2. Implement baseline profiling and expectation generation.
3. Add scheduled evaluation and anomaly recording.
4. Add AI explanations and suggested remediation flows.

## Next Task

- Specify the first expectation types and storage schema so profiling results can be persisted and replayed without locking in the AI layer prematurely.

