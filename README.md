# ekaya-engine

Ekaya Engine connects AI to databases safely and securely. It is an MCP Server that gives you full control over the tools available to the MCP Client, with intelligent ontology creation optimized for LLMs, by LLMs. Deploy on localhost for personal database access during application development or data engineering, or into your infrastructure for access by business users or AI agents.

Built with Go, React, and PostgreSQL with multi-tenant isolation via Row-Level Security.

## Prerequisites

- Go 1.25
- Node.js >= 22.0.0
- PostgreSQL >= 14 (uses `gen_random_uuid()` built-in, which requires PostgreSQL 13+; 14 is the oldest actively supported version)

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

### Build the binary

The UI is embedded in the binary. Build the frontend first, then the Go binary:

```bash
cd ui && npm install && npm run build && cd ..
go build -tags=all_adapters -ldflags="-X main.Version=$(git describe --tags --always --dirty)" -o ekaya-engine .
```

Cross-compile for other platforms by setting `GOOS` and `GOARCH`:

```bash
# Linux (amd64)
GOOS=linux GOARCH=amd64 go build -tags=all_adapters -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty)" -o ekaya-engine .

# Linux (arm64)
GOOS=linux GOARCH=arm64 go build -tags=all_adapters -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty)" -o ekaya-engine .

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -tags=all_adapters -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty)" -o ekaya-engine .

# Windows
GOOS=windows GOARCH=amd64 go build -tags=all_adapters -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty)" -o ekaya-engine.exe .
```

### Configure

Ekaya Engine reads `config.yaml` from the **current working directory**. Copy the example and edit it:

```bash
cp config.yaml.example config.yaml
```

At minimum, set `project_credentials_key` and `oauth_session_secret` to secure values:

```bash
openssl rand -base64 32  # generate a secret
```

See `config.yaml.example` for all options with detailed comments. Key settings for production:

| Setting | Env Var | Purpose |
|---------|---------|---------|
| `base_url` | `BASE_URL` | Public URL of your deployment (auto-derived if not set) |
| `tls_cert_path` | `TLS_CERT_PATH` | Path to TLS certificate (PEM) |
| `tls_key_path` | `TLS_KEY_PATH` | Path to TLS private key (PEM) |
| `bind_addr` | `BIND_ADDR` | Listen address (default `127.0.0.1`; set to `0.0.0.0` for all interfaces) |
| `port` | `PORT` | Listen port (default `3443`) |
| `project_credentials_key` | `PROJECT_CREDENTIALS_KEY` | Encrypts stored datasource credentials. **Cannot be changed after storing credentials.** |
| `oauth_session_secret` | `OAUTH_SESSION_SECRET` | Signs OAuth session cookies. All servers in a cluster must share the same value. |

All settings can be overridden with environment variables.

### TLS certificates

Ekaya Engine uses OAuth 2.1 with PKCE for authentication. PKCE requires the browser's Web Crypto API, which **only works in secure contexts** (HTTPS or `localhost`). This means:

- `http://localhost:3443` — works (browsers treat localhost as secure)
- `https://your.domain.com` — works with a valid TLS certificate
- `http://your.domain.com` — **will not work** (browser blocks Web Crypto API)

For non-localhost deployments, provide TLS certificates in `config.yaml`:

```yaml
tls_cert_path: "/path/to/cert.pem"
tls_key_path: "/path/to/key.pem"
base_url: "https://data.yourcompany.com:3443"
```

Self-signed certificates work if users' browsers trust your root CA. If TLS is terminated by a reverse proxy or load balancer, leave `tls_cert_path`/`tls_key_path` unset and set `base_url` to your public HTTPS URL.

### Run

Run the binary from the directory containing `config.yaml`:

```bash
./ekaya-engine
```

Migrations run automatically on startup. The server logs its listen address and base URL on start.

### Quickstart Docker (all-in-one)

For evaluation or single-user deployments, the quickstart image bundles PostgreSQL and Ekaya Engine:

```bash
make quickstart-run
```

This builds and runs an all-in-one image with embedded PostgreSQL on port 3443.
