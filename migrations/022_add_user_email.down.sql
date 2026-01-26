-- Remove email column from engine_users table

ALTER TABLE engine_users DROP COLUMN IF EXISTS email;
