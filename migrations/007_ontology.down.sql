-- Migration 007 DOWN: Remove ontology extraction system
-- Drops all tables in reverse dependency order

-- Drop tables with foreign key dependencies first
DROP TABLE IF EXISTS engine_ontology_questions CASCADE;
DROP TABLE IF EXISTS engine_workflow_state CASCADE;
DROP TABLE IF EXISTS engine_llm_conversations CASCADE;
DROP TABLE IF EXISTS engine_project_knowledge CASCADE;
DROP TABLE IF EXISTS engine_ontology_chat_messages CASCADE;
DROP TABLE IF EXISTS engine_ontologies CASCADE;
DROP TABLE IF EXISTS engine_ontology_workflows CASCADE;
