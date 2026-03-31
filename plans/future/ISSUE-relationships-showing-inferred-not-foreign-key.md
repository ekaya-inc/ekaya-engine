# ISSUE: Relationships showing as "Inferred" when actual Foreign Keys exist

**Status:** Open
**Dataset:** the_look (PostgreSQL) — project `9d2285e1-1009-46f8-815a-bdb8d6d9224e`

## Observed

The Relationship Manager page (`/projects/{id}/relationships`) shows all 9 relationships as "Inferred" with a count of "0 Foreign Keys". However, the database has 9 actual foreign key constraints defined.

**UI shows:**
- 0 Foreign Keys
- 9 Inferred
- 0 Manual

**Database has (verified via `information_schema.table_constraints`):**

| table | column | references |
|---|---|---|
| events | user_id | users.id |
| inventory_items | product_distribution_center_id | distribution_centers.id |
| inventory_items | product_id | products.id |
| order_items | inventory_item_id | inventory_items.id |
| order_items | order_id | orders.order_id |
| order_items | product_id | products.id |
| order_items | user_id | users.id |
| orders | user_id | users.id |
| products | distribution_center_id | distribution_centers.id |

All 9 inferred relationships are correct — they match the FK constraints exactly. The issue is classification, not correctness.

## Expected

The summary should show "9 Foreign Keys, 0 Inferred, 0 Manual" since all relationships correspond to actual FK constraints in the database. Each relationship should display the "Foreign Key" badge instead of "Inferred".

## Steps to reproduce

1. Connect to the_look PostgreSQL dataset (has FK constraints defined on all relationship columns)
2. Run ontology extraction
3. Navigate to Relationship Manager
4. Observe all relationships labeled "Inferred" despite FK constraints existing

## Likely cause

The schema extraction or relationship classification step may not be querying `information_schema.table_constraints` / `information_schema.key_column_usage` to detect existing FK constraints, or it detects them but doesn't set the provenance to "foreign_key" when storing the relationship.
