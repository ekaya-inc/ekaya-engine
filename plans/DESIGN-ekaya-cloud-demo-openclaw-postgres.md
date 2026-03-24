# DESIGN: Ekaya Cloud Demo - Connecting Postgres to OpenClaw (Safely and Securely)

**Status:** Draft
**Created:** 2026-03-24

## Overview

Ekaya Cloud will support **installable demos/templates/tutorial projects** from ekaya-central. For the first iteration, the user chooses a demo during project creation from the ekaya-central Projects screen or New Project flow. ekaya-central then creates the real project in the user's Ekaya Cloud account. The current provisioning payload in this repo only carries project identity, URLs, and assigned applications, so this feature requires an explicit extension of that existing provisioning contract to carry template metadata. The engine should still use the normal project provisioning path after redirect; it should not introduce a second bootstrap channel just for demos.

A demo is a curated project bootstrap, not a new standalone engine app. Installing a demo provisions a ready-to-use project experience inside Ekaya Cloud by combining existing app installations with seeded project data and configuration.

The first demo is **Connecting Postgres to OpenClaw (Safely and Securely)**. It gives a user an immediately usable project against the existing **The Look** PostgreSQL dataset hosted in Supabase, with a prebuilt ontology, curated approved queries, and named AI agents that already have limited query access.

The key product constraint is that the demo must be useful **without shipping AI credentials**. The ontology is already built, so the demo can be installed and used immediately. This lets the user copy the MCP Server address from their own cloud project and try OpenClaw without downloading or self-hosting ekaya-engine. If the user later invokes a feature that requires LLM access, Ekaya Cloud should fail clearly and direct the user to configure their own AI credentials.

---

## Goals

- Let a user install a complete demo from ekaya-central into an Ekaya Cloud project.
- Make the first demo selectable during normal project creation in ekaya-central.
- Provision a safe, read-only Postgres datasource for The Look dataset.
- Install the existing app surfaces needed to explore the dataset immediately.
- Seed a usable semantic layer: ontology, approved queries, and named agents.
- Exclude AI credentials from the demo package and from the installed project state.
- Fail gracefully when the user triggers an AI-dependent action in a credential-less demo project.
- Establish a reusable project-template mechanism that can support future demos, tutorials, and templates without ad hoc per-demo code paths.

## Non-Goals

- Detailed ontology export/import artifact design.
- Demo marketplace UX beyond what ekaya-central needs to expose installable demos.
- Automatic demo-to-project resync after installation.
- Shipping writable datasource credentials.
- Shipping shared AI credentials inside the demo.

---

## Demo Contents

The first demo installs the following project state:

| Component | Installed State |
|----------|------------------|
| `mcp-server` | Available as usual; demo provisions the datasource and project state that make MCP useful immediately. |
| Datasource | Supabase PostgreSQL connection to The Look dataset using read-only credentials. |
| `ontology-forge` | Installed with an already-imported ontology for The Look. |
| `ai-data-liaison` | Installed because the current product uses it as the governed surface for glossary and approved-query workflows. |
| Semantic bundle | Imported semantic state required for tutorial parity: ontology plus any glossary terms and project knowledge needed for the seeded experience. |
| Approved queries | Seeded curated query set for the dataset. |
| `ai-agents` | Installed with named agents that have pre-selected approved-query access. |
| AI config | Intentionally unset (`config_type=none` or equivalent no-credentials state). |

Because approved queries are part of the demo experience, the demo must seed them into the existing query system already used in the product, rather than introducing a parallel query catalog just for demos.

Because all OpenClaw tutorials use the same read-only The Look dataset, the demo should be treated as a canonical tutorial environment. Queries, visualizations, walkthroughs, and ETL examples should all be able to assume the same datasource contents and semantic setup.

---

## Core Design

### 1. Demos Are Project Templates, Not New Apps

The demo is modeled as a **versioned project template bundle** that Ekaya Cloud knows how to install into a project. It is not a new runtime app and it should not introduce a second app framework alongside `engine_installed_apps`.

The first use case is a demo, but the underlying mechanism should support a broader family of curated project types:

- demos
- tutorials
- templates
- other pre-seeded starter projects

The engine should therefore consume a general template/install manifest from project metadata rather than a hard-coded one-off demo flag.

The bundle provisions:

- required app installations
- datasource configuration
- selected schema scope
- imported ontology state
- approved queries
- named agents and their query access
- template metadata needed for provisioning, provenance, and versioning

ekaya-central remains responsible for project creation and for attaching the selected template metadata to the project record. Ekaya Cloud remains the system that writes project state into the metadata store. ekaya-central identifies what should be installed; ekaya-engine performs the installation during normal provisioning.

### 2. Installation Is a One-Time Seed, Not a Live Sync

Installing a demo copies curated state into the destination project. After installation, the project behaves like a normal Ekaya project:

- users can edit glossary/query/agent state
- users can replace datasource credentials
- users can add AI credentials later
- later local edits are not overwritten by background demo sync

This keeps the model understandable and avoids a long-lived coupling between a customer project and the demo source project.

### 3. Provisioning Extends the Existing Project Handshake

This feature should build on the current ekaya-central to ekaya-engine provisioning flow, not bypass it with a second setup channel.

Current repo knowledge:

- the checked-in ekaya-central `ProjectInfo` contract only returns project identity, URLs, and assigned applications
- ekaya-engine provisioning currently persists URLs and assigned applications from that payload
- template installation therefore requires a deliberate contract extension on that existing path

The required contract is:

- the user selects a demo/template in ekaya-central
- ekaya-central creates the project in the user's Ekaya Cloud account
- the project metadata contains the template identifier, version, and install manifest or equivalent provisioning data
- the existing redirect/provision flow sends that extended metadata to ekaya-engine
- ekaya-engine performs idempotent local setup from that metadata

This keeps template installation aligned with the existing source of truth for project creation.

### 4. Idempotent Seeding Requires Stable Template Asset Identity

The installer must be idempotent for retries and repeated project syncs. That requires stable identity for every seeded asset, not just "insert if missing" best effort.

For this design, the requirement is:

- every seeded asset must have a deterministic template-scoped identity, stable external ID, or equivalent template asset key
- the installer must reconcile against that identity before creating a datasource, query, agent, or semantic asset
- seeded provenance must be recorded in a way that lets support distinguish template-created assets from user-created assets without preventing later user edits

This matters especially for approved queries. In the current repo, queries are insert-oriented and do not have a natural unique key that would make naive replay idempotent.

### 5. Read-Only Database Access Is Part of the Demo Contract

The demo datasource must use credentials that are restricted to read-only access for the The Look dataset. The purpose of the demo is to show safe access from OpenClaw through Ekaya Cloud, not to allow mutation of the demo database.

Central security requirements:

- demo datasource credentials are curated and managed by Ekaya, not user-supplied at install time
- those credentials are not exposed through ekaya-central UI payloads
- agent access remains limited to approved queries rather than unconstrained SQL generation

### 6. The Demo Ships Semantic Assets, Not AI Secrets

The demo includes a prebuilt ontology and other seeded metadata, but it does **not** include LLM credentials. That means:

- the user can inspect schema, ontology, glossary/query/agent state, and execute permitted read paths immediately
- the user cannot re-run ontology extraction or other AI-dependent workflows until they configure AI

For the first demo, "semantic assets" is explicitly broader than ontology rows alone. If glossary terms or project knowledge are required for the source tutorial project to feel complete, they are part of the imported semantic bundle for this template.

This is a hard product boundary, not a temporary fallback.

### 7. AI-Dependent Actions Must Fail With a Product-Level Error

Today, some credential problems can surface as generic backend failures such as LLM configuration errors. For demo-installed projects, AI-dependent actions should instead fail with an explicit product-level response that the UI can recognize and explain.

Expected behavior:

- the UI may proactively disable clearly AI-only entry points when `config_type=none`
- the action fails fast
- the backend error states that the demo was installed without AI credentials and uses a structured product-level code, not only a generic LLM configuration failure
- the UI gives the user a direct path to configure AI credentials for the project
- after credentials are configured, the user can retry the operation normally

This applies to ontology re-extraction and any other workflow that requires project AI configuration, including direct API, MCP, or deep-link access that bypasses the normal UI checklist.

---

## Install Flow

The high-level install flow is:

1. A user chooses the demo/template in ekaya-central during project creation.
2. ekaya-central creates the project in the user's Ekaya Cloud account.
3. ekaya-central stores template metadata on the project so it is available during provisioning.
4. The user is redirected to the engine.
5. The extended provisioning protocol gives ekaya-engine the template metadata.
6. ekaya-engine detects the selected template and runs an idempotent installer for that template version using stable template asset identities.
7. The installer ensures required apps are installed.
8. The installer creates the read-only Supabase datasource for The Look.
9. The installer imports the curated semantic bundle.
10. The installer seeds approved queries into the existing query system.
11. The installer creates named agents and assigns approved-query access.
12. The installer leaves AI configuration unset and records demo/template provenance, asset identity, and version for supportability.

The install process must be idempotent so that a retry or repeated project sync does not duplicate datasources, semantic assets, queries, or agents.

---

## Semantic Bundle Import/Export Requirement

This demo depends on a new engine capability: **exporting a semantic bundle from one project and importing it into another**.

For this first demo, the important design decision is:

- the curated The Look semantic bundle will be produced once from a source project
- Ekaya Cloud demo install will import that bundle into each destination project
- the bundle includes the ontology plus any glossary and project knowledge required for the demo to behave like an already-extracted tutorial project
- the imported state must be sufficient for the destination project to behave like an already-extracted project

Semantic bundle export/import does **not** need to be exposed as a visible user-facing feature yet. For now, it is an internal engine capability required to support curated project templates.

The detailed artifact format and compatibility rules are intentionally deferred to follow-on design work, but the first-demo object boundary is no longer open-ended: the bundle must include all semantic assets required for tutorial parity.

---

## App and Data Responsibilities

### Existing Apps Reused

- `mcp-server` provides the connection and tool surface.
- `ontology-forge` provides ontology visibility and ontology lifecycle actions.
- `ai-data-liaison` provides the current governed glossary and approved-query workflow surface.
- approved-query functionality is reused rather than reimplemented for demos.
- `ai-agents` provides named agent identities and query scoping.

### New Template Capability

The new capability introduced by this design is a **template installer/orchestrator** in Ekaya Cloud plus an ontology export/import path that the installer can use.

No separate "demo runtime app" should be created for this feature.

---

## Graceful Credentialless Mode

The installed demo project should operate in a deliberate **credentialless semantic mode**:

- ontology exists
- glossary and project knowledge required for the tutorial exist
- approved queries exist
- named agents exist
- datasource works
- AI config does not exist

In this mode, non-AI functionality should work normally. AI-required actions should be blocked with a clear remediation path instead of silently degrading or throwing generic infrastructure errors.

This keeps the demo immediately useful while making the boundary around customer-provided AI credentials explicit.

---

## Open Questions

1. How should ekaya-central represent demo assignments: as a new demo catalog object, as bundled app metadata on the project, or through another project-scoped install manifest?
2. What exact template metadata shape should be included in the provisioning payload: template ID/version only, or a richer install manifest with seeded component definitions and asset IDs?
3. Where should template provenance and stable template asset identity be recorded in the metadata store for datasources, semantic assets, queries, and agents?
4. What is the versioning strategy for template bundles once the seeded ontology, glossary, project knowledge, approved queries, agents, or datasource assumptions evolve over time?
5. What exact structured error contract should AI-dependent demo failures use so UI, MCP clients, and support tooling can recognize the credentialless-demo condition consistently?

---

## Out of Scope for This Design

- the ontology artifact schema
- UI polish for demo catalog browsing
- demo uninstall/cleanup semantics
- upgrade/migration behavior between demo versions
- any shared hosted AI credential model
