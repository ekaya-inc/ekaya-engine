-- Enable Row Level Security for multi-tenant isolation
-- RLS ensures tenant data isolation at the database level (defense in depth)

-- Enable RLS on tenant-scoped tables
ALTER TABLE engine_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_projects ENABLE ROW LEVEL SECURITY;

-- Policy for engine_users:
-- Service mode (NULL): full access for central service to add admin users
-- User mode: restricted to their project
CREATE POLICY tenant_isolation ON engine_users
  FOR ALL
  USING (
    current_setting('app.current_project_id', true) IS NULL
    OR project_id = current_setting('app.current_project_id', true)::uuid
  );

-- Policy for engine_projects:
-- Service mode (NULL): full access for central service
-- User mode: only their project
CREATE POLICY project_access ON engine_projects
  FOR ALL
  USING (
    current_setting('app.current_project_id', true) IS NULL
    OR id = current_setting('app.current_project_id', true)::uuid
  );
