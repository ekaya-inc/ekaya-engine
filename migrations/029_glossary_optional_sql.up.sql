-- 029_glossary_optional_sql.up.sql
-- Make defining_sql optional for glossary terms.
-- Not every business term has a direct SQL representation — some are
-- schema-derived concepts discovered during ontology analysis.

ALTER TABLE engine_business_glossary ALTER COLUMN defining_sql DROP NOT NULL;
ALTER TABLE engine_business_glossary ALTER COLUMN defining_sql SET DEFAULT '';

COMMENT ON COLUMN engine_business_glossary.defining_sql IS 'Optional executable SQL that defines this metric (SELECT statement). Empty for conceptual terms without direct SQL representation.';
