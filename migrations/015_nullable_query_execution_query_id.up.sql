-- Make query_id nullable in engine_query_executions to support ad-hoc queries
-- (executed via MCP query/execute tools) that have no associated approved query.

ALTER TABLE engine_query_executions
    DROP CONSTRAINT engine_query_executions_query_id_fkey;

ALTER TABLE engine_query_executions
    ALTER COLUMN query_id DROP NOT NULL;
