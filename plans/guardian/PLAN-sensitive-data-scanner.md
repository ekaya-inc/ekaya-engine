# PLAN: Data Guardian Sensitive Data Scanner

**Status:** Open
**Priority:** High
**Date:** 2026-03-26
**Source:** Consolidated from retired branch `ddanieli/add-supabase-notes`
**Related:** `plans/guardian/PLAN-app-data-guardian.md`, `plans/guardian/DESIGN-guardian-data-protection.md`, `plans/guardian/DESIGN-guardian-access-control.md`

## Overview

Sensitive Data Scanner is the Data Guardian applet for discovering secrets, PII, PHI, financial data, and other protected information. The key distinction from Supabase-style advisors is content-aware detection: the scanner must catch sensitive values stored in generic or JSON fields, not just obvious column names.

## Scope

- Layer 1: column-name pattern detection using broad sensitive-data dictionaries.
- Layer 2: bounded content sampling for text and JSON-capable columns.
- Layer 3: persisted admin decisions for allow, block, and pending review states.
- UI and MCP behavior that respects those decisions.

## Detection Categories

- Secrets and credentials
- Identity documents
- Contact information
- Financial data
- Health data
- Demographic data

## Key Requirements

- Never persist sampled secret values in metadata tables.
- Decisions must survive re-scan and re-extraction flows.
- Blocked columns must be redacted consistently in MCP sample-value surfaces.
- The scanner must work with existing column metadata rather than inventing a parallel sensitivity source of truth.

## Existing Leverage

- Column metadata and `IsSensitive` semantics.
- Column statistics and sampling paths already used by probing tools.
- Existing schema and feature extraction services.

## Out of Scope

- Full consent-boundary enforcement.
- End-user masking policies by role.
- Cross-database DLP scanning beyond datasource scope.

## Sequencing

1. Ship name-pattern detection and result persistence.
2. Add safe content sampling for high-risk column types.
3. Add admin review workflow and redaction enforcement.
4. Expose scanner summaries through MCP and app dashboards.

## Next Task

- Define the classification model and decision persistence path so name-based detection can land first without blocking later content-sampling work.

