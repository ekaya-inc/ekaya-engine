# DESIGN: Color-Code Tiles by Installing Application

**Status:** TODO
**Created:** 2026-03-01

## Concept

Dashboard tiles should be color-coded by the application that installed them, giving users an immediate visual cue of which application provides which features.

Currently, tile icon backgrounds use arbitrary colors (blue, green, purple, orange, etc.) with no semantic meaning. Instead, each installed application should have an assigned color, and all tiles belonging to that application share its color.

## Current State

From the dashboard screenshots, the current tile layout is:

**Applications section:**
- MCP Server (blue icon)
- AI Data Liaison (blue icon)

**Data section (tiles from MCP Server):**
- Datasource (blue)
- Schema (blue)
- Pre-Approved Queries (orange)

**Intelligence section (tiles from MCP Server):**
- Ontology Extraction (purple)
- Ontology Questions (orange)
- Project Knowledge (green)
- Enrichment (orange)
- Relationships (green)

**Tiles from AI Data Liaison:**
- Audit Log (green)

The colors are inconsistent — MCP Server tiles use 4+ different colors, and nothing connects a tile visually to its parent application.

## Proposed Behavior

Each application gets a single accent color. All tiles installed by that application use its color for icon backgrounds and borders.

Example mapping:
- **MCP Server** = green: Datasource, Schema, Pre-Approved Queries, Ontology Extraction, Ontology Questions, Project Knowledge, Enrichment, Relationships
- **AI Data Liaison** = blue: Audit Log

When a new application is installed, its tiles appear in the application's assigned color. This makes it immediately clear which features came from which application.

## Broader Direction

Installing features via tiles is becoming the central mechanism for applications going forward. As more applications are added, color-coding becomes increasingly important — without it, users lose the ability to mentally group related features. The color mapping should be stored as part of the application metadata so it can be configured per-app and potentially customized by the user in the future.
