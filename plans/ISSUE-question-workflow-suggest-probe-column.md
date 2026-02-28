# ISSUE: Question Workflow Should Suggest probe_column for Data Verification

Status: FIXED

## Observed

When an MCP client answers ontology questions (especially enumeration and business_rules categories), it would benefit from verifying actual data values before committing metadata. The `probe_column` tool exists for this purpose — it returns distinct values, statistics, and sample data — but nothing in the question workflow suggests using it.

As a result, MCP clients may commit enum values or descriptions based on assumptions rather than verified data.

## Example

Question: "What are the possible values for 'submission_method'?"

Ideal workflow:
1. Call `probe_column(table='mcp_directories', column='submission_method')` to see actual values
2. Call `update_column` with verified enum values
3. Call `resolve_ontology_question`

Actual workflow (what happened):
1. Answered from memory of what was inserted (correct in this case, but fragile)
2. Called `update_column`
3. Called `resolve_ontology_question`

## Suggestion

Include a hint in the question context (from `list_ontology_questions`) suggesting `probe_column` when the question is about column values. For example, for enumeration-category questions that reference specific columns, include something like:

> "Tip: Use probe_column(table='...', column='...') to inspect actual values before updating metadata."

This could be added to the question's `reasoning` field or as a new `suggested_tools` field in the response.
