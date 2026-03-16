# ISSUE: Generated glossary SQL diverges from glossary definitions

**Date:** 2026-03-16
**Status:** TODO
**Priority:** HIGH
**Observed on:** MCP project `4cecc18a-b051-40c1-8308-ad3e3363b0a8`, datasource `the_look`
**Plan of record:** `plans/PLAN-glossary-sql-first-example-generation.md`

## Observation

The glossary generation flow is producing a mix of outcomes:

- Some glossary entries generate valid SQL that matches the written definition.
- Some entries generate SQL that runs but does not faithfully implement the definition.
- Some entries generate SQL that uses the wrong join path even though the ontology exposes the correct relationships.
- Some entries should not have produced final SQL at all because the definition is underspecified, requires parameters, or is not fully answerable from the available schema.

This makes the glossary the weakest remaining generation surface because bad SQL can look superficially plausible while encoding the wrong metric.

## Review scope

Ten glossary entries were reviewed. The stored SQL for each term was fetched from the glossary, executed when present, and compared against an independently derived SQL candidate using ontology/schema tools (`get_schema`, `get_ontology`, `get_context`, `probe_columns`).

## Partial mitigations completed

The core generation issue is still open, but a few adjacent problems have been fixed:

- The glossary UI now exposes an `Execute Query` action and reuses the shared query-results table so bad glossary SQL is visible immediately from the term details view.
- The glossary SQL editor now derives its CodeMirror dialect from the selected datasource instead of hardcoding PostgreSQL, so SQL Server-backed projects get the correct editor dialect.
- The glossary edit modal now stays constrained to the modal width for long SQL instead of stretching the page horizontally.
- Manual glossary edits now persist glossary provenance as `manual` rather than leaving the original inferred source in place.
- Glossary SQL validation now states explicitly that glossary definitions must return a single row.

These changes improve visibility, correctness of editing metadata, and SQL-editor behavior, but they do not solve the underlying glossary-generation quality problem described below.

## Findings by glossary term

| Term | Stored SQL outcome | Independent assessment | Current classification |
|------|--------------------|------------------------|------------------------|
| Customer Engagement | Ran and returned `67` | SQL shape is reasonable, but the definition says "within a defined time frame" and the stored SQL hardcodes `30 days` with no parameter or justification | Definition/process gap |
| Distribution Center Efficiency | Ran but returned `NULL` | Ontology exposes a valid path through `order_items` and `inventory_items` to `distribution_centers`; stored SQL incorrectly joins `orders.user_id = distribution_centers.id` | LLM SQL generation error |
| Inventory Availability | Ran and returned `1764` | Matches the definition closely | Working |
| Inventory Turnover | Ran and returned `1439.4298` | The schema lacks historical inventory snapshots, so the definition is not fully answerable as written; both stored and independent SQL are proxies | Definition/data-model gap |
| Order Completion Rate | Ran and returned `24.6293` | Matches the definition closely | Working |
| Order Fulfillment Rate | Ran and returned `65.4860` | Matches the definition closely | Working |
| Product Sales Velocity | Ran but returned `0` rows | Stored SQL depends on `inventory_items.sold_at`, but the datasource currently has no populated `sold_at` rows; `order_items` may be a better source, but that is a business interpretation choice | Definition/process gap |
| Return Rate | Ran and returned `10.9555` | Matches the definition closely | Working |
| Traffic Source Conversion | No stored SQL; glossary row already marked `failed` | A working query can be derived using `users.traffic_source` and `orders`, so this is not blocked by ontology absence | LLM SQL generation error |
| User Location Density | Ran and returned `100.0000` | Stored SQL computes percent of users with coordinates, not density per square kilometer within defined latitude/longitude bounds | Definition/process gap |

## Concrete examples

### 1. Distribution Center Efficiency uses the wrong relationship path

Stored SQL:

```sql
SELECT AVG(EXTRACT(EPOCH FROM (orders.shipped_at - orders.created_at)) / 3600) AS distribution_center_efficiency
FROM orders
JOIN distribution_centers dc ON orders.user_id = dc.id
WHERE dc.id = orders.user_id
```

Observed behavior:

- Query executed but returned `NULL`
- Validation query showed the join produces `0` rows

Relevant ontology/schema facts available during generation:

- `order_items.order_id -> orders.order_id`
- `order_items.inventory_item_id -> inventory_items.id`
- `inventory_items.product_distribution_center_id -> distribution_centers.id`

Independent SQL based on the ontology produced valid per-center results:

```sql
WITH dc_orders AS (
  SELECT DISTINCT
    o.order_id,
    ii.product_distribution_center_id AS distribution_center_id,
    o.created_at,
    o.shipped_at
  FROM orders o
  JOIN order_items oi ON oi.order_id = o.order_id
  JOIN inventory_items ii ON ii.id = oi.inventory_item_id
  WHERE o.shipped_at IS NOT NULL
    AND ii.product_distribution_center_id IS NOT NULL
)
SELECT
  dc.id AS distribution_center_id,
  dc.name AS distribution_center_name,
  AVG(EXTRACT(EPOCH FROM (dco.shipped_at - dco.created_at)) / 3600.0) AS distribution_center_efficiency_hours
FROM dc_orders dco
JOIN distribution_centers dc ON dc.id = dco.distribution_center_id
GROUP BY dc.id, dc.name
ORDER BY dc.id
```

This returned 10 distribution-center rows with averages ranging from about `32.37` to `41.55` hours.

### 2. Traffic Source Conversion failed even though a workable interpretation exists

Glossary row state:

- `defining_sql` is empty
- `enrichment_status = failed`
- Stored error: `more than one row returned by a subquery used as an expression`

Relevant ontology/schema facts available during generation:

- `users.traffic_source` is documented as the user acquisition/referral source
- `events.traffic_source` is documented as the session/event acquisition channel
- `orders.user_id -> users.id`

Independent SQL based on the written definition and available ontology:

```sql
SELECT
  u.traffic_source,
  COUNT(o.order_id) FILTER (WHERE o.status = 'Complete') * 1.0
    / NULLIF(COUNT(DISTINCT u.id), 0) AS traffic_source_conversion
FROM users u
LEFT JOIN orders o ON o.user_id = u.id
GROUP BY u.traffic_source
ORDER BY u.traffic_source
```

This returned valid per-source rates for 5 sources (`Display`, `Email`, `Facebook`, `Organic`, `Search`).

Note: the definition is still somewhat ambiguous because it could mean:

- completed orders per acquired user, or
- users who converted per acquired user

That ambiguity should trigger a clarification or a required metric shape, not an invalid SQL artifact.

### 3. User Location Density does not match its definition

Definition:

> Concentration of users in geographic areas, measured by the number of users per square kilometer within defined latitude and longitude ranges.

Stored SQL:

```sql
SELECT COUNT(*) * 100.0 / NULLIF((SELECT COUNT(*) FROM users), 0) AS user_location_density
FROM users
WHERE latitude IS NOT NULL AND longitude IS NOT NULL
```

Observed behavior:

- Query ran and returned `100.0000`
- This is "percent of users with coordinates", not "users per square kilometer within defined latitude/longitude ranges"

Independent check:

- The definition requires geographic bounds or another area definition
- Without those parameters, the metric is underspecified
- A rough bounding-box approximation can be computed, but that is already a new interpretation, not a faithful implementation of the definition

### 4. Product Sales Velocity likely uses the wrong operational source

Stored SQL:

```sql
SELECT
  p.id AS product_id,
  COUNT(i.id) * 1.0 / NULLIF(DATE_PART('day', CURRENT_DATE - MIN(i.created_at)), 0) AS product_sales_velocity
FROM inventory_items i
JOIN products p ON i.product_id = p.id
WHERE i.sold_at IS NOT NULL
GROUP BY p.id
```

Observed behavior:

- Query ran but returned `0` rows
- The datasource currently has `0` `inventory_items` with `sold_at IS NOT NULL`
- `order_items` does contain fulfillment states and timestamps (`shipped_at`, `delivered_at`, `returned_at`)

This indicates one of two problems:

- the definition picked a source column that is not reliably populated in this datasource, or
- the generation process should have preferred `order_items` as the operational sales table

Either way, the current process should not silently bless a query that has no usable data path in the current dataset.

### 5. Customer Engagement and Inventory Turnover reveal underspecified metric definitions

Customer Engagement:

- Definition says "within a defined time frame"
- Stored SQL chooses `NOW() - INTERVAL '30 days'`
- No parameter, glossary field, or ontology fact currently justifies `30 days`

Inventory Turnover:

- Definition says "total sales value / average inventory value" within a period
- The current schema does not expose historical inventory-value snapshots
- Both stored and independent SQL necessarily fall back to a proxy based on available `order_items.sale_price` and inventory/item cost data

These are not pure SQL-generation failures. They are cases where the generation pipeline should require:

- explicit parameters,
- a stored metric-shape decision, or
- refusal/clarification when the schema cannot support the definition faithfully

## Expected behavior

Glossary SQL generation should stop treating "generate something plausible" as success.

For each glossary term, the process should do one of the following:

1. Generate SQL that faithfully matches the written definition and the ontology-backed schema.
2. Generate parameterized SQL when the definition clearly requires user-supplied inputs (for example, time windows or geographic bounds).
3. Refuse generation and mark the entry for clarification when the definition is ambiguous or not answerable from the schema without making extra business assumptions.

## Actual behavior

The current process appears to finalize glossary SQL even when:

- the chosen join path ignores available ontology relationships,
- the metric definition requires parameters that are missing,
- the output metric is semantically different from the written definition, or
- the schema does not support the stated calculation without proxy assumptions.

## Current conclusion

This does not look like a fundamental failure of ontology-based text-to-SQL.

The ontology was sufficient to derive correct or defensible SQL for several entries, including some of the ones where the stored glossary SQL failed. The larger issue is that the glossary generation process lacks a hard decision gate for:

- faithful answerability from the current schema,
- required parameterization,
- ambiguity detection, and
- refusal when the definition cannot be implemented without inventing business assumptions.

## Terms that appear acceptable as currently generated

- `Inventory Availability`
- `Order Completion Rate`
- `Order Fulfillment Rate`
- `Return Rate`

## Terms that need follow-up

- `Customer Engagement`
- `Distribution Center Efficiency`
- `Inventory Turnover`
- `Product Sales Velocity`
- `Traffic Source Conversion`
- `User Location Density`
