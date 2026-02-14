# ISSUE: MCP Tools Expose Unselected Schema to Clients

## Observed

When an MCP client calls `search_schema`, it returns all tables and columns in the datasource regardless of whether the user selected them for ontology extraction. The client then attempts to update columns on non-selected tables (via `update_column`), which either fails with "table not found" or enriches tables the user didn't intend to include.

This also increases hallucination risk -- seeing table names like `billing_engagements` and `offers` led the MCP client to hallucinate a non-existent table `billing_offers`.

## Steps to Reproduce

1. Connect an MCP client to a project with a partial schema selection (not all tables selected)
2. Call `search_schema` with a broad query like "offer"
3. Observe that results include tables the user did not select (e.g., `billing_transactions`, `billing_engagements`)
4. Attempt `update_column` on a non-selected table -- either fails or enriches unintended tables

## Affected Tools

- `search_schema` - Returns all tables/columns, not just selected ones
- `get_schema` - Has `selected_only` parameter but defaults to `false`
- `list_ontology_questions` - Returns questions that may reference non-selected tables (separate fix tracked in PLAN-app-activation-notify-central branch)

## Expected Behavior

MCP tools should only expose selected tables/columns to clients by default. The client should never see schema elements the user chose to exclude. This prevents:
- Hallucinated table names from combining fragments of visible but non-actionable tables
- Failed `update_column` calls on non-selected tables
- Wasted LLM tokens processing irrelevant schema
