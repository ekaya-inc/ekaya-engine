#!/bin/bash
set -e

# Purge Garbage pk_match Relationships
#
# This script removes invalid pk_match relationships created by old extraction code
# before defensive filtering was added. These relationships incorrectly identify
# metric columns (costs, counts, ratings) as foreign keys.

echo "Purging garbage pk_match relationships..."

# Count before deletion
BEFORE=$(psql -tAX -c "SELECT COUNT(*) FROM engine_entity_relationships WHERE detection_method = 'pk_match';")
echo "Total pk_match relationships before: $BEFORE"

# Delete garbage relationships
# Categories being removed:
# 1. Metric columns: cost, total_revenue, etc.
# 2. Count columns: app_launches, visits, profile_views, etc.
# 3. Rating columns: rating, reviewee_rating, etc.
# 4. Level columns: mod_level, reporter_mod_level, etc.
# 5. Aggregate columns: num_users, visible_days_trigger_days, etc.

psql <<SQL
DELETE FROM engine_entity_relationships
WHERE detection_method = 'pk_match'
AND source_column_name IN (
    -- Metric/cost columns
    'cost',
    'total_revenue',

    -- Count/engagement columns
    'app_launches',
    'visits',
    'profile_views',
    'sign_ins',
    'asset_views',
    'engagements',
    'profile_updates',
    'redirects',

    -- Rating/score columns
    'rating',
    'reviewee_rating',

    -- Level columns
    'mod_level',
    'reporter_mod_level',

    -- Aggregate columns
    'num_users',
    'visible_days_trigger_days'
);
SQL

# Count after deletion
AFTER=$(psql -tAX -c "SELECT COUNT(*) FROM engine_entity_relationships WHERE detection_method = 'pk_match';")
DELETED=$((BEFORE - AFTER))

echo "Total pk_match relationships after: $AFTER"
echo "Deleted: $DELETED garbage relationships"

# Show sample of remaining pk_match relationships (if any)
if [ "$AFTER" -gt 0 ]; then
    echo ""
    echo "Sample of remaining pk_match relationships:"
    psql -c "SELECT source_column_table, source_column_name, target_column_table, target_column_name FROM engine_entity_relationships WHERE detection_method = 'pk_match' LIMIT 10;"
fi

echo ""
echo "✓ Purge complete"
