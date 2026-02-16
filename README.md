# ekaya-engine

Ekaya Engine connects AI to databases safely and securely. It is an MCP Server that gives you full control over the tools available to the MCP Client, with intelligent ontology creation optimized _for LLMs by LLMs_. Deploy on localhost for personal database access during application development or data engineering, or into your infrastructure for access by business users or AI agents.

Built with Go, React, and PostgreSQL with multi-tenant isolation via Row-Level Security.

## Quickstart

The quickstart image bundles PostgreSQL and Ekaya Engine into a single container that runs on localhost port 3443:

```bash
make run-quickstart
```

Open your browser to [http://localhost:3443](http://localhost:3443) to sign in and provision a project.

## Deployment

Since Ekaya Engine connects to your database, it needs to run in an environment:

1. That has network access to the database,
1. Where business users can access it; and
1. That has a Fully Qualified Domain Name (FQDN) with TLS certificates matching that domain.

These are security requirements so that data never leaves your network boundary, users can authentication via OAuth, and you have full control over _who_ accesses _what data_.

### Docker image

The easiest path to production. Assemble your config and TLS certs, build a self-contained image, and deploy it anywhere. See [deploy/docker/](deploy/docker/) for the full guide.

### Build from source

Build the Go binary with embedded UI and deploy anywhere. See [deploy/source/](deploy/source/) for the full guide.

## Development

### Option A — Dev Container (recommended)

Open this repo in VS Code and select **Reopen in Container** when prompted (or run `Dev Containers: Reopen in Container` from the command palette). This starts a pre-configured environment with Go 1.25, Node 22, and PostgreSQL 17 — no local setup required.

Once inside the container:

```bash
make dev-server  # Terminal 1: Go API with auto-reload (port 3443)
make dev-ui      # Terminal 2: UI with hot reload (port 5173)
```

Ports 3443 and 5173 are forwarded automatically. Open [http://localhost:3443](http://localhost:3443) in your browser.

### Option B — Local Development

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

### Secrets

Two secrets are required in `config.yaml` (or as environment variables):

| Secret | Env Var | Purpose |
|--------|---------|---------|
| `project_credentials_key` | `PROJECT_CREDENTIALS_KEY` | Encrypts datasource credentials. **Cannot be changed after storing credentials.** |
| `oauth_session_secret` | `OAUTH_SESSION_SECRET` | Signs OAuth session cookies. All servers in a cluster must share the same value. |

For development, the defaults in `config.yaml.example` work. For production:

```bash
openssl rand -base64 32
```
