-- 017_glossary_enrichment_status.up.sql
-- Add enrichment status tracking to glossary terms

ALTER TABLE engine_business_glossary
ADD COLUMN enrichment_status text DEFAULT 'pending'::text,
ADD COLUMN enrichment_error text;

COMMENT ON COLUMN engine_business_glossary.enrichment_status IS 'Status of SQL enrichment: pending, success, or failed';
COMMENT ON COLUMN engine_business_glossary.enrichment_error IS 'Error message if enrichment failed, NULL otherwise';

-- Add constraint for valid status values
ALTER TABLE engine_business_glossary
ADD CONSTRAINT engine_business_glossary_enrichment_status_check
CHECK (enrichment_status IS NULL OR enrichment_status = ANY (ARRAY['pending'::text, 'success'::text, 'failed'::text]));

-- Index for filtering by enrichment status (useful for finding failed terms)
CREATE INDEX idx_business_glossary_enrichment_status ON engine_business_glossary USING btree (enrichment_status);
