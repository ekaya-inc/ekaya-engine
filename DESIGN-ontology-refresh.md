# DESIGN: Incremental Ontology Refresh

## Problem Statement

The current "Re-extract Ontology" is a full wipe and rebuild:
- Deletes all entities (cascade deletes relationships)
- Re-runs the entire DAG from scratch
- All LLM calls re-executed
- User knowledge (answered questions, chat refinements) is orphaned or lost

This is wasteful and loses valuable user corrections. A true "Refresh" should do minimal work when new information arrives.

## Sources of New Information

1. **Schema changes** - new tables, removed tables, new columns, new FKs
2. **User answers** - responses to clarifying questions about entities/relationships
3. **Chat refinements** - user corrections via ontology chat interface
4. **MCP Client feedback** - `update_ontology` tool allowing LLM clients to report/fix issues
5. **Data discovery** - new enum values, cardinality changes (future)

## Design Goals

1. **Incremental** - process only what changed, not everything
2. **Knowledge-preserving** - user corrections survive refresh
3. **Confidence-aware** - track source and confidence of each fact
4. **Event-driven** - changes flow through a queue, not batch re-extraction
5. **MCP-integrated** - AI clients can contribute to ontology accuracy
6. **No-op when current** - refresh on an unchanged schema completes instantly

---

## Refresh Flow Overview

When the user clicks "Refresh Ontology", the system performs a **change detection scan** before any processing:

```
┌─────────────────────────────────────────────────────────┐
│                  1. CHANGE DETECTION SCAN                │
│                                                         │
│  Compare current state to stored fingerprints:          │
│  ┌─────────────────────────────────────────────────┐   │
│  │ • DDL changes (tables, columns, FKs, indexes)   │   │
│  │ • Enumeration changes (new values in columns)   │   │
│  │ • Pending user answers (unanswered questions)   │   │
│  │ • Pending MCP feedback (unprocessed corrections)│   │
│  │ • Pending chat refinements                      │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                               │
│                         ▼                               │
│              ┌──────────────────┐                       │
│              │ Changes found?   │                       │
│              └────────┬─────────┘                       │
│                       │                                 │
│         ┌─────────────┴─────────────┐                   │
│         │ NO                        │ YES               │
│         ▼                           ▼                   │
│  ┌──────────────────┐    ┌──────────────────────────┐  │
│  │ "Ontology is     │    │ Queue changes for        │  │
│  │  up-to-date"     │    │ processing               │  │
│  │                  │    │                          │  │
│  │ (no-op, instant) │    │ → Schema changes         │  │
│  └──────────────────┘    │ → Enum changes           │  │
│                          │ → User/MCP/Chat pending  │  │
│                          └────────────┬─────────────┘  │
│                                       │                 │
│                                       ▼                 │
│                          ┌──────────────────────────┐  │
│                          │ 2. PROCESS QUEUE         │  │
│                          │    (incremental work)    │  │
│                          └──────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### Change Detection Details

**DDL Changes:**
- Compare schema fingerprint (hash of tables + columns + FKs)
- Detect: new tables, removed tables, new columns, type changes, new FKs

**Enumeration Changes:**
- For columns marked as enums or with low cardinality
- Query `SELECT DISTINCT` and compare to stored enum values
- Detect: new enum values, removed enum values

**Pending Queue Items:**
- Check `engine_ontology_refresh_events` for unprocessed events
- Check `engine_ontology_questions` for unanswered questions
- Check MCP feedback that hasn't been applied

### No-Op Scenario

If the scan finds no changes:
1. Update `last_schema_check` timestamp
2. Return immediately with "Ontology is up-to-date"
3. No LLM calls, no database writes (except timestamp)
4. UI shows green checkmark

This makes "Refresh" safe to click frequently - it's cheap when nothing changed.

### Example Scenario: Admin Updates Entity

**Setup:** Admin navigates to Entities screen and:
1. Updates the description for "Billing Engagement" entity
2. Adds a new alias "Session Booking" for the same entity
3. Saves changes

**Behind the scenes:**
- Changes are written to `engine_ontology_refresh_events` queue:
  ```json
  {"type": "entity_update", "entity_id": "...", "fields": ["description"]}
  {"type": "alias_added", "entity_id": "...", "alias": "Session Booking"}
  ```
- Ontology tile badge updates to show "(2)" pending changes

**Admin clicks [Refresh Ontology]:**

1. **Change Detection Scan** runs:
   - Schema fingerprint: unchanged
   - Enum values: unchanged
   - Pending queue: 2 items found (entity_update, alias_added)

2. **Dependency Analysis** builds a minimal DAG:
   ```
   ┌─────────────────────────────────────────────────────────┐
   │           Refresh DAG (optimized for this change)       │
   │                                                         │
   │   Entity Update ─────┐                                  │
   │   (description)      │                                  │
   │                      ├──→ Ontology Finalization         │
   │   Alias Added ───────┘    (regenerate EntitySummary,    │
   │   ("Session Booking")      update synonyms)             │
   │                                                         │
   │   [Skipped: EntityDiscovery, EntityEnrichment,         │
   │    FKDiscovery, ColumnEnrichment, PKMatchDiscovery,    │
   │    RelationshipEnrichment - no changes detected]        │
   └─────────────────────────────────────────────────────────┘
   ```

3. **Execution:**
   - Steps 1-6 are **skipped** (no schema/entity/relationship changes)
   - Entity update and alias are applied directly (no LLM needed - user provided values)
   - **Ontology Finalization** runs to:
     - Update `EntitySummary` with new description
     - Add "Session Booking" to synonyms list
     - Regenerate domain summary if description materially changed (optional LLM call)

4. **Result:**
   - UI shows: "Refreshed: 1 entity updated, 1 alias added"
   - Total time: ~1-2 seconds (no full DAG, minimal LLM)
   - Badge clears

**Key insight:** The refresh DAG is dynamically built based on what changed. User-provided values (description, alias) don't need LLM enrichment - they flow directly to finalization. Only downstream dependencies are executed.

### Dependency-Aware DAG Optimization

When building the refresh DAG, the system analyzes what downstream steps are affected:

| Change Type | Triggers |
|-------------|----------|
| Schema: new table | EntityDiscovery → EntityEnrichment → FKDiscovery → ... → Finalization |
| Schema: new FK | FKDiscovery → ColumnEnrichment → RelationshipEnrichment → Finalization |
| User: entity description | Finalization only (direct write) |
| User: entity alias | Finalization only (direct write) |
| User: relationship correction | RelationshipEnrichment → Finalization |
| MCP: missing relationship | FKDiscovery or PKMatchDiscovery → RelationshipEnrichment → Finalization |

Steps can run in **parallel** when independent:
- EntityEnrichment and FKDiscovery can run in parallel (no dependency)
- ColumnEnrichment and PKMatchDiscovery can run in parallel

Steps must run **sequentially** when dependent:
- EntityDiscovery → EntityEnrichment (enrichment needs entities)
- FKDiscovery → ColumnEnrichment (column enrichment uses FK info)
- RelationshipEnrichment → Finalization (finalization writes relationship descriptions)

### Change Aggregation

When changes are found, they are normalized into queue events:

```go
type RefreshScanResult struct {
    HasChanges      bool
    SchemaChanges   []SchemaChange   // DDL diffs
    EnumChanges     []EnumChange     // New/removed enum values
    PendingAnswers  []QuestionID     // Unanswered questions
    PendingMCP      []MCPFeedback    // Unprocessed corrections
    PendingChat     []ChatRefinement // Unprocessed refinements
}

func (r RefreshScanResult) IsUpToDate() bool {
    return !r.HasChanges &&
           len(r.SchemaChanges) == 0 &&
           len(r.EnumChanges) == 0 &&
           len(r.PendingAnswers) == 0 &&
           len(r.PendingMCP) == 0 &&
           len(r.PendingChat) == 0
}
```

---

## Core Concepts

### 1. Ontology Item Provenance

Every entity, relationship, and column annotation should track:

```
source:           "ddl" | "llm" | "user" | "mcp"
confidence:       float (0.0 - 1.0)
verified_by_user: boolean
last_verified_at: timestamp
is_stale:         boolean
```

**Confidence hierarchy:**
- `user` verified = 1.0 (never overwritten by LLM)
- `mcp` corrected = 0.95 (high trust, but user can override)
- `llm` enriched = 0.7-0.9 (depends on model confidence)
- `ddl` inferred = 0.5-1.0 (deterministic but may be incomplete)

### 2. Refresh Event Queue

A persistent queue (Postgres-backed) for change events:

```sql
CREATE TABLE engine_ontology_refresh_events (
  id              UUID PRIMARY KEY,
  project_id      UUID NOT NULL,
  event_type      TEXT NOT NULL,  -- schema_change, user_answer, mcp_feedback, etc.
  payload         JSONB NOT NULL,
  priority        INT DEFAULT 0,  -- higher = process first
  created_at      TIMESTAMP,
  processed_at    TIMESTAMP,
  error_message   TEXT
);
```

Events are processed in order, with deduplication for redundant changes.

### 3. Affected Item Resolution

Each event type maps to affected ontology items:

| Event Type | Affected Items |
|------------|----------------|
| `table_added` | New entity candidate |
| `table_removed` | Entity marked stale, relationships orphaned |
| `column_added` | Entity column details, possible new FK |
| `fk_added` | New relationship candidate |
| `user_answer` | Specific entity or relationship referenced |
| `chat_refinement` | Entity/relationship from chat context |
| `mcp_feedback` | Explicitly specified items |

---

## Event Types & Processing

### Schema Change Events

Triggered by: schema sync, user-initiated "Check for changes"

**Detection:**
```
current_fingerprint = hash(sorted(tables + columns + fks))
if current_fingerprint != stored_fingerprint:
    diff = compute_schema_diff(old_schema, new_schema)
    emit events for each change
```

**Event payloads:**
```json
{"type": "table_added", "schema": "public", "table": "new_table", "columns": [...]}
{"type": "table_removed", "schema": "public", "table": "old_table"}
{"type": "column_added", "schema": "public", "table": "users", "column": "phone"}
{"type": "fk_added", "source": "orders.user_id", "target": "users.id"}
```

**Processing:**
1. `table_added` → Create entity shell (deterministic), queue LLM enrichment
2. `table_removed` → Mark entity stale, flag relationships for review
3. `column_added` → Queue column enrichment if entity exists
4. `fk_added` → Create relationship (deterministic), queue description enrichment

### User Answer Events

Triggered by: user answering an ontology question

**Payload:**
```json
{
  "type": "user_answer",
  "question_id": "...",
  "entity_id": "...",
  "answer": "This entity represents a customer, not a user account",
  "corrections": {
    "name": "Customer",
    "description": "A paying customer of the platform"
  }
}
```

**Processing:**
1. Apply corrections directly (user answers are authoritative)
2. Mark entity as `verified_by_user = true`
3. Store answer in `engine_project_knowledge` for future LLM context
4. No queue needed - apply immediately

### Chat Refinement Events

Triggered by: ontology chat messages that result in corrections

**Payload:**
```json
{
  "type": "chat_refinement",
  "message_id": "...",
  "affected_entities": ["..."],
  "affected_relationships": ["..."],
  "changes": [
    {"entity_id": "...", "field": "description", "old": "...", "new": "..."}
  ]
}
```

**Processing:**
1. Apply changes directly
2. Mark items as `source = "user"`, `verified_by_user = true`
3. Store in project knowledge

### MCP Feedback Events

Triggered by: `update_ontology` MCP tool call

**Payload:**
```json
{
  "type": "mcp_feedback",
  "session_id": "...",
  "corrections": [
    {
      "correction_type": "entity_name",
      "entity_id": "...",
      "current_value": "billing_activities",
      "suggested_value": "Billing Activity",
      "confidence": 0.9,
      "reason": "Table name should be human-readable singular noun"
    },
    {
      "correction_type": "missing_relationship",
      "source_entity": "User",
      "target_entity": "Account",
      "via_column": "users.account_id",
      "relationship_type": "belongs_to",
      "reason": "FK exists but relationship not in ontology"
    },
    {
      "correction_type": "wrong_description",
      "entity_id": "...",
      "suggested_description": "A reservation for a property stay",
      "reason": "Current description is too generic"
    }
  ]
}
```

**Processing options:**

**Option A: Auto-apply with audit trail**
- Apply MCP corrections immediately
- Mark as `source = "mcp"`, confidence = 0.95
- Log in audit table for user review
- User can revert if incorrect

**Option B: Queue for user approval**
- Store as pending corrections
- Show in UI: "AI suggested these improvements"
- User approves/rejects each
- Approved corrections become `source = "user"`

**Recommendation:** Start with Option A for low-risk changes (descriptions, names), Option B for structural changes (new relationships, entity merges).

---

## The Refresh Processor

A background worker that processes the event queue:

```
┌─────────────────────────────────────────────────────────┐
│                   Refresh Processor                      │
│                                                         │
│  1. Dequeue next event (by priority, then created_at)  │
│  2. Resolve affected ontology items                     │
│  3. Check if items are user-verified (skip if so)      │
│  4. Apply deterministic changes immediately             │
│  5. Queue LLM enrichment for non-deterministic          │
│  6. Update provenance (source, confidence, timestamps)  │
│  7. Mark event as processed                             │
└─────────────────────────────────────────────────────────┘
```

### Deterministic vs LLM-Required Changes

**Deterministic (apply immediately):**
- Entity creation from new table with PK
- Relationship creation from FK constraint
- Entity removal when table removed
- User/MCP corrections

**LLM-Required (queue for enrichment):**
- Entity name humanization ("billing_activities" → "Billing Activity")
- Entity description generation
- Domain classification
- Key column identification
- Relationship description

### Knowledge-Aware LLM Enrichment

When calling LLM for enrichment, inject project knowledge:

```
## Project Knowledge

The user has provided the following clarifications:
- "Customer" refers to paying users, not trial accounts
- The "reservations" table represents property bookings, not restaurant reservations
- "billing_activities" should be called "Billing Activity" (singular)

## Previous Corrections

The following corrections were made by AI clients and accepted:
- Entity "users" renamed to "User" with description "A registered platform user"
- Relationship added: User → Account (owns)

## Task

Given this context, enrich the following new entity...
```

This ensures:
1. User knowledge persists across refreshes
2. LLM learns from past corrections
3. Consistency with existing ontology style

---

## MCP Tool: `update_ontology`

### Tool Definition

```json
{
  "name": "update_ontology",
  "description": "Report ontology issues or suggest corrections. Use when you encounter entity names that don't match the data, missing relationships, or incorrect descriptions.",
  "input_schema": {
    "type": "object",
    "properties": {
      "corrections": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "correction_type": {
              "enum": ["entity_name", "entity_description", "entity_domain",
                       "missing_relationship", "wrong_relationship", "missing_entity",
                       "column_description", "alias_suggestion"]
            },
            "target": {
              "description": "Entity ID, relationship ID, or column reference"
            },
            "suggestion": {
              "description": "The corrected value or new item to add"
            },
            "confidence": {
              "type": "number",
              "description": "How confident you are (0.0-1.0)"
            },
            "reason": {
              "description": "Why this correction is needed"
            }
          }
        }
      }
    }
  }
}
```

### Example Usage

MCP client discovers an issue while generating a query:

```json
{
  "corrections": [
    {
      "correction_type": "missing_relationship",
      "target": {
        "source_entity": "Reservation",
        "target_entity": "Property"
      },
      "suggestion": {
        "relationship_type": "belongs_to",
        "via_column": "reservations.property_id",
        "description": "A reservation is for a specific property"
      },
      "confidence": 0.95,
      "reason": "Query required joining reservations to properties via property_id FK, but relationship was not in ontology"
    }
  ]
}
```

### Response

```json
{
  "accepted": [
    {
      "correction_type": "missing_relationship",
      "status": "applied",
      "relationship_id": "new-uuid"
    }
  ],
  "rejected": [],
  "pending_review": []
}
```

---

## Schema Diffing Strategy

### Fingerprint Computation

```go
func computeSchemaFingerprint(tables, columns, fks) string {
    // Sort for deterministic ordering
    sort.Slice(tables, ...)
    sort.Slice(columns, ...)
    sort.Slice(fks, ...)

    // Hash the concatenated representation
    h := sha256.New()
    for _, t := range tables {
        h.Write([]byte(t.Schema + "." + t.Name))
    }
    for _, c := range columns {
        h.Write([]byte(c.Table + "." + c.Name + ":" + c.Type))
    }
    for _, fk := range fks {
        h.Write([]byte(fk.Source + "->" + fk.Target))
    }
    return hex.EncodeToString(h.Sum(nil))
}
```

### Diff Algorithm

```go
func computeSchemaDiff(old, new SchemaSnapshot) []SchemaChange {
    var changes []SchemaChange

    // Tables
    oldTables := set(old.Tables)
    newTables := set(new.Tables)

    for t := range newTables - oldTables {
        changes = append(changes, TableAdded{t})
    }
    for t := range oldTables - newTables {
        changes = append(changes, TableRemoved{t})
    }

    // Similar for columns, FKs...

    return changes
}
```

---

## Database Schema Changes

### New/Modified Tables

```sql
-- Add provenance fields to entities
ALTER TABLE engine_ontology_entities ADD COLUMN source TEXT DEFAULT 'ddl';
ALTER TABLE engine_ontology_entities ADD COLUMN confidence FLOAT DEFAULT 0.5;
ALTER TABLE engine_ontology_entities ADD COLUMN verified_by_user BOOLEAN DEFAULT FALSE;
ALTER TABLE engine_ontology_entities ADD COLUMN last_verified_at TIMESTAMP;
ALTER TABLE engine_ontology_entities ADD COLUMN is_stale BOOLEAN DEFAULT FALSE;

-- Add provenance fields to relationships
ALTER TABLE engine_entity_relationships ADD COLUMN source TEXT DEFAULT 'ddl';
ALTER TABLE engine_entity_relationships ADD COLUMN verified_by_user BOOLEAN DEFAULT FALSE;
ALTER TABLE engine_entity_relationships ADD COLUMN last_verified_at TIMESTAMP;
ALTER TABLE engine_entity_relationships ADD COLUMN is_stale BOOLEAN DEFAULT FALSE;

-- Refresh event queue
CREATE TABLE engine_ontology_refresh_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    priority INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    processed_at TIMESTAMP,
    error_message TEXT,

    CONSTRAINT valid_event_type CHECK (event_type IN (
        'schema_table_added', 'schema_table_removed',
        'schema_column_added', 'schema_fk_added',
        'user_answer', 'chat_refinement', 'mcp_feedback'
    ))
);

CREATE INDEX idx_refresh_events_pending
    ON engine_ontology_refresh_events(project_id, priority DESC, created_at)
    WHERE processed_at IS NULL;

-- Schema fingerprint tracking
ALTER TABLE engine_ontologies ADD COLUMN schema_fingerprint TEXT;
ALTER TABLE engine_ontologies ADD COLUMN last_schema_check TIMESTAMP;
```

---

## UI Considerations

### Ontology Tile Badge

The Ontology tile on the project dashboard should display a badge when there are pending queue items:

```
┌─────────────────────────┐     ┌─────────────────────────┐
│                         │     │                    (3)  │
│      Ontology           │     │      Ontology      🔴   │
│                         │     │                         │
│   No pending changes    │     │   3 pending changes     │
└─────────────────────────┘     └─────────────────────────┘
       (no badge)                    (badge shown)
```

**Badge triggers:**
- Pending schema changes detected (DDL diff)
- New enum values discovered
- Unanswered ontology questions
- Unprocessed MCP feedback
- Unprocessed chat refinements

**Badge behavior:**
- Shows count of total pending items
- Color: yellow/orange for info, red if stale items exist
- Clicking tile navigates to ontology with pending panel open
- Badge clears when queue is empty (after refresh or dismissal)

**API endpoint:**
```
GET /api/projects/{id}/ontology/pending-count

Response: { "count": 3, "has_stale": false }
```

This count should be cheap to compute (single query on refresh_events table + questions table).

### Refresh Status Indicator

Show in ontology header:
- "Ontology up to date" (green)
- "3 pending changes" (yellow) - click to review
- "Schema changed - refresh available" (blue)

### Pending Changes Panel

List queued events with:
- Event type icon
- Brief description
- "Apply" / "Dismiss" buttons for user control
- "Apply All" for batch processing

### Item Provenance Display

On entity/relationship detail:
- Source badge: "DDL" | "AI Enriched" | "User Verified" | "MCP Corrected"
- Last verified timestamp
- "Mark as verified" button

### MCP Suggestions Panel

When MCP corrections are pending approval:
- Show diff view (current → suggested)
- Accept / Reject buttons
- "Trust this AI client" option to auto-approve future suggestions

---

## Implementation Phases

### Phase 1: Foundation
- Add provenance fields (source, confidence, verified_by_user) to entities & relationships
- Modify re-extract to preserve user-verified items
- Store user answers in project_knowledge with structured format

### Phase 2: Event Queue
- Create refresh_events table
- Build event processor skeleton
- Implement immediate processing for user answers & chat refinements

### Phase 3: Schema Diffing
- Implement fingerprint computation
- Build diff algorithm
- Emit schema change events
- UI: "Check for schema changes" button

### Phase 4: Selective Enrichment
- Modify LLM calls to inject project_knowledge
- Skip user-verified items during re-enrichment
- Process only affected items from schema diff

### Phase 5: MCP Integration
- Implement `update_ontology` tool
- Define correction types and validation
- Build approval workflow for structural changes
- Auto-apply low-risk changes (descriptions, names)

### Phase 6: Real-time & Polish
- Background processor for event queue
- UI refresh status indicators
- Provenance display on all items
- "Full Re-extract" as explicit nuclear option

---

## Open Questions

1. **Conflict resolution:** What happens if MCP suggests a change that contradicts a user verification?
   - Option: User always wins, MCP suggestion is logged but not applied
   - Option: Show conflict to user for resolution

2. **Staleness threshold:** How long before an unverified item is considered stale?
   - Option: Time-based (30 days)
   - Option: Schema-change-based (stale if schema changed since last verification)

3. **MCP trust levels:** Should different MCP clients have different trust levels?
   - A well-tested internal tool vs. a new experimental agent

4. **Batch vs. streaming:** Should the processor run continuously or on-demand?
   - On-demand is simpler, continuous is more responsive

5. **Versioning:** Should we keep history of ontology changes for rollback?
   - Current: no versioning
   - Future: could snapshot ontology state before major changes

---

## Success Metrics

1. **LLM call reduction:** Refresh should use <20% of the LLM calls of full re-extract
2. **Knowledge preservation:** User-verified items survive 100% of refreshes
3. **MCP adoption:** X corrections submitted via update_ontology per month
4. **Time to refresh:** Schema-only refresh completes in <10 seconds (no LLM)
5. **User satisfaction:** Reduced "I already told you this" complaints
