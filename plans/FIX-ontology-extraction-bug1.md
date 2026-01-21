# FIX: BUG-1 - Sample/Test Tables Extracted as Real Entities

**Bug Reference:** plans/BUGS-ontology-extraction.md - BUG-1
**Severity:** Critical
**Category:** Entity Discovery

## Problem Summary

Tables with prefixes `s1_` through `s10_` (sample/test data tables) are incorrectly extracted as legitimate business entities. The extraction produces entities like "Customer", "Student", "Contract" from test tables that have no relation to Tikr's actual business model.

## Root Cause

Table filtering is **completely absent** from the entity discovery pipeline:

1. **`is_selected` flag ignored**: The `engine_schema_tables.is_selected` field exists in the database but is never checked during entity discovery
2. **No table-level exclusion patterns**: Column filtering exists (`pkg/services/column_filter.go`) but there's no equivalent for tables
3. **All tables processed blindly**: `IdentifyEntitiesFromDDL()` calls `schemaRepo.ListTablesByDatasource()` without any filtering

### Code Flow

```
DAG: EntityDiscoveryNode.Execute()
  └─> EntityDiscoveryService.IdentifyEntitiesFromDDL()
        └─> schemaRepo.ListTablesByDatasource()  ← Returns ALL tables
              (no filtering applied)
        └─> Creates entity for EACH table
```

### Key Files

| File | Lines | Issue |
|------|-------|-------|
| `pkg/services/entity_discovery_service.go` | 90-105 | Retrieves all tables, no `is_selected` check |
| `pkg/services/dag/entity_discovery_node.go` | 48-79 | Passes through without filtering |
| `pkg/repositories/schema_repository.go` | 88-121 | Has `is_selected` field but it's unused |

## Fix Implementation

### Option A: Respect `is_selected` Flag (Recommended)

**Rationale:** The `is_selected` infrastructure already exists. Users can already mark tables as selected/unselected in the UI. Entity discovery should respect this.

#### 1. Modify `ListTablesByDatasource` to filter

**File:** `pkg/repositories/schema_repository.go`

Add parameter to filter by selection:
```go
// Change signature
func (r *schemaRepository) ListTablesByDatasource(ctx context.Context, projectID, datasourceID string, selectedOnly bool) ([]SchemaTable, error)

// Add WHERE clause when selectedOnly=true
if selectedOnly {
    query += " AND is_selected = true"
}
```

#### 2. Update `IdentifyEntitiesFromDDL` to use filter [DONE]

**File:** `pkg/services/entity_discovery_service.go`

```go
// Line ~90: Change to only get selected tables
tables, err := s.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID, true /* selectedOnly */)
```

#### 3. Auto-select tables during schema discovery

**File:** `pkg/services/schema_service.go`

When discovering tables, set `is_selected = true` by default but exclude obvious test patterns:
```go
func shouldAutoSelect(tableName string) bool {
    // Exclude common test/sample patterns
    testPatterns := []string{
        `^s\d+_`,      // s1_, s2_, etc.
        `^test_`,      // test_*
        `^tmp_`,       // tmp_*
        `^temp_`,      // temp_*
        `_test$`,      // *_test
        `_backup$`,    // *_backup
    }
    for _, pattern := range testPatterns {
        if matched, _ := regexp.MatchString(pattern, strings.ToLower(tableName)); matched {
            return false
        }
    }
    return true
}
```

### Option B: Add Table Exclusion Configuration

**Rationale:** Allow project-level configuration for table exclusion patterns.

#### 1. Add config to `engine_datasources`

```sql
ALTER TABLE engine_datasources
ADD COLUMN table_exclusion_patterns jsonb DEFAULT '[]'::jsonb;
```

#### 2. Create table filter service

**New file:** `pkg/services/table_filter.go`

```go
type TableFilter struct {
    exclusionPatterns []string
}

func (f *TableFilter) ShouldInclude(tableName string) bool {
    for _, pattern := range f.exclusionPatterns {
        if matched, _ := regexp.MatchString(pattern, tableName); matched {
            return false
        }
    }
    return true
}
```

#### 3. Apply filter in entity discovery

```go
func (s *entityDiscoveryService) IdentifyEntitiesFromDDL(...) {
    tables, _ := s.schemaRepo.ListTablesByDatasource(...)

    // Load exclusion patterns from datasource config
    filter := s.loadTableFilter(datasourceID)

    // Filter tables
    var filteredTables []SchemaTable
    for _, t := range tables {
        if filter.ShouldInclude(t.TableName) {
            filteredTables = append(filteredTables, t)
        }
    }
    // Continue with filteredTables...
}
```

## Recommended Approach

**Implement Option A first**, then consider Option B as an enhancement:

1. Option A is simpler - leverages existing `is_selected` infrastructure
2. Option A gives users explicit control via UI
3. Option B can be added later for more sophisticated patterns

## Testing

1. Create test tables with `s1_`, `test_`, `tmp_` prefixes
2. Run ontology extraction
3. Verify test tables are NOT extracted as entities
4. Verify legitimate tables ARE extracted
5. Verify UI allows manual override of `is_selected`

## Acceptance Criteria

- [x] Tables with `is_selected = false` are not extracted as entities
- [ ] New tables default to `is_selected = true` unless matching exclusion patterns
- [ ] Existing legitimate entities (Billing Engagement, User, Channel, etc.) still extracted
- [ ] Test tables (s1_*, s2_*, etc.) not extracted
