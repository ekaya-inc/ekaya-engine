# ekaya-engine

Regional controller for the Ekaya platform — Go backend with React frontend, multi-tenant isolation via PostgreSQL Row-Level Security.

## Prerequisites

- Go 1.25
- Node.js >= 22.0.0
- PostgreSQL 17

## Setup

### 1. Install dependencies

```bash
cd ui && npm install && cd ..
```

### 2. Configure

```bash
cp config.yaml.example config.yaml
```

The defaults work for local development. Optionally create `.env` from `.env.example` for environment overrides.

### 3. Set up PostgreSQL

**Option A — Docker (easiest):**

```bash
make dev-up
```

Starts PostgreSQL 17 on port 6432 (user: `ekaya`, password: `localdev`, database: `ekaya_engine`).

Set your environment to connect:

```bash
export PGHOST=localhost PGPORT=6432 PGUSER=ekaya PGPASSWORD=localdev PGDATABASE=ekaya_engine
```

**Option B — Local PostgreSQL:**

The `DANGER-recreate-database` target creates the database and an RLS-safe application role:

```bash
cp .env.example .env
# Edit .env: set PGPASSWORD and PROJECT_CREDENTIALS_KEY
source .env
CONFIRM=YES make DANGER-recreate-database
```

This connects as your system superuser and creates:
- Database `ekaya_engine`
- Role `ekaya` with `NOSUPERUSER NOBYPASSRLS`
- Grants on the database and public schema

### 4. Start development

```bash
make dev-server  # Terminal 1: Go API with auto-reload (port 3443)
make dev-ui      # Terminal 2: UI with hot reload (port 5173)
```

Migrations run automatically on server startup.

## Row-Level Security

The `ekaya` database role is intentionally non-superuser with `NOBYPASSRLS`. All tenant-scoped tables enforce RLS using a `app.current_project_id` GUC. The application sets this GUC per-request so the database enforces tenant isolation even if application code has bugs.

When querying directly via `psql`, set the tenant context first:

```sql
SELECT set_config('app.current_project_id', '<project-id>', false);
SELECT * FROM engine_ontologies;
```

Tables without RLS (admin tables): `engine_projects`, `engine_users`.

## Secrets

Two secrets are required in `config.yaml` (or as environment variables):

| Secret | Env Var | Purpose |
|--------|---------|---------|
| `project_credentials_key` | `PROJECT_CREDENTIALS_KEY` | Encrypts datasource credentials. **Cannot be changed after storing credentials.** |
| `oauth_session_secret` | `OAUTH_SESSION_SECRET` | Signs OAuth session cookies. All servers in a cluster must share the same value. |

For development, the defaults in `config.yaml.example` work. For production:

```bash
openssl rand -base64 32
```

## Testing

```bash
make test-short  # Unit tests only (no Docker)
make check       # Full suite: format, lint, typecheck, unit + integration tests (requires Docker)
```

## Deployment

Push to `main` deploys to dev. Push to `prod` deploys to production.

### Quickstart Docker (all-in-one)

```bash
make quickstart-run
```

Builds and runs an all-in-one image with embedded PostgreSQL on port 3443.
