-- Migration 019 down: Remove description field from entity relationships

ALTER TABLE engine_entity_relationships
    DROP COLUMN IF EXISTS description;
