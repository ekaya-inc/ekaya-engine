# PLAN: MCP Suggest and Approve Query

## Purpose

Enable MCP clients (like Claude Code) to suggest new queries that administrators can review and approve. This creates a controlled pathway for AI-assisted query creation while maintaining admin oversight.

**Key Stakeholders:**
- MCP clients: Can propose queries when they can't find a suitable pre-approved one
- Administrators: Review, modify, and approve/reject suggested queries
- End users: Benefit from an expanding library of vetted queries

---

## Implementation Status

| Component | Status | Notes |
|-----------|--------|-------|
| ToolGroupConfig.AllowClientSuggestions | ✅ Done | Field exists in model |
| UI toggle for Allow Suggestions | ✅ Done | Toggle in MCP Server page |
| Query.ApprovalStatus field | ❌ Pending | Need to add to model |
| Query.SuggestedBy/SuggestedAt fields | ❌ Pending | Need to add to model |
| Database migration | ❌ Pending | Add approval columns |
| suggest_query MCP tool | ❌ Pending | Core feature |
| Admin approval UI | ❌ Pending | Review queue interface |
| Notification system | ❌ Future | Alert admins of pending suggestions |

---

## Feature Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Query Suggestion Flow                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. MCP Client encounters question without matching pre-approved query       │
│                              ↓                                               │
│  2. Client calls suggest_query with:                                         │
│     - Natural language description                                           │
│     - Generated SQL                                                          │
│     - Parameter definitions (optional)                                       │
│                              ↓                                               │
│  3. System validates SQL syntax and creates pending query                    │
│                              ↓                                               │
│  4. Admin receives notification (future) or sees in review queue             │
│                              ↓                                               │
│  5. Admin reviews:                                                           │
│     - Approve as-is → Query becomes enabled                                  │
│     - Approve with edits → Admin modifies, then enables                      │
│     - Reject → Query marked rejected with reason                             │
│                              ↓                                               │
│  6. If approved, query appears in list_approved_queries for all clients      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Data Model Updates

### 1.1 Add ApprovalStatus to Query Model

**File:** `pkg/models/query.go`

```go
type ApprovalStatus string

const (
    ApprovalStatusApproved ApprovalStatus = "approved"  // Admin approved, ready for use
    ApprovalStatusPending  ApprovalStatus = "pending"   // Awaiting admin review
    ApprovalStatusRejected ApprovalStatus = "rejected"  // Admin rejected
)

// Add to Query struct:
type Query struct {
    // ... existing fields ...

    // Suggestion tracking
    ApprovalStatus ApprovalStatus `json:"approval_status" db:"approval_status"`
    SuggestedBy    *string        `json:"suggested_by,omitempty" db:"suggested_by"`
    SuggestedAt    *time.Time     `json:"suggested_at,omitempty" db:"suggested_at"`
    ReviewedBy     *string        `json:"reviewed_by,omitempty" db:"reviewed_by"`
    ReviewedAt     *time.Time     `json:"reviewed_at,omitempty" db:"reviewed_at"`
    RejectionReason *string       `json:"rejection_reason,omitempty" db:"rejection_reason"`
}
```

### 1.2 Database Migration

**File:** `migrations/NNNN_add_query_approval_fields.sql`

```sql
-- Add approval workflow fields to engine_queries
ALTER TABLE engine_queries
ADD COLUMN approval_status VARCHAR(20) NOT NULL DEFAULT 'approved';

ALTER TABLE engine_queries
ADD COLUMN suggested_by VARCHAR(255);

ALTER TABLE engine_queries
ADD COLUMN suggested_at TIMESTAMPTZ;

ALTER TABLE engine_queries
ADD COLUMN reviewed_by VARCHAR(255);

ALTER TABLE engine_queries
ADD COLUMN reviewed_at TIMESTAMPTZ;

ALTER TABLE engine_queries
ADD COLUMN rejection_reason TEXT;

-- Index for finding pending suggestions
CREATE INDEX idx_queries_pending ON engine_queries(project_id, approval_status)
WHERE approval_status = 'pending' AND deleted_at IS NULL;

-- Existing queries are already approved (created by admins)
COMMENT ON COLUMN engine_queries.approval_status IS
    'approved = ready for use, pending = awaiting review, rejected = declined by admin';
```

### 1.3 Update Query Repository

**File:** `pkg/repositories/query.go`

Add methods:
```go
// ListPending returns queries awaiting approval
ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.Query, error)

// UpdateApprovalStatus updates the approval status of a query
UpdateApprovalStatus(ctx context.Context, projectID, queryID uuid.UUID, status models.ApprovalStatus, reviewedBy string, reason *string) error

// CountPending returns count of pending suggestions for a project
CountPending(ctx context.Context, projectID uuid.UUID) (int, error)
```

---

## Phase 2: MCP Tool Implementation

### 2.1 suggest_query Tool

**File:** `pkg/mcp/tools/queries.go`

```go
// Tool: suggest_query
// Purpose: Submit a new query for admin approval

Input:
{
  "natural_language": "Total revenue by product category for last quarter",  // Required
  "sql": "SELECT c.name, SUM(oi.quantity * oi.unit_price) as revenue FROM ...",  // Required
  "parameters": [  // Optional
    {
      "name": "start_date",
      "type": "date",
      "description": "Start of date range",
      "required": true
    }
  ],
  "context": "User asked about category performance"  // Optional: why this query is needed
}

Output (success):
{
  "suggestion_id": "uuid",
  "status": "pending",
  "message": "Query submitted for admin approval. You will be able to use it once approved.",
  "estimated_review_time": null  // Future: based on admin response patterns
}

Output (validation error):
{
  "error": true,
  "error_type": "validation_failed",
  "message": "SQL syntax error at position 45",
  "details": {
    "sql_error": "column 'categorys' does not exist",
    "suggestion": "Did you mean 'categories'?"
  }
}

Output (feature disabled):
{
  "error": true,
  "error_type": "feature_disabled",
  "message": "Query suggestions are not enabled for this project. Contact the administrator."
}

MCP Annotations:
- read_only_hint: false
- destructive_hint: false
- idempotent_hint: false  // Creates new record each time
```

### 2.2 Tool Registration

**File:** `pkg/mcp/tools/queries.go`

Add to `approvedQueriesToolNames`:
```go
var approvedQueriesToolNames = map[string]bool{
    "list_approved_queries":   true,
    "execute_approved_query":  true,
    "suggest_query":           true,  // NEW
}
```

### 2.3 Suggestion Visibility Check

The `suggest_query` tool should only appear when:
1. `approved_queries` tool group is enabled, AND
2. `allowClientSuggestions` sub-option is true

**File:** `pkg/mcp/tools/developer.go`

Update `NewToolFilter` to handle suggest_query separately:
```go
// suggest_query has additional requirement: allowClientSuggestions must be true
if tool.Name == "suggest_query" {
    if !showApprovedQueries || !allowClientSuggestions {
        continue
    }
}
```

---

## Phase 3: Query Service Updates

### 3.1 Add Suggestion Methods

**File:** `pkg/services/query.go`

```go
// SuggestQueryRequest contains fields for suggesting a new query
type SuggestQueryRequest struct {
    NaturalLanguagePrompt string                  `json:"natural_language_prompt"`
    SQLQuery              string                  `json:"sql_query"`
    Parameters            []models.QueryParameter `json:"parameters,omitempty"`
    Context               string                  `json:"context,omitempty"`
    SuggestedBy           string                  `json:"suggested_by"`  // User ID from claims
}

// Suggest creates a new query in pending status
func (s *queryService) Suggest(ctx context.Context, projectID, datasourceID uuid.UUID, req *SuggestQueryRequest) (*models.Query, error)

// ApproveQuery approves a pending query
func (s *queryService) ApproveQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string) error

// RejectQuery rejects a pending query with a reason
func (s *queryService) RejectQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string, reason string) error

// ListPending returns all pending suggestions for a project
func (s *queryService) ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.Query, error)
```

### 3.2 Validation on Suggest

When a query is suggested:
1. Validate SQL syntax (use existing validation)
2. Check that referenced tables are in the approved schema
3. Validate parameter definitions if provided
4. Do NOT execute the query (that happens after approval)

---

## Phase 4: Admin API Endpoints

### 4.1 List Pending Suggestions

```
GET /api/projects/{pid}/datasources/{did}/queries/pending

Response:
{
  "queries": [
    {
      "id": "uuid",
      "natural_language_prompt": "Total revenue by category",
      "sql_query": "SELECT ...",
      "parameters": [...],
      "suggested_by": "user@example.com",
      "suggested_at": "2024-12-15T10:00:00Z",
      "approval_status": "pending"
    }
  ],
  "count": 3
}
```

### 4.2 Approve Query

```
POST /api/projects/{pid}/datasources/{did}/queries/{qid}/approve

Response:
{
  "id": "uuid",
  "approval_status": "approved",
  "is_enabled": true,
  "reviewed_by": "admin@example.com",
  "reviewed_at": "2024-12-15T11:00:00Z"
}
```

### 4.3 Reject Query

```
POST /api/projects/{pid}/datasources/{did}/queries/{qid}/reject
Body:
{
  "reason": "This query exposes sensitive salary data"
}

Response:
{
  "id": "uuid",
  "approval_status": "rejected",
  "rejection_reason": "This query exposes sensitive salary data",
  "reviewed_by": "admin@example.com",
  "reviewed_at": "2024-12-15T11:00:00Z"
}
```

---

## Phase 5: Admin UI

### 5.1 Pending Suggestions Badge

Add a badge to the Pre-Approved Queries tile showing pending count:

```
┌─────────────────────────────────────────┐
│ Pre-Approved Queries              [3]   │  ← Badge shows pending count
│                                         │
│ 12 queries enabled                      │
│ 3 pending review                        │
└─────────────────────────────────────────┘
```

### 5.2 Review Queue

Add a "Pending Review" tab to the Pre-Approved Queries page:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Pre-Approved Queries                                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│ [All Queries]  [Pending Review (3)]  [Rejected]                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│ ┌──────────────────────────────────────────────────────────────────────────┐│
│ │ "Total revenue by product category for last quarter"                     ││
│ │ Suggested by: claude-code-user@company.com                               ││
│ │ Suggested: 2 hours ago                                                   ││
│ │                                                                          ││
│ │ SQL:                                                                     ││
│ │ ┌────────────────────────────────────────────────────────────────────┐  ││
│ │ │ SELECT c.name as category,                                         │  ││
│ │ │        SUM(oi.quantity * oi.unit_price) as revenue                 │  ││
│ │ │ FROM categories c                                                  │  ││
│ │ │ JOIN products p ON p.category_id = c.id                            │  ││
│ │ │ JOIN order_items oi ON oi.product_id = p.id                        │  ││
│ │ │ JOIN orders o ON o.id = oi.order_id                                │  ││
│ │ │ WHERE o.created_at >= {{start_date}}                               │  ││
│ │ │ GROUP BY c.name                                                    │  ││
│ │ │ ORDER BY revenue DESC                                              │  ││
│ │ └────────────────────────────────────────────────────────────────────┘  ││
│ │                                                                          ││
│ │ Parameters: start_date (date, required)                                  ││
│ │                                                                          ││
│ │ [Test Query]  [Edit & Approve]  [Approve]  [Reject]                      ││
│ └──────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.3 Rejection Dialog

When rejecting, prompt for reason:

```
┌─────────────────────────────────────────────────────────────────┐
│ Reject Query Suggestion                                    [×]  │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Please provide a reason for rejection:                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ This query exposes sensitive salary data that should not    │ │
│ │ be accessible via MCP.                                      │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│ This reason will be logged for audit purposes.                  │
│                                                                 │
│                              [Cancel]  [Reject Query]           │
└─────────────────────────────────────────────────────────────────┘
```

---

## Phase 6: Security Considerations

### 6.1 Rate Limiting

Prevent suggestion spam:
- Max 10 suggestions per user per hour
- Max 50 pending suggestions per project

### 6.2 SQL Validation

Before accepting a suggestion:
1. Parse SQL to validate syntax
2. Extract table references
3. Verify all tables are in approved schema (is_selected = true)
4. Reject if query references unauthorized tables

### 6.3 Audit Logging

Log all suggestion events:
- `query_suggested` - New suggestion submitted
- `query_approved` - Admin approved
- `query_rejected` - Admin rejected
- `query_edited` - Admin modified before approving

---

## Phase 7: MCP Client Experience

### 7.1 Workflow for Claude Code

When Claude Code can't find a matching pre-approved query:

```
1. User asks: "What's our revenue by region?"

2. Claude calls list_approved_queries()
   → No matching query found

3. Claude generates SQL based on ontology

4. Claude calls suggest_query({
     natural_language: "Revenue breakdown by region",
     sql: "SELECT region, SUM(amount) FROM orders GROUP BY region",
     context: "User needed regional revenue analysis"
   })

5. Response: "Query submitted for approval (ID: abc123)"

6. Claude tells user: "I've submitted a query suggestion for admin
   approval. Once approved, this query will be available for future use.
   In the meantime, I can try a different approach or you can wait for
   approval."
```

### 7.2 Checking Suggestion Status (Future)

```
Tool: get_suggestion_status
Input: { "suggestion_id": "uuid" }
Output: {
  "status": "pending",  // pending, approved, rejected
  "submitted_at": "2024-12-15T10:00:00Z",
  "rejection_reason": null
}
```

---

## File Changes Summary

| Phase | File | Change |
|-------|------|--------|
| 1 | `pkg/models/query.go` | Add ApprovalStatus, SuggestedBy, etc. |
| 1 | `migrations/NNNN_*.sql` | Add approval columns |
| 1 | `pkg/repositories/query.go` | Add ListPending, UpdateApprovalStatus |
| 2 | `pkg/mcp/tools/queries.go` | Add suggest_query tool |
| 2 | `pkg/mcp/tools/developer.go` | Update filter for suggest_query |
| 3 | `pkg/services/query.go` | Add Suggest, Approve, Reject methods |
| 4 | `pkg/handlers/query.go` | Add approval API endpoints |
| 5 | `ui/src/pages/QueriesPage.tsx` | Add pending review tab |
| 5 | `ui/src/components/` | QueryReviewCard, RejectDialog |

---

## Testing Strategy

### Unit Tests
- Query service: Suggest, Approve, Reject flows
- Tool visibility based on allowClientSuggestions setting
- Validation of suggested SQL

### Integration Tests
- Full flow: suggest → approve → appears in list
- Full flow: suggest → reject → does not appear
- Rate limiting enforcement

### Manual Testing
- MCP client suggests query
- Admin sees in UI
- Admin approves/rejects
- Query appears/doesn't appear in MCP tools

---

## Open Questions

1. **Should rejected queries be permanently stored or soft-deleted?**
   - Recommendation: Keep for audit trail, filter from UI by default

2. **Can the suggester see their rejected queries?**
   - Recommendation: Yes, with rejection reason (helps them improve)

3. **Should there be a "request changes" status?**
   - Would allow back-and-forth between suggester and admin
   - Adds complexity, defer to V2

4. **Email notifications for pending suggestions?**
   - Useful for admins to respond quickly
   - Defer to Phase 7 or separate plan
