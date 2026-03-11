-- Restore query_id as NOT NULL with FK constraint.
-- Delete any rows with NULL query_id first (ad-hoc execution logs).

DELETE FROM engine_query_executions WHERE query_id IS NULL;

ALTER TABLE engine_query_executions
    ALTER COLUMN query_id SET NOT NULL;

ALTER TABLE engine_query_executions
    ADD CONSTRAINT engine_query_executions_query_id_fkey
    FOREIGN KEY (query_id) REFERENCES engine_queries(id) ON DELETE CASCADE;
