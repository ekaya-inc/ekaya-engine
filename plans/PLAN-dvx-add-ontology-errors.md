# Plan: Surface LLM Format Errors in Ontology UI

## Goal
When any LLM enrichment step returns valid JSON but in wrong format, surface these as **warnings** in the UI so users know their LLM isn't returning the expected format. The ontology extraction continues and completes, but with missing information.

## Problem
Currently:
- LLM format errors are only logged to application logs
- UI shows "success" even when enrichment items failed
- Users with "bring your own LLM" don't know their LLM isn't working properly
- No visibility into partial failures across any enrichment step

---

## Current State: LLM Enrichment Services

### 1. Entity Enrichment
**Files:**
- DAG Node: `pkg/services/dag/entity_enrichment_node.go`
- Service: `pkg/services/entity_discovery_service.go`

**LLM Call:** Lines 276-279 in `enrichEntitiesWithLLM()`

**Response Struct (lines 209-216):**
```go
type entityEnrichmentResponse struct {
    Entities []entityEnrichment `json:"entities"`
}
type entityEnrichment struct {
    TableName        string                `json:"table_name"`
    EntityName       string                `json:"entity_name"`
    Description      string                `json:"description"`
    Domain           string                `json:"domain"`
    KeyColumns       []keyColumnEnrichment `json:"key_columns"`
    AlternativeNames []string              `json:"alternative_names"`
}
```

**Current Error Handling (lines 282-286):**
- Parse failures: Logs WARN, returns nil (continues with table names as fallback)
- No per-entity failure tracking
- No failure count returned to DAG

---

### 2. Relationship Enrichment
**Files:**
- DAG Node: `pkg/services/dag/relationship_enrichment_node.go`
- Service: `pkg/services/relationship_enrichment.go`

**LLM Call:** Lines 256-275 in `enrichBatchInternal()` (with retry)

**Response Struct (lines 350-362):**
```go
type relationshipEnrichmentResponse struct {
    Relationships []relationshipEnrichment `json:"relationships"`
}
type relationshipEnrichment struct {
    SourceTable  string `json:"source_table"`
    SourceColumn string `json:"source_column"`
    TargetTable  string `json:"target_table"`
    TargetColumn string `json:"target_column"`
    Description  string `json:"description"`
}
```

**Current Error Handling:**
- Parse failures (lines 305-318): Logs ERROR, fails batch, increments `result.Failed`
- Missing enrichment (lines 329-336): Logs via `logRelationshipFailure()`, increments `result.Failed`
- Has `logRelationshipFailure()` (lines 483-509) with conversation_id correlation
- Returns `batchResult{Enriched, Failed, BatchSize}` but warnings not surfaced to UI

---

### 3. Column Enrichment
**Files:**
- DAG Node: `pkg/services/dag/column_enrichment_node.go`
- Service: `pkg/services/column_enrichment.go`

**LLM Call:** Lines 556-579 in `enrichColumnBatch()` (with retry)

**Response Struct (lines 378-392):**
```go
type columnEnrichmentResponse struct {
    Columns []columnEnrichment `json:"columns"`
}
type columnEnrichment struct {
    Name         string             `json:"name"`
    Description  string             `json:"description"`
    SemanticType string             `json:"semantic_type"`
    Role         string             `json:"role"`
    Synonyms     []string           `json:"synonyms,omitempty"`
    EnumValues   []models.EnumValue `json:"enum_values,omitempty"`
    FKRole       *string            `json:"fk_role"`
}
```

**Current Error Handling:**
- Parse failures (lines 595-603): Logs ERROR, fails entire table
- Has `logTableFailure()` (lines 846-862)
- Returns `EnrichColumnsResult{TablesEnriched, TablesFailed map[string]string}`
- `TablesFailed` stores failure reasons per table (good pattern to follow!)

---

### 4. Ontology Finalization
**Files:**
- DAG Node: `pkg/services/dag/ontology_finalization_node.go`
- Service: `pkg/services/ontology_finalization.go`

**LLM Call:** Lines 154-157 in `generateDomainDescription()`

**Response Struct (lines 229-232):**
```go
type domainDescriptionResponse struct {
    Description string `json:"description"`
}
```

**Current Error Handling:**
- Parse failures (lines 165-171): Propagates error, fails node
- Simple structure, no per-item failures possible

---

## Design Approach

### 1. Add Warnings to DAG Node Model

**New Migration** - `migrations/028_add_dag_warnings.up.sql`:
```sql
ALTER TABLE engine_dag_nodes ADD COLUMN warnings JSONB DEFAULT '[]';
```

**Update Go Model** - `pkg/models/ontology_dag.go`:
```go
// Add to DAGNode struct
type DAGNode struct {
    // ... existing fields (ID, DAGID, Name, NodeOrder, Status, Progress, Error, etc.)
    Warnings []DAGWarning `json:"warnings,omitempty"`
}

// New type
type DAGWarning struct {
    Code    string `json:"code"`     // e.g., "LLM_FORMAT_MISMATCH"
    Message string `json:"message"`  // Human-readable explanation
    Count   int    `json:"count"`    // How many items affected
    Details string `json:"details"`  // Example of the issue
}

// Warning codes
const (
    WarningCodeLLMFormatMismatch  = "LLM_FORMAT_MISMATCH"
    WarningCodeLLMMissingFields   = "LLM_MISSING_FIELDS"
    WarningCodeLLMPartialResponse = "LLM_PARTIAL_RESPONSE"
    WarningCodeLLMParseError      = "LLM_PARSE_ERROR"
)
```

### 2. Update Repository

**File:** `pkg/repositories/dag_repository.go`

Add warnings to INSERT/UPDATE/SELECT queries for `engine_dag_nodes`.
The `warnings` column is JSONB, marshal/unmarshal like other JSONB fields.

### 3. Create Shared Warning Collector

**New File:** `pkg/services/dag/warnings.go`
```go
package dag

import "github.com/ekaya-inc/ekaya-engine/pkg/models"

// WarningCollector accumulates warnings during node execution
type WarningCollector struct {
    warnings []models.DAGWarning
}

func NewWarningCollector() *WarningCollector {
    return &WarningCollector{}
}

func (w *WarningCollector) AddFormatMismatch(count int, expected, actual string) {
    w.warnings = append(w.warnings, models.DAGWarning{
        Code:    models.WarningCodeLLMFormatMismatch,
        Message: fmt.Sprintf("LLM returned %d items in unexpected format", count),
        Count:   count,
        Details: fmt.Sprintf("Expected: %s, Got: %s", expected, actual),
    })
}

func (w *WarningCollector) AddParseError(count int, preview string) {
    w.warnings = append(w.warnings, models.DAGWarning{
        Code:    models.WarningCodeLLMParseError,
        Message: fmt.Sprintf("Failed to parse LLM response for %d items", count),
        Count:   count,
        Details: preview,
    })
}

func (w *WarningCollector) AddPartialResponse(expected, received int) {
    w.warnings = append(w.warnings, models.DAGWarning{
        Code:    models.WarningCodeLLMPartialResponse,
        Message: fmt.Sprintf("LLM returned %d of %d expected items", received, expected),
        Count:   expected - received,
    })
}

func (w *WarningCollector) Warnings() []models.DAGWarning {
    return w.warnings
}
```

### 4. Update Each Enrichment Service

#### Entity Enrichment (`pkg/services/entity_discovery_service.go`)

**Change `enrichEntitiesWithLLM` signature** to return warnings:
```go
func (s *entityDiscoveryService) enrichEntitiesWithLLM(
    ctx context.Context,
    projectID uuid.UUID,
    entities []*models.OntologyEntity,
) ([]models.DAGWarning, error)
```

**At line ~286** - When parse fails, create warning instead of just logging:
```go
if err != nil {
    s.logger.Warn("Failed to parse entity enrichment response", zap.Error(err))
    warnings = append(warnings, models.DAGWarning{
        Code:    models.WarningCodeLLMParseError,
        Message: "Failed to parse entity enrichment response",
        Count:   len(entities),
        Details: truncateString(result.Content, 200),
    })
    return warnings, nil // Continue with table names as fallback
}
```

#### Relationship Enrichment (`pkg/services/relationship_enrichment.go`)

**Update `EnrichRelationshipsResult`** (line 27):
```go
type EnrichRelationshipsResult struct {
    RelationshipsEnriched int              `json:"relationships_enriched"`
    RelationshipsFailed   int              `json:"relationships_failed"`
    DurationMs            int64            `json:"duration_ms"`
    Warnings              []models.DAGWarning `json:"warnings,omitempty"`  // NEW
}
```

**Update `batchResult`** (line 34):
```go
type batchResult struct {
    Enriched  int
    Failed    int
    BatchSize int
    Warnings  []models.DAGWarning  // NEW
}
```

**At "No enrichment found" (line ~328)** - Detect format mismatch:
```go
if !ok || description == "" {
    // Check if LLM returned data but with wrong keys (format mismatch)
    if len(enrichments) > 0 {
        // LLM returned something, but keys don't match = format issue
        result.Warnings = append(result.Warnings, models.DAGWarning{
            Code:    models.WarningCodeLLMFormatMismatch,
            Message: "LLM returned relationship with mismatched keys",
            Count:   1,
            Details: fmt.Sprintf("Expected: %s, LLM returned keys like: %s.%s",
                key, enrichments[0].SourceTable, enrichments[0].SourceColumn),
        })
    }
    s.logRelationshipFailure(rel, "No enrichment found in LLM response", nil, llmResult.ConversationID)
    result.Failed++
    continue
}
```

#### Column Enrichment (`pkg/services/column_enrichment.go`)

**Update `EnrichColumnsResult`** (line 33):
```go
type EnrichColumnsResult struct {
    TablesEnriched []string            `json:"tables_enriched"`
    TablesFailed   map[string]string   `json:"tables_failed,omitempty"`
    DurationMs     int64               `json:"duration_ms"`
    Warnings       []models.DAGWarning `json:"warnings,omitempty"`  // NEW
}
```

**At parse failure (line ~595)** - Add warning:
```go
if err != nil {
    s.logger.Error("Failed to parse LLM response", ...)
    result.Warnings = append(result.Warnings, models.DAGWarning{
        Code:    models.WarningCodeLLMParseError,
        Message: fmt.Sprintf("Failed to parse column enrichment for table %s", entity.PrimaryTable),
        Count:   len(columns),
        Details: truncateString(result.Content, 200),
    })
    return nil, fmt.Errorf("parse LLM response: %w", err)
}
```

### 5. Update DAG Nodes to Store Warnings

Each DAG node's `Execute()` method needs to save warnings to the node.

**Example for RelationshipEnrichmentNode** (`pkg/services/dag/relationship_enrichment_node.go`):
```go
func (n *RelationshipEnrichmentNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
    result, err := n.service.EnrichProject(ctx, dag.ProjectID, n.progressCallback(dag))
    if err != nil {
        return err
    }

    // Store warnings on the node
    if len(result.Warnings) > 0 {
        if err := n.dagService.AddNodeWarnings(ctx, dag.ID, n.Name(), result.Warnings); err != nil {
            n.Logger().Warn("Failed to store warnings", zap.Error(err))
        }
    }

    // ... rest of existing code
}
```

**Add to DAG service** (`pkg/services/ontology_dag_service.go`):
```go
func (s *ontologyDAGService) AddNodeWarnings(ctx context.Context, dagID uuid.UUID, nodeName string, warnings []models.DAGWarning) error {
    return s.dagRepo.AddNodeWarnings(ctx, dagID, nodeName, warnings)
}
```

### 6. Update API Response

**File:** `pkg/handlers/ontology_dag_handler.go`

Update `DAGNodeResponse` (line ~30):
```go
type DAGNodeResponse struct {
    Name     string                  `json:"name"`
    Status   string                  `json:"status"`
    Progress *DAGProgressResponse    `json:"progress,omitempty"`
    Error    string                  `json:"error,omitempty"`
    Warnings []DAGWarningResponse    `json:"warnings,omitempty"`  // NEW
}

type DAGWarningResponse struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Count   int    `json:"count"`
    Details string `json:"details,omitempty"`
}
```

Update `toDAGNodeResponse()` to include warnings from the node.

### 7. Update Frontend Types

**File:** `ui/src/types/ontology.ts`

```typescript
export interface DAGWarning {
  code: string;
  message: string;
  count: number;
  details?: string;
}

export interface DAGNode {
  name: DAGNodeName;
  status: DAGNodeStatus;
  progress?: DAGNodeProgress;
  error?: string;
  warnings?: DAGWarning[];  // NEW
}

export const WarningCodes = {
  LLM_FORMAT_MISMATCH: 'LLM_FORMAT_MISMATCH',
  LLM_MISSING_FIELDS: 'LLM_MISSING_FIELDS',
  LLM_PARTIAL_RESPONSE: 'LLM_PARTIAL_RESPONSE',
  LLM_PARSE_ERROR: 'LLM_PARSE_ERROR',
} as const;
```

### 8. Update Frontend UI

**File:** `ui/src/components/ontology/OntologyDAG.tsx`

Add warning display in the node rendering section (around line 632):

```tsx
{/* After node status/progress, before closing the node container */}
{node.warnings && node.warnings.length > 0 && (
  <div className="mt-2 p-3 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg">
    <div className="flex items-start gap-2">
      <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 flex-shrink-0" />
      <div className="text-sm">
        <p className="font-medium text-amber-800 dark:text-amber-200">
          Completed with warnings
        </p>
        {node.warnings.map((warning, idx) => (
          <div key={idx} className="mt-1 text-amber-700 dark:text-amber-300">
            <p>{warning.message}</p>
            {warning.details && (
              <p className="text-xs mt-1 font-mono bg-amber-100 dark:bg-amber-900/40 p-1 rounded">
                {warning.details}
              </p>
            )}
          </div>
        ))}
        <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">
          The ontology will be missing this information. Consider using a Community or fine-tuned model.
        </p>
      </div>
    </div>
  </div>
)}
```

Also add a summary banner at the top when DAG completes with warnings:

```tsx
{/* After the "Extraction Complete" success banner */}
{dagStatus?.status === 'completed' && hasAnyWarnings(dagStatus.nodes) && (
  <div className="mb-4 p-4 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg">
    <div className="flex items-start gap-3">
      <AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400 mt-0.5" />
      <div>
        <h3 className="font-medium text-amber-800 dark:text-amber-200">
          Ontology extracted with warnings
        </h3>
        <p className="text-sm text-amber-700 dark:text-amber-300 mt-1">
          Some enrichment steps encountered LLM format issues. Your ontology may be missing
          descriptions or semantic information. Check the individual steps below for details.
        </p>
      </div>
    </div>
  </div>
)}
```

---

## Implementation Order

1. **Migration** - Add `warnings` column to `engine_dag_nodes`
2. **Models** - Add `DAGWarning` type and constants to `pkg/models/ontology_dag.go`
3. **Repository** - Update `pkg/repositories/dag_repository.go` for warnings
4. **Shared collector** - Create `pkg/services/dag/warnings.go`
5. **Entity enrichment** - Update service to return warnings
6. **Relationship enrichment** - Update service to detect format mismatches
7. **Column enrichment** - Update service to return warnings
8. **DAG nodes** - Update each node to store warnings
9. **DAG service** - Add `AddNodeWarnings()` method
10. **API handler** - Include warnings in response
11. **Frontend types** - Add TypeScript interfaces
12. **Frontend UI** - Display warnings per node and summary banner

---

## Testing

1. Use `sparkone:30000/v1` with `trtllm-qwen3-30b-nvfp4-self` model (known to return entity names instead of table names)
2. Run ontology extraction
3. Verify warnings appear in UI for RelationshipEnrichment node
4. Verify ontology completes successfully despite warnings
5. Verify warning details show the format mismatch (expected vs actual)

---

## Out of Scope (Future)
- Detailed per-item failure list in expandable UI
- Retry individual failed items
- LLM compatibility checker/validator tool
- Warning persistence for historical analysis
