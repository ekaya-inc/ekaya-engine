# Development

To build and run from source.

## Option A — Dev Container (recommended)

Open this repo in VS Code and select **Reopen in Container** when prompted (or run `Dev Containers: Reopen in Container` from the command palette). This starts a pre-configured environment with Go 1.25, Node 22, and PostgreSQL 17 — no local setup required.

Once inside the container:

```bash
make dev-server
```

Open [http://localhost:3443](http://localhost:3443) in your browser.

## Option B — Local Development

#### Prerequisites

- Go 1.25
- Node.js >= 22.0.0
- PostgreSQL >= 14

#### Setup

**1. Install dependencies**

```bash
cd ui && npm install && cd ..
```

**2. Configure**

```bash
cp config.yaml.example config.yaml
```

The defaults work for local development. Optionally create `.env` from `.env.example` for environment overrides.

**3. Set up PostgreSQL**

Docker (easiest):

```bash
make dev-up
```

This starts PostgreSQL 17 in a container.

Or use a local PostgreSQL — the `DANGER-recreate-database` target creates the database and an RLS-safe application role:

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

**4. Start development**

```bash
make dev-server  # Terminal 1: Go API with auto-reload (port 3443)
make dev-ui      # Terminal 2: UI with hot reload (port 5173)
```

Migrations run automatically on server startup.

### Testing

```bash
make test-short  # Unit tests only (no Docker)
make check       # Full suite: format, lint, typecheck, unit + integration tests (requires Docker)
```

### Row-Level Security

The `ekaya` database role is intentionally non-superuser with `NOBYPASSRLS`. All tenant-scoped tables enforce RLS using a `app.current_project_id` GUC. The application sets this GUC per-request so the database enforces tenant isolation even if application code has bugs.

When querying directly via `psql`, set the tenant context first:

```sql
SELECT set_config('app.current_project_id', '<project-id>', false);
SELECT * FROM engine_ontologies;
```

Tables without RLS (admin tables): `engine_projects`, `engine_users`.
