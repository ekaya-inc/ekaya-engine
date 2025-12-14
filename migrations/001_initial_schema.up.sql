-- Core projects table (admin table, no RLS)
CREATE TABLE IF NOT EXISTS engine_projects (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    parameters JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    status VARCHAR(50) DEFAULT 'active'
);

-- Users table for project access control (admin table, no RLS)
CREATE TABLE IF NOT EXISTS engine_users (
    project_id UUID REFERENCES engine_projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    role VARCHAR(50) NOT NULL CHECK (role IN ('admin', 'data', 'user')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (project_id, user_id)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_engine_projects_status ON engine_projects(status);
CREATE INDEX IF NOT EXISTS idx_engine_users_user ON engine_users(user_id);
