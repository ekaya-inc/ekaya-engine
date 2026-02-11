-- 029_glossary_optional_sql.down.sql
-- Restore NOT NULL constraint on defining_sql.

-- Ensure no NULLs exist before adding constraint
UPDATE engine_business_glossary SET defining_sql = '' WHERE defining_sql IS NULL;

ALTER TABLE engine_business_glossary ALTER COLUMN defining_sql SET NOT NULL;
ALTER TABLE engine_business_glossary ALTER COLUMN defining_sql DROP DEFAULT;

COMMENT ON COLUMN engine_business_glossary.defining_sql IS 'Complete executable SQL that defines this metric (SELECT statement)';
