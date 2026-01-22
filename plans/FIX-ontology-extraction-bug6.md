# FIX: BUG-6 - Missing Enum Value Extraction

**Bug Reference:** plans/BUGS-ontology-extraction.md - BUG-6
**Severity:** Medium
**Category:** Column Enrichment

## Problem Summary

Key enum columns don't have their values and meanings documented:

| Column | Table | Expected Values |
|--------|-------|-----------------|
| `offer_type` | offers, billing_engagements, etc. | OFFER_TYPE_FREE=1, OFFER_TYPE_PAID=2, etc. |
| `transaction_state` | billing_transactions | TRANSACTION_STATE_STARTED=1, _ENDED=2, etc. |
| `transaction_type` | billing_transactions | unknown, engagement, payout |
| `activity` | billing_activity_messages | confirmed, paused, resumed, refunded |

When calling `probe_column(table="billing_transactions", column="transaction_state")`, no enum values are returned.

## Root Cause Analysis

### What Works

The enum extraction mechanism exists and is well-designed:

1. **Pattern Detection** (`pkg/services/column_enrichment.go:345-383`)
   - Columns with "status", "state", "type", "kind", "category", "_code", "level", "tier", "role" are identified
   - ✓ `offer_type`, `transaction_state`, `transaction_type` SHOULD match patterns

2. **Sample Value Collection** (`pkg/services/column_enrichment.go:277-320`)
   - Distinct values queried via `GetDistinctValues()` (limit 50)
   - Values stored if count < 50

3. **LLM Enrichment** (`pkg/services/column_enrichment.go:711`)
   - Prompt asks for `enum_values` array
   - Custom unmarshalling handles strings, numbers, and objects

### What's Missing

**The LLM only sees raw values, not semantic meanings:**

```
| Column           | Type | Sample Values        |
|------------------|------|----------------------|
| transaction_state| int  | 1, 2, 3, 4, 5        |  ← No context!
```

The LLM cannot know that:
- `1` = "Transaction started"
- `2` = "Transaction ended"
- `7` = "Paid out"

**Enum descriptions come from code** (protobuf definitions in `utobe.pb.go`):
```go
TRANSACTION_STATE_STARTED = 1  // Transaction started
TRANSACTION_STATE_ENDED = 2    // Transaction ended
```

The system has **no code analysis**, so it can't extract these meanings.

### Possible Failure Points

1. **Column enrichment DAG node didn't run** - Check DAG status
2. **LLM response didn't include enum_values** - Check `engine_llm_conversations`
3. **Sample values exceeded 50** - High-cardinality columns skipped
4. **Parse failure** - LLM response malformed

## Fix Implementation

### Short-Term: Improve LLM Enum Inference

Guide the LLM to infer meanings from values when possible.

**File:** `pkg/services/column_enrichment.go:711`

```go
// Enhance enum_values instruction
sb.WriteString("5. **enum_values**: for status/type/state columns with sampled values:\n")
sb.WriteString("   - Return as objects: [{\"value\": \"1\", \"label\": \"Started\"}, ...]\n")
sb.WriteString("   - Infer labels from column context and common patterns\n")
sb.WriteString("   - If column is 'transaction_state' and values are [1,2,3], infer state progression\n")
sb.WriteString("   - For string enums, use the value as label if descriptive (e.g., \"active\")\n")
sb.WriteString("   - For integer enums, infer meaning from column name context\n")
```

### Medium-Term: External Enum Definitions

Allow projects to provide enum definitions that get merged during enrichment.

#### 1. Add Enum Definition Configuration

**File:** `pkg/models/project.go`

```go
type EnumDefinition struct {
    Table   string             `json:"table"`
    Column  string             `json:"column"`
    Values  map[string]string  `json:"values"`  // value → description
}

type ProjectConfig struct {
    // ...existing fields...
    EnumDefinitions []EnumDefinition `json:"enum_definitions,omitempty"`
}
```

#### 2. Enum Definition File Format

```yaml
# .ekaya/enums.yaml
enums:
  - table: billing_transactions
    column: transaction_state
    values:
      "0": "UNSPECIFIED - Not set"
      "1": "STARTED - Transaction started"
      "2": "ENDED - Transaction ended"
      "3": "WAITING - Awaiting chargeback period"
      "4": "AVAILABLE - Available for payout"
      "5": "PROCESSING - Processing payout"
      "6": "PAYING - Paying out"
      "7": "PAID - Paid out"
      "8": "ERROR - Error occurred"

  - table: "*"  # Apply to any table with this column
    column: offer_type
    values:
      "0": "UNSPECIFIED"
      "1": "FREE - Free Engagement"
      "2": "PAID - Preauthorized per-minute"
      "3": "START_FREE - Starts free then charges"
      "4": "CHARGE_IN_ENGAGEMENT - Charge during"
      "5": "IMMEDIATE_PAYMENT - Immediate charge"
      "6": "TIP - Visitor tips Host"
```

#### 3. Merge Definitions During Enrichment

```go
func (s *columnEnrichmentService) mergeEnumDefinitions(
    ctx context.Context,
    projectID uuid.UUID,
    column *models.SchemaColumn,
    sampledValues []string,
) []models.EnumValue {
    // Load project enum definitions
    config := s.projectRepo.GetConfig(ctx, projectID)

    // Find matching definition
    for _, def := range config.EnumDefinitions {
        if (def.Table == "*" || def.Table == column.TableName) &&
           def.Column == column.ColumnName {
            var result []models.EnumValue
            for _, v := range sampledValues {
                desc := def.Values[v]
                result = append(result, models.EnumValue{
                    Value:       v,
                    Description: desc,
                })
            }
            return result
        }
    }

    // Fallback to sampled values without descriptions
    return toEnumValues(sampledValues)
}
```

### Long-Term: Code Analysis for Enum Extraction

Implement protobuf/constant analysis to extract enum definitions automatically.

#### 1. Protobuf Enum Scanner

```go
func ScanProtobufEnums(repoPath string) (map[string]map[string]string, error) {
    // Find .proto or .pb.go files
    files := glob(repoPath, "**/*.proto", "**/*.pb.go")

    enums := make(map[string]map[string]string)  // name → value → description

    for _, file := range files {
        content := readFile(file)

        // Parse enum definitions
        // e.g., TRANSACTION_STATE_STARTED = 1;  // Transaction started
        matches := enumRegex.FindAllStringSubmatch(content, -1)
        for _, m := range matches {
            enumName := m[1]
            enumValue := m[2]
            comment := m[3]
            enums[enumName][enumValue] = comment
        }
    }
    return enums
}
```

#### 2. Integration with Column Enrichment

During column enrichment, match columns to enum definitions:
- `transaction_state` → `TransactionState` enum
- `offer_type` → `OfferType` enum

## Debugging Checklist

If enum values aren't appearing:

1. **Check DAG completed:**
   ```sql
   SELECT status, current_node FROM engine_ontology_dag
   WHERE project_id = ? ORDER BY created_at DESC LIMIT 1;
   ```

2. **Check column enrichment ran:**
   ```sql
   SELECT status FROM engine_dag_nodes
   WHERE dag_id = ? AND node_name = 'ColumnEnrichment';
   ```

3. **Check LLM response:**
   ```sql
   SELECT content, status, error_message
   FROM engine_llm_conversations
   WHERE project_id = ? AND context->>'step' = 'column_enrichment'
   ORDER BY created_at DESC;
   ```

4. **Check ontology has column details:**
   ```sql
   SELECT column_details->'billing_transactions'
   FROM engine_ontologies WHERE is_active = true AND project_id = ?;
   ```

5. **Check sample values persisted:**
   ```sql
   SELECT sample_values FROM engine_schema_columns
   WHERE table_name = 'billing_transactions' AND column_name = 'transaction_state';
   ```

## Acceptance Criteria

- [x] Columns matching patterns (state, type, etc.) have sample values extracted
- [x] LLM prompt guides towards descriptive enum labels
- [x] Project-level enum definitions can be provided via config (includes file format support)
- [x] Enum definitions merged with sampled values during enrichment
- [x] `probe_column` returns enum values with descriptions
- [x] Integer enums have meaningful labels (not just "1", "2", "3")
