# PLAN: User Questions in Ontology Extraction

## Problem Statement

Current ontology extraction generates over 100 individual questions (~1 question per 4 columns). This volume is unrealistic for users to answer. The workflow interrupts extraction to ask questions, creating a poor user experience.

## Solution: Table Confirmation Panels with LLM Guesses

Instead of asking 100 individual questions during extraction, the LLM makes best-guess descriptions for all entities. Users review and confirm tables one at a time through prioritized Table Confirmation Panels.

## Architecture Changes

### 1. Ontology Extraction Behavior

**Current:** Extraction pauses when LLM encounters unknown values (enums, unclear columns) and generates questions.

**New:** Extraction runs to completion. LLM generates descriptions for every table and column:
- **Can determine meaning:** Store LLM's best guess with `confidence: "high"` or `confidence: "medium"`
- **Cannot determine meaning:** Store placeholder guess with `confidence: "low"` and `requires_user_input: true`
- Never interrupt extraction to ask questions

### 2. Data Model

#### engine_ontologies Table Changes

```sql
-- Add to entity_summaries JSONB structure
{
  "tables": {
    "users": {
      "description": "User accounts and authentication data",
      "confidence": "high",
      "requires_user_input": false,
      "user_confirmed": false
    }
  }
}

-- Add to column_details JSONB structure
{
  "users.status": {
    "description": "User account status (active, suspended, deleted)",
    "confidence": "medium",
    "requires_user_input": false,
    "user_confirmed": false,
    "source": "llm_guess"  // or "user_modified"
  },
  "orders.status_code": {
    "description": "Numeric status code",
    "confidence": "low",
    "requires_user_input": true,
    "user_confirmed": false,
    "source": "llm_guess",
    "enum_values": [1, 2, 3, 4, 5]  // Store actual values for context
  }
}
```

#### New Table: engine_table_confirmations

Track table-level confirmation state:

```sql
CREATE TABLE engine_table_confirmations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),
    ontology_id UUID NOT NULL REFERENCES engine_ontologies(id),
    table_name TEXT NOT NULL,
    priority_score INT NOT NULL,  -- For sorting
    has_required_questions BOOLEAN NOT NULL,
    confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(ontology_id, table_name)
);
```

#### engine_ontology_questions Changes

**Current:** Questions generated during extraction and presented individually.

**New:** Questions are still generated but used differently:
- Generate questions during extraction for entities with `requires_user_input: true`
- Store in `engine_ontology_questions` with `entity_key` pointing to table.column
- Use questions as prompts in Table Confirmation Panel
- Mark `answered_at` when user provides answer in panel

### 3. Table Prioritization Logic

Calculate `priority_score` for each table based on:

```go
type TablePriority struct {
    TableName string
    Score     int
}

func CalculateTablePriority(tableName string, metadata TableMetadata) int {
    score := 0

    // Required questions = highest priority
    if metadata.HasRequiredQuestions {
        score += 1000
    }

    // Number of relationships (FK references)
    score += metadata.RelationshipCount * 10

    // Number of columns
    score += metadata.ColumnCount

    // Important column presence
    if metadata.HasColumn("user_id") { score += 50 }
    if metadata.HasColumn("created_at") { score += 20 }
    if metadata.HasColumn("id") { score += 10 }

    return score
}
```

### 4. Frontend API Flow

#### Endpoint: GET /api/projects/{id}/ontology/tables/next

Returns next table for user confirmation:

```json
{
  "table_name": "orders",
  "table_description": "Customer orders and purchase history",
  "table_confidence": "high",
  "columns": [
    {
      "name": "status_code",
      "description": "Numeric status code",
      "confidence": "low",
      "requires_input": true,
      "question": "The status_code column has enum values [1,2,3,4,5] that we couldn't determine. What do they mean?",
      "enum_values": [1, 2, 3, 4, 5]
    },
    {
      "name": "created_at",
      "description": "Time this order was created",
      "confidence": "high",
      "requires_input": false
    },
    {
      "name": "updated_at",
      "description": "Time this order was last updated",
      "confidence": "high",
      "requires_input": false
    }
  ],
  "priority_score": 1250,
  "has_required_questions": true
}
```

#### Endpoint: POST /api/projects/{id}/ontology/tables/{tableName}/confirm

Request body:

```json
{
  "table_description": "Customer orders and purchase history",  // May be unchanged or user-modified
  "columns": [
    {
      "name": "status_code",
      "description": "1=pending, 2=processing, 3=shipped, 4=delivered, 5=cancelled",
      "modified": true
    },
    {
      "name": "created_at",
      "description": "Time this order was created",
      "modified": false
    }
  ]
}
```

Response (immediate, non-blocking):

```json
{
  "status": "confirmed",
  "next_table": {
    "table_name": "users",
    ...
  }
}
```

### 5. Backend Processing

#### Synchronous Path (No Changes)

If user clicks [Approve All] without modifications:

```go
func (s *OntologyService) ConfirmTable(ctx context.Context, req ConfirmTableRequest) error {
    if !req.HasChanges() {
        // Mark all guesses as confirmed
        return s.repo.MarkTableConfirmed(ctx, req.ProjectID, req.TableName)
    }

    // Has changes - async path
    return s.processTableChangesAsync(ctx, req)
}
```

#### Asynchronous Path (User Made Changes)

For any user modifications:

```go
func (s *OntologyService) processTableChangesAsync(ctx context.Context, req ConfirmTableRequest) error {
    // Mark table as confirmed immediately (optimistic)
    if err := s.repo.MarkTableConfirmed(ctx, req.ProjectID, req.TableName); err != nil {
        return err
    }

    // Launch async processing for each changed entity
    for _, col := range req.Columns {
        if col.Modified {
            go s.processColumnChange(context.Background(), ProcessColumnChangeRequest{
                ProjectID:   req.ProjectID,
                TableName:   req.TableName,
                ColumnName:  col.Name,
                NewDescription: col.Description,
            })
        }
    }

    // If table description changed
    if req.TableDescriptionModified {
        go s.processTableChange(context.Background(), ProcessTableChangeRequest{
            ProjectID:      req.ProjectID,
            TableName:      req.TableName,
            NewDescription: req.TableDescription,
        })
    }

    return nil
}
```

#### LLM Processing for Changes

When user modifies a guess, send to LLM with table context:

```go
func (s *OntologyService) processColumnChange(ctx context.Context, req ProcessColumnChangeRequest) {
    // Get table schema for context
    schema := s.repo.GetTableSchema(ctx, req.ProjectID, req.TableName)

    // Ask LLM to update ontology based on user input
    prompt := fmt.Sprintf(`
The user has provided this description for column %s.%s:
%s

Based on this description and the table schema below, update the ontology entry for this column.
Include usage patterns, data type implications, and relationships.

Schema:
%s
`, req.TableName, req.ColumnName, req.NewDescription, schema)

    response := s.llm.Chat(ctx, prompt)

    // Update ontology with LLM's expanded description
    s.repo.UpdateColumnOntology(ctx, req.ProjectID, req.TableName, req.ColumnName, OntologyUpdate{
        Description:    req.NewDescription,
        UserConfirmed:  true,
        Source:         "user_modified",
        LLMAnalysis:    response.Analysis,
        Confidence:     "high",
    })
}
```

### 6. UI Task Indicator

**Challenge:** Current task list is tied to active workflow. Need to show async ontology updates without blocking workflow.

**Solution:** Add task tracking independent of workflow state:

```sql
CREATE TABLE engine_background_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),
    task_type TEXT NOT NULL,  -- 'ontology_update', 'schema_analysis', etc.
    description TEXT NOT NULL,
    status TEXT NOT NULL,  -- 'pending', 'processing', 'completed', 'failed'
    created_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);
```

When user confirms table with changes:

```go
// Create background task
task := BackgroundTask{
    ProjectID:   projectID,
    TaskType:    "ontology_update",
    Description: fmt.Sprintf("Updating %s", tableName),
    Status:      "processing",
}
s.repo.CreateBackgroundTask(ctx, task)

// Frontend polls GET /api/projects/{id}/tasks for active tasks
// Shows "Updating orders" in task list
```

### 7. UI Component Structure

```
TableConfirmationPanel
├── TableHeader
│   ├── TableName (editable on click)
│   └── ApproveAllButton (enabled when required questions answered)
├── TableDescription (editable on click X)
├── ColumnsList
│   ├── RequiredColumns (sorted first)
│   │   └── ColumnItem (with required text input)
│   └── OptionalColumns
│       └── ColumnItem (editable on click X)
└── ActionButtons
    ├── SkipButton (mark table for later review)
    ├── DeleteButton (exclude table from ontology)
    └── ApproveAllButton (confirm and move to next)
```

#### Component Behavior

**Edit State:**
- Click (x) on description → Show text input with current value
- Click [OK] → Update local state, mark as modified
- No save until [Approve All]

**Approve All:**
- Collect all modifications (table description + column descriptions)
- POST to /confirm endpoint
- Immediately render next table from response
- Show background task for this table if modifications exist

**Skip:**
- Move to next table without confirming
- Table remains in queue, deprioritized

**Delete:**
- Mark table as excluded from ontology
- Remove from confirmation queue

## Implementation Phases

### Phase 1: Data Model Changes
- Add confidence, requires_user_input, user_confirmed fields to ontology JSONB
- Create engine_table_confirmations table
- Create engine_background_tasks table
- Update ontology extraction to store guesses instead of generating immediate questions

### Phase 2: Backend API
- Implement table prioritization logic
- Create GET /tables/next endpoint
- Create POST /tables/{name}/confirm endpoint
- Implement async processing for user changes
- Background task tracking

### Phase 3: Frontend UI
- Build TableConfirmationPanel component
- Implement edit-on-click for descriptions
- Wire up API endpoints
- Add background task indicator

### Phase 4: LLM Integration
- Update extraction prompts to always generate best guesses
- Implement LLM processing for user modifications
- Add ontology update logic based on user input

## Testing Strategy

### Unit Tests
- Table prioritization algorithm
- Confidence scoring logic
- Async task creation and tracking

### Integration Tests
- Full extraction → confirmation flow
- User modification → LLM update → ontology storage
- Background task lifecycle

### Manual Testing
- Extract ontology for test_data database
- Verify all tables have guesses
- Walk through Table Confirmation Panels
- Modify descriptions and verify async updates
- Check background task indicators

## Success Metrics

- Ontology extraction completes without user interruption
- User sees prioritized tables (required questions first)
- [Approve All] immediately shows next table (no blocking)
- Background tasks update ontology without blocking UI
- Final ontology contains mix of LLM guesses and user confirmations

## Open Questions

1. **Background task polling frequency** - How often should frontend poll for task updates?
2. **Task retention** - When to delete completed background tasks?
3. **Confidence thresholds** - What confidence level triggers requires_user_input?
4. **Skip behavior** - Should skipped tables reappear later or require explicit "Review Skipped" action?
5. **Bulk operations** - Should we support "Approve All Tables" for confident users?
