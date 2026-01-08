-- Migration 038 rollback: Drop query executions history table

DROP TABLE IF EXISTS engine_query_executions CASCADE;
