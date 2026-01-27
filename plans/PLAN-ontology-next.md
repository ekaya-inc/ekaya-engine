# PLAN: Ontology System - Next Phase

**Date:** 2026-01-25
**Status:** Planning

## Overview

This plan consolidates remaining ontology work from previous FIX/DESIGN/PLAN documents into actionable tasks. The ontology system has made significant progress but several items need completion.

## Current State Analysis

### Already Implemented
- **Probe tools:** `probe_column`, `probe_columns`, `probe_relationship` - fully functional
- **Glossary read tools:** `list_glossary`, `get_glossary_sql` - working
- **Glossary write tools:** `update_glossary_term`, `delete_glossary_term` - working
- **Question MCP tools:** `list_ontology_questions`, `resolve_ontology_question`, etc. - working
- **Question service:** `OntologyQuestionService.CreateQuestions` exists but not called during extraction

### Needs Work

#### 1. Stats Collection Bug (BUG-9) - HIGH PRIORITY
**Problem:** 32.8% of joinable columns have NULL `distinct_count` despite code fixes being applied.

**Symptoms:**
- `row_count` and `non_null_count` are populated correctly
- Only `distinct_count` is lost
- Pattern: primarily `text` columns (60% null) and `integer` columns (50% null)
- `timestamp` and `numeric` columns work correctly (0% null)

**Investigation needed:**
- Runtime logging to trace values from `AnalyzeColumnStats` return through `UpdateColumnJoinability` persistence
- Check for column name mismatch in statsMap lookup
- Check for type conversion issues with pointer assignment

**Files:** `pkg/services/deterministic_relationship_service.go`, `pkg/adapters/datasource/postgres/schema.go`

#### 2. Glossary SQL Validation (BUG-10) - MEDIUM PRIORITY
**Problem:** 4/15 glossary terms have invalid SQL due to LLM hallucinations.

**Failed terms:**
- "Active Sessions" - column `started_at` doesn't exist
- "Content Popularity Score" - column `cl.channel_likes_count` doesn't exist
- "Offer Redemption Rate" - operator mismatch (bigint = text)
- "Payout Conversion Rate" - operator mismatch (bigint = text)

**Root cause:** LLM generates SQL referencing non-existent columns or using wrong type comparisons.

**Fix:** Improve LLM prompts with actual column names and type information.

**Files:** `pkg/services/glossary_service.go`

#### 3. Ontology Refresh - HIGH PRIORITY (DESIGN EXISTS)
**Problem:** Current "Re-extract Ontology" does full wipe and rebuild, losing user corrections.

**Design exists:** `DESIGN-ontology-refresh.md` describes incremental refresh with:
- Change detection scan (schema fingerprint, pending queue items)
- No-op when nothing changed
- Event queue for schema changes, user answers, MCP feedback
- Provenance tracking (source, confidence, verified_by_user)

**Implementation phases:**
1. Add provenance fields to entities/relationships
2. Implement event queue table
3. Schema fingerprinting and diffing
4. Selective enrichment (skip user-verified items)
5. MCP `update_ontology` tool

**Files:** Many - see DESIGN-ontology-refresh.md for full list

#### 4. Question Generation During Extraction - MEDIUM PRIORITY
**Problem:** DAG completes without generating any questions. The `engine_ontology_questions` table is always empty.

**Design exists:** `PLAN-ontology-question-generation.md` describes:
- Question categories: terminology, enumeration, relationship, business_rules, temporal, data_quality
- Generation points in DAG steps
- LLM prompt modifications to request questions
- Deterministic question generation for data patterns (high NULL rates, cryptic enum values)

**Key change:** Add question extraction to LLM response parsing and call `CreateQuestions` after each DAG step.

**Files:** DAG step services, LLM prompts, `OntologyQuestionService`

#### 5. Missing create_glossary_term Tool - LOW PRIORITY
**Problem:** Design exists for full CRUD but `create_glossary_term` not implemented.

**Other tools work:** update_glossary_term, delete_glossary_term are functional.

**Files:** `pkg/mcp/tools/glossary.go`

#### 6. Column Types in Entity Relationships - LOW PRIORITY
**Problem:** API response has `source_column_type` and `target_column_type` fields but they're never populated.

**Fix described in:** `FIX-add-column-type-to-entity-relationships.md`
- Add `source_column_id`, `target_column_id` FKs to `engine_entity_relationships`
- JOIN to `engine_schema_columns` to get types
- Update model, repository, service, handler

**Files:** Migration, `pkg/models/entity_relationship.go`, `pkg/repositories/entity_relationship_repository.go`

## Prioritized Task List

| Priority | Task | Type | Est. Complexity |
|----------|------|------|-----------------|
| 1 | BUG-9: Debug stats collection flow | FIX | Medium |
| 2 | Ontology refresh Phase 1: Provenance fields | TASK | Medium |
| 3 | Question generation during extraction | TASK | Large |
| 4 | BUG-10: Improve glossary SQL generation | FIX | Medium |
| 5 | Ontology refresh Phase 2: Event queue | TASK | Large |
| 6 | create_glossary_term MCP tool | TASK | Small |
| 7 | Column types in relationships | FIX | Medium |

## Success Criteria

- [ ] Stats collection: <10% joinable columns with NULL distinct_count
- [ ] Glossary: All terms have valid, executable SQL
- [ ] Refresh: Can refresh ontology without losing user corrections
- [ ] Questions: DAG generates at least some clarifying questions
- [ ] All MCP tools functional and tested

## Archived Documents

The following documents have been consolidated into this plan:
- `plans/FIX-ontology-extraction-bug9.md` → Task: FIX-stats-collection-debug.md
- `plans/FIX-ontology-extraction-bug10.md` → Task: FIX-glossary-sql-validation.md
- `plans/DESIGN-ontology-refresh.md` → Task: TASK-ontology-refresh-phase1.md
- `plans/PLAN-ontology-probe-tools.md` → Already implemented
- `plans/PLAN-ontology-question-generation.md` → Task: TASK-question-generation.md
- `plans/DESIGN-glossary-client-updates.md` → Task: TASK-create-glossary-term-tool.md
- `plans/FIX-add-column-type-to-entity-relationships.md` → Task: FIX-relationship-column-types.md
