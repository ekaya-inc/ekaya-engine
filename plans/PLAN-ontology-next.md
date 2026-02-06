# PLAN: Ontology System - Next Phase

**Date:** 2026-01-25 (updated 2026-02-06)
**Status:** In Progress

## Overview

Remaining ontology work after consolidation and triage. Items completed since original plan: glossary SQL validation (BUG-10), `create_glossary_term` MCP tool.

## Remaining Tasks

### 1. Stats Collection Bug (BUG-9) - NEEDS VERIFICATION

**Problem:** 32.8% of joinable columns had NULL `distinct_count` despite `row_count` and `non_null_count` being populated correctly. Primarily affected `text` (60% null) and `integer` (50% null) columns.

**Current state:** Code fixes have been applied in `deterministic_relationship_service.go` and `schema_repository.go` — the persistence path looks correct. However, no columns currently have non-null `distinct_count` in the database, so the fix has not been verified by a fresh extraction.

**To verify:** Run a full extraction and check:
```sql
SELECT COUNT(*) AS total,
       COUNT(distinct_count) AS has_distinct,
       ROUND(100.0 * COUNT(distinct_count) / COUNT(*), 1) AS pct
FROM engine_schema_columns
WHERE is_joinable = true AND project_id = '<project-id>';
```

**Files:** `pkg/services/deterministic_relationship_service.go`, `pkg/adapters/datasource/postgres/schema.go`, `pkg/repositories/schema_repository.go`

### 2. Ontology Refresh (Incremental) - HIGH PRIORITY

**Problem:** "Re-extract Ontology" does a full wipe and rebuild, losing all user corrections and MCP feedback.

**What exists today:**
- Provenance infrastructure in `pkg/models/provenance.go` (SourceInferred, SourceMCP, SourceManual)
- Provenance fields on `engine_ontology_column_metadata` and `engine_ontology_table_metadata`

**What's missing:**
1. Event queue table for schema changes, user answers, MCP feedback
2. Schema fingerprinting and change detection (no-op when nothing changed)
3. Selective enrichment (skip user-verified items during re-extraction)
4. MCP `update_ontology` tool

**Implementation phases:**
1. Event queue table + schema fingerprinting
2. Change detection scan (diff current schema against stored fingerprint)
3. Selective DAG execution (only re-process changed/new items)
4. MCP `update_ontology` tool for ad-hoc corrections

### 3. Question Generation During Extraction - MEDIUM PRIORITY

**Problem:** DAG completes without generating any clarifying questions. `OntologyQuestionService.CreateQuestions` exists but is never called from any DAG step.

**Question categories:** terminology, enumeration, relationship, business_rules, temporal, data_quality

**What's needed:**
- Add question extraction to LLM response parsing in DAG steps
- Call `CreateQuestions` after relevant DAG steps (column feature extraction, relationship discovery, glossary)
- Add deterministic question generation for data patterns (high NULL rates, cryptic enum values, ambiguous relationships)

**Files:** `pkg/services/dag/` (step implementations), LLM prompts, `pkg/services/ontology_question.go`

### 4. Column Types in Entity Relationships - LOW PRIORITY

**Problem:** `SchemaRelationship` model has `SourceColumnID` and `TargetColumnID` but no denormalized column type fields. API consumers must join to `engine_schema_columns` to get types.

**Fix:** Add `source_column_type` and `target_column_type` to the relationship query/response by JOINing to `engine_schema_columns` in the repository layer.

**Files:** `pkg/models/schema.go` (SchemaRelationship), `pkg/repositories/` (relationship queries)

## Prioritized Task List

| Priority | Task | Type | Est. Complexity |
|----------|------|------|-----------------|
| 1 | Verify BUG-9 stats collection fix | VERIFY | Small (just needs extraction run) |
| 2 | Ontology refresh: event queue + fingerprinting | FEATURE | Large |
| 3 | Question generation during extraction | FEATURE | Large |
| 4 | Column types in relationship responses | ENHANCEMENT | Small |

## Success Criteria

- [ ] Stats collection: <10% joinable columns with NULL distinct_count after fresh extraction
- [ ] Refresh: Can refresh ontology without losing user corrections
- [ ] Questions: DAG generates clarifying questions during extraction
- [ ] Relationship API includes column types without extra lookups
