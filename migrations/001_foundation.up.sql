-- 001_foundation.up.sql
-- Foundation tables: projects, users, and shared trigger function

-- Trigger function for auto-updating updated_at timestamps
CREATE FUNCTION update_updated_at_column() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

-- Projects table (core tenant entity)
CREATE TABLE engine_projects (
    id uuid NOT NULL,
    name character varying(255) NOT NULL,
    parameters jsonb DEFAULT '{}'::jsonb,
    industry_type text DEFAULT 'general',
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    status character varying(50) DEFAULT 'active'::character varying,
    PRIMARY KEY (id)
);

COMMENT ON COLUMN engine_projects.industry_type IS 'Industry classification for template selection (e.g., general, video_streaming, marketplace, creator_economy)';

CREATE INDEX idx_engine_projects_status ON engine_projects USING btree (status);

-- Users table (project access control)
CREATE TABLE engine_users (
    project_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role character varying(50) NOT NULL,
    email character varying(255),
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    PRIMARY KEY (project_id, user_id),
    CONSTRAINT engine_users_role_check CHECK ((role)::text = ANY (ARRAY['admin'::text, 'data'::text, 'user'::text])),
    CONSTRAINT engine_users_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE
);

COMMENT ON COLUMN engine_users.email IS 'Email from JWT claims when user authenticates (not unique across users)';

CREATE INDEX idx_engine_users_user ON engine_users USING btree (user_id);

-- RLS for projects (allows null project_id for admin access)
ALTER TABLE engine_projects ENABLE ROW LEVEL SECURITY;
CREATE POLICY project_access ON engine_projects FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR id = current_setting('app.current_project_id', true)::uuid);

-- RLS for users
ALTER TABLE engine_users ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON engine_users FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
