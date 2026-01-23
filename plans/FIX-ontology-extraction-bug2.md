# FIX: BUG-2 - File-Based Knowledge Loading

**Bug Reference:** BUGS-ontology-extraction.md, BUG-2
**Severity:** Critical
**Type:** Architectural Flaw

## Problem Summary

The KnowledgeSeeding step and related code expect to load project-specific knowledge from files on disk. This is a fundamental architectural flaw - ekaya-engine is a **cloud service** with no project-specific files on disk.

## Design Principle

The system should work as:
1. **Infer** knowledge from database schema and data analysis
2. **Refine** via MCP tools (agents with access to code/docs can update ontology)
3. **Clarify** via ontology questions that MCP tools can answer

**There should be ZERO file loading for project-specific data.**

## Files to Delete

### 1. Delete Entire File: `pkg/services/knowledge_discovery.go` (734 lines)

This file scans Go/TypeScript/Markdown files from disk to discover project knowledge. Delete entirely.

### 2. Delete Entire File: `pkg/services/knowledge_discovery_test.go`

Tests for the deleted file.

## Code to Remove

### From `pkg/services/knowledge.go`

Remove the following (approximately lines 131-275):

1. **`SeedKnowledgeFromFile` method** (~60 lines, line 131)
   ```go
   func (s *knowledgeService) SeedKnowledgeFromFile(ctx context.Context, projectID uuid.UUID) (int, error)
   ```

2. **`loadKnowledgeSeedFile` function** (~30 lines, line 257)
   ```go
   func loadKnowledgeSeedFile(path string) (*KnowledgeSeedFile, error)
   ```

3. **Struct definitions:**
   - `KnowledgeSeedFile` (line 198)
   - `KnowledgeSeedFact` (line 206)
   - `knowledgeSeedFactWithType` (internal type)

4. **`AllFacts` method** (line 219)
   ```go
   func (f *KnowledgeSeedFile) AllFacts() []knowledgeSeedFactWithType
   ```

5. **Interface definition** - Remove from `KnowledgeService` interface (line 36):
   ```go
   SeedKnowledgeFromFile(ctx context.Context, projectID uuid.UUID) (int, error)
   ```

### From `pkg/services/dag/knowledge_seeding_node.go`

**Option A: Make Node a No-Op**

Change the `Execute` method to immediately succeed:
```go
func (n *KnowledgeSeedingNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
    n.Logger().Info("Knowledge seeding skipped (no file-based loading)",
        zap.String("project_id", dag.ProjectID.String()))

    if err := n.ReportProgress(ctx, 100, 100, "Knowledge seeding complete (inference-based)"); err != nil {
        n.Logger().Warn("Failed to report progress", zap.Error(err))
    }
    return nil
}
```

Remove `KnowledgeSeedingMethods` interface and the `knowledgeSeeding` field.

**Option B: Remove Node Entirely**

If the DAG can handle missing nodes, remove the node from the DAG definition. This requires checking how the DAG is constructed.

### From `pkg/services/dag_adapters.go`

Remove the entire `KnowledgeSeedingAdapter` (lines 200-211):
```go
type KnowledgeSeedingAdapter struct {
    svc KnowledgeService
}

func NewKnowledgeSeedingAdapter(svc KnowledgeService) dag.KnowledgeSeedingMethods {
    return &KnowledgeSeedingAdapter{svc: svc}
}

func (a *KnowledgeSeedingAdapter) SeedKnowledgeFromFile(ctx context.Context, projectID uuid.UUID) (int, error) {
    return a.svc.SeedKnowledgeFromFile(ctx, projectID)
}
```

### From `pkg/models/project.go`

Remove `ParseEnumFile` function (line 108):
```go
func ParseEnumFile(path string) ([]EnumDefinition, error)
```

**Keep** `ParseEnumFileContent` - it may be useful for parsing content stored in the database.

### From `pkg/services/column_enrichment.go`

Remove `loadEnumDefinitions` function (line 860) and its usage. This function loads enums via `project.Parameters["enums_path"]`.

## Test Files to Update/Delete

| File | Action |
|------|--------|
| `pkg/services/knowledge_discovery_test.go` | DELETE entirely |
| `pkg/services/knowledge_test.go` | Remove file-based test cases |
| `pkg/services/knowledge_seeding_integration_test.go` | Remove file-based test cases |
| `pkg/services/dag/knowledge_seeding_node_test.go` | Update to test no-op behavior |
| `pkg/models/project_test.go` | Remove `TestParseEnumFile*` tests (keep `ParseEnumFileContent` tests) |
| `pkg/services/column_enrichment_test.go` | Remove `enums_path` test cases |

## Implementation Steps

### Step 1: Delete Files ✓
```bash
rm pkg/services/knowledge_discovery.go
rm pkg/services/knowledge_discovery_test.go
```

### Step 2: Update knowledge.go ✓
Remove the interface method, implementation, structs, and helper functions.

### Step 3: Update DAG Node ✓
Made it a no-op - knowledge seeding now immediately succeeds with "inference-based" message.

### Step 4: Remove Adapter ✓
Removed `KnowledgeSeedingAdapter` from dag_adapters.go.

### Step 5: Update project.go ✓
Removed `ParseEnumFile`, kept `ParseEnumFileContent`.

### Step 6: Update column_enrichment.go ✓
Removed `loadEnumDefinitions` and its usage.

### Step 7: Update Tests ✓
- Removed `knowledge_test.go` (file-based tests)
- Removed `knowledge_seeding_integration_test.go` (file-based tests)
- Updated `knowledge_seeding_node_test.go` to test no-op behavior
- Removed `TestParseEnumFile*` tests from `project_test.go`
- Removed `TestColumnEnrichmentService_loadEnumDefinitions` from `column_enrichment_test.go`
- Removed mock `SeedKnowledgeFromFile` from handler tests

### Step 8: Verify No File I/O Remains ✓
```bash
# Should find NO results after cleanup:
grep -r "os.ReadFile\|ioutil.ReadFile" pkg/services/ --include="*.go" | grep -v "_test.go"
grep -r "knowledge_seed_path\|enums_path" pkg/ --include="*.go"
```

## Future Consideration

After removing file-based loading, consider adding auto-inference during OntologyFinalization:
- Detect `deleted_at` columns → soft delete convention
- Detect `*_amount` columns with large integers → currency in cents
- Detect UUID text columns → entity ID format

This replaces file-based knowledge with schema-inferred knowledge.

## Success Criteria

- [x] No file I/O for project-specific data
- [x] DAG completes without file loading step
- [x] Knowledge comes only from schema inference or MCP tool updates
- [x] Tests pass without file fixtures
- [x] `grep` verification commands return no results
- [ ] `make check` passes

## Risk Assessment

**Low Risk:** This removes dead/unusable code in a cloud service. The file loading was never usable in production since there are no project files on the server.

**Migration:** Any projects with `knowledge_seed_path` or `enums_path` in their parameters will simply have those values ignored. No data loss occurs.
