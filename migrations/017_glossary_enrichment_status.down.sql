-- 017_glossary_enrichment_status.down.sql
-- Remove enrichment status tracking from glossary terms

DROP INDEX IF EXISTS idx_business_glossary_enrichment_status;

ALTER TABLE engine_business_glossary
DROP CONSTRAINT IF EXISTS engine_business_glossary_enrichment_status_check;

ALTER TABLE engine_business_glossary
DROP COLUMN IF EXISTS enrichment_status,
DROP COLUMN IF EXISTS enrichment_error;
