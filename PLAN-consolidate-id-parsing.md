# Plan: Consolidate ID Parsing in Handlers

## Problem

Every handler has its own duplicated `parseProjectID`, `parseDatasourceID`, `parseEntityID`, etc. methods. These are nearly identical implementations copy-pasted across 10+ handler files.

**Current duplication (16 methods across handlers):**

| File | Methods |
|------|---------|
| `entity_handler.go` | `parseProjectID`, `parseEntityID`, `parseAliasID` |
| `entity_relationship_handler.go` | `parseProjectID`, `parseDatasourceID` |
| `entity_discovery_handler.go` | `parseProjectAndDatasourceIDs` |
| `relationship_workflow.go` | `parseProjectAndDatasourceIDs` |
| `ontology.go` | `parseProjectID` |
| `ontology_questions.go` | `parseProjectID`, `parseQuestionID` |
| `ontology_chat.go` | `parseProjectID` |
| `schema.go` | `parseProjectID`, `parseProjectAndDatasourceIDs` |
| `queries.go` | `parseProjectAndDatasource`, `parseQueryID` |
| `mcp_config.go` | `parseProjectID` |

**Example of duplication** - these are nearly identical:

```go
// entity_handler.go:388
func (h *EntityHandler) parseProjectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
    projectIDStr := r.PathValue("pid")
    projectID, err := uuid.Parse(projectIDStr)
    if err != nil {
        if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
            h.logger.Error("Failed to write error response", zap.Error(err))
        }
        return uuid.Nil, false
    }
    return projectID, true
}

// entity_relationship_handler.go:187 (IDENTICAL except receiver)
func (h *EntityRelationshipHandler) parseProjectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
    projectIDStr := r.PathValue("pid")
    projectID, err := uuid.Parse(projectIDStr)
    if err != nil {
        if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
            h.logger.Error("Failed to write error response", zap.Error(err))
        }
        return uuid.Nil, false
    }
    return projectID, true
}
```

## Why This Matters

1. **Maintenance burden** - Bug fixes or changes must be applied to 16 places
2. **Inconsistency risk** - Some handlers might have slightly different error messages or behavior
3. **Code bloat** - Each handler file is larger than necessary
4. **DRY violation** - Classic code smell

## Solution

Create a new `pkg/handlers/params.go` file with shared parsing functions that all handlers can use.

## Implementation Steps

### Step 1: Create the params.go file

**File:** `pkg/handlers/params.go`

```go
package handlers

import (
    "net/http"

    "github.com/google/uuid"
    "go.uber.org/zap"
)

// ParseProjectID extracts and validates the project ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
func ParseProjectID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
    return parseUUID(w, r, "pid", "invalid_project_id", "Invalid project ID format", logger)
}

// ParseDatasourceID extracts and validates the datasource ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
func ParseDatasourceID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
    return parseUUID(w, r, "dsid", "invalid_datasource_id", "Invalid datasource ID format", logger)
}

// ParseEntityID extracts and validates the entity ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
func ParseEntityID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
    return parseUUID(w, r, "eid", "invalid_entity_id", "Invalid entity ID format", logger)
}

// ParseAliasID extracts and validates the alias ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
func ParseAliasID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
    return parseUUID(w, r, "aid", "invalid_alias_id", "Invalid alias ID format", logger)
}

// ParseQuestionID extracts and validates the question ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
func ParseQuestionID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
    return parseUUID(w, r, "qid", "invalid_question_id", "Invalid question ID format", logger)
}

// ParseQueryID extracts and validates the query ID from the request path.
// Returns the parsed UUID and true on success, or uuid.Nil and false on error
// (after writing an error response).
func ParseQueryID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, bool) {
    return parseUUID(w, r, "queryid", "invalid_query_id", "Invalid query ID format", logger)
}

// ParseProjectAndDatasourceIDs extracts and validates both project and datasource IDs.
// Returns both UUIDs and true on success, or uuid.Nil values and false on error.
func ParseProjectAndDatasourceIDs(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (uuid.UUID, uuid.UUID, bool) {
    projectID, ok := ParseProjectID(w, r, logger)
    if !ok {
        return uuid.Nil, uuid.Nil, false
    }

    datasourceID, ok := ParseDatasourceID(w, r, logger)
    if !ok {
        return uuid.Nil, uuid.Nil, false
    }

    return projectID, datasourceID, true
}

// parseUUID is the internal helper that does the actual parsing work.
func parseUUID(w http.ResponseWriter, r *http.Request, pathParam, errorCode, errorMessage string, logger *zap.Logger) (uuid.UUID, bool) {
    idStr := r.PathValue(pathParam)
    id, err := uuid.Parse(idStr)
    if err != nil {
        if err := ErrorResponse(w, http.StatusBadRequest, errorCode, errorMessage); err != nil {
            logger.Error("Failed to write error response", zap.Error(err))
        }
        return uuid.Nil, false
    }
    return id, true
}
```

### Step 2: Update each handler file

For each handler file, replace the local parse methods with calls to the shared functions.

#### 2a. entity_handler.go

**Remove** (lines 388-422):
- `func (h *EntityHandler) parseProjectID(...)`
- `func (h *EntityHandler) parseEntityID(...)`
- `func (h *EntityHandler) parseAliasID(...)`

**Replace usages** throughout the file:
```go
// BEFORE
projectID, ok := h.parseProjectID(w, r)

// AFTER
projectID, ok := ParseProjectID(w, r, h.logger)
```

```go
// BEFORE
entityID, ok := h.parseEntityID(w, r)

// AFTER
entityID, ok := ParseEntityID(w, r, h.logger)
```

```go
// BEFORE
aliasID, ok := h.parseAliasID(w, r)

// AFTER
aliasID, ok := ParseAliasID(w, r, h.logger)
```

#### 2b. entity_relationship_handler.go

**Remove** (lines 187-209):
- `func (h *EntityRelationshipHandler) parseProjectID(...)`
- `func (h *EntityRelationshipHandler) parseDatasourceID(...)`

**Replace usages**:
```go
// BEFORE
projectID, ok := h.parseProjectID(w, r)

// AFTER
projectID, ok := ParseProjectID(w, r, h.logger)
```

```go
// BEFORE
datasourceID, ok := h.parseDatasourceID(w, r)

// AFTER
datasourceID, ok := ParseDatasourceID(w, r, h.logger)
```

#### 2c. entity_discovery_handler.go

**Remove** (lines 192-212):
- `func (h *EntityDiscoveryHandler) parseProjectAndDatasourceIDs(...)`

**Replace usages**:
```go
// BEFORE
projectID, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)

// AFTER
projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
```

#### 2d. relationship_workflow.go (handler file)

**Remove** (lines 483-503):
- `func (h *RelationshipWorkflowHandler) parseProjectAndDatasourceIDs(...)`

**Replace usages**:
```go
// BEFORE
projectID, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)

// AFTER
projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
```

#### 2e. ontology.go

**Remove** (around line 406):
- `func (h *OntologyHandler) parseProjectID(...)`

**Replace usages**:
```go
// BEFORE
projectID, ok := h.parseProjectID(w, r)

// AFTER
projectID, ok := ParseProjectID(w, r, h.logger)
```

#### 2f. ontology_questions.go

**Remove** (lines 326-338):
- `func (h *OntologyQuestionsHandler) parseProjectID(...)`
- `func (h *OntologyQuestionsHandler) parseQuestionID(...)`

**Replace usages**:
```go
// BEFORE
projectID, ok := h.parseProjectID(w, r)

// AFTER
projectID, ok := ParseProjectID(w, r, h.logger)
```

```go
// BEFORE
questionID, ok := h.parseQuestionID(w, r)

// AFTER
questionID, ok := ParseQuestionID(w, r, h.logger)
```

#### 2g. ontology_chat.go

**Remove** (line 319):
- `func (h *OntologyChatHandler) parseProjectID(...)`

**Replace usages**:
```go
// BEFORE
projectID, ok := h.parseProjectID(w, r)

// AFTER
projectID, ok := ParseProjectID(w, r, h.logger)
```

#### 2h. schema.go

**Remove** (lines 752-790):
- `func (h *SchemaHandler) parseProjectID(...)`
- `func (h *SchemaHandler) parseProjectAndDatasourceIDs(...)`

**Replace usages**:
```go
// BEFORE
projectID, ok := h.parseProjectID(w, r)

// AFTER
projectID, ok := ParseProjectID(w, r, h.logger)
```

```go
// BEFORE
projectID, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)

// AFTER
projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
```

#### 2i. queries.go

**Remove** (lines 638-670):
- `func (h *QueriesHandler) parseProjectAndDatasource(...)`
- `func (h *QueriesHandler) parseQueryID(...)`

**Replace usages**:
```go
// BEFORE
projectID, datasourceID, ok := h.parseProjectAndDatasource(w, r)

// AFTER
projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
```

```go
// BEFORE
queryID, ok := h.parseQueryID(w, r)

// AFTER
queryID, ok := ParseQueryID(w, r, h.logger)
```

#### 2j. mcp_config.go

**Remove** (line 112):
- `func (h *MCPConfigHandler) parseProjectID(...)`

**Replace usages**:
```go
// BEFORE
projectID, ok := h.parseProjectID(w, r)

// AFTER
projectID, ok := ParseProjectID(w, r, h.logger)
```

### Step 3: Verify path parameter names

Before implementing, verify that all handlers use consistent path parameter names:
- `pid` for project ID
- `dsid` for datasource ID
- `eid` for entity ID
- `aid` for alias ID
- `qid` for question ID
- `queryid` for query ID

Check route registrations if any use different names and adjust `params.go` accordingly.

## Testing

1. Run `make check` to ensure everything compiles and passes
2. All existing integration tests should pass with no changes
3. The behavior should be identical - just the code organization is different

## Files Changed

| File | Change |
|------|--------|
| `pkg/handlers/params.go` | **NEW** - Shared parsing functions |
| `pkg/handlers/entity_handler.go` | Remove 3 methods, update usages |
| `pkg/handlers/entity_relationship_handler.go` | Remove 2 methods, update usages |
| `pkg/handlers/entity_discovery_handler.go` | Remove 1 method, update usages |
| `pkg/handlers/relationship_workflow.go` | Remove 1 method, update usages |
| `pkg/handlers/ontology.go` | Remove 1 method, update usages |
| `pkg/handlers/ontology_questions.go` | Remove 2 methods, update usages |
| `pkg/handlers/ontology_chat.go` | Remove 1 method, update usages |
| `pkg/handlers/schema.go` | Remove 2 methods, update usages |
| `pkg/handlers/queries.go` | Remove 2 methods, update usages |
| `pkg/handlers/mcp_config.go` | Remove 1 method, update usages |

## Estimated Scope

- ~80 lines of new params.go code
- ~200 lines removed across handler files
- Net reduction of ~120 lines
- Low risk - mechanical refactor with no behavior change

## Order of Operations

1. [x] Create `params.go` first
2. Update handlers one at a time (run `go build ./...` after each)
   - [x] 2a. entity_handler.go - Remove 3 methods, update usages
   - [x] 2b. entity_relationship_handler.go - Remove 2 methods, update usages
   - [x] 2c. entity_discovery_handler.go - Remove 1 method, update usages
   - [x] 2d. relationship_workflow.go - Remove 1 method, update usages
   - [x] 2e. ontology.go - Remove 1 method, update usages
   - [x] 2f. ontology_questions.go - Remove 2 methods, update usages
   - [x] 2g. ontology_chat.go - Remove 1 method, update usages
   - [ ] 2h. schema.go - Remove 2 methods, update usages
   - [ ] 2i. queries.go - Remove 2 methods, update usages
   - [ ] 2j. mcp_config.go - Remove 1 method, update usages
3. [ ] Run `make check` after all changes
