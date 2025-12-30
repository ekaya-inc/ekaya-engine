-- Migration 012: Add parameters column to engine_queries table
-- Enables parameterized queries with typed parameters for MCP clients

-- Add parameters column with default empty array
ALTER TABLE engine_queries
ADD COLUMN parameters JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Parameters schema structure:
-- [
--   {
--     "name": "customer_id",
--     "type": "string",
--     "description": "The customer's unique identifier",
--     "required": true,
--     "default": null
--   },
--   {
--     "name": "start_date",
--     "type": "date",
--     "description": "Start of the date range",
--     "required": false,
--     "default": "2024-01-01"
--   }
-- ]

-- Partial index for queries with parameters (performance optimization)
CREATE INDEX idx_engine_queries_has_parameters
ON engine_queries ((parameters != '[]'::jsonb))
WHERE deleted_at IS NULL;

-- Add comment documenting the parameter types supported
COMMENT ON COLUMN engine_queries.parameters IS
'Array of parameter definitions with name, type, description, required flag, and optional default value. Supported types: string, integer, decimal, boolean, date, timestamp, uuid, string[], integer[]';
