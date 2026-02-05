# ISSUES: Ontology Benchmark API Testing - 2026-01-30

Issues discovered during comprehensive MCP API testing before production push.

---

## Critical Issues

### 1. Sensitive Data Exposure in sample_values

**Severity**: CRITICAL
**Tool**: `get_context` with `include: ["sample_values"]`
**Status**: In Progress (split into subtasks)

**Description**: When requesting sample_values, sensitive data is exposed. The `agent_data` column in `users` table shows LiveKit API keys and secrets in plaintext.

**Reproduction**:
```
get_context(depth='columns', tables=['users'], include=['sample_values'])
```

**Observed output**:
```json
{
  "column_name": "agent_data",
  "sample_values": [
    "",
    "{\"livekit_url\":\"wss://tikragents-xxx.livekit.cloud\",\"livekit_api_key\":\"API67e2wiyw3KvB\",\"livekit_api_secret\":\"MATPBGtZAPGGxyslrsjHaZjN3W6KsU2pIfdwNHMfR0i\",\"livekit_agent_id\":\"kitt\"}"
  ]
}
```

**Expected**: Columns containing sensitive patterns (api_key, api_secret, password, token, credential) should be redacted or excluded from sample_values.

---

#### 1.1 Add sensitive column detection with regex patterns

- [x] **Status**: Complete

Create a reusable sensitive column detector in the MCP tools package that identifies columns containing secrets based on naming patterns and content analysis.

**Files to create/modify:**
- Create `pkg/mcp/tools/sensitive.go` - New file for sensitive data detection

**Implementation details:**

1. Create a `SensitiveDetector` struct with configurable patterns:
```go
type SensitiveDetector struct {
    columnPatterns  []*regexp.Regexp  // patterns for column names
    contentPatterns []*regexp.Regexp  // patterns for JSON keys in content
}
```

2. Default column name patterns to detect (case-insensitive):
   - `(?i)(api[_-]?key|apikey)`
   - `(?i)(api[_-]?secret|apisecret)`
   - `(?i)(password|passwd|pwd)`
   - `(?i)(secret[_-]?key|secretkey)`
   - `(?i)(access[_-]?token|accesstoken)`
   - `(?i)(auth[_-]?token|authtoken)`
   - `(?i)(private[_-]?key|privatekey)`
   - `(?i)(credential|cred)`
   - `(?i)(bearer[_-]?token)`
   - `(?i)(client[_-]?secret)`

3. Default content patterns to detect secrets in JSON values:
   - `"(api_key|api_secret|password|token|secret|credential|private_key|livekit_api_key|livekit_api_secret)"\s*:\s*"[^"]+"`

4. Implement methods:
   - `IsSensitiveColumn(columnName string) bool` - checks column name against patterns
   - `IsSensitiveContent(content string) bool` - checks if content contains secret patterns
   - `RedactContent(content string) string` - replaces secret values with `[REDACTED]`

5. Add unit tests in `pkg/mcp/tools/sensitive_test.go`:
   - Test column name detection for all patterns
   - Test JSON content detection (the LiveKit example from the issue)
   - Test redaction preserves JSON structure
   - Test case-insensitivity

**Acceptance criteria:**
- All secret patterns from the issue are detected
- The LiveKit `agent_data` example would be flagged as sensitive
- Redaction produces valid JSON with `[REDACTED]` replacing secret values

---

#### 1.2 Integrate sensitive detection into get_context sample_values output

- [x] **Status**: Complete

Modify the `get_context` MCP tool to use the SensitiveDetector when returning sample_values, automatically redacting sensitive data.

**Files to modify:**
- `pkg/mcp/tools/context.go` - The get_context tool implementation
- `pkg/mcp/tools/schema.go` - May also need updates if sample_values logic is shared

**Implementation details:**

1. Import and instantiate `SensitiveDetector` (use default patterns)

2. In the sample_values collection logic, before adding values to the response:
   - Check if column name matches sensitive patterns via `IsSensitiveColumn()`
   - If column is sensitive, either:
     - Option A: Exclude sample_values entirely for that column and add a note
     - Option B: Redact each sample value via `RedactContent()`
   - For non-sensitive columns with JSON content, still check `IsSensitiveContent()` and redact if needed

3. Add a `redacted` boolean field or `redaction_note` string to the column output when values are redacted, so the MCP client knows why values appear as `[REDACTED]`

4. Test manually with the reproduction case from the issue:
   ```
   get_context(depth='columns', tables=['users'], include=['sample_values'])
   ```
   The `agent_data` column should show redacted LiveKit secrets.

**Acceptance criteria:**
- `get_context` with `sample_values` no longer exposes LiveKit API keys/secrets
- Redacted columns are clearly marked in the response
- Non-sensitive columns are unaffected
- JSON structure is preserved in redacted values

---

#### 1.3 Add column-level sensitive flag to ontology schema and MCP tools

- [x] **Status**: Complete

Extend the ontology data model to support an explicit `is_sensitive` flag on columns, allowing manual override of automatic detection.

**Files to modify:**
- `pkg/models/ontology.go` or equivalent - Add `IsSensitive` field to column metadata model
- Database migration file (new) - Add `is_sensitive` boolean column to `engine_schema_columns` table
- `pkg/mcp/tools/schema.go` - Include `is_sensitive` in schema output
- `pkg/mcp/tools/context.go` - Check `is_sensitive` flag in addition to pattern detection
- `pkg/mcp/tools/column.go` (update_column tool) - Allow setting `is_sensitive` flag

**Implementation details:**

1. Add migration to add `is_sensitive BOOLEAN DEFAULT NULL` to `engine_schema_columns`:
   - NULL = use automatic detection
   - TRUE = always treat as sensitive (manual override)
   - FALSE = never treat as sensitive (manual override)

2. Update column metadata model:
```go
type ColumnMetadata struct {
    // existing fields...
    IsSensitive *bool `json:"is_sensitive,omitempty"`
}
```

3. Update `update_column` MCP tool to accept optional `sensitive` parameter:
   - Add to tool schema: `"sensitive": {"type": "boolean", "description": "Mark column as containing sensitive data"}`
   - Update handler to persist the flag

4. Update `get_schema` to include `is_sensitive` in column output when set

5. Update sensitive detection logic in `get_context`:
   - If `IsSensitive == true`: always redact
   - If `IsSensitive == false`: never redact (even if patterns match)
   - If `IsSensitive == nil`: use automatic pattern detection

6. Add integration test that:
   - Creates a column with `is_sensitive: true`
   - Verifies sample_values are redacted
   - Updates to `is_sensitive: false`
   - Verifies sample_values are not redacted

**Acceptance criteria:**
- New database column exists with proper migration
- `update_column` accepts `sensitive` parameter
- `get_schema` shows `is_sensitive` flag when set
- Manual flag overrides automatic detection in both directions
- Existing columns without explicit flag continue to use automatic detection

---

## High Priority Issues

### 2. create_approved_query Error Message Unclear

**Severity**: HIGH (UX)
**Tool**: `create_approved_query`
**Status**: FIXED

**Description**: When `output_column_descriptions` is not provided, the error message is confusing.

**Fix Applied**: Error message updated to:
```
"output_column_descriptions parameter is required. Provide descriptions for output columns, e.g., {\"total\": \"Total count of records\"}"
```

See `pkg/services/query.go:216`.

---

### 3. probe_relationship Returns Empty for Known Relationships

**Severity**: HIGH
**Tool**: `probe_relationship`
**Status**: N/A - Tool Removed

**Description**: When probing for relationships between entities that should be related (User -> Account), the tool returns empty results.

**Resolution**: The `probe_relationship` tool was removed as part of the entity removal refactor. Relationships are now handled through the schema-based FK discovery in `get_context` and `get_schema`.

---

### 4. get_query_history Returns Empty

**Severity**: MEDIUM
**Tool**: `get_query_history`
**Status**: Fixed

**Description**: Query history appears to not be recording executed queries.

**Reproduction**:
```
# Execute several queries via execute_approved_query and query tools
get_query_history(limit=10, hours_back=24)
```

**Observed output**:
```json
{"recent_queries": null, "count": 0, "hours_back": 24}
```

**Expected**: Should show recently executed queries.

**Root cause**: The `query` and `execute` tools in `developer.go` were not logging query executions to the `engine_query_executions` table. Only `execute_approved_query` was logging.

**Fix**: Added query execution logging to both `query` and `execute` tools via `logQueryExecution()`. Created a `QueryLoggingDeps` interface that both `MCPToolDeps` and `QueryToolDeps` implement, allowing the logging function to be shared.

---

## Medium Priority Issues

### 5. Glossary Terms with Invalid SQL Functions

**Severity**: MEDIUM
**Tool**: Glossary enrichment
**Status**: Open

**Description**: 3 out of 10 glossary terms have SQL that fails validation due to PostgreSQL function compatibility.

**Affected terms**:
| Term | Error |
|------|-------|
| Engagement Completion Rate | SQL references non-existent columns: =, ) |
| Engagement Duration | SQL references non-existent columns: EPOCH |
| Engagement Duration Per User | SQL references non-existent columns: DATE_PART, (, =, ) |

**Analysis**: The SQL validator may be misinterpreting PostgreSQL functions like `EXTRACT(EPOCH FROM ...)` and `DATE_PART(...)` as column references.

**Recommendation**:
- Improve SQL parser to recognize PostgreSQL date/time functions
- Update glossary terms with compatible SQL syntax
- Consider using simpler SQL patterns that avoid complex functions

---

### 6. refresh_schema Reports 634 Columns Added on Every Run

**Severity**: LOW
**Tool**: `refresh_schema`
**Status**: FIXED

**Description**: Running refresh_schema reports 634 columns added even when no schema changes occurred.

**Fix Applied**: The `columns_added` field now reports only NEW columns, not total columns upserted.

See test: `TestRefreshSchema_ColumnsAdded_ReportsOnlyNewColumns_Integration` in `pkg/mcp/tools/refresh_schema_integration_test.go:226`.

---

## Observations (Not Issues)

### Tools Working Correctly

All tools below passed testing:

**Core Tools**:
- `health` - Returns correct status
- `echo` - Returns input correctly
- `get_schema` - Returns full schema with entities
- `query` - Executes SELECT queries correctly
- `validate` - Validates SQL syntax
- `sample` - Returns sample rows
- `explain_query` - Returns execution plan

**Context Tools**:
- `get_context` (all depths: domain, entities, tables, columns)
- `get_ontology` - Returns entity information
- `search_schema` - Returns relevant results

**Probe Tools**:
- `probe_column` - Returns statistics and features
- `probe_columns` - Batch probe working

**Entity/Ontology Tools**:
- `get_entity` - Returns entity details
- `update_entity` - Creates/updates entities
- `delete_entity` - Soft deletes entities
- `update_column` - Updates column metadata
- `delete_column_metadata` - Removes custom metadata
- `update_relationship` - Creates relationships
- `delete_relationship` - Removes relationships

**Glossary Tools**:
- `list_glossary` - Lists all terms
- `get_glossary_sql` - Returns SQL for terms
- `create_glossary_term` - Creates new terms (with validation against test data)
- `update_glossary_term` - Updates existing terms
- `delete_glossary_term` - Removes terms

**Approved Query Tools**:
- `list_approved_queries` - Lists queries
- `create_approved_query` - Creates queries (requires output_column_descriptions)
- `execute_approved_query` - Executes queries with parameters
- `update_approved_query` - Updates query metadata
- `delete_approved_query` - Removes queries (auto-rejects pending suggestions)

**Query Suggestion Tools**:
- `suggest_approved_query` - Creates pending suggestions
- `list_query_suggestions` - Lists pending suggestions
- `approve_query_suggestion` - Approves suggestions
- `reject_query_suggestion` - Rejects with reason
- `suggest_query_update` - Suggests updates to existing queries

**Ontology Question Tools**:
- `list_ontology_questions` - Lists by status
- `resolve_ontology_question` - Marks as answered

**Project Knowledge Tools**:
- `update_project_knowledge` - Creates/updates facts
- `delete_project_knowledge` - Removes facts

**Change Management Tools**:
- `list_pending_changes` - Lists pending changes
- `approve_all_changes` - Batch approval
- `scan_data_changes` - Scans for data changes

### Good Validation Behaviors

1. **Test data rejection**: `create_glossary_term` correctly rejects terms that appear to be test data (e.g., "Test Metric")

2. **Cascading deletes**: `delete_approved_query` automatically rejects related pending update suggestions

3. **SQL validation**: `suggest_approved_query` validates SQL and captures output columns automatically

---

## Testing Summary

| Category | Tools Tested | Working | Issues |
|----------|--------------|---------|--------|
| Core | 7 | 7 | 0 |
| Context | 4 | 4 | 0 |
| Probe | 3 | 2 | 1 |
| Entity/Ontology | 7 | 7 | 0 |
| Glossary | 5 | 5 | 0 |
| Approved Queries | 5 | 5 | 1 (UX) |
| Query Suggestions | 5 | 5 | 0 |
| Ontology Questions | 6 | 6 | 0 |
| Project Knowledge | 2 | 2 | 0 |
| Change Management | 4 | 4 | 0 |
| History | 1 | 0 | 1 |
| Schema | 1 | 1 | 1 (minor) |

**Total**: 50 tools tested, 6 issues found (1 critical, 2 high, 2 medium, 1 low)

---

## Recommendations Before Production

### Must Fix (Blocking)
1. ~~**Sensitive data exposure**~~ ✅ FIXED - Redaction implemented (subtasks 1.1, 1.2, 1.3)

### Should Fix
2. ~~**Error message clarity**~~ ✅ FIXED - create_approved_query error message improved
3. ~~**Query history**~~ ✅ FIXED - Logging verified

### Nice to Have
4. ~~**probe_relationship**~~ N/A - Tool removed with entity refactor
5. **Glossary SQL validation** - Improve PostgreSQL function recognition (OPEN)
6. ~~**refresh_schema delta**~~ ✅ FIXED - Column count now reports only new columns

---

## Status Summary (Updated 2026-02-05)

| Issue | Status |
|-------|--------|
| 1. Sensitive Data Exposure | ✅ FIXED |
| 2. Error Message Clarity | ✅ FIXED |
| 3. probe_relationship | N/A (tool removed) |
| 4. get_query_history | ✅ FIXED |
| 5. Glossary SQL Validation | **OPEN** |
| 6. refresh_schema Delta | ✅ FIXED |

**Remaining:** Only Issue 5 (Glossary SQL validation) is still open - low priority.
