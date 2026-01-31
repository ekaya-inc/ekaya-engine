# TEST SUITE: MCP Tools Verification - 2026-01-31

This file documents a comprehensive test pass of all Ekaya MCP tools. Run this test suite after changes to verify tool functionality.

---

## Quick Start

When the user says **"Run MCP test suite"** or **"Test the MCP tools"**:
1. Read this file
2. Follow the TEST PLAYBOOK section exactly (28 read-only tools)
3. Document any failures in the RESULTS section at the bottom
4. If all pass, update the "Last Successful Run" date

When the user says **"Run MCP write tests"** or **"Test write operations"**:
1. Verify connected to `test_data` database (not production!)
2. Follow the TEST_DATA WRITE OPERATIONS PLAYBOOK section (46 additional tests)
3. Tests create, update, and delete data - only safe on test_data

When the user says **"Run full MCP test suite"**:
1. Run both playbooks in sequence (74 total tests)

**Last Successful Run (Read-Only):** 2026-01-31
**Last Successful Run (Write Tests):** 2026-01-31

---

## TEST PLAYBOOK

### Phase 1: Connection & Health (3 tools)

Run these first to verify basic connectivity:

```
1. health()
   Expected: {"engine":"healthy","version":"...","datasource":{"status":"connected"}}

2. echo(message="MCP test")
   Expected: {"echo":"MCP test"}

3. get_schema(include_entities=true, selected_only=true)
   Expected: Large response with DOMAIN ENTITIES section and table definitions
   Verify: At least 30+ tables listed
```

### Phase 2: Context & Ontology (4 tools)

Test ontology retrieval at different depths:

```
4. get_context(depth="domain")
   Expected: domain description, primary_domains, conventions, glossary
   Known Issue: table_count may show wrong number

5. get_context(depth="entities", include_relationships=true)
   Expected: entities object with descriptions, key_columns, occurrences
   Known Issue: relationships may be null

6. get_ontology(depth="domain")
   Expected: domain, entities array, relationships array
   Verify: relationships is NOT null (unlike get_context)

7. search_schema(query="user", limit=5)
   Expected: tables, columns, entities arrays with relevance scores
   Verify: User entity found in entities array
```

### Phase 3: Query Execution (5 tools)

Test SQL execution capabilities:

```
8. query(sql="SELECT COUNT(*) as count FROM users", limit=10)
   Expected: {"columns":["count"],"rows":[{"count":NNNN}],"row_count":1}

9. sample(table="users", limit=3)
   Expected: 3 rows with all user columns
   CRITICAL: This previously crashed the server - verify it works!

10. validate(sql="SELECT * FROM nonexistent_table")
    Expected: {"valid":false,"error":"relation \"nonexistent_table\" does not exist"}

11. validate(sql="SELECT * FROM users LIMIT 1")
    Expected: {"valid":true}

12. explain_query(sql="SELECT * FROM users LIMIT 10")
    Expected: plan with execution details, performance_hints array
```

### Phase 4: Metadata Tools (4 tools)

Test column and entity metadata retrieval:

```
13. get_column_metadata(table="users", column="deleted_at")
    Expected: schema info + metadata with description, semantic_type
    Known Issue: semantic_type may be "audit_updated" instead of "soft_delete"

14. get_entity(name="User")
    Expected: name, primary_table, description, aliases, key_columns

15. probe_column(table="users", column="avg_rating")
    Expected: semantic info + features with purpose, semantic_type, confidence
    Known Issue: purpose may show "text" for numeric columns

16. probe_columns(columns=[{"table":"users","column":"deleted_at"},{"table":"users","column":"marker_at"}])
    Expected: results object with both columns' data
```

### Phase 5: Relationship Tools (1 tool)

```
17. probe_relationship(from_entity="User")
    Expected: relationships array (may be empty)
    Known Issue: Returns empty even for entities with FKs
```

### Phase 6: Glossary Tools (2 tools)

```
18. list_glossary()
    Expected: terms array with 10 terms, each with enrichment_status="success"

19. get_glossary_sql(term="Host Earnings")
    Expected: term, definition, defining_sql, base_table, output_columns
```

### Phase 7: Question Tools (1 tool)

```
20. list_ontology_questions(status="answered", limit=3)
    Expected: questions array with answered questions, counts_by_status
```

### Phase 8: Change Management (2 tools)

```
21. list_pending_changes(status="pending", limit=5)
    Expected: changes array (may be empty)

22. scan_data_changes(tables="users")
    Expected: total_changes count, changes array
```

### Phase 9: Approved Queries (3 tools)

```
23. list_approved_queries()
    Expected: queries array with at least 1 query

24. execute_approved_query(query_id="<id from step 23>", limit=3)
    Expected: query_name, rows array with data

25. get_query_history(limit=5)
    Expected: recent_queries array (may be empty)
```

### Phase 10: Query Suggestions (1 tool)

```
26. list_query_suggestions(status="pending")
    Expected: suggestions array (may be empty)
```

### Phase 11: Schema Refresh (1 tool)

```
27. refresh_schema(auto_select=false)
    Expected: tables_added, tables_removed, columns_added counts
```

### Phase 12: DDL Execution (1 tool)

```
28. execute(sql="SELECT 1 as test")
    Expected: {"columns":["test"],"rows":[{"test":1}],"row_count":1}
```

---

## PASS/FAIL CRITERIA

### Must Pass (Critical)
- [ ] health() returns healthy
- [ ] sample() does NOT crash the server
- [ ] query() executes successfully
- [ ] list_glossary() returns terms with success status

### Should Pass (Important)
- [ ] All 28 tools return without error
- [ ] get_entity() finds User entity
- [ ] execute_approved_query() returns data

### Known Issues (Document but don't fail)
- get_context relationships=null
- table_count wrong in domain
- probe_column purpose/type mismatches
- probe_relationship returns empty

---

## TEST EXECUTION TEMPLATE

Copy this for documenting a test run:

```markdown
## Test Run: YYYY-MM-DD

**Tester:** [Claude session / Human]
**Ekaya Version:** [from health response]
**Duration:** [time to complete]

### Results

| Phase | Tools | Pass | Fail | Notes |
|-------|-------|------|------|-------|
| 1. Connection | 3 | | | |
| 2. Context | 4 | | | |
| 3. Query | 5 | | | |
| 4. Metadata | 4 | | | |
| 5. Relationship | 1 | | | |
| 6. Glossary | 2 | | | |
| 7. Questions | 1 | | | |
| 8. Changes | 2 | | | |
| 9. Approved | 3 | | | |
| 10. Suggestions | 1 | | | |
| 11. Refresh | 1 | | | |
| 12. DDL | 1 | | | |
| **Total** | **28** | | | |

### Failures
[List any failures with details]

### New Issues Discovered
[List any new issues]
```

---

## RESULTS: 2026-01-31 (Baseline)

**Tester:** Claude session
**Ekaya Version:** dev
**Duration:** ~5 minutes

### Results

| Phase | Tools | Pass | Fail | Notes |
|-------|-------|------|------|-------|
| 1. Connection | 3 | 3 | 0 | All healthy |
| 2. Context | 4 | 4 | 0 | Known issues documented |
| 3. Query | 5 | 5 | 0 | sample() fixed! |
| 4. Metadata | 4 | 4 | 0 | Semantic type issues |
| 5. Relationship | 1 | 1 | 0 | Returns empty |
| 6. Glossary | 2 | 2 | 0 | 10/10 terms valid |
| 7. Questions | 1 | 1 | 0 | 124 answered |
| 8. Changes | 2 | 2 | 0 | No pending changes |
| 9. Approved | 3 | 3 | 0 | 1 query available |
| 10. Suggestions | 1 | 1 | 0 | None pending |
| 11. Refresh | 1 | 1 | 0 | No changes detected |
| 12. DDL | 1 | 1 | 0 | SELECT works |
| **Total** | **28** | **28** | **0** | |

### Critical Fix Verified
- `sample(table="users", limit=3)` no longer crashes the server

### Known Issues (Not Failures)
See ISSUES-ontology-enrichment-2026-01-31.md for full list

---

## TOOLS NOT IN AUTOMATED SUITE (Production)

These tools require write operations. On production datasources, test manually.
**On `test_data` database:** These are covered in the TEST_DATA WRITE OPERATIONS PLAYBOOK below.

| Tool | Reason | Manual Test Procedure |
|------|--------|----------------------|
| update_column | Modifies ontology | Test on dev datasource |
| update_entity | Modifies ontology | Test on dev datasource |
| update_relationship | Modifies ontology | Test on dev datasource |
| update_table | Modifies ontology | Test on dev datasource |
| update_project_knowledge | Modifies ontology | Test on dev datasource |
| update_glossary_term | Modifies ontology | Test on dev datasource |
| create_glossary_term | Creates data | Test on dev datasource |
| create_approved_query | Creates data | Test on dev datasource |
| delete_column_metadata | Deletes data | Test on dev datasource |
| delete_entity | Deletes data | Test on dev datasource |
| delete_relationship | Deletes data | Test on dev datasource |
| delete_table_metadata | Deletes data | Test on dev datasource |
| delete_project_knowledge | Deletes data | Test on dev datasource |
| delete_glossary_term | Deletes data | Test on dev datasource |
| delete_approved_query | Deletes data | Test on dev datasource |
| update_approved_query | Modifies data | Test on dev datasource |
| suggest_approved_query | Creates suggestion | Test on dev datasource |
| suggest_query_update | Creates suggestion | Test on dev datasource |
| approve_query_suggestion | Modifies data | Test on dev datasource |
| reject_query_suggestion | Modifies data | Test on dev datasource |
| approve_change | Applies change | Test on dev datasource |
| reject_change | Rejects change | Test on dev datasource |
| approve_all_changes | Applies changes | Test on dev datasource |
| resolve_ontology_question | Modifies ontology | Tested during benchmark |
| skip_ontology_question | Modifies ontology | Test on dev datasource |
| dismiss_ontology_question | Modifies ontology | Test on dev datasource |
| escalate_ontology_question | Modifies ontology | Test on dev datasource |

---

## TEST_DATA WRITE OPERATIONS PLAYBOOK

**IMPORTANT:** Only run this section when connected to the `test_data` database. These tests modify data.

When the user says **"Run MCP write tests"** or **"Test write operations"**:
1. Verify you are connected to `test_data` (check health response)
2. Follow phases W1-W8 in order (later phases depend on earlier ones)
3. Document any failures

### Phase W1: Entity CRUD (3 tools)

Test entity creation, update, and deletion:

```
W1. update_entity(name="TestEntity", description="A test entity for MCP testing", aliases=["test_ent", "te"])
    Expected: success message, entity created
    Verify: Entity appears in get_entity response

W2. update_entity(name="TestEntity", key_columns=["id", "name"], is_promoted=true)
    Expected: success message, entity updated
    Verify: key_columns and is_promoted updated

W3. get_entity(name="TestEntity")
    Expected: name="TestEntity", description, aliases=["test_ent","te"], key_columns=["id","name"]
    Verify: All fields from W1 and W2 are present

W4. delete_entity(name="TestEntity")
    Expected: success message
    Verify: get_entity("TestEntity") returns not found
```

### Phase W2: Relationship CRUD (2 tools)

Test relationship creation and deletion:

```
W5. update_relationship(from_entity="User", to_entity="Account", label="owns", cardinality="1:N", description="User owns multiple accounts")
    Expected: success message, relationship created/updated
    Verify: Relationship appears in probe_relationship

W6. probe_relationship(from_entity="User", to_entity="Account")
    Expected: relationship with label="owns", cardinality="1:N"

W7. delete_relationship(from_entity="User", to_entity="Account")
    Expected: success message
    Note: Only delete if this was created in W5 (check if it existed before)
```

### Phase W3: Table & Column Metadata (4 tools)

Test table and column metadata updates:

```
W8. update_table(table="users", description="Test description for users table", usage_notes="Primary user data table")
    Expected: success message
    Verify: Description appears in get_context

W9. update_column(table="users", column="created_at", description="Timestamp when user was created", semantic_type="timestamp")
    Expected: success message

W10. get_column_metadata(table="users", column="created_at")
     Expected: description="Timestamp when user was created"

W11. delete_column_metadata(table="users", column="created_at")
     Expected: success message
     Verify: Custom description cleared (schema info preserved)

W12. delete_table_metadata(table="users")
     Expected: success message
     Verify: Custom table metadata cleared
```

### Phase W4: Project Knowledge CRUD (2 tools)

Test project-level facts:

```
W13. update_project_knowledge(fact="Test fact: MCP testing in progress", category="convention", context="Added during MCP test suite")
     Expected: success message with fact_id

W14. list_ontology_questions(limit=1)
     Note: Just to verify project knowledge doesn't break other tools

W15. delete_project_knowledge(fact_id="<fact_id from W13>")
     Expected: success message
     Note: Requires fact_id from W13 response
```

### Phase W5: Glossary Term CRUD (3 tools)

Test glossary term lifecycle:

```
W16. create_glossary_term(term="Test Metric", definition="A metric created for testing", defining_sql="SELECT COUNT(*) as test_count FROM users", base_table="users")
     Expected: success message, term created

W17. list_glossary()
     Expected: "Test Metric" appears in terms array

W18. get_glossary_sql(term="Test Metric")
     Expected: term, definition, defining_sql with "SELECT COUNT(*)"

W19. update_glossary_term(term="Test Metric", definition="Updated test metric definition", aliases=["TM", "TestM"])
     Expected: success message

W20. get_glossary_sql(term="TM")
     Expected: Resolves alias to "Test Metric"

W21. delete_glossary_term(term="Test Metric")
     Expected: success message
     Verify: Term no longer in list_glossary()
```

### Phase W6: Approved Query CRUD (4 tools)

Test approved query lifecycle:

```
W22. create_approved_query(name="Test Query", description="Query for MCP testing", sql="SELECT id, created_at FROM users WHERE created_at > {{start_date}}", parameters=[{"name":"start_date","type":"date","description":"Start date filter","required":true}], tags=["test","mcp"])
     Expected: success message with query_id

W23. list_approved_queries(tags=["test"])
     Expected: "Test Query" appears in queries array

W24. execute_approved_query(query_id="<id from W22>", parameters={"start_date":"2020-01-01"}, limit=5)
     Expected: rows array with user data

W25. update_approved_query(query_id="<id from W22>", description="Updated description for test query", tags=["test","mcp","updated"])
     Expected: success message

W26. delete_approved_query(query_id="<id from W22>")
     Expected: success message
     Verify: Query no longer in list_approved_queries()
```

### Phase W7: Query Suggestions (4 tools)

Test suggestion workflow:

```
W27. suggest_approved_query(name="Suggested Test Query", description="A suggested query", sql="SELECT COUNT(*) as user_count FROM users")
     Expected: success message with suggestion_id

W28. list_query_suggestions(status="pending")
     Expected: "Suggested Test Query" in suggestions array

W29. reject_query_suggestion(suggestion_id="<id from W27>", reason="Rejected during MCP testing")
     Expected: success message
     Verify: Suggestion status changed to rejected

W30. suggest_approved_query(name="Another Test Query", description="Will be approved", sql="SELECT 1 as test")
     Expected: success message with suggestion_id

W31. approve_query_suggestion(suggestion_id="<id from W30>")
     Expected: success message, query now in approved list

W32. suggest_query_update(query_id="<query_id from W31>", sql="SELECT 2 as updated_test", context="Testing suggest_query_update tool")
     Expected: success message with suggestion_id for update

W33. list_query_suggestions(status="pending")
     Expected: Update suggestion appears

W34. reject_query_suggestion(suggestion_id="<id from W32>", reason="Update rejected during testing")
     Expected: success message

W35. delete_approved_query(query_id="<query_id from approved suggestion>")
     Expected: Cleanup the approved query
```

### Phase W8: Ontology Questions (4 tools)

Test question status management:

```
W36. list_ontology_questions(status="pending", limit=5)
     Expected: questions array (may be empty)
     Note: If empty, skip W37-W41

W37. skip_ontology_question(question_id="<first pending question>", reason="Skipped during MCP testing")
     Expected: success message
     Verify: Question status changed to "skipped"

W38. list_ontology_questions(status="skipped", limit=1)
     Expected: The skipped question appears

W39. dismiss_ontology_question(question_id="<a pending question>", reason="Not relevant - MCP test")
     Expected: success message
     Verify: Question status changed to "dismissed"

W40. escalate_ontology_question(question_id="<a pending question>", reason="Needs human review - MCP test")
     Expected: success message
     Verify: Question status changed to "escalated"

W41. resolve_ontology_question(question_id="<a pending question>", resolution_notes="Resolved during MCP testing")
     Expected: success message
     Verify: Question status changed to "answered"
```

### Phase W9: Change Management (3 tools)

Test change approval workflow:

```
W42. scan_data_changes(tables="users")
     Expected: changes array (creates pending changes if data changed)

W43. list_pending_changes(status="pending", limit=5)
     Expected: changes array
     Note: If empty, W44-W45 will have nothing to test

W44. reject_change(change_id="<first pending change>")
     Expected: success message (if pending changes exist)
     Note: Only run if W43 returned changes

W45. approve_change(change_id="<second pending change>")
     Expected: success message (if pending changes exist)
     Note: Only run if W43 returned multiple changes

W46. approve_all_changes()
     Expected: summary of approved/skipped changes
```

---

## WRITE TESTS PASS/FAIL CRITERIA

### Must Pass (Critical)
- [ ] update_entity() creates entity successfully
- [ ] create_glossary_term() creates term successfully
- [ ] create_approved_query() creates query successfully
- [ ] execute_approved_query() runs created query

### Should Pass (Important)
- [ ] All CRUD operations complete without error
- [ ] Delete operations remove created test data
- [ ] Query suggestions workflow completes

### Cleanup Verification
- [ ] TestEntity deleted
- [ ] Test Metric glossary term deleted
- [ ] Test Query approved query deleted
- [ ] No test artifacts remain

---

## WRITE TESTS EXECUTION TEMPLATE

```markdown
## Write Test Run: YYYY-MM-DD

**Tester:** [Claude session]
**Database:** test_data
**Duration:** [time to complete]

### Results

| Phase | Tools | Pass | Fail | Notes |
|-------|-------|------|------|-------|
| W1. Entity | 4 | | | |
| W2. Relationship | 3 | | | |
| W3. Table/Column | 5 | | | |
| W4. Knowledge | 3 | | | |
| W5. Glossary | 6 | | | |
| W6. Approved Query | 5 | | | |
| W7. Suggestions | 9 | | | |
| W8. Questions | 6 | | | |
| W9. Changes | 5 | | | |
| **Total** | **46** | | | |

### Created IDs (for cleanup reference)
- Entity: TestEntity
- Glossary: Test Metric
- Query ID: [from W22]
- Approved Query from Suggestion: [from W31]
- Suggestion IDs: [from W27, W30, W32]
- Knowledge fact_id: [from W13]

### Failures
[List any failures with details]

### Cleanup Status
- [ ] All test entities removed
- [ ] All test glossary terms removed
- [ ] All test queries removed
```

---

## CRASH DETECTION

If the server crashes during testing:

1. Note which tool was called
2. Note the exact parameters
3. Check if previous tools in sequence matter
4. Document in ISSUES file
5. Restart server and continue from next tool

**Previous Crash Sequence (Fixed):**
```
health → echo → get_schema → get_context → get_ontology →
search_schema → query → sample ← CRASHED HERE
```

---

## AUTOMATION NOTES

To automate this test suite:

1. Tools can be called in parallel within phases (no dependencies)
2. Phase 9 step 24 depends on step 23 (needs query_id)
3. All tools should complete within 30 seconds each
4. Total suite should complete in under 5 minutes
5. Consider adding to CI/CD after Ekaya server startup
