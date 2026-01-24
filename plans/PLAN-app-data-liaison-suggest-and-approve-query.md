# PLAN: Query Management Tools for AI Data Liaison

## Purpose

Implement a complete query management system with two distinct tool groups:

1. **Base Tools** (all users) - Query discovery and execution
2. **AI Data Liaison Tools** (Business Users - paid product) - Suggest queries for admin review
3. **Dev Tools** (Administrators) - Direct query management and suggestion review

This creates a governed workflow where Business Users (using ChatGPT, Claude Desktop, Copilot, etc.) can propose queries that Admins review and approve, while Admins retain full direct control.

---

## Tool Hierarchy

### Base Tools (Available to All)
| Tool | Description |
|------|-------------|
| `list_approved_queries` | List pre-approved SQL queries |
| `execute_approved_query` | Execute a pre-approved query by ID |

### AI Data Liaison Tools (Business Users - Paid Product)
| Tool | Description |
|------|-------------|
| `suggest_approved_query` | Suggest a new query for admin approval (creates pending record) |
| `suggest_query_update` | Suggest updates to existing query (creates pending record with parent_query_id) |

### Dev Tools (Administrators)
| Tool | Description |
|------|-------------|
| `list_query_suggestions` | Get all pending suggestions for review |
| `approve_query_suggestion` | Approve a pending suggestion |
| `reject_query_suggestion` | Reject a pending suggestion with reason |
| `create_approved_query` | Create a new approved query directly (no review) |
| `update_approved_query` | Update existing query directly (no pending record) |
| `delete_approved_query` | Delete a query |

**Naming pattern:** Business Users "suggest" (requires review), Devs take direct action (no review).

---

## Implementation Status

| Component | Status | Notes |
|-----------|--------|-------|
| **Data Model** | | |
| Query.Status field | ✅ Done | "pending", "approved", "rejected" |
| Query.SuggestedBy field | ✅ Done | "user", "agent", "admin" |
| Query.SuggestionContext field | ✅ Done | JSONB for validation results |
| Query.ReviewedBy/ReviewedAt fields | ❌ Pending | Task 1.1 |
| Query.RejectionReason field | ❌ Pending | Task 1.1 |
| Query.ParentQueryID field | ❌ Pending | Task 1.1 |
| Database migration (audit) | ❌ Pending | Task 1.2 |
| **Repository** | | |
| ListPending method | ❌ Pending | Task 1.3 |
| CountPending method | ❌ Pending | Task 1.3 |
| UpdateApprovalStatus method | ❌ Pending | Task 1.3 |
| **Service** | | |
| SuggestUpdate method | ❌ Pending | Task 1.4 |
| ApproveQuery method | ❌ Pending | Task 1.4 |
| RejectQuery method | ❌ Pending | Task 1.4 |
| DirectCreate method | ❌ Pending | Task 1.4 |
| DirectUpdate method | ❌ Pending | Task 1.4 |
| **Business User MCP Tools** | | |
| suggest_approved_query | ✅ Done | Creates pending record |
| suggest_query_update | ❌ Pending | Task 2.1 |
| **Dev MCP Tools** | | |
| list_query_suggestions | ❌ Pending | Task 3.1 |
| approve_query_suggestion | ❌ Pending | Task 3.2 |
| reject_query_suggestion | ❌ Pending | Task 3.3 |
| create_approved_query | ❌ Pending | Task 3.4 |
| update_approved_query | ❌ Pending | Task 3.5 |
| delete_approved_query | ❌ Pending | Task 3.6 |
| **Tool Group Configuration** | | |
| ToolGroupConfig.AllowClientSuggestions | ✅ Done | Field exists in model |
| UI toggle for Allow Suggestions | ✅ Done | Toggle in MCP Server page |
| Dev tools tool group | ❌ Pending | Task 4.1 |
| **Admin REST API** | | |
| GET /queries/pending | ❌ Pending | Task 5.1 |
| POST /queries/{id}/approve | ❌ Pending | Task 5.2 |
| POST /queries/{id}/reject | ❌ Pending | Task 5.3 |
| **Admin UI** | | |
| Pending count badge | ❌ Pending | Task 6.1 |
| Pending filter option | ❌ Pending | Task 6.2 |
| Review card with diff | ❌ Pending | Task 6.3 |
| Rejection dialog | ❌ Pending | Task 6.4 |

---

## Task List

### Phase 1: Data Model & Repository

#### Task 1.1: Add Audit Fields to Query Model
**File:** `pkg/models/query.go`

Add fields:
```go
// Audit trail fields
ReviewedBy      *string    `json:"reviewed_by,omitempty" db:"reviewed_by"`
ReviewedAt      *time.Time `json:"reviewed_at,omitempty" db:"reviewed_at"`
RejectionReason *string    `json:"rejection_reason,omitempty" db:"rejection_reason"`

// Update suggestion tracking
ParentQueryID *uuid.UUID `json:"parent_query_id,omitempty" db:"parent_query_id"`
```

#### Task 1.2: Database Migration
**File:** `migrations/0XX_add_query_approval_audit.up.sql`

```sql
ALTER TABLE engine_queries
ADD COLUMN reviewed_by VARCHAR(255),
ADD COLUMN reviewed_at TIMESTAMPTZ,
ADD COLUMN rejection_reason TEXT,
ADD COLUMN parent_query_id UUID REFERENCES engine_queries(id);

CREATE INDEX idx_queries_pending_review
ON engine_queries(project_id, datasource_id)
WHERE status = 'pending' AND deleted_at IS NULL;

CREATE INDEX idx_queries_parent
ON engine_queries(parent_query_id)
WHERE parent_query_id IS NOT NULL AND deleted_at IS NULL;
```

#### Task 1.3: Repository Methods
**File:** `pkg/repositories/query_repository.go`

Add methods:
```go
ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.Query, error)
ListPendingByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error)
CountPending(ctx context.Context, projectID uuid.UUID) (int, error)
GetPendingUpdatesForQuery(ctx context.Context, projectID, queryID uuid.UUID) ([]*models.Query, error)
UpdateApprovalStatus(ctx context.Context, projectID, queryID uuid.UUID, status string, reviewedBy string, reason *string) error
```

#### Task 1.4: Service Methods
**File:** `pkg/services/query.go`

Add methods:
```go
// For Business User tools (creates pending records)
SuggestUpdate(ctx context.Context, projectID uuid.UUID, req *SuggestUpdateRequest) (*models.Query, error)

// For Dev tools (direct operations)
DirectCreate(ctx context.Context, projectID uuid.UUID, req *CreateQueryRequest) (*models.Query, error)
DirectUpdate(ctx context.Context, projectID, queryID uuid.UUID, req *UpdateQueryRequest) (*models.Query, error)
Delete(ctx context.Context, projectID, queryID uuid.UUID) error

// For approval workflow
ApproveQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string) error
RejectQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string, reason string) error
ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.Query, error)
```

---

### Phase 2: Business User MCP Tools (AI Data Liaison)

#### Task 2.1: suggest_query_update
**File:** `pkg/mcp/tools/queries.go`

Creates a pending change record for admin review.

```go
var suggestQueryUpdateTool = mcp.NewTool(
    "suggest_query_update",
    mcp.WithDescription(`Suggest an update to an existing pre-approved query.
The suggestion will be reviewed by an administrator before being applied.
The original query remains active until the update is approved.`),
    mcp.WithString("query_id", mcp.Required(), mcp.Description("UUID of the existing query to update")),
    mcp.WithString("sql", mcp.Description("Updated SQL query")),
    mcp.WithString("name", mcp.Description("Updated name")),
    mcp.WithString("description", mcp.Description("Updated description")),
    mcp.WithArray("parameters", mcp.Description("Updated parameter definitions")),
    mcp.WithObject("output_column_descriptions", mcp.Description("Updated output column descriptions")),
    mcp.WithArray("tags", mcp.Description("Updated tags for organizing queries")),
    mcp.WithString("context", mcp.Required(), mcp.Description("Explanation of why this update is needed")),
)
```

**Behavior:**
1. Fetch original query
2. Validate updated SQL if provided
3. Create new query record with `status="pending"` and `parent_query_id` pointing to original
4. Return suggestion ID and confirmation message

---

### Phase 3: Dev MCP Tools

#### Task 3.1: list_query_suggestions
**File:** `pkg/mcp/tools/dev_queries.go` (new file)

```go
var listQuerySuggestionsTool = mcp.NewTool(
    "list_query_suggestions",
    mcp.WithDescription(`List all pending query suggestions awaiting review.
Returns both new query suggestions and update suggestions for existing queries.`),
    mcp.WithString("status", mcp.Description("Filter by status: pending, approved, rejected (default: pending)")),
    mcp.WithString("datasource_id", mcp.Description("Filter by datasource UUID")),
)
```

**Response format:**
```json
{
  "suggestions": [
    {
      "id": "uuid",
      "type": "new",
      "name": "Get user orders",
      "sql": "SELECT ...",
      "suggested_by": "agent",
      "created_at": "2024-12-15T10:00:00Z",
      "context": "User requested this query..."
    },
    {
      "id": "uuid",
      "type": "update",
      "parent_query_id": "original-uuid",
      "parent_query_name": "Subscribe to list",
      "name": "Subscribe to list",
      "sql": "SELECT * FROM subscribe_to_list(...)",
      "suggested_by": "agent",
      "created_at": "2024-12-15T10:00:00Z",
      "context": "Added duplicate prevention",
      "changes": ["sql", "parameters"]
    }
  ],
  "count": 2
}
```

#### Task 3.2: approve_query_suggestion
```go
var approveQuerySuggestionTool = mcp.NewTool(
    "approve_query_suggestion",
    mcp.WithDescription(`Approve a pending query suggestion.
For new queries: Sets status to approved and enables the query.
For update suggestions: Applies changes to the original query and deletes the pending record.`),
    mcp.WithString("suggestion_id", mcp.Required(), mcp.Description("UUID of the pending suggestion to approve")),
)
```

**Behavior:**
1. If `parent_query_id` is null → new query: set status="approved", is_enabled=true
2. If `parent_query_id` is not null → update: copy fields to original, soft-delete pending record

#### Task 3.3: reject_query_suggestion
```go
var rejectQuerySuggestionTool = mcp.NewTool(
    "reject_query_suggestion",
    mcp.WithDescription(`Reject a pending query suggestion with a reason.
The suggestion will be marked as rejected and the reason will be recorded.`),
    mcp.WithString("suggestion_id", mcp.Required(), mcp.Description("UUID of the pending suggestion to reject")),
    mcp.WithString("reason", mcp.Required(), mcp.Description("Explanation for why the suggestion was rejected")),
)
```

#### Task 3.4: create_approved_query
```go
var createApprovedQueryTool = mcp.NewTool(
    "create_approved_query",
    mcp.WithDescription(`Create a new pre-approved query directly (no review required).
The query will be immediately available for execution.`),
    mcp.WithString("name", mcp.Required(), mcp.Description("Human-readable name for the query")),
    mcp.WithString("description", mcp.Required(), mcp.Description("What business question this query answers")),
    mcp.WithString("sql", mcp.Required(), mcp.Description("SQL query with {{parameter}} placeholders")),
    mcp.WithString("datasource_id", mcp.Required(), mcp.Description("UUID of the datasource")),
    mcp.WithArray("parameters", mcp.Description("Parameter definitions")),
    mcp.WithObject("output_column_descriptions", mcp.Description("Descriptions for output columns")),
    mcp.WithArray("tags", mcp.Description("Tags for organizing queries")),
)
```

**Behavior:**
- Creates query with `status="approved"`, `is_enabled=true`, `suggested_by="admin"`
- Validates SQL syntax before creation
- Returns created query details

#### Task 3.5: update_approved_query
```go
var updateApprovedQueryTool = mcp.NewTool(
    "update_approved_query",
    mcp.WithDescription(`Update an existing pre-approved query directly (no review required).
Changes are applied immediately.`),
    mcp.WithString("query_id", mcp.Required(), mcp.Description("UUID of the query to update")),
    mcp.WithString("sql", mcp.Description("Updated SQL query")),
    mcp.WithString("name", mcp.Description("Updated name")),
    mcp.WithString("description", mcp.Description("Updated description")),
    mcp.WithArray("parameters", mcp.Description("Updated parameter definitions")),
    mcp.WithObject("output_column_descriptions", mcp.Description("Updated output column descriptions")),
    mcp.WithArray("tags", mcp.Description("Updated tags")),
    mcp.WithBoolean("is_enabled", mcp.Description("Enable or disable the query")),
)
```

**Behavior:**
- Updates query directly (no pending record)
- Validates updated SQL if provided
- Returns updated query details

#### Task 3.6: delete_approved_query
```go
var deleteApprovedQueryTool = mcp.NewTool(
    "delete_approved_query",
    mcp.WithDescription(`Delete a pre-approved query.
The query will be soft-deleted and no longer available for execution.
Any pending update suggestions for this query will also be rejected.`),
    mcp.WithString("query_id", mcp.Required(), mcp.Description("UUID of the query to delete")),
)
```

**Behavior:**
- Soft-deletes the query
- Auto-rejects any pending update suggestions with reason "Original query was deleted"
- Returns confirmation

---

### Phase 4: Tool Group Configuration

#### Task 4.1: Register Tool Groups
**File:** `pkg/mcp/tools/tool_filter.go`

Add AI Data Liaison tools (Business User):
```go
var aiDataLiaisonToolNames = map[string]bool{
    "suggest_approved_query":  true,
    "suggest_query_update":    true,
}
```

Add dev query tools:
```go
var devQueryToolNames = map[string]bool{
    "list_query_suggestions":    true,
    "approve_query_suggestion":  true,
    "reject_query_suggestion":   true,
    "create_approved_query":     true,
    "update_approved_query":     true,
    "delete_approved_query":     true,
}
```

---

### Phase 5: Admin REST API

#### Task 5.1: List Pending Endpoint
```
GET /api/projects/{pid}/queries/pending

Response:
{
  "queries": [...],
  "count": 2
}
```

#### Task 5.2: Approve Endpoint
```
POST /api/projects/{pid}/queries/{qid}/approve

Response:
{
  "success": true,
  "message": "Query approved and enabled",
  "query": { ... }
}
```

#### Task 5.3: Reject Endpoint
```
POST /api/projects/{pid}/queries/{qid}/reject
Body: { "reason": "..." }

Response:
{
  "success": true,
  "message": "Query rejected",
  "query": { ... }
}
```

---

### Phase 6: Admin UI

#### Task 6.1: Pending Count Badge
Show pending count in Queries page header.

#### Task 6.2: Filter Options
Add "Pending review" and "Rejected" to filter dropdown.

#### Task 6.3: Review Card with Diff
When viewing a pending suggestion:
- Show suggestion context
- For updates: show side-by-side diff with original
- Show [Approve] and [Reject] buttons

#### Task 6.4: Rejection Dialog
Modal to enter rejection reason before confirming rejection.

---

## Flows

### Business User Suggests New Query
```
1. Business User (via ChatGPT/Claude/Copilot) encounters need for query
2. MCP Client calls suggest_approved_query
3. System validates SQL, creates query with status="pending"
4. Admin sees in list_query_suggestions or UI
5. Admin approves → query becomes available
   Admin rejects → query marked rejected with reason
```

### Business User Suggests Query Update
```
1. Business User identifies improvement to existing query
2. MCP Client calls suggest_query_update
3. System creates pending record with parent_query_id
4. Original query continues working
5. Admin reviews diff, approves or rejects
6. If approved: original updated, pending deleted
   If rejected: pending marked rejected, original unchanged
```

### Admin Creates Query Directly
```
1. Admin (via Claude Code or similar) creates query
2. MCP Client calls create_approved_query
3. System validates and creates with status="approved"
4. Query immediately available for execution
```

### Admin Updates Query Directly
```
1. Admin identifies need to update query
2. MCP Client calls update_approved_query
3. System validates and updates directly
4. Changes immediately effective
```

---

## Security Considerations

### Rate Limiting
- Max 10 suggestions per agent session per hour
- Max 50 pending suggestions per project total

### Validation
- Parse SQL to validate syntax before accepting suggestions
- Verify SQL is safe (no DROP, TRUNCATE outside allowed patterns)
- For updates: verify access to original query's datasource

### Audit Logging
Log all events:
- `query_suggested` - New query suggested via `suggest_approved_query`
- `query_update_suggested` - Update suggested via `suggest_query_update`
- `query_approved` - Admin approved via `approve_query_suggestion`
- `query_rejected` - Admin rejected via `reject_query_suggestion`
- `query_created_direct` - Admin created via `create_approved_query`
- `query_updated_direct` - Admin updated via `update_approved_query`
- `query_deleted` - Query deleted via `delete_approved_query`

---

## Edge Cases

### Original query deleted while update is pending
- Auto-reject pending updates with reason "Original query was deleted"

### Multiple pending updates for same query
- Allow multiple pending updates
- Admin can approve one (others remain pending or can be rejected)
- If one is approved, others should show warning that original has changed

### Query update changes parameters
- Existing executions using old parameters may fail
- Consider: version queries instead of updating in place (future enhancement)

---

## Testing Strategy

### Unit Tests
- SuggestUpdate creates pending record with parent_query_id
- ApproveQuery for new: enables and sets status
- ApproveQuery for update: updates original, deletes pending
- RejectQuery sets status and reason
- DirectCreate creates with status="approved"
- DirectUpdate updates without creating pending record
- Delete soft-deletes and auto-rejects pending updates

### Integration Tests
- Full flow: suggest → approve → available
- Full flow: suggest update → approve → original updated
- Full flow: suggest → reject → marked rejected
- Dev flow: create directly → immediately available
- Dev flow: update directly → immediately changed
- Concurrent: two pending suggestions for same query

### Manual Testing via MCP
1. As Business User: suggest query, verify pending
2. As Admin: list suggestions, approve one
3. Verify approved query available in list_approved_queries
4. As Business User: suggest update to existing query
5. As Admin: review diff, approve
6. Verify original query updated
7. Test rejection flow with reason
