# PLAN: Data Guardian Audit and Compliance

**Status:** Open
**Priority:** High
**Date:** 2026-03-26
**Source:** Consolidated from retired branch `ddanieli/add-supabase-notes`
**Related:** `plans/guardian/PLAN-app-data-guardian.md`, `plans/guardian/DESIGN-guardian-audit-visibility.md`, `plans/guardian/DESIGN-guardian-compliance.md`, `plans/guardian/PLAN-alerts-tile-and-screen.md`

## Overview

Audit & Compliance is the Data Guardian applet that turns existing engine telemetry into auditor-facing evidence. The core design choice is to reuse Ekaya's current audit, provenance, and retention data rather than introducing another parallel instrumentation stack.

## Scope

- Aggregate MCP audit events, general audit history, sensitivity metadata, role information, and retention evidence.
- Map those inputs to compliance frameworks such as SOC 2, GDPR, and HIPAA.
- Generate human-readable narratives and structured evidence packages.
- Provide summary and detail views in both UI and MCP form.

## Required Inputs

- MCP audit events and security classification.
- General audit log and provenance history.
- Column sensitivity metadata.
- Role and access-control state.
- Retention and lifecycle policy evidence.

## Deliverables

- Evidence summary dashboard.
- Framework-specific compliance summaries.
- Exportable evidence packages.
- `get_audit_events()` and `get_compliance_summary()` MCP surfaces.

## Out of Scope

- Building a new audit-event pipeline.
- Manual evidence curation workflows.
- Frameworks beyond the initial supported set.

## Sequencing

1. Normalize the evidence inputs and control-mapping model.
2. Implement framework summaries over existing audit data.
3. Add evidence package export.
4. Add AI narratives once the structured evidence path is stable.

## Next Task

- Define the first compliance control mapping table so the applet can produce deterministic summaries before narrative generation is introduced.

