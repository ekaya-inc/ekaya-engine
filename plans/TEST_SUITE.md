# TEST SUITE: MCP Tools Verification - 2026-01-31

This file documents a comprehensive test pass of all Ekaya MCP tools. Run this test suite after changes to verify tool functionality.

---

## Quick Start

When the user says **"Run MCP test suite"** or **"Test the MCP tools"**:
1. Read this file
2. Follow the TEST PLAYBOOK section exactly (26 read-only tools)
3. Document any failures in the RESULTS section at the bottom
4. If all pass, update the "Last Successful Run" date

When the user says **"Run MCP write tests"** or **"Test write operations"**:
1. Verify connected to `test_data` database (not production!)
2. Follow the TEST_DATA WRITE OPERATIONS PLAYBOOK section (39 additional tests)
3. Tests create, update, and delete data - only safe on test_data

When the user says **"Run full MCP test suite"**:
1. Run both playbooks in sequence (65 total tests)

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

### Phase 4: Metadata Tools (3 tools)

Test column metadata retrieval:

```
13. get_column_metadata(table="users", column="deleted_at")
    Expected: schema info + metadata with description, semantic_type
    Known Issue: semantic_type may be "audit_updated" instead of "soft_delete"

14. probe_column(table="users", column="avg_rating")
    Expected: semantic info + features with purpose, semantic_type, confidence
    Known Issue: purpose may show "text" for numeric columns

15. probe_columns(columns=[{"table":"users","column":"deleted_at"},{"table":"users","column":"marker_at"}])
    Expected: results object with both columns' data
```

### Phase 5: Glossary Tools (2 tools)

```
16. list_glossary()
    Expected: terms array with 10 terms, each with enrichment_status="success"

17. get_glossary_sql(term="Host Earnings")
    Expected: term, definition, defining_sql, base_table, output_columns
```

### Phase 6: Question Tools (1 tool)

```
18. list_ontology_questions(status="answered", limit=3)
    Expected: questions array with answered questions, counts_by_status
```

### Phase 7: Change Management (2 tools)

```
19. list_pending_changes(status="pending", limit=5)
    Expected: changes array (may be empty)

20. scan_data_changes(tables="users")
    Expected: total_changes count, changes array
```

### Phase 8: Approved Queries (3 tools)

```
21. list_approved_queries()
    Expected: queries array with at least 1 query

22. execute_approved_query(query_id="<id from step 21>", limit=3)
    Expected: query_name, rows array with data

23. get_query_history(limit=5)
    Expected: recent_queries array (may be empty)
```

### Phase 9: Query Suggestions (1 tool)

```
24. list_query_suggestions(status="pending")
    Expected: suggestions array (may be empty)
```

### Phase 10: Schema Refresh (1 tool)

```
25. refresh_schema(auto_select=false)
    Expected: tables_added, tables_removed, columns_added counts
```

### Phase 11: DDL Execution (1 tool)

```
26. execute(sql="SELECT 1 as test")
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
- [ ] All 26 tools return without error
- [ ] execute_approved_query() returns data

### Known Issues (Document but don't fail)
- get_context relationships=null
- table_count wrong in domain
- probe_column purpose/type mismatches

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
| 4. Metadata | 3 | | | |
| 5. Glossary | 2 | | | |
| 6. Questions | 1 | | | |
| 7. Changes | 2 | | | |
| 8. Approved | 3 | | | |
| 9. Suggestions | 1 | | | |
| 10. Refresh | 1 | | | |
| 11. DDL | 1 | | | |
| **Total** | **26** | | | |

### Failures
[List any failures with details]

### New Issues Discovered
[List any new issues]
```

---

## RESULTS: 2026-01-31 (Baseline - before entity tools removed)

**Tester:** Claude session
**Ekaya Version:** dev
**Duration:** ~5 minutes
**Note:** Entity and relationship tools were removed after this baseline.

### Results

| Phase | Tools | Pass | Fail | Notes |
|-------|-------|------|------|-------|
| 1. Connection | 3 | 3 | 0 | All healthy |
| 2. Context | 4 | 4 | 0 | Known issues documented |
| 3. Query | 5 | 5 | 0 | sample() fixed! |
| 4. Metadata | 3 | 3 | 0 | Semantic type issues |
| 5. Glossary | 2 | 2 | 0 | 10/10 terms valid |
| 6. Questions | 1 | 1 | 0 | 124 answered |
| 7. Changes | 2 | 2 | 0 | No pending changes |
| 8. Approved | 3 | 3 | 0 | 1 query available |
| 9. Suggestions | 1 | 1 | 0 | None pending |
| 10. Refresh | 1 | 1 | 0 | No changes detected |
| 11. DDL | 1 | 1 | 0 | SELECT works |
| **Total** | **26** | **26** | **0** | |

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
| update_table | Modifies ontology | Test on dev datasource |
| update_project_knowledge | Modifies ontology | Test on dev datasource |
| update_glossary_term | Modifies ontology | Test on dev datasource |
| create_glossary_term | Creates data | Test on dev datasource |
| create_approved_query | Creates data | Test on dev datasource |
| delete_column_metadata | Deletes data | Test on dev datasource |
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
2. Follow phases W1-W7 in order (later phases depend on earlier ones)
3. Document any failures

### Phase W1: Table & Column Metadata (5 tools)

Test table and column metadata updates:

```
W1. update_table(table="users", description="Test description for users table", usage_notes="Primary user data table")
    Expected: success message
    Verify: Description appears in get_context

W2. update_column(table="users", column="created_at", description="Timestamp when user was created", semantic_type="timestamp")
    Expected: success message

W3. get_column_metadata(table="users", column="created_at")
    Expected: description="Timestamp when user was created"

W4. delete_column_metadata(table="users", column="created_at")
    Expected: success message
    Verify: Custom description cleared (schema info preserved)

W5. delete_table_metadata(table="users")
    Expected: success message
    Verify: Custom table metadata cleared
```

### Phase W2: Project Knowledge CRUD (3 tools)

Test project-level facts:

```
W6. update_project_knowledge(fact="Test fact: MCP testing in progress", category="convention", context="Added during MCP test suite")
    Expected: success message with fact_id

W7. list_ontology_questions(limit=1)
    Note: Just to verify project knowledge doesn't break other tools

W8. delete_project_knowledge(fact_id="<fact_id from W6>")
    Expected: success message
    Note: Requires fact_id from W6 response
```

### Phase W3: Glossary Term CRUD (6 tools)

Test glossary term lifecycle:

```
W9. create_glossary_term(term="Test Metric", definition="A metric created for testing", defining_sql="SELECT COUNT(*) as test_count FROM users", base_table="users")
    Expected: success message, term created

W10. list_glossary()
     Expected: "Test Metric" appears in terms array

W11. get_glossary_sql(term="Test Metric")
     Expected: term, definition, defining_sql with "SELECT COUNT(*)"

W12. update_glossary_term(term="Test Metric", definition="Updated test metric definition", aliases=["TM", "TestM"])
     Expected: success message

W13. get_glossary_sql(term="TM")
     Expected: Resolves alias to "Test Metric"

W14. delete_glossary_term(term="Test Metric")
     Expected: success message
     Verify: Term no longer in list_glossary()
```

### Phase W4: Approved Query CRUD (5 tools)

Test approved query lifecycle:

```
W15. create_approved_query(name="Test Query", description="Query for MCP testing", sql="SELECT id, created_at FROM users WHERE created_at > {{start_date}}", parameters=[{"name":"start_date","type":"date","description":"Start date filter","required":true}], tags=["test","mcp"])
     Expected: success message with query_id

W16. list_approved_queries(tags=["test"])
     Expected: "Test Query" appears in queries array

W17. execute_approved_query(query_id="<id from W15>", parameters={"start_date":"2020-01-01"}, limit=5)
     Expected: rows array with user data

W18. update_approved_query(query_id="<id from W15>", description="Updated description for test query", tags=["test","mcp","updated"])
     Expected: success message

W19. delete_approved_query(query_id="<id from W15>")
     Expected: success message
     Verify: Query no longer in list_approved_queries()
```

### Phase W5: Query Suggestions (9 tools)

Test suggestion workflow:

```
W20. suggest_approved_query(name="Suggested Test Query", description="A suggested query", sql="SELECT COUNT(*) as user_count FROM users")
     Expected: success message with suggestion_id

W21. list_query_suggestions(status="pending")
     Expected: "Suggested Test Query" in suggestions array

W22. reject_query_suggestion(suggestion_id="<id from W20>", reason="Rejected during MCP testing")
     Expected: success message
     Verify: Suggestion status changed to rejected

W23. suggest_approved_query(name="Another Test Query", description="Will be approved", sql="SELECT 1 as test")
     Expected: success message with suggestion_id

W24. approve_query_suggestion(suggestion_id="<id from W23>")
     Expected: success message, query now in approved list

W25. suggest_query_update(query_id="<query_id from W24>", sql="SELECT 2 as updated_test", context="Testing suggest_query_update tool")
     Expected: success message with suggestion_id for update

W26. list_query_suggestions(status="pending")
     Expected: Update suggestion appears

W27. reject_query_suggestion(suggestion_id="<id from W25>", reason="Update rejected during testing")
     Expected: success message

W28. delete_approved_query(query_id="<query_id from approved suggestion>")
     Expected: Cleanup the approved query
```

### Phase W6: Ontology Questions (6 tools)

Test question status management:

```
W29. list_ontology_questions(status="pending", limit=5)
     Expected: questions array (may be empty)
     Note: If empty, skip W30-W34

W30. skip_ontology_question(question_id="<first pending question>", reason="Skipped during MCP testing")
     Expected: success message
     Verify: Question status changed to "skipped"

W31. list_ontology_questions(status="skipped", limit=1)
     Expected: The skipped question appears

W32. dismiss_ontology_question(question_id="<a pending question>", reason="Not relevant - MCP test")
     Expected: success message
     Verify: Question status changed to "dismissed"

W33. escalate_ontology_question(question_id="<a pending question>", reason="Needs human review - MCP test")
     Expected: success message
     Verify: Question status changed to "escalated"

W34. resolve_ontology_question(question_id="<a pending question>", resolution_notes="Resolved during MCP testing")
     Expected: success message
     Verify: Question status changed to "answered"
```

### Phase W7: Change Management (5 tools)

Test change approval workflow:

```
W35. scan_data_changes(tables="users")
     Expected: changes array (creates pending changes if data changed)

W36. list_pending_changes(status="pending", limit=5)
     Expected: changes array
     Note: If empty, W37-W38 will have nothing to test

W37. reject_change(change_id="<first pending change>")
     Expected: success message (if pending changes exist)
     Note: Only run if W36 returned changes

W38. approve_change(change_id="<second pending change>")
     Expected: success message (if pending changes exist)
     Note: Only run if W36 returned multiple changes

W39. approve_all_changes()
     Expected: summary of approved/skipped changes
```

---

## WRITE TESTS PASS/FAIL CRITERIA

### Must Pass (Critical)
- [ ] create_glossary_term() creates term successfully
- [ ] create_approved_query() creates query successfully
- [ ] execute_approved_query() runs created query

### Should Pass (Important)
- [ ] All CRUD operations complete without error
- [ ] Delete operations remove created test data
- [ ] Query suggestions workflow completes

### Cleanup Verification
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
| W1. Table/Column | 5 | | | |
| W2. Knowledge | 3 | | | |
| W3. Glossary | 6 | | | |
| W4. Approved Query | 5 | | | |
| W5. Suggestions | 9 | | | |
| W6. Questions | 6 | | | |
| W7. Changes | 5 | | | |
| **Total** | **39** | | | |

### Created IDs (for cleanup reference)
- Glossary: Test Metric
- Query ID: [from W15]
- Approved Query from Suggestion: [from W24]
- Suggestion IDs: [from W20, W23, W25]
- Knowledge fact_id: [from W6]

### Failures
[List any failures with details]

### Cleanup Status
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
