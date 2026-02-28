# FIX: Ontology Question Tool Descriptions Must Require Metadata Updates

Status: FIXED

## Problem

When an MCP client (e.g., Claude Code) answers ontology questions via `resolve_ontology_question`, it often writes resolution notes but **skips updating the actual table/column metadata** that the question was about. This means the knowledge captured in the answer is locked in resolution notes (which nothing reads programmatically) instead of being persisted to the ontology where it improves future query generation.

### Root Cause

The tool descriptions for `list_ontology_questions` and `resolve_ontology_question` are too soft about the requirement to update metadata. The current `resolve_ontology_question` description says "Use this after you've used other update tools" — but this reads as a suggestion, not a requirement. MCP clients treat it as optional and skip straight to resolving with notes.

There is no automated process that reads resolution notes and updates metadata after the fact. The MCP client answering questions is the **only** opportunity to persist knowledge to the ontology.

### Observed Behavior

Out of 27 resolved questions in a real session:
- 9 had corresponding `update_column` calls (the enum questions)
- 6 had `update_project_knowledge` calls (key business rules)
- 12 had **only** resolution notes — the knowledge was lost to the ontology

## Solution

Update the tool descriptions in `pkg/mcp/tools/questions.go` to make metadata updates a hard requirement, not a suggestion.

### File: `pkg/mcp/tools/questions.go`

### Change 1: `list_ontology_questions` description (line 78)

Add a workflow instruction block to the end of the description so the MCP client sees it every time it lists questions:

```go
// Current (line 78-83):
mcp.WithDescription(
    "List ontology questions generated during schema extraction with flexible filtering and pagination. "+
        "Filter by status (pending/skipped/answered/deleted), category (business_rules/relationship/terminology/enumeration/temporal/data_quality), "+
        "entity (affected entity name), or priority (1-5, where 1=highest). "+
        "Returns questions with id, text, category, priority, context, created_at, and counts_by_status for dashboard display. "+
        "Use this to batch-process pending questions or review answered questions. "+
        "Example: list_ontology_questions(status='pending', priority=1, limit=20) returns high-priority unanswered questions.",
),

// Updated:
mcp.WithDescription(
    "List ontology questions generated during schema extraction with flexible filtering and pagination. "+
        "Filter by status (pending/skipped/answered/deleted), category (business_rules/relationship/terminology/enumeration/temporal/data_quality), "+
        "entity (affected entity name), or priority (1-5, where 1=highest). "+
        "Returns questions with id, text, category, priority, context, created_at, and counts_by_status for dashboard display. "+
        "Use this to batch-process pending questions or review answered questions. "+
        "Example: list_ontology_questions(status='pending', priority=1, limit=20) returns high-priority unanswered questions. "+
        "IMPORTANT: When answering questions, you MUST update the affected table/column metadata (via update_column, update_table, "+
        "update_project_knowledge, or update_glossary_term) BEFORE calling resolve_ontology_question. "+
        "Resolution notes alone are NOT persisted to the ontology — only metadata updates are. "+
        "The MCP client answering questions is the only opportunity to enrich the ontology with this knowledge.",
),
```

### Change 2: `resolve_ontology_question` description (line 404)

Replace the soft "Use this after" language with an explicit prerequisite:

```go
// Current (line 404-410):
mcp.WithDescription(
    "Mark an ontology question as resolved after researching and updating the ontology. "+
        "Use this after you've used other update tools (update_entity, update_column, update_glossary_term, etc.) "+
        "to capture the knowledge you learned while answering the question. "+
        "This transitions the question status from 'pending' to 'answered' and sets the answered_at timestamp. "+
        "Example workflow: 1) Research code/docs to answer question, 2) Update ontology with learned knowledge via update tools, "+
        "3) Call resolve_ontology_question with optional resolution_notes explaining how you found the answer.",
),

// Updated:
mcp.WithDescription(
    "Mark an ontology question as resolved after researching and updating the ontology. "+
        "PREREQUISITE: Before calling this tool, you MUST have already updated the affected metadata using the appropriate tool(s): "+
        "update_column (for column descriptions, enum values, entity, role), "+
        "update_table (for table descriptions, usage notes, table type), "+
        "update_glossary_term (for business term definitions), or "+
        "update_project_knowledge (for business rules, conventions, terminology). "+
        "Resolution notes alone do NOT update the ontology — they are only recorded for audit purposes. "+
        "If you skip the metadata update, the knowledge from your answer will be lost. "+
        "This transitions the question status from 'pending' to 'answered' and sets the answered_at timestamp. "+
        "Workflow: 1) Research to find the answer, 2) Call update_column/update_table/update_project_knowledge with the answer, "+
        "3) Call this tool with resolution_notes explaining how you found the answer.",
),
```

## Verification

1. Read the updated descriptions in `questions.go` and confirm the language is imperative, not suggestive
2. Run existing tests: `go test ./pkg/mcp/tools/ -run TestQuestion -v`
3. Manual test: connect an MCP client, list questions, and verify the description text appears in the tool definition
