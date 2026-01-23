# FIX: BUG-6 - Low/Zero Entity Occurrence Counts

**Bug Reference:** BUGS-ontology-extraction.md, BUG-6
**Severity:** Medium
**Type:** Symptom of Other Bugs

## Problem Summary

Several real Tikr entities have `occurrence_count: 0`:
- Billing Engagement: 0
- Billing Transaction: 0
- Billing Activity Message: 0
- Engagement Payment Intent: 0
- Session: 0
- Offer: 0

## Root Cause Analysis

### What is `occurrence_count`?

`OccurrenceCount` is **computed at runtime** from inbound relationships, NOT stored in the database.

**File:** `pkg/services/entity_service.go`

```go
// computeOccurrences derives entity occurrences from inbound relationships.
// Each inbound relationship represents an occurrence of this entity at the source column location.
func (s *entityService) computeOccurrences(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
    // Get all inbound relationships (where this entity is the target)
    relationships, err := s.relationshipRepo.GetByTargetEntity(ctx, entityID)
    // ... converts relationships to occurrences
}
```

**Key insight:** `OccurrenceCount = len(inbound_relationships)`

### Why Counts Are Zero

If an entity has **no inbound relationships** (no other entities point to it via FK), its occurrence count is 0.

Example:
- User entity has high occurrence count because many tables have `user_id` FKs pointing to it
- Billing Engagement has 0 occurrences because no relationships were discovered (BUG-3)

### Connection to Other Bugs

| Bug | Connection |
|-----|------------|
| **BUG-3** | Missing FK relationships → 0 inbound relationships → 0 occurrences |
| **BUG-9** | Stats collection failures → NULL stats → columns filtered out → no relationships |

**BUG-6 is a SYMPTOM, not a root cause.** Fixing BUG-3 and BUG-9 will automatically fix occurrence counts.

## The Fix

### No Direct Code Changes Needed

Since `occurrence_count` is computed from relationships:
1. Fix BUG-3 (Missing FK relationships) → relationships discovered
2. Fix BUG-9 (Stats collection) → more columns have stats → more relationships discovered
3. Occurrence counts automatically become correct

### Verification After Fixing Dependencies

After BUG-3/BUG-9 are fixed:

```sql
-- Check entities and their inbound relationship counts
SELECT e.name, COUNT(r.id) as inbound_rels
FROM engine_ontology_entities e
LEFT JOIN engine_entity_relationships r ON r.target_entity_id = e.id
JOIN engine_ontologies o ON e.ontology_id = o.id
WHERE o.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND o.is_active = true
AND e.deleted_at IS NULL
GROUP BY e.id, e.name
ORDER BY inbound_rels DESC;
```

**Expected after fix:**
- User: High count (many tables reference users)
- Account: Medium count (users, channels reference accounts)
- Billing Engagement: Non-zero (billing_transactions reference it)
- Session: Non-zero (if participants reference sessions)

### Optional Enhancement: Outbound Occurrence Tracking

Currently, occurrences only count **inbound** relationships (where entity is target). Could extend to track:
- Outbound relationships (where entity is source)
- Both directions for comprehensive usage tracking

**Not recommended for BUG-6:** Would change behavior. Current approach (inbound = where entity is referenced) is semantically correct.

## Testing

### After Dependencies Are Fixed

```python
# Run ontology extraction
# Then verify occurrence counts via API:
GET /api/v1/projects/{project_id}/entities

# Check Billing Engagement:
{
  "name": "Billing Engagement",
  "occurrence_count": 4  // visitor, host, session, offer references
}
```

### SQL Verification

```sql
-- Verify Billing Engagement has inbound relationships
SELECT r.source_column_table, r.source_column_name, r.association
FROM engine_entity_relationships r
JOIN engine_ontology_entities e ON r.target_entity_id = e.id
WHERE e.name = 'Billing Engagement';

-- Should show:
-- billing_transactions | engagement_id | references
-- (etc.)
```

## Success Criteria

- [ ] Entities with FK references to them have occurrence_count > 0
- [ ] Occurrence count matches number of inbound relationships
- [ ] Entities without inbound references correctly have count = 0
- [x] BUG-3 and BUG-9 are fixed first

## Dependencies

| Dependency | Status | Impact |
|------------|--------|--------|
| BUG-3 (Missing FK relationships) | ✅ Fixed | Root cause of missing relationships |
| BUG-9 (Stats collection) | ✅ Fixed | Root cause of missing stats → filtered columns |

## Implementation Order

1. **First:** Fix BUG-9 (Stats collection) - ensures all columns have stats
2. **Second:** Fix BUG-3 (Missing FK relationships) - ensures columns aren't filtered
3. **Third:** Re-run ontology extraction
4. **Fourth:** Verify BUG-6 is resolved

## Notes

This bug demonstrates the importance of understanding data flow:

```
Stats Collection → Columns have distinct_count
                        ↓
              Relationship Discovery filters columns
                        ↓
              Relationships created between entities
                        ↓
              Occurrence count computed from relationships
```

A failure at any upstream step propagates downstream. BUG-6 is the visible symptom of upstream failures (BUG-3, BUG-9).
