# Plan: Parameterized Queries for MCP Clients

## Overview

Enable saved queries to accept named, typed parameters that MCP clients can supply at execution time. This transforms static saved queries into reusable query templates while maintaining security through proper parameter sanitization and injection detection.

**Key Benefits:**
- MCP clients can execute pre-approved queries with dynamic values
- Type-safe parameter binding prevents SQL injection by design
- Security audit logging for SIEM integration
- No raw SQL execution by MCP clients - only parameterized templates

---

## Current State

### Database Schema (`migrations/005_queries.up.sql`)
- `engine_queries` table stores: `natural_language_prompt`, `sql_query`, `dialect`, `is_enabled`, etc.
- No parameter metadata storage

### Backend
- `pkg/models/query.go` - Query model without parameters
- `pkg/services/query.go` - QueryService with Create, Execute, Test, Validate
- `pkg/sql/validator.go` - Custom validator detecting multiple statements
- `pkg/mcp/tools/developer.go` - MCP tools (query, schema, sample, execute, validate)

### Frontend
- `ui/src/components/QueriesView.tsx` - Full CRUD UI for queries
- `ui/src/types/query.ts` - TypeScript interfaces
- SQL editor with CodeMirror, validation, test-before-save workflow

### MCP Server
- Uses `github.com/mark3labs/mcp-go` library
- Developer tools registered via `RegisterDeveloperTools()`
- Tool filtering based on project config

---

## Implementation Plan

### Phase 1: Database Schema Changes

**Status: [x] Complete**

**New Migration: `012_query_parameters.up.sql`**

```sql
-- Add parameters column to engine_queries table
-- Using JSONB array for flexibility and query capability
ALTER TABLE engine_queries
ADD COLUMN parameters JSONB DEFAULT '[]'::jsonb;

-- Parameters schema:
-- [
--   {
--     "name": "customer_id",
--     "type": "string",
--     "description": "The customer's unique identifier",
--     "required": true,
--     "default": null
--   },
--   {
--     "name": "start_date",
--     "type": "date",
--     "description": "Start of the date range",
--     "required": false,
--     "default": "2024-01-01"
--   }
-- ]

-- Index for queries with parameters (partial index)
CREATE INDEX idx_engine_queries_has_parameters
ON engine_queries ((parameters != '[]'::jsonb))
WHERE deleted_at IS NULL;

COMMENT ON COLUMN engine_queries.parameters IS
'Array of parameter definitions with name, type, description, required flag, and optional default value';
```

**Parameter Types to Support:**
| Type | Go Type | PostgreSQL Binding | Example |
|------|---------|-------------------|---------|
| `string` | `string` | `$N::text` | `"abc"` |
| `integer` | `int64` | `$N::bigint` | `123` |
| `decimal` | `float64` | `$N::numeric` | `99.95` |
| `boolean` | `bool` | `$N::boolean` | `true` |
| `date` | `string` (ISO) | `$N::date` | `"2024-01-15"` |
| `timestamp` | `string` (ISO) | `$N::timestamptz` | `"2024-01-15T10:30:00Z"` |
| `uuid` | `string` | `$N::uuid` | `"550e8400-..."` |
| `string[]` | `[]string` | `$N::text[]` | `["a","b"]` |
| `integer[]` | `[]int64` | `$N::bigint[]` | `[1,2,3]` |

---

### Phase 2: Backend Model & Repository

**Status: [x] Complete**

**File: `pkg/models/query.go`**

```go
// QueryParameter defines a single parameter for a parameterized query.
type QueryParameter struct {
    Name        string `json:"name"`
    Type        string `json:"type"`        // string, integer, decimal, boolean, date, timestamp, uuid, string[], integer[]
    Description string `json:"description"`
    Required    bool   `json:"required"`
    Default     any    `json:"default,omitempty"` // nil if no default
}

// Query represents a saved SQL query with metadata.
type Query struct {
    // ... existing fields ...
    Parameters []QueryParameter `json:"parameters,omitempty"` // NEW
}
```

**File: `pkg/repositories/query_repository.go`**

- Update `Create`, `Update`, `GetByID`, `ListByDatasource` to handle `parameters` JSONB column
- Use `pgx` JSON scanning for the parameters array

---

### Phase 3: Parameter Template Syntax

**Status: [x] Complete**

**Choose Template Syntax: `{{parameter_name}}`**

Rationale:
- Distinct from PostgreSQL's `$1` (positional) to avoid confusion
- Distinct from shell variables `${var}`
- Common in templating systems (Mustache, Handlebars)
- Easy to parse with regex: `\{\{([a-zA-Z_]\w*)\}\}`

**Documentation:**
- `pkg/sql/parameter_syntax.go` - Comprehensive Go package documentation
- `docs/query-parameters.md` - Standalone markdown documentation
- `pkg/sql/parameter_syntax_test.go` - Test suite validating all documented examples

**SQL Template Example:**
```sql
SELECT customer_name, order_total, order_date
FROM orders o
JOIN customers c ON o.customer_id = c.id
WHERE c.id = {{customer_id}}
  AND o.order_date >= {{start_date}}
  AND o.order_date < {{end_date}}
  AND o.status IN ({{statuses}})
ORDER BY o.order_date DESC
LIMIT {{limit}}
```

---

### Phase 4: SQL Validation & Parameter Extraction

**Status: [x] Complete**

**New File: `pkg/sql/parameters.go`**

```go
package sql

import (
    "fmt"
    "regexp"
    "github.com/ekaya-inc/ekaya-engine/pkg/models"
)

var parameterRegex = regexp.MustCompile(`\{\{([a-zA-Z_]\w*)\}\}`)

// ExtractParameters finds all {{param}} placeholders in SQL.
func ExtractParameters(sqlQuery string) []string {
    matches := parameterRegex.FindAllStringSubmatch(sqlQuery, -1)
    seen := make(map[string]bool)
    var params []string
    for _, match := range matches {
        name := match[1]
        if !seen[name] {
            seen[name] = true
            params = append(params, name)
        }
    }
    return params
}

// ValidateParameterDefinitions checks that all template params are defined.
func ValidateParameterDefinitions(sqlQuery string, params []models.QueryParameter) error {
    extracted := ExtractParameters(sqlQuery)
    defined := make(map[string]bool)
    for _, p := range params {
        defined[p.Name] = true
    }

    for _, name := range extracted {
        if !defined[name] {
            return fmt.Errorf("parameter {{%s}} used in SQL but not defined", name)
        }
    }

    // Check for defined but unused parameters (warning, not error)
    for _, p := range params {
        found := false
        for _, name := range extracted {
            if p.Name == name {
                found = true
                break
            }
        }
        if !found {
            // Could log warning or return soft error
        }
    }

    return nil
}

// SubstituteParameters replaces {{param}} with $N and returns ordered values.
// Returns the prepared SQL and the ordered parameter values for binding.
func SubstituteParameters(
    sqlQuery string,
    paramDefs []models.QueryParameter,
    suppliedValues map[string]any,
) (string, []any, error) {
    // Build lookup for parameter definitions
    defLookup := make(map[string]models.QueryParameter)
    for _, p := range paramDefs {
        defLookup[p.Name] = p
    }

    // Track parameter order for positional binding
    var orderedValues []any
    paramIndex := 1
    paramPositions := make(map[string]int)

    result := parameterRegex.ReplaceAllStringFunc(sqlQuery, func(match string) string {
        name := parameterRegex.FindStringSubmatch(match)[1]

        // Check if already assigned position (same param used multiple times)
        if pos, exists := paramPositions[name]; exists {
            return fmt.Sprintf("$%d", pos)
        }

        def := defLookup[name]
        value, supplied := suppliedValues[name]

        if !supplied {
            if def.Required && def.Default == nil {
                // Will be caught by validation
                return match
            }
            value = def.Default
        }

        paramPositions[name] = paramIndex
        orderedValues = append(orderedValues, value)
        pos := paramIndex
        paramIndex++

        return fmt.Sprintf("$%d", pos)
    })

    return result, orderedValues, nil
}
```

---

### Phase 5: SQL Injection Detection with libinjection

**Status: [x] Complete**

**Dependency: `github.com/corazawaf/libinjection-go`**

This library is a Go port of the battle-tested libinjection C library used by ModSecurity WAF.

**New File: `pkg/sql/injection.go`**

```go
package sql

import (
    libinjection "github.com/corazawaf/libinjection-go"
    "github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// InjectionCheckResult contains the result of an injection check.
type InjectionCheckResult struct {
    IsSQLi      bool
    Fingerprint string
    ParamName   string
    ParamValue  any
}

// CheckParameterForInjection uses libinjection to detect SQL injection patterns.
func CheckParameterForInjection(paramName string, value any) *InjectionCheckResult {
    // Only check string values - numbers/booleans can't contain injection
    strValue, ok := value.(string)
    if !ok {
        return nil
    }

    isSQLi, fingerprint := libinjection.IsSQLi(strValue)
    if isSQLi {
        return &InjectionCheckResult{
            IsSQLi:      true,
            Fingerprint: string(fingerprint),
            ParamName:   paramName,
            ParamValue:  value,
        }
    }

    return nil
}

// CheckAllParameters validates all parameter values for injection attempts.
func CheckAllParameters(params map[string]any) []*InjectionCheckResult {
    var results []*InjectionCheckResult
    for name, value := range params {
        if result := CheckParameterForInjection(name, value); result != nil {
            results = append(results, result)
        }
    }
    return results
}
```

**References:**
- [corazawaf/libinjection-go](https://pkg.go.dev/github.com/corazawaf/libinjection-go) - Pure Go port
- [wasilibs/go-libinjection](https://github.com/wasilibs/go-libinjection) - WebAssembly-based alternative

---

### Phase 6: Security Audit Logging for SIEM

**Status: [x] Complete**

**New File: `pkg/audit/security.go`**

```go
package audit

import (
    "context"
    "time"
    "encoding/json"

    "go.uber.org/zap"
    "github.com/google/uuid"
)

// SecurityEventType categorizes security-relevant events.
type SecurityEventType string

const (
    EventSQLInjectionAttempt SecurityEventType = "sql_injection_attempt"
    EventParameterValidation SecurityEventType = "parameter_validation_failure"
    EventQueryExecution      SecurityEventType = "query_execution"
)

// SecurityEvent represents an auditable security event.
type SecurityEvent struct {
    Timestamp   time.Time         `json:"timestamp"`
    EventType   SecurityEventType `json:"event_type"`
    ProjectID   uuid.UUID         `json:"project_id"`
    QueryID     uuid.UUID         `json:"query_id,omitempty"`
    UserID      string            `json:"user_id,omitempty"`
    ClientIP    string            `json:"client_ip,omitempty"`
    Details     any               `json:"details"`
    Severity    string            `json:"severity"` // info, warning, critical
}

// SQLInjectionDetails contains specifics of an injection attempt.
type SQLInjectionDetails struct {
    ParamName   string `json:"param_name"`
    ParamValue  string `json:"param_value"`
    Fingerprint string `json:"fingerprint"`
    QueryName   string `json:"query_name"`
}

// SecurityAuditor logs security events for SIEM consumption.
type SecurityAuditor struct {
    logger *zap.Logger
}

func NewSecurityAuditor(logger *zap.Logger) *SecurityAuditor {
    // Create a child logger with security-specific fields for SIEM parsing
    securityLogger := logger.Named("security_audit")
    return &SecurityAuditor{logger: securityLogger}
}

// LogInjectionAttempt records a detected SQL injection attempt.
func (a *SecurityAuditor) LogInjectionAttempt(
    ctx context.Context,
    projectID, queryID uuid.UUID,
    details SQLInjectionDetails,
    clientIP string,
) {
    event := SecurityEvent{
        Timestamp: time.Now().UTC(),
        EventType: EventSQLInjectionAttempt,
        ProjectID: projectID,
        QueryID:   queryID,
        ClientIP:  clientIP,
        Details:   details,
        Severity:  "critical",
    }

    eventJSON, _ := json.Marshal(event)

    // Structured logging for SIEM ingestion
    a.logger.Error("SQL injection attempt detected",
        zap.String("event_json", string(eventJSON)),
        zap.String("project_id", projectID.String()),
        zap.String("query_id", queryID.String()),
        zap.String("param_name", details.ParamName),
        zap.String("fingerprint", details.Fingerprint),
        zap.String("client_ip", clientIP),
        zap.String("severity", "critical"),
    )
}
```

**SIEM Integration Notes:**
- Log events use structured JSON for easy parsing
- Critical severity for injection attempts enables alerting
- Include client IP for threat attribution
- Include fingerprint for attack pattern analysis
- Consider also writing to dedicated audit table for persistence

---

### Phase 7: QueryService Changes

**Status: [x] Complete**

**File: `pkg/services/query.go`**

Add new methods and modify existing ones:

```go
// ExecuteWithParameters runs a parameterized query with supplied values.
func (s *queryService) ExecuteWithParameters(
    ctx context.Context,
    projectID, queryID uuid.UUID,
    params map[string]any,
    req *ExecuteQueryRequest,
) (*datasource.QueryExecutionResult, error) {
    // 1. Get query
    query, err := s.queryRepo.GetByID(ctx, projectID, queryID)
    if err != nil {
        return nil, err
    }

    // 2. Validate required parameters are provided
    if err := s.validateRequiredParameters(query.Parameters, params); err != nil {
        return nil, err
    }

    // 3. Type-check and coerce parameter values
    coercedParams, err := s.coerceParameterTypes(query.Parameters, params)
    if err != nil {
        return nil, err
    }

    // 4. Check for SQL injection attempts
    injectionResults := sql.CheckAllParameters(coercedParams)
    if len(injectionResults) > 0 {
        // Log to SIEM
        for _, result := range injectionResults {
            s.auditor.LogInjectionAttempt(ctx, projectID, queryID,
                audit.SQLInjectionDetails{
                    ParamName:   result.ParamName,
                    ParamValue:  fmt.Sprintf("%v", result.ParamValue),
                    Fingerprint: result.Fingerprint,
                    QueryName:   query.NaturalLanguagePrompt,
                },
                getClientIP(ctx),
            )
        }
        return nil, fmt.Errorf("potential SQL injection detected in parameter '%s'",
            injectionResults[0].ParamName)
    }

    // 5. Substitute parameters to get prepared SQL
    preparedSQL, orderedValues, err := sql.SubstituteParameters(
        query.SQLQuery, query.Parameters, coercedParams)
    if err != nil {
        return nil, err
    }

    // 6. Execute with parameterized binding
    executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config)
    if err != nil {
        return nil, err
    }
    defer executor.Close()

    // Use ExecuteQueryWithParams (new method on executor interface)
    result, err := executor.ExecuteQueryWithParams(ctx, preparedSQL, orderedValues, req.Limit)
    if err != nil {
        return nil, err
    }

    // 7. Increment usage count
    _ = s.queryRepo.IncrementUsageCount(ctx, queryID)

    return result, nil
}

// ValidateParameterizedQuery validates SQL template and parameter definitions.
func (s *queryService) ValidateParameterizedQuery(
    sqlQuery string,
    params []models.QueryParameter,
) error {
    // Check all {{param}} in SQL have definitions
    return sql.ValidateParameterDefinitions(sqlQuery, params)
}
```

---

### Phase 8: HTTP Handler Changes

**Status: [x] Complete**

**File: `pkg/handlers/queries.go`**

**Updated Request/Response Types:**

```go
// CreateQueryRequest with parameters
type CreateQueryRequest struct {
    NaturalLanguagePrompt string                   `json:"natural_language_prompt"`
    AdditionalContext     string                   `json:"additional_context,omitempty"`
    SQLQuery              string                   `json:"sql_query"`
    IsEnabled             bool                     `json:"is_enabled"`
    Parameters            []models.QueryParameter  `json:"parameters,omitempty"` // NEW
}

// ExecuteQueryRequest with parameter values
type ExecuteQueryRequest struct {
    Limit      int            `json:"limit,omitempty"`
    Parameters map[string]any `json:"parameters,omitempty"` // NEW
}

// TestQueryRequest with parameters
type TestQueryRequest struct {
    SQLQuery   string                   `json:"sql_query"`
    Limit      int                      `json:"limit,omitempty"`
    Parameters []models.QueryParameter  `json:"parameter_definitions,omitempty"` // NEW
    Values     map[string]any           `json:"parameter_values,omitempty"`      // NEW
}
```

**New Endpoint: Parameter Validation**

```
POST /api/projects/{pid}/datasources/{did}/queries/validate-parameters
```

Validates that:
- All `{{param}}` placeholders have definitions
- Parameter types are valid
- Required parameters have either no default or a valid default

---

### Phase 9: MCP Tool for Pre-Approved Queries

**Status: [x] Complete**

**File: `pkg/mcp/tools/queries.go`**

```go
// registerApprovedQueriesTools registers tools for executing pre-approved queries.
func registerApprovedQueriesTools(s *server.MCPServer, deps *QueryToolDeps) {
    registerListApprovedQueriesTool(s, deps)
    registerExecuteApprovedQueryTool(s, deps)
}

// listApprovedQueriesResult is the response structure for list_approved_queries.
type listApprovedQueriesResult struct {
    Queries []approvedQueryInfo `json:"queries"`
}

type approvedQueryInfo struct {
    ID                    string              `json:"id"`
    Name                  string              `json:"name"`        // natural_language_prompt
    Description           string              `json:"description"` // additional_context
    SQL                   string              `json:"sql"`         // The SQL template
    Parameters            []parameterInfo     `json:"parameters"`
    Dialect               string              `json:"dialect"`
}

type parameterInfo struct {
    Name        string `json:"name"`
    Type        string `json:"type"`
    Description string `json:"description"`
    Required    bool   `json:"required"`
    Default     any    `json:"default,omitempty"`
}

// registerListApprovedQueriesTool - Lists all enabled parameterized queries
func registerListApprovedQueriesTool(s *server.MCPServer, deps *QueryToolDeps) {
    tool := mcp.NewTool(
        "list_approved_queries",
        mcp.WithDescription(
            "List all pre-approved SQL queries available for execution. "+
            "Returns query metadata including parameters needed for execution. "+
            "Use execute_approved_query to run a specific query with parameters.",
        ),
        mcp.WithReadOnlyHintAnnotation(true),
        mcp.WithDestructiveHintAnnotation(false),
        mcp.WithIdempotentHintAnnotation(true),
        mcp.WithOpenWorldHintAnnotation(false),
    )

    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        projectID, tenantCtx, cleanup, err := checkApprovedQueriesEnabled(ctx, deps)
        if err != nil {
            return nil, err
        }
        defer cleanup()

        // Get default datasource
        dsID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
        if err != nil {
            return nil, fmt.Errorf("failed to get default datasource: %w", err)
        }

        // List enabled queries only
        queries, err := deps.QueryService.ListEnabled(tenantCtx, projectID, dsID)
        if err != nil {
            return nil, fmt.Errorf("failed to list queries: %w", err)
        }

        result := listApprovedQueriesResult{
            Queries: make([]approvedQueryInfo, len(queries)),
        }

        for i, q := range queries {
            params := make([]parameterInfo, len(q.Parameters))
            for j, p := range q.Parameters {
                params[j] = parameterInfo{
                    Name:        p.Name,
                    Type:        p.Type,
                    Description: p.Description,
                    Required:    p.Required,
                    Default:     p.Default,
                }
            }

            desc := ""
            if q.AdditionalContext != nil {
                desc = *q.AdditionalContext
            }

            result.Queries[i] = approvedQueryInfo{
                ID:          q.ID.String(),
                Name:        q.NaturalLanguagePrompt,
                Description: desc,
                SQL:         q.SQLQuery,
                Parameters:  params,
                Dialect:     q.Dialect,
            }
        }

        jsonResult, _ := json.Marshal(result)
        return mcp.NewToolResultText(string(jsonResult)), nil
    })
}

// registerExecuteApprovedQueryTool - Executes a pre-approved query with parameters
func registerExecuteApprovedQueryTool(s *server.MCPServer, deps *QueryToolDeps) {
    tool := mcp.NewTool(
        "execute_approved_query",
        mcp.WithDescription(
            "Execute a pre-approved SQL query by ID with optional parameters. "+
            "Use list_approved_queries first to see available queries and required parameters. "+
            "Parameters are type-checked and validated before execution.",
        ),
        mcp.WithString(
            "query_id",
            mcp.Required(),
            mcp.Description("The ID of the approved query to execute (from list_approved_queries)"),
        ),
        mcp.WithObject(
            "parameters",
            mcp.Description("Parameter values as key-value pairs matching the query's parameter definitions"),
        ),
        mcp.WithNumber(
            "limit",
            mcp.Description("Max rows to return (default: 100, max: 1000)"),
        ),
        mcp.WithReadOnlyHintAnnotation(true),  // Approved queries should be SELECT only
        mcp.WithDestructiveHintAnnotation(false),
        mcp.WithIdempotentHintAnnotation(true),
        mcp.WithOpenWorldHintAnnotation(false),
    )

    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        projectID, tenantCtx, cleanup, err := checkApprovedQueriesEnabled(ctx, deps)
        if err != nil {
            return nil, err
        }
        defer cleanup()

        // Get query_id parameter
        queryIDStr, err := req.RequireString("query_id")
        if err != nil {
            return nil, err
        }
        queryID, err := uuid.Parse(queryIDStr)
        if err != nil {
            return nil, fmt.Errorf("invalid query_id format: %w", err)
        }

        // Get optional parameters
        params := make(map[string]any)
        if args, ok := req.Params.Arguments.(map[string]any); ok {
            if p, ok := args["parameters"].(map[string]any); ok {
                params = p
            }
        }

        // Get limit
        limit := 100
        if limitVal, ok := getOptionalFloat(req, "limit"); ok {
            limit = int(limitVal)
        }
        if limit > 1000 {
            limit = 1000
        }

        // Execute with parameters (includes injection detection)
        execReq := &services.ExecuteQueryRequest{Limit: limit}
        result, err := deps.QueryService.ExecuteWithParameters(
            tenantCtx, projectID, queryID, params, execReq)
        if err != nil {
            return nil, fmt.Errorf("query execution failed: %w", err)
        }

        // Format response
        truncated := len(result.Rows) > limit
        rows := result.Rows
        if truncated {
            rows = rows[:limit]
        }

        response := struct {
            Columns   []string         `json:"columns"`
            Rows      []map[string]any `json:"rows"`
            RowCount  int              `json:"row_count"`
            Truncated bool             `json:"truncated"`
        }{
            Columns:   result.Columns,
            Rows:      rows,
            RowCount:  len(rows),
            Truncated: truncated,
        }

        jsonResult, _ := json.Marshal(response)
        return mcp.NewToolResultText(string(jsonResult)), nil
    })
}
```

**MCP Tool Config:**

Add new tool group for approved queries (separate from developer tools):

```go
const approvedQueriesToolGroup = "approved_queries"

var approvedQueriesToolNames = map[string]bool{
    "list_approved_queries":    true,
    "execute_approved_query":   true,
}
```

---

### Phase 10: Frontend UI Changes

**Status: [x] Complete**

**File: `ui/src/components/QueriesView.tsx`**

#### Parameter Definition UI

Add a collapsible "Parameters" section when creating/editing queries:

```tsx
interface ParameterDefinition {
  name: string;
  type: ParameterType;
  description: string;
  required: boolean;
  default: string | null;
}

type ParameterType =
  | 'string' | 'integer' | 'decimal' | 'boolean'
  | 'date' | 'timestamp' | 'uuid'
  | 'string[]' | 'integer[]';

// Parameter editor component
const ParameterEditor = ({
  parameters,
  onChange,
  extractedParams // from SQL template
}: {
  parameters: ParameterDefinition[];
  onChange: (params: ParameterDefinition[]) => void;
  extractedParams: string[];
}) => {
  // Show warning for params in SQL but not defined
  // Show warning for params defined but not in SQL
  // Allow adding/removing/editing parameter definitions
};
```

#### Parameter Auto-Detection

When SQL changes, extract `{{param}}` placeholders and:
1. Highlight undefined parameters in red
2. Auto-suggest adding missing parameter definitions
3. Show unused parameter warnings

#### Test Query with Parameters

When testing a query that has parameters:
1. Show a form with inputs for each parameter
2. Use parameter type to render appropriate input (date picker for dates, number input for integers, etc.)
3. Show required vs optional indicators
4. Pre-fill defaults where defined

```tsx
const ParameterInputForm = ({
  parameters,
  values,
  onChange,
}: {
  parameters: ParameterDefinition[];
  values: Record<string, unknown>;
  onChange: (values: Record<string, unknown>) => void;
}) => {
  return (
    <div className="space-y-4">
      {parameters.map((param) => (
        <div key={param.name}>
          <label className="block text-sm font-medium">
            {param.name}
            {param.required && <span className="text-red-500">*</span>}
          </label>
          <p className="text-xs text-text-tertiary">{param.description}</p>
          <ParameterInput
            type={param.type}
            value={values[param.name]}
            defaultValue={param.default}
            onChange={(v) => onChange({ ...values, [param.name]: v })}
          />
        </div>
      ))}
    </div>
  );
};
```

**File: `ui/src/types/query.ts`**

```typescript
export type ParameterType =
  | 'string' | 'integer' | 'decimal' | 'boolean'
  | 'date' | 'timestamp' | 'uuid'
  | 'string[]' | 'integer[]';

export interface QueryParameter {
  name: string;
  type: ParameterType;
  description: string;
  required: boolean;
  default: unknown | null;
}

export interface Query {
  // ... existing fields ...
  parameters: QueryParameter[];
}

export interface CreateQueryRequest {
  // ... existing fields ...
  parameters?: QueryParameter[];
}

export interface ExecuteQueryRequest {
  limit?: number;
  parameters?: Record<string, unknown>;
}

export interface TestQueryRequest {
  sql_query: string;
  limit?: number;
  parameter_definitions?: QueryParameter[];
  parameter_values?: Record<string, unknown>;
}
```

---

### Phase 11: Query Executor Interface Changes

**File: `pkg/adapters/datasource/interfaces.go`**

Add parameterized query execution method:

```go
type QueryExecutor interface {
    // Existing methods
    ExecuteQuery(ctx context.Context, sql string, limit int) (*QueryExecutionResult, error)
    Execute(ctx context.Context, sql string) (*ExecuteResult, error)
    ValidateQuery(ctx context.Context, sql string) error
    Close() error

    // NEW: Execute with positional parameters
    ExecuteQueryWithParams(
        ctx context.Context,
        sql string,
        params []any,
        limit int,
    ) (*QueryExecutionResult, error)
}
```

**File: `pkg/adapters/datasource/postgres/executor.go`**

Implement parameterized execution using pgx:

```go
func (e *PostgresQueryExecutor) ExecuteQueryWithParams(
    ctx context.Context,
    sql string,
    params []any,
    limit int,
) (*QueryExecutionResult, error) {
    // pgx handles parameterized queries natively
    rows, err := e.conn.Query(ctx, sql, params...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    // ... same result processing as ExecuteQuery ...
}
```

---

## Testing Strategy

### Unit Tests

1. **Parameter Extraction** (`pkg/sql/parameters_test.go`)
   - Extract single parameter
   - Extract multiple parameters
   - Handle duplicate parameters
   - Handle no parameters
   - Handle nested braces edge cases

2. **Parameter Substitution** (`pkg/sql/parameters_test.go`)
   - Single parameter substitution
   - Multiple parameters
   - Same parameter used multiple times
   - Default value application
   - Required parameter missing

3. **Injection Detection** (`pkg/sql/injection_test.go`)
   - Classic `' OR '1'='1` patterns
   - Union-based injection
   - Comment-based injection
   - Time-based blind injection patterns
   - Clean values pass through

4. **Type Coercion** (`pkg/services/query_test.go`)
   - String to integer
   - String to date
   - Array parsing
   - Invalid type handling

### Integration Tests

1. **End-to-end parameterized query execution**
2. **MCP tool execution with parameters**
3. **Injection attempt blocked and logged**
4. **UI parameter form rendering**

---

## Security Considerations

### Defense in Depth

1. **Template Syntax** - Parameters use `{{name}}` not raw SQL
2. **Parameterized Binding** - Values bound as parameters, never interpolated
3. **Type Checking** - Values coerced to declared types
4. **Injection Detection** - libinjection scans string values
5. **Audit Logging** - All injection attempts logged for SIEM
6. **Query Approval** - MCP clients can only run pre-approved queries

### Logging Requirements for SIEM

Log entries for injection attempts MUST include:
- Timestamp (ISO 8601 UTC)
- Project ID
- Query ID
- Parameter name with suspicious value
- libinjection fingerprint
- Client IP address
- Severity level ("critical")
- Event type ("sql_injection_attempt")

---

## File Summary

| Component | Files to Create/Modify |
|-----------|----------------------|
| Database | `migrations/XXX_query_parameters.up.sql` |
| Models | `pkg/models/query.go` |
| Repository | `pkg/repositories/query_repository.go` |
| SQL Utils | `pkg/sql/parameters.go` (NEW), `pkg/sql/injection.go` (NEW) |
| Audit | `pkg/audit/security.go` (NEW) |
| Service | `pkg/services/query.go` |
| Handlers | `pkg/handlers/queries.go` |
| MCP Tools | `pkg/mcp/tools/queries.go` (NEW) |
| Executor | `pkg/adapters/datasource/interfaces.go`, `pkg/adapters/datasource/postgres/executor.go` |
| Frontend Types | `ui/src/types/query.ts` |
| Frontend Components | `ui/src/components/QueriesView.tsx`, `ui/src/components/ParameterEditor.tsx` (NEW), `ui/src/components/ParameterInputForm.tsx` (NEW) |
| Tests | Multiple `*_test.go` files |

---

## Dependencies to Add

**Go:**
```
go get github.com/corazawaf/libinjection-go
```

**Frontend:**
None required - uses existing UI components

---

## Open Questions for Implementation

1. **Parameter Syntax Choice**: `{{name}}` vs `${name}` vs `:name` - Plan uses `{{name}}`
2. **Array Syntax**: How should array values be passed via MCP? JSON array `["a","b"]`?
3. **Default Value Types**: Store as JSON any? Type-specific storage?
4. **Partial Execution**: If 3 of 5 params fail validation, do we validate all or fail fast?
5. **Rate Limiting**: Should injection attempts trigger rate limiting?
