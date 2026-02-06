# SECURITY: Cross-Account Access to Glossary Terms via RLS Policy

**Severity:** Critical
**Type:** Data Leakage / Multi-Tenant Isolation Violation
**Discovered:** 2026-01-24

## Summary

The `engine_business_glossary` table's Row-Level Security (RLS) policy allows SELECT access to records from other projects when `app.current_project_id` is set to a different project ID.

## Evidence

With RLS context set to project `2bb984fc-a677-45e9-94ba-9f65712ade70`:

```sql
SELECT set_config('app.current_project_id', '2bb984fc-a677-45e9-94ba-9f65712ade70', false);
SELECT term, source, ontology_id FROM engine_business_glossary;
```

**Returned records from a DIFFERENT project:**

| term | source | ontology_id | actual project_id |
|------|--------|-------------|-------------------|
| Active Threads | mcp | 52cb5185-8f25-449c-b6d4-c273a0943651 | 697ebf65-7992-4ebd-b741-1d7e1b2b6c02 |
| Recent Messages | mcp | 52cb5185-8f25-449c-b6d4-c273a0943651 | 697ebf65-7992-4ebd-b741-1d7e1b2b6c02 |

The records belong to project `697ebf65-7992-4ebd-b741-1d7e1b2b6c02` but were visible when context was set to `2bb984fc-a677-45e9-94ba-9f65712ade70`.

## Interesting Behavior

- SELECT returned records from the wrong project
- DELETE returned 0 rows (correctly blocked by RLS or different policy)

This suggests the RLS policy may have different conditions for SELECT vs DELETE, or there's a policy misconfiguration.

## Tables to Investigate

1. **`engine_business_glossary`** - Confirmed affected
2. **`engine_project_knowledge`** - Same structure (project_id based), likely same issue
3. **Other RLS-protected tables** - May have similar issues

## How to Investigate

1. Check the RLS policies on `engine_business_glossary`:
   ```sql
   SELECT * FROM pg_policies WHERE tablename = 'engine_business_glossary';
   ```

2. Compare with policies on tables that work correctly (e.g., `engine_ontologies`)

3. Check if the policy references the correct column or has OR conditions that bypass project filtering

4. Test other RLS-protected tables for the same issue

## Expected RLS Policy

The policy should enforce:
```sql
project_id = current_setting('app.current_project_id')::uuid
```

For all operations (SELECT, INSERT, UPDATE, DELETE).

## Impact

- **Data Leakage**: One tenant can see another tenant's glossary terms
- **Business Terms Exposure**: Glossary terms may contain sensitive business logic, calculations, or SQL
- **Compliance Risk**: Multi-tenant isolation is a fundamental security requirement

## Partial Mitigation

A WARN-level log was added at server startup (commit `4dba72e`) that detects superuser or bypassrls privileges on the engine database connection and alerts the operator. This makes the risk visible but does not fix the underlying RLS policy misconfiguration.

## Mitigation

1. Fix the RLS policy immediately
2. Audit access logs for any cross-tenant access
3. Review all other RLS policies for similar issues
4. Consider adding integration tests for RLS isolation
