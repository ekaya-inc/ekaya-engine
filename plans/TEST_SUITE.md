# TEST SUITE: MCP Verification for The Look - 2026-03-16

This suite replaces the older `tikr`-specific checks. It is scoped to the tutorial `the_look` datasource now wired into the local Ekaya MCP server.

Use this file when the user says:
- "Run The Look MCP test suite"
- "Test the MCP server against the tutorial database"
- "Review and answer ontology questions for the tutorial dataset"

The suite has two goals:
1. Verify the MCP read path against the live `the_look` project.
2. Review pending ontology questions and prove whether each one could have been answered from the data alone before using source material or a documented best guess.

## Source Material

Use these as the non-data sources of truth:

- `/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-tutorials/exploring/tutorial-01-creating-new-database-from-csvs/data_dictionary.csv`
- `/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-tutorials/exploring/tutorial-01-creating-new-database-from-csvs/THE_LOOK_README.md`

Use the live Ekaya MCP server as the runtime source of truth for:

- current project and datasource health
- schema and ontology state
- pending ontology questions
- observed data values and distributions

## Quick Start

When running this suite:

1. Start with `health()` and confirm the datasource is `the_look`.
2. Run Phases 1 through 5 in order.
3. In Phase 6, audit every pending ontology question:
   - first decide whether the answer was already available from MCP data access alone
   - if yes, record a `question-generation miss`
   - if no, use the source material above
   - if the source material still does not answer it, resolve with a clearly labeled best guess or use `escalate_ontology_question` if the goal is to preserve strict human review
4. Always update ontology metadata before `resolve_ontology_question`.
5. Record failures and newly discovered data quirks at the bottom of the run log.

## Live Baseline Snapshot

These values were observed from the live MCP server on 2026-03-16 and are the current sanity baseline for this tutorial project:

- `health()` returned `engine=healthy`, `version=dev`, and `datasource.name=the_look`
- `get_context(depth="domain")` returned `table_count=7` and `column_count=75`
- `list_glossary()` returned `count=0`
- `list_pending_changes(status="pending")` returned `count=0`
- `list_ontology_questions(status="pending")` returned `total_count=20`
- `list_ontology_questions(status="answered")` returned `total_count=0`

Current row-count snapshot:

| Table | Expected Rows |
|-------|---------------|
| `distribution_centers` | 10 |
| `events` | 12658 |
| `inventory_items` | 1764 |
| `order_items` | 1764 |
| `orders` | 1214 |
| `products` | 1708 |
| `users` | 1000 |

## Phase 1: Health, Schema, and Discovery

Run these first.

```text
1. health()
   Verify:
   - engine is "healthy"
   - datasource status is "connected"
   - datasource name is "the_look"

2. echo(message="the look mcp test")
   Verify:
   - the payload is echoed back unchanged

3. get_schema()
   Verify:
   - exactly 7 tables are listed
   - the row counts match the baseline snapshot
   - relationships include:
     - products.distribution_center_id -> distribution_centers.id
     - orders.user_id -> users.id
     - events.user_id -> users.id
     - order_items.order_id -> orders.order_id
     - order_items.inventory_item_id -> inventory_items.id

4. get_context(depth="domain", include_relationships=true)
   Verify:
   - description clearly identifies retail / ecommerce behavior
   - table_count is 7
   - column_count is 75
   - project_knowledge mentions order lifecycle and traffic sources

5. get_context(depth="tables", include_relationships=true)
   Verify:
   - all 7 tables appear
   - table types make sense:
     - `orders`, `order_items`, `inventory_items`, `events` are transactional
     - `users`, `products`, `distribution_centers` are reference

6. get_ontology(depth="tables", include_relationships=true)
   Verify:
   - each table has a description
   - enum-bearing columns are marked as having enum values
   - `users.gender`, `events.browser`, `events.traffic_source`, `events.event_type`, `orders.status`, `order_items.status`, and `products.department` are surfaced as attributes

7. search_schema(query="user", limit=10)
   Verify:
   - the `users` table is the top table hit
   - `user_id` columns appear in `events`, `orders`, and `order_items`

8. search_schema(query="sold_at", limit=10)
   Verify:
   - `inventory_items.sold_at` is returned as the matching column
```

## Phase 2: Query and Query-Tool Checks

Use `query()` for the dataset-specific checks below. If the current tool loadout also exposes `validate()`, `sample()`, or `explain_query()`, run them against the same SQL.

### 2A. Enumerations and Value Sets

```sql
SELECT 'users.gender' AS column_name, array_agg(DISTINCT gender ORDER BY gender) AS values FROM users
UNION ALL
SELECT 'events.traffic_source' AS column_name, array_agg(DISTINCT traffic_source ORDER BY traffic_source) AS values FROM events
UNION ALL
SELECT 'events.event_type' AS column_name, array_agg(DISTINCT event_type ORDER BY event_type) AS values FROM events
UNION ALL
SELECT 'products.department' AS column_name, array_agg(DISTINCT department ORDER BY department) AS values FROM products
UNION ALL
SELECT 'events.browser' AS column_name, array_agg(DISTINCT browser ORDER BY browser) AS values FROM events
```

Verify:

- `users.gender` = `F`, `M`
- `events.traffic_source` = `Adwords`, `Email`, `Facebook`, `Organic`, `YouTube`
- `events.event_type` = `cart`, `department`, `home`, `product`, `purchase`
- `products.department` = `Men`, `Women`
- `events.browser` = `Chrome`, `Firefox`, `IE`, `Other`, `Safari`

### 2B. Coordinate Completeness and Ranges

```sql
SELECT 'distribution_centers' AS table_name,
       MIN(latitude) AS min_latitude,
       MAX(latitude) AS max_latitude,
       MIN(longitude) AS min_longitude,
       MAX(longitude) AS max_longitude,
       COUNT(*) FILTER (WHERE latitude IS NULL OR longitude IS NULL) AS missing_coordinate_rows
FROM distribution_centers
UNION ALL
SELECT 'users' AS table_name,
       MIN(latitude) AS min_latitude,
       MAX(latitude) AS max_latitude,
       MIN(longitude) AS min_longitude,
       MAX(longitude) AS max_longitude,
       COUNT(*) FILTER (WHERE latitude IS NULL OR longitude IS NULL) AS missing_coordinate_rows
FROM users
```

Verify:

- `distribution_centers` has `missing_coordinate_rows = 0`
- `users` has `missing_coordinate_rows = 0`
- all observed latitudes are within `[-90, 90]`
- all observed longitudes are within `[-180, 180]`

### 2C. Price Relationships

```sql
SELECT 'products' AS table_name,
       COUNT(*) AS row_count,
       COUNT(*) FILTER (WHERE retail_price < cost) AS retail_below_cost,
       MIN(retail_price - cost) AS min_margin,
       MAX(retail_price - cost) AS max_margin
FROM products
UNION ALL
SELECT 'inventory_items' AS table_name,
       COUNT(*) AS row_count,
       COUNT(*) FILTER (WHERE product_retail_price < cost) AS retail_below_cost,
       MIN(product_retail_price - cost) AS min_margin,
       MAX(product_retail_price - cost) AS max_margin
FROM inventory_items
UNION ALL
SELECT 'order_items' AS table_name,
       COUNT(*) AS row_count,
       COUNT(*) FILTER (WHERE sale_price < 0) AS retail_below_cost,
       MIN(sale_price) AS min_margin,
       MAX(sale_price) AS max_margin
FROM order_items
```

Verify:

- `products.retail_below_cost = 0`
- `inventory_items.retail_below_cost = 0`
- `order_items.sale_price` has no negative values
- current observed `sale_price` range is positive

### 2D. Status and Timestamp Completeness

```sql
SELECT
  COUNT(*) AS row_count,
  COUNT(*) FILTER (WHERE shipped_at IS NOT NULL AND shipped_at < created_at) AS shipped_before_created,
  COUNT(*) FILTER (WHERE delivered_at IS NOT NULL AND shipped_at IS NOT NULL AND delivered_at < shipped_at) AS delivered_before_shipped,
  COUNT(*) FILTER (WHERE returned_at IS NOT NULL AND delivered_at IS NOT NULL AND returned_at < delivered_at) AS returned_before_delivered,
  COUNT(*) FILTER (WHERE status IN ('Shipped','Complete','Returned') AND shipped_at IS NULL) AS shipped_status_missing_shipped_at,
  COUNT(*) FILTER (WHERE status = 'Complete' AND delivered_at IS NULL) AS complete_missing_delivered_at,
  COUNT(*) FILTER (WHERE status = 'Returned' AND returned_at IS NULL) AS returned_missing_returned_at
FROM order_items
```

Verify:

- `shipped_status_missing_shipped_at = 0`
- `complete_missing_delivered_at = 0`
- `returned_missing_returned_at = 0`
- current baseline has `shipped_before_created = 359`
- current baseline has `delivered_before_shipped = 0`
- current baseline has `returned_before_delivered = 0`

The `shipped_before_created = 359` result is a known data quirk for this seed. Do not use that anomaly to redefine the intended business semantics of the lifecycle timestamps.

### 2E. Inventory Linkage and `sold_at`

```sql
SELECT
  COUNT(*) AS inventory_row_count,
  COUNT(*) FILTER (WHERE oi.inventory_item_id IS NOT NULL) AS inventory_rows_linked_to_order_items,
  COUNT(*) FILTER (WHERE sold_at IS NULL) AS sold_at_nulls,
  COUNT(*) FILTER (WHERE sold_at IS NOT NULL) AS sold_at_present
FROM inventory_items ii
LEFT JOIN order_items oi ON oi.inventory_item_id = ii.id
```

Verify:

- all 1764 `inventory_items` rows are linked to `order_items`
- current baseline has `sold_at_nulls = 1764`
- current baseline has `sold_at_present = 0`

This means the current seed leaves `inventory_items.sold_at` completely unpopulated even though every inventory item is linked to an order item. Record this as a data quirk during the run.

### 2F. Completeness Spot Checks

```sql
SELECT
  (SELECT COUNT(*) FILTER (WHERE street_address IS NULL OR BTRIM(street_address) = '') FROM users) AS missing_street_address,
  (SELECT COUNT(*) FILTER (WHERE browser IS NULL OR BTRIM(browser) = '') FROM events) AS missing_browser,
  (SELECT COUNT(*) FILTER (WHERE status IN ('Shipped','Complete','Returned') AND shipped_at IS NULL) FROM orders) AS shipped_orders_missing_shipped_at
```

Verify:

- `missing_street_address = 0`
- `missing_browser = 0`
- `shipped_orders_missing_shipped_at = 0`

### 2G. Optional Generic Query-Tool Checks

If these tools are enabled in the current loadout, run them too:

```text
- validate(sql="SELECT status, COUNT(*) FROM orders GROUP BY status")
  Expected: valid=true

- validate(sql="SELECT * FROM nonexistent_table")
  Expected: valid=false

- explain_query(sql="SELECT status, COUNT(*) FROM orders GROUP BY status")
  Expected: returns a plan without execution errors

- sample(table="orders", limit=3)
  Expected: 3 rows, including `status`, `created_at`, `shipped_at`, `delivered_at`, and `returned_at`
```

## Phase 3: Metadata and Probe Checks

```text
1. get_column_metadata(table="inventory_items", column="sold_at")
   Verify:
   - data_type is `timestamp with time zone`
   - column is nullable
   - metadata description is "The timestamp when the inventory item was sold"
   - semantic_type is `event_time`

2. probe_column(table="inventory_items", column="sold_at")
   Verify:
   - description matches the metadata
   - purpose is timestamp
   - semantic_type is `event_time`

3. probe_columns(columns=[
     {"table":"users","column":"gender"},
     {"table":"events","column":"traffic_source"},
     {"table":"products","column":"department"}
   ])
   Verify:
   - `users.gender` exposes enum labels `F=Female`, `M=Male`
   - `events.traffic_source` exposes the five observed sources
   - `products.department` exposes `Men` and `Women`
```

If `probe_columns()` already surfaces the answer, the matching ontology question should not remain pending.

## Phase 4: List and State Checks

```text
1. list_glossary()
   Expected baseline:
   - `count=0`
   - `terms=[]`

2. list_pending_changes(status="pending", limit=20)
   Expected baseline:
   - `count=0`
   - `changes=[]`

3. list_ontology_questions(status="pending", limit=50)
   Expected baseline:
   - current total_count is 20
   - every item has `id`, `question`, `category`, `priority`, and `context`

4. list_ontology_questions(status="answered", limit=50)
   Expected baseline:
   - current total_count is 0
```

## Phase 5: Ontology Question Audit Rules

This is the important project-specific addition.

For every pending ontology question:

1. Pull the question with `list_ontology_questions(status="pending")`.
2. Gather evidence from MCP data access first:
   - `query`
   - `probe_column`
   - `probe_columns`
   - `get_context`
   - `get_ontology`
3. Decide whether the answer was already available from data alone.
4. If yes:
   - classify it as a `question-generation miss`
   - answer it from the observed data
   - document why it should not have remained pending
5. If no:
   - use `data_dictionary.csv` and `THE_LOOK_README.md`
6. If the source material still does not answer it:
   - resolve it with an explicit best guess and mark the inference in `resolution_notes`
   - or use `escalate_ontology_question` if the purpose of the run is to preserve strict human review instead of completing the ontology

Never call `resolve_ontology_question` before one of these metadata updates:

- `update_column`
- `update_columns`
- `update_project_knowledge`
- `update_table`
- `update_glossary_term`

Question status changes without metadata updates do not persist the learned knowledge.

## Phase 6: Current Pending Question Resolution Matrix

Use this matrix for the current `the_look` project.

### 6A. Questions That Should Be Answered From Data Alone

These are `question-generation miss` candidates.

- `users.gender`: `probe_columns()` already infers `F=Female` and `M=Male`. Resolve from data plus obvious enum expansion.
- `distribution_centers latitude/longitude invalid or missing`: the coordinate query shows `missing_coordinate_rows=0` and all observed values are valid coordinates.
- `products.department` possible values: the enumeration query shows only `Men` and `Women` in the current dataset.
- `inventory_items.sold_at` null percentage: the inventory linkage query shows `sold_at_nulls=1764` and `sold_at_present=0`, so the current percentage is `100%`.
- `orders shipped_at missing even though shipped`: current orders data shows all `Shipped`, `Complete`, and `Returned` rows have `shipped_at`; missing values are confined to `Processing` and `Cancelled`.
- `users latitude/longitude present and valid`: the coordinate query shows zero missing values and values within valid latitude/longitude bounds.
- `users.street_address` missing values: current data shows zero null or blank rows.
- `events.browser` missing or inconsistent values: current data shows zero null or blank rows and only `Chrome`, `Firefox`, `IE`, `Other`, and `Safari`.

Recommended metadata actions:

- `update_column` or `update_columns` for enum columns such as `users.gender`, `products.department`, and `events.browser`
- `update_project_knowledge` for current-seed data facts such as `inventory_items.sold_at is 100% null in the current tutorial seed`

### 6B. Questions That Need Source Material

Use the source files above after confirming the data alone is insufficient.

- `latitude/longitude unit of measurement`: best supported answer is decimal degrees. The source material treats these as geographic coordinates, not radians.
- `traffic_source meanings`: these are acquisition channels through which users arrived at the site. Current observed values are `Adwords`, `Email`, `Facebook`, `Organic`, and `YouTube`.
- `relationship between created_at, shipped_at, delivered_at, returned_at`: the source material describes an order lifecycle of created -> shipped -> delivered, with return as an optional later event, and null timestamps meaning the event has not happened yet.
- `cost` vs `retail_price`: the README says all prices are USD and there is no fixed markup; current data happens to have no rows with `retail_price < cost`.
- `product_retail_price` vs `cost`: same answer as above, but for the denormalized inventory copy.
- `sold_at is null`: the README says null means the inventory item is still in stock or not yet sold. The current seed does not populate `sold_at`, so note that the source gives the intended meaning while the data shows an implementation gap.
- `event_type` categorization: the README describes website activity as page views, product views, cart additions, and purchases. In this seed, those categories appear as `home`, `department`, `product`, `cart`, and `purchase`.
- `sale_price` constraints: the README says prices are in USD. Current data shows only positive sale prices, with an observed range of `1.82` to `499`, but the source material does not define a fixed minimum or maximum.

Recommended metadata actions:

- `update_project_knowledge` for lifecycle, currency, traffic-source, and null-timestamp semantics
- `update_column` for enum descriptions on `events.traffic_source` and `events.event_type`

### 6C. Questions That Require Best Guess or Explicit Inference

These are not answered cleanly by the current data or the provided source material. If the goal of the run is completion rather than strict escalation, resolve them with explicit inference notes.

- `acceptable latitude/longitude range for distribution centers`: best guess is standard valid earth coordinates, with current tutorial data falling inside US warehouse ranges.
- `partial returns for order_items`: best guess is that partial returns are modeled across multiple `order_items` rows, not within a single row, because each row links to one `inventory_item`.
- `policy for marking an order as returned`: best guess is that `returned_at` records when the return is finalized in the system, not necessarily when the warehouse physically receives the item.
- `policy for marking an order as delivered`: best guess is that `delivered_at` records the system's delivery confirmation event, not proven customer receipt semantics.

If you resolve these instead of escalating them, say so directly in `resolution_notes`.

## Phase 7: Suggested Question-Resolution Workflow

Use this order when actually answering the questions:

```text
1. list_ontology_questions(status="pending", limit=50)
2. run the Phase 2 evidence queries
3. classify each question:
   - data-only
   - source-backed
   - inference
4. persist the answer:
   - enum / column meaning -> update_column or update_columns
   - table-level meaning -> update_table
   - cross-table rule / convention -> update_project_knowledge
5. resolve the question:
   - resolve_ontology_question(question_id=..., resolution_notes="Answered from data query ...")
   - or resolution_notes="Answered from THE_LOOK_README.md ..."
   - or resolution_notes="Inference based on row grain and available docs ..."
6. rerun list_ontology_questions(status="pending") and confirm the count dropped
7. rerun get_context(depth="domain") or get_column_metadata(...) to confirm the knowledge is now visible
```

## Known Project-Specific Quirks

Document these if they still appear during a run:

- `inventory_items.sold_at` is null for every row in the current seed
- `order_items` currently has 359 rows where `shipped_at < created_at`
- `list_glossary()` is empty in the current baseline, so glossary-specific read tests are not meaningful unless the run creates a temporary term first

## Report Template

Copy this section when documenting a run:

```markdown
## Test Run: YYYY-MM-DD

**Tester:** [Codex session / Human]
**Ekaya Version:** [from `health()`]
**Datasource:** [from `health()`]
**Project ID:** [from `health()`]

### Core MCP Results

| Phase | Result | Notes |
|------|--------|-------|
| 1. Health / Schema / Discovery | | |
| 2. Query Checks | | |
| 3. Metadata / Probe Checks | | |
| 4. List / State Checks | | |
| 5. Question Audit Rules Applied | | |
| 6. Question Resolution Pass | | |

### Question Audit Summary

| Bucket | Count | Notes |
|-------|-------|-------|
| Answerable from data alone | | |
| Needed source material | | |
| Resolved by best guess / inference | | |
| Escalated instead of resolved | | |

### Question-Generation Misses

[List every pending question that should have been answerable from data alone]

### Resolutions Applied

[List metadata updates and matching question IDs]

### Failures

[List tool failures or mismatches]

### New Issues / Data Quirks

[List anything newly discovered]
```
