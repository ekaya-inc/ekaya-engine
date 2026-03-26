# PLAN: Data Guardian Application Shell

**Status:** Open
**Priority:** High
**Date:** 2026-03-26
**Source:** Consolidated from retired branch `ddanieli/add-supabase-notes`
**Related:** `plans/guardian/README.md`, `plans/guardian/DESIGN-guardian-security-alerting.md`, `plans/guardian/DESIGN-guardian-audit-visibility.md`, `plans/guardian/DESIGN-guardian-compliance.md`, `plans/guardian/DESIGN-guardian-data-protection.md`, `plans/guardian/DESIGN-guardian-data-quality.md`

## Overview

Data Guardian is the security, privacy, data quality, and compliance application in the Ekaya product line. This plan captures the application shell decisions that were scattered through the retired branch notes so the work can proceed under `plans/guardian/` instead of through a single oversized root-level plan.

## Product Decisions

- Product name: `Data Guardian`
- Positioning: "Protect your data. Prove your compliance. Automatically."
- Buyer: security, compliance, governance, and platform leads
- Billing model: Live Demo -> 7-Day Trial -> Paid
- Applets in initial scope:
  - Security Advisor
  - Sensitive Data Scanner
  - Data Quality Monitor
  - Audit & Compliance

## Application-Level Scope

- Add Data Guardian as its own installable application with an app shell, tile, and routing model distinct from AI Data Liaison.
- Present the four initial applets as first-class screens within the application rather than hiding them in unrelated project pages.
- Expose the application through MCP so agents can request security findings, sensitive-data scans, quality summaries, and compliance views without UI-only workflows.
- Keep the first implementation Go-native inside `ekaya-engine`; do not wait for the WASM runtime.

## UI and Surface Area

- Application route should use a dedicated namespace rather than piggybacking on existing AI Data Liaison routes.
- The application tile should describe Data Guardian as a bundled security/compliance product, not as a single feature.
- Each applet needs its own page contract and install-time visibility rules.
- Shared navigation, alert counts, and cross-applet links belong in the application shell, not reimplemented applet by applet.

## MCP Surface

- `get_security_advisors()`
- `scan_sensitive_data(tables?)`
- `get_data_quality_report(tables?)`
- `get_audit_events(since?, severity?)`
- `get_compliance_summary(framework?)`

These tools should be thin façades over application services rather than bespoke MCP-only logic.

## Sequencing

1. Establish the installable application shell and route model.
2. Land the four applet MVPs behind that shell.
3. Add shared dashboards, cross-applet summaries, and application-level navigation.
4. Expand into the broader Guardian design set already documented in `plans/guardian/`.

## Next Task

- Define the installed-app identifier, route structure, and shell responsibilities for Data Guardian so individual applet work can target a stable application contract.

