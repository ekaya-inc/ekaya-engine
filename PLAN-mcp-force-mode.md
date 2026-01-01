# PLAN: MCP Force Mode (Pre-Approved Queries Only)

## Purpose

Implement the "FORCE all access through Pre-Approved Queries" mode where MCP clients can ONLY execute pre-approved queries. No dynamic SQL generation, no schema access, no developer tools.

**Design Philosophy:** Precise matching, not fuzzy search. The MCP client must determine if a pre-approved query **exactly** answers the user's question. If no query is an exact match, the client should tell the user "no matching query available" rather than execute a close-but-wrong query.

---

## Current State

### What Exists

| Component | Status | Location |
|-----------|--------|----------|
| `list_approved_queries` tool | Implemented | `pkg/mcp/tools/queries.go:104-172` |
| `execute_approved_query` tool | Implemented | `pkg/mcp/tools/queries.go:174-267` |
| `ForceMode` config field | Implemented | `pkg/models/mcp_config.go:15` |
| Tool filtering logic | **Buggy** | `pkg/mcp/tools/developer.go:150-171` |
| Query model | Implemented | `pkg/models/query.go` |
| Query service | Implemented | `pkg/services/query.go` |

### Known Bugs

1. **Schema tools not filtered** (`pkg/mcp/tools/developer.go:150-171`)
   - `get_schema` always appears regardless of configuration
   - `schemaToolNames` is defined in `schema.go:29-31` but never checked in `filterTools`

2. **Approved queries tools may not appear** (`pkg/mcp/tools/developer.go:131-143`)
   - `ShouldShowApprovedQueriesTools` requires both:
     - `approved_queries` tool group enabled
     - At least one enabled query exists
   - If no enabled queries exist, tools don't appear even when group is enabled

---

## Phase 1: Fix Tool Filtering

### 1.1 Add Schema Tool Filtering

**File:** `pkg/mcp/tools/developer.go`

The `filterTools` function needs to filter schema tools when Developer Tools is disabled:

```go
// filterTools filters tools based on visibility flags for each tool group.
func filterTools(tools []mcp.Tool, showDeveloper, showExecute, showApprovedQueries bool) []mcp.Tool {
    filtered := make([]mcp.Tool, 0, len(tools))
    for _, tool := range tools {
        // Check developer tools
        if developerToolNames[tool.Name] {
            if !showDeveloper {
                continue
            }
            if tool.Name == "execute" && !showExecute {
                continue
            }
        }

        // Check schema tools - tied to developer tools visibility
        // ADD THIS BLOCK:
        if schemaToolNames[tool.Name] && !showDeveloper {
            continue
        }

        // Check approved_queries tools
        if approvedQueriesToolNames[tool.Name] && !showApprovedQueries {
            continue
        }

        filtered = append(filtered, tool)
    }
    return filtered
}
```

**Rationale:** The UI says Developer Tools enables "raw access to the Datasource **and Schema**". The `get_schema` tool should be controlled by the Developer Tools toggle.

### 1.2 Import schemaToolNames

**File:** `pkg/mcp/tools/developer.go`

The `schemaToolNames` variable is defined in `schema.go`. Either:
- Export it (`SchemaToolNames`) and import in `developer.go`
- Or move the variable to a shared location

Simplest fix: In `schema.go:29`, change to exported:
```go
// SchemaToolNames lists all tools in the schema group.
var SchemaToolNames = map[string]bool{
    "get_schema": true,
}
```

Then reference `SchemaToolNames` in `developer.go`.

### 1.3 Force Mode Disables Developer Tools

**File:** `pkg/mcp/tools/developer.go`

In `NewToolFilter`, when `ForceMode` is enabled, developer tools should be forcibly disabled:

```go
// Check if force mode is enabled (approved_queries only)
approvedQueriesConfig, err := deps.MCPConfigService.GetToolGroupConfig(tenantCtx, projectID, "approved_queries")
if err != nil {
    deps.Logger.Error("Tool filter: failed to get approved_queries config", ...)
}

forceMode := approvedQueriesConfig != nil && approvedQueriesConfig.ForceMode

// Force mode overrides developer tools
if forceMode {
    showDeveloper = false
    showExecute = false
}
```

---

## Phase 2: list_approved_queries Response

### 2.1 Current Response Structure (Already Implemented)

The current `list_approved_queries` returns all the essential fields:

```json
{
  "queries": [
    {
      "id": "uuid",
      "name": "Total revenue by customer",
      "description": "Additional context explaining what this query does and doesn't do",
      "sql": "SELECT u.name, SUM(o.total_amount) FROM users u JOIN orders o ON ...",
      "parameters": [
        {
          "name": "start_date",
          "type": "date",
          "description": "Start of date range (inclusive)",
          "required": true,
          "default": null
        }
      ],
      "dialect": "postgres"
    }
  ]
}
```

**Field Mapping:**

| JSON Field | Model Field | Purpose |
|------------|-------------|---------|
| `name` | `NaturalLanguagePrompt` | Primary question the query answers |
| `description` | `AdditionalContext` | **CRITICAL:** Detailed context for precise matching |
| `sql` | `SQLQuery` | The actual SQL (helps MCP client understand what's returned) |
| `parameters` | `Parameters[]` | What dimensions can be filtered |

**Location:** `pkg/mcp/tools/queries.go:87-94` (struct), `pkg/mcp/tools/queries.go:154-166` (population)

### 2.2 The description Field is Critical for Matching

The `description` field (from `AdditionalContext` in the model) is what allows the MCP client to determine if a query **exactly** matches the user's question. The MCP client should use this field to:

1. **Understand scope** - What the query includes and excludes
2. **Identify constraints** - Assumptions baked into the query
3. **Disambiguate** - Distinguish between similar-sounding queries

**Example of a well-written description:**
```
Returns total order revenue grouped by customer for completed orders only.

Includes:
- Orders with status 'completed'
- All product types

Excludes:
- Pending orders (status = 'pending')
- Cancelled orders (status = 'cancelled')
- Refunded amounts

Note: Revenue is order.total_amount, not sum of line items.
Date filtering is based on order.created_at, not delivery date.
```

### 2.3 Future Enhancement: Structured Fields (Optional)

If free-text `description` proves insufficient for matching, consider adding structured fields to the Query model:

| Field | Purpose | Implementation |
|-------|---------|----------------|
| `output_columns` | Describes what data is returned | Parse from SQL or store in model |
| `constraints` | Explicit limitations/assumptions | Store in model |

**Recommendation:** Start with the current approach (rich `description` text). Only add structured fields if MCP clients struggle with matching.

---

## Phase 3: MCP Client Matching Strategy

This section provides guidance for MCP clients (like Claude Code) on how to match user questions to pre-approved queries.

### 3.1 Matching Algorithm

```
1. User asks: "What was our revenue last month?"

2. MCP client calls list_approved_queries()

3. For each query, evaluate:
   a. Does the NAME describe what the user wants?
   b. Do the PARAMETERS allow filtering to what the user specified?
   c. Do the CONSTRAINTS rule out this query?
   d. Does the DESCRIPTION confirm or deny this is the right query?

4. Classification:
   - EXACT_MATCH: Query precisely answers the question
   - PARTIAL_MATCH: Query could answer but requires clarification
   - NO_MATCH: Query doesn't answer this question

5. Decision:
   - If exactly ONE EXACT_MATCH: Execute it
   - If multiple EXACT_MATCH: Ask user to clarify
   - If only PARTIAL_MATCH: Ask user if this query works
   - If NO_MATCH for all: Tell user "no matching query available"
```

### 3.2 Conservative Matching

**The MCP client should be CONSERVATIVE:**

- If unsure whether a query matches, classify as PARTIAL_MATCH
- Never assume a query covers something not explicitly stated
- Treat missing constraints as "unknown" not "unrestricted"

**Example:**

User: "Show me revenue by product"
Query name: "Total revenue by customer"

This is a NO_MATCH because:
- User wants grouping by product
- Query groups by customer
- Even though both are "revenue", the grouping dimension is different

### 3.3 Parameter Extraction

When a query matches, the MCP client must extract parameter values from the user's question:

```
User: "What was revenue in January 2024?"

Query: "Total revenue by customer for a date range"
Parameters: start_date (date), end_date (date)

Extraction:
- "January 2024" → start_date: "2024-01-01", end_date: "2024-01-31"
```

If parameter extraction is ambiguous, ASK the user rather than guess.

---

## Phase 4: Admin Guidelines for Query Metadata

### 4.1 Writing Effective Natural Language Prompts

**Good:** "Total revenue by customer for a date range"
**Bad:** "Revenue report"

The prompt should include:
- The metric (what is being calculated)
- The grouping dimension (how results are organized)
- The filter dimensions (what parameters control)

### 4.2 Writing Effective Additional Context

The `additional_context` field should clarify:
- What the query INCLUDES
- What the query EXCLUDES
- Assumptions made by the query

**Example:**
```
Returns total order revenue grouped by customer for completed orders only.

Includes:
- Orders with status 'completed'
- All product types

Excludes:
- Pending orders
- Cancelled orders
- Refunded amounts

Note: Revenue is calculated from order total_amount, not line item prices.
```

### 4.3 Documenting Parameters

Each parameter description should include:
- What the parameter filters
- Expected format
- Edge case behavior

**Example:**
```json
{
  "name": "start_date",
  "type": "date",
  "description": "Start of date range (inclusive). Orders created on this date ARE included. Format: YYYY-MM-DD",
  "required": true
}
```

---

## Phase 5: Execute Approved Query

### 5.1 Current Implementation

The existing `execute_approved_query` tool is mostly complete:

**Input:**
```json
{
  "query_id": "uuid",
  "parameters": {
    "start_date": "2024-01-01",
    "end_date": "2024-01-31"
  },
  "limit": 100
}
```

**Output:**
```json
{
  "columns": ["name", "revenue"],
  "rows": [
    {"name": "Acme Corp", "revenue": 15000.00},
    {"name": "Beta Inc", "revenue": 12500.00}
  ],
  "row_count": 2,
  "truncated": false
}
```

### 5.2 Enhancements Needed

1. **Include query metadata in response** - helps MCP client explain what was executed:

```json
{
  "query_name": "Total revenue by customer for a date range",
  "parameters_used": {
    "start_date": "2024-01-01",
    "end_date": "2024-01-31"
  },
  "columns": [...],
  "rows": [...],
  "row_count": 2,
  "truncated": false,
  "execution_time_ms": 145
}
```

2. **Better error messages** - when parameters are invalid:

```json
{
  "error": true,
  "error_type": "parameter_validation",
  "message": "Parameter 'start_date' is required",
  "query_name": "Total revenue by customer for a date range"
}
```

---

## Implementation Checklist

### Bug Fixes (Do First)

- [x] Export `SchemaToolNames` in `pkg/mcp/tools/schema.go`
- [x] Add schema tool filtering to `filterTools()` in `pkg/mcp/tools/developer.go`
- [x] Write tests for tool filtering with schema tools
- [ ] Add Force Mode check to disable developer tools in `NewToolFilter()`

### Enhancements

- [ ] Add `query_name` and `parameters_used` to execute response
- [ ] Add `execution_time_ms` to execute response
- [ ] Improve error messages with query context

### Future (Optional)

- [ ] Add `output_columns` to Query model
- [ ] Add `constraints` to Query model
- [ ] Database migration for new fields
- [ ] Update admin UI to capture new fields
- [ ] Parse SELECT columns from SQL as fallback

---

## File Changes Summary

| File | Change |
|------|--------|
| `pkg/mcp/tools/schema.go` | Export `SchemaToolNames` |
| `pkg/mcp/tools/developer.go` | Add schema filtering, Force Mode check |
| `pkg/mcp/tools/queries.go` | Enhance execute response |
| `pkg/mcp/tools/developer_filter_test.go` | Add Force Mode test cases |

---

## Testing Strategy

### Unit Tests

1. **Tool Filtering Tests** (`developer_filter_test.go`)
   - Force Mode ON → only `list_approved_queries`, `execute_approved_query`, `health`
   - Force Mode OFF + Developer ON → all developer tools + approved queries
   - Developer OFF + Approved Queries ON → only approved queries + health
   - Schema tools hidden when Developer Tools disabled

2. **Query Execution Tests** (`queries_test.go`)
   - Response includes `query_name`
   - Response includes `parameters_used`
   - Error responses include query context

### Integration Tests

1. Connect as MCP client with Force Mode enabled
2. Verify only approved query tools appear
3. List queries, execute one, verify response format

### Manual Testing

1. Enable Force Mode in UI
2. Reconnect Claude Code (`/mcp`)
3. Verify `tools/list` shows only approved query tools
4. Ask a question that matches a pre-approved query
5. Verify correct query is executed

---

## Appendix: Expected Tool List by Mode

| Mode | Tools Available |
|------|-----------------|
| Force Mode ON | `health`, `list_approved_queries`, `execute_approved_query` |
| Pre-Approved ON, Developer OFF | `health`, `list_approved_queries`, `execute_approved_query` |
| Pre-Approved ON, Developer ON | `health`, `list_approved_queries`, `execute_approved_query`, `get_schema`, `query`, `sample`, `validate`, `echo` |
| Pre-Approved ON, Developer ON, Execute ON | Above + `execute` |
| Pre-Approved OFF, Developer ON | `health`, `get_schema`, `query`, `sample`, `validate`, `echo` |
