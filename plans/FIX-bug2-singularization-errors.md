# FIX: Bug 2 - Entity Name Suggestions Have Incorrect Singularization

**Priority:** Low
**Component:** Schema Change Detection / Entity Naming

## Problem Statement

When `refresh_schema` suggests entity names from table names, the singularization logic produces incorrect results for some table names.

**Examples from pending changes:**
- `s4_categories` → suggested "S4_categorie" (should be "S4_category" or "Category")
- `s5_activities` → suggested "S5_activitie" (should be "S5_activity" or "Activity")

## Root Cause Analysis

### Location 1: `pkg/services/schema_change_detection.go:148-165`

```go
// toEntityName converts a table name to an entity name.
// Examples: "public.users" -> "User", "orders" -> "Order"
func toEntityName(tableName string) string {
    // Strip schema prefix if present
    name := tableName
    if idx := strings.LastIndex(tableName, "."); idx >= 0 {
        name = tableName[idx+1:]
    }

    // Convert to singular PascalCase
    // Simple heuristic: remove trailing 's' and capitalize first letter
    name = strings.TrimSuffix(name, "s")  // BUG: Line 159
    if len(name) > 0 {
        name = strings.ToUpper(name[:1]) + name[1:]
    }

    return name
}
```

**The Bug:** Line 159 uses `strings.TrimSuffix(name, "s")` which only removes a trailing "s". This fails for:
- Words ending in "ies": `categories` → `categorie` (should be `category`)
- Words ending in "es": `boxes` → `boxe` (should be `box`)
- Irregular plurals: `people` → `people` (should be `person`)

### Location 2: `pkg/services/data_change_detection.go:330-333`

```go
// Build a map of table names to their info
tableByName := make(map[string]*models.SchemaTable)
for _, t := range allTables {
    tableByName[t.TableName] = t
    // Also map without 's' suffix for pluralized tables
    if strings.HasSuffix(t.TableName, "s") {
        tableByName[strings.TrimSuffix(t.TableName, "s")] = t  // BUG: Line 332
    }
}
```

This has the same issue - it creates a lookup key by just removing trailing "s", so:
- `categories` maps to `categorie` (wrong key)
- Should map to `category`

## English Pluralization Rules

English pluralization is complex. Key rules:

1. **-ies → -y** (consonant + y):
   - categories → category
   - activities → activity
   - companies → company

2. **-es → (remove)** (s, x, z, ch, sh endings):
   - boxes → box
   - addresses → address
   - matches → match

3. **-ves → -f/-fe**:
   - knives → knife
   - leaves → leaf

4. **Regular -s → (remove)**:
   - users → user
   - orders → order

5. **Irregular**:
   - people → person
   - children → child
   - data → datum (often left as "data")

## Recommended Fix

### Option A: Use a Go inflection library (Recommended)

Add `github.com/jinzhu/inflection` dependency:

```bash
go get github.com/jinzhu/inflection
```

Update `schema_change_detection.go`:

```go
import "github.com/jinzhu/inflection"

// toEntityName converts a table name to an entity name.
func toEntityName(tableName string) string {
    // Strip schema prefix if present
    name := tableName
    if idx := strings.LastIndex(tableName, "."); idx >= 0 {
        name = tableName[idx+1:]
    }

    // Singularize using proper English rules
    name = inflection.Singular(name)

    // Capitalize first letter
    if len(name) > 0 {
        name = strings.ToUpper(name[:1]) + name[1:]
    }

    return name
}
```

Update `data_change_detection.go`:

```go
import "github.com/jinzhu/inflection"

// Build a map of table names to their info
tableByName := make(map[string]*models.SchemaTable)
for _, t := range allTables {
    tableByName[t.TableName] = t
    // Also map singular form for FK lookups
    singular := inflection.Singular(t.TableName)
    if singular != t.TableName {
        tableByName[singular] = t
    }
}
```

### Option B: Implement core singularization rules (if avoiding dependencies)

Create a utility function in `pkg/utils/inflection.go`:

```go
package utils

import "strings"

// Singularize converts a plural English word to singular.
// Handles common cases but not all irregular plurals.
func Singularize(word string) string {
    word = strings.ToLower(word)

    // Handle common irregular plurals
    irregulars := map[string]string{
        "people":   "person",
        "children": "child",
        "men":      "man",
        "women":    "woman",
        "teeth":    "tooth",
        "feet":     "foot",
        "mice":     "mouse",
        "geese":    "goose",
    }
    if singular, ok := irregulars[word]; ok {
        return singular
    }

    // Handle -ies → -y (categories → category)
    if strings.HasSuffix(word, "ies") && len(word) > 3 {
        return word[:len(word)-3] + "y"
    }

    // Handle -ves → -f (knives → knife)
    if strings.HasSuffix(word, "ves") && len(word) > 3 {
        return word[:len(word)-3] + "f"
    }

    // Handle -es after s, x, z, ch, sh
    if strings.HasSuffix(word, "es") && len(word) > 2 {
        base := word[:len(word)-2]
        if strings.HasSuffix(base, "s") || strings.HasSuffix(base, "x") ||
           strings.HasSuffix(base, "z") || strings.HasSuffix(base, "ch") ||
           strings.HasSuffix(base, "sh") {
            return base
        }
        // Check for -ses (addresses → address)
        if strings.HasSuffix(base, "ss") {
            return base
        }
    }

    // Handle regular -s
    if strings.HasSuffix(word, "s") && len(word) > 1 {
        // Don't singularize words that end in 'ss' (e.g., "class")
        if !strings.HasSuffix(word, "ss") {
            return word[:len(word)-1]
        }
    }

    return word
}
```

## Files to Modify

1. **pkg/services/schema_change_detection.go:148-165**
   - Replace `toEntityName` implementation with proper singularization
   - Add import for inflection library

2. **pkg/services/data_change_detection.go:330-333**
   - Replace naive singularization with proper implementation
   - Add import for inflection library

3. **go.mod** (if using external library)
   - Add `github.com/jinzhu/inflection` dependency

4. **pkg/utils/inflection.go** (if implementing custom)
   - Create new utility file with `Singularize` function

## Testing Verification

After implementing, verify these conversions:

| Table Name | Expected Entity | Current (Wrong) |
|------------|----------------|-----------------|
| categories | Category | Categorie |
| activities | Activity | Activitie |
| boxes | Box | Boxe |
| addresses | Address | Addresse |
| companies | Company | Companie |
| users | User | User ✓ |
| orders | Order | Order ✓ |
| knives | Knife | Knive |
| people | Person | People |

Test with MCP:
```
1. Create tables: s_categories, s_activities, s_boxes
2. Call refresh_schema
3. Check pending_changes for entity name suggestions
4. Verify suggestions are correctly singularized
```

## Notes

- The `jinzhu/inflection` library is widely used (Rails inflector port to Go)
- It handles most English pluralization rules including irregulars
- Alternative: `github.com/gobuffalo/flect` (from Buffalo framework)
- The custom implementation is sufficient for 90%+ of cases but misses edge cases

## Tasks

- [x] Add inflection library dependency (`go get github.com/jinzhu/inflection`)
- [x] Update `toEntityName` in `pkg/services/schema_change_detection.go` to use inflection.Singular
- [ ] Update table name mapping in `pkg/services/data_change_detection.go` to use inflection.Singular
- [ ] Add unit tests for singularization edge cases
- [ ] Manual verification with MCP refresh_schema
