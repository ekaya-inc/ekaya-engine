-- Add email column to engine_users table
-- Email is populated from JWT claims when user authenticates
-- Note: Same email can appear with multiple userIds (not unique)

ALTER TABLE engine_users ADD COLUMN email VARCHAR(255);
