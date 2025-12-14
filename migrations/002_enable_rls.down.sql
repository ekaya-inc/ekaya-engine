-- Rollback RLS policies and disable RLS

DROP POLICY IF EXISTS tenant_isolation ON engine_users;
DROP POLICY IF EXISTS project_access ON engine_projects;

ALTER TABLE engine_users DISABLE ROW LEVEL SECURITY;
ALTER TABLE engine_projects DISABLE ROW LEVEL SECURITY;
