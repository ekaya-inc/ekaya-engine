# PLAN: Data Guardian Security Advisor

**Status:** Open
**Priority:** High
**Date:** 2026-03-26
**Source:** Consolidated from retired branch `ddanieli/add-supabase-notes`
**Related:** `plans/guardian/PLAN-app-data-guardian.md`, `plans/guardian/DESIGN-guardian-security-alerting.md`, `plans/guardian/DESIGN-guardian-access-control.md`, `plans/guardian/DESIGN-guardian-compliance.md`

## Overview

Security Advisor is Data Guardian's proactive security linting and remediation applet. It generalizes the useful parts of Supabase's security advisors for any PostgreSQL deployment, then layers on Ekaya-specific context, AI explanations, risk scoring, and fix generation.

## Scope

- Detect database security misconfigurations with read-only catalog queries.
- Surface structured findings with severity, evidence, and remediation guidance.
- Use ontology and sensitivity metadata to explain business impact, not just technical symptoms.
- Map findings to compliance controls where appropriate.
- Expose findings in both the UI and `get_security_advisors()`.

## MVP Checks

- Policies exist but RLS is disabled.
- RLS is enabled with no policies.
- Always-true write policies.
- Functions without explicit `search_path`.
- Extensions installed in `public`.
- Outdated extension versions.
- Dangerous `reg*` column types.
- Roles with excessive or risky access.

## Existing Leverage

- `QueryExecutor` for catalog inspection.
- Datasource schema discovery for table and permission context.
- Column sensitivity and ontology metadata for risk scoring.
- Existing audit and compliance models for downstream control mapping.

## Out of Scope

- Full Data Guardian cross-applet dashboards.
- Sensitive-data content sampling.
- Compliance evidence export flows.

## Sequencing

1. Define the normalized finding schema and severity model.
2. Implement the generic PostgreSQL checks.
3. Add fix SQL generation and AI explanation hooks.
4. Wire the service into UI and MCP exposure.

## Next Task

- Draft the Security Advisor finding model and the first batch of generic PostgreSQL checks so the applet can ship a non-AI baseline before adding risk scoring and narratives.

