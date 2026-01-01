-- Migration 019: Add description field to entity relationships
-- Allows users to describe relationships when creating them through chat

ALTER TABLE engine_entity_relationships
    ADD COLUMN description TEXT;

COMMENT ON COLUMN engine_entity_relationships.description IS
    'Optional description of the relationship, typically provided when created through chat';
