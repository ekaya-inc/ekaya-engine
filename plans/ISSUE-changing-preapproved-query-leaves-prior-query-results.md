# ISSUE: Switching pre-approved queries leaves stale results from previous query

**Status:** Open
**Page:** Pre-Approved Queries (`/projects/{id}/queries/view/{query_id}`)

## Observed

When viewing pre-approved queries, executing one query and then clicking on a different query in the sidebar shows the previous query's results under the new query's SQL.

**Screenshot evidence:** After executing "Customer Lifetime Value - Top 10" (10 rows, 4 columns: first_name, last_name, completed_orders, total_spend), clicking on "Top 3 Products per Category by Revenue" shows:
- The SQL panel correctly updates to the new query (DENSE_RANK with category, name, brand, etc.)
- The URL correctly updates to the new query ID
- The Query Results panel still shows "Showing 10 of 10 rows - 4 columns" with first_name, last_name, completed_orders, total_spend from the previous query

This is confusing because the results table doesn't match the SQL shown above it.

## Steps to reproduce

1. Navigate to Pre-Approved Queries page
2. Have multiple queries available (e.g., the 5 advanced SQL tutorial queries)
3. Click on any query and press [Execute Query]
4. Click on a different query in the sidebar
5. Observe the results panel still shows results from the previously executed query

## Expected behavior

When switching to a different query in the sidebar:
- If it has **not been executed** in this session: show no results (empty state)
- If it **has been executed** in this session: show its own cached results from the last execution
- A full page refresh should clear all cached results

## Actual behavior

The results panel retains whatever was last displayed, regardless of which query is now selected. The SQL updates but the results do not.

## Likely cause

The results state is stored as a single value at the page level rather than being keyed per query ID. When the selected query changes, the SQL panel re-renders but the results state is not cleared or swapped.
